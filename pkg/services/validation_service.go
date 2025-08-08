package services

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	mcpv1 "github.com/FantasyNitroGEN/mcp_operator/api/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/labels"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// DefaultValidationService implements ValidationService interface
type DefaultValidationService struct {
	kubernetesClient KubernetesClientService
}

// NewDefaultValidationService creates a new DefaultValidationService
func NewDefaultValidationService(kubernetesClient KubernetesClientService) *DefaultValidationService {
	return &DefaultValidationService{
		kubernetesClient: kubernetesClient,
	}
}

// ValidateMCPServer validates MCPServer specification
func (v *DefaultValidationService) ValidateMCPServer(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	logger := log.FromContext(ctx).WithValues("mcpserver", mcpServer.Name, "namespace", mcpServer.Namespace)
	logger.V(1).Info("Validating MCPServer specification")

	// Validate registry information
	if err := v.validateRegistryInfo(mcpServer.Spec.Registry); err != nil {
		return fmt.Errorf("registry validation failed: %w", err)
	}

	// Validate runtime specification
	if err := v.validateRuntimeSpec(mcpServer.Spec.Runtime); err != nil {
		return fmt.Errorf("runtime validation failed: %w", err)
	}

	// Validate resource requirements
	if err := v.validateResourceRequirements(mcpServer.Spec.Resources); err != nil {
		return fmt.Errorf("resource requirements validation failed: %w", err)
	}

	// Validate replicas
	if mcpServer.Spec.Replicas != nil && *mcpServer.Spec.Replicas < 0 {
		return fmt.Errorf("replicas cannot be negative")
	}

	// Validate autoscaling configuration
	if mcpServer.Spec.Autoscaling != nil {
		if err := v.validateAutoscalingSpec(*mcpServer.Spec.Autoscaling); err != nil {
			return fmt.Errorf("autoscaling validation failed: %w", err)
		}
	}

	// Validate tenancy configuration
	if mcpServer.Spec.Tenancy != nil {
		if err := v.validateTenancySpec(*mcpServer.Spec.Tenancy); err != nil {
			return fmt.Errorf("tenancy validation failed: %w", err)
		}
	}

	logger.V(1).Info("MCPServer validation completed successfully")
	return nil
}

// ValidateTenantResourceQuotas validates tenant resource quotas
func (v *DefaultValidationService) ValidateTenantResourceQuotas(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	logger := log.FromContext(ctx).WithValues("mcpserver", mcpServer.Name, "namespace", mcpServer.Namespace)
	logger.V(1).Info("Validating tenant resource quotas")

	if mcpServer.Spec.Tenancy == nil || mcpServer.Spec.Tenancy.ResourceQuotas == nil {
		return nil // No tenancy configuration or quotas, nothing to validate
	}

	tenancy := mcpServer.Spec.Tenancy
	quotas := tenancy.ResourceQuotas

	logger.Info("Validating tenant resource quotas", "tenant", tenancy.TenantID)

	// Validate basic quota specification
	if err := v.validateTenantResourceQuotas(*quotas); err != nil {
		return fmt.Errorf("resource quotas validation failed: %w", err)
	}

	// Validate CPU quota against current usage
	if quotas.CPU != nil {
		if err := v.validateCPUQuota(ctx, mcpServer, quotas.CPU); err != nil {
			return fmt.Errorf("CPU quota validation failed: %w", err)
		}
	}

	// Validate Memory quota against current usage
	if quotas.Memory != nil {
		if err := v.validateMemoryQuota(ctx, mcpServer, quotas.Memory); err != nil {
			return fmt.Errorf("memory quota validation failed: %w", err)
		}
	}

	// Validate Pod quota against current usage
	if quotas.Pods != nil {
		if err := v.validatePodQuota(ctx, mcpServer, quotas.Pods); err != nil {
			return fmt.Errorf("pod quota validation failed: %w", err)
		}
	}

	// Validate Service quota against current usage
	if quotas.Services != nil {
		if err := v.validateServiceQuota(ctx, mcpServer, quotas.Services); err != nil {
			return fmt.Errorf("service quota validation failed: %w", err)
		}
	}

	// Validate isolation level
	if err := v.validateIsolationLevel(tenancy.IsolationLevel); err != nil {
		return fmt.Errorf("isolation level validation failed: %w", err)
	}

	// Validate tenant ID
	if tenancy.TenantID == "" {
		return fmt.Errorf("tenant ID is required when tenancy is enabled")
	}

	if len(tenancy.TenantID) > 63 {
		return fmt.Errorf("tenant ID cannot be longer than 63 characters")
	}

	// Validate tenant ID format (must be a valid DNS label)
	if !v.isValidDNSLabel(tenancy.TenantID) {
		return fmt.Errorf("tenant ID must be a valid DNS label")
	}

	logger.V(1).Info("Tenant resource quota validation completed successfully")
	return nil
}

