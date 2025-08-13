package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	mcpv1 "github.com/FantasyNitroGEN/mcp_operator/api/v1"
	"github.com/FantasyNitroGEN/mcp_operator/pkg/metrics"
	"github.com/FantasyNitroGEN/mcp_operator/pkg/render"
	"github.com/FantasyNitroGEN/mcp_operator/pkg/services"
)

const (
	// MCPServerFinalizer is the finalizer used for MCPServer resources
	MCPServerFinalizer = "mcp.allbeone.io/finalizer"

	// DeploymentTimeout is the maximum time to wait for a deployment to become ready
	// before marking it as failed (watchdog timeout)
	DeploymentTimeout = 10 * time.Minute
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
	RendererService   render.RendererService
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
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=networking.istio.io,resources=virtualservices,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=networking.istio.io,resources=destinationrules,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch

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
		"registry_name", mcpServer.Spec.Registry.RegistryName,
		"server_name", mcpServer.Spec.Registry.ServerName,
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
		mcpServer.Spec.Registry.RegistryName,
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

	// Enrich MCPServer with registry data from cache if registry is specified (before validation)
	if mcpServer.Spec.Registry.RegistryName != "" && mcpServer.Spec.Registry.ServerName != "" {
		logger.Info("Enriching server from registry cache",
			"registry_name", mcpServer.Spec.Registry.RegistryName,
			"server_name", mcpServer.Spec.Registry.ServerName,
			"phase", "registry_cache_enrichment",
		)

		registryStartTime := time.Now()
		if err := r.RegistryService.EnrichMCPServerFromCache(ctx, mcpServer, req.Namespace); err != nil {
			logger.Error(err, "Failed to enrich MCPServer from registry cache",
				"registry_name", mcpServer.Spec.Registry.RegistryName,
				"server_name", mcpServer.Spec.Registry.ServerName,
				"phase", "registry_cache_enrichment",
				"duration", time.Since(registryStartTime),
				"error_type", "registry_cache_enrich_error",
			)
			metrics.RecordRegistryOperation(mcpServer.Namespace, mcpServer.Spec.Registry.RegistryName, "error", time.Since(registryStartTime).Seconds())
			r.StatusService.SetCondition(mcpServer, mcpv1.MCPServerConditionRegistryFetched,
				string(metav1.ConditionFalse), "RegistryMissing", fmt.Sprintf("Registry cache missing: %v", err))
			r.EventService.RecordWarning(mcpServer, "RegistryMissing", fmt.Sprintf("Registry cache missing: %v", err))

			// Update status once and return without requeue - cache errors will be retried when ConfigMap is created
			if statusErr := r.StatusService.UpdateMCPServerStatus(ctx, mcpServer); statusErr != nil {
				logger.Error(statusErr, "Failed to update MCPServer status")
			}
			return ctrl.Result{}, nil
		}

		logger.Info("Successfully enriched server from registry cache",
			"registry_name", mcpServer.Spec.Registry.RegistryName,
			"server_name", mcpServer.Spec.Registry.ServerName,
			"phase", "registry_cache_enrichment",
			"duration", time.Since(registryStartTime),
		)
		metrics.RecordRegistryOperation(mcpServer.Namespace, mcpServer.Spec.Registry.RegistryName, "success", time.Since(registryStartTime).Seconds())
		r.StatusService.SetCondition(mcpServer, mcpv1.MCPServerConditionRegistryFetched,
			string(metav1.ConditionTrue), "RegistryFetched", "Successfully enriched from registry cache")
		r.EventService.RecordNormal(mcpServer, "RegistryFetched", "Successfully enriched from registry cache")

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

	// Validate MCPServer specification
	if err := r.ValidationService.ValidateMCPServer(ctx, mcpServer); err != nil {
		logger.Error(err, "MCPServer validation failed")
		r.StatusService.SetCondition(mcpServer, mcpv1.MCPServerConditionReady,
			string(metav1.ConditionFalse), "ValidationFailed", fmt.Sprintf("MCPServer validation failed: %v", err))
		r.EventService.RecordWarning(mcpServer, "ValidationFailed", fmt.Sprintf("MCPServer validation failed: %v", err))

		// Update status once at the end and return without requeue - validation errors are spec issues
		if statusErr := r.StatusService.UpdateMCPServerStatus(ctx, mcpServer); statusErr != nil {
			logger.Error(statusErr, "Failed to update MCPServer status")
		}
		return ctrl.Result{}, nil
	}

	// Validate tenant resource quotas if multi-tenancy is enabled
	if mcpServer.Spec.Tenancy != nil {
		if err := r.ValidationService.ValidateTenantResourceQuotas(ctx, mcpServer); err != nil {
			logger.Error(err, "Tenant resource quota validation failed")
			r.StatusService.SetCondition(mcpServer, mcpv1.MCPServerConditionReady,
				string(metav1.ConditionFalse), "TenantValidationFailed", fmt.Sprintf("Tenant validation failed: %v", err))
			r.EventService.RecordWarning(mcpServer, "TenantValidationFailed", fmt.Sprintf("Tenant validation failed: %v", err))

			// Update status once and return without requeue - tenant validation errors are spec issues
			if statusErr := r.StatusService.UpdateMCPServerStatus(ctx, mcpServer); statusErr != nil {
				logger.Error(statusErr, "Failed to update MCPServer status")
			}
			return ctrl.Result{}, nil
		}
	}

	// Render resources to YAML and create ConfigMap
	logger.Info("Rendering resources to YAML")
	renderStartTime := time.Now()

	resources, yamlContent, err := r.RendererService.RenderResources(mcpServer)
	if err != nil {
		logger.Error(err, "Failed to render resources to YAML")
		r.StatusService.SetCondition(mcpServer, mcpv1.MCPServerConditionRendered,
			string(metav1.ConditionFalse), "RenderingFailed", fmt.Sprintf("Failed to render resources: %v", err))
		r.EventService.RecordWarning(mcpServer, "RenderingFailed", fmt.Sprintf("Failed to render resources: %v", err))

		// Update status and return without requeue - rendering errors are likely spec issues
		if statusErr := r.StatusService.UpdateMCPServerStatus(ctx, mcpServer); statusErr != nil {
			logger.Error(statusErr, "Failed to update MCPServer status")
		}
		return ctrl.Result{}, nil
	}

	// Calculate hash of rendered content
	renderedHash := r.RendererService.CalculateHash(yamlContent)

	// Create or update ConfigMap with rendered manifests
	configMapName := fmt.Sprintf("mcpserver-%s-rendered", mcpServer.Name)
	// Truncate to 63 characters if needed (Kubernetes name limit)
	if len(configMapName) > 63 {
		configMapName = configMapName[:63]
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: mcpServer.Namespace,
			Labels: map[string]string{
				"mcp.allbeone.io/name":      mcpServer.Name,
				"mcp.allbeone.io/namespace": mcpServer.Namespace,
				"mcp.allbeone.io/component": "rendered-manifests",
			},
		},
		Data: map[string]string{
			"manifests.yaml": yamlContent,
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(mcpServer, configMap, r.Scheme); err != nil {
		logger.Error(err, "Failed to set controller reference for ConfigMap")
		r.StatusService.SetCondition(mcpServer, mcpv1.MCPServerConditionRendered,
			string(metav1.ConditionFalse), "ConfigMapOwnerRefFailed", fmt.Sprintf("Failed to set owner reference: %v", err))
		r.EventService.RecordWarning(mcpServer, "ConfigMapOwnerRefFailed", fmt.Sprintf("Failed to set owner reference: %v", err))

		if statusErr := r.StatusService.UpdateMCPServerStatus(ctx, mcpServer); statusErr != nil {
			logger.Error(statusErr, "Failed to update MCPServer status")
		}
		return ctrl.Result{}, nil
	}

	// Create or update the ConfigMap
	if err := r.RetryService.RetryDeploymentOperation(ctx, func() error {
		_, err := controllerutil.CreateOrUpdate(ctx, r.Client, configMap, func() error {
			// Update data on each reconciliation
			configMap.Data = map[string]string{
				"manifests.yaml": yamlContent,
			}
			return nil
		})
		return err
	}); err != nil {
		logger.Error(err, "Failed to create or update rendered ConfigMap")
		r.StatusService.SetCondition(mcpServer, mcpv1.MCPServerConditionRendered,
			string(metav1.ConditionFalse), "ConfigMapCreateFailed", fmt.Sprintf("Failed to create ConfigMap: %v", err))
		r.EventService.RecordWarning(mcpServer, "ConfigMapCreateFailed", fmt.Sprintf("Failed to create ConfigMap: %v", err))

		if statusErr := r.StatusService.UpdateMCPServerStatus(ctx, mcpServer); statusErr != nil {
			logger.Error(statusErr, "Failed to update MCPServer status")
		}
		return ctrl.Result{}, nil
	}

	// Update MCPServer status with rendered information
	mcpServer.Status.ResolvedHash = renderedHash
	mcpServer.Status.ResolvedConfigMap = &corev1.LocalObjectReference{
		Name: configMapName,
	}

	// Set successful rendering condition
	resourceCount := len(resources)
	r.StatusService.SetCondition(mcpServer, mcpv1.MCPServerConditionRendered,
		string(metav1.ConditionTrue), "Assembled", fmt.Sprintf("resources=%d", resourceCount))
	r.EventService.RecordNormal(mcpServer, "Rendered", fmt.Sprintf("Successfully rendered %d resources to ConfigMap %s", resourceCount, configMapName))

	logger.Info("Successfully rendered resources and created ConfigMap",
		"configMapName", configMapName,
		"resourceCount", resourceCount,
		"hash", renderedHash,
		"duration", time.Since(renderStartTime),
	)

	// Set MCPServer phase to pending
	mcpServer.Status.Phase = mcpv1.MCPServerPhasePending
	r.StatusService.SetCondition(mcpServer, mcpv1.MCPServerConditionProgressing,
		string(metav1.ConditionTrue), "DeploymentInProgress", "MCPServer deployment in progress")

	// Apply rendered resources using Server-Side Apply
	logger.Info("Applying rendered resources using Server-Side Apply")
	applyStartTime := time.Now()

	if err := r.RetryService.RetryDeploymentOperation(ctx, func() error {
		return r.applyRenderedResources(ctx, logger, mcpServer, resources, configMapName, renderedHash)
	}); err != nil {
		logger.Error(err, "Failed to apply rendered resources")
		mcpServer.Status.Phase = mcpv1.MCPServerPhaseFailed
		r.StatusService.SetCondition(mcpServer, mcpv1.MCPServerConditionApplied,
			string(metav1.ConditionFalse), "ApplyFailed", fmt.Sprintf("Failed to apply resources: %v", err))
		r.EventService.RecordWarning(mcpServer, "ApplyFailed", fmt.Sprintf("Failed to apply resources: %v", err))

		// Update status once and return without requeue - apply errors will be retried on next event
		if statusErr := r.StatusService.UpdateMCPServerStatus(ctx, mcpServer); statusErr != nil {
			logger.Error(statusErr, "Failed to update MCPServer status")
		}
		return ctrl.Result{}, nil
	}

	// Set successful apply condition
	r.StatusService.SetCondition(mcpServer, mcpv1.MCPServerConditionApplied,
		string(metav1.ConditionTrue), "Applied", fmt.Sprintf("Successfully applied %d resources", len(resources)))
	r.EventService.RecordNormal(mcpServer, "Applied", fmt.Sprintf("Successfully applied %d resources using Server-Side Apply", len(resources)))

	logger.Info("Successfully applied rendered resources",
		"resourceCount", len(resources),
		"duration", time.Since(applyStartTime),
	)

	// Check deployment status and update conditions with enhanced rolling update tracking
	deployment, err := r.DeploymentService.GetDeploymentStatus(ctx, mcpServer)
	if err != nil {
		logger.Error(err, "Failed to get deployment status")
		// Don't requeue - rely on event-driven updates from deployment changes
		// Set condition to indicate deployment status check failed
		r.StatusService.SetCondition(mcpServer, mcpv1.MCPServerConditionReady,
			string(metav1.ConditionFalse), "DeploymentStatusCheckFailed", fmt.Sprintf("Failed to get deployment status: %v", err))
	} else {
		// Update MCPServer status with deployment replica counts
		mcpServer.Status.Replicas = deployment.Status.Replicas
		mcpServer.Status.ReadyReplicas = deployment.Status.ReadyReplicas
		mcpServer.Status.AvailableReplicas = deployment.Status.AvailableReplicas

		// Check if deployment is in rolling update state
		isRollingUpdate := r.isDeploymentRollingUpdate(deployment)
		desiredReplicas := int32(1)
		if deployment.Spec.Replicas != nil {
			desiredReplicas = *deployment.Spec.Replicas
		}

		logger.V(1).Info("Deployment status analysis",
			"desired", desiredReplicas,
			"replicas", deployment.Status.Replicas,
			"readyReplicas", deployment.Status.ReadyReplicas,
			"availableReplicas", deployment.Status.AvailableReplicas,
			"updatedReplicas", deployment.Status.UpdatedReplicas,
			"isRollingUpdate", isRollingUpdate,
		)

		// Update status based on deployment state
		if isRollingUpdate {
			// Deployment is in rolling update state
			mcpServer.Status.Phase = mcpv1.MCPServerPhaseRunning // Keep running during updates
			r.StatusService.SetCondition(mcpServer, mcpv1.MCPServerConditionProgressing,
				string(metav1.ConditionTrue), "RollingUpdate", "MCPServer deployment is performing rolling update")
			r.StatusService.SetCondition(mcpServer, mcpv1.MCPServerConditionReady,
				string(metav1.ConditionFalse), "RollingUpdate", "MCPServer deployment is updating")
			r.EventService.RecordNormal(mcpServer, "RollingUpdate", "MCPServer deployment is performing rolling update")
		} else if deployment.Status.ReadyReplicas == desiredReplicas && deployment.Status.ReadyReplicas > 0 {
			// All replicas are ready and deployment is stable
			mcpServer.Status.Phase = mcpv1.MCPServerPhaseRunning
			r.StatusService.SetCondition(mcpServer, mcpv1.MCPServerConditionReady,
				string(metav1.ConditionTrue), "DeploymentReady", fmt.Sprintf("ReadyReplicas=%d/%d", deployment.Status.ReadyReplicas, desiredReplicas))
			r.StatusService.SetCondition(mcpServer, mcpv1.MCPServerConditionProgressing,
				string(metav1.ConditionFalse), "DeploymentComplete", "MCPServer deployment completed successfully")
			r.EventService.RecordNormal(mcpServer, "DeploymentReady", "MCPServer deployment is ready and running")
		} else if deployment.Status.Replicas == 0 {
			// No replicas yet - deployment is scaling up
			mcpServer.Status.Phase = mcpv1.MCPServerPhasePending
			r.StatusService.SetCondition(mcpServer, mcpv1.MCPServerConditionProgressing,
				string(metav1.ConditionTrue), "DeploymentScaling", "Waiting for deployment to scale up")
			r.StatusService.SetCondition(mcpServer, mcpv1.MCPServerConditionReady,
				string(metav1.ConditionFalse), "DeploymentScaling", "Deployment is scaling up")
		} else if deployment.Status.ReadyReplicas > 0 && deployment.Status.ReadyReplicas < desiredReplicas {
			// Some replicas are ready but not all - could be scaling or recovering
			mcpServer.Status.Phase = mcpv1.MCPServerPhaseRunning
			r.StatusService.SetCondition(mcpServer, mcpv1.MCPServerConditionProgressing,
				string(metav1.ConditionTrue), "DeploymentPartiallyReady", fmt.Sprintf("Deployment has %d/%d ready replicas", deployment.Status.ReadyReplicas, desiredReplicas))
			r.StatusService.SetCondition(mcpServer, mcpv1.MCPServerConditionReady,
				string(metav1.ConditionFalse), "DeploymentPartiallyReady", fmt.Sprintf("Only %d/%d replicas are ready", deployment.Status.ReadyReplicas, desiredReplicas))
		} else {
			// Fallback case - deployment exists but status is unclear
			mcpServer.Status.Phase = mcpv1.MCPServerPhasePending
			r.StatusService.SetCondition(mcpServer, mcpv1.MCPServerConditionProgressing,
				string(metav1.ConditionTrue), "DeploymentPending", "Deployment status is pending")
			r.StatusService.SetCondition(mcpServer, mcpv1.MCPServerConditionReady,
				string(metav1.ConditionFalse), "DeploymentPending", "Deployment is not ready yet")
		}
	}

	// Watchdog: Check for deployment timeout
	if r.StatusService.CheckDeploymentTimeout(mcpServer, DeploymentTimeout) {
		logger.Info("Deployment timeout detected, marking MCPServer as failed",
			"timeout", DeploymentTimeout,
			"phase", mcpServer.Status.Phase,
		)

		// Mark as timed out
		r.StatusService.MarkDeploymentAsTimedOut(mcpServer, DeploymentTimeout)

		// Record warning event
		r.EventService.RecordWarning(mcpServer, "DeploymentTimeout",
			fmt.Sprintf("Deployment has been stuck for %v, exceeding timeout", DeploymentTimeout))

		// Update metrics for failed reconciliation
		metrics.RecordReconcileError(req.Namespace, "deployment_timeout", time.Since(startTime).Seconds())
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

	// No requeue needed - rely on event-driven updates from owned resources
	return ctrl.Result{}, nil
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
		// Don't requeue - rely on event-driven updates from owned resource deletions
		return ctrl.Result{}, err
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
	return r.SetupWithManagerAndConcurrency(mgr, 5)
}

// SetupWithManagerAndConcurrency sets up the controller with the Manager and configurable concurrency.
func (r *MCPServerReconciler) SetupWithManagerAndConcurrency(mgr ctrl.Manager, maxConcurrentReconciles int) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mcpv1.MCPServer{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Secret{}).
		Owns(&autoscalingv2.HorizontalPodAutoscaler{}).
		Owns(&networkingv1.NetworkPolicy{}).
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(r.findMCPServersForConfigMap),
		).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: maxConcurrentReconciles,
		}).
		Complete(r)
}

