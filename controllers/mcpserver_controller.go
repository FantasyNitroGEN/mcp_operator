package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/uuid"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	mcpv1 "github.com/FantasyNitroGEN/mcp_operator/api/v1"
	"github.com/FantasyNitroGEN/mcp_operator/pkg/metrics"
	"github.com/FantasyNitroGEN/mcp_operator/pkg/registry"
	"github.com/FantasyNitroGEN/mcp_operator/pkg/sync"
)

const (
	// MCPServerFinalizer is the finalizer used for MCPServer resources
	MCPServerFinalizer = "mcp.allbeone.io/finalizer"
)

// MCPServerReconciler reconciles a MCPServer object
type MCPServerReconciler struct {
	client.Client
	Scheme         *runtime.Scheme
	RegistryClient *registry.Client
	Syncer         *sync.Syncer
}

//+kubebuilder:rbac:groups=mcp.allbeone.io,resources=mcpservers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=mcp.allbeone.io,resources=mcpservers/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=mcp.allbeone.io,resources=mcpservers/finalizers,verbs=update
//+kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=services,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=core,resources=configmaps,verbs=get;list;watch
//+kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// Note: Secrets access is conditionally granted in Helm template based on backup.enabled value
//+kubebuilder:rbac:groups=autoscaling,resources=horizontalpodautoscalers,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=networking.k8s.io,resources=networkpolicies,verbs=get;list;watch;create;update;patch;delete

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
	logger.Info("Starting reconciliation",
		"namespace", req.Namespace,
		"name", req.Name,
	)

	// Record reconciliation start
	metrics.RecordReconcileStart(req.Namespace)

	// Fetch the MCPServer instance
	mcpServer := &mcpv1.MCPServer{}
	err := r.Get(ctx, req.NamespacedName, mcpServer)
	if err != nil {
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

	// Handle deletion
	if mcpServer.GetDeletionTimestamp() != nil {
		logger.Info("MCPServer is being deleted", "phase", "deletion")
		return r.handleDeletion(ctx, logger, mcpServer)
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(mcpServer, MCPServerFinalizer) {
		logger.Info("Adding finalizer to MCPServer", "phase", "finalizer_addition")
		controllerutil.AddFinalizer(mcpServer, MCPServerFinalizer)

		err = r.Update(ctx, mcpServer)
		if err != nil {
			logger.Error(err, "Failed to add finalizer",
				"phase", "finalizer_addition",
				"error_type", "finalizer_update_error",
			)
			return ctrl.Result{}, err
		}

		logger.Info("Finalizer added successfully", "phase", "finalizer_addition")
		return ctrl.Result{RequeueAfter: time.Second * 1}, nil
	}

	// Auto-enrich server from registry if server name is specified
	if mcpServer.Spec.Registry.Name != "" {
		logger.Info("Enriching server from registry",
			"server_name", mcpServer.Spec.Registry.Name,
			"phase", "registry_enrichment",
		)

		registryStartTime := time.Now()
		if err := r.enrichFromRegistry(ctx, mcpServer); err != nil {
			logger.Error(err, "Failed to enrich server from registry",
				"server_name", mcpServer.Spec.Registry.Name,
				"phase", "registry_enrichment",
				"duration", time.Since(registryStartTime),
				"error_type", "registry_enrich_error",
			)
			metrics.RecordRegistryOperation(mcpServer.Namespace, mcpServer.Spec.Registry.Name, "error", time.Since(registryStartTime).Seconds())
			// Don't return error here - continue with deployment even if registry enrichment fails
			// The status condition will reflect the failure
		} else {
			logger.Info("Successfully enriched server from registry",
				"server_name", mcpServer.Spec.Registry.Name,
				"phase", "registry_enrichment",
				"duration", time.Since(registryStartTime),
			)
			metrics.RecordRegistryOperation(mcpServer.Namespace, mcpServer.Spec.Registry.Name, "success", time.Since(registryStartTime).Seconds())
		}

		// Update status after registry enrichment
		if err := r.Status().Update(ctx, mcpServer); err != nil {
			logger.Error(err, "Failed to update MCPServer status after registry enrichment",
				"phase", "registry_enrichment",
				"error_type", "status_update_error",
			)
		}
	}

	// Check for registry loading annotation (legacy support)
	if mcpServer.Annotations != nil {
		if registryName, exists := mcpServer.Annotations["mcp.allbeone.io/registry-server"]; exists {
			logger.Info("Registry loading requested via annotation",
				"registry_server", registryName,
				"phase", "annotation_registry_loading",
			)

			registryStartTime := time.Now()
			if err := r.loadServerFromRegistry(ctx, mcpServer, registryName); err != nil {
				logger.Error(err, "Failed to load server from registry via annotation",
					"registry_server", registryName,
					"phase", "annotation_registry_loading",
					"duration", time.Since(registryStartTime),
					"error_type", "registry_load_error",
				)
				metrics.RecordRegistryOperation(mcpServer.Namespace, registryName, "error", time.Since(registryStartTime).Seconds())
				return ctrl.Result{}, err
			}

			logger.Info("Successfully loaded server from registry via annotation",
				"registry_server", registryName,
				"phase", "annotation_registry_loading",
				"duration", time.Since(registryStartTime),
			)
			metrics.RecordRegistryOperation(mcpServer.Namespace, registryName, "success", time.Since(registryStartTime).Seconds())
		}
	}

	// Validate tenant resource quotas if tenancy is configured
	if err := r.validateTenantResourceQuotas(ctx, logger, mcpServer); err != nil {
		logger.Error(err, "Tenant resource quota validation failed",
			"phase", "quota_validation",
			"error_type", "quota_exceeded",
		)
		metrics.RecordReconcileError(mcpServer.Namespace, "quota_exceeded", time.Since(startTime).Seconds())
		return ctrl.Result{}, err
	}

	// Create network policy for tenant isolation if configured
	if err := r.createNetworkPolicyForTenant(ctx, logger, mcpServer); err != nil {
		logger.Error(err, "Failed to create network policy for tenant",
			"phase", "network_policy_creation",
			"error_type", "network_policy_error",
		)
		metrics.RecordReconcileError(mcpServer.Namespace, "network_policy_error", time.Since(startTime).Seconds())
		return ctrl.Result{}, err
	}

	// Check if the deployment already exists, if not create a new one
	logger.Info("Checking deployment existence", "phase", "deployment_reconciliation")
	deployment := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: mcpServer.Name, Namespace: mcpServer.Namespace}, deployment)
	if err != nil && errors.IsNotFound(err) {
		// Define a new deployment
		logger.Info("Deployment not found, creating new one", "phase", "deployment_creation")

		deploymentStartTime := time.Now()
		dep := r.deploymentForMCPServer(mcpServer)

		logger.Info("Creating new Deployment",
			"deployment_namespace", dep.Namespace,
			"deployment_name", dep.Name,
			"phase", "deployment_creation",
			"replicas", *dep.Spec.Replicas,
			"image", dep.Spec.Template.Spec.Containers[0].Image,
		)

		err = r.Create(ctx, dep)
		if err != nil {
			logger.Error(err, "Failed to create new Deployment",
				"deployment_namespace", dep.Namespace,
				"deployment_name", dep.Name,
				"phase", "deployment_creation",
				"duration", time.Since(deploymentStartTime),
				"error_type", "deployment_creation_error",
			)
			metrics.RecordDeploymentOperation(mcpServer.Namespace, "create", "error")
			return ctrl.Result{}, err
		}

		logger.Info("Deployment created successfully",
			"deployment_namespace", dep.Namespace,
			"deployment_name", dep.Name,
			"phase", "deployment_creation",
			"duration", time.Since(deploymentStartTime),
		)
		metrics.RecordDeploymentOperation(mcpServer.Namespace, "create", "success")

		// Deployment created successfully - return and requeue
		return ctrl.Result{RequeueAfter: time.Second * 1}, nil
	} else if err != nil {
		logger.Error(err, "Failed to get Deployment",
			"phase", "deployment_reconciliation",
			"error_type", "deployment_fetch_error",
		)
		return ctrl.Result{}, err
	}

	logger.Info("Deployment found",
		"phase", "deployment_reconciliation",
		"deployment_replicas", deployment.Status.Replicas,
		"ready_replicas", deployment.Status.ReadyReplicas,
		"available_replicas", deployment.Status.AvailableReplicas,
	)

	// Check if the service already exists, if not create a new one
	logger.Info("Checking service existence", "phase", "service_reconciliation")
	service := &corev1.Service{}
	err = r.Get(ctx, types.NamespacedName{Name: mcpServer.Name, Namespace: mcpServer.Namespace}, service)
	if err != nil && errors.IsNotFound(err) {
		// Define a new service
		logger.Info("Service not found, creating new one", "phase", "service_creation")

		serviceStartTime := time.Now()
		srv := r.serviceForMCPServer(mcpServer)

		logger.Info("Creating new Service",
			"service_namespace", srv.Namespace,
			"service_name", srv.Name,
			"phase", "service_creation",
			"service_type", srv.Spec.Type,
		)

		err = r.Create(ctx, srv)
		if err != nil {
			logger.Error(err, "Failed to create new Service",
				"service_namespace", srv.Namespace,
				"service_name", srv.Name,
				"phase", "service_creation",
				"duration", time.Since(serviceStartTime),
				"error_type", "service_creation_error",
			)
			metrics.RecordServiceOperation(mcpServer.Namespace, "create", "error")
			return ctrl.Result{}, err
		}

		logger.Info("Service created successfully",
			"service_namespace", srv.Namespace,
			"service_name", srv.Name,
			"phase", "service_creation",
			"duration", time.Since(serviceStartTime),
		)
		metrics.RecordServiceOperation(mcpServer.Namespace, "create", "success")

		// Service created successfully - return and requeue
		return ctrl.Result{RequeueAfter: time.Second * 1}, nil
	} else if err != nil {
		logger.Error(err, "Failed to get Service",
			"phase", "service_reconciliation",
			"error_type", "service_fetch_error",
		)
		return ctrl.Result{}, err
	}

	logger.Info("Service found",
		"phase", "service_reconciliation",
		"service_type", service.Spec.Type,
		"cluster_ip", service.Spec.ClusterIP,
	)

	// Check if HPA should be created/updated
	if mcpServer.Spec.Autoscaling != nil && mcpServer.Spec.Autoscaling.HPA != nil && mcpServer.Spec.Autoscaling.HPA.Enabled {
		logger.Info("Checking HPA existence", "phase", "hpa_reconciliation")
		hpa := &autoscalingv2.HorizontalPodAutoscaler{}
		err = r.Get(ctx, types.NamespacedName{Name: mcpServer.Name, Namespace: mcpServer.Namespace}, hpa)
		if err != nil && errors.IsNotFound(err) {
			// Define a new HPA
			logger.Info("HPA not found, creating new one", "phase", "hpa_creation")

			hpaStartTime := time.Now()
			newHPA := r.hpaForMCPServer(mcpServer)

			logger.Info("Creating new HPA",
				"hpa_namespace", newHPA.Namespace,
				"hpa_name", newHPA.Name,
				"phase", "hpa_creation",
				"min_replicas", *newHPA.Spec.MinReplicas,
				"max_replicas", newHPA.Spec.MaxReplicas,
			)

			err = r.Create(ctx, newHPA)
			if err != nil {
				logger.Error(err, "Failed to create new HPA",
					"hpa_namespace", newHPA.Namespace,
					"hpa_name", newHPA.Name,
					"phase", "hpa_creation",
					"duration", time.Since(hpaStartTime),
					"error_type", "hpa_creation_error",
				)
				metrics.RecordServiceOperation(mcpServer.Namespace, "hpa_create", "error")
				return ctrl.Result{}, err
			}

			logger.Info("HPA created successfully",
				"hpa_namespace", newHPA.Namespace,
				"hpa_name", newHPA.Name,
				"phase", "hpa_creation",
				"duration", time.Since(hpaStartTime),
			)
			metrics.RecordServiceOperation(mcpServer.Namespace, "hpa_create", "success")

			// HPA created successfully - return and requeue
			return ctrl.Result{RequeueAfter: time.Second * 1}, nil
		} else if err != nil {
			logger.Error(err, "Failed to get HPA",
				"phase", "hpa_reconciliation",
				"error_type", "hpa_fetch_error",
			)
			return ctrl.Result{}, err
		}

		logger.Info("HPA found",
			"phase", "hpa_reconciliation",
			"current_replicas", hpa.Status.CurrentReplicas,
			"desired_replicas", hpa.Status.DesiredReplicas,
		)
	} else {
		// Check if HPA exists and should be deleted
		hpa := &autoscalingv2.HorizontalPodAutoscaler{}
		err = r.Get(ctx, types.NamespacedName{Name: mcpServer.Name, Namespace: mcpServer.Namespace}, hpa)
		if err == nil {
			logger.Info("Deleting HPA as autoscaling is disabled", "phase", "hpa_cleanup")
			err = r.Delete(ctx, hpa)
			if err != nil {
				logger.Error(err, "Failed to delete HPA", "phase", "hpa_cleanup")
				return ctrl.Result{}, err
			}
			logger.Info("HPA deleted successfully", "phase", "hpa_cleanup")
		} else if !errors.IsNotFound(err) {
			logger.Error(err, "Failed to check HPA existence", "phase", "hpa_cleanup")
			return ctrl.Result{}, err
		}
	}

	// Check if VPA should be created/updated
	if mcpServer.Spec.Autoscaling != nil && mcpServer.Spec.Autoscaling.VPA != nil && mcpServer.Spec.Autoscaling.VPA.Enabled {
		logger.Info("Checking VPA existence", "phase", "vpa_reconciliation")
		vpa := &unstructured.Unstructured{}
		vpa.SetAPIVersion("autoscaling.k8s.io/v1")
		vpa.SetKind("VerticalPodAutoscaler")

		err = r.Get(ctx, types.NamespacedName{Name: mcpServer.Name, Namespace: mcpServer.Namespace}, vpa)
		if err != nil && errors.IsNotFound(err) {
			// Define a new VPA
			logger.Info("VPA not found, creating new one", "phase", "vpa_creation")

			vpaStartTime := time.Now()
			newVPA := r.vpaForMCPServer(mcpServer)

			logger.Info("Creating new VPA",
				"vpa_namespace", newVPA.GetNamespace(),
				"vpa_name", newVPA.GetName(),
				"phase", "vpa_creation",
			)

			err = r.Create(ctx, newVPA)
			if err != nil {
				logger.Error(err, "Failed to create new VPA",
					"vpa_namespace", newVPA.GetNamespace(),
					"vpa_name", newVPA.GetName(),
					"phase", "vpa_creation",
					"duration", time.Since(vpaStartTime),
					"error_type", "vpa_creation_error",
				)
				metrics.RecordServiceOperation(mcpServer.Namespace, "vpa_create", "error")
				return ctrl.Result{}, err
			}

			logger.Info("VPA created successfully",
				"vpa_namespace", newVPA.GetNamespace(),
				"vpa_name", newVPA.GetName(),
				"phase", "vpa_creation",
				"duration", time.Since(vpaStartTime),
			)
			metrics.RecordServiceOperation(mcpServer.Namespace, "vpa_create", "success")

			// VPA created successfully - return and requeue
			return ctrl.Result{RequeueAfter: time.Second * 1}, nil
		} else if err != nil {
			logger.Error(err, "Failed to get VPA",
				"phase", "vpa_reconciliation",
				"error_type", "vpa_fetch_error",
			)
			return ctrl.Result{}, err
		}

		logger.Info("VPA found",
			"phase", "vpa_reconciliation",
			"vpa_name", vpa.GetName(),
		)
	} else {
		// Check if VPA exists and should be deleted
		vpa := &unstructured.Unstructured{}
		vpa.SetAPIVersion("autoscaling.k8s.io/v1")
		vpa.SetKind("VerticalPodAutoscaler")

		err = r.Get(ctx, types.NamespacedName{Name: mcpServer.Name, Namespace: mcpServer.Namespace}, vpa)
		if err == nil {
			logger.Info("Deleting VPA as autoscaling is disabled", "phase", "vpa_cleanup")
			err = r.Delete(ctx, vpa)
			if err != nil {
				logger.Error(err, "Failed to delete VPA", "phase", "vpa_cleanup")
				return ctrl.Result{}, err
			}
			logger.Info("VPA deleted successfully", "phase", "vpa_cleanup")
		} else if !errors.IsNotFound(err) {
			logger.Error(err, "Failed to check VPA existence", "phase", "vpa_cleanup")
			return ctrl.Result{}, err
		}
	}

	// Update MCPServer status
	logger.Info("Updating MCPServer status", "phase", "status_update")
	statusStartTime := time.Now()

	err = r.updateMCPServerStatus(ctx, mcpServer, deployment, service)
	if err != nil {
		logger.Error(err, "Failed to update MCPServer status",
			"phase", "status_update",
			"duration", time.Since(statusStartTime),
			"error_type", "status_update_error",
		)
		return ctrl.Result{}, err
	}

	logger.Info("MCPServer status updated successfully",
		"phase", "status_update",
		"duration", time.Since(statusStartTime),
		"new_phase", mcpServer.Status.Phase,
		"replicas", mcpServer.Status.Replicas,
		"ready_replicas", mcpServer.Status.ReadyReplicas,
	)

	logger.Info("Reconciliation completed successfully",
		"total_duration", time.Since(startTime),
		"final_phase", mcpServer.Status.Phase,
	)

	// Record successful reconciliation
	metrics.RecordReconcileSuccess(mcpServer.Namespace, time.Since(startTime).Seconds(), string(mcpServer.Status.Phase))

	return ctrl.Result{RequeueAfter: time.Minute * 1}, nil
}

