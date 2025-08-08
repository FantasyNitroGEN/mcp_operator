package retry

import (
	"context"
	"fmt"
	"math/rand"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// GitHubRetryConfig holds configuration for GitHub-specific retry operations
type GitHubRetryConfig struct {
	// MaxRetries is the maximum number of retry attempts for general errors
	MaxRetries int
	// InitialDelay is the initial delay before the first retry
	InitialDelay time.Duration
	// MaxDelay is the maximum delay between retries
	MaxDelay time.Duration
	// Multiplier is the multiplier for exponential backoff
	Multiplier float64
	// Jitter adds randomness to the delay
	Jitter bool

	// Rate limit specific configuration
	RateLimitMaxRetries int
	RateLimitBaseDelay  time.Duration
	RateLimitMaxDelay   time.Duration

	// Network error specific configuration
	NetworkMaxRetries int
	NetworkBaseDelay  time.Duration
	NetworkMaxDelay   time.Duration

	// OnRetry callback for logging and metrics
	OnRetry func(attempt int, err error, errorType GitHubErrorType, delay time.Duration)
}

// DefaultGitHubRetryConfig returns a default configuration for GitHub operations
func DefaultGitHubRetryConfig() GitHubRetryConfig {
	return GitHubRetryConfig{
		MaxRetries:   5,
		InitialDelay: time.Second,
		MaxDelay:     time.Minute * 5,
		Multiplier:   2.0,
		Jitter:       true,

		// Rate limit: fewer retries but longer delays
		RateLimitMaxRetries: 3,
		RateLimitBaseDelay:  time.Minute * 15, // Start with 15 minutes
		RateLimitMaxDelay:   time.Hour,        // Up to 1 hour as requested

		// Network errors: more retries with shorter delays
		NetworkMaxRetries: 5,
		NetworkBaseDelay:  time.Second * 2,
		NetworkMaxDelay:   time.Minute * 2,
	}
}

// GitHubErrorType represents different types of GitHub API errors
type GitHubErrorType int

const (
	GitHubErrorGeneral GitHubErrorType = iota
	GitHubErrorRateLimit
	GitHubErrorNetwork
	GitHubErrorAuth
	GitHubErrorNotFound
)

func (t GitHubErrorType) String() string {
	switch t {
	case GitHubErrorRateLimit:
		return "rate_limit"
	case GitHubErrorNetwork:
		return "network"
	case GitHubErrorAuth:
		return "auth"
	case GitHubErrorNotFound:
		return "not_found"
	default:
		return "general"
	}
}

// GitHubError represents a classified GitHub API error
type GitHubError struct {
	Type           GitHubErrorType
	StatusCode     int
	Message        string
	RetryAfter     time.Duration
	RateLimitReset time.Time
	OriginalError  error
}

func (e *GitHubError) Error() string {
	return fmt.Sprintf("GitHub API error [%s]: %s (status: %d)", e.Type, e.Message, e.StatusCode)
}

// GitHubRetrier handles GitHub-specific retry operations
type GitHubRetrier struct {
	config GitHubRetryConfig
}

// NewGitHubRetrier creates a new GitHub-specific retrier
func NewGitHubRetrier(config GitHubRetryConfig) *GitHubRetrier {
	return &GitHubRetrier{config: config}
}

// Metrics for GitHub retry operations
var (
	githubRetryAttempts = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mcp_github_retry_attempts_total",
			Help: "Total number of GitHub API retry attempts",
		},
		[]string{"operation", "error_type", "result"},
	)

	githubRetryDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "mcp_github_retry_duration_seconds",
			Help:    "Duration of GitHub API retry operations in seconds",
			Buckets: []float64{1, 5, 10, 30, 60, 300, 900, 1800, 3600}, // Up to 1 hour
		},
		[]string{"operation", "error_type"},
	)

	githubRateLimitHits = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mcp_github_rate_limit_hits_total",
			Help: "Total number of GitHub API rate limit hits",
		},
		[]string{"operation"},
	)
)

func init() {
	metrics.Registry.MustRegister(
		githubRetryAttempts,
		githubRetryDuration,
		githubRateLimitHits,
	)
}