// findMCPServersForConfigMap returns reconcile requests for MCPServers that use a specific registry ConfigMap
func (r *MCPServerReconciler) findMCPServersForConfigMap(ctx context.Context, configMap client.Object) []reconcile.Request {
	var requests []reconcile.Request

	// Check if this is a registry ConfigMap by its labels
	labels := configMap.GetLabels()
	if labels == nil {
		return requests
	}

	registryName, hasRegistryLabel := labels["mcp.allbeone.io/registry"]
	serverName, hasServerLabel := labels["mcp.allbeone.io/server"]

	if !hasRegistryLabel || !hasServerLabel {
		return requests
	}

	// Also check if the ConfigMap name matches the expected pattern
	expectedName := fmt.Sprintf("mcpregistry-%s-%s", registryName, serverName)
	if configMap.GetName() != expectedName {
		return requests
	}

	// Find all MCPServers that use this registry and server
	mcpServerList := &mcpv1.MCPServerList{}

	if err := r.List(ctx, mcpServerList, client.InNamespace(configMap.GetNamespace())); err != nil {
		// Log error but continue
		log.FromContext(ctx).Error(err, "Failed to list MCPServers for ConfigMap watch",
			"configMap", configMap.GetName(),
			"namespace", configMap.GetNamespace())
		return requests
	}

	for _, mcpServer := range mcpServerList.Items {
		if mcpServer.Spec.Registry.RegistryName == registryName &&
			mcpServer.Spec.Registry.ServerName == serverName {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      mcpServer.Name,
					Namespace: mcpServer.Namespace,
				},
			})
		}
	}

	return requests
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