// handleDeletion handles the deletion of MCPServer resources with graceful cleanup
func (r *MCPServerReconciler) handleDeletion(ctx context.Context, logger logr.Logger, mcpServer *mcpv1.MCPServer) (ctrl.Result, error) {
	deletionStartTime := time.Now()

	logger.Info("Starting graceful deletion process",
		"phase", "deletion",
		"finalizers", mcpServer.GetFinalizers(),
	)

	// Perform cleanup operations
	if err := r.performCleanup(ctx, logger, mcpServer); err != nil {
		logger.Error(err, "Failed to perform cleanup",
			"phase", "deletion",
			"duration", time.Since(deletionStartTime),
			"error_type", "cleanup_error",
		)
		metrics.RecordCleanupOperation(mcpServer.Namespace, "cleanup", "error", time.Since(deletionStartTime).Seconds())
		return ctrl.Result{}, err
	}

	// Record successful cleanup
	metrics.RecordCleanupOperation(mcpServer.Namespace, "cleanup", "success", time.Since(deletionStartTime).Seconds())

	// Remove MCPServer info metrics
	metrics.RemoveMCPServerInfo(
		mcpServer.Namespace,
		mcpServer.Name,
		mcpServer.Spec.Registry.Name,
		mcpServer.Spec.Runtime.Type,
		mcpServer.Spec.Runtime.Image,
		mcpServer.Spec.Registry.Version,
	)

	// Remove finalizer to allow deletion
	logger.Info("Removing finalizer", "phase", "deletion")
	controllerutil.RemoveFinalizer(mcpServer, MCPServerFinalizer)

	err := r.Update(ctx, mcpServer)
	if err != nil {
		logger.Error(err, "Failed to remove finalizer",
			"phase", "deletion",
			"duration", time.Since(deletionStartTime),
			"error_type", "finalizer_removal_error",
		)
		return ctrl.Result{}, err
	}

	logger.Info("MCPServer deletion completed successfully",
		"phase", "deletion",
		"duration", time.Since(deletionStartTime),
	)

	return ctrl.Result{}, nil
}

// performCleanup performs cleanup operations before MCPServer deletion
func (r *MCPServerReconciler) performCleanup(ctx context.Context, logger logr.Logger, mcpServer *mcpv1.MCPServer) error {
	cleanupStartTime := time.Now()

	logger.Info("Performing cleanup operations", "phase", "cleanup")

	// Scale down deployment to 0 for graceful shutdown
	deployment := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: mcpServer.Name, Namespace: mcpServer.Namespace}, deployment)
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to get deployment for cleanup: %w", err)
	}

	if err == nil && deployment.Spec.Replicas != nil && *deployment.Spec.Replicas > 0 {
		logger.Info("Scaling down deployment for graceful shutdown",
			"phase", "cleanup",
			"current_replicas", *deployment.Spec.Replicas,
		)

		// Scale to 0
		replicas := int32(0)
		deployment.Spec.Replicas = &replicas

		err = r.Update(ctx, deployment)
		if err != nil {
			return fmt.Errorf("failed to scale down deployment: %w", err)
		}

		// Wait for pods to terminate gracefully (with timeout)
		timeout := time.Second * 30
		if err := r.waitForPodsTermination(ctx, logger, mcpServer, timeout); err != nil {
			logger.Info("Warning: Timeout waiting for pods to terminate gracefully",
				"phase", "cleanup",
				"timeout", timeout,
				"warning", true,
			)
		}
	}

	// Additional cleanup operations can be added here
	// For example: cleanup external resources, notify external systems, etc.

	logger.Info("Cleanup operations completed",
		"phase", "cleanup",
		"duration", time.Since(cleanupStartTime),
	)

	return nil
}

// waitForPodsTermination waits for pods to terminate gracefully
func (r *MCPServerReconciler) waitForPodsTermination(ctx context.Context, logger logr.Logger, mcpServer *mcpv1.MCPServer, timeout time.Duration) error {
	logger.Info("Waiting for pods to terminate gracefully",
		"phase", "cleanup",
		"timeout", timeout,
	)

	timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	ticker := time.NewTicker(time.Second * 2)
	defer ticker.Stop()

	for {
		select {
		case <-timeoutCtx.Done():
			return fmt.Errorf("timeout waiting for pods to terminate")
		case <-ticker.C:
			// Check if deployment still has running pods
			deployment := &appsv1.Deployment{}
			err := r.Get(ctx, types.NamespacedName{Name: mcpServer.Name, Namespace: mcpServer.Namespace}, deployment)
			if err != nil {
				if errors.IsNotFound(err) {
					logger.Info("Deployment not found, pods terminated", "phase", "cleanup")
					return nil
				}
				return fmt.Errorf("failed to check deployment status: %w", err)
			}

			if deployment.Status.Replicas == 0 {
				logger.Info("All pods terminated successfully", "phase", "cleanup")
				return nil
			}

			logger.Info("Waiting for pods to terminate",
				"phase", "cleanup",
				"remaining_replicas", deployment.Status.Replicas,
			)
		}
	}
}

