package grpc

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	apperrors "github.com/rajathongal-intangles/payment-aggregator/go-service/internal/errors"
	"github.com/rajathongal-intangles/payment-aggregator/go-service/internal/kafka"
	pb "github.com/rajathongal-intangles/payment-aggregator/go-service/pb"
)

// PaymentServer implements pb.PaymentServiceServer
type PaymentServer struct {
	pb.UnimplementedPaymentServiceServer // Required embed

	// In-memory storage (fed by Kafka consumers)
	mu       sync.RWMutex
	payments map[string]*pb.Payment // key: "topic:payment_id"

	// Consumer manager for on-demand topic consumption
	consumerManager *kafka.ConsumerManager
}

// NewPaymentServer creates a new server instance
func NewPaymentServer(manager *kafka.ConsumerManager) *PaymentServer {
	s := &PaymentServer{
		payments:        make(map[string]*pb.Payment),
		consumerManager: manager,
	}

	// Set handler for received payments
	if manager != nil {
		manager.SetPaymentHandler(func(topic string, payment *pb.Payment) error {
			return s.AddPayment(topic, payment)
		})
	}

	return s
}

// AddPayment adds a payment to storage (called by ConsumerManager)
func (s *PaymentServer) AddPayment(topic string, p *pb.Payment) error {
	if p == nil {
		return apperrors.InvalidArgument("payment", "cannot be nil")
	}
	if p.Id == "" {
		return apperrors.InvalidArgument("payment.id", "cannot be empty")
	}

	// Store with topic prefix for uniqueness across topics
	key := fmt.Sprintf("%s:%s", topic, p.Id)
	s.mu.Lock()
	s.payments[key] = p
	s.mu.Unlock()

	log.Printf("[STORE] Added: %s (topic: %s)", p.Id, topic)
	return nil
}

// ============================================
// gRPC Method Implementations
// ============================================

// GetPayment returns a single payment by ID with proper error handling
func (s *PaymentServer) GetPayment(
	ctx context.Context,
	req *pb.GetPaymentRequest,
) (*pb.Payment, error) {
	log.Printf("[RPC] GetPayment: %s", req.PaymentId)

	// Validate input
	if req.PaymentId == "" {
		return nil, apperrors.InvalidArgument("payment_id", "cannot be empty").ToGRPCError()
	}

	// Check context deadline
	if ctx.Err() == context.DeadlineExceeded {
		return nil, status.Error(codes.DeadlineExceeded, "request timed out")
	}

	if ctx.Err() == context.Canceled {
		return nil, status.Error(codes.Canceled, "request cancelled")
	}

	// Look up payment
	s.mu.RLock()
	payment, exists := s.payments[req.PaymentId]
	s.mu.RUnlock()

	if !exists {
		return nil, apperrors.NotFound("payment", req.PaymentId).ToGRPCError()
	}

	return payment, nil
}

// ListPayments returns filtered list of payments with validation
func (s *PaymentServer) ListPayments(
	ctx context.Context,
	req *pb.ListPaymentsRequest,
) (*pb.PaymentList, error) {
	log.Printf("[RPC] ListPayments: provider=%v, status=%v, limit=%d",
		req.Provider, req.Status, req.Limit)

	// Validate limit
	limit := int(req.Limit)
	if limit < 0 {
		return nil, apperrors.InvalidArgument("limit", "cannot be negative").ToGRPCError()
	}
	if limit == 0 || limit > 100 {
		limit = 10
	}

	s.mu.RLock()
	defer s.mu.RUnlock()

	// Filter and collect payments
	var result []*pb.Payment
	for _, p := range s.payments {
		// Apply filters
		if req.Provider != pb.Provider_PROVIDER_UNKNOWN && p.Provider != req.Provider {
			continue
		}
		if req.Status != pb.PaymentStatus_STATUS_UNKNOWN && p.Status != req.Status {
			continue
		}

		result = append(result, p)

		if len(result) >= limit {
			break
		}
	}

	return &pb.PaymentList{
		Payments:   result,
		TotalCount: int32(len(result)),
		NextCursor: "", // Simplified: no pagination yet
	}, nil
}