// isDeploymentRollingUpdate determines if a deployment is currently performing a rolling update
// applyRenderedResources applies all rendered resources using Server-Side Apply
func (r *MCPServerReconciler) applyRenderedResources(ctx context.Context, logger logr.Logger, mcpServer *mcpv1.MCPServer, resources []client.Object, configMapName, renderedHash string) error {
	fieldManager := "mcp-operator"

	for _, resource := range resources {
		// Add required labels
		labels := resource.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		labels["mcp.allbeone.io/name"] = mcpServer.Name
		labels["mcp.allbeone.io/registry"] = mcpServer.Spec.Registry.RegistryName
		labels["mcp.allbeone.io/server"] = mcpServer.Spec.Registry.ServerName
		labels["mcp.allbeone.io/hash"] = renderedHash
		resource.SetLabels(labels)

		// Add required annotation
		annotations := resource.GetAnnotations()
		if annotations == nil {
			annotations = make(map[string]string)
		}
		annotations["mcp.allbeone.io/rendered-cm"] = configMapName
		resource.SetAnnotations(annotations)

		// Set owner reference for proper cleanup
		if err := controllerutil.SetControllerReference(mcpServer, resource, r.Scheme); err != nil {
			logger.Error(err, "Failed to set controller reference for resource",
				"resource", resource.GetObjectKind().GroupVersionKind().String(),
				"name", resource.GetName(),
			)
			return fmt.Errorf("failed to set controller reference for %s/%s: %w",
				resource.GetObjectKind().GroupVersionKind().Kind, resource.GetName(), err)
		}

		// Apply the resource using Server-Side Apply
		if err := r.Patch(ctx, resource, client.Apply, client.ForceOwnership, client.FieldOwner(fieldManager)); err != nil {
			logger.Error(err, "Failed to apply resource using Server-Side Apply",
				"resource", resource.GetObjectKind().GroupVersionKind().String(),
				"name", resource.GetName(),
				"namespace", resource.GetNamespace(),
			)
			return fmt.Errorf("failed to apply %s/%s: %w",
				resource.GetObjectKind().GroupVersionKind().Kind, resource.GetName(), err)
		}

		logger.V(1).Info("Successfully applied resource using Server-Side Apply",
			"resource", resource.GetObjectKind().GroupVersionKind().String(),
			"name", resource.GetName(),
			"namespace", resource.GetNamespace(),
		)
	}

	return nil
}