// deploymentForMCPServer returns a MCPServer Deployment object
func (r *MCPServerReconciler) deploymentForMCPServer(mcpServer *mcpv1.MCPServer) *appsv1.Deployment {
	labels := r.labelsForMCPServerWithTenancy(mcpServer)
	replicas := int32(1)
	if mcpServer.Spec.Replicas != nil {
		replicas = *mcpServer.Spec.Replicas
	}

	port := int32(8080)
	if mcpServer.Spec.Runtime.Port != 0 {
		port = mcpServer.Spec.Runtime.Port
	}

	image := mcpServer.Spec.Runtime.Image

	dep := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcpServer.Name,
			Namespace: mcpServer.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Image:   image,
						Name:    "mcp-server",
						Command: mcpServer.Spec.Runtime.Command,
						Args:    mcpServer.Spec.Runtime.Args,
						Ports: []corev1.ContainerPort{{
							ContainerPort: port,
							Name:          "http",
						}},
						// Environment variables
						Env:     r.buildEnvironmentVariables(mcpServer),
						EnvFrom: r.buildEnvFromSources(mcpServer),
						// Volume mounts
						VolumeMounts: r.buildVolumeMounts(mcpServer),
						// Health checks and readiness probes
						LivenessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/health",
									Port: intstr.FromInt32(port),
								},
							},
							InitialDelaySeconds: 30,
							PeriodSeconds:       10,
							TimeoutSeconds:      5,
							FailureThreshold:    3,
							SuccessThreshold:    1,
						},
						ReadinessProbe: &corev1.Probe{
							ProbeHandler: corev1.ProbeHandler{
								HTTPGet: &corev1.HTTPGetAction{
									Path: "/ready",
									Port: intstr.FromInt32(port),
								},
							},
							InitialDelaySeconds: 5,
							PeriodSeconds:       5,
							TimeoutSeconds:      3,
							FailureThreshold:    3,
							SuccessThreshold:    1,
						},
						// Resource requirements
						Resources: r.buildResourceRequirements(mcpServer),
						// Security context
						SecurityContext: r.buildSecurityContext(mcpServer),
					}},
					// Volumes
					Volumes: r.buildVolumes(mcpServer),
					// Pod security context
					SecurityContext: r.buildPodSecurityContext(mcpServer),
					// Node selector, tolerations, and affinity
					NodeSelector: r.buildNodeSelector(mcpServer),
					Tolerations:  r.buildTolerations(mcpServer),
					Affinity:     r.buildAffinity(mcpServer),
				},
			},
		},
	}

	// Set MCPServer instance as the owner and controller
	if err := controllerutil.SetControllerReference(mcpServer, dep, r.Scheme); err != nil {
		return nil
	}
	return dep
}

// serviceForMCPServer returns a MCPServer Service object
func (r *MCPServerReconciler) serviceForMCPServer(mcpServer *mcpv1.MCPServer) *corev1.Service {
	labels := r.labelsForMCPServerWithTenancy(mcpServer)

	port := int32(8080)
	if mcpServer.Spec.Runtime.Port != 0 {
		port = mcpServer.Spec.Runtime.Port
	}

	serviceType := corev1.ServiceTypeClusterIP

	srv := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcpServer.Name,
			Namespace: mcpServer.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Type:     serviceType,
			Ports: []corev1.ServicePort{{
				Port:       port,
				TargetPort: intstr.FromInt32(port),
				Name:       "http",
			}},
		},
	}

	// Set MCPServer instance as the owner and controller
	if err := controllerutil.SetControllerReference(mcpServer, srv, r.Scheme); err != nil {
		return nil
	}
	return srv
}

// hpaForMCPServer returns a HorizontalPodAutoscaler object for MCPServer
func (r *MCPServerReconciler) hpaForMCPServer(mcpServer *mcpv1.MCPServer) *autoscalingv2.HorizontalPodAutoscaler {
	labels := r.labelsForMCPServerWithTenancy(mcpServer)
	hpaSpec := mcpServer.Spec.Autoscaling.HPA

	// Set default values
	minReplicas := int32(1)
	if hpaSpec.MinReplicas != nil {
		minReplicas = *hpaSpec.MinReplicas
	}

	maxReplicas := hpaSpec.MaxReplicas
	if maxReplicas == 0 {
		maxReplicas = 10 // Default max replicas
	}

	hpa := &autoscalingv2.HorizontalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcpServer.Name,
			Namespace: mcpServer.Namespace,
			Labels:    labels,
		},
		Spec: autoscalingv2.HorizontalPodAutoscalerSpec{
			ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       mcpServer.Name,
			},
			MinReplicas: &minReplicas,
			MaxReplicas: maxReplicas,
			Metrics:     r.buildHPAMetrics(hpaSpec),
		},
	}

	// Add behavior if specified
	if hpaSpec.Behavior != nil {
		hpa.Spec.Behavior = r.buildHPABehavior(hpaSpec.Behavior)
	}

	// Set MCPServer instance as the owner and controller
	if err := controllerutil.SetControllerReference(mcpServer, hpa, r.Scheme); err != nil {
		return nil
	}
	return hpa
}

// buildHPAMetrics converts MCPServer HPA metrics to Kubernetes HPA metrics
func (r *MCPServerReconciler) buildHPAMetrics(hpaSpec *mcpv1.HPASpec) []autoscalingv2.MetricSpec {
	var metricsspec []autoscalingv2.MetricSpec

	// Add CPU utilization metric if specified
	if hpaSpec.TargetCPUUtilizationPercentage != nil {
		metricsspec = append(metricsspec, autoscalingv2.MetricSpec{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceCPU,
				Target: autoscalingv2.MetricTarget{
					Type:               autoscalingv2.UtilizationMetricType,
					AverageUtilization: hpaSpec.TargetCPUUtilizationPercentage,
				},
			},
		})
	}

	// Add Memory utilization metric if specified
	if hpaSpec.TargetMemoryUtilizationPercentage != nil {
		metricsspec = append(metricsspec, autoscalingv2.MetricSpec{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceMemory,
				Target: autoscalingv2.MetricTarget{
					Type:               autoscalingv2.UtilizationMetricType,
					AverageUtilization: hpaSpec.TargetMemoryUtilizationPercentage,
				},
			},
		})
	}

	// Add custom metricsspec if specified
	for _, customMetric := range hpaSpec.Metrics {
		k8sMetric := r.convertCustomMetric(customMetric)
		if k8sMetric != nil {
			metricsspec = append(metricsspec, *k8sMetric)
		}
	}

	// If no metricsspec specified, use default CPU utilization of 70%
	if len(metricsspec) == 0 {
		defaultCPUUtilization := int32(70)
		metricsspec = append(metricsspec, autoscalingv2.MetricSpec{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceCPU,
				Target: autoscalingv2.MetricTarget{
					Type:               autoscalingv2.UtilizationMetricType,
					AverageUtilization: &defaultCPUUtilization,
				},
			},
		})
	}

	return metricsspec
}

// buildHPABehavior converts MCPServer HPA behavior to Kubernetes HPA behavior
func (r *MCPServerReconciler) buildHPABehavior(behavior *mcpv1.HPABehavior) *autoscalingv2.HorizontalPodAutoscalerBehavior {
	hpaBehavior := &autoscalingv2.HorizontalPodAutoscalerBehavior{}

	if behavior.ScaleUp != nil {
		hpaBehavior.ScaleUp = r.buildHPAScalingRules(behavior.ScaleUp)
	}

	if behavior.ScaleDown != nil {
		hpaBehavior.ScaleDown = r.buildHPAScalingRules(behavior.ScaleDown)
	}

	return hpaBehavior
}

// buildHPAScalingRules converts MCPServer scaling rules to Kubernetes scaling rules
func (r *MCPServerReconciler) buildHPAScalingRules(rules *mcpv1.HPAScalingRules) *autoscalingv2.HPAScalingRules {
	scalingRules := &autoscalingv2.HPAScalingRules{}

	if rules.StabilizationWindowSeconds != nil {
		scalingRules.StabilizationWindowSeconds = rules.StabilizationWindowSeconds
	}

	if rules.SelectPolicy != nil {
		policy := autoscalingv2.ScalingPolicySelect(*rules.SelectPolicy)
		scalingRules.SelectPolicy = &policy
	}

	for _, policy := range rules.Policies {
		scalingRules.Policies = append(scalingRules.Policies, autoscalingv2.HPAScalingPolicy{
			Type:          autoscalingv2.HPAScalingPolicyType(policy.Type),
			Value:         policy.Value,
			PeriodSeconds: policy.PeriodSeconds,
		})
	}

	return scalingRules
}

// convertCustomMetric converts MCPServer custom metric to Kubernetes metric
func (r *MCPServerReconciler) convertCustomMetric(customMetric mcpv1.MetricSpec) *autoscalingv2.MetricSpec {
	switch customMetric.Type {
	case mcpv1.MetricTypeResource:
		if customMetric.Resource != nil {
			return &autoscalingv2.MetricSpec{
				Type: autoscalingv2.ResourceMetricSourceType,
				Resource: &autoscalingv2.ResourceMetricSource{
					Name:   customMetric.Resource.Name,
					Target: r.convertMetricTarget(customMetric.Resource.Target),
				},
			}
		}
	case mcpv1.MetricTypePods:
		if customMetric.Pods != nil {
			return &autoscalingv2.MetricSpec{
				Type: autoscalingv2.PodsMetricSourceType,
				Pods: &autoscalingv2.PodsMetricSource{
					Metric: autoscalingv2.MetricIdentifier{
						Name:     customMetric.Pods.Metric.Name,
						Selector: customMetric.Pods.Metric.Selector,
					},
					Target: r.convertMetricTarget(customMetric.Pods.Target),
				},
			}
		}
	case mcpv1.MetricTypeObject:
		if customMetric.Object != nil {
			return &autoscalingv2.MetricSpec{
				Type: autoscalingv2.ObjectMetricSourceType,
				Object: &autoscalingv2.ObjectMetricSource{
					DescribedObject: autoscalingv2.CrossVersionObjectReference{
						Kind:       customMetric.Object.DescribedObject.Kind,
						Name:       customMetric.Object.DescribedObject.Name,
						APIVersion: customMetric.Object.DescribedObject.APIVersion,
					},
					Metric: autoscalingv2.MetricIdentifier{
						Name:     customMetric.Object.Metric.Name,
						Selector: customMetric.Object.Metric.Selector,
					},
					Target: r.convertMetricTarget(customMetric.Object.Target),
				},
			}
		}
	case mcpv1.MetricTypeExternal:
		if customMetric.External != nil {
			return &autoscalingv2.MetricSpec{
				Type: autoscalingv2.ExternalMetricSourceType,
				External: &autoscalingv2.ExternalMetricSource{
					Metric: autoscalingv2.MetricIdentifier{
						Name:     customMetric.External.Metric.Name,
						Selector: customMetric.External.Metric.Selector,
					},
					Target: r.convertMetricTarget(customMetric.External.Target),
				},
			}
		}
	}
	return nil
}

// convertMetricTarget converts MCPServer metric target to Kubernetes metric target
func (r *MCPServerReconciler) convertMetricTarget(target mcpv1.MetricTarget) autoscalingv2.MetricTarget {
	k8sTarget := autoscalingv2.MetricTarget{
		Type: autoscalingv2.MetricTargetType(target.Type),
	}

	if target.Value != nil {
		k8sTarget.Value = target.Value
	}
	if target.AverageValue != nil {
		k8sTarget.AverageValue = target.AverageValue
	}
	if target.AverageUtilization != nil {
		k8sTarget.AverageUtilization = target.AverageUtilization
	}

	return k8sTarget
}

