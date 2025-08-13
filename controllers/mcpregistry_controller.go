package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/record"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	mcpv1 "github.com/FantasyNitroGEN/mcp_operator/api/v1"
	"github.com/FantasyNitroGEN/mcp_operator/pkg/registry"
	"github.com/FantasyNitroGEN/mcp_operator/pkg/registry/cache"
	"github.com/FantasyNitroGEN/mcp_operator/pkg/registry/github"
)

// MCPRegistryReconciler reconciles a MCPRegistry object
type MCPRegistryReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	Recorder record.EventRecorder
}

// +kubebuilder:rbac:groups=mcp.allbeone.io,resources=mcpregistries,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups=mcp.allbeone.io,resources=mcpregistries/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=mcp.allbeone.io,resources=mcpregistries/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;create;update;patch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
func (r *MCPRegistryReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("mcpregistry", req.NamespacedName)
	logger.Info("Starting reconciliation")

	// Fetch the MCPRegistry instance
	mcpRegistry := &mcpv1.MCPRegistry{}
	if err := r.Get(ctx, req.NamespacedName, mcpRegistry); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("MCPRegistry resource not found, ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		logger.Error(err, "Failed to get MCPRegistry")
		return ctrl.Result{}, err
	}

	// Handle deletion
	if mcpRegistry.DeletionTimestamp != nil {
		return r.handleDeletion(ctx, logger, mcpRegistry)
	}

	// Validate that source is configured
	if mcpRegistry.Spec.Source == nil || mcpRegistry.Spec.Source.Type != "github" || mcpRegistry.Spec.Source.Github == nil {
		err := fmt.Errorf("only GitHub source type is supported currently")
		logger.Error(err, "Invalid source configuration")
		r.setCondition(mcpRegistry, mcpv1.MCPRegistryConditionReachable, metav1.ConditionFalse, "InvalidSource", err.Error())
		r.setCondition(mcpRegistry, mcpv1.MCPRegistryConditionSynced, metav1.ConditionFalse, "InvalidSource", err.Error())
		r.Recorder.Event(mcpRegistry, corev1.EventTypeWarning, "ValidationFailed", err.Error())
		if statusErr := r.updateStatus(ctx, mcpRegistry); statusErr != nil {
			logger.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	}

	// Set syncing condition
	r.setCondition(mcpRegistry, mcpv1.MCPRegistryConditionSyncing, metav1.ConditionTrue, "SyncInProgress", "Registry synchronization in progress")
	mcpRegistry.Status.Phase = mcpv1.MCPRegistryPhaseSyncing

	if err := r.updateStatus(ctx, mcpRegistry); err != nil {
		logger.Error(err, "Failed to update status to syncing")
		return ctrl.Result{}, err
	}

	// Get authentication token if configured
	token, err := r.getAuthToken(ctx, mcpRegistry)
	if err != nil {
		logger.Error(err, "Failed to get authentication token")
		r.setCondition(mcpRegistry, mcpv1.MCPRegistryConditionAuthFailed, metav1.ConditionTrue, "AuthTokenFailed", err.Error())
		r.setCondition(mcpRegistry, mcpv1.MCPRegistryConditionReachable, metav1.ConditionFalse, "AuthTokenFailed", err.Error())
		r.setCondition(mcpRegistry, mcpv1.MCPRegistryConditionSynced, metav1.ConditionFalse, "AuthTokenFailed", err.Error())
		r.Recorder.Event(mcpRegistry, corev1.EventTypeWarning, "AuthFailed", err.Error())
		if statusErr := r.updateStatus(ctx, mcpRegistry); statusErr != nil {
			logger.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: time.Minute * 5}, nil
	}

	// Create GitHub client and perform synchronization
	registryClient := registry.NewClient()
	if token != "" {
		registryClient = registry.NewClientWithToken(token)
		logger.V(1).Info("Using authenticated GitHub client")
	} else {
		logger.V(1).Info("Using anonymous GitHub client")
	}

	githubClient := github.NewGitHubRegistryClient(registryClient)

	// Add token to context for GitHub client
	if token != "" {
		ctx = context.WithValue(ctx, registry.AuthTokenContextKey, token)
	}

	// Perform synchronization
	syncResult, err := githubClient.SyncRegistry(ctx, mcpRegistry)
	if err != nil {
		logger.Error(err, "Failed to sync registry")
		r.setCondition(mcpRegistry, mcpv1.MCPRegistryConditionSyncing, metav1.ConditionFalse, "SyncFailed", err.Error())
		r.setCondition(mcpRegistry, mcpv1.MCPRegistryConditionReachable, metav1.ConditionFalse, "SyncFailed", err.Error())
		r.setCondition(mcpRegistry, mcpv1.MCPRegistryConditionSynced, metav1.ConditionFalse, "SyncFailed", err.Error())
		r.Recorder.Event(mcpRegistry, corev1.EventTypeWarning, "SyncFailed", err.Error())
		mcpRegistry.Status.Phase = mcpv1.MCPRegistryPhaseFailed
		if statusErr := r.updateStatus(ctx, mcpRegistry); statusErr != nil {
			logger.Error(statusErr, "Failed to update status")
		}
		return ctrl.Result{RequeueAfter: time.Minute * 15}, nil
	}

	// Create cache manager and cache servers
	cacheManager := cache.NewCacheManager(r.Client)
	var cacheErrors []string

	for _, server := range syncResult.Servers {
		if serverSpec, ok := syncResult.ServerSpecs[server.Name]; ok {
			err := cacheManager.CacheServer(ctx, mcpRegistry.Name, &server, serverSpec, mcpRegistry)
			if err != nil {
				cacheError := fmt.Sprintf("failed to cache server %s: %v", server.Name, err)
				cacheErrors = append(cacheErrors, cacheError)
				logger.Error(err, "Failed to cache server", "server", server.Name)
			} else {
				logger.V(1).Info("Successfully cached server", "server", server.Name)
			}
		}
	}

	// Create registry index and cache it
	registryIndex := &registry.RegistryIndex{
		Name:         mcpRegistry.Name,
		Description:  fmt.Sprintf("MCP Registry index for %s", mcpRegistry.Name),
		LastUpdated:  time.Now(),
		ServersCount: syncResult.ServersCount,
		Servers:      syncResult.Servers,
		Metadata: map[string]string{
			"source": mcpRegistry.Spec.Source.Github.Repo,
			"branch": mcpRegistry.Spec.Source.Github.Branch,
			"path":   mcpRegistry.Spec.Source.Path,
		},
	}

	// Cache the registry index
	if err := cacheManager.CacheIndex(ctx, mcpRegistry.Name, registryIndex, mcpRegistry); err != nil {
		logger.Error(err, "Failed to cache registry index")
		cacheErrors = append(cacheErrors, fmt.Sprintf("failed to cache registry index: %v", err))
	}

	// Convert MCPServerInfo to RegistryServer for status
	var registryServers []mcpv1.RegistryServer
	for _, server := range syncResult.Servers {
		registryServer := mcpv1.RegistryServer{
			Name: server.Name,
			Path: server.Path,
		}

		// Add additional info from server spec if available
		if serverSpec, ok := syncResult.ServerSpecs[server.Name]; ok {
			registryServer.Title = serverSpec.Name
			registryServer.Description = serverSpec.Description
			registryServer.Version = serverSpec.Version
			if len(serverSpec.Keywords) > 0 {
				registryServer.Tags = serverSpec.Keywords
			}
		}

		registryServers = append(registryServers, registryServer)
	}

	// Update status with sync results
	now := metav1.Now()
	mcpRegistry.Status.LastSyncTime = &now
	mcpRegistry.Status.ServersDiscovered = syncResult.ServersCount
	mcpRegistry.Status.ObservedRevision = syncResult.ObservedSHA
	mcpRegistry.Status.ObservedGeneration = mcpRegistry.Generation
	mcpRegistry.Status.Servers = registryServers
	mcpRegistry.Status.Phase = mcpv1.MCPRegistryPhaseReady

	// Update errors in status
	allErrors := append(syncResult.Errors, cacheErrors...)
	if len(allErrors) > 10 {
		allErrors = allErrors[:10] // Keep only last 10 errors
	}
	mcpRegistry.Status.Errors = allErrors

	// Update rate limit info if available
	var rateLimitedRequeue *time.Duration
	if syncResult.RateLimitInfo != nil {
		mcpRegistry.Status.RateLimitRemaining = &syncResult.RateLimitInfo.Remaining
		if syncResult.RateLimitInfo.Remaining < 100 {
			r.setCondition(mcpRegistry, mcpv1.MCPRegistryConditionRateLimited, metav1.ConditionTrue, "RateLimited",
				fmt.Sprintf("GitHub API rate limit low: %d remaining, resets at %s",
					syncResult.RateLimitInfo.Remaining, syncResult.RateLimitInfo.Reset.Format(time.RFC3339)))
			r.Recorder.Event(mcpRegistry, corev1.EventTypeNormal, "RateLimited",
				fmt.Sprintf("GitHub API rate limit low: %d remaining", syncResult.RateLimitInfo.Remaining))

			// Calculate RetryAfter based on rate limit reset time
			resetDelay := time.Until(syncResult.RateLimitInfo.Reset)
			if resetDelay > 0 {
				// Add buffer time to ensure reset has occurred
				retryAfter := resetDelay + time.Minute*2
				rateLimitedRequeue = &retryAfter
				logger.V(1).Info("Rate limit hit, will retry after reset",
					"retryAfter", retryAfter,
					"resetTime", syncResult.RateLimitInfo.Reset)
			}
		} else {
			r.setCondition(mcpRegistry, mcpv1.MCPRegistryConditionRateLimited, metav1.ConditionFalse, "RateLimitOK", "GitHub API rate limit is sufficient")
		}
	}

	// Set successful conditions
	r.setCondition(mcpRegistry, mcpv1.MCPRegistryConditionSyncing, metav1.ConditionFalse, "SyncCompleted", "Registry synchronization completed")
	r.setCondition(mcpRegistry, mcpv1.MCPRegistryConditionReachable, metav1.ConditionTrue, "OK", "Registry is reachable")
	r.setCondition(mcpRegistry, mcpv1.MCPRegistryConditionSynced, metav1.ConditionTrue, "OK",
		fmt.Sprintf("Registry synchronized successfully, %d servers discovered", syncResult.ServersCount))
	r.setCondition(mcpRegistry, mcpv1.MCPRegistryConditionAuthFailed, metav1.ConditionFalse, "AuthSuccessful", "Authentication successful")

	r.Recorder.Event(mcpRegistry, corev1.EventTypeNormal, "Synced",
		fmt.Sprintf("Registry synchronized successfully, %d servers discovered", syncResult.ServersCount))

	// Update final status
	if err := r.updateStatus(ctx, mcpRegistry); err != nil {
		logger.Error(err, "Failed to update final status")
		return ctrl.Result{}, err
	}

	// Calculate next sync interval - use rate limit retry delay if rate limited, otherwise use RefreshInterval
	var nextRequeue time.Duration
	if rateLimitedRequeue != nil {
		nextRequeue = *rateLimitedRequeue
		logger.Info("Using rate limit retry delay for next reconciliation",
			"retryAfter", nextRequeue)
	} else {
		nextRequeue = time.Minute * 15 // Default 15m as per issue requirements
		if mcpRegistry.Spec.RefreshInterval != nil {
			nextRequeue = mcpRegistry.Spec.RefreshInterval.Duration
		}
	}

	logger.Info("Registry reconciliation completed successfully",
		"phase", mcpRegistry.Status.Phase,
		"serversDiscovered", mcpRegistry.Status.ServersDiscovered,
		"nextSync", nextRequeue)

	return ctrl.Result{RequeueAfter: nextRequeue}, nil
}

// handleDeletion handles the deletion of a MCPRegistry
func (r *MCPRegistryReconciler) handleDeletion(ctx context.Context, logger logr.Logger, mcpRegistry *mcpv1.MCPRegistry) (ctrl.Result, error) {
	logger.Info("Handling MCPRegistry deletion")

	// Clean up associated ConfigMaps
	cacheManager := cache.NewCacheManager(r.Client)
	if err := cacheManager.CleanupRegistry(ctx, mcpRegistry.Name, mcpRegistry.Namespace); err != nil {
		logger.Error(err, "Failed to cleanup registry ConfigMaps")
		// Continue with deletion even if cleanup fails
	}

	logger.Info("MCPRegistry deletion handled successfully")
	return ctrl.Result{}, nil
}

// getAuthToken retrieves the authentication token from the secret if configured
func (r *MCPRegistryReconciler) getAuthToken(ctx context.Context, mcpRegistry *mcpv1.MCPRegistry) (string, error) {
	if mcpRegistry.Spec.Auth == nil || mcpRegistry.Spec.Auth.SecretRef == nil {
		return "", nil // No authentication configured
	}

	secretRef := mcpRegistry.Spec.Auth.SecretRef
	secret := &corev1.Secret{}

	err := r.Get(ctx, types.NamespacedName{
		Name:      secretRef.Name,
		Namespace: mcpRegistry.Namespace,
	}, secret)

	if err != nil {
		if errors.IsNotFound(err) && secretRef.Optional {
			return "", nil // Optional secret not found
		}
		return "", fmt.Errorf("failed to get secret %s: %w", secretRef.Name, err)
	}

	token, ok := secret.Data[secretRef.Key]
	if !ok {
		return "", fmt.Errorf("key %s not found in secret %s", secretRef.Key, secretRef.Name)
	}

	return string(token), nil
}

// setCondition sets a condition on the MCPRegistry status
func (r *MCPRegistryReconciler) setCondition(mcpRegistry *mcpv1.MCPRegistry, conditionType mcpv1.MCPRegistryConditionType, status metav1.ConditionStatus, reason, message string) {
	now := metav1.Now()
	condition := mcpv1.MCPRegistryCondition{
		Type:               conditionType,
		Status:             status,
		LastTransitionTime: now,
		Reason:             reason,
		Message:            message,
	}

	// Find existing condition and update it
	for i, existingCondition := range mcpRegistry.Status.Conditions {
		if existingCondition.Type == conditionType {
			if existingCondition.Status != status {
				condition.LastTransitionTime = now
			} else {
				condition.LastTransitionTime = existingCondition.LastTransitionTime
			}
			mcpRegistry.Status.Conditions[i] = condition
			return
		}
	}

	// Condition not found, add it
	mcpRegistry.Status.Conditions = append(mcpRegistry.Status.Conditions, condition)
}

// updateStatus updates the MCPRegistry status
func (r *MCPRegistryReconciler) updateStatus(ctx context.Context, mcpRegistry *mcpv1.MCPRegistry) error {
	return r.Status().Update(ctx, mcpRegistry)
}

// SetupWithManager sets up the controller with the Manager.
func (r *MCPRegistryReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&mcpv1.MCPRegistry{}).
		Owns(&corev1.ConfigMap{}).
		Complete(r)
}
