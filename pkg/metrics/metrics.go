package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// MCPServersTotal tracks the total number of MCPServer resources by namespace and phase
	MCPServersTotal = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mcp_servers_total",
			Help: "Total number of MCP servers by namespace and phase",
		},
		[]string{"namespace", "phase"},
	)

	// ReconcileTotal tracks the total number of reconciliation attempts
	ReconcileTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mcp_controller_reconcile_total",
			Help: "Total number of reconciliation attempts",
		},
		[]string{"namespace", "result"},
	)

	// ReconcileErrors tracks reconciliation errors by type
	ReconcileErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mcp_controller_reconcile_errors_total",
			Help: "Total number of reconcile errors by type",
		},
		[]string{"namespace", "error_type"},
	)

	// ReconcileDuration tracks reconciliation duration
	ReconcileDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "mcp_controller_reconcile_duration_seconds",
			Help:    "Duration of reconciliation operations in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"namespace", "phase"},
	)

	// DeploymentOperations tracks deployment operations
	DeploymentOperations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mcp_deployment_operations_total",
			Help: "Total number of deployment operations",
		},
		[]string{"namespace", "operation", "result"},
	)

	// ServiceOperations tracks service operations
	ServiceOperations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mcp_service_operations_total",
			Help: "Total number of service operations",
		},
		[]string{"namespace", "operation", "result"},
	)

	// RegistryOperations tracks registry loading operations
	RegistryOperations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mcp_registry_operations_total",
			Help: "Total number of registry operations",
		},
		[]string{"namespace", "registry_server", "result"},
	)

	// RegistryDuration tracks registry operation duration
	RegistryDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "mcp_registry_operation_duration_seconds",
			Help:    "Duration of registry operations in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"namespace", "registry_server"},
	)

	// CleanupOperations tracks cleanup operations during deletion
	CleanupOperations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mcp_cleanup_operations_total",
			Help: "Total number of cleanup operations during deletion",
		},
		[]string{"namespace", "operation", "result"},
	)

	// CleanupDuration tracks cleanup operation duration
	CleanupDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "mcp_cleanup_duration_seconds",
			Help:    "Duration of cleanup operations in seconds",
			Buckets: prometheus.DefBuckets,
		},
		[]string{"namespace", "operation"},
	)

	// MCPServerInfo provides information about MCPServer resources
	MCPServerInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "mcp_server_info",
			Help: "Information about MCP servers",
		},
		[]string{"namespace", "name", "registry_name", "runtime_type", "image", "version"},
	)

	// RegistryReconciliations tracks registry reconciliation attempts
	RegistryReconciliations = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "mcp_registry_reconciliations_total",
			Help: "Total number of registry reconciliation attempts",
		},
		[]string{"name", "namespace"},
	)
)

// init registers all metrics with the controller-runtime metrics registry
func init() {
	metrics.Registry.MustRegister(
		MCPServersTotal,
		ReconcileTotal,
		ReconcileErrors,
		ReconcileDuration,
		DeploymentOperations,
		ServiceOperations,
		RegistryOperations,
		RegistryDuration,
		CleanupOperations,
		CleanupDuration,
		MCPServerInfo,
		RegistryReconciliations,
	)
}

// RecordReconcileStart records the start of a reconciliation
func RecordReconcileStart(namespace string) {
	ReconcileTotal.WithLabelValues(namespace, "started").Inc()
}

// RecordReconcileSuccess records a successful reconciliation
func RecordReconcileSuccess(namespace string, duration float64, phase string) {
	ReconcileTotal.WithLabelValues(namespace, "success").Inc()
	ReconcileDuration.WithLabelValues(namespace, phase).Observe(duration)
}

// RecordReconcileError records a reconciliation error
func RecordReconcileError(namespace, errorType string, duration float64) {
	ReconcileTotal.WithLabelValues(namespace, "error").Inc()
	ReconcileErrors.WithLabelValues(namespace, errorType).Inc()
	ReconcileDuration.WithLabelValues(namespace, "error").Observe(duration)
}

// RecordDeploymentOperation records a deployment operation
func RecordDeploymentOperation(namespace, operation, result string) {
	DeploymentOperations.WithLabelValues(namespace, operation, result).Inc()
}

// RecordServiceOperation records a service operation
func RecordServiceOperation(namespace, operation, result string) {
	ServiceOperations.WithLabelValues(namespace, operation, result).Inc()
}

// RecordRegistryOperation records a registry operation
func RecordRegistryOperation(namespace, registryServer, result string, duration float64) {
	RegistryOperations.WithLabelValues(namespace, registryServer, result).Inc()
	RegistryDuration.WithLabelValues(namespace, registryServer).Observe(duration)
}

// RecordCleanupOperation records a cleanup operation
func RecordCleanupOperation(namespace, operation, result string, duration float64) {
	CleanupOperations.WithLabelValues(namespace, operation, result).Inc()
	CleanupDuration.WithLabelValues(namespace, operation).Observe(duration)
}

// UpdateMCPServerCount updates the count of MCPServers by phase
func UpdateMCPServerCount(namespace, phase string, count float64) {
	MCPServersTotal.WithLabelValues(namespace, phase).Set(count)
}

// UpdateMCPServerInfo updates information about an MCPServer
func UpdateMCPServerInfo(namespace, name, registryName, runtimeType, image, version string) {
	MCPServerInfo.WithLabelValues(namespace, name, registryName, runtimeType, image, version).Set(1)
}

// RemoveMCPServerInfo removes information about an MCPServer
func RemoveMCPServerInfo(namespace, name, registryName, runtimeType, image, version string) {
	MCPServerInfo.DeleteLabelValues(namespace, name, registryName, runtimeType, image, version)
}
