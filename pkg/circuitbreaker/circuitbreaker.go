package circuitbreaker

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// State represents the circuit breaker state
type State int

const (
	// StateClosed means the circuit breaker is closed and requests are allowed
	StateClosed State = iota
	// StateOpen means the circuit breaker is open and requests are rejected
	StateOpen
	// StateHalfOpen means the circuit breaker is testing if the service has recovered
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
		return "half-open"
	default:
		return "unknown"
	}
}

// Config holds the configuration for the circuit breaker
type Config struct {
	// Name is the name of the circuit breaker for metrics and logging
	Name string
	// MaxFailures is the maximum number of failures before opening the circuit
	MaxFailures int
	// Timeout is the duration to wait before transitioning from open to half-open
	Timeout time.Duration
	// MaxRequests is the maximum number of requests allowed in half-open state
	MaxRequests int
	// OnStateChange is called when the state changes
	OnStateChange func(name string, from State, to State)
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	config          Config
	state           State
	failures        int
	requests        int
	lastFailureTime time.Time
	mutex           sync.RWMutex
}

// Metrics for circuit breaker
var (
	circuitBreakerState = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mcp_circuit_breaker_state",
			Help: "Current state of circuit breakers (0=closed, 1=open, 2=half-open)",
		},
		[]string{"name"},
	)

	circuitBreakerRequests = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mcp_circuit_breaker_requests_total",
			Help: "Total number of requests through circuit breaker",
		},
		[]string{"name", "result"},
	)

	circuitBreakerFailures = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mcp_circuit_breaker_failures_total",
			Help: "Total number of failures in circuit breaker",
		},
		[]string{"name"},
	)

	circuitBreakerStateChanges = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mcp_circuit_breaker_state_changes_total",
			Help: "Total number of state changes in circuit breaker",
		},
		[]string{"name", "from_state", "to_state"},
	)
)

// init registers metrics
func init() {
	metrics.Registry.MustRegister(
		circuitBreakerState,
		circuitBreakerRequests,
		circuitBreakerFailures,
		circuitBreakerStateChanges,
	)
}

// NewCircuitBreaker creates a new circuit breaker with the given configuration
func NewCircuitBreaker(config Config) *CircuitBreaker {
	// Set default values
	if config.MaxFailures == 0 {
		config.MaxFailures = 5
	}
	if config.Timeout == 0 {
		config.Timeout = 60 * time.Second
	}
	if config.MaxRequests == 0 {
		config.MaxRequests = 1
	}
	if config.Name == "" {
		config.Name = "default"
	}

	cb := &CircuitBreaker{
		config: config,
		state:  StateClosed,
	}

	// Initialize metrics
	circuitBreakerState.WithLabelValues(config.Name).Set(float64(StateClosed))

	return cb
}

// ErrCircuitBreakerOpen is returned when the circuit breaker is open
var ErrCircuitBreakerOpen = errors.New("circuit breaker is open")

// ErrTooManyRequests is returned when too many requests are made in half-open state
var ErrTooManyRequests = errors.New("too many requests in half-open state")

// Call executes the given function with circuit breaker protection
func (cb *CircuitBreaker) Call(ctx context.Context, fn func(context.Context) error) error {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	// Check if we can make the request
	if err := cb.beforeRequest(); err != nil {
		circuitBreakerRequests.WithLabelValues(cb.config.Name, "rejected").Inc()
		return err
	}

	// Execute the function
	err := fn(ctx)

	// Handle the result
	cb.afterRequest(err)

	if err != nil {
		circuitBreakerRequests.WithLabelValues(cb.config.Name, "failed").Inc()
	} else {
		circuitBreakerRequests.WithLabelValues(cb.config.Name, "success").Inc()
	}

	return err
}

// beforeRequest checks if the request can be made and updates counters
func (cb *CircuitBreaker) beforeRequest() error {
	switch cb.state {
	case StateClosed:
		return nil
	case StateOpen:
		// Check if we should transition to half-open
		if time.Since(cb.lastFailureTime) > cb.config.Timeout {
			cb.setState(StateHalfOpen)
			cb.requests = 0
			return nil
		}
		return ErrCircuitBreakerOpen
	case StateHalfOpen:
		if cb.requests >= cb.config.MaxRequests {
			return ErrTooManyRequests
		}
		cb.requests++
		return nil
	default:
		return ErrCircuitBreakerOpen
	}
}

// afterRequest handles the result of the request and updates state
func (cb *CircuitBreaker) afterRequest(err error) {
	if err != nil {
		cb.onFailure()
	} else {
		cb.onSuccess()
	}
}

// onFailure handles a failed request
func (cb *CircuitBreaker) onFailure() {
	cb.failures++
	cb.lastFailureTime = time.Now()
	circuitBreakerFailures.WithLabelValues(cb.config.Name).Inc()

	switch cb.state {
	case StateClosed:
		if cb.failures >= cb.config.MaxFailures {
			cb.setState(StateOpen)
		}
	case StateHalfOpen:
		cb.setState(StateOpen)
	}
}

// onSuccess handles a successful request
func (cb *CircuitBreaker) onSuccess() {
	switch cb.state {
	case StateClosed:
		cb.failures = 0
	case StateHalfOpen:
		cb.setState(StateClosed)
		cb.failures = 0
		cb.requests = 0
	}
}

// setState changes the circuit breaker state and triggers callbacks
func (cb *CircuitBreaker) setState(newState State) {
	if cb.state == newState {
		return
	}

	oldState := cb.state
	cb.state = newState

	// Update metrics
	circuitBreakerState.WithLabelValues(cb.config.Name).Set(float64(newState))
	circuitBreakerStateChanges.WithLabelValues(cb.config.Name, oldState.String(), newState.String()).Inc()

	// Call state change callback
	if cb.config.OnStateChange != nil {
		cb.config.OnStateChange(cb.config.Name, oldState, newState)
	}
}

// GetState returns the current state of the circuit breaker
func (cb *CircuitBreaker) GetState() State {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.state
}

// GetFailures returns the current number of failures
func (cb *CircuitBreaker) GetFailures() int {
	cb.mutex.RLock()
	defer cb.mutex.RUnlock()
	return cb.failures
}

// Reset resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mutex.Lock()
	defer cb.mutex.Unlock()

	cb.setState(StateClosed)
	cb.failures = 0
	cb.requests = 0
}
