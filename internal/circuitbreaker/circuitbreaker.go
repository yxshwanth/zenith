package circuitbreaker

import (
	"context"
	"sync"
	"time"
)

// State represents the circuit breaker state
type State int

const (
	// StateClosed means the circuit is closed and requests flow normally
	StateClosed State = iota
	// StateOpen means the circuit is open and requests are rejected immediately
	StateOpen
	// StateHalfOpen means the circuit is testing if the service has recovered
	StateHalfOpen
)

// String returns the string representation of the state
func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half_open"
	default:
		return "unknown"
	}
}

// Config holds circuit breaker configuration
type Config struct {
	// FailureThreshold is the percentage of failures that trigger the circuit to open (0-100)
	FailureThreshold float64
	// FailureWindow is the time window for calculating failure rate
	FailureWindow time.Duration
	// OpenDuration is how long the circuit stays open before transitioning to half-open
	OpenDuration time.Duration
	// HalfOpenMaxRequests is the maximum number of requests allowed in half-open state
	HalfOpenMaxRequests int
	// MinRequests is the minimum number of requests needed before the circuit can open
	MinRequests int
}

// DefaultConfig returns a default circuit breaker configuration
func DefaultConfig() Config {
	return Config{
		FailureThreshold:    50.0,  // 50% failure rate
		FailureWindow:       1 * time.Minute,
		OpenDuration:         30 * time.Second,
		HalfOpenMaxRequests: 5,
		MinRequests:         10,
	}
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	config Config
	mu     sync.RWMutex

	state State

	// Failure tracking
	failures    []time.Time
	successes   []time.Time
	halfOpenReq int

	// State transitions
	stateChangedAt time.Time
}

// NewCircuitBreaker creates a new circuit breaker with the given configuration
func NewCircuitBreaker(config Config) *CircuitBreaker {
	return &CircuitBreaker{
		config:        config,
		state:         StateClosed,
		failures:      make([]time.Time, 0),
		successes:     make([]time.Time, 0),
		stateChangedAt: time.Now(),
	}
}

// IsOpen returns true if the circuit breaker is open
func (cb *CircuitBreaker) IsOpen() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state == StateOpen
}

// IsClosed returns true if the circuit breaker is closed
func (cb *CircuitBreaker) IsClosed() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state == StateClosed
}

// State returns the current state of the circuit breaker
func (cb *CircuitBreaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// RecordSuccess records a successful operation
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()
	cb.successes = append(cb.successes, now)

	// Clean old successes
	cb.cleanOldEntries(&cb.successes, now)

	// Transition from half-open to closed on success
	if cb.state == StateHalfOpen {
		cb.halfOpenReq++
		if cb.halfOpenReq >= cb.config.HalfOpenMaxRequests {
			cb.transitionTo(StateClosed, now)
		}
	}

	// If we're in half-open and got enough successes, close the circuit
	if cb.state == StateHalfOpen && len(cb.successes) >= cb.config.HalfOpenMaxRequests {
		cb.transitionTo(StateClosed, now)
	}
}

// RecordFailure records a failed operation
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()
	cb.failures = append(cb.failures, now)

	// Clean old failures
	cb.cleanOldEntries(&cb.failures, now)

	// If in half-open state, immediately open on failure
	if cb.state == StateHalfOpen {
		cb.transitionTo(StateOpen, now)
		return
	}

	// Check if we should open the circuit
	if cb.shouldTrip(now) {
		cb.transitionTo(StateOpen, now)
	}
}

// ShouldTrip returns true if the circuit breaker should trip (open)
func (cb *CircuitBreaker) ShouldTrip() bool {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.shouldTrip(time.Now())
}

// shouldTrip checks if the circuit should trip based on failure rate
func (cb *CircuitBreaker) shouldTrip(now time.Time) bool {
	// Need minimum requests before we can trip
	totalRequests := len(cb.failures) + len(cb.successes)
	if totalRequests < cb.config.MinRequests {
		return false
	}

	// Calculate failure rate in the window
	recentFailures := cb.countRecentEntries(cb.failures, now)
	recentSuccesses := cb.countRecentEntries(cb.successes, now)
	recentTotal := recentFailures + recentSuccesses

	if recentTotal == 0 {
		return false
	}

	failureRate := float64(recentFailures) / float64(recentTotal) * 100.0
	return failureRate >= cb.config.FailureThreshold
}

// Allow checks if a request should be allowed
func (cb *CircuitBreaker) Allow(ctx context.Context) bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	// Check if we should transition from open to half-open
	if cb.state == StateOpen {
		if time.Since(cb.stateChangedAt) >= cb.config.OpenDuration {
			cb.transitionTo(StateHalfOpen, now)
			cb.halfOpenReq = 0
			return true
		}
		return false
	}

	// Closed or half-open: allow the request
	return true
}

// transitionTo transitions to a new state
func (cb *CircuitBreaker) transitionTo(newState State, now time.Time) {
	if cb.state != newState {
		cb.state = newState
		cb.stateChangedAt = now
		if newState == StateHalfOpen {
			cb.halfOpenReq = 0
			// Clear old entries when entering half-open
			cb.failures = cb.failures[:0]
			cb.successes = cb.successes[:0]
		}
	}
}

// cleanOldEntries removes entries older than the failure window
func (cb *CircuitBreaker) cleanOldEntries(entries *[]time.Time, now time.Time) {
	cutoff := now.Add(-cb.config.FailureWindow)
	valid := 0
	for _, t := range *entries {
		if t.After(cutoff) {
			(*entries)[valid] = t
			valid++
		}
	}
	*entries = (*entries)[:valid]
}

// countRecentEntries counts entries within the failure window
func (cb *CircuitBreaker) countRecentEntries(entries []time.Time, now time.Time) int {
	cutoff := now.Add(-cb.config.FailureWindow)
	count := 0
	for _, t := range entries {
		if t.After(cutoff) {
			count++
		}
	}
	return count
}

// Stats returns statistics about the circuit breaker
type Stats struct {
	State            State
	FailureCount     int
	SuccessCount     int
	FailureRate      float64
	TimeInState      time.Duration
	HalfOpenRequests int
}

// GetStats returns current statistics
func (cb *CircuitBreaker) GetStats() Stats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	now := time.Now()
	recentFailures := cb.countRecentEntries(cb.failures, now)
	recentSuccesses := cb.countRecentEntries(cb.successes, now)
	recentTotal := recentFailures + recentSuccesses

	var failureRate float64
	if recentTotal > 0 {
		failureRate = float64(recentFailures) / float64(recentTotal) * 100.0
	}

	return Stats{
		State:            cb.state,
		FailureCount:     recentFailures,
		SuccessCount:     recentSuccesses,
		FailureRate:     failureRate,
		TimeInState:      now.Sub(cb.stateChangedAt),
		HalfOpenRequests: cb.halfOpenReq,
	}
}