// StreamPayments subscribes to a Kafka topic and streams events to the client
func (s *PaymentServer) StreamPayments(req *pb.StreamRequest, stream pb.PaymentService_StreamPaymentsServer) error {
	// Validate topic
	if req.Topic == "" {
		return status.Error(codes.InvalidArgument, "topic is required")
	}

	log.Printf("[RPC] StreamPayments started for topic: %s", req.Topic)

	// Subscribe to topic via ConsumerManager
	if s.consumerManager == nil {
		return status.Error(codes.Internal, "consumer manager not initialized")
	}

	ch, err := s.consumerManager.Subscribe(req.Topic)
	if err != nil {
		log.Printf("[RPC] Failed to subscribe to %s: %v", req.Topic, err)
		return status.Errorf(codes.Internal, "failed to subscribe: %v", err)
	}
	defer s.consumerManager.Unsubscribe(req.Topic, ch)

	// Send existing payments for this topic first
	s.mu.RLock()
	for key, p := range s.payments {
		// Only send payments from this topic
		if !s.isPaymentFromTopic(key, req.Topic) {
			continue
		}

		// Apply provider filter if specified
		if req.Provider != pb.Provider_PROVIDER_UNKNOWN && p.Provider != req.Provider {
			continue
		}

		// Apply status filter if specified
		if req.Status != pb.PaymentStatus_STATUS_UNKNOWN && p.Status != req.Status {
			continue
		}

		event := &pb.PaymentEvent{
			Payment:   p,
			EventType: "existing",
			Topic:     req.Topic,
		}
		if err := stream.Send(event); err != nil {
			s.mu.RUnlock()
			log.Printf("[RPC] StreamPayments send error: %v", err)
			return status.Errorf(codes.Internal, "failed to send: %v", err)
		}
	}
	s.mu.RUnlock()

	// Stream new payments with context awareness
	for {
		select {
		case <-stream.Context().Done():
			err := stream.Context().Err()
			if err == context.Canceled {
				log.Printf("[RPC] StreamPayments: client disconnected from %s", req.Topic)
				return nil
			}
			if err == context.DeadlineExceeded {
				return status.Error(codes.DeadlineExceeded, "stream deadline exceeded")
			}
			return nil

		case event, ok := <-ch:
			if !ok {
				// Channel closed (consumer stopped or server shutting down)
				return status.Error(codes.Unavailable, "stream closed")
			}

			// Apply provider filter if specified
			if req.Provider != pb.Provider_PROVIDER_UNKNOWN &&
				event.Payment.Provider != req.Provider {
				continue
			}

			// Apply status filter if specified
			if req.Status != pb.PaymentStatus_STATUS_UNKNOWN &&
				event.Payment.Status != req.Status {
				continue
			}

			if err := stream.Send(event); err != nil {
				log.Printf("[RPC] StreamPayments send error: %v", err)
				return status.Errorf(codes.Internal, "send failed: %v", err)
			}
		}
	}
}

// isPaymentFromTopic checks if a payment key belongs to a topic
func (s *PaymentServer) isPaymentFromTopic(key, topic string) bool {
	return len(key) > len(topic)+1 && key[:len(topic)+1] == topic+":"
}

// GetTopics returns list of active topics with consumers
func (s *PaymentServer) GetTopics(ctx context.Context, req *pb.GetTopicsRequest) (*pb.TopicList, error) {
	if s.consumerManager == nil {
		return &pb.TopicList{Topics: []string{}, Count: 0}, nil
	}

	topics := s.consumerManager.GetActiveTopics()
	return &pb.TopicList{
		Topics: topics,
		Count:  int32(len(topics)),
	}, nil
}

// Shutdown gracefully closes all consumers and streams
func (s *PaymentServer) Shutdown() {
	if s.consumerManager != nil {
		s.consumerManager.Shutdown()
	}
	log.Println("[SERVER] Shutdown complete")
}

// ============================================
// Helper: Seed with test data
// ============================================

func (s *PaymentServer) SeedTestData(topic string) {
	testPayments := []*pb.Payment{
		{
			Id:            "pay_001",
			Provider:      pb.Provider_PROVIDER_STRIPE,
			Amount:        99.99,
			Currency:      "USD",
			Status:        pb.PaymentStatus_STATUS_COMPLETED,
			CustomerEmail: "alice@example.com",
			CreatedAt:     time.Now().Unix(),
			ProcessedAt:   time.Now().Unix(),
			Metadata:      map[string]string{"order_id": "ORD-001"},
		},
		{
			Id:            "pay_002",
			Provider:      pb.Provider_PROVIDER_RAZORPAY,
			Amount:        1500.00,
			Currency:      "INR",
			Status:        pb.PaymentStatus_STATUS_PENDING,
			CustomerEmail: "bob@example.com",
			CreatedAt:     time.Now().Unix(),
			Metadata:      map[string]string{"order_id": "ORD-002"},
		},
		{
			Id:            "pay_003",
			Provider:      pb.Provider_PROVIDER_PAYPAL,
			Amount:        250.00,
			Currency:      "EUR",
			Status:        pb.PaymentStatus_STATUS_COMPLETED,
			CustomerEmail: "carol@example.com",
			CreatedAt:     time.Now().Unix(),
			ProcessedAt:   time.Now().Unix(),
			Metadata:      map[string]string{"order_id": "ORD-003"},
		},
	}

	for _, p := range testPayments {
		s.AddPayment(topic, p)
	}

	fmt.Printf("Seeded %d test payments for topic: %s\n", len(testPayments), topic)
}
