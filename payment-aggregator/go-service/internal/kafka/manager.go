package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/confluentinc/confluent-kafka-go/v2/kafka"

	"github.com/rajathongal-intangles/payment-aggregator/go-service/internal/circuit"
	"github.com/rajathongal-intangles/payment-aggregator/go-service/internal/config"
	pb "github.com/rajathongal-intangles/payment-aggregator/go-service/pb"
)

// TopicConsumer represents an active consumer for a specific topic
type TopicConsumer struct {
	consumer    *kafka.Consumer
	dlqProducer *kafka.Producer
	topic       string
	dlqTopic    string
	breaker     *circuit.Breaker
	cancel      context.CancelFunc
	ctx         context.Context

	// Subscribers for this topic
	subscribersMu sync.RWMutex
	subscribers   map[chan *pb.PaymentEvent]struct{}
}

// ConsumerManager manages on-demand Kafka consumers for multiple topics
type ConsumerManager struct {
	cfg *config.Config

	mu        sync.RWMutex
	consumers map[string]*TopicConsumer // topic -> consumer

	// Callback when payment is received (for storage)
	onPayment func(topic string, payment *pb.Payment) error
}

// NewConsumerManager creates a new manager instance
func NewConsumerManager(cfg *config.Config) *ConsumerManager {
	return &ConsumerManager{
		cfg:       cfg,
		consumers: make(map[string]*TopicConsumer),
	}
}

// SetPaymentHandler sets the callback for received payments
func (m *ConsumerManager) SetPaymentHandler(handler func(topic string, payment *pb.Payment) error) {
	m.onPayment = handler
}

// Subscribe adds a subscriber to a topic, creating consumer if needed
// Returns a channel that receives PaymentEvents for this topic
func (m *ConsumerManager) Subscribe(topic string) (chan *pb.PaymentEvent, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Get or create consumer for this topic
	tc, exists := m.consumers[topic]
	if !exists {
		var err error
		tc, err = m.createTopicConsumer(topic)
		if err != nil {
			return nil, fmt.Errorf("failed to create consumer for topic %s: %w", topic, err)
		}
		m.consumers[topic] = tc
		log.Printf("[MANAGER] Created new consumer for topic: %s", topic)
	}

	// Create subscriber channel
	ch := make(chan *pb.PaymentEvent, 10)
	tc.subscribersMu.Lock()
	tc.subscribers[ch] = struct{}{}
	count := len(tc.subscribers)
	tc.subscribersMu.Unlock()

	log.Printf("[MANAGER] New subscriber for %s (total: %d)", topic, count)
	return ch, nil
}

// Unsubscribe removes a subscriber from a topic
// Stops consumer if no subscribers remain
func (m *ConsumerManager) Unsubscribe(topic string, ch chan *pb.PaymentEvent) {
	m.mu.Lock()
	defer m.mu.Unlock()

	tc, exists := m.consumers[topic]
	if !exists {
		return
	}

	tc.subscribersMu.Lock()
	if _, ok := tc.subscribers[ch]; ok {
		delete(tc.subscribers, ch)
		close(ch)
	}
	remaining := len(tc.subscribers)
	tc.subscribersMu.Unlock()

	log.Printf("[MANAGER] Subscriber left %s (remaining: %d)", topic, remaining)

	// Stop consumer if no subscribers
	if remaining == 0 {
		log.Printf("[MANAGER] No subscribers for %s, stopping consumer...", topic)
		tc.cancel()
		tc.consumer.Close()
		if tc.dlqProducer != nil {
			tc.dlqProducer.Flush(5000)
			tc.dlqProducer.Close()
		}
		delete(m.consumers, topic)
		log.Printf("[MANAGER] Consumer stopped for topic: %s", topic)
	}
}

// GetActiveTopics returns list of topics with active consumers
func (m *ConsumerManager) GetActiveTopics() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	topics := make([]string, 0, len(m.consumers))
	for topic := range m.consumers {
		topics = append(topics, topic)
	}
	return topics
}

// GetSubscriberCount returns number of subscribers for a topic
func (m *ConsumerManager) GetSubscriberCount(topic string) int {
	m.mu.RLock()
	tc, exists := m.consumers[topic]
	m.mu.RUnlock()

	if !exists {
		return 0
	}

	tc.subscribersMu.RLock()
	defer tc.subscribersMu.RUnlock()
	return len(tc.subscribers)
}

