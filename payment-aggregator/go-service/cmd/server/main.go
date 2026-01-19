package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/joho/godotenv"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"

	"github.com/rajathongal-intangles/payment-aggregator/go-service/internal/config"
	grpcserver "github.com/rajathongal-intangles/payment-aggregator/go-service/internal/grpc"
	"github.com/rajathongal-intangles/payment-aggregator/go-service/internal/kafka"
	pb "github.com/rajathongal-intangles/payment-aggregator/go-service/pb"
)

const (
	defaultPort = "50051"
)

func main() {
	// Load .env from multiple locations
	godotenv.Load()             // current dir
	godotenv.Load("../../.env") // repo root from cmd/server/

	cfg := config.Load()

	// Get port from config
	port := cfg.GRPCPort
	if port == "" {
		port = defaultPort
	}

	// Create TCP listener
	addr := fmt.Sprintf(":%s", port)
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", addr, err)
	}

	// Create gRPC server
	grpcServer := grpc.NewServer()

	// Create and register our payment server
	paymentServer := grpcserver.NewPaymentServer()
	pb.RegisterPaymentServiceServer(grpcServer, paymentServer)

	// Enable reflection (for grpcurl and debugging)
	reflection.Register(grpcServer)

	// Seed test data (remove in production)
	paymentServer.SeedTestData()

	// Start Kafka consumer in background
	ctx, cancel := context.WithCancel(context.Background())
	consumer, err := kafka.NewConsumer(cfg, func(p *pb.Payment) {
		// This handler is called for each Kafka message
		paymentServer.AddPayment(p)
	})
	if err != nil {
		log.Printf("⚠️  Kafka consumer disabled: %v", err)
	} else {
		go func() {
			log.Printf("[KAFKA] Starting consumer for topic: %s", cfg.KafkaTopic)
			if err := consumer.Start(ctx); err != nil {
				log.Printf("[KAFKA] Consumer error: %v", err)
			}
		}()
		log.Printf("✅ Kafka consumer started (topic: %s)", cfg.KafkaTopic)
	}

	// Graceful shutdown handling
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		log.Println("\n⏳ Shutting down...")

		// 1. Stop Kafka consumer
		cancel()
		if consumer != nil {
			consumer.Close()
		}

		// 2. Close all gRPC streams (so GracefulStop doesn't hang)
		paymentServer.Shutdown()

		// 3. Stop gRPC server
		grpcServer.GracefulStop()
	}()

	// Start server
	log.Printf("🚀 gRPC server listening on %s", addr)
	log.Println("   Services: PaymentService")
	log.Println("   Methods:  GetPayment, ListPayments, StreamPayments")
	log.Printf("   Kafka:    %s -> %s", cfg.KafkaBrokers, cfg.KafkaTopic)
	log.Println("\nPress Ctrl+C to stop")

	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}

	log.Println("👋 Server stopped")
}