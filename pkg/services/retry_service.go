package services

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// DefaultRetryService implements RetryService interface
type DefaultRetryService struct {
	maxRetries           int
	baseDelay            time.Duration
	maxDelay             time.Duration
	registryMaxRetries   int
	deploymentMaxRetries int
}

// NewDefaultRetryService creates a new DefaultRetryService
func NewDefaultRetryService() *DefaultRetryService {
	return &DefaultRetryService{
		maxRetries:           5,
		baseDelay:            time.Second * 2,
		maxDelay:             time.Minute * 5,
		registryMaxRetries:   3,
		deploymentMaxRetries: 5,
	}
}

// RetryWithBackoff executes a function with exponential backoff
func (r *DefaultRetryService) RetryWithBackoff(ctx context.Context, operation func() error, maxRetries int) error {
	logger := log.FromContext(ctx)

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		err := operation()
		if err == nil {
			if attempt > 0 {
				logger.Info("Operation succeeded after retry", "attempt", attempt+1)
			}
			return nil
		}

		lastErr = err
		if attempt == maxRetries-1 {
			break
		}

		// Calculate exponential backoff delay
		delay := r.calculateDelay(attempt)
		logger.Info("Operation failed, retrying",
			"attempt", attempt+1,
			"maxRetries", maxRetries,
			"delay", delay,
			"error", err.Error())

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}

	return fmt.Errorf("operation failed after %d attempts: %w", maxRetries, lastErr)
}

// RetryRegistryOperation retries registry operations with specific backoff
func (r *DefaultRetryService) RetryRegistryOperation(ctx context.Context, operation func() error) error {
	return r.RetryWithBackoff(ctx, operation, r.registryMaxRetries)
}

// RetryDeploymentOperation retries deployment operations with specific backoff
func (r *DefaultRetryService) RetryDeploymentOperation(ctx context.Context, operation func() error) error {
	return r.RetryWithBackoff(ctx, operation, r.deploymentMaxRetries)
}

// calculateDelay calculates the delay for exponential backoff
func (r *DefaultRetryService) calculateDelay(attempt int) time.Duration {
	// Exponential backoff: baseDelay * 2^attempt
	delay := time.Duration(float64(r.baseDelay) * math.Pow(2, float64(attempt)))

	// Cap the delay at maxDelay
	if delay > r.maxDelay {
		delay = r.maxDelay
	}

	return delay
}

// SetMaxRetries sets the default maximum number of retries
func (r *DefaultRetryService) SetMaxRetries(maxRetries int) {
	r.maxRetries = maxRetries
}

// SetBaseDelay sets the base delay for exponential backoff
func (r *DefaultRetryService) SetBaseDelay(delay time.Duration) {
	r.baseDelay = delay
}

// SetMaxDelay sets the maximum delay for exponential backoff
func (r *DefaultRetryService) SetMaxDelay(delay time.Duration) {
	r.maxDelay = delay
}

// SetRegistryMaxRetries sets the maximum retries for registry operations
func (r *DefaultRetryService) SetRegistryMaxRetries(maxRetries int) {
	r.registryMaxRetries = maxRetries
}

// SetDeploymentMaxRetries sets the maximum retries for deployment operations
func (r *DefaultRetryService) SetDeploymentMaxRetries(maxRetries int) {
	r.deploymentMaxRetries = maxRetries
}

// GetRetryConfig returns the current retry configuration
func (r *DefaultRetryService) GetRetryConfig() map[string]interface{} {
	return map[string]interface{}{
		"maxRetries":           r.maxRetries,
		"baseDelay":            r.baseDelay,
		"maxDelay":             r.maxDelay,
		"registryMaxRetries":   r.registryMaxRetries,
		"deploymentMaxRetries": r.deploymentMaxRetries,
	}
}

// LogRetryAttempt logs a retry attempt with context
func (r *DefaultRetryService) LogRetryAttempt(logger logr.Logger, attempt, maxRetries int, err error, delay time.Duration) {
	logger.Info("Retry attempt",
		"attempt", attempt,
		"maxRetries", maxRetries,
		"error", err.Error(),
		"nextDelay", delay,
	)
}