// vpaForMCPServer returns a VerticalPodAutoscaler object for MCPServer
func (r *MCPServerReconciler) vpaForMCPServer(mcpServer *mcpv1.MCPServer) *unstructured.Unstructured {
	labels := r.labelsForMCPServerWithTenancy(mcpServer)
	vpaSpec := mcpServer.Spec.Autoscaling.VPA

	vpa := &unstructured.Unstructured{}
	vpa.SetAPIVersion("autoscaling.k8s.io/v1")
	vpa.SetKind("VerticalPodAutoscaler")
	vpa.SetName(mcpServer.Name)
	vpa.SetNamespace(mcpServer.Namespace)
	vpa.SetLabels(labels)

	// Build VPA spec
	spec := map[string]interface{}{
		"targetRef": map[string]interface{}{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"name":       mcpServer.Name,
		},
	}

	// Set update mode
	if vpaSpec.UpdateMode != "" {
		spec["updatePolicy"] = map[string]interface{}{
			"updateMode": string(vpaSpec.UpdateMode),
		}
	} else {
		// Default to Auto mode
		spec["updatePolicy"] = map[string]interface{}{
			"updateMode": "Auto",
		}
	}

	// Set resource policy if specified
	if vpaSpec.ResourcePolicy != nil {
		resourcePolicy := map[string]interface{}{}

		if len(vpaSpec.ResourcePolicy.ContainerPolicies) > 0 {
			containerPolicies := make([]map[string]interface{}, 0, len(vpaSpec.ResourcePolicy.ContainerPolicies))

			for _, policy := range vpaSpec.ResourcePolicy.ContainerPolicies {
				containerPolicy := map[string]interface{}{}

				if policy.ContainerName != "" {
					containerPolicy["containerName"] = policy.ContainerName
				} else {
					containerPolicy["containerName"] = "mcp-server"
				}

				if policy.Mode != "" {
					containerPolicy["mode"] = string(policy.Mode)
				}

				if policy.MinAllowed != nil {
					minAllowed := make(map[string]interface{})
					for k, v := range policy.MinAllowed {
						minAllowed[k] = v
					}
					containerPolicy["minAllowed"] = minAllowed
				}

				if policy.MaxAllowed != nil {
					maxAllowed := make(map[string]interface{})
					for k, v := range policy.MaxAllowed {
						maxAllowed[k] = v
					}
					containerPolicy["maxAllowed"] = maxAllowed
				}

				if len(policy.ControlledResources) > 0 {
					controlledResources := make([]string, len(policy.ControlledResources))
					for i, ctrlResource := range policy.ControlledResources {
						controlledResources[i] = string(ctrlResource)
					}
					containerPolicy["controlledResources"] = controlledResources
				}

				if policy.ControlledValues != "" {
					containerPolicy["controlledValues"] = string(policy.ControlledValues)
				}

				containerPolicies = append(containerPolicies, containerPolicy)
			}

			resourcePolicy["containerPolicies"] = containerPolicies
		}

		spec["resourcePolicy"] = resourcePolicy
	}

	vpa.Object["spec"] = spec

	// Set MCPServer instance as the owner and controller
	if err := controllerutil.SetControllerReference(mcpServer, vpa, r.Scheme); err != nil {
		return nil
	}
	return vpa
}

// buildEnvironmentVariables builds environment variables from SecretRefs and runtime env
func (r *MCPServerReconciler) buildEnvironmentVariables(mcpServer *mcpv1.MCPServer) []corev1.EnvVar {
	var envVars []corev1.EnvVar

	// Add runtime environment variables
	for key, value := range mcpServer.Spec.Runtime.Env {
		envVars = append(envVars, corev1.EnvVar{
			Name:  key,
			Value: value,
		})
	}

	// Add environment variables from SecretRefs
	for _, secretRef := range mcpServer.Spec.SecretRefs {
		envVar := corev1.EnvVar{
			Name: secretRef.EnvVar,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: secretRef.Name,
					},
					Key:      secretRef.Key,
					Optional: &secretRef.Optional,
				},
			},
		}
		envVars = append(envVars, envVar)
	}

	return envVars
}

// buildEnvFromSources builds EnvFrom sources from ConfigMaps and Secrets
func (r *MCPServerReconciler) buildEnvFromSources(mcpServer *mcpv1.MCPServer) []corev1.EnvFromSource {
	var envFromSources []corev1.EnvFromSource

	for _, envFrom := range mcpServer.Spec.EnvFrom {
		envFromSource := corev1.EnvFromSource{
			Prefix: envFrom.Prefix,
		}

		if envFrom.ConfigMapRef != nil {
			envFromSource.ConfigMapRef = &corev1.ConfigMapEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: envFrom.ConfigMapRef.Name,
				},
				Optional: &envFrom.ConfigMapRef.Optional,
			}
		}

		if envFrom.SecretRef != nil {
			envFromSource.SecretRef = &corev1.SecretEnvSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: envFrom.SecretRef.Name,
				},
				Optional: &envFrom.SecretRef.Optional,
			}
		}

		envFromSources = append(envFromSources, envFromSource)
	}

	return envFromSources
}

// buildVolumeMounts builds volume mounts for the container
func (r *MCPServerReconciler) buildVolumeMounts(mcpServer *mcpv1.MCPServer) []corev1.VolumeMount {
	var volumeMounts []corev1.VolumeMount

	// Add volume mounts from ConfigSources
	for i, configSource := range mcpServer.Spec.ConfigSources {
		volumeMount := corev1.VolumeMount{
			Name:      fmt.Sprintf("config-source-%d", i),
			MountPath: configSource.MountPath,
			SubPath:   configSource.SubPath,
			ReadOnly:  configSource.ReadOnly,
		}
		volumeMounts = append(volumeMounts, volumeMount)
	}

	// Add custom volume mounts
	for _, vm := range mcpServer.Spec.VolumeMounts {
		volumeMount := corev1.VolumeMount{
			Name:        vm.Name,
			MountPath:   vm.MountPath,
			SubPath:     vm.SubPath,
			ReadOnly:    vm.ReadOnly,
			SubPathExpr: vm.SubPathExpr,
		}

		if vm.MountPropagation != nil {
			propagation := corev1.MountPropagationMode(*vm.MountPropagation)
			volumeMount.MountPropagation = &propagation
		}

		volumeMounts = append(volumeMounts, volumeMount)
	}

	return volumeMounts
}

// buildVolumes builds volumes for the pod
func (r *MCPServerReconciler) buildVolumes(mcpServer *mcpv1.MCPServer) []corev1.Volume {
	var volumes []corev1.Volume

	// Add volumes from ConfigSources
	for i, configSource := range mcpServer.Spec.ConfigSources {
		volume := corev1.Volume{
			Name: fmt.Sprintf("config-source-%d", i),
		}

		if configSource.ConfigMap != nil {
			volume.VolumeSource = corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: configSource.ConfigMap.Name,
					},
					Optional: &configSource.ConfigMap.Optional,
				},
			}

			// Add specific keys if specified
			if len(configSource.ConfigMap.Keys) > 0 {
				var items []corev1.KeyToPath
				for _, key := range configSource.ConfigMap.Keys {
					items = append(items, corev1.KeyToPath{
						Key:  key,
						Path: key,
					})
				}
				volume.ConfigMap.Items = items
			}
		}

		if configSource.Secret != nil {
			volume.VolumeSource = corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: configSource.Secret.Name,
					Optional:   &configSource.Secret.Optional,
				},
			}

			// Add specific keys if specified
			if len(configSource.Secret.Keys) > 0 {
				var items []corev1.KeyToPath
				for _, key := range configSource.Secret.Keys {
					items = append(items, corev1.KeyToPath{
						Key:  key,
						Path: key,
					})
				}
				volume.Secret.Items = items
			}
		}

		volumes = append(volumes, volume)
	}

	// Add custom volumes
	for _, vol := range mcpServer.Spec.Volumes {
		volume := corev1.Volume{
			Name: vol.Name,
		}

		// Convert MCPVolumeSource to corev1.VolumeSource
		volume.VolumeSource = r.convertMCPVolumeSource(vol.MCPVolumeSource)
		volumes = append(volumes, volume)
	}

	return volumes
}

// convertMCPVolumeSource converts MCPVolumeSource to corev1.VolumeSource
func (r *MCPServerReconciler) convertMCPVolumeSource(mcpVolumeSource mcpv1.MCPVolumeSource) corev1.VolumeSource {
	volumeSource := corev1.VolumeSource{}

	if mcpVolumeSource.ConfigMap != nil {
		volumeSource.ConfigMap = &corev1.ConfigMapVolumeSource{
			LocalObjectReference: corev1.LocalObjectReference{
				Name: mcpVolumeSource.ConfigMap.Name,
			},
			Optional:    &mcpVolumeSource.ConfigMap.Optional,
			DefaultMode: mcpVolumeSource.ConfigMap.DefaultMode,
		}

		// Convert items
		if len(mcpVolumeSource.ConfigMap.Items) > 0 {
			var items []corev1.KeyToPath
			for _, item := range mcpVolumeSource.ConfigMap.Items {
				items = append(items, corev1.KeyToPath{
					Key:  item.Key,
					Path: item.Path,
					Mode: item.Mode,
				})
			}
			volumeSource.ConfigMap.Items = items
		}
	}

	if mcpVolumeSource.Secret != nil {
		volumeSource.Secret = &corev1.SecretVolumeSource{
			SecretName:  mcpVolumeSource.Secret.Name,
			Optional:    &mcpVolumeSource.Secret.Optional,
			DefaultMode: mcpVolumeSource.Secret.DefaultMode,
		}

		// Convert items
		if len(mcpVolumeSource.Secret.Items) > 0 {
			var items []corev1.KeyToPath
			for _, item := range mcpVolumeSource.Secret.Items {
				items = append(items, corev1.KeyToPath{
					Key:  item.Key,
					Path: item.Path,
					Mode: item.Mode,
				})
			}
			volumeSource.Secret.Items = items
		}
	}

	if mcpVolumeSource.EmptyDir != nil {
		volumeSource.EmptyDir = &corev1.EmptyDirVolumeSource{
			Medium: corev1.StorageMedium(mcpVolumeSource.EmptyDir.Medium),
		}
		if mcpVolumeSource.EmptyDir.SizeLimit != nil {
			sizeLimit := resource.MustParse(*mcpVolumeSource.EmptyDir.SizeLimit)
			volumeSource.EmptyDir.SizeLimit = &sizeLimit
		}
	}

	if mcpVolumeSource.PersistentVolumeClaim != nil {
		volumeSource.PersistentVolumeClaim = &corev1.PersistentVolumeClaimVolumeSource{
			ClaimName: mcpVolumeSource.PersistentVolumeClaim.ClaimName,
			ReadOnly:  mcpVolumeSource.PersistentVolumeClaim.ReadOnly,
		}
	}

	if mcpVolumeSource.HostPath != nil {
		volumeSource.HostPath = &corev1.HostPathVolumeSource{
			Path: mcpVolumeSource.HostPath.Path,
		}
		if mcpVolumeSource.HostPath.Type != nil {
			hostPathType := corev1.HostPathType(*mcpVolumeSource.HostPath.Type)
			volumeSource.HostPath.Type = &hostPathType
		}
	}

	if mcpVolumeSource.Projected != nil {
		volumeSource.Projected = &corev1.ProjectedVolumeSource{
			DefaultMode: mcpVolumeSource.Projected.DefaultMode,
		}

		// Convert projected sources
		for _, source := range mcpVolumeSource.Projected.Sources {
			projectedSource := corev1.VolumeProjection{}

			if source.ConfigMap != nil {
				projectedSource.ConfigMap = &corev1.ConfigMapProjection{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: source.ConfigMap.Name,
					},
					Optional: &source.ConfigMap.Optional,
				}

				// Convert items
				if len(source.ConfigMap.Items) > 0 {
					var items []corev1.KeyToPath
					for _, item := range source.ConfigMap.Items {
						items = append(items, corev1.KeyToPath{
							Key:  item.Key,
							Path: item.Path,
							Mode: item.Mode,
						})
					}
					projectedSource.ConfigMap.Items = items
				}
			}

			if source.Secret != nil {
				projectedSource.Secret = &corev1.SecretProjection{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: source.Secret.Name,
					},
					Optional: &source.Secret.Optional,
				}

				// Convert items
				if len(source.Secret.Items) > 0 {
					var items []corev1.KeyToPath
					for _, item := range source.Secret.Items {
						items = append(items, corev1.KeyToPath{
							Key:  item.Key,
							Path: item.Path,
							Mode: item.Mode,
						})
					}
					projectedSource.Secret.Items = items
				}
			}

			if source.ServiceAccountToken != nil {
				projectedSource.ServiceAccountToken = &corev1.ServiceAccountTokenProjection{
					Audience:          source.ServiceAccountToken.Audience,
					ExpirationSeconds: source.ServiceAccountToken.ExpirationSeconds,
					Path:              source.ServiceAccountToken.Path,
				}
			}

			volumeSource.Projected.Sources = append(volumeSource.Projected.Sources, projectedSource)
		}
	}

	return volumeSource
}

