package kafka

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"strings"
	"time"

	pb "github.com/rajathongal-intangles/payment-aggregator/go-service/pb"
)

// RawPayment represents the JSON message from Kafka
type RawPayment struct {
	ID            string            `json:"id"`
	Provider      string            `json:"provider"`
	Amount        float64           `json:"amount"`
	Currency      string            `json:"currency"`
	Status        string            `json:"status"`
	CustomerEmail string            `json:"customer_email"`
	Metadata      map[string]string `json:"metadata"`
	Timestamp     int64             `json:"timestamp"`
}

// ToProto converts raw payment to protobuf Payment
func (r *RawPayment) ToProto() *pb.Payment {
	return &pb.Payment{
		Id:            r.ID,
		Provider:      mapProvider(r.Provider),
		Amount:        normalizeAmount(r.Amount, r.Currency),
		Currency:      strings.ToUpper(r.Currency),
		Status:        mapStatus(r.Status),
		CustomerEmail: r.CustomerEmail,
		Metadata:      r.Metadata,
		CreatedAt:     r.Timestamp,
		ProcessedAt:   time.Now().Unix(),
	}
}

// ParseMessage parses JSON bytes into RawPayment with validation
func ParseMessage(data []byte) (*RawPayment, error) {
	var raw RawPayment
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("invalid JSON: %w", err)
	}

	// Validate required fields
	if raw.ID == "" {
		return nil, fmt.Errorf("missing required field: id")
	}
	if raw.Provider == "" {
		return nil, fmt.Errorf("missing required field: provider")
	}
	if raw.Amount <= 0 {
		return nil, fmt.Errorf("invalid amount: must be positive")
	}
	if raw.Currency == "" {
		return nil, fmt.Errorf("missing required field: currency")
	}

	return &raw, nil
}

func mapProvider(provider string) pb.Provider {
	switch strings.ToLower(provider) {
	case "stripe":
		return pb.Provider_PROVIDER_STRIPE
	case "razorpay":
		return pb.Provider_PROVIDER_RAZORPAY
	case "paypal":
		return pb.Provider_PROVIDER_PAYPAL
	default:
		return pb.Provider_PROVIDER_UNKNOWN
	}
}

func mapStatus(status string) pb.PaymentStatus {
	switch strings.ToLower(status) {
	case "pending", "processing":
		return pb.PaymentStatus_STATUS_PENDING
	case "succeeded", "completed", "paid":
		return pb.PaymentStatus_STATUS_COMPLETED
	case "failed", "declined":
		return pb.PaymentStatus_STATUS_FAILED
	case "refunded":
		return pb.PaymentStatus_STATUS_REFUNDED
	default:
		return pb.PaymentStatus_STATUS_UNKNOWN
	}
}

// normalizeAmount converts cents to dollars for USD/EUR
func normalizeAmount(amount float64, currency string) float64 {
	currency = strings.ToLower(currency)
	if currency == "usd" || currency == "eur" || currency == "gbp" {
		if amount >= 100 {
			return amount / 100
		}
	}
	return amount
}

// GenerateTestPayment creates a random test payment
func GenerateTestPayment() *RawPayment {
	providers := []string{"stripe", "razorpay", "paypal"}
	currencies := []string{"USD", "EUR", "INR", "GBP"}
	statuses := []string{"pending", "succeeded", "failed"}

	return &RawPayment{
		ID:            fmt.Sprintf("pay_%d", rand.Int63()),
		Provider:      providers[rand.Intn(len(providers))],
		Amount:        float64(rand.Intn(10000)+100) / 100, // $1.00 - $100.00
		Currency:      currencies[rand.Intn(len(currencies))],
		Status:        statuses[rand.Intn(len(statuses))],
		CustomerEmail: fmt.Sprintf("user%d@example.com", rand.Intn(1000)),
		Metadata:      map[string]string{"source": "test-producer"},
		Timestamp:     time.Now().Unix(),
	}
}