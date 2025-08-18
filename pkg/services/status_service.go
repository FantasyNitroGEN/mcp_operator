package services

import (
	"context"
	"fmt"
	"time"

	mcpv1 "github.com/FantasyNitroGEN/mcp_operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// DefaultStatusService implements StatusService interface
type DefaultStatusService struct {
	kubernetesClient KubernetesClientService
}

// NewDefaultStatusService creates a new DefaultStatusService
func NewDefaultStatusService(kubernetesClient KubernetesClientService) *DefaultStatusService {
	return &DefaultStatusService{
		kubernetesClient: kubernetesClient,
	}
}

// UpdateMCPServerStatus updates the status of MCPServer
func (s *DefaultStatusService) UpdateMCPServerStatus(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	logger := log.FromContext(ctx).WithValues("mcpserver", mcpServer.Name, "namespace", mcpServer.Namespace)
	logger.V(1).Info("Updating MCPServer status")

	// Update last update time
	now := metav1.Now()
	mcpServer.Status.LastUpdateTime = &now

	// Update the status subresource
	if err := s.kubernetesClient.UpdateStatus(ctx, mcpServer); err != nil {
		logger.Error(err, "Failed to update MCPServer status")
		return fmt.Errorf("failed to update MCPServer status: %w", err)
	}

	logger.V(1).Info("Successfully updated MCPServer status",
		"phase", mcpServer.Status.Phase,
		"replicas", mcpServer.Status.Replicas,
		"readyReplicas", mcpServer.Status.ReadyReplicas)

	return nil
}

// UpdateMCPRegistryStatus updates the status of MCPRegistry
func (s *DefaultStatusService) UpdateMCPRegistryStatus(ctx context.Context, registry *mcpv1.MCPRegistry) error {
	logger := log.FromContext(ctx).WithValues("mcpregistry", registry.Name, "namespace", registry.Namespace)
	logger.V(1).Info("Updating MCPRegistry status")

	// Update last sync time if the registry is in a synced state
	if registry.Status.Phase == mcpv1.MCPRegistryPhaseReady || registry.Status.Phase == mcpv1.MCPRegistryPhaseSyncing {
		now := metav1.Now()
		registry.Status.LastSyncTime = &now
	}

	// Update the status subresource
	if err := s.kubernetesClient.UpdateStatus(ctx, registry); err != nil {
		logger.Error(err, "Failed to update MCPRegistry status")
		return fmt.Errorf("failed to update MCPRegistry status: %w", err)
	}

	logger.V(1).Info("Successfully updated MCPRegistry status",
		"phase", registry.Status.Phase,
		"serversDiscovered", registry.Status.ServersDiscovered,
		"lastSyncTime", registry.Status.LastSyncTime,
		"message", registry.Status.Message)

	return nil
}

// SetCondition sets a condition on MCPServer
func (s *DefaultStatusService) SetCondition(mcpServer *mcpv1.MCPServer, conditionType mcpv1.MCPServerConditionType, status string, reason, message string) {
	logger := log.FromContext(context.Background()).WithValues("mcpserver", mcpServer.Name, "conditionType", conditionType)
	logger.V(1).Info("Setting MCPServer condition", "status", status, "reason", reason)

	condition := mcpv1.MCPServerCondition{
		Type:               conditionType,
		Status:             metav1.ConditionStatus(status),
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}

	// Find existing condition or add new one
	found := false
	for i, existingCondition := range mcpServer.Status.Conditions {
		if existingCondition.Type == conditionType {
			// Update only if status changed
			if existingCondition.Status != condition.Status {
				mcpServer.Status.Conditions[i] = condition
				logger.V(1).Info("Updated existing MCPServer condition", "oldStatus", existingCondition.Status, "newStatus", condition.Status)
			} else {
				// Update message and reason, but not transition time
				mcpServer.Status.Conditions[i].Reason = reason
				mcpServer.Status.Conditions[i].Message = message
				logger.V(1).Info("Updated MCPServer condition message/reason")
			}
			found = true
			break
		}
	}

	if !found {
		mcpServer.Status.Conditions = append(mcpServer.Status.Conditions, condition)
		logger.V(1).Info("Added new MCPServer condition")
	}
}

// SetRegistryCondition sets a condition on MCPRegistry
func (s *DefaultStatusService) SetRegistryCondition(registry *mcpv1.MCPRegistry, conditionType mcpv1.MCPRegistryConditionType, status string, reason, message string) {
	logger := log.FromContext(context.Background()).WithValues("mcpregistry", registry.Name, "conditionType", conditionType)
	logger.V(1).Info("Setting MCPRegistry condition", "status", status, "reason", reason)

	condition := mcpv1.MCPRegistryCondition{
		Type:               conditionType,
		Status:             metav1.ConditionStatus(status),
		LastTransitionTime: metav1.Now(),
		Reason:             reason,
		Message:            message,
	}

	// Find existing condition or add new one
	found := false
	for i, existingCondition := range registry.Status.Conditions {
		if existingCondition.Type == conditionType {
			// Update only if status changed
			if existingCondition.Status != condition.Status {
				registry.Status.Conditions[i] = condition
				logger.V(1).Info("Updated existing MCPRegistry condition", "oldStatus", existingCondition.Status, "newStatus", condition.Status)
			} else {
				// Update message and reason, but not transition time
				registry.Status.Conditions[i].Reason = reason
				registry.Status.Conditions[i].Message = message
				logger.V(1).Info("Updated MCPRegistry condition message/reason")
			}
			found = true
			break
		}
	}

	if !found {
		registry.Status.Conditions = append(registry.Status.Conditions, condition)
		logger.V(1).Info("Added new MCPRegistry condition")
	}
}

// SetMCPServerPhase sets the phase of MCPServer with optional message and reason
func (s *DefaultStatusService) SetMCPServerPhase(mcpServer *mcpv1.MCPServer, phase mcpv1.MCPServerPhase, message, reason string) {
	logger := log.FromContext(context.Background()).WithValues("mcpserver", mcpServer.Name)
	logger.Info("Setting MCPServer phase", "oldPhase", mcpServer.Status.Phase, "newPhase", phase)

	mcpServer.Status.Phase = phase
	if message != "" {
		mcpServer.Status.Message = message
	}
	if reason != "" {
		mcpServer.Status.Reason = reason
	}

	now := metav1.Now()
	mcpServer.Status.LastUpdateTime = &now
}

// SetMCPRegistryPhase sets the phase of MCPRegistry with optional message and reason
func (s *DefaultStatusService) SetMCPRegistryPhase(registry *mcpv1.MCPRegistry, phase mcpv1.MCPRegistryPhase, message, reason string) {
	logger := log.FromContext(context.Background()).WithValues("mcpregistry", registry.Name)
	logger.Info("Setting MCPRegistry phase", "oldPhase", registry.Status.Phase, "newPhase", phase)

	registry.Status.Phase = phase
	if message != "" {
		registry.Status.Message = message
	}
	if reason != "" {
		registry.Status.Reason = reason
	}
}

// GetMCPServerCondition gets a specific condition from MCPServer
func (s *DefaultStatusService) GetMCPServerCondition(mcpServer *mcpv1.MCPServer, conditionType mcpv1.MCPServerConditionType) *mcpv1.MCPServerCondition {
	for _, condition := range mcpServer.Status.Conditions {
		if condition.Type == conditionType {
			return &condition
		}
	}
	return nil
}

// GetMCPRegistryCondition gets a specific condition from MCPRegistry
func (s *DefaultStatusService) GetMCPRegistryCondition(registry *mcpv1.MCPRegistry, conditionType mcpv1.MCPRegistryConditionType) *mcpv1.MCPRegistryCondition {
	for _, condition := range registry.Status.Conditions {
		if condition.Type == conditionType {
			return &condition
		}
	}
	return nil
}

// IsMCPServerConditionTrue checks if a condition is true
func (s *DefaultStatusService) IsMCPServerConditionTrue(mcpServer *mcpv1.MCPServer, conditionType mcpv1.MCPServerConditionType) bool {
	condition := s.GetMCPServerCondition(mcpServer, conditionType)
	return condition != nil && condition.Status == metav1.ConditionTrue
}

// IsMCPRegistryConditionTrue checks if a registry condition is true
func (s *DefaultStatusService) IsMCPRegistryConditionTrue(registry *mcpv1.MCPRegistry, conditionType mcpv1.MCPRegistryConditionType) bool {
	condition := s.GetMCPRegistryCondition(registry, conditionType)
	return condition != nil && condition.Status == metav1.ConditionTrue
}

// UpdateMCPServerFromDeployment updates MCPServer status based on deployment status
func (s *DefaultStatusService) UpdateMCPServerFromDeployment(ctx context.Context, mcpServer *mcpv1.MCPServer, deployment *appsv1.Deployment) error {
	logger := log.FromContext(ctx).WithValues("mcpserver", mcpServer.Name)
	logger.V(1).Info("Updating MCPServer status from deployment")

	// Update replica counts
	mcpServer.Status.Replicas = deployment.Status.Replicas
	mcpServer.Status.ReadyReplicas = deployment.Status.ReadyReplicas
	mcpServer.Status.AvailableReplicas = deployment.Status.AvailableReplicas

	// Update phase based on deployment status
	if deployment.Status.ReadyReplicas > 0 {
		s.SetMCPServerPhase(mcpServer, mcpv1.MCPServerPhaseRunning, "MCP server is running", "DeploymentReady")
		s.SetCondition(mcpServer, mcpv1.MCPServerConditionReady, string(metav1.ConditionTrue), "DeploymentReady", "MCPServer deployment is ready")

		// Set service endpoint if not already set
		if mcpServer.Status.ServiceEndpoint == "" {
			port := int32(8080) // Default port
			if mcpServer.Spec.Runtime.Port > 0 {
				port = mcpServer.Spec.Runtime.Port
			}
			mcpServer.Status.ServiceEndpoint = fmt.Sprintf("%s.%s.svc.cluster.local:%d",
				mcpServer.Name, mcpServer.Namespace, port)
		}
	} else if deployment.Status.Replicas == 0 {
		s.SetMCPServerPhase(mcpServer, mcpv1.MCPServerPhasePending, "Waiting for deployment", "DeploymentScaling")
		s.SetCondition(mcpServer, mcpv1.MCPServerConditionProgressing, string(metav1.ConditionTrue), "DeploymentScaling", "Waiting for deployment to scale up")
	} else {
		s.SetMCPServerPhase(mcpServer, mcpv1.MCPServerPhasePending, "Deployment in progress", "DeploymentProgressing")
		s.SetCondition(mcpServer, mcpv1.MCPServerConditionProgressing, string(metav1.ConditionTrue), "DeploymentProgressing", "Deployment is progressing")
	}

	// Check for deployment conditions
	for _, deploymentCondition := range deployment.Status.Conditions {
		switch deploymentCondition.Type {
		case appsv1.DeploymentProgressing:
			if deploymentCondition.Status == corev1.ConditionFalse {
				s.SetCondition(mcpServer, mcpv1.MCPServerConditionProgressing, string(metav1.ConditionFalse),
					deploymentCondition.Reason, deploymentCondition.Message)
			}
		case appsv1.DeploymentReplicaFailure:
			if deploymentCondition.Status == corev1.ConditionTrue {
				s.SetMCPServerPhase(mcpServer, mcpv1.MCPServerPhaseFailed, deploymentCondition.Message, deploymentCondition.Reason)
				s.SetCondition(mcpServer, mcpv1.MCPServerConditionReady, string(metav1.ConditionFalse),
					deploymentCondition.Reason, deploymentCondition.Message)
			}
		}
	}

	return s.UpdateMCPServerStatus(ctx, mcpServer)
}

// ClearMCPServerConditions clears all conditions from MCPServer
func (s *DefaultStatusService) ClearMCPServerConditions(mcpServer *mcpv1.MCPServer) {
	mcpServer.Status.Conditions = []mcpv1.MCPServerCondition{}
}

// ClearMCPRegistryConditions clears all conditions from MCPRegistry
func (s *DefaultStatusService) ClearMCPRegistryConditions(registry *mcpv1.MCPRegistry) {
	registry.Status.Conditions = []mcpv1.MCPRegistryCondition{}
}

// CheckDeploymentTimeout checks if an MCPServer has been stuck in Pending/Progressing state
// for longer than the specified timeout duration. Returns true if timeout exceeded.
func (s *DefaultStatusService) CheckDeploymentTimeout(mcpServer *mcpv1.MCPServer, timeout time.Duration) bool {
	logger := log.FromContext(context.Background()).WithValues("mcpserver", mcpServer.Name)

	// Only check timeout for servers in Pending phase
	if mcpServer.Status.Phase != mcpv1.MCPServerPhasePending {
		return false
	}

	// Get the Progressing condition to check when deployment started
	progressingCondition := s.GetMCPServerCondition(mcpServer, mcpv1.MCPServerConditionProgressing)
	if progressingCondition == nil {
		logger.V(1).Info("No Progressing condition found, skipping timeout check")
		return false
	}

	// Only check timeout if the condition is True (deployment is progressing)
	if progressingCondition.Status != metav1.ConditionTrue {
		return false
	}

	// Calculate how long the deployment has been progressing
	now := time.Now()
	progressingDuration := now.Sub(progressingCondition.LastTransitionTime.Time)

	logger.V(1).Info("Checking deployment timeout",
		"progressingDuration", progressingDuration,
		"timeout", timeout,
		"lastTransitionTime", progressingCondition.LastTransitionTime.Time,
	)

	// Return true if timeout exceeded
	if progressingDuration > timeout {
		logger.Info("Deployment timeout detected",
			"progressingDuration", progressingDuration,
			"timeout", timeout,
			"reason", progressingCondition.Reason,
		)
		return true
	}

	return false
}

// MarkDeploymentAsTimedOut marks an MCPServer as failed due to deployment timeout
func (s *DefaultStatusService) MarkDeploymentAsTimedOut(mcpServer *mcpv1.MCPServer, timeout time.Duration) {
	logger := log.FromContext(context.Background()).WithValues("mcpserver", mcpServer.Name)
	logger.Info("Marking MCPServer as failed due to deployment timeout", "timeout", timeout)

	// Set phase to Failed
	s.SetMCPServerPhase(mcpServer, mcpv1.MCPServerPhaseFailed,
		fmt.Sprintf("Deployment timeout exceeded (%v)", timeout), "DeploymentTimeout")

	// Set Ready condition to False
	s.SetCondition(mcpServer, mcpv1.MCPServerConditionReady,
		string(metav1.ConditionFalse), "DeploymentTimeout",
		fmt.Sprintf("Deployment has been stuck for %v, exceeding timeout of %v", timeout, timeout))

	// Set Progressing condition to False
	s.SetCondition(mcpServer, mcpv1.MCPServerConditionProgressing,
		string(metav1.ConditionFalse), "DeploymentTimeout",
		fmt.Sprintf("Deployment timeout exceeded after %v", timeout))
}