// buildResourceRequirements converts MCPServer resource requirements to Kubernetes format
func (r *MCPServerReconciler) buildResourceRequirements(mcpServer *mcpv1.MCPServer) corev1.ResourceRequirements {
	if mcpServer.Spec.Resources.Limits == nil && mcpServer.Spec.Resources.Requests == nil {
		// Set default resource requirements
		return corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("500m"),
				corev1.ResourceMemory: resource.MustParse("512Mi"),
			},
		}
	}

	requirements := corev1.ResourceRequirements{}

	// Convert custom ResourceList to Kubernetes ResourceList
	if mcpServer.Spec.Resources.Requests != nil {
		requirements.Requests = make(corev1.ResourceList)
		for k, v := range mcpServer.Spec.Resources.Requests {
			if quantity, err := resource.ParseQuantity(v); err == nil {
				requirements.Requests[corev1.ResourceName(k)] = quantity
			}
		}
	}

	if mcpServer.Spec.Resources.Limits != nil {
		requirements.Limits = make(corev1.ResourceList)
		for k, v := range mcpServer.Spec.Resources.Limits {
			if quantity, err := resource.ParseQuantity(v); err == nil {
				requirements.Limits[corev1.ResourceName(k)] = quantity
			}
		}
	}

	return requirements
}

// buildSecurityContext builds container security context
func (r *MCPServerReconciler) buildSecurityContext(mcpServer *mcpv1.MCPServer) *corev1.SecurityContext {
	if mcpServer.Spec.SecurityContext != nil {
		return mcpServer.Spec.SecurityContext
	}

	// Default security context
	runAsNonRoot := true
	runAsUser := int64(1000)
	allowPrivilegeEscalation := false
	readOnlyRootFilesystem := true

	return &corev1.SecurityContext{
		RunAsNonRoot:             &runAsNonRoot,
		RunAsUser:                &runAsUser,
		AllowPrivilegeEscalation: &allowPrivilegeEscalation,
		ReadOnlyRootFilesystem:   &readOnlyRootFilesystem,
		Capabilities: &corev1.Capabilities{
			Drop: []corev1.Capability{"ALL"},
		},
	}
}

// buildPodSecurityContext builds pod security context
func (r *MCPServerReconciler) buildPodSecurityContext(mcpServer *mcpv1.MCPServer) *corev1.PodSecurityContext {
	runAsNonRoot := true
	runAsUser := int64(1000)
	runAsGroup := int64(1000)
	fsGroup := int64(1000)

	return &corev1.PodSecurityContext{
		RunAsNonRoot: &runAsNonRoot,
		RunAsUser:    &runAsUser,
		RunAsGroup:   &runAsGroup,
		FSGroup:      &fsGroup,
		SeccompProfile: &corev1.SeccompProfile{
			Type: corev1.SeccompProfileTypeRuntimeDefault,
		},
	}
}

// buildNodeSelector builds node selector with tenant-specific requirements
func (r *MCPServerReconciler) buildNodeSelector(mcpServer *mcpv1.MCPServer) map[string]string {
	nodeSelector := make(map[string]string)

	// Add base node selector from MCPServer spec
	for k, v := range mcpServer.Spec.NodeSelector {
		nodeSelector[k] = v
	}

	// Add tenant-specific node selector if tenancy is configured
	if mcpServer.Spec.Tenancy != nil {
		// Add tenant ID as node selector for strict isolation
		if mcpServer.Spec.Tenancy.IsolationLevel == mcpv1.IsolationLevelNode ||
			mcpServer.Spec.Tenancy.IsolationLevel == mcpv1.IsolationLevelStrict {
			nodeSelector["mcp.allbeone.io/tenant"] = mcpServer.Spec.Tenancy.TenantID
		}

		// Add tenant labels as node selectors
		for k, v := range mcpServer.Spec.Tenancy.Labels {
			if strings.HasPrefix(k, "node.") {
				// Remove "node." prefix and use as node selector
				nodeKey := strings.TrimPrefix(k, "node.")
				nodeSelector[nodeKey] = v
			}
		}
	}

	return nodeSelector
}

// buildTolerations builds tolerations with tenant-specific requirements
func (r *MCPServerReconciler) buildTolerations(mcpServer *mcpv1.MCPServer) []corev1.Toleration {
	var tolerations []corev1.Toleration

	// Add base tolerations from MCPServer spec
	tolerations = append(tolerations, mcpServer.Spec.Tolerations...)

	// Add tenant-specific tolerations if tenancy is configured
	if mcpServer.Spec.Tenancy != nil {
		// Add tenant toleration for dedicated nodes
		if mcpServer.Spec.Tenancy.IsolationLevel == mcpv1.IsolationLevelNode ||
			mcpServer.Spec.Tenancy.IsolationLevel == mcpv1.IsolationLevelStrict {
			tolerations = append(tolerations, corev1.Toleration{
				Key:      "mcp.allbeone.io/tenant",
				Operator: corev1.TolerationOpEqual,
				Value:    mcpServer.Spec.Tenancy.TenantID,
				Effect:   corev1.TaintEffectNoSchedule,
			})
		}

		// Add general tenant toleration
		tolerations = append(tolerations, corev1.Toleration{
			Key:      "mcp.allbeone.io/tenant-workload",
			Operator: corev1.TolerationOpEqual,
			Value:    "true",
			Effect:   corev1.TaintEffectNoSchedule,
		})
	}

	return tolerations
}

// buildAffinity builds affinity rules with tenant-specific requirements
func (r *MCPServerReconciler) buildAffinity(mcpServer *mcpv1.MCPServer) *corev1.Affinity {
	var affinity *corev1.Affinity

	// Start with base affinity from MCPServer spec
	if mcpServer.Spec.Affinity != nil {
		affinity = mcpServer.Spec.Affinity.DeepCopy()
	} else {
		affinity = &corev1.Affinity{}
	}

	// Add tenant-specific affinity rules if tenancy is configured
	if mcpServer.Spec.Tenancy != nil {
		if affinity.PodAffinity == nil {
			affinity.PodAffinity = &corev1.PodAffinity{}
		}
		if affinity.PodAntiAffinity == nil {
			affinity.PodAntiAffinity = &corev1.PodAntiAffinity{}
		}

		// Add pod affinity to prefer scheduling with same tenant
		tenantAffinityTerm := corev1.WeightedPodAffinityTerm{
			Weight: 100,
			PodAffinityTerm: corev1.PodAffinityTerm{
				LabelSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"mcp.allbeone.io/tenant": mcpServer.Spec.Tenancy.TenantID,
					},
				},
				TopologyKey: "kubernetes.io/hostname",
			},
		}
		affinity.PodAffinity.PreferredDuringSchedulingIgnoredDuringExecution =
			append(affinity.PodAffinity.PreferredDuringSchedulingIgnoredDuringExecution, tenantAffinityTerm)

		// Add pod anti-affinity to avoid scheduling with different tenants for strict isolation
		if mcpServer.Spec.Tenancy.IsolationLevel == mcpv1.IsolationLevelStrict {
			tenantAntiAffinityTerm := corev1.WeightedPodAffinityTerm{
				Weight: 100,
				PodAffinityTerm: corev1.PodAffinityTerm{
					LabelSelector: &metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "mcp.allbeone.io/tenant",
								Operator: metav1.LabelSelectorOpNotIn,
								Values:   []string{mcpServer.Spec.Tenancy.TenantID},
							},
						},
					},
					TopologyKey: "kubernetes.io/hostname",
				},
			}
			affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution =
				append(affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution, tenantAntiAffinityTerm)
		}
	}

	return affinity
}

// labelsForMCPServer returns the labels for selecting the resources
func labelsForMCPServer(name string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":       "mcpserver",
		"app.kubernetes.io/instance":   name,
		"app.kubernetes.io/component":  "mcp-server",
		"app.kubernetes.io/created-by": "mcp-operator",
	}
}

// labelsForMCPServerWithTenancy returns the labels for selecting the resources with tenant information
func (r *MCPServerReconciler) labelsForMCPServerWithTenancy(mcpServer *mcpv1.MCPServer) map[string]string {
	labels := labelsForMCPServer(mcpServer.Name)

	// Add tenant-specific labels if tenancy is configured
	if mcpServer.Spec.Tenancy != nil {
		labels["mcp.allbeone.io/tenant"] = mcpServer.Spec.Tenancy.TenantID
		labels["mcp.allbeone.io/isolation-level"] = string(mcpServer.Spec.Tenancy.IsolationLevel)

		// Add custom tenant labels
		for k, v := range mcpServer.Spec.Tenancy.Labels {
			// Avoid overriding system labels
			if !strings.HasPrefix(k, "app.kubernetes.io/") && !strings.HasPrefix(k, "mcp.allbeone.io/") {
				labels[k] = v
			}
		}
	}

	return labels
}

// validateTenantResourceQuotas validates that the MCPServer doesn't exceed tenant resource quotas
func (r *MCPServerReconciler) validateTenantResourceQuotas(ctx context.Context, logger logr.Logger, mcpServer *mcpv1.MCPServer) error {
	if mcpServer.Spec.Tenancy == nil || mcpServer.Spec.Tenancy.ResourceQuotas == nil {
		return nil // No quotas to validate
	}

	logger.Info("Validating tenant resource quotas", "tenant", mcpServer.Spec.Tenancy.TenantID)

	quotas := mcpServer.Spec.Tenancy.ResourceQuotas

	// Validate CPU quota
	if quotas.CPU != nil {
		if err := r.validateCPUQuota(ctx, mcpServer, quotas.CPU); err != nil {
			return fmt.Errorf("CPU quota validation failed: %w", err)
		}
	}

	// Validate Memory quota
	if quotas.Memory != nil {
		if err := r.validateMemoryQuota(ctx, mcpServer, quotas.Memory); err != nil {
			return fmt.Errorf("memory quota validation failed: %w", err)
		}
	}

	// Validate Pod quota
	if quotas.Pods != nil {
		if err := r.validatePodQuota(ctx, mcpServer, quotas.Pods); err != nil {
			return fmt.Errorf("pod quota validation failed: %w", err)
		}
	}

	// Validate Service quota
	if quotas.Services != nil {
		if err := r.validateServiceQuota(ctx, mcpServer, quotas.Services); err != nil {
			return fmt.Errorf("service quota validation failed: %w", err)
		}
	}

	logger.Info("Tenant resource quotas validation passed", "tenant", mcpServer.Spec.Tenancy.TenantID)
	return nil
}

