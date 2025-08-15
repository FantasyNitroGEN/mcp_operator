package services

import (
	"fmt"
	"strconv"

	mcpv1 "github.com/FantasyNitroGEN/mcp_operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// SimpleResourceBuilderService implements ResourceBuilderService interface with simplified functionality
type SimpleResourceBuilderService struct{}

// NewSimpleResourceBuilderService creates a new SimpleResourceBuilderService
func NewSimpleResourceBuilderService() *SimpleResourceBuilderService {
	return &SimpleResourceBuilderService{}
}

// BuildDeployment builds a deployment for MCPServer
func (r *SimpleResourceBuilderService) BuildDeployment(mcpServer *mcpv1.MCPServer) *appsv1.Deployment {
	logger := log.Log.WithValues("mcpserver", mcpServer.Name, "namespace", mcpServer.Namespace)
	logger.V(1).Info("Building deployment")

	// Default values
	replicas := int32(1)
	if mcpServer.Spec.Replicas != nil {
		replicas = *mcpServer.Spec.Replicas
	}

	// Build labels
	labels := r.buildLabels(mcpServer)

	// Build selector
	selector := r.buildSelector(mcpServer)

	// Build container
	container := r.buildContainer(mcpServer)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcpServer.Name,
			Namespace: mcpServer.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: selector,
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels:      labels,
					Annotations: r.buildPodAnnotations(mcpServer),
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{container},
				},
			},
			Strategy: r.buildDeploymentStrategy(mcpServer),
		},
	}

	logger.V(1).Info("Deployment built successfully", "replicas", replicas)
	return deployment
}

// BuildService builds a service for MCPServer
func (r *SimpleResourceBuilderService) BuildService(mcpServer *mcpv1.MCPServer) *corev1.Service {
	logger := log.Log.WithValues("mcpserver", mcpServer.Name, "namespace", mcpServer.Namespace)
	logger.V(1).Info("Building service")

	labels := r.buildLabels(mcpServer)
	selector := r.buildSelector(mcpServer)

	// Default port
	port := int32(8080)
	if mcpServer.Spec.Runtime.Port > 0 {
		port = mcpServer.Spec.Runtime.Port
	}

	// Service type
	serviceType := corev1.ServiceTypeClusterIP
	if mcpServer.Spec.ServiceType != "" {
		serviceType = mcpServer.Spec.ServiceType
	}

	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcpServer.Name,
			Namespace: mcpServer.Namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Type:     serviceType,
			Selector: selector,
			Ports: []corev1.ServicePort{
				{
					Name:       "mcp",
					Port:       port,
					TargetPort: intstr.FromInt(int(port)),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	logger.V(1).Info("Service built successfully", "port", port, "type", serviceType)
	return service
}

// BuildConfigMap builds config map for MCPServer (simplified)
func (r *SimpleResourceBuilderService) BuildConfigMap(mcpServer *mcpv1.MCPServer) *corev1.ConfigMap {
	logger := log.Log.WithValues("mcpserver", mcpServer.Name, "namespace", mcpServer.Namespace)
	logger.V(1).Info("Building config map (simple)")

	labels := r.buildLabels(mcpServer)

	// Build simple config map data
	data := make(map[string]string)

	// Add configuration from spec.config if present
	if mcpServer.Spec.Config != nil && mcpServer.Spec.Config.Raw != nil {
		data["config.json"] = string(mcpServer.Spec.Config.Raw)
	}

	// Add environment variables as config
	for _, envVar := range mcpServer.Spec.Env {
		data[envVar.Name] = envVar.Value
	}

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-config", mcpServer.Name),
			Namespace: mcpServer.Namespace,
			Labels:    labels,
		},
		Data: data,
	}

	logger.V(1).Info("ConfigMap built successfully (simple)", "dataKeys", len(data))
	return configMap
}

