/*
Copyright 2024.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

// Package reliability provides reliability patterns like retry logic and circuit breakers.
package reliability

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"
)

var (
	// ErrMaxRetriesExceeded indicates all retry attempts were exhausted
	ErrMaxRetriesExceeded = errors.New("maximum retry attempts exceeded")
	// ErrCircuitOpen indicates the circuit breaker is open
	ErrCircuitOpen = errors.New("circuit breaker is open")
)

// RetryConfig configures the retry behavior
type RetryConfig struct {
	// MaxRetries is the maximum number of retry attempts
	MaxRetries int
	// InitialBackoff is the initial backoff duration
	InitialBackoff time.Duration
	// MaxBackoff is the maximum backoff duration
	MaxBackoff time.Duration
	// BackoffMultiplier is the multiplier for exponential backoff
	BackoffMultiplier float64
	// RetryableErrors is a function that determines if an error is retryable
	RetryableErrors func(error) bool
}

// DefaultRetryConfig returns a default retry configuration
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:        5,
		InitialBackoff:    1 * time.Second,
		MaxBackoff:        30 * time.Second,
		BackoffMultiplier: 2.0,
		RetryableErrors:   IsRetryableError,
	}
}

// RetryOperation executes an operation with exponential backoff retry logic
func RetryOperation(ctx context.Context, config RetryConfig, operation func() error) error {
	var lastErr error
	backoff := config.InitialBackoff

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		// Execute the operation
		err := operation()
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if config.RetryableErrors != nil && !config.RetryableErrors(err) {
			return fmt.Errorf("non-retryable error: %w", err)
		}

		// Don't sleep after the last attempt
		if attempt == config.MaxRetries {
			break
		}

		// Calculate backoff with jitter
		sleepDuration := calculateBackoffWithJitter(backoff, config.MaxBackoff)

		// Check if context is canceled
		select {
		case <-ctx.Done():
			return fmt.Errorf("operation canceled: %w", ctx.Err())
		case <-time.After(sleepDuration):
			// Continue to next attempt
		}

		// Increase backoff for next attempt
		backoff = time.Duration(float64(backoff) * config.BackoffMultiplier)
		if backoff > config.MaxBackoff {
			backoff = config.MaxBackoff
		}
	}

	return fmt.Errorf("%w after %d attempts: %w", ErrMaxRetriesExceeded, config.MaxRetries+1, lastErr)
}

// calculateBackoffWithJitter adds jitter to prevent thundering herd
func calculateBackoffWithJitter(backoff, maxBackoff time.Duration) time.Duration {
	// Add up to 25% jitter
	jitter := float64(backoff) * 0.25
	jitterDuration := time.Duration(jitter * (0.5 + (float64(time.Now().UnixNano()%1000) / 2000.0)))

	total := backoff + jitterDuration
	if total > maxBackoff {
		return maxBackoff
	}
	return total
}

// IsRetryableError determines if an error is retryable
// You can customize this based on your API's error responses
func IsRetryableError(err error) bool {
	if err == nil {
		return false
	}

	// Network errors are usually retryable
	errMsg := err.Error()

	// Temporary network errors
	if errors.Is(err, context.DeadlineExceeded) ||
		errors.Is(err, context.Canceled) {
		return false // Don't retry context errors
	}

	// Check for specific error patterns that are retryable
	retryablePatterns := []string{
		"connection refused",
		"connection reset",
		"timeout",
		"temporary failure",
		"rate limit",
		"too many requests",
		"429",
		"503",
		"502",
		"504",
	}

	for _, pattern := range retryablePatterns {
		if contains(errMsg, pattern) {
			return true
		}
	}

	return false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) &&
		(s[:len(substr)] == substr || s[len(s)-len(substr):] == substr ||
			findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// CircuitBreakerState represents the state of a circuit breaker
type CircuitBreakerState int

const (
	// StateClosed means the circuit is closed and requests are flowing
	StateClosed CircuitBreakerState = iota
	// StateOpen means the circuit is open and requests are blocked
	StateOpen
	// StateHalfOpen means the circuit is testing if it should close
	StateHalfOpen
)

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	maxFailures     int
	resetTimeout    time.Duration
	failureCount    int
	lastFailureTime time.Time
	state           CircuitBreakerState
}

// CircuitBreakerConfig configures the circuit breaker
type CircuitBreakerConfig struct {
	// MaxFailures is the number of failures before opening the circuit
	MaxFailures int
	// ResetTimeout is how long to wait before trying again after opening
	ResetTimeout time.Duration
}

// DefaultCircuitBreakerConfig returns a default circuit breaker configuration
func DefaultCircuitBreakerConfig() CircuitBreakerConfig {
	return CircuitBreakerConfig{
		MaxFailures:  5,
		ResetTimeout: 60 * time.Second,
	}
}

// NewCircuitBreaker creates a new circuit breaker
func NewCircuitBreaker(config CircuitBreakerConfig) *CircuitBreaker {
	return &CircuitBreaker{
		maxFailures:  config.MaxFailures,
		resetTimeout: config.ResetTimeout,
		state:        StateClosed,
	}
}

// Execute runs an operation through the circuit breaker
func (cb *CircuitBreaker) Execute(operation func() error) error {
	// Check if circuit should transition from open to half-open
	switch cb.state {
	case StateOpen:
		if time.Since(cb.lastFailureTime) > cb.resetTimeout {
			cb.state = StateHalfOpen
			cb.failureCount = 0
		} else {
			return ErrCircuitOpen
		}
	case StateClosed, StateHalfOpen:
		// Proceed with operation execution
	}

	// Execute the operation
	err := operation()

	if err != nil {
		cb.onFailure()
		return err
	}

	cb.onSuccess()
	return nil
}

// onFailure is called when an operation fails
func (cb *CircuitBreaker) onFailure() {
	cb.failureCount++
	cb.lastFailureTime = time.Now()

	if cb.state == StateHalfOpen {
		// If it fails in half-open state, go back to open
		cb.state = StateOpen
	} else if cb.failureCount >= cb.maxFailures {
		// Open the circuit if max failures reached
		cb.state = StateOpen
	}
}

// onSuccess is called when an operation succeeds
func (cb *CircuitBreaker) onSuccess() {
	switch cb.state {
	case StateHalfOpen:
		// If it succeeds in half-open state, close the circuit
		cb.state = StateClosed
		cb.failureCount = 0
	case StateClosed:
		// Reset failure count on success
		cb.failureCount = 0
	}
}

// GetState returns the current state of the circuit breaker
func (cb *CircuitBreaker) GetState() CircuitBreakerState {
	return cb.state
}

// Reset resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.state = StateClosed
	cb.failureCount = 0
}

// RetryWithCircuitBreaker combines retry logic with circuit breaker pattern
func RetryWithCircuitBreaker(
	ctx context.Context,
	retryConfig RetryConfig,
	cb *CircuitBreaker,
	operation func() error,
) error {
	return RetryOperation(ctx, retryConfig, func() error {
		return cb.Execute(operation)
	})
}

// ExponentialBackoff calculates the backoff duration for a given attempt
func ExponentialBackoff(attempt int, initialBackoff, maxBackoff time.Duration) time.Duration {
	backoff := float64(initialBackoff) * math.Pow(2.0, float64(attempt))
	if backoff > float64(maxBackoff) {
		return maxBackoff
	}
	return time.Duration(backoff)
}
