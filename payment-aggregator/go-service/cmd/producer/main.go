package main

import (
	"flag"
	"fmt"
	"log"
	"time"

	confluentkafka "github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/joho/godotenv"
	"github.com/rajathongal-intangles/payment-aggregator/go-service/internal/config"
	"github.com/rajathongal-intangles/payment-aggregator/go-service/internal/kafka"
)

func main() {
	count := flag.Int("count", 5, "Number of payments")
	interval := flag.Duration("interval", time.Second, "Interval between messages")
	invalid := flag.Int("invalid", 0, "Number of invalid messages to send (for DLQ testing)")
	flag.Parse()

	// Try loading .env from multiple locations
	godotenv.Load()             // current dir
	godotenv.Load("../../.env") // repo root from cmd/producer/

	cfg := config.Load()
	log.Printf("[CONFIG] Brokers: %s, Topic: %s, SSL: %v", cfg.KafkaBrokers, cfg.KafkaTopic, cfg.KafkaSSL)

	producer, err := kafka.NewProducer(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer producer.Close()

	// Send valid payments
	for i := 0; i < *count; i++ {
		payment := kafka.GenerateTestPayment()
		log.Printf("[SEND] %s | %s | %.2f %s",
			payment.ID, payment.Provider, payment.Amount, payment.Currency)
		if err := producer.SendPayment(payment); err != nil {
			log.Printf("[ERROR] Failed to send: %v", err)
		}
		time.Sleep(*interval)
	}

	// Send invalid messages for DLQ testing
	if *invalid > 0 {
		log.Printf("\n[DLQ TEST] Sending %d invalid messages...", *invalid)
		sendInvalidMessages(cfg, *invalid)
	}

	producer.Flush(10000)
	log.Println("✅ Done!")
}

// sendInvalidMessages sends malformed messages to test DLQ
func sendInvalidMessages(cfg *config.Config, count int) {
	configMap := &confluentkafka.ConfigMap{
		"bootstrap.servers": cfg.KafkaBrokers,
		"acks":              "all",
	}

	if cfg.KafkaSSL {
		configMap.SetKey("security.protocol", "SASL_SSL")
	}
	if cfg.HasSASL() {
		configMap.SetKey("sasl.mechanisms", cfg.KafkaSASLMechanism)
		configMap.SetKey("sasl.username", cfg.KafkaSASLUsername)
		configMap.SetKey("sasl.password", cfg.KafkaSASLPassword)
	}

	rawProducer, err := confluentkafka.NewProducer(configMap)
	if err != nil {
		log.Printf("[ERROR] Failed to create raw producer: %v", err)
		return
	}
	defer rawProducer.Close()

	invalidMessages := []string{
		`{invalid json`,
		`{"id": ""}`,
		`{"broken": true, "no_required_fields": 1}`,
		`not even json`,
		`{"id": "test", "amount": "not-a-number"}`,
	}

	topic := cfg.KafkaTopic
	for i := 0; i < count; i++ {
		msg := invalidMessages[i%len(invalidMessages)]
		key := fmt.Sprintf("invalid-%d", i)

		err := rawProducer.Produce(&confluentkafka.Message{
			TopicPartition: confluentkafka.TopicPartition{
				Topic:     &topic,
				Partition: confluentkafka.PartitionAny,
			},
			Key:   []byte(key),
			Value: []byte(msg),
		}, nil)

		if err != nil {
			log.Printf("[ERROR] Failed to send invalid message: %v", err)
		} else {
			log.Printf("[DLQ TEST] Sent invalid message: %s", key)
		}
	}

	rawProducer.Flush(5000)
	log.Println("[DLQ TEST] Invalid messages sent - watch server logs for retries and DLQ delivery")
}