// validateCPUQuota validates CPU usage against tenant quota
func (r *MCPServerReconciler) validateCPUQuota(ctx context.Context, mcpServer *mcpv1.MCPServer, cpuQuota *resource.Quantity) error {
	// Get current CPU usage for the tenant
	currentUsage, err := r.getTenantCPUUsage(ctx, mcpServer.Spec.Tenancy.TenantID, mcpServer.Namespace)
	if err != nil {
		return fmt.Errorf("failed to get current CPU usage: %w", err)
	}

	// Calculate requested CPU for this MCPServer
	requestedCPU := resource.Quantity{}
	if mcpServer.Spec.Resources.Requests != nil {
		if cpuStr, exists := mcpServer.Spec.Resources.Requests["cpu"]; exists {
			if parsed, err := resource.ParseQuantity(cpuStr); err == nil {
				requestedCPU = parsed
			}
		}
	}

	// Apply replica multiplier
	replicas := int32(1)
	if mcpServer.Spec.Replicas != nil {
		replicas = *mcpServer.Spec.Replicas
	}
	requestedCPU.Set(requestedCPU.Value() * int64(replicas))

	// Check if total usage would exceed quota
	totalUsage := currentUsage.DeepCopy()
	totalUsage.Add(requestedCPU)

	if totalUsage.Cmp(*cpuQuota) > 0 {
		return fmt.Errorf("CPU quota exceeded: requested %s + current %s = %s > quota %s",
			requestedCPU.String(), currentUsage.String(), totalUsage.String(), cpuQuota.String())
	}

	return nil
}

// validateMemoryQuota validates Memory usage against tenant quota
func (r *MCPServerReconciler) validateMemoryQuota(ctx context.Context, mcpServer *mcpv1.MCPServer, memoryQuota *resource.Quantity) error {
	// Get current Memory usage for the tenant
	currentUsage, err := r.getTenantMemoryUsage(ctx, mcpServer.Spec.Tenancy.TenantID, mcpServer.Namespace)
	if err != nil {
		return fmt.Errorf("failed to get current Memory usage: %w", err)
	}

	// Calculate requested Memory for this MCPServer
	requestedMemory := resource.Quantity{}
	if mcpServer.Spec.Resources.Requests != nil {
		if memoryStr, exists := mcpServer.Spec.Resources.Requests["memory"]; exists {
			if parsed, err := resource.ParseQuantity(memoryStr); err == nil {
				requestedMemory = parsed
			}
		}
	}

	// Apply replica multiplier
	replicas := int32(1)
	if mcpServer.Spec.Replicas != nil {
		replicas = *mcpServer.Spec.Replicas
	}
	requestedMemory.Set(requestedMemory.Value() * int64(replicas))

	// Check if total usage would exceed quota
	totalUsage := currentUsage.DeepCopy()
	totalUsage.Add(requestedMemory)

	if totalUsage.Cmp(*memoryQuota) > 0 {
		return fmt.Errorf("memory quota exceeded: requested %s + current %s = %s > quota %s",
			requestedMemory.String(), currentUsage.String(), totalUsage.String(), memoryQuota.String())
	}

	return nil
}

// validatePodQuota validates Pod count against tenant quota
func (r *MCPServerReconciler) validatePodQuota(ctx context.Context, mcpServer *mcpv1.MCPServer, podQuota *int32) error {
	// Get current Pod count for the tenant
	currentCount, err := r.getTenantPodCount(ctx, mcpServer.Spec.Tenancy.TenantID, mcpServer.Namespace)
	if err != nil {
		return fmt.Errorf("failed to get current Pod count: %w", err)
	}

	// Calculate requested Pods for this MCPServer
	requestedPods := int32(1)
	if mcpServer.Spec.Replicas != nil {
		requestedPods = *mcpServer.Spec.Replicas
	}

	// Check if total count would exceed quota
	totalCount := currentCount + requestedPods

	if totalCount > *podQuota {
		return fmt.Errorf("pod quota exceeded: requested %d + current %d = %d > quota %d",
			requestedPods, currentCount, totalCount, *podQuota)
	}

	return nil
}

// validateServiceQuota validates Service count against tenant quota
func (r *MCPServerReconciler) validateServiceQuota(ctx context.Context, mcpServer *mcpv1.MCPServer, serviceQuota *int32) error {
	// Get current Service count for the tenant
	currentCount, err := r.getTenantServiceCount(ctx, mcpServer.Spec.Tenancy.TenantID, mcpServer.Namespace)
	if err != nil {
		return fmt.Errorf("failed to get current Service count: %w", err)
	}

	// Each MCPServer creates one service
	requestedServices := int32(1)

	// Check if total count would exceed quota
	totalCount := currentCount + requestedServices

	if totalCount > *serviceQuota {
		return fmt.Errorf("service quota exceeded: requested %d + current %d = %d > quota %d",
			requestedServices, currentCount, totalCount, *serviceQuota)
	}

	return nil
}

// getTenantCPUUsage gets current CPU usage for a tenant
func (r *MCPServerReconciler) getTenantCPUUsage(ctx context.Context, tenantID, namespace string) (resource.Quantity, error) {
	// List all MCPServers for this tenant
	mcpServerList := &mcpv1.MCPServerList{}
	err := r.List(ctx, mcpServerList, client.InNamespace(namespace), client.MatchingLabels{
		"mcp.allbeone.io/tenant": tenantID,
	})
	if err != nil {
		return resource.Quantity{}, err
	}

	totalCPU := resource.Quantity{}
	for _, server := range mcpServerList.Items {
		if server.Spec.Resources.Requests != nil {
			if cpuStr, exists := server.Spec.Resources.Requests["cpu"]; exists {
				if parsed, err := resource.ParseQuantity(cpuStr); err == nil {
					replicas := int32(1)
					if server.Spec.Replicas != nil {
						replicas = *server.Spec.Replicas
					}
					parsed.Set(parsed.Value() * int64(replicas))
					totalCPU.Add(parsed)
				}
			}
		}
	}

	return totalCPU, nil
}

// getTenantMemoryUsage gets current Memory usage for a tenant
func (r *MCPServerReconciler) getTenantMemoryUsage(ctx context.Context, tenantID, namespace string) (resource.Quantity, error) {
	// List all MCPServers for this tenant
	mcpServerList := &mcpv1.MCPServerList{}
	err := r.List(ctx, mcpServerList, client.InNamespace(namespace), client.MatchingLabels{
		"mcp.allbeone.io/tenant": tenantID,
	})
	if err != nil {
		return resource.Quantity{}, err
	}

	totalMemory := resource.Quantity{}
	for _, server := range mcpServerList.Items {
		if server.Spec.Resources.Requests != nil {
			if memoryStr, exists := server.Spec.Resources.Requests["memory"]; exists {
				if parsed, err := resource.ParseQuantity(memoryStr); err == nil {
					replicas := int32(1)
					if server.Spec.Replicas != nil {
						replicas = *server.Spec.Replicas
					}
					parsed.Set(parsed.Value() * int64(replicas))
					totalMemory.Add(parsed)
				}
			}
		}
	}

	return totalMemory, nil
}

// getTenantPodCount gets current Pod count for a tenant
func (r *MCPServerReconciler) getTenantPodCount(ctx context.Context, tenantID, namespace string) (int32, error) {
	// List all MCPServers for this tenant
	mcpServerList := &mcpv1.MCPServerList{}
	err := r.List(ctx, mcpServerList, client.InNamespace(namespace), client.MatchingLabels{
		"mcp.allbeone.io/tenant": tenantID,
	})
	if err != nil {
		return 0, err
	}

	totalPods := int32(0)
	for _, server := range mcpServerList.Items {
		replicas := int32(1)
		if server.Spec.Replicas != nil {
			replicas = *server.Spec.Replicas
		}
		totalPods += replicas
	}

	return totalPods, nil
}

// getTenantServiceCount gets current Service count for a tenant
func (r *MCPServerReconciler) getTenantServiceCount(ctx context.Context, tenantID, namespace string) (int32, error) {
	// List all Services for this tenant
	serviceList := &corev1.ServiceList{}
	err := r.List(ctx, serviceList, client.InNamespace(namespace), client.MatchingLabels{
		"mcp.allbeone.io/tenant": tenantID,
	})
	if err != nil {
		return 0, err
	}

	return int32(len(serviceList.Items)), nil
}

// createNetworkPolicyForTenant creates a NetworkPolicy for tenant isolation
func (r *MCPServerReconciler) createNetworkPolicyForTenant(ctx context.Context, logger logr.Logger, mcpServer *mcpv1.MCPServer) error {
	if mcpServer.Spec.Tenancy == nil || mcpServer.Spec.Tenancy.NetworkPolicy == nil || !mcpServer.Spec.Tenancy.NetworkPolicy.Enabled {
		return nil // No network policy needed
	}

	logger.Info("Creating network policy for tenant", "tenant", mcpServer.Spec.Tenancy.TenantID)

	networkPolicy := r.networkPolicyForMCPServer(mcpServer)

	// Check if a network policy already exists
	existingPolicy := &networkingv1.NetworkPolicy{}
	err := r.Get(ctx, types.NamespacedName{Name: networkPolicy.Name, Namespace: networkPolicy.Namespace}, existingPolicy)
	if err != nil && errors.IsNotFound(err) {
		// Create a new network policy
		logger.Info("Creating new NetworkPolicy",
			"policy_name", networkPolicy.Name,
			"tenant", mcpServer.Spec.Tenancy.TenantID,
		)

		err = r.Create(ctx, networkPolicy)
		if err != nil {
			return fmt.Errorf("failed to create NetworkPolicy: %w", err)
		}

		logger.Info("NetworkPolicy created successfully",
			"policy_name", networkPolicy.Name,
			"tenant", mcpServer.Spec.Tenancy.TenantID,
		)
	} else if err != nil {
		return fmt.Errorf("failed to get NetworkPolicy: %w", err)
	} else {
		// Update existing network policy if needed
		logger.Info("NetworkPolicy already exists",
			"policy_name", networkPolicy.Name,
			"tenant", mcpServer.Spec.Tenancy.TenantID,
		)
	}

	return nil
}

