package kafka

import (
	"context"
	"fmt"
	"log"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"
	"github.com/rajathongal-intangles/payment-aggregator/go-service/internal/config"
	pb "github.com/rajathongal-intangles/payment-aggregator/go-service/pb"
)

// PaymentHandler is called for each consumed payment
type PaymentHandler func(*pb.Payment)

type Consumer struct {
	consumer *kafka.Consumer
	topic    string
	handler  PaymentHandler
}

// NewConsumer creates a Kafka consumer from config
func NewConsumer(cfg *config.Config, handler PaymentHandler) (*Consumer, error) {
	configMap := &kafka.ConfigMap{
		"bootstrap.servers":  cfg.KafkaBrokers,
		"group.id":           cfg.KafkaGroupID,
		"auto.offset.reset":  "earliest",
		"enable.auto.commit": true,
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

	consumer, err := kafka.NewConsumer(configMap)
	if err != nil {
		return nil, fmt.Errorf("failed to create consumer: %w", err)
	}

	return &Consumer{consumer: consumer, topic: cfg.KafkaTopic, handler: handler}, nil
}

// Start begins consuming messages (blocking)
func (c *Consumer) Start(ctx context.Context) error {
	if err := c.consumer.Subscribe(c.topic, nil); err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	log.Printf("[CONSUMER] 📡 Subscribed to: %s", c.topic)

	for {
		select {
		case <-ctx.Done():
			log.Println("[CONSUMER] Context cancelled, stopping...")
			return nil
		default:
			// Use 1 second timeout so we can check ctx.Done() periodically
			msg, err := c.consumer.ReadMessage(1000)
			if err != nil {
				// Timeout is expected, just continue to check context
				if err.(kafka.Error).Code() == kafka.ErrTimedOut {
					continue
				}
				log.Printf("[CONSUMER] Read error: %v", err)
				continue
			}
			c.processMessage(msg)
		}
	}
}

func (c *Consumer) processMessage(msg *kafka.Message) {
	raw, err := ParseMessage(msg.Value)
	if err != nil {
		log.Printf("[CONSUMER] ❌ Parse error: %v", err)
		return
	}

	payment := raw.ToProto()
	log.Printf("[CONSUMER] ✅ %s | %s | %.2f %s",
		payment.Id, payment.Provider, payment.Amount, payment.Currency)

	// This triggers broadcast to all streaming Node.js clients!
	c.handler(payment)
}

func (c *Consumer) Close() error { return c.consumer.Close() }