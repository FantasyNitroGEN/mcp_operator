package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/log"

	mcpv1 "github.com/FantasyNitroGEN/mcp_operator/api/v1"
	"github.com/FantasyNitroGEN/mcp_operator/pkg/services"
)

// MCPRegistryReconciler reconciles a MCPRegistry object
type MCPRegistryReconciler struct {
	client.Client
	Scheme *runtime.Scheme

	// Service dependencies
	RegistryService   services.RegistryService
	StatusService     services.StatusService
	ValidationService services.ValidationService
	RetryService      services.RetryService
	EventService      services.EventService
}

// +kubebuilder:rbac:groups=mcp.allbeone.io,resources=mcpregistries,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mcp.allbeone.io,resources=mcpregistries/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=mcp.allbeone.io,resources=mcpregistries/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *MCPRegistryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("mcpregistry", req.NamespacedName)
	logger.Info("Starting reconciliation")

	// Fetch the MCPRegistry instance
	registry := &mcpv1.MCPRegistry{}
	if err := r.Get(ctx, req.NamespacedName, registry); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("MCPRegistry resource not found, ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get MCPRegistry")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if registry.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, logger, registry)
	}

	// Update metrics
	// TODO: Add metrics when metrics package is updated
	// metrics.RegistryReconciliations.WithLabelValues(registry.Name, registry.Namespace).Inc()

	// Validate registry specification
	if err := r.ValidationService.ValidateRegistry(ctx, registry); err != nil {
		logger.Error(err, "Registry validation failed")
		r.StatusService.SetRegistryCondition(registry, mcpv1.MCPRegistryConditionReady,
			string(metav1.ConditionFalse), "ValidationFailed", fmt.Sprintf("Registry validation failed: %v", err))
		r.EventService.RecordWarning(registry, "ValidationFailed", fmt.Sprintf("Registry validation failed: %v", err))

		if statusErr := r.StatusService.UpdateMCPRegistryStatus(ctx, registry); statusErr != nil {
			logger.Error(statusErr, "Failed to update registry status")
		}
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	}

	// Set registry phase to syncing
	registry.Status.Phase = mcpv1.MCPRegistryPhaseSyncing
	r.StatusService.SetRegistryCondition(registry, mcpv1.MCPRegistryConditionSynced,
		string(metav1.ConditionUnknown), "SyncInProgress", "Registry synchronization in progress")

	// Validate registry connection with retry
	if err := r.RetryService.RetryRegistryOperation(ctx, func() error {
		return r.RegistryService.ValidateRegistryConnection(ctx, registry)
	}); err != nil {
		logger.Error(err, "Failed to validate registry connection")
		registry.Status.Phase = mcpv1.MCPRegistryPhaseFailed
		r.StatusService.SetRegistryCondition(registry, mcpv1.MCPRegistryConditionAuthenticated,
			string(metav1.ConditionFalse), "ConnectionFailed", fmt.Sprintf("Failed to connect to registry: %v", err))
		r.EventService.RecordWarning(registry, "ConnectionFailed", fmt.Sprintf("Failed to connect to registry: %v", err))

		if statusErr := r.StatusService.UpdateMCPRegistryStatus(ctx, registry); statusErr != nil {
			logger.Error(statusErr, "Failed to update registry status")
		}
		return ctrl.Result{RequeueAfter: time.Minute * 10}, nil
	}

	// Set authentication condition to true
	r.StatusService.SetRegistryCondition(registry, mcpv1.MCPRegistryConditionAuthenticated,
		string(metav1.ConditionTrue), "ConnectionSuccessful", "Successfully connected to registry")

	// Sync registry data with retry
	if err := r.RetryService.RetryRegistryOperation(ctx, func() error {
		return r.RegistryService.SyncRegistry(ctx, registry)
	}); err != nil {
		logger.Error(err, "Failed to sync registry")
		registry.Status.Phase = mcpv1.MCPRegistryPhaseFailed
		r.StatusService.SetRegistryCondition(registry, mcpv1.MCPRegistryConditionSynced,
			string(metav1.ConditionFalse), "SyncFailed", fmt.Sprintf("Failed to sync registry: %v", err))
		r.EventService.RecordWarning(registry, "SyncFailed", fmt.Sprintf("Failed to sync registry: %v", err))

		if statusErr := r.StatusService.UpdateMCPRegistryStatus(ctx, registry); statusErr != nil {
			logger.Error(statusErr, "Failed to update registry status")
		}
		return ctrl.Result{RequeueAfter: time.Minute * 15}, nil
	}

	// Update registry status to ready
	registry.Status.Phase = mcpv1.MCPRegistryPhaseReady
	now := metav1.Now()
	registry.Status.LastSyncTime = &now

	r.StatusService.SetRegistryCondition(registry, mcpv1.MCPRegistryConditionReady,
		string(metav1.ConditionTrue), "SyncSuccessful", "Registry synchronized successfully")
	r.StatusService.SetRegistryCondition(registry, mcpv1.MCPRegistryConditionSynced,
		string(metav1.ConditionTrue), "SyncSuccessful", "Registry synchronized successfully")

	r.EventService.RecordNormal(registry, "SyncSuccessful", "Registry synchronized successfully")

	// Update status
	if err := r.StatusService.UpdateMCPRegistryStatus(ctx, registry); err != nil {
		logger.Error(err, "Failed to update registry status")
		return ctrl.Result{}, err
	}

	// Calculate next sync time
	syncInterval := time.Hour * 24 // Default sync interval
	if registry.Spec.SyncInterval != nil {
		syncInterval = registry.Spec.SyncInterval.Duration
	}

	logger.Info("Registry reconciliation completed successfully",
		"phase", registry.Status.Phase,
		"availableServers", registry.Status.AvailableServers,
		"nextSync", syncInterval)

	return ctrl.Result{RequeueAfter: syncInterval}, nil
}