// ClassifyGitHubError analyzes an error and classifies it for appropriate retry handling
func ClassifyGitHubError(err error, resp *http.Response) *GitHubError {
	if err == nil {
		return nil
	}

	githubErr := &GitHubError{
		Type:          GitHubErrorGeneral,
		Message:       err.Error(),
		OriginalError: err,
	}

	// Check for network errors
	if isNetworkError(err) {
		githubErr.Type = GitHubErrorNetwork
		return githubErr
	}

	// If we have an HTTP response, analyze the status code
	if resp != nil {
		githubErr.StatusCode = resp.StatusCode

		switch resp.StatusCode {
		case http.StatusForbidden, http.StatusTooManyRequests:
			githubErr.Type = GitHubErrorRateLimit
			githubErr.Message = "GitHub API rate limit exceeded"

			// Parse rate limit headers
			if resetHeader := resp.Header.Get("X-RateLimit-Reset"); resetHeader != "" {
				if resetTime, err := strconv.ParseInt(resetHeader, 10, 64); err == nil {
					githubErr.RateLimitReset = time.Unix(resetTime, 0)
				}
			}

			if retryAfterHeader := resp.Header.Get("Retry-After"); retryAfterHeader != "" {
				if retryAfter, err := strconv.Atoi(retryAfterHeader); err == nil {
					githubErr.RetryAfter = time.Duration(retryAfter) * time.Second
				}
			}

		case http.StatusUnauthorized:
			githubErr.Type = GitHubErrorAuth
			githubErr.Message = "GitHub API authentication failed"

		case http.StatusNotFound:
			githubErr.Type = GitHubErrorNotFound
			githubErr.Message = "GitHub API resource not found"
		}
	}

	return githubErr
}

// isNetworkError checks if an error is a network-related error
func isNetworkError(err error) bool {
	if err == nil {
		return false
	}

	// Check for common network error types
	if _, ok := err.(net.Error); ok {
		return true
	}

	if _, ok := err.(*net.OpError); ok {
		return true
	}

	if _, ok := err.(*net.DNSError); ok {
		return true
	}

	// Check error message for common network error patterns
	errMsg := strings.ToLower(err.Error())
	networkPatterns := []string{
		"connection refused",
		"connection reset",
		"connection timeout",
		"network is unreachable",
		"no such host",
		"timeout",
		"dns",
		"tcp",
		"i/o timeout",
	}

	for _, pattern := range networkPatterns {
		if strings.Contains(errMsg, pattern) {
			return true
		}
	}

	return false
}

// DoWithGitHubRetry executes a function with GitHub-specific retry logic
func (r *GitHubRetrier) DoWithGitHubRetry(ctx context.Context, operation string, fn func(ctx context.Context) (*http.Response, error)) error {
	logger := log.FromContext(ctx).WithValues("operation", operation)
	startTime := time.Now()

	var lastErr *GitHubError
	maxRetries := r.config.MaxRetries

	for attempt := 0; attempt <= maxRetries; attempt++ {
		// Execute the function
		resp, err := fn(ctx)

		// Classify the error
		githubErr := ClassifyGitHubError(err, resp)
		if githubErr == nil {
			// Success
			githubRetryAttempts.WithLabelValues(operation, "none", "success").Inc()
			githubRetryDuration.WithLabelValues(operation, "none").Observe(time.Since(startTime).Seconds())
			if resp != nil && resp.Body != nil {
				if err := resp.Body.Close(); err != nil {
					logger.V(1).Info("Failed to close response body", "error", err)
				}
			}
			return nil
		}

		lastErr = githubErr

		// Determine retry strategy based on error type
		shouldRetry, retryDelay := r.shouldRetryError(githubErr, attempt)

		if !shouldRetry {
			githubRetryAttempts.WithLabelValues(operation, githubErr.Type.String(), "non_retryable").Inc()
			githubRetryDuration.WithLabelValues(operation, githubErr.Type.String()).Observe(time.Since(startTime).Seconds())
			if resp != nil && resp.Body != nil {
				if err := resp.Body.Close(); err != nil {
					logger.V(1).Info("Failed to close response body", "error", err)
				}
			}
			return fmt.Errorf("non-retryable GitHub error after %d attempts: %w", attempt+1, githubErr)
		}

		// Check if we've exhausted all attempts
		if attempt == maxRetries {
			githubRetryAttempts.WithLabelValues(operation, githubErr.Type.String(), "exhausted").Inc()
			githubRetryDuration.WithLabelValues(operation, githubErr.Type.String()).Observe(time.Since(startTime).Seconds())
			if resp != nil && resp.Body != nil {
				if err := resp.Body.Close(); err != nil {
					logger.V(1).Info("Failed to close response body", "error", err)
				}
			}
			return fmt.Errorf("max retries (%d) exceeded for GitHub operation: %w", maxRetries, githubErr)
		}

		// Record rate limit hits
		if githubErr.Type == GitHubErrorRateLimit {
			githubRateLimitHits.WithLabelValues(operation).Inc()
		}

		// Call retry callback if provided
		if r.config.OnRetry != nil {
			r.config.OnRetry(attempt+1, githubErr, githubErr.Type, retryDelay)
		}

		// Log retry attempt
		logger.Info("GitHub API operation failed, retrying",
			"attempt", attempt+1,
			"maxRetries", maxRetries,
			"errorType", githubErr.Type.String(),
			"statusCode", githubErr.StatusCode,
			"delay", retryDelay,
			"error", githubErr.Message)

		// Record retry attempt
		githubRetryAttempts.WithLabelValues(operation, githubErr.Type.String(), "retry").Inc()

		// Close response body before retry
		if resp != nil && resp.Body != nil {
			if err := resp.Body.Close(); err != nil {
				logger.V(1).Info("Failed to close response body", "error", err)
			}
		}

		// Wait before next attempt
		select {
		case <-ctx.Done():
			githubRetryAttempts.WithLabelValues(operation, githubErr.Type.String(), "cancelled").Inc()
			githubRetryDuration.WithLabelValues(operation, githubErr.Type.String()).Observe(time.Since(startTime).Seconds())
			return fmt.Errorf("context cancelled during GitHub retry: %w", ctx.Err())
		case <-time.After(retryDelay):
			// Continue to next attempt
		}
	}

	// This should never be reached, but just in case
	githubRetryAttempts.WithLabelValues(operation, lastErr.Type.String(), "unexpected").Inc()
	githubRetryDuration.WithLabelValues(operation, lastErr.Type.String()).Observe(time.Since(startTime).Seconds())
	return fmt.Errorf("unexpected end of GitHub retry loop: %w", lastErr)
}