// networkPolicyForMCPServer creates a NetworkPolicy for tenant isolation
func (r *MCPServerReconciler) networkPolicyForMCPServer(mcpServer *mcpv1.MCPServer) *networkingv1.NetworkPolicy {
	labels := r.labelsForMCPServerWithTenancy(mcpServer)
	tenantPolicy := mcpServer.Spec.Tenancy.NetworkPolicy

	// Create pod selector for this tenant
	podSelector := metav1.LabelSelector{
		MatchLabels: map[string]string{
			"mcp.allbeone.io/tenant": mcpServer.Spec.Tenancy.TenantID,
		},
	}

	networkPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("mcp-tenant-%s", mcpServer.Spec.Tenancy.TenantID),
			Namespace: mcpServer.Namespace,
			Labels:    labels,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: podSelector,
			PolicyTypes: []networkingv1.PolicyType{},
		},
	}

	// Add ingress rules if specified
	if len(tenantPolicy.IngressRules) > 0 {
		networkPolicy.Spec.PolicyTypes = append(networkPolicy.Spec.PolicyTypes, networkingv1.PolicyTypeIngress)
		networkPolicy.Spec.Ingress = r.convertIngressRules(tenantPolicy.IngressRules)
	} else {
		// Default ingress rules for tenant isolation
		networkPolicy.Spec.PolicyTypes = append(networkPolicy.Spec.PolicyTypes, networkingv1.PolicyTypeIngress)
		networkPolicy.Spec.Ingress = r.buildDefaultIngressRules(mcpServer)
	}

	// Add egress rules if specified
	if len(tenantPolicy.EgressRules) > 0 {
		networkPolicy.Spec.PolicyTypes = append(networkPolicy.Spec.PolicyTypes, networkingv1.PolicyTypeEgress)
		networkPolicy.Spec.Egress = r.convertEgressRules(tenantPolicy.EgressRules)
	} else {
		// Default egress rules for tenant isolation
		networkPolicy.Spec.PolicyTypes = append(networkPolicy.Spec.PolicyTypes, networkingv1.PolicyTypeEgress)
		networkPolicy.Spec.Egress = r.buildDefaultEgressRules(mcpServer)
	}

	// Set MCPServer instance as the owner and controller
	if err := controllerutil.SetControllerReference(mcpServer, networkPolicy, r.Scheme); err != nil {
		return nil
	}
	return networkPolicy
}

// convertIngressRules converts custom ingress rules to Kubernetes NetworkPolicy ingress rules
func (r *MCPServerReconciler) convertIngressRules(ingressRules []mcpv1.NetworkPolicyIngressRule) []networkingv1.NetworkPolicyIngressRule {
	var rules []networkingv1.NetworkPolicyIngressRule

	for _, rule := range ingressRules {
		k8sRule := networkingv1.NetworkPolicyIngressRule{}

		// Convert ports
		for _, port := range rule.Ports {
			k8sPort := networkingv1.NetworkPolicyPort{}
			if port.Protocol != nil {
				k8sPort.Protocol = port.Protocol
			}
			if port.Port != nil {
				k8sPort.Port = port.Port
			}
			if port.EndPort != nil {
				k8sPort.EndPort = port.EndPort
			}
			k8sRule.Ports = append(k8sRule.Ports, k8sPort)
		}

		// Convert from peers
		for _, peer := range rule.From {
			k8sPeer := networkingv1.NetworkPolicyPeer{}
			if peer.PodSelector != nil {
				k8sPeer.PodSelector = peer.PodSelector
			}
			if peer.NamespaceSelector != nil {
				k8sPeer.NamespaceSelector = peer.NamespaceSelector
			}
			if peer.IPBlock != nil {
				k8sPeer.IPBlock = &networkingv1.IPBlock{
					CIDR:   peer.IPBlock.CIDR,
					Except: peer.IPBlock.Except,
				}
			}
			k8sRule.From = append(k8sRule.From, k8sPeer)
		}

		rules = append(rules, k8sRule)
	}

	return rules
}

// convertEgressRules converts custom egress rules to Kubernetes NetworkPolicy egress rules
func (r *MCPServerReconciler) convertEgressRules(egressRules []mcpv1.NetworkPolicyEgressRule) []networkingv1.NetworkPolicyEgressRule {
	var rules []networkingv1.NetworkPolicyEgressRule

	for _, rule := range egressRules {
		k8sRule := networkingv1.NetworkPolicyEgressRule{}

		// Convert ports
		for _, port := range rule.Ports {
			k8sPort := networkingv1.NetworkPolicyPort{}
			if port.Protocol != nil {
				k8sPort.Protocol = port.Protocol
			}
			if port.Port != nil {
				k8sPort.Port = port.Port
			}
			if port.EndPort != nil {
				k8sPort.EndPort = port.EndPort
			}
			k8sRule.Ports = append(k8sRule.Ports, k8sPort)
		}

		// Convert to peers
		for _, peer := range rule.To {
			k8sPeer := networkingv1.NetworkPolicyPeer{}
			if peer.PodSelector != nil {
				k8sPeer.PodSelector = peer.PodSelector
			}
			if peer.NamespaceSelector != nil {
				k8sPeer.NamespaceSelector = peer.NamespaceSelector
			}
			if peer.IPBlock != nil {
				k8sPeer.IPBlock = &networkingv1.IPBlock{
					CIDR:   peer.IPBlock.CIDR,
					Except: peer.IPBlock.Except,
				}
			}
			k8sRule.To = append(k8sRule.To, k8sPeer)
		}

		rules = append(rules, k8sRule)
	}

	return rules
}

// buildDefaultIngressRules builds default ingress rules for tenant isolation
func (r *MCPServerReconciler) buildDefaultIngressRules(mcpServer *mcpv1.MCPServer) []networkingv1.NetworkPolicyIngressRule {
	var rules []networkingv1.NetworkPolicyIngressRule

	// Allow ingress from same tenant
	sameTenantRule := networkingv1.NetworkPolicyIngressRule{
		From: []networkingv1.NetworkPolicyPeer{
			{
				PodSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"mcp.allbeone.io/tenant": mcpServer.Spec.Tenancy.TenantID,
					},
				},
			},
		},
	}
	rules = append(rules, sameTenantRule)

	// Allow ingress from allowed namespaces if specified
	if mcpServer.Spec.Tenancy.NetworkPolicy != nil {
		for _, allowedNS := range mcpServer.Spec.Tenancy.NetworkPolicy.AllowedNamespaces {
			nsRule := networkingv1.NetworkPolicyIngressRule{
				From: []networkingv1.NetworkPolicyPeer{
					{
						NamespaceSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"kubernetes.io/metadata.name": allowedNS,
							},
						},
					},
				},
			}
			rules = append(rules, nsRule)
		}

		// Allow ingress from allowed pods if specified
		if mcpServer.Spec.Tenancy.NetworkPolicy.AllowedPods != nil {
			podRule := networkingv1.NetworkPolicyIngressRule{
				From: []networkingv1.NetworkPolicyPeer{
					{
						PodSelector: mcpServer.Spec.Tenancy.NetworkPolicy.AllowedPods,
					},
				},
			}
			rules = append(rules, podRule)
		}
	}

	return rules
}

// buildDefaultEgressRules builds default egress rules for tenant isolation
func (r *MCPServerReconciler) buildDefaultEgressRules(mcpServer *mcpv1.MCPServer) []networkingv1.NetworkPolicyEgressRule {
	var rules []networkingv1.NetworkPolicyEgressRule

	// Allow egress to same tenant
	sameTenantRule := networkingv1.NetworkPolicyEgressRule{
		To: []networkingv1.NetworkPolicyPeer{
			{
				PodSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"mcp.allbeone.io/tenant": mcpServer.Spec.Tenancy.TenantID,
					},
				},
			},
		},
	}
	rules = append(rules, sameTenantRule)

	// Allow egress to DNS (kube-system namespace)
	dnsRule := networkingv1.NetworkPolicyEgressRule{
		To: []networkingv1.NetworkPolicyPeer{
			{
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"kubernetes.io/metadata.name": "kube-system",
					},
				},
			},
		},
		Ports: []networkingv1.NetworkPolicyPort{
			{
				Protocol: &[]corev1.Protocol{corev1.ProtocolUDP}[0],
				Port:     &[]intstr.IntOrString{intstr.FromInt32(53)}[0],
			},
			{
				Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
				Port:     &[]intstr.IntOrString{intstr.FromInt32(53)}[0],
			},
		},
	}
	rules = append(rules, dnsRule)

	// Allow egress to allowed namespaces if specified
	if mcpServer.Spec.Tenancy.NetworkPolicy != nil {
		for _, allowedNS := range mcpServer.Spec.Tenancy.NetworkPolicy.AllowedNamespaces {
			nsRule := networkingv1.NetworkPolicyEgressRule{
				To: []networkingv1.NetworkPolicyPeer{
					{
						NamespaceSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"kubernetes.io/metadata.name": allowedNS,
							},
						},
					},
				},
			}
			rules = append(rules, nsRule)
		}

		// Allow egress to allowed pods if specified
		if mcpServer.Spec.Tenancy.NetworkPolicy.AllowedPods != nil {
			podRule := networkingv1.NetworkPolicyEgressRule{
				To: []networkingv1.NetworkPolicyPeer{
					{
						PodSelector: mcpServer.Spec.Tenancy.NetworkPolicy.AllowedPods,
					},
				},
			}
			rules = append(rules, podRule)
		}
	}

	return rules
}

// updateMCPServerStatus updates the status of the MCPServer resource
func (r *MCPServerReconciler) updateMCPServerStatus(ctx context.Context, mcpServer *mcpv1.MCPServer, deployment *appsv1.Deployment, service *corev1.Service) error {
	// Update status based on deployment status
	mcpServer.Status.ReadyReplicas = deployment.Status.ReadyReplicas
	mcpServer.Status.AvailableReplicas = deployment.Status.AvailableReplicas

	// Determine phase
	switch readyReplicas := deployment.Status.ReadyReplicas; readyReplicas {
	case 0:
		mcpServer.Status.Phase = "Pending"
		mcpServer.Status.Message = "Waiting for pods to be ready"
	case *deployment.Spec.Replicas:
		mcpServer.Status.Phase = "Running"
		mcpServer.Status.Message = "All replicas are ready"
	default:
		mcpServer.Status.Phase = "Pending"
		mcpServer.Status.Message = fmt.Sprintf("Ready replicas: %d/%d", readyReplicas, *deployment.Spec.Replicas)
	}

	// Set service endpoint
	if service.Spec.Type == corev1.ServiceTypeClusterIP {
		mcpServer.Status.ServiceEndpoint = fmt.Sprintf("%s.%s.svc.cluster.local:%d", service.Name, service.Namespace, service.Spec.Ports[0].Port)
	}

	// Update last update time
	now := metav1.Now()
	mcpServer.Status.LastUpdateTime = &now

	// Update the status
	return r.Status().Update(ctx, mcpServer)
}

// enrichFromRegistry обогащает MCPServer данными из реестра
func (r *MCPServerReconciler) enrichFromRegistry(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	logger := log.FromContext(ctx)

	if r.RegistryClient == nil {
		r.setRegistryCondition(mcpServer, metav1.ConditionFalse, "RegistryClientNotConfigured", "Registry client is not configured")
		return nil // Регистр не настроен
	}

	// Если данные уже есть, пропускаем
	if mcpServer.Spec.Registry.Version != "" && mcpServer.Spec.Runtime.Image != "" {
		r.setRegistryCondition(mcpServer, metav1.ConditionTrue, "RegistryDataAlreadyPresent", "Registry data is already present in the spec")
		return nil
	}

	logger.Info("Fetching server specification from registry", "serverName", mcpServer.Spec.Registry.Name)

	spec, err := r.RegistryClient.GetServerSpec(ctx, mcpServer.Spec.Registry.Name)
	if err != nil {
		r.setRegistryCondition(mcpServer, metav1.ConditionFalse, "RegistryFetchFailed", fmt.Sprintf("Failed to fetch server spec from registry: %v", err))
		return fmt.Errorf("failed to get server spec: %w", err)
	}

	// Обогащаем данными из реестра
	mcpServer.Spec.Registry.Version = spec.Version
	mcpServer.Spec.Registry.Description = spec.Description
	mcpServer.Spec.Registry.Repository = spec.Repository
	mcpServer.Spec.Registry.License = spec.License
	mcpServer.Spec.Registry.Author = spec.Author
	mcpServer.Spec.Registry.Keywords = spec.Keywords
	mcpServer.Spec.Registry.Capabilities = spec.Capabilities

	// Обогащаем runtime если не задан
	if mcpServer.Spec.Runtime.Type == "" {
		mcpServer.Spec.Runtime.Type = spec.Runtime.Type
	}
	if mcpServer.Spec.Runtime.Image == "" {
		mcpServer.Spec.Runtime.Image = spec.Runtime.Image
	}
	if len(mcpServer.Spec.Runtime.Command) == 0 {
		mcpServer.Spec.Runtime.Command = spec.Runtime.Command
	}
	if len(mcpServer.Spec.Runtime.Args) == 0 {
		mcpServer.Spec.Runtime.Args = spec.Runtime.Args
	}

	// Обогащаем переменные окружения из реестра
	if mcpServer.Spec.Runtime.Env == nil {
		mcpServer.Spec.Runtime.Env = make(map[string]string)
	}
	for k, v := range spec.Runtime.Env {
		if _, exists := mcpServer.Spec.Runtime.Env[k]; !exists {
			mcpServer.Spec.Runtime.Env[k] = v
		}
	}

	r.setRegistryCondition(mcpServer, metav1.ConditionTrue, "RegistryFetchSuccessful", "Successfully fetched and applied server specification from registry")

	logger.Info("Successfully enriched MCPServer with registry data",
		"serverName", mcpServer.Spec.Registry.Name,
		"version", spec.Version,
		"image", spec.Runtime.Image)

	return r.Update(ctx, mcpServer)
}