// Shutdown stops all consumers gracefully
func (m *ConsumerManager) Shutdown() {
	m.mu.Lock()
	defer m.mu.Unlock()

	log.Printf("[MANAGER] Shutting down %d consumers...", len(m.consumers))

	for topic, tc := range m.consumers {
		// Close all subscriber channels
		tc.subscribersMu.Lock()
		for ch := range tc.subscribers {
			close(ch)
		}
		tc.subscribers = make(map[chan *pb.PaymentEvent]struct{})
		tc.subscribersMu.Unlock()

		// Stop consumer
		tc.cancel()
		tc.consumer.Close()
		if tc.dlqProducer != nil {
			tc.dlqProducer.Flush(5000)
			tc.dlqProducer.Close()
		}

		log.Printf("[MANAGER] Stopped consumer for: %s", topic)
	}

	m.consumers = make(map[string]*TopicConsumer)
	log.Println("[MANAGER] All consumers stopped")
}

// createTopicConsumer creates a new consumer for a specific topic
func (m *ConsumerManager) createTopicConsumer(topic string) (*TopicConsumer, error) {
	configMap := &kafka.ConfigMap{
		"bootstrap.servers":  m.cfg.KafkaBrokers,
		"group.id":           m.cfg.KafkaGroupID,
		"auto.offset.reset":  "earliest",
		"enable.auto.commit": false,
	}

	if m.cfg.KafkaSSL {
		configMap.SetKey("security.protocol", "SASL_SSL")
	}

	if m.cfg.HasSASL() {
		if !m.cfg.KafkaSSL {
			configMap.SetKey("security.protocol", "SASL_PLAINTEXT")
		}
		configMap.SetKey("sasl.mechanisms", m.cfg.KafkaSASLMechanism)
		configMap.SetKey("sasl.username", m.cfg.KafkaSASLUsername)
		configMap.SetKey("sasl.password", m.cfg.KafkaSASLPassword)
	}

	consumer, err := kafka.NewConsumer(configMap)
	if err != nil {
		return nil, fmt.Errorf("failed to create consumer: %w", err)
	}

	// Subscribe to topic
	if err := consumer.Subscribe(topic, nil); err != nil {
		consumer.Close()
		return nil, fmt.Errorf("failed to subscribe to %s: %w", topic, err)
	}

	// Create DLQ producer
	producerConfig := &kafka.ConfigMap{
		"bootstrap.servers": m.cfg.KafkaBrokers,
		"acks":              "all",
	}

	if m.cfg.KafkaSSL {
		producerConfig.SetKey("security.protocol", "SASL_SSL")
	}
	if m.cfg.HasSASL() {
		if !m.cfg.KafkaSSL {
			producerConfig.SetKey("security.protocol", "SASL_PLAINTEXT")
		}
		producerConfig.SetKey("sasl.mechanisms", m.cfg.KafkaSASLMechanism)
		producerConfig.SetKey("sasl.username", m.cfg.KafkaSASLUsername)
		producerConfig.SetKey("sasl.password", m.cfg.KafkaSASLPassword)
	}

	dlqProducer, err := kafka.NewProducer(producerConfig)
	if err != nil {
		consumer.Close()
		return nil, fmt.Errorf("failed to create DLQ producer: %w", err)
	}

	// Handle DLQ delivery reports
	go func() {
		for e := range dlqProducer.Events() {
			switch ev := e.(type) {
			case *kafka.Message:
				if ev.TopicPartition.Error != nil {
					log.Printf("[DLQ:%s] Delivery failed: %v", topic, ev.TopicPartition.Error)
				}
			}
		}
	}()

	breaker := circuit.NewBreaker(circuit.Config{
		FailureThreshold: 5,
		SuccessThreshold: 2,
		Timeout:          30 * time.Second,
		OnStateChange: func(from, to circuit.State) {
			log.Printf("[CIRCUIT:%s] %s -> %s", topic, from, to)
		},
	})

	ctx, cancel := context.WithCancel(context.Background())

	tc := &TopicConsumer{
		consumer:    consumer,
		dlqProducer: dlqProducer,
		topic:       topic,
		dlqTopic:    topic + ".dlq",
		breaker:     breaker,
		ctx:         ctx,
		cancel:      cancel,
		subscribers: make(map[chan *pb.PaymentEvent]struct{}),
	}

	// Start consuming in goroutine
	go m.consumeLoop(tc)

	return tc, nil
}

