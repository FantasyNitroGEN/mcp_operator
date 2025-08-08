package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	mcpv1 "github.com/FantasyNitroGEN/mcp_operator/api/v1"
	"github.com/FantasyNitroGEN/mcp_operator/pkg/metrics"
	"github.com/FantasyNitroGEN/mcp_operator/pkg/services"
)

const (
	// MCPServerFinalizer is the finalizer used for MCPServer resources
	MCPServerFinalizer = "mcp.allbeone.io/finalizer"
)

// MCPServerReconciler reconciles a MCPServer object with decomposed architecture
type MCPServerReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Service dependencies - injected for better testability and modularity
	RegistryService   services.RegistryService
	DeploymentService services.DeploymentService
	StatusService     services.StatusService
	ValidationService services.ValidationService
	RetryService      services.RetryService
	EventService      services.EventService
	AutoUpdateService services.AutoUpdateService
	CacheService      services.CacheService
}

// +kubebuilder:rbac:groups=mcp.allbeone.io,resources=mcpservers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mcp.allbeone.io,resources=mcpservers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=mcp.allbeone.io,resources=mcpservers/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *MCPServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Create a structured logger with correlation ID and context
	reconcileID := uuid.New().String()
	logger := log.FromContext(ctx).WithValues(
		"mcpserver", req.NamespacedName,
		"reconcile_id", reconcileID,
		"controller", "MCPServerReconciler",
	)

	startTime := time.Now()
	logger.Info("Starting MCPServer reconciliation",
		"namespace", req.Namespace,
		"name", req.Name,
	)

	// Record reconciliation start
	metrics.RecordReconcileStart(req.Namespace)

	// Fetch the MCPServer instance
	mcpServer := &mcpv1.MCPServer{}
	if err := r.Get(ctx, req.NamespacedName, mcpServer); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("MCPServer resource not found, assuming deletion",
				"duration", time.Since(startTime),
			)
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get MCPServer",
			"duration", time.Since(startTime),
			"error_type", "resource_fetch_error",
		)
		metrics.RecordReconcileError(req.Namespace, "resource_fetch_error", time.Since(startTime).Seconds())
		return ctrl.Result{}, err
	}

	// Add MCPServer details to logger context
	logger = logger.WithValues(
		"generation", mcpServer.Generation,
		"resource_version", mcpServer.ResourceVersion,
		"current_phase", mcpServer.Status.Phase,
		"registry_name", mcpServer.Spec.Registry.Name,
		"runtime_type", mcpServer.Spec.Runtime.Type,
	)

	logger.Info("MCPServer resource found",
		"replicas", mcpServer.Spec.Replicas,
		"image", mcpServer.Spec.Runtime.Image,
		"port", mcpServer.Spec.Runtime.Port,
	)

	// Update MCPServer info metrics
	metrics.UpdateMCPServerInfo(
		mcpServer.Namespace,
		mcpServer.Name,
		mcpServer.Spec.Registry.Name,
		mcpServer.Spec.Runtime.Type,
		mcpServer.Spec.Runtime.Image,
		mcpServer.Spec.Registry.Version,
	)

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(mcpServer, MCPServerFinalizer) {
		controllerutil.AddFinalizer(mcpServer, MCPServerFinalizer)
		if err := r.Update(ctx, mcpServer); err != nil {
			logger.Error(err, "Failed to add finalizer")
			metrics.RecordReconcileError(req.Namespace, "finalizer_add_error", time.Since(startTime).Seconds())
			return ctrl.Result{}, err
		}
		logger.Info("Added finalizer to MCPServer")
	}

	// Handle deletion
	if mcpServer.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, logger, mcpServer)
	}

	// Validate MCPServer specification
	if err := r.ValidationService.ValidateMCPServer(ctx, mcpServer); err != nil {
		logger.Error(err, "MCPServer validation failed")
		r.StatusService.SetCondition(mcpServer, mcpv1.MCPServerConditionReady,
			string(metav1.ConditionFalse), "ValidationFailed", fmt.Sprintf("MCPServer validation failed: %v", err))
		r.EventService.RecordWarning(mcpServer, "ValidationFailed", fmt.Sprintf("MCPServer validation failed: %v", err))

		if statusErr := r.StatusService.UpdateMCPServerStatus(ctx, mcpServer); statusErr != nil {
			logger.Error(statusErr, "Failed to update MCPServer status")
		}
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	}

	// Validate tenant resource quotas if multi-tenancy is enabled
	if mcpServer.Spec.Tenancy != nil {
		if err := r.ValidationService.ValidateTenantResourceQuotas(ctx, mcpServer); err != nil {
			logger.Error(err, "Tenant resource quota validation failed")
			r.StatusService.SetCondition(mcpServer, mcpv1.MCPServerConditionReady,
				string(metav1.ConditionFalse), "TenantValidationFailed", fmt.Sprintf("Tenant validation failed: %v", err))
			r.EventService.RecordWarning(mcpServer, "TenantValidationFailed", fmt.Sprintf("Tenant validation failed: %v", err))

			if statusErr := r.StatusService.UpdateMCPServerStatus(ctx, mcpServer); statusErr != nil {
				logger.Error(statusErr, "Failed to update MCPServer status")
			}
			return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
		}
	}

	// Enrich MCPServer with registry data if registry is specified
	if mcpServer.Spec.Registry.Name != "" {
		logger.Info("Enriching server from registry",
			"server_name", mcpServer.Spec.Registry.Name,
			"phase", "registry_enrichment",
		)

		registryStartTime := time.Now()
		if err := r.RetryService.RetryRegistryOperation(ctx, func() error {
			return r.RegistryService.EnrichMCPServer(ctx, mcpServer, mcpServer.Spec.Registry.Name)
		}); err != nil {
			logger.Error(err, "Failed to enrich MCPServer with registry data",
				"server_name", mcpServer.Spec.Registry.Name,
				"phase", "registry_enrichment",
				"duration", time.Since(registryStartTime),
				"error_type", "registry_enrich_error",
			)
			metrics.RecordRegistryOperation(mcpServer.Namespace, mcpServer.Spec.Registry.Name, "error", time.Since(registryStartTime).Seconds())
			r.StatusService.SetCondition(mcpServer, mcpv1.MCPServerConditionRegistryFetched,
				string(metav1.ConditionFalse), "RegistryEnrichmentFailed", fmt.Sprintf("Failed to enrich from registry: %v", err))
			r.EventService.RecordWarning(mcpServer, "RegistryEnrichmentFailed", fmt.Sprintf("Failed to enrich from registry: %v", err))

			if statusErr := r.StatusService.UpdateMCPServerStatus(ctx, mcpServer); statusErr != nil {
				logger.Error(statusErr, "Failed to update MCPServer status")
			}
			return ctrl.Result{RequeueAfter: time.Minute * 2}, nil
		}

		logger.Info("Successfully enriched server from registry",
			"server_name", mcpServer.Spec.Registry.Name,
			"phase", "registry_enrichment",
			"duration", time.Since(registryStartTime),
		)
		metrics.RecordRegistryOperation(mcpServer.Namespace, mcpServer.Spec.Registry.Name, "success", time.Since(registryStartTime).Seconds())

		// Update MCPServer with enriched data
		if err := r.Update(ctx, mcpServer); err != nil {
			logger.Error(err, "Failed to update MCPServer with enriched data")
			return ctrl.Result{}, err
		}

		// Invalidate cache after update
		if r.CacheService != nil {
			cacheKey := fmt.Sprintf("mcpserver:%s:%s", mcpServer.Namespace, mcpServer.Name)
			r.CacheService.InvalidateMCPServer(ctx, cacheKey)
			// Also invalidate registry servers cache since server was updated
			if mcpServer.Spec.Registry.Name != "" {
				r.CacheService.InvalidateRegistryServers(ctx, mcpServer.Spec.Registry.Name)
			}
		}

		r.StatusService.SetCondition(mcpServer, mcpv1.MCPServerConditionRegistryFetched,
			string(metav1.ConditionTrue), "RegistryEnrichmentSuccessful", "Successfully enriched MCPServer with registry data")
		r.EventService.RecordNormal(mcpServer, "RegistryEnrichmentSuccessful", "Successfully enriched MCPServer with registry data")
	}

	// Set MCPServer phase to pending
	mcpServer.Status.Phase = mcpv1.MCPServerPhasePending
	r.StatusService.SetCondition(mcpServer, mcpv1.MCPServerConditionProgressing,
		string(metav1.ConditionTrue), "DeploymentInProgress", "MCPServer deployment in progress")

	// Create or update deployment with retry
	if err := r.RetryService.RetryDeploymentOperation(ctx, func() error {
		_, err := r.DeploymentService.CreateOrUpdateDeployment(ctx, mcpServer)
		return err
	}); err != nil {
		logger.Error(err, "Failed to create or update deployment")
		mcpServer.Status.Phase = mcpv1.MCPServerPhaseFailed
		r.StatusService.SetCondition(mcpServer, mcpv1.MCPServerConditionReady,
			string(metav1.ConditionFalse), "DeploymentFailed", fmt.Sprintf("Failed to create deployment: %v", err))
		r.EventService.RecordWarning(mcpServer, "DeploymentFailed", fmt.Sprintf("Failed to create deployment: %v", err))

		if statusErr := r.StatusService.UpdateMCPServerStatus(ctx, mcpServer); statusErr != nil {
			logger.Error(statusErr, "Failed to update MCPServer status")
		}
		return ctrl.Result{RequeueAfter: time.Minute * 2}, nil
	}

	// Create or update service with retry
	if err := r.RetryService.RetryDeploymentOperation(ctx, func() error {
		_, err := r.DeploymentService.CreateOrUpdateService(ctx, mcpServer)
		return err
	}); err != nil {
		logger.Error(err, "Failed to create or update service")
		r.StatusService.SetCondition(mcpServer, mcpv1.MCPServerConditionReady,
			string(metav1.ConditionFalse), "ServiceFailed", fmt.Sprintf("Failed to create service: %v", err))
		r.EventService.RecordWarning(mcpServer, "ServiceFailed", fmt.Sprintf("Failed to create service: %v", err))

		if statusErr := r.StatusService.UpdateMCPServerStatus(ctx, mcpServer); statusErr != nil {
			logger.Error(statusErr, "Failed to update MCPServer status")
		}
		return ctrl.Result{RequeueAfter: time.Minute * 2}, nil
	}

	// Create or update HPA if autoscaling is enabled
	if mcpServer.Spec.Autoscaling != nil && mcpServer.Spec.Autoscaling.HPA != nil {
		if err := r.RetryService.RetryDeploymentOperation(ctx, func() error {
			_, err := r.DeploymentService.CreateOrUpdateHPA(ctx, mcpServer)
			return err
		}); err != nil {
			logger.Error(err, "Failed to create or update HPA")
			r.EventService.RecordWarning(mcpServer, "HPAFailed", fmt.Sprintf("Failed to create HPA: %v", err))
			// Don't fail the entire reconciliation for HPA issues
		}
	}

	// Create or update VPA if autoscaling is enabled
	if mcpServer.Spec.Autoscaling != nil && mcpServer.Spec.Autoscaling.VPA != nil {
		if err := r.RetryService.RetryDeploymentOperation(ctx, func() error {
			_, err := r.DeploymentService.CreateOrUpdateVPA(ctx, mcpServer)
			return err
		}); err != nil {
			logger.Error(err, "Failed to create or update VPA")
			r.EventService.RecordWarning(mcpServer, "VPAFailed", fmt.Sprintf("Failed to create VPA: %v", err))
			// Don't fail the entire reconciliation for VPA issues
		}
	}

	// Create or update network policy if multi-tenancy is enabled
	if mcpServer.Spec.Tenancy != nil && mcpServer.Spec.Tenancy.NetworkPolicy != nil {
		if err := r.RetryService.RetryDeploymentOperation(ctx, func() error {
			_, err := r.DeploymentService.CreateOrUpdateNetworkPolicy(ctx, mcpServer)
			return err
		}); err != nil {
			logger.Error(err, "Failed to create or update network policy")
			r.EventService.RecordWarning(mcpServer, "NetworkPolicyFailed", fmt.Sprintf("Failed to create network policy: %v", err))
			// Don't fail the entire reconciliation for network policy issues
		}
	}

	// Update MCPServer status based on deployment status
	if err := r.StatusService.UpdateMCPServerStatus(ctx, mcpServer); err != nil {
		logger.Error(err, "Failed to update MCPServer status")
		return ctrl.Result{}, err
	}

	// Check deployment status and update conditions
	deployment, err := r.DeploymentService.GetDeploymentStatus(ctx, mcpServer)
	if err != nil {
		logger.Error(err, "Failed to get deployment status")
		return ctrl.Result{RequeueAfter: time.Second * 30}, nil
	}

	// Update status based on deployment readiness
	if deployment.Status.ReadyReplicas > 0 {
		mcpServer.Status.Phase = mcpv1.MCPServerPhaseRunning
		r.StatusService.SetCondition(mcpServer, mcpv1.MCPServerConditionReady,
			string(metav1.ConditionTrue), "DeploymentReady", "MCPServer deployment is ready")
		r.StatusService.SetCondition(mcpServer, mcpv1.MCPServerConditionProgressing,
			string(metav1.ConditionFalse), "DeploymentComplete", "MCPServer deployment completed successfully")
		r.EventService.RecordNormal(mcpServer, "DeploymentReady", "MCPServer deployment is ready and running")
	} else if deployment.Status.Replicas == 0 {
		mcpServer.Status.Phase = mcpv1.MCPServerPhasePending
		r.StatusService.SetCondition(mcpServer, mcpv1.MCPServerConditionProgressing,
			string(metav1.ConditionTrue), "DeploymentScaling", "Waiting for deployment to scale up")
	}

	// Final status update
	if err := r.StatusService.UpdateMCPServerStatus(ctx, mcpServer); err != nil {
		logger.Error(err, "Failed to update final MCPServer status")
		return ctrl.Result{}, err
	}

	logger.Info("MCPServer reconciliation completed successfully",
		"phase", mcpServer.Status.Phase,
		"replicas", mcpServer.Status.Replicas,
		"readyReplicas", mcpServer.Status.ReadyReplicas,
		"duration", time.Since(startTime))

	// Record successful reconciliation
	metrics.RecordReconcileSuccess(req.Namespace, time.Since(startTime).Seconds(), string(mcpServer.Status.Phase))

	// Requeue after a reasonable interval to check status
	return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
}

