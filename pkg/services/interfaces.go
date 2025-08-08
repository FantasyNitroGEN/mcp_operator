package services

import (
	"context"
	"time"

	mcpv1 "github.com/FantasyNitroGEN/mcp_operator/api/v1"
	"github.com/FantasyNitroGEN/mcp_operator/pkg/registry"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// RegistryService defines the interface for registry operations
type RegistryService interface {
	// FetchServerSpec fetches server specification from registry
	FetchServerSpec(ctx context.Context, registryName, serverName string) (*registry.MCPServerSpec, error)

	// EnrichMCPServer enriches MCPServer with registry data
	EnrichMCPServer(ctx context.Context, mcpServer *mcpv1.MCPServer, registryName string) error

	// ForceEnrichMCPServer enriches MCPServer with registry data, bypassing "already enriched" check
	ForceEnrichMCPServer(ctx context.Context, mcpServer *mcpv1.MCPServer, registryName string) error

	// SyncRegistry synchronizes registry data
	SyncRegistry(ctx context.Context, registry *mcpv1.MCPRegistry) error

	// ListAvailableServers lists all available servers in a registry
	ListAvailableServers(ctx context.Context, registryName string) ([]registry.MCPServerInfo, error)

	// ValidateRegistryConnection validates connection to registry
	ValidateRegistryConnection(ctx context.Context, registry *mcpv1.MCPRegistry) error
}

// DeploymentService defines the interface for deployment operations
type DeploymentService interface {
	// CreateOrUpdateDeployment creates or updates a Kubernetes deployment
	CreateOrUpdateDeployment(ctx context.Context, mcpServer *mcpv1.MCPServer) (*appsv1.Deployment, error)

	// CreateOrUpdateService creates or updates a Kubernetes service
	CreateOrUpdateService(ctx context.Context, mcpServer *mcpv1.MCPServer) (*corev1.Service, error)

	// CreateOrUpdateHPA creates or updates horizontal pod autoscaler
	CreateOrUpdateHPA(ctx context.Context, mcpServer *mcpv1.MCPServer) (*autoscalingv2.HorizontalPodAutoscaler, error)

	// CreateOrUpdateVPA creates or updates vertical pod autoscaler
	CreateOrUpdateVPA(ctx context.Context, mcpServer *mcpv1.MCPServer) (*unstructured.Unstructured, error)

	// CreateOrUpdateNetworkPolicy creates or updates network policy
	CreateOrUpdateNetworkPolicy(ctx context.Context, mcpServer *mcpv1.MCPServer) (*networkingv1.NetworkPolicy, error)

	// DeleteResources deletes all resources associated with MCPServer
	DeleteResources(ctx context.Context, mcpServer *mcpv1.MCPServer) error

	// GetDeploymentStatus gets the current status of deployment
	GetDeploymentStatus(ctx context.Context, mcpServer *mcpv1.MCPServer) (*appsv1.Deployment, error)
}

// StatusService defines the interface for status management operations
type StatusService interface {
	// UpdateMCPServerStatus updates the status of MCPServer
	UpdateMCPServerStatus(ctx context.Context, mcpServer *mcpv1.MCPServer) error

	// UpdateMCPRegistryStatus updates the status of MCPRegistry
	UpdateMCPRegistryStatus(ctx context.Context, registry *mcpv1.MCPRegistry) error

	// SetCondition sets a condition on MCPServer
	SetCondition(mcpServer *mcpv1.MCPServer, conditionType mcpv1.MCPServerConditionType, status string, reason, message string)

	// SetRegistryCondition sets a condition on MCPRegistry
	SetRegistryCondition(registry *mcpv1.MCPRegistry, conditionType mcpv1.MCPRegistryConditionType, status string, reason, message string)
}

// ValidationService defines the interface for validation operations
type ValidationService interface {
	// ValidateMCPServer validates MCPServer specification
	ValidateMCPServer(ctx context.Context, mcpServer *mcpv1.MCPServer) error

	// ValidateTenantResourceQuotas validates tenant resource quotas
	ValidateTenantResourceQuotas(ctx context.Context, mcpServer *mcpv1.MCPServer) error

	// ValidateRegistry validates MCPRegistry specification
	ValidateRegistry(ctx context.Context, registry *mcpv1.MCPRegistry) error
}

// ResourceBuilderService defines the interface for building Kubernetes resources
type ResourceBuilderService interface {
	// BuildDeployment builds a deployment for MCPServer
	BuildDeployment(mcpServer *mcpv1.MCPServer) *appsv1.Deployment

	// BuildService builds a service for MCPServer
	BuildService(mcpServer *mcpv1.MCPServer) *corev1.Service

	// BuildHPA builds horizontal pod autoscaler for MCPServer
	BuildHPA(mcpServer *mcpv1.MCPServer) *autoscalingv2.HorizontalPodAutoscaler

	// BuildVPA builds vertical pod autoscaler for MCPServer
	BuildVPA(mcpServer *mcpv1.MCPServer) *unstructured.Unstructured

	// BuildNetworkPolicy builds network policy for MCPServer
	BuildNetworkPolicy(mcpServer *mcpv1.MCPServer) *networkingv1.NetworkPolicy
}

// KubernetesClientService defines the interface for Kubernetes operations
type KubernetesClientService interface {
	// Get retrieves a Kubernetes object
	Get(ctx context.Context, key client.ObjectKey, obj client.Object) error

	// Create creates a Kubernetes object
	Create(ctx context.Context, obj client.Object) error

	// Update updates a Kubernetes object
	Update(ctx context.Context, obj client.Object) error

	// Delete deletes a Kubernetes object
	Delete(ctx context.Context, obj client.Object) error

	// Patch patches a Kubernetes object
	Patch(ctx context.Context, obj client.Object, patch client.Patch) error

	// List lists Kubernetes objects
	List(ctx context.Context, list client.ObjectList, opts ...client.ListOption) error

	// UpdateStatus updates the status of a Kubernetes object
	UpdateStatus(ctx context.Context, obj client.Object) error
}

// RetryService defines the interface for retry operations
type RetryService interface {
	// RetryWithBackoff executes a function with exponential backoff
	RetryWithBackoff(ctx context.Context, operation func() error, maxRetries int) error

	// RetryRegistryOperation retries registry operations with specific backoff
	RetryRegistryOperation(ctx context.Context, operation func() error) error

	// RetryDeploymentOperation retries deployment operations with specific backoff
	RetryDeploymentOperation(ctx context.Context, operation func() error) error
}

// EventService defines the interface for Kubernetes event operations
type EventService interface {
	// RecordEvent records a Kubernetes event
	RecordEvent(obj client.Object, eventType, reason, message string)

	// RecordWarning records a warning event
	RecordWarning(obj client.Object, reason, message string)

	// RecordNormal records a normal event
	RecordNormal(obj client.Object, reason, message string)
}

// AutoUpdateService defines the interface for auto-updating server templates
type AutoUpdateService interface {
	// CheckForUpdates checks if any MCPServers need updates from registry
	CheckForUpdates(ctx context.Context) error

	// UpdateServerFromTemplate updates a specific MCPServer with latest template data
	UpdateServerFromTemplate(ctx context.Context, mcpServer *mcpv1.MCPServer) (bool, error)

	// IsUpdateRequired checks if an MCPServer needs updating based on registry template
	IsUpdateRequired(ctx context.Context, mcpServer *mcpv1.MCPServer) (bool, *registry.MCPServerSpec, error)

	// StartPeriodicSync starts periodic synchronization of templates
	StartPeriodicSync(ctx context.Context, interval time.Duration) error

	// StopPeriodicSync stops periodic synchronization
	StopPeriodicSync()
}
