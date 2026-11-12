package circuitbreaker

import (
	"errors"
	"sync"
	"time"

	"github.com/subculture-collective/clustr/backend/internal/metrics"
)

var (
	// ErrCircuitOpen is returned when the circuit breaker is open
	ErrCircuitOpen = errors.New("circuit breaker is open")
)

// State represents the circuit breaker state
type State int

const (
	StateClosed State = iota
	StateOpen
	StateHalfOpen
)

// CircuitBreaker implements a circuit breaker pattern
type CircuitBreaker struct {
	mu              sync.RWMutex
	state           State
	failureCount    int
	successCount    int
	lastFailureTime time.Time
	name            string

	// Configuration
	failureThreshold int
	successThreshold int
	timeout          time.Duration
}

// Config holds circuit breaker configuration
type Config struct {
	Name             string
	FailureThreshold int           // Number of failures before opening
	SuccessThreshold int           // Number of successes needed to close from half-open
	Timeout          time.Duration // Time to wait before trying half-open
}

// New creates a new circuit breaker
func New(cfg Config) *CircuitBreaker {
	if cfg.FailureThreshold == 0 {
		cfg.FailureThreshold = 5
	}
	if cfg.SuccessThreshold == 0 {
		cfg.SuccessThreshold = 2
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 60 * time.Second
	}

	cb := &CircuitBreaker{
		state:            StateClosed,
		name:             cfg.Name,
		failureThreshold: cfg.FailureThreshold,
		successThreshold: cfg.SuccessThreshold,
		timeout:          cfg.Timeout,
	}

	// Initialize metrics
	metrics.CircuitBreakerState.WithLabelValues(cfg.Name).Set(0)

	return cb
}

// Call executes the given function if the circuit breaker allows it
func (cb *CircuitBreaker) Call(fn func() error) error {
	if !cb.canAttempt() {
		return ErrCircuitOpen
	}

	err := fn()
	if err != nil {
		cb.recordFailure()
		return err
	}

	cb.recordSuccess()
	return nil
}

// canAttempt checks if we can attempt the operation
func (cb *CircuitBreaker) canAttempt() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	switch cb.state {
	case StateClosed:
		return true
	case StateOpen:
		if time.Since(cb.lastFailureTime) > cb.timeout {
			// Transition to half-open
			cb.mu.RUnlock()
			cb.mu.Lock()
			cb.state = StateHalfOpen
			cb.successCount = 0
			metrics.CircuitBreakerState.WithLabelValues(cb.name).Set(2)
			cb.mu.Unlock()
			cb.mu.RLock()
			return true
		}
		return false
	case StateHalfOpen:
		return true
	default:
		return false
	}
}

// recordFailure records a failure
func (cb *CircuitBreaker) recordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.lastFailureTime = time.Now()
	cb.successCount = 0

	switch cb.state {
	case StateClosed:
		cb.failureCount++
		if cb.failureCount >= cb.failureThreshold {
			cb.state = StateOpen
			metrics.CircuitBreakerTrips.WithLabelValues(cb.name).Inc()
			metrics.CircuitBreakerState.WithLabelValues(cb.name).Set(1)
		}
	case StateHalfOpen:
		cb.state = StateOpen
		cb.failureCount = 0
		metrics.CircuitBreakerTrips.WithLabelValues(cb.name).Inc()
		metrics.CircuitBreakerState.WithLabelValues(cb.name).Set(1)
	}
}

// recordSuccess records a success
func (cb *CircuitBreaker) recordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		cb.failureCount = 0
	case StateHalfOpen:
		cb.successCount++
		if cb.successCount >= cb.successThreshold {
			cb.state = StateClosed
			cb.failureCount = 0
			cb.successCount = 0
			metrics.CircuitBreakerState.WithLabelValues(cb.name).Set(0)
		}
	}
}

// GetState returns the current state
func (cb *CircuitBreaker) GetState() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}
