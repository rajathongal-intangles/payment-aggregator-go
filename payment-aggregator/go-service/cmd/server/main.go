package main

import (
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

	// Create consumer manager for on-demand topic consumption
	consumerManager := kafka.NewConsumerManager(cfg)

	// Create and register our payment server with consumer manager
	paymentServer := grpcserver.NewPaymentServer(consumerManager)
	pb.RegisterPaymentServiceServer(grpcServer, paymentServer)

	// Enable reflection (for grpcurl and debugging)
	reflection.Register(grpcServer)

	// Seed test data for default topic (remove in production)
	paymentServer.SeedTestData(cfg.KafkaTopic)

	// Graceful shutdown handling
	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		<-sigCh

		log.Println("\nShutting down...")

		// 1. Close all consumers and gRPC streams
		paymentServer.Shutdown()

		// 2. Stop gRPC server
		grpcServer.GracefulStop()
	}()

	// Start server
	log.Printf("gRPC server listening on %s", addr)
	log.Println("Services: PaymentService")
	log.Println("Methods:  GetPayment, ListPayments, StreamPayments, GetTopics")
	log.Printf("Kafka:    %s (consumers created on-demand)", cfg.KafkaBrokers)
	log.Println("\nClients subscribe to topics via StreamPayments(topic)")
	log.Println("Consumers are created when first client subscribes")
	log.Println("Consumers are destroyed when last client disconnects")
	log.Println("\nPress Ctrl+C to stop")

	if err := grpcServer.Serve(listener); err != nil {
		log.Fatalf("Failed to serve: %v", err)
	}

	log.Println("Server stopped")
}
