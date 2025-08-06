package retry

import (
	"context"
	"fmt"
	"math"
	"math/rand"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// Config holds the configuration for retry operations
type Config struct {
	// MaxRetries is the maximum number of retry attempts
	MaxRetries int
	// InitialDelay is the initial delay before the first retry
	InitialDelay time.Duration
	// MaxDelay is the maximum delay between retries
	MaxDelay time.Duration
	// Multiplier is the multiplier for exponential backoff
	Multiplier float64
	// Jitter adds randomness to the delay to avoid thundering herd
	Jitter bool
	// OnRetry is called before each retry attempt
	OnRetry func(attempt int, err error)
}

// DefaultConfig returns a default retry configuration
func DefaultConfig() Config {
	return Config{
		MaxRetries:   5,
		InitialDelay: time.Second,
		MaxDelay:     time.Minute * 5,
		Multiplier:   2.0,
		Jitter:       true,
	}
}

// Retrier handles retry operations with exponential backoff
type Retrier struct {
	config Config
}

// NewRetrier creates a new retrier with the given configuration
func NewRetrier(config Config) *Retrier {
	// Set defaults if not provided
	if config.MaxRetries == 0 {
		config.MaxRetries = 5
	}
	if config.InitialDelay == 0 {
		config.InitialDelay = time.Second
	}
	if config.MaxDelay == 0 {
		config.MaxDelay = time.Minute * 5
	}
	if config.Multiplier == 0 {
		config.Multiplier = 2.0
	}

	return &Retrier{config: config}
}

// Metrics for retry operations
var (
	retryAttempts = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mcp_retry_attempts_total",
			Help: "Total number of retry attempts",
		},
		[]string{"operation", "result"},
	)

	retryDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "mcp_retry_duration_seconds",
			Help:    "Duration of retry operations in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"operation"},
	)

	retryBackoffDelay = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "mcp_retry_backoff_delay_seconds",
			Help:    "Backoff delay between retry attempts in seconds",
			Buckets: []float64{0.1, 0.5, 1, 2, 5, 10, 30, 60, 120, 300},
		},
		[]string{"operation"},
	)
)

// init registers metrics
func init() {
	metrics.Registry.MustRegister(
		retryAttempts,
		retryDuration,
		retryBackoffDelay,
	)
}

// RetryableFunc is a function that can be retried
type RetryableFunc func(ctx context.Context) error

// IsRetryableFunc determines if an error is retryable
type IsRetryableFunc func(error) bool

// DefaultIsRetryable returns true for most errors, excluding context cancellation
func DefaultIsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// Don't retry context cancellation or deadline exceeded
	if err == context.Canceled || err == context.DeadlineExceeded {
		return false
	}

	// Retry most other errors
	return true
}

// Do executes the function with retry logic
func (r *Retrier) Do(ctx context.Context, operation string, fn RetryableFunc) error {
	return r.DoWithRetryable(ctx, operation, fn, DefaultIsRetryable)
}

// DoWithRetryable executes the function with retry logic and custom retryable check
func (r *Retrier) DoWithRetryable(ctx context.Context, operation string, fn RetryableFunc, isRetryable IsRetryableFunc) error {
	startTime := time.Now()
	var lastErr error

	for attempt := 0; attempt <= r.config.MaxRetries; attempt++ {
		// Execute the function
		err := fn(ctx)
		if err == nil {
			// Success
			retryAttempts.WithLabelValues(operation, "success").Inc()
			retryDuration.WithLabelValues(operation).Observe(time.Since(startTime).Seconds())
			return nil
		}

		lastErr = err

		// Check if we should retry
		if !isRetryable(err) {
			retryAttempts.WithLabelValues(operation, "non_retryable").Inc()
			retryDuration.WithLabelValues(operation).Observe(time.Since(startTime).Seconds())
			return fmt.Errorf("non-retryable error after %d attempts: %w", attempt+1, err)
		}

		// Check if we've exhausted all attempts
		if attempt == r.config.MaxRetries {
			retryAttempts.WithLabelValues(operation, "exhausted").Inc()
			retryDuration.WithLabelValues(operation).Observe(time.Since(startTime).Seconds())
			return fmt.Errorf("max retries (%d) exceeded: %w", r.config.MaxRetries, err)
		}

		// Call retry callback if provided
		if r.config.OnRetry != nil {
			r.config.OnRetry(attempt+1, err)
		}

		// Calculate delay for next attempt
		delay := r.calculateDelay(attempt)
		retryBackoffDelay.WithLabelValues(operation).Observe(delay.Seconds())

		// Record retry attempt
		retryAttempts.WithLabelValues(operation, "retry").Inc()

		// Wait before next attempt
		select {
		case <-ctx.Done():
			retryAttempts.WithLabelValues(operation, "cancelled").Inc()
			retryDuration.WithLabelValues(operation).Observe(time.Since(startTime).Seconds())
			return fmt.Errorf("context cancelled during retry: %w", ctx.Err())
		case <-time.After(delay):
			// Continue to next attempt
		}
	}

	// This should never be reached, but just in case
	retryAttempts.WithLabelValues(operation, "unexpected").Inc()
	retryDuration.WithLabelValues(operation).Observe(time.Since(startTime).Seconds())
	return fmt.Errorf("unexpected end of retry loop: %w", lastErr)
}