// BuildSecret builds secret for MCPServer (simplified)
func (r *SimpleResourceBuilderService) BuildSecret(mcpServer *mcpv1.MCPServer) *corev1.Secret {
	logger := log.Log.WithValues("mcpserver", mcpServer.Name, "namespace", mcpServer.Namespace)
	logger.V(1).Info("Building secret (simple)")

	labels := r.buildLabels(mcpServer)

	// Build simple secret data from secret references
	data := make(map[string][]byte)

	// Add any sensitive configuration from secretRefs
	for _, secretRef := range mcpServer.Spec.SecretRefs {
		data[secretRef.Key] = []byte(fmt.Sprintf("secret-data-for-%s", secretRef.Key))
	}

	// Only create secret if there's data to store
	if len(data) == 0 {
		logger.V(1).Info("No secret data to store, skipping secret creation (simple)")
		return nil
	}

	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-secret", mcpServer.Name),
			Namespace: mcpServer.Namespace,
			Labels:    labels,
		},
		Type: corev1.SecretTypeOpaque,
		Data: data,
	}

	logger.V(1).Info("Secret built successfully (simple)", "dataKeys", len(data))
	return secret
}

// BuildHPA builds horizontal pod autoscaler for MCPServer
func (r *SimpleResourceBuilderService) BuildHPA(mcpServer *mcpv1.MCPServer) *autoscalingv2.HorizontalPodAutoscaler {
	if mcpServer.Spec.Autoscaling == nil || mcpServer.Spec.Autoscaling.HPA == nil {
		return nil
	}

	logger := log.Log.WithValues("mcpserver", mcpServer.Name, "namespace", mcpServer.Namespace)
	logger.V(1).Info("Building HPA")

	hpaSpec := mcpServer.Spec.Autoscaling.HPA
	labels := r.buildLabels(mcpServer)

	// Default values
	minReplicas := int32(1)
	if hpaSpec.MinReplicas != nil {
		minReplicas = *hpaSpec.MinReplicas
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
			MaxReplicas: hpaSpec.MaxReplicas,
			Metrics:     r.buildHPAMetrics(hpaSpec),
		},
	}

	logger.V(1).Info("HPA built successfully", "minReplicas", minReplicas, "maxReplicas", hpaSpec.MaxReplicas)
	return hpa
}

// BuildVPA builds vertical pod autoscaler for MCPServer
func (r *SimpleResourceBuilderService) BuildVPA(mcpServer *mcpv1.MCPServer) *unstructured.Unstructured {
	if mcpServer.Spec.Autoscaling == nil || mcpServer.Spec.Autoscaling.VPA == nil {
		return nil
	}

	logger := log.Log.WithValues("mcpserver", mcpServer.Name, "namespace", mcpServer.Namespace)
	logger.V(1).Info("Building VPA")

	vpaSpec := mcpServer.Spec.Autoscaling.VPA
	labels := r.buildLabels(mcpServer)

	vpa := &unstructured.Unstructured{}
	vpa.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "autoscaling.k8s.io",
		Version: "v1",
		Kind:    "VerticalPodAutoscaler",
	})

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
		"updatePolicy": map[string]interface{}{
			"updateMode": string(vpaSpec.UpdateMode),
		},
	}

	if err := unstructured.SetNestedMap(vpa.Object, spec, "spec"); err != nil {
		logger.Error(err, "Failed to set VPA spec")
		return nil
	}

	logger.V(1).Info("VPA built successfully", "updateMode", vpaSpec.UpdateMode)
	return vpa
}

// BuildNetworkPolicy builds network policy for MCPServer (simplified)
func (r *SimpleResourceBuilderService) BuildNetworkPolicy(mcpServer *mcpv1.MCPServer) *networkingv1.NetworkPolicy {
	if mcpServer.Spec.Tenancy == nil || mcpServer.Spec.Tenancy.NetworkPolicy == nil {
		return nil
	}

	logger := log.Log.WithValues("mcpserver", mcpServer.Name, "namespace", mcpServer.Namespace)
	logger.V(1).Info("Building network policy")

	networkPolicySpec := mcpServer.Spec.Tenancy.NetworkPolicy
	if !networkPolicySpec.Enabled {
		return nil
	}

	labels := r.buildLabels(mcpServer)
	selector := r.buildSelector(mcpServer)

	// Create a basic network policy that allows ingress on the MCP port
	port := int32(8080)
	if mcpServer.Spec.Runtime.Port > 0 {
		port = mcpServer.Spec.Runtime.Port
	}

	networkPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("%s-network-policy", mcpServer.Name),
			Namespace: mcpServer.Namespace,
			Labels:    labels,
		},
		Spec: networkingv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchLabels: selector,
			},
			PolicyTypes: []networkingv1.PolicyType{
				networkingv1.PolicyTypeIngress,
			},
			Ingress: []networkingv1.NetworkPolicyIngressRule{
				{
					Ports: []networkingv1.NetworkPolicyPort{
						{
							Protocol: &[]corev1.Protocol{corev1.ProtocolTCP}[0],
							Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: port},
						},
					},
				},
			},
		},
	}

	logger.V(1).Info("Network policy built successfully")
	return networkPolicy
}

