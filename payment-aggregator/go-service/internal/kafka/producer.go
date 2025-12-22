package kafka

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/rajathongal-intangles/payment-aggregator/go-service/internal/config"
)

type Producer struct {
	producer *kafka.Producer
	topic    string
}

// NewProducer creates a Kafka producer from config
func NewProducer(cfg *config.Config) (*Producer, error) {
	configMap := &kafka.ConfigMap{
		"bootstrap.servers": cfg.KafkaBrokers,
		"acks":              "all",
	}

	// Add SSL if enabled
	if cfg.KafkaSSL {
		configMap.SetKey("security.protocol", "SASL_SSL")
	}

	// Add SASL authentication if configured
	if cfg.HasSASL() {
		if !cfg.KafkaSSL {
			configMap.SetKey("security.protocol", "SASL_PLAINTEXT")
		}
		configMap.SetKey("sasl.mechanisms", cfg.KafkaSASLMechanism)
		configMap.SetKey("sasl.username", cfg.KafkaSASLUsername)
		configMap.SetKey("sasl.password", cfg.KafkaSASLPassword)
	}

	producer, err := kafka.NewProducer(configMap)
	if err != nil {
		return nil, fmt.Errorf("failed to create producer: %w", err)
	}

	// Handle delivery reports in background
	go func() {
		for e := range producer.Events() {
			switch ev := e.(type) {
			case *kafka.Message:
				if ev.TopicPartition.Error != nil {
					log.Printf("[PRODUCER] ❌ Delivery failed: %v", ev.TopicPartition.Error)
				} else {
					log.Printf("[PRODUCER] ✅ Delivered: %s [partition %d]",
						string(ev.Key), ev.TopicPartition.Partition)
				}
			}
		}
	}()

	return &Producer{producer: producer, topic: cfg.KafkaTopic}, nil
}

// SendPayment sends a payment message to Kafka
func (p *Producer) SendPayment(payment *RawPayment) error {
	data, err := json.Marshal(payment)
	if err != nil {
		return fmt.Errorf("failed to marshal: %w", err)
	}

	return p.producer.Produce(&kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &p.topic,
			Partition: kafka.PartitionAny,
		},
		Key:   []byte(payment.ID),
		Value: data,
	}, nil)
}

func (p *Producer) Flush(timeoutMs int) { p.producer.Flush(timeoutMs) }
func (p *Producer) Close()              { p.producer.Close() }