// handleDeletion handles the deletion of MCPServer
func (r *MCPServerReconciler) handleDeletion(ctx context.Context, logger logr.Logger, mcpServer *mcpv1.MCPServer) (ctrl.Result, error) {
	logger.Info("Handling MCPServer deletion")

	// Invalidate cache entries before deletion
	if r.CacheService != nil {
		cacheKey := fmt.Sprintf("mcpserver:%s:%s", mcpServer.Namespace, mcpServer.Name)
		r.CacheService.InvalidateMCPServer(ctx, cacheKey)
		// Also invalidate registry servers cache since server is being deleted
		if mcpServer.Spec.Registry.Name != "" {
			r.CacheService.InvalidateRegistryServers(ctx, mcpServer.Spec.Registry.Name)
		}
		logger.V(1).Info("Invalidated cache entries for deleted MCPServer", "cacheKey", cacheKey)
	}

	// Set phase to terminating
	mcpServer.Status.Phase = mcpv1.MCPServerPhaseTerminating
	r.StatusService.SetCondition(mcpServer, mcpv1.MCPServerConditionProgressing,
		string(metav1.ConditionTrue), "DeletionInProgress", "MCPServer deletion in progress")

	// Delete all associated resources
	if err := r.DeploymentService.DeleteResources(ctx, mcpServer); err != nil {
		logger.Error(err, "Failed to delete MCPServer resources")
		r.EventService.RecordWarning(mcpServer, "DeletionFailed", fmt.Sprintf("Failed to delete resources: %v", err))
		return ctrl.Result{RequeueAfter: time.Second * 30}, nil
	}

	// Remove finalizer to allow deletion
	controllerutil.RemoveFinalizer(mcpServer, MCPServerFinalizer)
	if err := r.Update(ctx, mcpServer); err != nil {
		logger.Error(err, "Failed to remove finalizer")
		return ctrl.Result{}, err
	}

	r.EventService.RecordNormal(mcpServer, "Deleted", "MCPServer and all associated resources deleted successfully")

	logger.Info("MCPServer deletion completed")
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MCPServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mcpv1.MCPServer{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 5, // Higher concurrency for deployment operations
		}).
		Complete(r)
}