// calculateDelay calculates the delay for the given attempt using exponential backoff
func (r *Retrier) calculateDelay(attempt int) time.Duration {
	// Calculate exponential backoff delay
	delay := float64(r.config.InitialDelay) * math.Pow(r.config.Multiplier, float64(attempt))

	// Apply maximum delay limit
	if delay > float64(r.config.MaxDelay) {
		delay = float64(r.config.MaxDelay)
	}

	// Add jitter if enabled
	if r.config.Jitter {
		// Add up to 10% jitter
		jitterRange := delay * 0.1
		jitter := (rand.Float64() - 0.5) * 2 * jitterRange
		delay += jitter

		// Ensure delay is not negative
		if delay < 0 {
			delay = float64(r.config.InitialDelay)
		}
	}

	return time.Duration(delay)
}

// DoWithTimeout executes the function with retry logic and a timeout
func (r *Retrier) DoWithTimeout(ctx context.Context, timeout time.Duration, operation string, fn RetryableFunc) error {
	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	return r.Do(timeoutCtx, operation, fn)
}

// DoAsync executes the function with retry logic asynchronously
func (r *Retrier) DoAsync(ctx context.Context, operation string, fn RetryableFunc) <-chan error {
	errChan := make(chan error, 1)

	go func() {
		defer close(errChan)
		err := r.Do(ctx, operation, fn)
		if err != nil {
			errChan <- err
		}
	}()

	return errChan
}

// LinearRetrier implements linear backoff instead of exponential
type LinearRetrier struct {
	config LinearConfig
}

// LinearConfig holds configuration for linear retry
type LinearConfig struct {
	MaxRetries int
	Delay      time.Duration
	OnRetry    func(attempt int, err error)
}

// NewLinearRetrier creates a new linear retrier
func NewLinearRetrier(config LinearConfig) *LinearRetrier {
	if config.MaxRetries == 0 {
		config.MaxRetries = 3
	}
	if config.Delay == 0 {
		config.Delay = time.Second
	}

	return &LinearRetrier{config: config}
}

// Do executes the function with linear retry logic
func (lr *LinearRetrier) Do(ctx context.Context, operation string, fn RetryableFunc) error {
	return lr.DoWithRetryable(ctx, operation, fn, DefaultIsRetryable)
}

// DoWithRetryable executes the function with linear retry logic and custom retryable check
func (lr *LinearRetrier) DoWithRetryable(ctx context.Context, operation string, fn RetryableFunc, isRetryable IsRetryableFunc) error {
	startTime := time.Now()
	var lastErr error

	for attempt := 0; attempt <= lr.config.MaxRetries; attempt++ {
		// Execute the function
		err := fn(ctx)
		if err == nil {
			// Success
			retryAttempts.WithLabelValues(operation, "success").Inc()
			retryDuration.WithLabelValues(operation).Observe(time.Since(startTime).Seconds())
			return nil
		}

		lastErr = err

		// Check if we should retry
		if !isRetryable(err) {
			retryAttempts.WithLabelValues(operation, "non_retryable").Inc()
			retryDuration.WithLabelValues(operation).Observe(time.Since(startTime).Seconds())
			return fmt.Errorf("non-retryable error after %d attempts: %w", attempt+1, err)
		}

		// Check if we've exhausted all attempts
		if attempt == lr.config.MaxRetries {
			retryAttempts.WithLabelValues(operation, "exhausted").Inc()
			retryDuration.WithLabelValues(operation).Observe(time.Since(startTime).Seconds())
			return fmt.Errorf("max retries (%d) exceeded: %w", lr.config.MaxRetries, err)
		}

		// Call retry callback if provided
		if lr.config.OnRetry != nil {
			lr.config.OnRetry(attempt+1, err)
		}

		// Record retry attempt
		retryAttempts.WithLabelValues(operation, "retry").Inc()
		retryBackoffDelay.WithLabelValues(operation).Observe(lr.config.Delay.Seconds())

		// Wait before next attempt (linear delay)
		select {
		case <-ctx.Done():
			retryAttempts.WithLabelValues(operation, "cancelled").Inc()
			retryDuration.WithLabelValues(operation).Observe(time.Since(startTime).Seconds())
			return fmt.Errorf("context cancelled during retry: %w", ctx.Err())
		case <-time.After(lr.config.Delay):
			// Continue to next attempt
		}
	}

	// This should never be reached, but just in case
	retryAttempts.WithLabelValues(operation, "unexpected").Inc()
	retryDuration.WithLabelValues(operation).Observe(time.Since(startTime).Seconds())
	return fmt.Errorf("unexpected end of retry loop: %w", lastErr)
}

// Helper functions for common retry scenarios

// RetryHTTPRequest retries HTTP requests with appropriate error checking
func RetryHTTPRequest(ctx context.Context, operation string, fn RetryableFunc) error {
	retrier := NewRetrier(Config{
		MaxRetries:   3,
		InitialDelay: time.Second,
		MaxDelay:     time.Second * 30,
		Multiplier:   2.0,
		Jitter:       true,
	})

	return retrier.Do(ctx, operation, fn)
}

// RetryKubernetesOperation retries Kubernetes API operations
func RetryKubernetesOperation(ctx context.Context, operation string, fn RetryableFunc) error {
	retrier := NewRetrier(Config{
		MaxRetries:   5,
		InitialDelay: time.Millisecond * 500,
		MaxDelay:     time.Second * 10,
		Multiplier:   1.5,
		Jitter:       true,
	})

	return retrier.Do(ctx, operation, fn)
}

// RetryDatabaseOperation retries database operations
func RetryDatabaseOperation(ctx context.Context, operation string, fn RetryableFunc) error {
	retrier := NewRetrier(Config{
		MaxRetries:   3,
		InitialDelay: time.Millisecond * 100,
		MaxDelay:     time.Second * 5,
		Multiplier:   2.0,
		Jitter:       false, // Database operations might benefit from consistent timing
	})

	return retrier.Do(ctx, operation, fn)
}