// consumeLoop reads messages from Kafka and broadcasts to subscribers
func (m *ConsumerManager) consumeLoop(tc *TopicConsumer) {
	log.Printf("[CONSUMER:%s] Started consuming", tc.topic)

	for {
		select {
		case <-tc.ctx.Done():
			log.Printf("[CONSUMER:%s] Context cancelled, stopping", tc.topic)
			return
		default:
			msg, err := tc.consumer.ReadMessage(1000) // 1s timeout
			if err != nil {
				if kafkaErr, ok := err.(kafka.Error); ok {
					if kafkaErr.Code() == kafka.ErrTimedOut {
						continue
					}
				}
				log.Printf("[CONSUMER:%s] Read error: %v", tc.topic, err)
				continue
			}

			m.processMessage(tc, msg)
		}
	}
}

// processMessage processes a single message with retry and circuit breaker
func (m *ConsumerManager) processMessage(tc *TopicConsumer, msg *kafka.Message) {
	maxRetries := 3

	for attempt := 0; attempt <= maxRetries; attempt++ {
		err := tc.breaker.Execute(func() error {
			return m.handleMessage(tc, msg)
		})

		if err == nil {
			tc.consumer.CommitMessage(msg)
			return
		}

		if err == circuit.ErrCircuitOpen {
			log.Printf("[CONSUMER:%s] Circuit open, sending to DLQ", tc.topic)
			m.sendToDLQ(tc, msg, "circuit_breaker_open", 0)
			tc.consumer.CommitMessage(msg)
			return
		}

		if attempt < maxRetries {
			backoff := time.Duration(100*(1<<attempt)) * time.Millisecond
			log.Printf("[CONSUMER:%s] Retry %d/%d in %v", tc.topic, attempt+1, maxRetries, backoff)
			time.Sleep(backoff)
		}
	}

	log.Printf("[CONSUMER:%s] Max retries exceeded, sending to DLQ", tc.topic)
	m.sendToDLQ(tc, msg, "max_retries_exceeded", maxRetries)
	tc.consumer.CommitMessage(msg)
}

// handleMessage parses and broadcasts a message
func (m *ConsumerManager) handleMessage(tc *TopicConsumer, msg *kafka.Message) error {
	raw, err := ParseMessage(msg.Value)
	if err != nil {
		return fmt.Errorf("parse error: %w", err)
	}

	payment := raw.ToProto()

	// Call storage handler if set
	if m.onPayment != nil {
		if err := m.onPayment(tc.topic, payment); err != nil {
			return fmt.Errorf("handler error: %w", err)
		}
	}

	// Broadcast to subscribers
	event := &pb.PaymentEvent{
		Payment:   payment,
		EventType: "new",
		Topic:     tc.topic,
	}

	tc.subscribersMu.RLock()
	for ch := range tc.subscribers {
		select {
		case ch <- event:
		default:
			log.Printf("[CONSUMER:%s] Subscriber slow, skipping", tc.topic)
		}
	}
	tc.subscribersMu.RUnlock()

	log.Printf("[CONSUMER:%s] %s | %s | %.2f %s",
		tc.topic, payment.Id, payment.Provider, payment.Amount, payment.Currency)

	return nil
}

// sendToDLQ sends a failed message to the Dead Letter Queue
func (m *ConsumerManager) sendToDLQ(tc *TopicConsumer, msg *kafka.Message, reason string, retryCount int) {
	failed := FailedMessage{
		OriginalTopic: tc.topic,
		Partition:     msg.TopicPartition.Partition,
		Offset:        int64(msg.TopicPartition.Offset),
		Key:           string(msg.Key),
		Value:         string(msg.Value),
		Error:         reason,
		Timestamp:     time.Now().Unix(),
		RetryCount:    retryCount,
	}

	data, _ := json.Marshal(failed)

	err := tc.dlqProducer.Produce(&kafka.Message{
		TopicPartition: kafka.TopicPartition{
			Topic:     &tc.dlqTopic,
			Partition: kafka.PartitionAny,
		},
		Key:   msg.Key,
		Value: data,
	}, nil)

	if err != nil {
		log.Printf("[DLQ:%s] Failed to send: %v", tc.topic, err)
	} else {
		log.Printf("[DLQ:%s] Sent: %s", tc.topic, string(msg.Key))
	}
}
