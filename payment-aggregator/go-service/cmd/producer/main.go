package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	confluentkafka "github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/joho/godotenv"
	"github.com/rajathongal-intangles/payment-aggregator/go-service/internal/config"
	"github.com/rajathongal-intangles/payment-aggregator/go-service/internal/kafka"
)

func main() {
	count := flag.Int("count", 5, "Number of payments per topic")
	interval := flag.Duration("interval", time.Second, "Interval between messages (use 0 for no delay)")
	invalid := flag.Int("invalid", 0, "Number of invalid messages per topic (for DLQ testing)")
	topics := flag.String("topics", "", "Comma-separated topics (default: uses KAFKA_TOPIC from env)")
	flag.Parse()

	// Try loading .env from multiple locations
	godotenv.Load()             // current dir
	godotenv.Load("../../.env") // repo root from cmd/producer/

	cfg := config.Load()

	// Parse topics
	var topicList []string
	if *topics != "" {
		for _, t := range strings.Split(*topics, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				topicList = append(topicList, t)
			}
		}
	} else {
		topicList = []string{cfg.KafkaTopic}
	}

	log.Printf("[CONFIG] Brokers: %s, Topics: %v, SSL: %v", cfg.KafkaBrokers, topicList, cfg.KafkaSSL)
	log.Printf("[CONFIG] Messages per topic: %d valid + %d invalid", *count, *invalid)

	// Create producer
	producer, err := createProducer(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer producer.Close()

	// Handle delivery reports
	go func() {
		for e := range producer.Events() {
			switch ev := e.(type) {
			case *confluentkafka.Message:
				if ev.TopicPartition.Error != nil {
					log.Printf("[ERROR] Delivery failed: %v", ev.TopicPartition.Error)
				}
			}
		}
	}()

	// Send to all topics in parallel
	var wg sync.WaitGroup
	for _, topic := range topicList {
		wg.Add(1)
		go func(t string) {
			defer wg.Done()
			sendToTopic(producer, t, *count, *invalid, *interval)
		}(topic)
	}

	wg.Wait()
	producer.Flush(15000)

	log.Printf("\nDone! Sent %d messages to %d topics", (*count+*invalid)*len(topicList), len(topicList))
}

func createProducer(cfg *config.Config) (*confluentkafka.Producer, error) {
	configMap := &confluentkafka.ConfigMap{
		"bootstrap.servers": cfg.KafkaBrokers,
		"acks":              "all",
	}

	if cfg.KafkaSSL {
		configMap.SetKey("security.protocol", "SASL_SSL")
	}
	if cfg.HasSASL() {
		if !cfg.KafkaSSL {
			configMap.SetKey("security.protocol", "SASL_PLAINTEXT")
		}
		configMap.SetKey("sasl.mechanisms", cfg.KafkaSASLMechanism)
		configMap.SetKey("sasl.username", cfg.KafkaSASLUsername)
		configMap.SetKey("sasl.password", cfg.KafkaSASLPassword)
	}

	return confluentkafka.NewProducer(configMap)
}

func sendToTopic(producer *confluentkafka.Producer, topic string, count, invalid int, interval time.Duration) {
	log.Printf("[%s] Sending %d valid + %d invalid messages...", topic, count, invalid)

	// Send valid payments
	for i := 0; i < count; i++ {
		payment := kafka.GenerateTestPayment()
		data, _ := json.Marshal(payment)

		err := producer.Produce(&confluentkafka.Message{
			TopicPartition: confluentkafka.TopicPartition{
				Topic:     &topic,
				Partition: confluentkafka.PartitionAny,
			},
			Key:   []byte(payment.ID),
			Value: data,
		}, nil)

		if err != nil {
			log.Printf("[%s] ERROR: %v", topic, err)
		} else {
			log.Printf("[%s] %s | %s | %.2f %s",
				topic, payment.ID, payment.Provider, payment.Amount, payment.Currency)
		}

		if interval > 0 {
			time.Sleep(interval)
		}
	}

	// Send invalid messages for DLQ testing
	if invalid > 0 {
		sendInvalidToTopic(producer, topic, invalid)
	}
}

func sendInvalidToTopic(producer *confluentkafka.Producer, topic string, count int) {
	invalidMessages := []string{
		`{invalid json`,
		`{"id": ""}`,
		`{"broken": true, "no_required_fields": 1}`,
		`not even json`,
		`{"id": "test", "amount": "not-a-number"}`,
	}

	for i := 0; i < count; i++ {
		msg := invalidMessages[i%len(invalidMessages)]
		key := fmt.Sprintf("invalid-%s-%d", topic, i)

		err := producer.Produce(&confluentkafka.Message{
			TopicPartition: confluentkafka.TopicPartition{
				Topic:     &topic,
				Partition: confluentkafka.PartitionAny,
			},
			Key:   []byte(key),
			Value: []byte(msg),
		}, nil)

		if err != nil {
			log.Printf("[%s] ERROR sending invalid: %v", topic, err)
		} else {
			log.Printf("[%s] [DLQ] Sent invalid: %s", topic, key)
		}
	}
}