// ValidateRegistry validates MCPRegistry specification
func (v *DefaultValidationService) ValidateRegistry(ctx context.Context, registry *mcpv1.MCPRegistry) error {
	logger := log.FromContext(ctx).WithValues("mcpregistry", registry.Name, "namespace", registry.Namespace)
	logger.V(1).Info("Validating MCPRegistry specification")

	// Validate URL
	if registry.Spec.URL == "" {
		return fmt.Errorf("registry URL is required")
	}

	if _, err := url.Parse(registry.Spec.URL); err != nil {
		return fmt.Errorf("invalid registry URL: %w", err)
	}

	// Validate registry type
	if registry.Spec.Type != "" {
		validTypes := []string{"github", "local", "http", "https"}
		if !v.contains(validTypes, registry.Spec.Type) {
			return fmt.Errorf("invalid registry type: %s, must be one of: %s",
				registry.Spec.Type, strings.Join(validTypes, ", "))
		}
	}

	// Validate authentication configuration
	if registry.Spec.Auth != nil {
		if err := v.validateRegistryAuth(*registry.Spec.Auth); err != nil {
			return fmt.Errorf("authentication validation failed: %w", err)
		}
	}

	// Validate sync interval
	if registry.Spec.SyncInterval != nil {
		if registry.Spec.SyncInterval.Duration <= 0 {
			return fmt.Errorf("sync interval must be positive")
		}
	}

	logger.V(1).Info("MCPRegistry validation completed successfully")
	return nil
}

// validateRegistryInfo validates registry information in MCPServer
func (v *DefaultValidationService) validateRegistryInfo(registry mcpv1.MCPRegistryInfo) error {
	if registry.Name == "" {
		return fmt.Errorf("registry name is required")
	}

	if len(registry.Name) > 253 {
		return fmt.Errorf("registry name cannot be longer than 253 characters")
	}

	return nil
}

// validateRuntimeSpec validates runtime specification
func (v *DefaultValidationService) validateRuntimeSpec(runtime mcpv1.MCPRuntimeSpec) error {
	if runtime.Type == "" {
		return fmt.Errorf("runtime type is required")
	}

	validTypes := []string{"docker", "node", "python", "binary"}
	if !v.contains(validTypes, runtime.Type) {
		return fmt.Errorf("invalid runtime type: %s, must be one of: %s",
			runtime.Type, strings.Join(validTypes, ", "))
	}

	// For docker runtime, image is required
	if runtime.Type == "docker" && runtime.Image == "" {
		return fmt.Errorf("image is required for docker runtime")
	}

	// Validate port
	if runtime.Port < 0 || runtime.Port > 65535 {
		return fmt.Errorf("port must be between 0 and 65535")
	}

	return nil
}

// validateResourceRequirements validates resource requirements
func (v *DefaultValidationService) validateResourceRequirements(resources mcpv1.ResourceRequirements) error {
	// Validate limits
	if err := v.validateResourceList(resources.Limits, "limits"); err != nil {
		return err
	}

	// Validate requests
	if err := v.validateResourceList(resources.Requests, "requests"); err != nil {
		return err
	}

	// Validate that requests don't exceed limits
	if err := v.validateRequestsVsLimits(resources.Requests, resources.Limits); err != nil {
		return err
	}

	return nil
}

// validateResourceList validates a resource list
func (v *DefaultValidationService) validateResourceList(resourceList mcpv1.ResourceList, listType string) error {
	for resourceName, resourceValue := range resourceList {
		if resourceValue == "" {
			return fmt.Errorf("%s.%s cannot be empty", listType, resourceName)
		}

		// Try to parse the resource quantity
		if _, err := resource.ParseQuantity(resourceValue); err != nil {
			return fmt.Errorf("invalid %s.%s value '%s': %w", listType, resourceName, resourceValue, err)
		}
	}

	return nil
}

