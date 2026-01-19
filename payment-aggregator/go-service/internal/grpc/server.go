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
	pb "github.com/rajathongal-intangles/payment-aggregator/go-service/pb"
)

// PaymentServer implements pb.PaymentServiceServer
type PaymentServer struct {
	pb.UnimplementedPaymentServiceServer // Required embed

	// In-memory storage (later: fed by Kafka)
	mu       sync.RWMutex
	payments map[string]*pb.Payment

	// Real-time streaming subscribers
	subscribersMu sync.RWMutex
	subscribers   map[chan *pb.PaymentEvent]struct{}
}

// NewPaymentServer creates a new server instance
func NewPaymentServer() *PaymentServer {
	return &PaymentServer{
		payments:    make(map[string]*pb.Payment),
		subscribers: make(map[chan *pb.PaymentEvent]struct{}),
	}
}

// AddPayment adds a payment to storage (called by Kafka consumer)
func (s *PaymentServer) AddPayment(p *pb.Payment) error {
	if p == nil {
		return apperrors.InvalidArgument("payment", "cannot be nil")
	}
	if p.Id == "" {
		return apperrors.InvalidArgument("payment.id", "cannot be empty")
	}

	s.mu.Lock()
	s.payments[p.Id] = p
	s.mu.Unlock()

	log.Printf("[STORE] Added: %s", p.Id)

	// Broadcast to ALL connected Node.js clients!
	event := &pb.PaymentEvent{Payment: p, EventType: "new"}
	s.broadcast(event)

	return nil
}

func (s *PaymentServer) broadcast(event *pb.PaymentEvent) {
	s.subscribersMu.RLock()
	defer s.subscribersMu.RUnlock()

	for ch := range s.subscribers {
		select {
		case ch <- event: // Send to subscriber
		default: // Skip if slow
			log.Printf("[BROADCAST] Subscriber slow, skipping")
		}
	}
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

// StreamPayments sends real-time payment events to client with error recovery
func (s *PaymentServer) StreamPayments(req *pb.ListPaymentsRequest, stream pb.PaymentService_StreamPaymentsServer) error {
	log.Printf("[RPC] StreamPayments started")

	ch := s.subscribe()
	defer s.unsubscribe(ch)

	// Send existing payments first (with optional provider filter)
	s.mu.RLock()
	for _, p := range s.payments {
		// Apply provider filter if specified
		if req.Provider != pb.Provider_PROVIDER_UNKNOWN && p.Provider != req.Provider {
			continue
		}

		event := &pb.PaymentEvent{Payment: p, EventType: "existing"}
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
				log.Println("[RPC] StreamPayments: client disconnected")
				return nil
			}
			if err == context.DeadlineExceeded {
				return status.Error(codes.DeadlineExceeded, "stream deadline exceeded")
			}
			return nil

		case event, ok := <-ch:
			if !ok {
				// Channel closed (server shutting down)
				return status.Error(codes.Unavailable, "stream closed")
			}

			// Apply provider filter if specified
			if req.Provider != pb.Provider_PROVIDER_UNKNOWN &&
				event.Payment.Provider != req.Provider {
				continue
			}

			if err := stream.Send(event); err != nil {
				log.Printf("[RPC] StreamPayments send error: %v", err)
				return status.Errorf(codes.Internal, "send failed: %v", err)
			}
		}
	}
}

// subscribe adds a new subscriber channel
func (s *PaymentServer) subscribe() chan *pb.PaymentEvent {
	ch := make(chan *pb.PaymentEvent, 10)
	s.subscribersMu.Lock()
	s.subscribers[ch] = struct{}{}
	s.subscribersMu.Unlock()
	log.Printf("[SUBSCRIBE] New subscriber (total: %d)", len(s.subscribers))
	return ch
}

// unsubscribe removes a subscriber channel
func (s *PaymentServer) unsubscribe(ch chan *pb.PaymentEvent) {
	s.subscribersMu.Lock()
	defer s.subscribersMu.Unlock()

	// Only close if still in map (not already closed by Shutdown)
	if _, exists := s.subscribers[ch]; exists {
		delete(s.subscribers, ch)
		close(ch)
		log.Printf("[UNSUBSCRIBE] Subscriber left (total: %d)", len(s.subscribers))
	}
}

// Shutdown gracefully closes all subscriber streams
func (s *PaymentServer) Shutdown() {
	s.subscribersMu.Lock()
	defer s.subscribersMu.Unlock()

	log.Printf("[SERVER] Closing %d subscriber streams...", len(s.subscribers))

	for ch := range s.subscribers {
		close(ch)
	}
	// Clear the map
	s.subscribers = make(map[chan *pb.PaymentEvent]struct{})

	log.Println("[SERVER] All streams closed")
}

// ============================================
// Helper: Seed with test data
// ============================================

func (s *PaymentServer) SeedTestData() {
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
		s.AddPayment(p)
	}

	fmt.Printf("✅ Seeded %d test payments\n", len(testPayments))
}