func (r *MCPServerReconciler) isDeploymentRollingUpdate(deployment *appsv1.Deployment) bool {
	if deployment == nil {
		return false
	}

	desiredReplicas := int32(1)
	if deployment.Spec.Replicas != nil {
		desiredReplicas = *deployment.Spec.Replicas
	}

	// Check deployment conditions for rolling update indicators
	for _, condition := range deployment.Status.Conditions {
		if condition.Type == appsv1.DeploymentProgressing {
			// If deployment is progressing and reason indicates rolling update
			if condition.Status == corev1.ConditionTrue &&
				(condition.Reason == "ReplicaSetUpdated" || condition.Reason == "NewReplicaSetCreated") {
				return true
			}
		}
	}

	// Check replica counts to detect rolling update state
	// During rolling update, we might have:
	// - UpdatedReplicas < DesiredReplicas (old pods still exist)
	// - Replicas > DesiredReplicas (temporary surge during update)
	// - ReadyReplicas != UpdatedReplicas (some old pods still ready)

	// If we have more replicas than desired, we're likely in surge phase
	if deployment.Status.Replicas > desiredReplicas {
		return true
	}

	// If updated replicas is less than desired, update is in progress
	if deployment.Status.UpdatedReplicas < desiredReplicas && deployment.Status.UpdatedReplicas > 0 {
		return true
	}

	// If we have both old and new replicas (total replicas > ready replicas and updated replicas > 0)
	if deployment.Status.Replicas > deployment.Status.ReadyReplicas && deployment.Status.UpdatedReplicas > 0 {
		return true
	}

	// Check if deployment generation is newer than observed generation
	// This indicates a spec change that hasn't been fully processed yet
	if deployment.Generation > deployment.Status.ObservedGeneration {
		return true
	}

	return false
}