// GetMCPServerByNameIndexed retrieves an MCPServer by name using field indexer for efficient lookup with caching
func (r *MCPServerReconciler) GetMCPServerByNameIndexed(ctx context.Context, name, namespace string) (*mcpv1.MCPServer, error) {
	// Create cache key
	cacheKey := fmt.Sprintf("mcpserver:%s:%s", namespace, name)

	// Try to get from cache first
	if r.CacheService != nil {
		if cachedServer, found := r.CacheService.GetMCPServer(ctx, cacheKey); found {
			return cachedServer, nil
		}
	}

	mcpServerList := &mcpv1.MCPServerList{}
	listOpts := []client.ListOption{
		client.MatchingFields{"metadata.name": name},
	}

	if namespace != "" {
		listOpts = append(listOpts, client.InNamespace(namespace))
	}

	if err := r.List(ctx, mcpServerList, listOpts...); err != nil {
		return nil, err
	}

	if len(mcpServerList.Items) == 0 {
		return nil, fmt.Errorf("MCPServer %s not found in namespace %s", name, namespace)
	}

	if len(mcpServerList.Items) > 1 {
		return nil, fmt.Errorf("multiple MCPServers found with name %s in namespace %s", name, namespace)
	}

	server := &mcpServerList.Items[0]

	// Cache the result for 5 minutes (shorter TTL for servers as they change more frequently)
	if r.CacheService != nil {
		r.CacheService.SetMCPServer(ctx, cacheKey, server, 5*time.Minute)
	}

	return server, nil
}