// validateRequestsVsLimits validates that requests don't exceed limits
func (v *DefaultValidationService) validateRequestsVsLimits(requests, limits mcpv1.ResourceList) error {
	for resourceName, requestValue := range requests {
		if limitValue, exists := limits[resourceName]; exists {
			requestQuantity, err := resource.ParseQuantity(requestValue)
			if err != nil {
				continue // Already validated in validateResourceList
			}

			limitQuantity, err := resource.ParseQuantity(limitValue)
			if err != nil {
				continue // Already validated in validateResourceList
			}

			if requestQuantity.Cmp(limitQuantity) > 0 {
				return fmt.Errorf("requests.%s (%s) exceeds limits.%s (%s)",
					resourceName, requestValue, resourceName, limitValue)
			}
		}
	}

	return nil
}

// validateAutoscalingSpec validates autoscaling specification
func (v *DefaultValidationService) validateAutoscalingSpec(autoscaling mcpv1.AutoscalingSpec) error {
	// Validate HPA
	if autoscaling.HPA != nil {
		hpa := *autoscaling.HPA
		if hpa.MinReplicas != nil && *hpa.MinReplicas < 1 {
			return fmt.Errorf("HPA minReplicas must be at least 1")
		}
		if hpa.MaxReplicas < 1 {
			return fmt.Errorf("HPA maxReplicas must be at least 1")
		}
		if hpa.MinReplicas != nil && *hpa.MinReplicas > hpa.MaxReplicas {
			return fmt.Errorf("HPA minReplicas cannot be greater than maxReplicas")
		}
	}

	// Validate VPA
	if autoscaling.VPA != nil {
		vpa := *autoscaling.VPA
		validModes := []string{"Off", "Initial", "Recreate", "Auto"}
		if !v.contains(validModes, string(vpa.UpdateMode)) {
			return fmt.Errorf("invalid VPA update mode: %s, must be one of: %s",
				vpa.UpdateMode, strings.Join(validModes, ", "))
		}
	}

	return nil
}

// validateTenancySpec validates tenancy specification
func (v *DefaultValidationService) validateTenancySpec(tenancy mcpv1.TenancySpec) error {
	if tenancy.TenantID == "" {
		return fmt.Errorf("tenant ID is required")
	}

	if err := v.validateIsolationLevel(tenancy.IsolationLevel); err != nil {
		return err
	}

	if tenancy.ResourceQuotas != nil {
		if err := v.validateTenantResourceQuotas(*tenancy.ResourceQuotas); err != nil {
			return err
		}
	}

	return nil
}

// validateTenantResourceQuotas validates tenant resource quotas specification
func (v *DefaultValidationService) validateTenantResourceQuotas(quotas mcpv1.TenantResourceQuotas) error {
	// Validate CPU quota
	if quotas.CPU != nil {
		if quotas.CPU.IsZero() || quotas.CPU.Sign() < 0 {
			return fmt.Errorf("CPU quota must be positive")
		}
	}

	// Validate Memory quota
	if quotas.Memory != nil {
		if quotas.Memory.IsZero() || quotas.Memory.Sign() < 0 {
			return fmt.Errorf("Memory quota must be positive")
		}
	}

	// Validate Storage quota
	if quotas.Storage != nil {
		if quotas.Storage.IsZero() || quotas.Storage.Sign() < 0 {
			return fmt.Errorf("Storage quota must be positive")
		}
	}

	// Validate Pods quota
	if quotas.Pods != nil {
		if *quotas.Pods <= 0 {
			return fmt.Errorf("Pods quota must be positive")
		}
	}

	// Validate Services quota
	if quotas.Services != nil {
		if *quotas.Services <= 0 {
			return fmt.Errorf("Services quota must be positive")
		}
	}

	// Validate PersistentVolumeClaims quota
	if quotas.PersistentVolumeClaims != nil {
		if *quotas.PersistentVolumeClaims <= 0 {
			return fmt.Errorf("PersistentVolumeClaims quota must be positive")
		}
	}

	// Validate ConfigMaps quota
	if quotas.ConfigMaps != nil {
		if *quotas.ConfigMaps <= 0 {
			return fmt.Errorf("ConfigMaps quota must be positive")
		}
	}

	// Validate Secrets quota
	if quotas.Secrets != nil {
		if *quotas.Secrets <= 0 {
			return fmt.Errorf("Secrets quota must be positive")
		}
	}

	return nil
}

