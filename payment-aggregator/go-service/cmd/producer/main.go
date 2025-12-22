package main

import (
	"flag"
	"log"
	"time"

	"github.com/joho/godotenv"
	"github.com/rajathongal-intangles/payment-aggregator/go-service/internal/config"
	"github.com/rajathongal-intangles/payment-aggregator/go-service/internal/kafka"
)

func main() {
	count := flag.Int("count", 5, "Number of payments")
	interval := flag.Duration("interval", time.Second, "Interval between messages")
	flag.Parse()

	// Try loading .env from multiple locations
	godotenv.Load()           // current dir
	godotenv.Load("../../.env") // repo root from cmd/producer/

	cfg := config.Load()
	log.Printf("[CONFIG] Brokers: %s, Topic: %s, SSL: %v", cfg.KafkaBrokers, cfg.KafkaTopic, cfg.KafkaSSL)

	producer, err := kafka.NewProducer(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer producer.Close()

	for i := 0; i < *count; i++ {
		payment := kafka.GenerateTestPayment()
		log.Printf("[SEND] %s | %s | %.2f %s",
			payment.ID, payment.Provider, payment.Amount, payment.Currency)
		if err := producer.SendPayment(payment); err != nil {
			log.Printf("[ERROR] Failed to send: %v", err)
		}
		time.Sleep(*interval)
	}

	producer.Flush(10000)
	log.Println("✅ Done!")
}