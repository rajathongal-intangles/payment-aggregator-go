package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"

	"github.com/rajathongal-intangles/payment-aggregator/go-service/internal/circuit"
	"github.com/rajathongal-intangles/payment-aggregator/go-service/internal/config"
	pb "github.com/rajathongal-intangles/payment-aggregator/go-service/pb"
)

// PaymentHandler is called for each consumed payment
type PaymentHandler func(*pb.Payment) error

// FailedMessage represents a message that couldn't be processed
type FailedMessage struct {
	OriginalTopic string `json:"original_topic"`
	Partition     int32  `json:"partition"`
	Offset        int64  `json:"offset"`
	Key           string `json:"key"`
	Value         string `json:"value"`
	Error         string `json:"error"`
	Timestamp     int64  `json:"timestamp"`
	RetryCount    int    `json:"retry_count"`
}

type Consumer struct {
	consumer    *kafka.Consumer
	dlqProducer *kafka.Producer // Dead Letter Queue producer
	topic       string
	dlqTopic    string
	handler     PaymentHandler
	breaker     *circuit.Breaker
}

// NewConsumer creates a Kafka consumer from config with DLQ support
func NewConsumer(cfg *config.Config, handler PaymentHandler) (*Consumer, error) {
	configMap := &kafka.ConfigMap{
		"bootstrap.servers":  cfg.KafkaBrokers,
		"group.id":           cfg.KafkaGroupID,
		"auto.offset.reset":  "earliest",
		"enable.auto.commit": false, // Manual commit for better control
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

	// Create DLQ producer with same config
	producerConfig := &kafka.ConfigMap{
		"bootstrap.servers": cfg.KafkaBrokers,
		"acks":              "all",
	}

	if cfg.KafkaSSL {
		producerConfig.SetKey("security.protocol", "SASL_SSL")
	}
	if cfg.HasSASL() {
		if !cfg.KafkaSSL {
			producerConfig.SetKey("security.protocol", "SASL_PLAINTEXT")
		}
		producerConfig.SetKey("sasl.mechanisms", cfg.KafkaSASLMechanism)
		producerConfig.SetKey("sasl.username", cfg.KafkaSASLUsername)
		producerConfig.SetKey("sasl.password", cfg.KafkaSASLPassword)
	}

	dlqProducer, err := kafka.NewProducer(producerConfig)
	if err != nil {
		consumer.Close()
		return nil, fmt.Errorf("failed to create DLQ producer: %w", err)
	}

	// Handle DLQ delivery reports in background
	go func() {
		for e := range dlqProducer.Events() {
			switch ev := e.(type) {
			case *kafka.Message:
				if ev.TopicPartition.Error != nil {
					log.Printf("[DLQ] ❌ Delivery failed: %v", ev.TopicPartition.Error)
				} else {
					log.Printf("[DLQ] ✅ Delivered to DLQ [partition %d]", ev.TopicPartition.Partition)
				}
			}
		}
	}()

	breaker := circuit.NewBreaker(circuit.Config{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		Timeout:          30 * time.Second,
		OnStateChange: func(from, to circuit.State) {
			log.Printf("[CIRCUIT] %s → %s", from, to)
		},
	})

	return &Consumer{
		consumer:    consumer,
		dlqProducer: dlqProducer,
		topic:       cfg.KafkaTopic,
		dlqTopic:    cfg.KafkaTopic + ".dlq",
		handler:     handler,
		breaker:     breaker,
	}, nil
}

// Start begins consuming messages (blocking)
func (c *Consumer) Start(ctx context.Context) error {
	if err := c.consumer.Subscribe(c.topic, nil); err != nil {
		return fmt.Errorf("failed to subscribe: %w", err)
	}

	log.Printf("[CONSUMER] 📡 Subscribed to: %s", c.topic)
	log.Printf("[CONSUMER] 📭 DLQ topic: %s", c.dlqTopic)

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
				if kafkaErr, ok := err.(kafka.Error); ok {
					if kafkaErr.Code() == kafka.ErrTimedOut {
						continue
					}
				}
				log.Printf("[CONSUMER] Read error: %v", err)
				continue
			}
			c.processWithRetry(msg)
		}
	}
}

// processWithRetry processes a message with retry and circuit breaker
func (c *Consumer) processWithRetry(msg *kafka.Message) {
	maxRetries := 3

	for attempt := 0; attempt <= maxRetries; attempt++ {
		err := c.breaker.Execute(func() error {
			return c.processMessage(msg)
		})

		if err == nil {
			// Success - commit offset
			c.consumer.CommitMessage(msg)
			return
		}

		if err == circuit.ErrCircuitOpen {
			log.Printf("[CONSUMER] ⚡ Circuit open, sending to DLQ")
			c.sendToDLQ(msg, "circuit_breaker_open", 0)
			c.consumer.CommitMessage(msg)
			return
		}

		if attempt < maxRetries {
			backoff := time.Duration(100*(1<<attempt)) * time.Millisecond
			log.Printf("[CONSUMER] 🔄 Retry %d/%d in %v", attempt+1, maxRetries, backoff)
			time.Sleep(backoff)
		}
	}

	// All retries failed - send to DLQ
	log.Printf("[CONSUMER] ❌ Max retries exceeded, sending to DLQ")
	c.sendToDLQ(msg, "max_retries_exceeded", maxRetries)
	c.consumer.CommitMessage(msg)
}

func (c *Consumer) processMessage(msg *kafka.Message) error {
	raw, err := ParseMessage(msg.Value)
	if err != nil {
		return fmt.Errorf("parse error: %w", err)
	}

	payment := raw.ToProto()

	if err := c.handler(payment); err != nil {
		return fmt.Errorf("handler error: %w", err)
	}

	log.Printf("[CONSUMER] ✅ %s | %s | %.2f %s",
		payment.Id, payment.Provider, payment.Amount, payment.Currency)

	return nil
}

// sendToDLQ sends a failed message to the Dead Letter Queue
func (c *Consumer) sendToDLQ(msg *kafka.Message, reason string, retryCount int) {
	failed := FailedMessage{
		OriginalTopic: c.topic,
		Partition:     msg.TopicPartition.Partition,
		Offset:        int64(msg.TopicPartition.Offset),
		Key:           string(msg.Key),
		Value:         string(msg.Value),
		Error:         reason,
		Timestamp:     time.Now().Unix(),
		RetryCount:    retryCount,
	}

	data, _ := json.Marshal(failed)

	err := c.dlqProducer.Produce(&kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &c.dlqTopic,
			Partition: kafka.PartitionAny,
		},
		Key:   msg.Key,
		Value: data,
	}, nil)

	if err != nil {
		log.Printf("[DLQ] ❌ Failed to send to DLQ: %v", err)
	} else {
		log.Printf("[DLQ] 📤 Sent to DLQ: %s", string(msg.Key))
	}
}

func (c *Consumer) Close() error {
	if c.dlqProducer != nil {
		c.dlqProducer.Flush(5000)
		c.dlqProducer.Close()
	}
	return c.consumer.Close()
}