// validateIsolationLevel validates isolation level
func (v *DefaultValidationService) validateIsolationLevel(level mcpv1.IsolationLevel) error {
	validLevels := []mcpv1.IsolationLevel{
		mcpv1.IsolationLevelNone,
		mcpv1.IsolationLevelNamespace,
		mcpv1.IsolationLevelNode,
		mcpv1.IsolationLevelStrict,
	}

	for _, validLevel := range validLevels {
		if level == validLevel {
			return nil
		}
	}

	return fmt.Errorf("invalid isolation level: %s", level)
}

// validateRegistryAuth validates registry authentication configuration
func (v *DefaultValidationService) validateRegistryAuth(auth mcpv1.RegistryAuth) error {
	if auth.SecretRef == nil && auth.Token == "" {
		return fmt.Errorf("either secretRef or token must be provided for authentication")
	}

	if auth.SecretRef != nil && auth.Token != "" {
		return fmt.Errorf("cannot specify both secretRef and token for authentication")
	}

	if auth.SecretRef != nil {
		if auth.SecretRef.Name == "" {
			return fmt.Errorf("secret name is required in secretRef")
		}
	}

	return nil
}

// validateCPUQuota validates CPU usage against tenant quota
func (v *DefaultValidationService) validateCPUQuota(ctx context.Context, mcpServer *mcpv1.MCPServer, cpuQuota *resource.Quantity) error {
	// Get current CPU usage for the tenant
	currentUsage, err := v.getTenantCPUUsage(ctx, mcpServer.Spec.Tenancy.TenantID, mcpServer.Namespace)
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
func (v *DefaultValidationService) validateMemoryQuota(ctx context.Context, mcpServer *mcpv1.MCPServer, memoryQuota *resource.Quantity) error {
	// Get current Memory usage for the tenant
	currentUsage, err := v.getTenantMemoryUsage(ctx, mcpServer.Spec.Tenancy.TenantID, mcpServer.Namespace)
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

// validatePodQuota validates Pod usage against tenant quota
func (v *DefaultValidationService) validatePodQuota(ctx context.Context, mcpServer *mcpv1.MCPServer, podQuota *int32) error {
	// Get current Pod count for the tenant
	currentCount, err := v.getTenantPodCount(ctx, mcpServer.Spec.Tenancy.TenantID, mcpServer.Namespace)
	if err != nil {
		return fmt.Errorf("failed to get current Pod count: %w", err)
	}

	// Calculate requested pods for this MCPServer
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

// validateServiceQuota validates Service usage against tenant quota
func (v *DefaultValidationService) validateServiceQuota(ctx context.Context, mcpServer *mcpv1.MCPServer, serviceQuota *int32) error {
	// Get current Service count for the tenant
	currentCount, err := v.getTenantServiceCount(ctx, mcpServer.Spec.Tenancy.TenantID, mcpServer.Namespace)
	if err != nil {
		return fmt.Errorf("failed to get current Service count: %w", err)
	}

	// This MCPServer will create 1 service
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
func (v *DefaultValidationService) getTenantCPUUsage(ctx context.Context, tenantID string, namespace string) (resource.Quantity, error) {
	logger := log.FromContext(ctx).WithValues("tenantID", tenantID, "namespace", namespace)
	logger.V(1).Info("Getting tenant CPU usage")

	// Create label selector for tenant pods
	labelSelector := labels.SelectorFromSet(labels.Set{
		"mcp.allbeone.io/tenant": tenantID,
	})

	// List pods with tenant label
	podList := &corev1.PodList{}
	listOpts := &client.ListOptions{
		Namespace:     namespace,
		LabelSelector: labelSelector,
	}

	if err := v.kubernetesClient.List(ctx, podList, listOpts); err != nil {
		return resource.Quantity{}, fmt.Errorf("failed to list pods for tenant %s: %w", tenantID, err)
	}

	// Calculate total CPU usage from pod resource requests
	totalCPU := resource.Quantity{}
	for _, pod := range podList.Items {
		// Only count running pods
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		for _, container := range pod.Spec.Containers {
			if cpuRequest, exists := container.Resources.Requests[corev1.ResourceCPU]; exists {
				totalCPU.Add(cpuRequest)
			}
		}
	}

	logger.V(1).Info("Calculated tenant CPU usage", "totalCPU", totalCPU.String(), "podCount", len(podList.Items))
	return totalCPU, nil
}

// getTenantMemoryUsage gets current Memory usage for a tenant
func (v *DefaultValidationService) getTenantMemoryUsage(ctx context.Context, tenantID string, namespace string) (resource.Quantity, error) {
	logger := log.FromContext(ctx).WithValues("tenantID", tenantID, "namespace", namespace)
	logger.V(1).Info("Getting tenant Memory usage")

	// Create label selector for tenant pods
	labelSelector := labels.SelectorFromSet(labels.Set{
		"mcp.allbeone.io/tenant": tenantID,
	})

	// List pods with tenant label
	podList := &corev1.PodList{}
	listOpts := &client.ListOptions{
		Namespace:     namespace,
		LabelSelector: labelSelector,
	}

	if err := v.kubernetesClient.List(ctx, podList, listOpts); err != nil {
		return resource.Quantity{}, fmt.Errorf("failed to list pods for tenant %s: %w", tenantID, err)
	}

	// Calculate total Memory usage from pod resource requests
	totalMemory := resource.Quantity{}
	for _, pod := range podList.Items {
		// Only count running pods
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		for _, container := range pod.Spec.Containers {
			if memoryRequest, exists := container.Resources.Requests[corev1.ResourceMemory]; exists {
				totalMemory.Add(memoryRequest)
			}
		}
	}

	logger.V(1).Info("Calculated tenant Memory usage", "totalMemory", totalMemory.String(), "podCount", len(podList.Items))
	return totalMemory, nil
}

// getTenantPodCount gets current Pod count for a tenant
func (v *DefaultValidationService) getTenantPodCount(ctx context.Context, tenantID string, namespace string) (int32, error) {
	logger := log.FromContext(ctx).WithValues("tenantID", tenantID, "namespace", namespace)
	logger.V(1).Info("Getting tenant Pod count")

	// Create label selector for tenant pods
	labelSelector := labels.SelectorFromSet(labels.Set{
		"mcp.allbeone.io/tenant": tenantID,
	})

	// List pods with tenant label
	podList := &corev1.PodList{}
	listOpts := &client.ListOptions{
		Namespace:     namespace,
		LabelSelector: labelSelector,
	}

	if err := v.kubernetesClient.List(ctx, podList, listOpts); err != nil {
		return 0, fmt.Errorf("failed to list pods for tenant %s: %w", tenantID, err)
	}

	// Count running pods
	runningPodCount := int32(0)
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			runningPodCount++
		}
	}

	logger.V(1).Info("Calculated tenant Pod count", "runningPods", runningPodCount, "totalPods", len(podList.Items))
	return runningPodCount, nil
}

// getTenantServiceCount gets current Service count for a tenant
func (v *DefaultValidationService) getTenantServiceCount(ctx context.Context, tenantID string, namespace string) (int32, error) {
	logger := log.FromContext(ctx).WithValues("tenantID", tenantID, "namespace", namespace)
	logger.V(1).Info("Getting tenant Service count")

	// Create label selector for tenant services
	labelSelector := labels.SelectorFromSet(labels.Set{
		"mcp.allbeone.io/tenant": tenantID,
	})

	// List services with tenant label
	serviceList := &corev1.ServiceList{}
	listOpts := &client.ListOptions{
		Namespace:     namespace,
		LabelSelector: labelSelector,
	}

	if err := v.kubernetesClient.List(ctx, serviceList, listOpts); err != nil {
		return 0, fmt.Errorf("failed to list services for tenant %s: %w", tenantID, err)
	}

	serviceCount := int32(len(serviceList.Items))
	logger.V(1).Info("Calculated tenant Service count", "serviceCount", serviceCount)
	return serviceCount, nil
}

// Helper functions

// contains checks if a slice contains a string
func (v *DefaultValidationService) contains(slice []string, item string) bool {
	for _, s := range slice {
		if s == item {
			return true
		}
	}
	return false
}

// isValidDNSLabel checks if a string is a valid DNS label
func (v *DefaultValidationService) isValidDNSLabel(label string) bool {
	if len(label) == 0 || len(label) > 63 {
		return false
	}

	// Must start and end with alphanumeric character
	if !v.isAlphaNumeric(label[0]) || !v.isAlphaNumeric(label[len(label)-1]) {
		return false
	}

	// Can contain alphanumeric characters and hyphens
	for _, char := range label {
		if !v.isAlphaNumeric(byte(char)) && char != '-' {
			return false
		}
	}

	return true
}

// isAlphaNumeric checks if a byte is alphanumeric
func (v *DefaultValidationService) isAlphaNumeric(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}