// Helper methods

// buildLabels builds standard labels for resources
func (r *SimpleResourceBuilderService) buildLabels(mcpServer *mcpv1.MCPServer) map[string]string {
	labels := map[string]string{
		"app.kubernetes.io/name":       "mcp-server",
		"app.kubernetes.io/instance":   mcpServer.Name,
		"app.kubernetes.io/component":  "server",
		"app.kubernetes.io/part-of":    "mcp-operator",
		"app.kubernetes.io/managed-by": "mcp-operator",
	}

	// Add registry information
	//nolint:staticcheck
	if mcpServer.Spec.Registry.Server != "" {
		labels["mcp.allbeone.io/server-name"] = mcpServer.Spec.Registry.Server
	}
	// Version is stored in annotations, get it from there if needed
	if mcpServer.Annotations != nil && mcpServer.Annotations["mcp.allbeone.io/registry-version"] != "" {
		labels["mcp.allbeone.io/server-version"] = mcpServer.Annotations["mcp.allbeone.io/registry-version"]
	}

	// Add tenancy information
	if mcpServer.Spec.Tenancy != nil && mcpServer.Spec.Tenancy.TenantID != "" {
		labels["mcp.allbeone.io/tenant-id"] = mcpServer.Spec.Tenancy.TenantID
	}

	return labels
}

// buildSelector builds selector labels for resources
func (r *SimpleResourceBuilderService) buildSelector(mcpServer *mcpv1.MCPServer) map[string]string {
	selector := map[string]string{
		"app.kubernetes.io/name":     "mcp-server",
		"app.kubernetes.io/instance": mcpServer.Name,
	}

	// Use custom selector if provided
	if len(mcpServer.Spec.Selector) > 0 {
		for k, v := range mcpServer.Spec.Selector {
			selector[k] = v
		}
	}

	return selector
}

