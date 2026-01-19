package retry

import (
	"context"
	"math"
	"math/rand"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Config holds retry configuration
type Config struct {
	MaxRetries     int
	InitialBackoff time.Duration
	MaxBackoff     time.Duration
	BackoffFactor  float64
	Jitter         bool
}

// DefaultConfig returns sensible defaults
func DefaultConfig() Config {
	return Config{
		MaxRetries:     3,
		InitialBackoff: 100 * time.Millisecond,
		MaxBackoff:     10 * time.Second,
		BackoffFactor:  2.0,
		Jitter:         true,
	}
}

// Do executes fn with retries
func Do(ctx context.Context, cfg Config, fn func() error) error {
	var lastErr error

	for attempt := 0; attempt <= cfg.MaxRetries; attempt++ {
		// Check context before attempt
		if ctx.Err() != nil {
			return ctx.Err()
		}

		// Execute function
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if !isRetryable(err) {
			return err
		}

		// Don't sleep after last attempt
		if attempt == cfg.MaxRetries {
			break
		}

		// Calculate backoff
		backoff := calculateBackoff(attempt, cfg)

		// Wait or context cancel
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(backoff):
			// Continue to next attempt
		}
	}

	return lastErr
}

func calculateBackoff(attempt int, cfg Config) time.Duration {
	backoff := float64(cfg.InitialBackoff) * math.Pow(cfg.BackoffFactor, float64(attempt))

	if backoff > float64(cfg.MaxBackoff) {
		backoff = float64(cfg.MaxBackoff)
	}

	if cfg.Jitter {
		// Add ±25% jitter
		jitter := backoff * 0.25 * (rand.Float64()*2 - 1)
		backoff += jitter
	}

	return time.Duration(backoff)
}

func isRetryable(err error) bool {
	st, ok := status.FromError(err)
	if !ok {
		return false
	}

	switch st.Code() {
	case codes.Unavailable, codes.Aborted, codes.ResourceExhausted, codes.DeadlineExceeded:
		return true
	default:
		return false
	}
}
