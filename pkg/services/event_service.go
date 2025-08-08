package services

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/record"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// DefaultEventService implements EventService interface
type DefaultEventService struct {
	recorder record.EventRecorder
}

// NewDefaultEventService creates a new DefaultEventService
func NewDefaultEventService(recorder record.EventRecorder) *DefaultEventService {
	return &DefaultEventService{
		recorder: recorder,
	}
}

// RecordEvent records a Kubernetes event
func (e *DefaultEventService) RecordEvent(obj client.Object, eventType, reason, message string) {
	logger := log.Log.WithValues("object", fmt.Sprintf("%s/%s", obj.GetNamespace(), obj.GetName()), "eventType", eventType, "reason", reason)
	logger.V(1).Info("Recording event", "message", message)

	if e.recorder == nil {
		logger.Error(nil, "Event recorder is not initialized")
		return
	}

	e.recorder.Event(obj, eventType, reason, message)
	logger.V(1).Info("Event recorded successfully")
}

// RecordWarning records a warning event
func (e *DefaultEventService) RecordWarning(obj client.Object, reason, message string) {
	logger := log.Log.WithValues("object", fmt.Sprintf("%s/%s", obj.GetNamespace(), obj.GetName()), "reason", reason)
	logger.Info("Recording warning event", "message", message)

	e.RecordEvent(obj, corev1.EventTypeWarning, reason, message)
}

// RecordNormal records a normal event
func (e *DefaultEventService) RecordNormal(obj client.Object, reason, message string) {
	logger := log.Log.WithValues("object", fmt.Sprintf("%s/%s", obj.GetNamespace(), obj.GetName()), "reason", reason)
	logger.V(1).Info("Recording normal event", "message", message)

	e.RecordEvent(obj, corev1.EventTypeNormal, reason, message)
}

// RecordEventf records a formatted event
func (e *DefaultEventService) RecordEventf(obj client.Object, eventType, reason, messageFmt string, args ...interface{}) {
	message := fmt.Sprintf(messageFmt, args...)
	e.RecordEvent(obj, eventType, reason, message)
}

// RecordWarningf records a formatted warning event
func (e *DefaultEventService) RecordWarningf(obj client.Object, reason, messageFmt string, args ...interface{}) {
	message := fmt.Sprintf(messageFmt, args...)
	e.RecordWarning(obj, reason, message)
}

// RecordNormalf records a formatted normal event
func (e *DefaultEventService) RecordNormalf(obj client.Object, reason, messageFmt string, args ...interface{}) {
	message := fmt.Sprintf(messageFmt, args...)
	e.RecordNormal(obj, reason, message)
}

// RecordDeploymentEvent records deployment-related events
func (e *DefaultEventService) RecordDeploymentEvent(obj client.Object, eventType, phase, message string) {
	reason := fmt.Sprintf("Deployment%s", phase)
	e.RecordEvent(obj, eventType, reason, message)
}

// RecordRegistryEvent records registry-related events
func (e *DefaultEventService) RecordRegistryEvent(obj client.Object, eventType, operation, message string) {
	reason := fmt.Sprintf("Registry%s", operation)
	e.RecordEvent(obj, eventType, reason, message)
}

// RecordValidationEvent records validation-related events
func (e *DefaultEventService) RecordValidationEvent(obj client.Object, eventType, validationType, message string) {
	reason := fmt.Sprintf("Validation%s", validationType)
	e.RecordEvent(obj, eventType, reason, message)
}

// RecordScalingEvent records scaling-related events
func (e *DefaultEventService) RecordScalingEvent(obj client.Object, eventType, direction, message string) {
	reason := fmt.Sprintf("Scaling%s", direction)
	e.RecordEvent(obj, eventType, reason, message)
}

// RecordNetworkPolicyEvent records network policy-related events
func (e *DefaultEventService) RecordNetworkPolicyEvent(obj client.Object, eventType, action, message string) {
	reason := fmt.Sprintf("NetworkPolicy%s", action)
	e.RecordEvent(obj, eventType, reason, message)
}

// RecordTenancyEvent records tenancy-related events
func (e *DefaultEventService) RecordTenancyEvent(obj client.Object, eventType, action, message string) {
	reason := fmt.Sprintf("Tenancy%s", action)
	e.RecordEvent(obj, eventType, reason, message)
}

// RecordResourceQuotaEvent records resource quota-related events
func (e *DefaultEventService) RecordResourceQuotaEvent(obj client.Object, eventType, action, message string) {
	reason := fmt.Sprintf("ResourceQuota%s", action)
	e.RecordEvent(obj, eventType, reason, message)
}

// RecordAutoscalingEvent records autoscaling-related events
func (e *DefaultEventService) RecordAutoscalingEvent(obj client.Object, eventType, scalerType, message string) {
	reason := fmt.Sprintf("Autoscaling%s", scalerType)
	e.RecordEvent(obj, eventType, reason, message)
}

// RecordStatusEvent records status update events
func (e *DefaultEventService) RecordStatusEvent(obj client.Object, eventType, statusType, message string) {
	reason := fmt.Sprintf("Status%s", statusType)
	e.RecordEvent(obj, eventType, reason, message)
}

// RecordConfigurationEvent records configuration-related events
func (e *DefaultEventService) RecordConfigurationEvent(obj client.Object, eventType, configType, message string) {
	reason := fmt.Sprintf("Configuration%s", configType)
	e.RecordEvent(obj, eventType, reason, message)
}

// RecordHealthEvent records health check events
func (e *DefaultEventService) RecordHealthEvent(obj client.Object, eventType, healthStatus, message string) {
	reason := fmt.Sprintf("Health%s", healthStatus)
	e.RecordEvent(obj, eventType, reason, message)
}

// RecordSecurityEvent records security-related events
func (e *DefaultEventService) RecordSecurityEvent(obj client.Object, eventType, securityAction, message string) {
	reason := fmt.Sprintf("Security%s", securityAction)
	e.RecordEvent(obj, eventType, reason, message)
}

// RecordBackupEvent records backup-related events
func (e *DefaultEventService) RecordBackupEvent(obj client.Object, eventType, backupAction, message string) {
	reason := fmt.Sprintf("Backup%s", backupAction)
	e.RecordEvent(obj, eventType, reason, message)
}

// RecordUpgradeEvent records upgrade-related events
func (e *DefaultEventService) RecordUpgradeEvent(obj client.Object, eventType, upgradePhase, message string) {
	reason := fmt.Sprintf("Upgrade%s", upgradePhase)
	e.RecordEvent(obj, eventType, reason, message)
}

// GetEventRecorder returns the underlying event recorder
func (e *DefaultEventService) GetEventRecorder() record.EventRecorder {
	return e.recorder
}

// SetEventRecorder sets the event recorder (useful for testing)
func (e *DefaultEventService) SetEventRecorder(recorder record.EventRecorder) {
	e.recorder = recorder
}

// IsRecorderInitialized checks if the event recorder is initialized
func (e *DefaultEventService) IsRecorderInitialized() bool {
	return e.recorder != nil
}
