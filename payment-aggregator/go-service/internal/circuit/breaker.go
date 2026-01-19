package circuit

import (
	"errors"
	"sync"
	"time"
)

var ErrCircuitOpen = errors.New("circuit breaker is open")

type State int

const (
	StateClosed State = iota
	StateOpen
	StateHalfOpen
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "CLOSED"
	case StateOpen:
		return "OPEN"
	case StateHalfOpen:
		return "HALF-OPEN"
	default:
		return "UNKNOWN"
	}
}

type Breaker struct {
	mu sync.RWMutex

	state            State
	failureCount     int
	successCount     int
	failureThreshold int
	successThreshold int
	timeout          time.Duration
	lastFailure      time.Time

	onStateChange func(from, to State)
}

type Config struct {
	FailureThreshold int           // Failures before opening
	SuccessThreshold int           // Successes in half-open before closing
	Timeout          time.Duration // Time before trying again
	OnStateChange    func(from, to State)
}

func NewBreaker(cfg Config) *Breaker {
	if cfg.FailureThreshold == 0 {
		cfg.FailureThreshold = 5
	}
	if cfg.SuccessThreshold == 0 {
		cfg.SuccessThreshold = 2
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Second
	}

	return &Breaker{
		state:            StateClosed,
		failureThreshold: cfg.FailureThreshold,
		successThreshold: cfg.SuccessThreshold,
		timeout:          cfg.Timeout,
		onStateChange:    cfg.OnStateChange,
	}
}

// Execute runs fn if circuit allows
func (b *Breaker) Execute(fn func() error) error {
	if !b.Allow() {
		return ErrCircuitOpen
	}

	err := fn()

	if err != nil {
		b.RecordFailure()
	} else {
		b.RecordSuccess()
	}

	return err
}

// Allow checks if request should be allowed
func (b *Breaker) Allow() bool {
	b.mu.RLock()
	state := b.state
	lastFailure := b.lastFailure
	b.mu.RUnlock()

	switch state {
	case StateClosed:
		return true
	case StateOpen:
		// Check if timeout has passed
		if time.Since(lastFailure) > b.timeout {
			b.toHalfOpen()
			return true
		}
		return false
	case StateHalfOpen:
		return true
	default:
		return false
	}
}

// RecordSuccess records a successful call
func (b *Breaker) RecordSuccess() {
	b.mu.Lock()
	defer b.mu.Unlock()

	switch b.state {
	case StateClosed:
		b.failureCount = 0
	case StateHalfOpen:
		b.successCount++
		if b.successCount >= b.successThreshold {
			b.setState(StateClosed)
			b.failureCount = 0
			b.successCount = 0
		}
	}
}

// RecordFailure records a failed call
func (b *Breaker) RecordFailure() {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.failureCount++
	b.lastFailure = time.Now()

	switch b.state {
	case StateClosed:
		if b.failureCount >= b.failureThreshold {
			b.setState(StateOpen)
		}
	case StateHalfOpen:
		b.setState(StateOpen)
		b.successCount = 0
	}
}

func (b *Breaker) toHalfOpen() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.setState(StateHalfOpen)
	b.successCount = 0
}

func (b *Breaker) setState(new State) {
	old := b.state
	b.state = new
	if b.onStateChange != nil && old != new {
		go b.onStateChange(old, new)
	}
}

// State returns current state
func (b *Breaker) State() State {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.state
}