// shouldRetryError determines if an error should be retried and calculates the delay
func (r *GitHubRetrier) shouldRetryError(err *GitHubError, attempt int) (bool, time.Duration) {
	switch err.Type {
	case GitHubErrorRateLimit:
		if attempt >= r.config.RateLimitMaxRetries {
			return false, 0
		}

		// Use X-RateLimit-Reset if available
		if !err.RateLimitReset.IsZero() {
			resetDelay := time.Until(err.RateLimitReset)
			if resetDelay > 0 && resetDelay <= r.config.RateLimitMaxDelay {
				return true, resetDelay
			}
		}

		// Use Retry-After header if available
		if err.RetryAfter > 0 && err.RetryAfter <= r.config.RateLimitMaxDelay {
			return true, err.RetryAfter
		}

		// Fall back to exponential backoff for rate limits
		delay := r.calculateRateLimitDelay(attempt)
		return true, delay

	case GitHubErrorNetwork:
		if attempt >= r.config.NetworkMaxRetries {
			return false, 0
		}
		delay := r.calculateNetworkDelay(attempt)
		return true, delay

	case GitHubErrorAuth, GitHubErrorNotFound:
		// Don't retry authentication or not found errors
		return false, 0

	default:
		// General errors use standard retry logic
		if attempt >= r.config.MaxRetries {
			return false, 0
		}
		delay := r.calculateGeneralDelay(attempt)
		return true, delay
	}
}

// calculateRateLimitDelay calculates delay for rate limit errors
func (r *GitHubRetrier) calculateRateLimitDelay(attempt int) time.Duration {
	delay := float64(r.config.RateLimitBaseDelay) * pow(r.config.Multiplier, float64(attempt))

	if delay > float64(r.config.RateLimitMaxDelay) {
		delay = float64(r.config.RateLimitMaxDelay)
	}

	if r.config.Jitter {
		delay = addJitter(delay)
	}

	return time.Duration(delay)
}

// calculateNetworkDelay calculates delay for network errors
func (r *GitHubRetrier) calculateNetworkDelay(attempt int) time.Duration {
	delay := float64(r.config.NetworkBaseDelay) * pow(r.config.Multiplier, float64(attempt))

	if delay > float64(r.config.NetworkMaxDelay) {
		delay = float64(r.config.NetworkMaxDelay)
	}

	if r.config.Jitter {
		delay = addJitter(delay)
	}

	return time.Duration(delay)
}

// calculateGeneralDelay calculates delay for general errors
func (r *GitHubRetrier) calculateGeneralDelay(attempt int) time.Duration {
	delay := float64(r.config.InitialDelay) * pow(r.config.Multiplier, float64(attempt))

	if delay > float64(r.config.MaxDelay) {
		delay = float64(r.config.MaxDelay)
	}

	if r.config.Jitter {
		delay = addJitter(delay)
	}

	return time.Duration(delay)
}

// Helper functions
func pow(base, exp float64) float64 {
	result := 1.0
	for i := 0; i < int(exp); i++ {
		result *= base
	}
	return result
}

func addJitter(delay float64) float64 {
	jitterRange := delay * 0.1
	jitter := (rand.Float64() - 0.5) * 2 * jitterRange
	result := delay + jitter

	if result < 0 {
		result = delay * 0.1 // Minimum 10% of original delay
	}

	return result
}