// handleDeletion handles the deletion of MCPRegistry
func (r *MCPRegistryReconciler) handleDeletion(ctx context.Context, logger logr.Logger, registry *mcpv1.MCPRegistry) (ctrl.Result, error) {
	logger.Info("Handling MCPRegistry deletion")

	// Perform cleanup operations here if needed
	// For example, cleanup cached registry data, notify dependent MCPServers, etc.

	r.EventService.RecordNormal(registry, "Deleted", "MCPRegistry deleted successfully")

	logger.Info("MCPRegistry deletion completed")
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *MCPRegistryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mcpv1.MCPRegistry{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 3, // Lower concurrency for registry operations
		}).
		Complete(r)
}

// GetRegistryByName retrieves a registry by name from the cluster
func (r *MCPRegistryReconciler) GetRegistryByName(ctx context.Context, name, namespace string) (*mcpv1.MCPRegistry, error) {
	registry := &mcpv1.MCPRegistry{}
	key := types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}

	if err := r.Get(ctx, key, registry); err != nil {
		return nil, err
	}

	return registry, nil
}

// ListRegistries lists all registries in the cluster
func (r *MCPRegistryReconciler) ListRegistries(ctx context.Context, namespace string) (*mcpv1.MCPRegistryList, error) {
	registryList := &mcpv1.MCPRegistryList{}
	listOpts := []client.ListOption{}

	if namespace != "" {
		listOpts = append(listOpts, client.InNamespace(namespace))
	}

	if err := r.List(ctx, registryList, listOpts...); err != nil {
		return nil, err
	}

	return registryList, nil
}

// TriggerRegistrySync manually triggers a registry sync
func (r *MCPRegistryReconciler) TriggerRegistrySync(ctx context.Context, registry *mcpv1.MCPRegistry) error {
	logger := log.FromContext(ctx).WithValues("mcpregistry", registry.Name)
	logger.Info("Manually triggering registry sync")

	// Update annotation to trigger reconciliation
	if registry.Annotations == nil {
		registry.Annotations = make(map[string]string)
	}
	registry.Annotations["mcp.allbeone.io/sync-trigger"] = time.Now().Format(time.RFC3339)

	return r.Update(ctx, registry)
}