// setRegistryCondition устанавливает условие RegistryFetched в статусе MCPServer
func (r *MCPServerReconciler) setRegistryCondition(mcpServer *mcpv1.MCPServer, status metav1.ConditionStatus, reason, message string) {
	condition := mcpv1.MCPServerCondition{
		Type:               mcpv1.MCPServerConditionRegistryFetched,
		Status:             status,
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}

	// Найти существующее условие или добавить новое
	found := false
	for i, existingCondition := range mcpServer.Status.Conditions {
		if existingCondition.Type == mcpv1.MCPServerConditionRegistryFetched {
			// Обновляем только если статус изменился
			if existingCondition.Status != status {
				mcpServer.Status.Conditions[i] = condition
			} else {
				// Обновляем сообщение и причину, но не время перехода
				mcpServer.Status.Conditions[i].Reason = reason
				mcpServer.Status.Conditions[i].Message = message
			}
			found = true
			break
		}
	}

	if !found {
		mcpServer.Status.Conditions = append(mcpServer.Status.Conditions, condition)
	}
}

// reconcileDeployment создает или обновляет Deployment
func (r *MCPServerReconciler) reconcileDeployment(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	deployment := &appsv1.Deployment{}
	deploymentName := mcpServer.Name

	err := r.Get(ctx, types.NamespacedName{
		Name:      deploymentName,
		Namespace: mcpServer.Namespace,
	}, deployment)

	if errors.IsNotFound(err) {
		// Создаем новый Deployment
		deployment = r.createDeploymentForMCPServer(mcpServer)
		if err := controllerutil.SetControllerReference(mcpServer, deployment, r.Scheme); err != nil {
			return err
		}
		return r.Create(ctx, deployment)
	} else if err != nil {
		return err
	}

	// Обновляем существующий Deployment если нужно
	// Здесь можно добавить логику сравнения и обновления
	return nil
}

// createDeploymentForMCPServer создает Deployment для MCPServer
func (r *MCPServerReconciler) createDeploymentForMCPServer(mcpServer *mcpv1.MCPServer) *appsv1.Deployment {
	replicas := int32(1)
	if mcpServer.Spec.Replicas != nil {
		replicas = *mcpServer.Spec.Replicas
	}

	labels := r.labelsForMCPServer(mcpServer)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcpServer.Name,
			Namespace: mcpServer.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: labels,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: labels,
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:    "mcp-server",
							Image:   mcpServer.Spec.Runtime.Image,
							Command: mcpServer.Spec.Runtime.Command,
							Args:    mcpServer.Spec.Runtime.Args,
							Ports: []corev1.ContainerPort{
								{
									Name:          "mcp",
									ContainerPort: r.getMCPPort(mcpServer),
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Env: r.buildEnvironmentVars(mcpServer),
							// Добавить ресурсы если указаны
							Resources: r.buildResourceRequirements(mcpServer),
						},
					},
					ServiceAccountName: mcpServer.Spec.ServiceAccount,
				},
			},
		},
	}

	return deployment
}

// reconcileService создает или обновляет Service
func (r *MCPServerReconciler) reconcileService(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	service := &corev1.Service{}
	serviceName := mcpServer.Name

	err := r.Get(ctx, types.NamespacedName{
		Name:      serviceName,
		Namespace: mcpServer.Namespace,
	}, service)

	if errors.IsNotFound(err) {
		// Создаем новый Service
		service = r.createServiceForMCPServer(mcpServer)
		if err := controllerutil.SetControllerReference(mcpServer, service, r.Scheme); err != nil {
			return err
		}
		return r.Create(ctx, service)
	} else if err != nil {
		return err
	}

	return nil
}

// createServiceForMCPServer создает Service для MCPServer
func (r *MCPServerReconciler) createServiceForMCPServer(mcpServer *mcpv1.MCPServer) *corev1.Service {
	labels := r.labelsForMCPServer(mcpServer)
	port := r.getMCPPort(mcpServer)

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcpServer.Name,
			Namespace: mcpServer.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Selector: labels,
			Ports: []corev1.ServicePort{
				{
					Name:       "mcp",
					Port:       port,
					TargetPort: intstr.FromInt(int(port)),
					Protocol:   corev1.ProtocolTCP,
				},
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}

	return service
}

// labelsForMCPServer returns the base labels for MCPServer resources
func (r *MCPServerReconciler) labelsForMCPServer(mcpServer *mcpv1.MCPServer) map[string]string {
	labels := map[string]string{
		"app.kubernetes.io/name":       "mcp-server",
		"app.kubernetes.io/instance":   mcpServer.Name,
		"app.kubernetes.io/component":  "mcp-server",
		"app.kubernetes.io/created-by": "mcp-operator",
	}

	if mcpServer.Spec.Selector != nil {
		for k, v := range mcpServer.Spec.Selector {
			labels[k] = v
		}
	}

	return labels
}

func (r *MCPServerReconciler) getMCPPort(mcpServer *mcpv1.MCPServer) int32 {
	if mcpServer.Spec.Runtime.Port > 0 {
		return mcpServer.Spec.Runtime.Port
	}
	return 8080 // default port
}

func (r *MCPServerReconciler) buildEnvironmentVars(mcpServer *mcpv1.MCPServer) []corev1.EnvVar {
	var envVars []corev1.EnvVar

	// Добавляем переменные из spec.environment
	for k, v := range mcpServer.Spec.Environment {
		envVars = append(envVars, corev1.EnvVar{
			Name:  k,
			Value: v,
		})
	}

	// Добавляем переменные из runtime
	for k, v := range mcpServer.Spec.Runtime.Env {
		envVars = append(envVars, corev1.EnvVar{
			Name:  k,
			Value: v,
		})
	}

	return envVars
}

func (r *MCPServerReconciler) updateStatus(ctx context.Context, mcpServer *mcpv1.MCPServer, phase mcpv1.MCPServerPhase, message, reason string) {
	mcpServer.Status.Phase = phase
	mcpServer.Status.Message = message
	mcpServer.Status.Reason = reason
	mcpServer.Status.LastUpdateTime = &metav1.Time{Time: time.Now()}
	if err := r.Status().Update(ctx, mcpServer); err != nil {
		log.FromContext(ctx).Error(err, "Failed to update MCPServer status")
	}
}

func (r *MCPServerReconciler) updateStatusFromDeployment(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	deployment := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      mcpServer.Name,
		Namespace: mcpServer.Namespace,
	}, deployment)

	if err != nil {
		return err
	}

	// Обновляем статус на основе состояния Deployment
	mcpServer.Status.Replicas = deployment.Status.Replicas
	mcpServer.Status.ReadyReplicas = deployment.Status.ReadyReplicas
	mcpServer.Status.AvailableReplicas = deployment.Status.AvailableReplicas

	if deployment.Status.ReadyReplicas > 0 {
		mcpServer.Status.Phase = mcpv1.MCPServerPhaseRunning
		mcpServer.Status.Message = "MCP server is running"
		mcpServer.Status.ServiceEndpoint = fmt.Sprintf("%s.%s.svc.cluster.local:%d",
			mcpServer.Name, mcpServer.Namespace, r.getMCPPort(mcpServer))
	} else if deployment.Status.Replicas == 0 {
		mcpServer.Status.Phase = mcpv1.MCPServerPhasePending
		mcpServer.Status.Message = "Waiting for deployment"
	}

	mcpServer.Status.LastUpdateTime = &metav1.Time{Time: time.Now()}
	return r.Status().Update(ctx, mcpServer)
}

// loadServerFromRegistry загружает спецификацию сервера из реестра
func (r *MCPServerReconciler) loadServerFromRegistry(ctx context.Context, mcpServer *mcpv1.MCPServer, serverName string) error {
	// Инициализируем клиенты если их нет
	if r.RegistryClient == nil {
		r.RegistryClient = registry.NewClient()
	}
	if r.Syncer == nil {
		r.Syncer = sync.NewSyncer(r.RegistryClient, "servers")
	}

	// Проверяем, есть ли локальная спецификация
	spec, err := r.Syncer.LoadServerSpec(serverName)
	if err != nil {
		// Если нет локальной спецификации, загружаем из реестра
		if err := r.Syncer.DownloadServerSpec(ctx, serverName); err != nil {
			return fmt.Errorf("failed to download server spec from registry: %w", err)
		}
		// Загружаем снова после скачивания
		spec, err = r.Syncer.LoadServerSpec(serverName)
		if err != nil {
			return fmt.Errorf("failed to load downloaded server spec: %w", err)
		}
	}

	// Обновляем спецификацию MCPServer на основе данных из реестра
	if mcpServer.Spec.Runtime.Image == "" && spec.Runtime.Image != "" {
		mcpServer.Spec.Runtime.Image = spec.Runtime.Image
	}
	if len(mcpServer.Spec.Runtime.Command) == 0 && len(spec.Runtime.Command) > 0 {
		mcpServer.Spec.Runtime.Command = spec.Runtime.Command
	}
	if len(mcpServer.Spec.Runtime.Args) == 0 && len(spec.Runtime.Args) > 0 {
		mcpServer.Spec.Runtime.Args = spec.Runtime.Args
	}

	// Обновляем переменные окружения
	if mcpServer.Spec.Runtime.Env == nil {
		mcpServer.Spec.Runtime.Env = make(map[string]string)
	}
	for k, v := range spec.Runtime.Env {
		if _, exists := mcpServer.Spec.Runtime.Env[k]; !exists {
			mcpServer.Spec.Runtime.Env[k] = v
		}
	}

	// Обновляем описание если оно пустое
	if mcpServer.Spec.Registry.Description == "" {
		mcpServer.Spec.Registry.Description = spec.Description
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MCPServerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Инициализируем клиенты реестра
	r.RegistryClient = registry.NewClient()
	r.Syncer = sync.NewSyncer(r.RegistryClient, "servers")

	return ctrl.NewControllerManagedBy(mgr).
		For(&mcpv1.MCPServer{}).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&autoscalingv2.HorizontalPodAutoscaler{}).
		Owns(&networkingv1.NetworkPolicy{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 5,
			// Rate limiting is handled by the default controller-runtime rate limiter
			// Custom rate limiting can be implemented in the reconcile loop if needed
		}).
		Complete(r)
}