// buildContainer builds the main container for the pod
func (r *SimpleResourceBuilderService) buildContainer(mcpServer *mcpv1.MCPServer) corev1.Container {
	// Default port
	port := int32(8080)
	if mcpServer.Spec.Runtime.Port > 0 {
		port = mcpServer.Spec.Runtime.Port
	}

	container := corev1.Container{
		Name:  "mcp-server",
		Image: mcpServer.Spec.Runtime.Image,
		Ports: []corev1.ContainerPort{
			{
				Name:          "mcp",
				ContainerPort: port,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env: r.buildEnvironmentVariables(mcpServer),
	}

	// Add command and args
	if len(mcpServer.Spec.Runtime.Command) > 0 {
		container.Command = mcpServer.Spec.Runtime.Command
	}
	if len(mcpServer.Spec.Runtime.Args) > 0 {
		container.Args = mcpServer.Spec.Runtime.Args
	}

	// Add resource requirements
	if !r.isResourceRequirementsEmpty(mcpServer.Spec.Resources) {
		container.Resources = r.buildResourceRequirements(mcpServer.Spec.Resources)
	}

	// Add security context
	if mcpServer.Spec.SecurityContext != nil {
		container.SecurityContext = mcpServer.Spec.SecurityContext
	}

	return container
}

// buildEnvironmentVariables builds environment variables for the container
func (r *SimpleResourceBuilderService) buildEnvironmentVariables(mcpServer *mcpv1.MCPServer) []corev1.EnvVar {
	envVars := []corev1.EnvVar{}

	// Add runtime environment variables
	for k, v := range mcpServer.Spec.Runtime.Env {
		envVars = append(envVars, corev1.EnvVar{
			Name:  k,
			Value: v,
		})
	}

	// Add general environment variables
	for _, envVar := range mcpServer.Spec.Env {
		envVars = append(envVars, corev1.EnvVar{
			Name:  envVar.Name,
			Value: envVar.Value,
		})
	}

	// Add standard MCP environment variables
	envVars = append(envVars, []corev1.EnvVar{
		{
			Name: "MCP_SERVER_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		},
		{
			Name: "MCP_SERVER_NAMESPACE",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.namespace",
				},
			},
		},
		{
			Name:  "MCP_SERVER_PORT",
			Value: strconv.Itoa(int(mcpServer.Spec.Runtime.Port)),
		},
	}...)

	return envVars
}

// buildResourceRequirements builds resource requirements for the container
func (r *SimpleResourceBuilderService) buildResourceRequirements(resources mcpv1.ResourceRequirements) corev1.ResourceRequirements {
	requirements := corev1.ResourceRequirements{}

	if len(resources.Limits) > 0 {
		requirements.Limits = make(corev1.ResourceList)
		for k, v := range resources.Limits {
			if quantity, err := resource.ParseQuantity(v); err == nil {
				requirements.Limits[corev1.ResourceName(k)] = quantity
			}
		}
	}

	if len(resources.Requests) > 0 {
		requirements.Requests = make(corev1.ResourceList)
		for k, v := range resources.Requests {
			if quantity, err := resource.ParseQuantity(v); err == nil {
				requirements.Requests[corev1.ResourceName(k)] = quantity
			}
		}
	}

	return requirements
}

// buildHPAMetrics builds HPA metrics
func (r *SimpleResourceBuilderService) buildHPAMetrics(hpaSpec *mcpv1.HPASpec) []autoscalingv2.MetricSpec {
	metrics := []autoscalingv2.MetricSpec{}

	// Add CPU utilization metric if specified
	if hpaSpec.TargetCPUUtilizationPercentage != nil {
		metrics = append(metrics, autoscalingv2.MetricSpec{
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

	// Add memory utilization metric if specified
	if hpaSpec.TargetMemoryUtilizationPercentage != nil {
		metrics = append(metrics, autoscalingv2.MetricSpec{
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

	return metrics
}

// Helper utility methods

// isResourceRequirementsEmpty checks if resource requirements are empty
func (r *SimpleResourceBuilderService) isResourceRequirementsEmpty(resources mcpv1.ResourceRequirements) bool {
	return len(resources.Limits) == 0 && len(resources.Requests) == 0
}

// buildDeploymentStrategy builds deployment strategy configuration
func (r *SimpleResourceBuilderService) buildDeploymentStrategy(mcpServer *mcpv1.MCPServer) appsv1.DeploymentStrategy {
	// Default strategy for zero-downtime deployments
	defaultMaxUnavailable := &intstr.IntOrString{Type: intstr.Int, IntVal: 0}
	defaultMaxSurge := &intstr.IntOrString{Type: intstr.Int, IntVal: 1}

	// If no deployment strategy is specified, use zero-downtime defaults
	if mcpServer.Spec.DeploymentStrategy == nil {
		return appsv1.DeploymentStrategy{
			Type: appsv1.RollingUpdateDeploymentStrategyType,
			RollingUpdate: &appsv1.RollingUpdateDeployment{
				MaxUnavailable: defaultMaxUnavailable,
				MaxSurge:       defaultMaxSurge,
			},
		}
	}

	strategy := mcpServer.Spec.DeploymentStrategy

	// Handle Recreate strategy
	if strategy.Type == "Recreate" {
		return appsv1.DeploymentStrategy{
			Type: appsv1.RecreateDeploymentStrategyType,
		}
	}

	// Handle RollingUpdate strategy (default)
	rollingUpdate := &appsv1.RollingUpdateDeployment{
		MaxUnavailable: defaultMaxUnavailable,
		MaxSurge:       defaultMaxSurge,
	}

	// Use custom rolling update parameters if specified
	if strategy.RollingUpdate != nil {
		if strategy.RollingUpdate.MaxUnavailable != nil {
			rollingUpdate.MaxUnavailable = strategy.RollingUpdate.MaxUnavailable
		}
		if strategy.RollingUpdate.MaxSurge != nil {
			rollingUpdate.MaxSurge = strategy.RollingUpdate.MaxSurge
		}
	}

	return appsv1.DeploymentStrategy{
		Type:          appsv1.RollingUpdateDeploymentStrategyType,
		RollingUpdate: rollingUpdate,
	}
}

// BuildVirtualService builds Istio VirtualService for MCPServer (simplified version)
func (r *SimpleResourceBuilderService) BuildVirtualService(mcpServer *mcpv1.MCPServer) *unstructured.Unstructured {
	logger := log.Log.WithValues("mcpserver", mcpServer.Name, "namespace", mcpServer.Namespace)
	logger.V(1).Info("Building VirtualService (simple)")

	// Check if Istio is enabled and VirtualService is configured
	if mcpServer.Spec.Istio == nil || !mcpServer.Spec.Istio.Enabled || mcpServer.Spec.Istio.VirtualService == nil {
		return nil
	}

	istioConfig := mcpServer.Spec.Istio
	vsConfig := istioConfig.VirtualService

	// Set default path if not specified
	path := vsConfig.Path
	if path == "" {
		path = "/"
	}

	// Set default gateway if not specified
	gateway := istioConfig.Gateway
	if gateway == "" {
		gateway = "default"
	}

	// Build simplified VirtualService spec
	spec := map[string]interface{}{
		"hosts":    []string{vsConfig.Host},
		"gateways": []string{gateway},
		"http": []map[string]interface{}{
			{
				"match": []map[string]interface{}{
					{
						"uri": map[string]interface{}{
							"prefix": path,
						},
					},
				},
				"route": []map[string]interface{}{
					{
						"destination": map[string]interface{}{
							"host": fmt.Sprintf("%s.%s.svc.cluster.local", mcpServer.Name, mcpServer.Namespace),
							"port": map[string]interface{}{
								"number": mcpServer.Spec.Runtime.Port,
							},
						},
					},
				},
			},
		},
	}

	// Create VirtualService resource
	virtualService := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.istio.io/v1beta1",
			"kind":       "VirtualService",
			"metadata": map[string]interface{}{
				"name":      mcpServer.Name,
				"namespace": mcpServer.Namespace,
				"labels":    r.buildLabels(mcpServer),
			},
			"spec": spec,
		},
	}

	return virtualService
}

// BuildDestinationRule builds Istio DestinationRule for MCPServer (simplified version)
func (r *SimpleResourceBuilderService) BuildDestinationRule(mcpServer *mcpv1.MCPServer) *unstructured.Unstructured {
	logger := log.Log.WithValues("mcpserver", mcpServer.Name, "namespace", mcpServer.Namespace)
	logger.V(1).Info("Building DestinationRule (simple)")

	// Check if Istio is enabled and DestinationRule is configured
	if mcpServer.Spec.Istio == nil || !mcpServer.Spec.Istio.Enabled || mcpServer.Spec.Istio.DestinationRule == nil {
		return nil
	}

	// Build simplified DestinationRule spec with basic mTLS
	spec := map[string]interface{}{
		"host": fmt.Sprintf("%s.%s.svc.cluster.local", mcpServer.Name, mcpServer.Namespace),
		"trafficPolicy": map[string]interface{}{
			"tls": map[string]interface{}{
				"mode": "ISTIO_MUTUAL",
			},
		},
	}

	// Create DestinationRule resource
	destinationRule := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.istio.io/v1beta1",
			"kind":       "DestinationRule",
			"metadata": map[string]interface{}{
				"name":      mcpServer.Name,
				"namespace": mcpServer.Namespace,
				"labels":    r.buildLabels(mcpServer),
			},
			"spec": spec,
		},
	}

	return destinationRule
}

// buildPodAnnotations builds annotations for pod template
func (r *SimpleResourceBuilderService) buildPodAnnotations(mcpServer *mcpv1.MCPServer) map[string]string {
	annotations := make(map[string]string)

	// Add Istio sidecar injection annotation if enabled
	if mcpServer.Spec.Istio != nil && mcpServer.Spec.Istio.Enabled {
		// Check if sidecar injection is explicitly configured
		if mcpServer.Spec.Istio.SidecarInject != nil {
			if *mcpServer.Spec.Istio.SidecarInject {
				annotations["sidecar.istio.io/inject"] = "true"
			} else {
				annotations["sidecar.istio.io/inject"] = "false"
			}
		} else {
			// Default to true if Istio is enabled but sidecar injection is not explicitly set
			annotations["sidecar.istio.io/inject"] = "true"
		}
	}

	return annotations
}