// ListMCPServersByRegistryIndexed lists all MCPServers that use a specific registry using field indexer
func (r *MCPServerReconciler) ListMCPServersByRegistryIndexed(ctx context.Context, registryName, namespace string) (*mcpv1.MCPServerList, error) {
	mcpServerList := &mcpv1.MCPServerList{}
	listOpts := []client.ListOption{
		client.MatchingFields{"spec.registry.name": registryName},
	}

	if namespace != "" {
		listOpts = append(listOpts, client.InNamespace(namespace))
	}

	if err := r.List(ctx, mcpServerList, listOpts...); err != nil {
		return nil, err
	}

	return mcpServerList, nil
}

// ListMCPServersEfficient lists MCPServers with optional filtering using indexed fields
func (r *MCPServerReconciler) ListMCPServersEfficient(ctx context.Context, namespace string, registryName string) (*mcpv1.MCPServerList, error) {
	mcpServerList := &mcpv1.MCPServerList{}
	listOpts := []client.ListOption{}

	// Use indexed field if registry name is specified
	if registryName != "" {
		listOpts = append(listOpts, client.MatchingFields{"spec.registry.name": registryName})
	}

	if namespace != "" {
		listOpts = append(listOpts, client.InNamespace(namespace))
	}

	if err := r.List(ctx, mcpServerList, listOpts...); err != nil {
		return nil, err
	}

	return mcpServerList, nil
}
