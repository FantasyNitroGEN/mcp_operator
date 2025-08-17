package services

import (
	"fmt"
	"strconv"
	"strings"

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

// DefaultResourceBuilderService implements ResourceBuilderService interface
type DefaultResourceBuilderService struct{}

// NewDefaultResourceBuilderService creates a new DefaultResourceBuilderService
func NewDefaultResourceBuilderService() *DefaultResourceBuilderService {
	return &DefaultResourceBuilderService{}
}

// BuildDeployment builds a deployment for MCPServer
func (r *DefaultResourceBuilderService) BuildDeployment(mcpServer *mcpv1.MCPServer) *appsv1.Deployment {
	logger := log.Log.WithValues("mcpserver", mcpServer.Name, "namespace", mcpServer.Namespace)
	logger.V(1).Info("Building deployment")

	// Build labels
	labels := r.buildLabels(mcpServer)

	// Build selector
	selector := r.buildSelector(mcpServer)

	// Build pod template
	podTemplate := r.buildPodTemplate(mcpServer, labels)

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      mcpServer.Name,
			Namespace: mcpServer.Namespace,
			Labels:    labels,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: selector,
			},
			Template: podTemplate,
			Strategy: r.buildDeploymentStrategy(mcpServer),
		},
	}

	// Only set replicas if HPA is not enabled
	// When HPA is enabled, it will manage the replica count
	if mcpServer.Spec.Autoscaling == nil || mcpServer.Spec.Autoscaling.HPA == nil || !mcpServer.Spec.Autoscaling.HPA.Enabled {
		// Default values
		replicas := int32(1)
		if mcpServer.Spec.Replicas != nil {
			replicas = *mcpServer.Spec.Replicas
		}
		deployment.Spec.Replicas = &replicas
		logger.V(1).Info("Deployment built successfully", "replicas", replicas, "hpa_enabled", false)
	} else {
		// When HPA is enabled, use MinReplicas as initial replica count if specified
		if mcpServer.Spec.Autoscaling.HPA.MinReplicas != nil {
			deployment.Spec.Replicas = mcpServer.Spec.Autoscaling.HPA.MinReplicas
			logger.V(1).Info("Deployment built successfully", "initial_replicas", *mcpServer.Spec.Autoscaling.HPA.MinReplicas, "hpa_enabled", true)
		} else {
			// Use default of 1 if no MinReplicas specified
			replicas := int32(1)
			deployment.Spec.Replicas = &replicas
			logger.V(1).Info("Deployment built successfully", "initial_replicas", replicas, "hpa_enabled", true)
		}
	}

	return deployment
}

// BuildService builds a service for MCPServer
func (r *DefaultResourceBuilderService) BuildService(mcpServer *mcpv1.MCPServer) *corev1.Service {
	logger := log.Log.WithValues("mcpserver", mcpServer.Name, "namespace", mcpServer.Namespace)
	logger.V(1).Info("Building service")

	labels := r.buildLabels(mcpServer)
	selector := r.buildSelector(mcpServer)

	// Collect service ports based on gateway configuration
	var servicePorts []corev1.ServicePort

	if mcpServer.Spec.Gateway != nil && mcpServer.Spec.Gateway.Enabled {
		// If gateway is enabled, publish only the gateway port
		gatewayPort := mcpServer.Spec.Gateway.Port
		if gatewayPort == 0 {
			gatewayPort = 8080 // Default gateway port
		}
		servicePorts = []corev1.ServicePort{
			{
				Name:       "gateway",
				Port:       gatewayPort,
				TargetPort: intstr.FromInt32(gatewayPort),
				Protocol:   corev1.ProtocolTCP,
			},
		}
		logger.V(1).Info("Service configured for gateway", "gatewayPort", gatewayPort)
	} else {
		// Publish all ports from spec.ports[]
		containerPorts := r.collectContainerPorts(mcpServer)
		servicePorts = make([]corev1.ServicePort, len(containerPorts))

		for i, containerPort := range containerPorts {
			targetPort := containerPort.ContainerPort

			protocol := corev1.ProtocolTCP
			if containerPort.Protocol != "" {
				protocol = containerPort.Protocol
			}

			servicePorts[i] = corev1.ServicePort{
				Name:       containerPort.Name,
				Port:       containerPort.ContainerPort,
				TargetPort: intstr.FromInt32(targetPort),
				Protocol:   protocol,
			}
		}
		logger.V(1).Info("Service configured for all container ports", "portsCount", len(servicePorts))
	}

	// Fallback: if no ports configured, use default
	if len(servicePorts) == 0 {
		defaultPort := int32(8080)
		servicePorts = []corev1.ServicePort{
			{
				Name:       "mcp",
				Port:       defaultPort,
				TargetPort: intstr.FromInt32(defaultPort),
				Protocol:   corev1.ProtocolTCP,
			},
		}
		logger.V(1).Info("Using fallback default port", "port", defaultPort)
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
			Ports:    servicePorts,
		},
	}

	logger.V(1).Info("Service built successfully", "portsCount", len(servicePorts), "type", serviceType)
	return service
}

// BuildConfigMap builds config map for MCPServer
func (r *DefaultResourceBuilderService) BuildConfigMap(mcpServer *mcpv1.MCPServer) *corev1.ConfigMap {
	logger := log.Log.WithValues("mcpserver", mcpServer.Name, "namespace", mcpServer.Namespace)
	logger.V(1).Info("Building config map")

	labels := r.buildLabels(mcpServer)

	// Build config map data from config sources
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

	logger.V(1).Info("ConfigMap built successfully", "dataKeys", len(data))
	return configMap
}

// BuildSecret builds secret for MCPServer
func (r *DefaultResourceBuilderService) BuildSecret(mcpServer *mcpv1.MCPServer) *corev1.Secret {
	logger := log.Log.WithValues("mcpserver", mcpServer.Name, "namespace", mcpServer.Namespace)
	logger.V(1).Info("Building secret")

	labels := r.buildLabels(mcpServer)

	// Build secret data from secret references
	data := make(map[string][]byte)

	// Add any sensitive configuration or credentials
	// This is a placeholder - in practice, you might populate this from secretRefs
	for _, secretRef := range mcpServer.Spec.SecretRefs {
		// In a real implementation, you might want to copy data from referenced secrets
		// For now, we'll create a placeholder entry
		data[secretRef.Key] = []byte(fmt.Sprintf("secret-data-for-%s", secretRef.Key))
	}

	// Only create secret if there's data to store
	if len(data) == 0 {
		logger.V(1).Info("No secret data to store, skipping secret creation")
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

	logger.V(1).Info("Secret built successfully", "dataKeys", len(data))
	return secret
}

// BuildHPA builds horizontal pod autoscaler for MCPServer
func (r *DefaultResourceBuilderService) BuildHPA(mcpServer *mcpv1.MCPServer) *autoscalingv2.HorizontalPodAutoscaler {
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
			Behavior:    r.buildHPABehavior(hpaSpec),
		},
	}

	logger.V(1).Info("HPA built successfully", "minReplicas", minReplicas, "maxReplicas", hpaSpec.MaxReplicas)
	return hpa
}

// BuildVPA builds vertical pod autoscaler for MCPServer
func (r *DefaultResourceBuilderService) BuildVPA(mcpServer *mcpv1.MCPServer) *unstructured.Unstructured {
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

// BuildNetworkPolicy builds network policy for MCPServer with tenant isolation
func (r *DefaultResourceBuilderService) BuildNetworkPolicy(mcpServer *mcpv1.MCPServer) *networkingv1.NetworkPolicy {
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

	// Create tenant-specific pod selector for proper isolation
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

	// Add ingress rules if specified, otherwise use default tenant isolation rules
	if len(networkPolicySpec.IngressRules) > 0 {
		networkPolicy.Spec.PolicyTypes = append(networkPolicy.Spec.PolicyTypes, networkingv1.PolicyTypeIngress)
		networkPolicy.Spec.Ingress = r.buildIngressRules(networkPolicySpec.IngressRules)
	} else {
		// Default ingress rules for tenant isolation
		networkPolicy.Spec.PolicyTypes = append(networkPolicy.Spec.PolicyTypes, networkingv1.PolicyTypeIngress)
		networkPolicy.Spec.Ingress = r.buildDefaultIngressRules(mcpServer)
	}

	// Add egress rules if specified, otherwise use default tenant isolation rules
	if len(networkPolicySpec.EgressRules) > 0 {
		networkPolicy.Spec.PolicyTypes = append(networkPolicy.Spec.PolicyTypes, networkingv1.PolicyTypeEgress)
		networkPolicy.Spec.Egress = r.buildEgressRules(networkPolicySpec.EgressRules)
	} else {
		// Default egress rules for tenant isolation
		networkPolicy.Spec.PolicyTypes = append(networkPolicy.Spec.PolicyTypes, networkingv1.PolicyTypeEgress)
		networkPolicy.Spec.Egress = r.buildDefaultEgressRules(mcpServer)
	}

	logger.V(1).Info("Network policy built successfully")
	return networkPolicy
}

// Helper methods

// buildLabels builds standard labels for resources
func (r *DefaultResourceBuilderService) buildLabels(mcpServer *mcpv1.MCPServer) map[string]string {
	labels := map[string]string{
		"app.kubernetes.io/name":       "mcp-server",
		"app.kubernetes.io/instance":   mcpServer.Name,
		"app.kubernetes.io/component":  "server",
		"app.kubernetes.io/part-of":    "mcp-operator",
		"app.kubernetes.io/managed-by": "mcp-operator",
	}

	// Add registry information - use current fields only
	if mcpServer.Spec.Registry != nil {
		serverName := mcpServer.Spec.Registry.ServerName
		if serverName == "" {
			serverName = mcpServer.Name // default to MCPServer name
		}
		if serverName != "" {
			labels["mcp.allbeone.io/server-name"] = serverName
		}
	}
	// Version is stored in annotations, get it from there if needed
	if mcpServer.Annotations != nil && mcpServer.Annotations["mcp.allbeone.io/registry-version"] != "" {
		labels["mcp.allbeone.io/server-version"] = mcpServer.Annotations["mcp.allbeone.io/registry-version"]
	}

	// Add tenancy information
	if mcpServer.Spec.Tenancy != nil && mcpServer.Spec.Tenancy.TenantID != "" {
		labels["mcp.allbeone.io/tenant-id"] = mcpServer.Spec.Tenancy.TenantID
	}

	// Add custom labels
	for k, v := range mcpServer.Labels {
		if !r.isSystemLabel(k) {
			labels[k] = v
		}
	}

	return labels
}

// buildSelector builds selector labels for resources
func (r *DefaultResourceBuilderService) buildSelector(mcpServer *mcpv1.MCPServer) map[string]string {
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

// buildPodTemplate builds pod template for deployment
func (r *DefaultResourceBuilderService) buildPodTemplate(mcpServer *mcpv1.MCPServer, labels map[string]string) corev1.PodTemplateSpec {
	container := r.buildContainer(mcpServer)

	podSpec := corev1.PodSpec{
		Containers: []corev1.Container{container},
	}

	// Add service account
	if mcpServer.Spec.ServiceAccount != "" {
		podSpec.ServiceAccountName = mcpServer.Spec.ServiceAccount
	}

	// Add security context
	if mcpServer.Spec.SecurityContext != nil {
		podSpec.SecurityContext = &corev1.PodSecurityContext{
			RunAsUser:  mcpServer.Spec.SecurityContext.RunAsUser,
			RunAsGroup: mcpServer.Spec.SecurityContext.RunAsGroup,
		}
	}

	// Add node selector
	if len(mcpServer.Spec.NodeSelector) > 0 {
		podSpec.NodeSelector = mcpServer.Spec.NodeSelector
	}

	// Add tolerations
	if len(mcpServer.Spec.Tolerations) > 0 {
		podSpec.Tolerations = mcpServer.Spec.Tolerations
	}

	// Add affinity
	if mcpServer.Spec.Affinity != nil {
		podSpec.Affinity = mcpServer.Spec.Affinity
	}

	// Add volumes
	if len(mcpServer.Spec.Volumes) > 0 {
		podSpec.Volumes = r.buildVolumes(mcpServer.Spec.Volumes)
	}

	// Build annotations for pod template
	annotations := make(map[string]string)

	// Add Istio sidecar injection annotation if enabled via gateway.istio
	if mcpServer.Spec.Gateway != nil && mcpServer.Spec.Gateway.Istio != nil && mcpServer.Spec.Gateway.Istio.Enabled {
		// Default to true for sidecar injection when Istio is enabled
		annotations["sidecar.istio.io/inject"] = "true"
	}

	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels:      labels,
			Annotations: annotations,
		},
		Spec: podSpec,
	}
}

// collectContainerPorts collects all container ports from spec.ports[], legacy spec.port, and runtime.port
func (r *DefaultResourceBuilderService) collectContainerPorts(mcpServer *mcpv1.MCPServer) []corev1.ContainerPort {
	var containerPorts []corev1.ContainerPort

	// Skip ports for STDIO transport without gateway
	if mcpServer.Spec.Transport != nil && mcpServer.Spec.Transport.Type == "stdio" {
		if mcpServer.Spec.Gateway == nil || !mcpServer.Spec.Gateway.Enabled {
			return containerPorts // return empty ports
		}
	}

	// First priority: use spec.ports[] if provided
	if len(mcpServer.Spec.Ports) > 0 {
		for _, portSpec := range mcpServer.Spec.Ports {
			targetPort := portSpec.Port
			if portSpec.TargetPort > 0 {
				targetPort = portSpec.TargetPort
			}

			protocol := corev1.ProtocolTCP
			if portSpec.Protocol != "" {
				if portSpec.Protocol == "UDP" {
					protocol = corev1.ProtocolUDP
				}
			}

			containerPorts = append(containerPorts, corev1.ContainerPort{
				Name:          portSpec.Name,
				ContainerPort: targetPort,
				Protocol:      protocol,
			})
		}
		return containerPorts
	}

	// Second priority: convert legacy spec.port if provided
	if mcpServer.Spec.Port != nil && *mcpServer.Spec.Port > 0 {
		containerPorts = append(containerPorts, corev1.ContainerPort{
			Name:          "mcp",
			ContainerPort: *mcpServer.Spec.Port,
			Protocol:      corev1.ProtocolTCP,
		})
		return containerPorts
	}

	// Third priority: use runtime.port if provided
	if mcpServer.Spec.Runtime != nil && mcpServer.Spec.Runtime.Port > 0 {
		containerPorts = append(containerPorts, corev1.ContainerPort{
			Name:          "mcp",
			ContainerPort: mcpServer.Spec.Runtime.Port,
			Protocol:      corev1.ProtocolTCP,
		})
		return containerPorts
	}

	// Fallback: use default port 8080
	containerPorts = append(containerPorts, corev1.ContainerPort{
		Name:          "mcp",
		ContainerPort: 8080,
		Protocol:      corev1.ProtocolTCP,
	})

	return containerPorts
}

// buildContainer builds the main container for the pod
func (r *DefaultResourceBuilderService) buildContainer(mcpServer *mcpv1.MCPServer) corev1.Container {
	// Check if gateway is enabled with stdio transport - use gateway container
	if mcpServer.Spec.Gateway != nil && mcpServer.Spec.Gateway.Enabled &&
		mcpServer.Spec.Transport != nil && mcpServer.Spec.Transport.Type == "stdio" {
		return r.buildGatewayContainer(mcpServer)
	}

	// Check if runtime type is binary - use binary container with proxy
	if mcpServer.Spec.Runtime != nil && mcpServer.Spec.Runtime.Type == "binary" {
		return r.buildBinaryContainer(mcpServer)
	}

	// Collect container ports dynamically
	containerPorts := r.collectContainerPorts(mcpServer)

	container := corev1.Container{
		Name:  "mcp-server",
		Image: mcpServer.Spec.Runtime.Image,
		Ports: containerPorts,
		Env:   r.buildEnvironmentVariables(mcpServer, containerPorts),
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

	// Add volume mounts
	if len(mcpServer.Spec.VolumeMounts) > 0 {
		container.VolumeMounts = r.buildVolumeMounts(mcpServer.Spec.VolumeMounts)
	}

	// Add security context
	if mcpServer.Spec.SecurityContext != nil {
		container.SecurityContext = mcpServer.Spec.SecurityContext
	}

	// Add health checks (TCP probes on first container port)
	// Skip health checks for STDIO transport without gateway (no ports available)
	if len(containerPorts) > 0 {
		firstPort := containerPorts[0].ContainerPort

		container.LivenessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt32(firstPort),
				},
			},
			InitialDelaySeconds: 30,
			PeriodSeconds:       10,
			TimeoutSeconds:      5,
			FailureThreshold:    3,
			SuccessThreshold:    1,
		}

		container.ReadinessProbe = &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt32(firstPort),
				},
			},
			InitialDelaySeconds: 5,
			PeriodSeconds:       5,
			TimeoutSeconds:      3,
			FailureThreshold:    3,
			SuccessThreshold:    1,
		}
	}

	return container
}

// buildBinaryContainer builds a container for binary runtime that runs binary executable with TCP-to-STDIO proxy
func (r *DefaultResourceBuilderService) buildBinaryContainer(mcpServer *mcpv1.MCPServer) corev1.Container {
	runtime := mcpServer.Spec.Runtime

	// Use default proxy port if runtime port is not specified
	proxyPort := runtime.Port
	if proxyPort == 0 {
		proxyPort = 8080 // Default proxy port for binary runtime
	}

	// Build container ports for the proxy
	containerPorts := []corev1.ContainerPort{
		{
			Name:          "mcp",
			ContainerPort: proxyPort,
			Protocol:      corev1.ProtocolTCP,
		},
	}

	// For binary runtime, we need a base image that can run the proxy and the binary
	// If no image is specified, use a default base image with proxy capability
	image := runtime.Image
	if image == "" {
		image = "alpine:latest" // Default base image for binary runtime
	}

	// Build command to run the binary executable
	var containerCommand []string
	var containerArgs []string

	// If specific command is provided, use it
	if len(runtime.Command) > 0 {
		containerCommand = runtime.Command
		if len(runtime.Args) > 0 {
			containerArgs = runtime.Args
		}
	} else if len(runtime.Args) > 0 {
		// If only args provided, assume they contain the executable
		containerCommand = []string{runtime.Args[0]}
		if len(runtime.Args) > 1 {
			containerArgs = runtime.Args[1:]
		}
	} else {
		// Default command for binary runtime - this should be overridden
		containerCommand = []string{"/bin/sh", "-c", "echo 'Binary executable not specified'; sleep infinity"}
	}

	container := corev1.Container{
		Name:    "mcp-binary",
		Image:   image,
		Command: containerCommand,
		Args:    containerArgs,
		Ports:   containerPorts,
		Env:     r.buildBinaryEnvironmentVariables(mcpServer, runtime, proxyPort),
	}

	// Add resource requirements
	if !r.isResourceRequirementsEmpty(mcpServer.Spec.Resources) {
		container.Resources = r.buildResourceRequirements(mcpServer.Spec.Resources)
	}

	// Add volume mounts
	if len(mcpServer.Spec.VolumeMounts) > 0 {
		container.VolumeMounts = r.buildVolumeMounts(mcpServer.Spec.VolumeMounts)
	}

	// Add security context
	if mcpServer.Spec.SecurityContext != nil {
		container.SecurityContext = mcpServer.Spec.SecurityContext
	}

	// Add health checks - for binary runtime, use TCP probe on the proxy port
	container.LivenessProbe = &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			TCPSocket: &corev1.TCPSocketAction{
				Port: intstr.FromInt32(proxyPort),
			},
		},
		InitialDelaySeconds: 30,
		PeriodSeconds:       10,
		TimeoutSeconds:      5,
		FailureThreshold:    3,
		SuccessThreshold:    1,
	}

	container.ReadinessProbe = &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			TCPSocket: &corev1.TCPSocketAction{
				Port: intstr.FromInt32(proxyPort),
			},
		},
		InitialDelaySeconds: 5,
		PeriodSeconds:       5,
		TimeoutSeconds:      3,
		FailureThreshold:    3,
		SuccessThreshold:    1,
	}

	return container
}

// buildGatewayContainer builds a gateway container that runs STDIO server as subprocess
func (r *DefaultResourceBuilderService) buildGatewayContainer(mcpServer *mcpv1.MCPServer) corev1.Container {
	gateway := mcpServer.Spec.Gateway
	runtime := mcpServer.Spec.Runtime

	// Default gateway port if not specified
	gatewayPort := gateway.Port
	if gatewayPort == 0 {
		gatewayPort = 8000
	}

	// Build gateway container ports
	containerPorts := []corev1.ContainerPort{
		{
			Name:          "gateway",
			ContainerPort: gatewayPort,
			Protocol:      corev1.ProtocolTCP,
		},
	}

	// Build gateway args - replace <cmd> placeholder with runtime command
	var gatewayArgs []string
	if len(gateway.Args) > 0 {
		gatewayArgs = make([]string, len(gateway.Args))
		copy(gatewayArgs, gateway.Args)

		// Replace <cmd> placeholder with runtime command
		for i, arg := range gatewayArgs {
			if arg == "<cmd>" {
				// Build the subprocess command from runtime
				subprocessCmd := ""
				if len(runtime.Command) > 0 {
					subprocessCmd = runtime.Command[0]
					if len(runtime.Command) > 1 {
						subprocessCmd += " " + strings.Join(runtime.Command[1:], " ")
					}
				}
				if len(runtime.Args) > 0 {
					if subprocessCmd != "" {
						subprocessCmd += " " + strings.Join(runtime.Args, " ")
					} else {
						subprocessCmd = strings.Join(runtime.Args, " ")
					}
				}
				gatewayArgs[i] = subprocessCmd
			}
		}
	}

	container := corev1.Container{
		Name:  "gateway",
		Image: gateway.Image,
		Args:  gatewayArgs,
		Ports: containerPorts,
		Env:   r.buildGatewayEnvironmentVariables(mcpServer, runtime),
	}

	// Add resource requirements
	if !r.isResourceRequirementsEmpty(mcpServer.Spec.Resources) {
		container.Resources = r.buildResourceRequirements(mcpServer.Spec.Resources)
	}

	// Add volume mounts
	if len(mcpServer.Spec.VolumeMounts) > 0 {
		container.VolumeMounts = r.buildVolumeMounts(mcpServer.Spec.VolumeMounts)
	}

	// Add security context
	if mcpServer.Spec.SecurityContext != nil {
		container.SecurityContext = mcpServer.Spec.SecurityContext
	}

	// Add health checks (TCP probes on gateway port)
	container.LivenessProbe = &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			TCPSocket: &corev1.TCPSocketAction{
				Port: intstr.FromInt32(gatewayPort),
			},
		},
		InitialDelaySeconds: 30,
		PeriodSeconds:       10,
		TimeoutSeconds:      5,
		FailureThreshold:    3,
		SuccessThreshold:    1,
	}

	container.ReadinessProbe = &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			TCPSocket: &corev1.TCPSocketAction{
				Port: intstr.FromInt32(gatewayPort),
			},
		},
		InitialDelaySeconds: 5,
		PeriodSeconds:       5,
		TimeoutSeconds:      3,
		FailureThreshold:    3,
		SuccessThreshold:    1,
	}

	return container
}

// buildGatewayEnvironmentVariables builds environment variables for the gateway container
func (r *DefaultResourceBuilderService) buildGatewayEnvironmentVariables(mcpServer *mcpv1.MCPServer, runtime *mcpv1.RuntimeSpec) []corev1.EnvVar {
	envVars := []corev1.EnvVar{}

	// Add gateway environment variables
	if mcpServer.Spec.Gateway != nil {
		envVars = append(envVars, mcpServer.Spec.Gateway.Env...)
	}

	// Add runtime environment variables for the subprocess
	if runtime != nil {
		for k, v := range runtime.Env {
			envVars = append(envVars, corev1.EnvVar{
				Name:  k,
				Value: v,
			})
		}
	}

	// Add general environment variables
	envVars = append(envVars, mcpServer.Spec.Env...)

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
	}...)

	// Add gateway port
	gatewayPort := mcpServer.Spec.Gateway.Port
	if gatewayPort == 0 {
		gatewayPort = 8000
	}
	envVars = append(envVars, corev1.EnvVar{
		Name:  "GATEWAY_PORT",
		Value: strconv.Itoa(int(gatewayPort)),
	})

	return envVars
}

// buildBinaryEnvironmentVariables builds environment variables for binary runtime containers
func (r *DefaultResourceBuilderService) buildBinaryEnvironmentVariables(mcpServer *mcpv1.MCPServer, runtime *mcpv1.RuntimeSpec, proxyPort int32) []corev1.EnvVar {
	envVars := []corev1.EnvVar{}

	// Add runtime environment variables for the binary executable
	if runtime != nil {
		for k, v := range runtime.Env {
			envVars = append(envVars, corev1.EnvVar{
				Name:  k,
				Value: v,
			})
		}
	}

	// Add general environment variables
	envVars = append(envVars, mcpServer.Spec.Env...)

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
	}...)

	// Add binary proxy port environment variable
	envVars = append(envVars, corev1.EnvVar{
		Name:  "MCP_SERVER_PORT",
		Value: strconv.Itoa(int(proxyPort)),
	})

	// Add binary-specific environment variable to indicate runtime type
	envVars = append(envVars, corev1.EnvVar{
		Name:  "MCP_RUNTIME_TYPE",
		Value: "binary",
	})

	return envVars
}

// buildEnvironmentVariables builds environment variables for the container
func (r *DefaultResourceBuilderService) buildEnvironmentVariables(mcpServer *mcpv1.MCPServer, containerPorts []corev1.ContainerPort) []corev1.EnvVar {
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

	// Add environment variables from sources (EnvFrom loads all keys from ConfigMap/Secret)
	// Note: EnvFrom in our custom API loads all environment variables from the source,
	// not individual keys like in standard Kubernetes EnvVarSource

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
	}...)

	// Add MCP_SERVER_PORT using first container port
	if len(containerPorts) > 0 {
		envVars = append(envVars, corev1.EnvVar{
			Name:  "MCP_SERVER_PORT",
			Value: strconv.Itoa(int(containerPorts[0].ContainerPort)),
		})
	} else {
		// Fallback to default port if no container ports
		envVars = append(envVars, corev1.EnvVar{
			Name:  "MCP_SERVER_PORT",
			Value: "8080",
		})
	}

	return envVars
}

// buildResourceRequirements builds resource requirements for the container
func (r *DefaultResourceBuilderService) buildResourceRequirements(resources mcpv1.ResourceRequirements) corev1.ResourceRequirements {
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
func (r *DefaultResourceBuilderService) buildHPAMetrics(hpaSpec *mcpv1.HPASpec) []autoscalingv2.MetricSpec {
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

	// Add custom metrics if specified
	for _, customMetric := range hpaSpec.Metrics {
		metric := r.buildCustomMetric(customMetric)
		if metric != nil {
			metrics = append(metrics, *metric)
		}
	}

	// If no metrics are specified, default to 80% CPU utilization
	if len(metrics) == 0 {
		defaultCPUTarget := int32(80)
		metrics = append(metrics, autoscalingv2.MetricSpec{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceCPU,
				Target: autoscalingv2.MetricTarget{
					Type:               autoscalingv2.UtilizationMetricType,
					AverageUtilization: &defaultCPUTarget,
				},
			},
		})
	}

	return metrics
}

// buildCustomMetric builds a custom metric from MCPv1 MetricSpec
func (r *DefaultResourceBuilderService) buildCustomMetric(metricSpec mcpv1.MetricSpec) *autoscalingv2.MetricSpec {
	switch metricSpec.Type {
	case mcpv1.MetricTypeResource:
		if metricSpec.Resource != nil {
			return &autoscalingv2.MetricSpec{
				Type: autoscalingv2.ResourceMetricSourceType,
				Resource: &autoscalingv2.ResourceMetricSource{
					Name:   corev1.ResourceName(metricSpec.Resource.Name),
					Target: r.buildMetricTarget(metricSpec.Resource.Target),
				},
			}
		}
	case mcpv1.MetricTypePods:
		if metricSpec.Pods != nil {
			return &autoscalingv2.MetricSpec{
				Type: autoscalingv2.PodsMetricSourceType,
				Pods: &autoscalingv2.PodsMetricSource{
					Metric: autoscalingv2.MetricIdentifier{
						Name:     metricSpec.Pods.Metric.Name,
						Selector: metricSpec.Pods.Metric.Selector,
					},
					Target: r.buildMetricTarget(metricSpec.Pods.Target),
				},
			}
		}
	case mcpv1.MetricTypeObject:
		if metricSpec.Object != nil {
			return &autoscalingv2.MetricSpec{
				Type: autoscalingv2.ObjectMetricSourceType,
				Object: &autoscalingv2.ObjectMetricSource{
					DescribedObject: autoscalingv2.CrossVersionObjectReference{
						APIVersion: metricSpec.Object.DescribedObject.APIVersion,
						Kind:       metricSpec.Object.DescribedObject.Kind,
						Name:       metricSpec.Object.DescribedObject.Name,
					},
					Metric: autoscalingv2.MetricIdentifier{
						Name:     metricSpec.Object.Metric.Name,
						Selector: metricSpec.Object.Metric.Selector,
					},
					Target: r.buildMetricTarget(metricSpec.Object.Target),
				},
			}
		}
	case mcpv1.MetricTypeExternal:
		if metricSpec.External != nil {
			return &autoscalingv2.MetricSpec{
				Type: autoscalingv2.ExternalMetricSourceType,
				External: &autoscalingv2.ExternalMetricSource{
					Metric: autoscalingv2.MetricIdentifier{
						Name:     metricSpec.External.Metric.Name,
						Selector: metricSpec.External.Metric.Selector,
					},
					Target: r.buildMetricTarget(metricSpec.External.Target),
				},
			}
		}
	}
	return nil
}

// buildMetricTarget builds a metric target from MCPv1 MetricTarget
func (r *DefaultResourceBuilderService) buildMetricTarget(target mcpv1.MetricTarget) autoscalingv2.MetricTarget {
	metricTarget := autoscalingv2.MetricTarget{
		Type: autoscalingv2.MetricTargetType(target.Type),
	}

	if target.Value != nil {
		metricTarget.Value = target.Value
	}

	if target.AverageValue != nil {
		metricTarget.AverageValue = target.AverageValue
	}

	if target.AverageUtilization != nil {
		metricTarget.AverageUtilization = target.AverageUtilization
	}

	return metricTarget
}

// buildHPABehavior builds HPA behavior policies
func (r *DefaultResourceBuilderService) buildHPABehavior(hpaSpec *mcpv1.HPASpec) *autoscalingv2.HorizontalPodAutoscalerBehavior {
	if hpaSpec.Behavior == nil {
		return nil
	}

	behavior := &autoscalingv2.HorizontalPodAutoscalerBehavior{}

	// Build scale up behavior
	if hpaSpec.Behavior.ScaleUp != nil {
		behavior.ScaleUp = r.buildHPAScalingRules(hpaSpec.Behavior.ScaleUp)
	}

	// Build scale down behavior
	if hpaSpec.Behavior.ScaleDown != nil {
		behavior.ScaleDown = r.buildHPAScalingRules(hpaSpec.Behavior.ScaleDown)
	}

	return behavior
}

// buildHPAScalingRules builds HPA scaling rules
func (r *DefaultResourceBuilderService) buildHPAScalingRules(rules *mcpv1.HPAScalingRules) *autoscalingv2.HPAScalingRules {
	if rules == nil {
		return nil
	}

	scalingRules := &autoscalingv2.HPAScalingRules{}

	// Set stabilization window
	if rules.StabilizationWindowSeconds != nil {
		scalingRules.StabilizationWindowSeconds = rules.StabilizationWindowSeconds
	}

	// Set select policy
	if rules.SelectPolicy != nil {
		selectPolicy := autoscalingv2.ScalingPolicySelect(*rules.SelectPolicy)
		scalingRules.SelectPolicy = &selectPolicy
	}

	// Build policies
	if len(rules.Policies) > 0 {
		policies := make([]autoscalingv2.HPAScalingPolicy, len(rules.Policies))
		for i, policy := range rules.Policies {
			policies[i] = autoscalingv2.HPAScalingPolicy{
				Type:          autoscalingv2.HPAScalingPolicyType(policy.Type),
				Value:         policy.Value,
				PeriodSeconds: policy.PeriodSeconds,
			}
		}
		scalingRules.Policies = policies
	}

	return scalingRules
}

// buildIngressRules builds ingress rules for network policy
func (r *DefaultResourceBuilderService) buildIngressRules(rules []mcpv1.NetworkPolicyIngressRule) []networkingv1.NetworkPolicyIngressRule {
	ingressRules := []networkingv1.NetworkPolicyIngressRule{}

	for _, rule := range rules {
		ingressRule := networkingv1.NetworkPolicyIngressRule{
			Ports: r.buildNetworkPolicyPorts(rule.Ports),
			From:  r.buildNetworkPolicyPeers(rule.From),
		}
		ingressRules = append(ingressRules, ingressRule)
	}

	return ingressRules
}

// buildEgressRules builds egress rules for network policy
func (r *DefaultResourceBuilderService) buildEgressRules(rules []mcpv1.NetworkPolicyEgressRule) []networkingv1.NetworkPolicyEgressRule {
	egressRules := []networkingv1.NetworkPolicyEgressRule{}

	for _, rule := range rules {
		egressRule := networkingv1.NetworkPolicyEgressRule{
			Ports: r.buildNetworkPolicyPorts(rule.Ports),
			To:    r.buildNetworkPolicyPeers(rule.To),
		}
		egressRules = append(egressRules, egressRule)
	}

	return egressRules
}

// buildNetworkPolicyPorts builds network policy ports
func (r *DefaultResourceBuilderService) buildNetworkPolicyPorts(ports []mcpv1.NetworkPolicyPort) []networkingv1.NetworkPolicyPort {
	networkPorts := []networkingv1.NetworkPolicyPort{}

	for _, port := range ports {
		networkPort := networkingv1.NetworkPolicyPort{
			Protocol: port.Protocol,
			Port:     port.Port,
			EndPort:  port.EndPort,
		}
		networkPorts = append(networkPorts, networkPort)
	}

	return networkPorts
}

// buildNetworkPolicyPeers builds network policy peers
func (r *DefaultResourceBuilderService) buildNetworkPolicyPeers(peers []mcpv1.NetworkPolicyPeer) []networkingv1.NetworkPolicyPeer {
	networkPeers := []networkingv1.NetworkPolicyPeer{}

	for _, peer := range peers {
		networkPeer := networkingv1.NetworkPolicyPeer{
			PodSelector:       peer.PodSelector,
			NamespaceSelector: peer.NamespaceSelector,
		}

		if peer.IPBlock != nil {
			networkPeer.IPBlock = &networkingv1.IPBlock{
				CIDR:   peer.IPBlock.CIDR,
				Except: peer.IPBlock.Except,
			}
		}

		networkPeers = append(networkPeers, networkPeer)
	}

	return networkPeers
}

// buildDefaultIngressRules builds default ingress rules for tenant isolation
func (r *DefaultResourceBuilderService) buildDefaultIngressRules(mcpServer *mcpv1.MCPServer) []networkingv1.NetworkPolicyIngressRule {
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
func (r *DefaultResourceBuilderService) buildDefaultEgressRules(mcpServer *mcpv1.MCPServer) []networkingv1.NetworkPolicyEgressRule {
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

// buildVolumes builds volumes for the pod
func (r *DefaultResourceBuilderService) buildVolumes(volumes []mcpv1.Volume) []corev1.Volume {
	podVolumes := []corev1.Volume{}

	for _, volume := range volumes {
		podVolume := corev1.Volume{
			Name: volume.Name,
		}

		// Set volume source based on type (accessing through MCPVolumeSource inline struct)
		if volume.MCPVolumeSource.ConfigMap != nil {
			podVolume.ConfigMap = &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: volume.MCPVolumeSource.ConfigMap.Name,
				},
				Items:       r.convertKeyToPathItems(volume.MCPVolumeSource.ConfigMap.Items),
				DefaultMode: volume.MCPVolumeSource.ConfigMap.DefaultMode,
				Optional:    &volume.MCPVolumeSource.ConfigMap.Optional,
			}
		} else if volume.MCPVolumeSource.Secret != nil {
			podVolume.Secret = &corev1.SecretVolumeSource{
				SecretName:  volume.MCPVolumeSource.Secret.Name,
				Items:       r.convertKeyToPathItems(volume.MCPVolumeSource.Secret.Items),
				DefaultMode: volume.MCPVolumeSource.Secret.DefaultMode,
				Optional:    &volume.MCPVolumeSource.Secret.Optional,
			}
		} else if volume.MCPVolumeSource.EmptyDir != nil {
			emptyDirVolume := &corev1.EmptyDirVolumeSource{
				Medium: corev1.StorageMedium(volume.MCPVolumeSource.EmptyDir.Medium),
			}
			// Convert SizeLimit from *string to *resource.Quantity if present
			if volume.MCPVolumeSource.EmptyDir.SizeLimit != nil {
				if quantity, err := resource.ParseQuantity(*volume.MCPVolumeSource.EmptyDir.SizeLimit); err == nil {
					emptyDirVolume.SizeLimit = &quantity
				}
			}
			podVolume.EmptyDir = emptyDirVolume
		} else if volume.MCPVolumeSource.PersistentVolumeClaim != nil {
			podVolume.PersistentVolumeClaim = &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: volume.MCPVolumeSource.PersistentVolumeClaim.ClaimName,
				ReadOnly:  volume.MCPVolumeSource.PersistentVolumeClaim.ReadOnly,
			}
		} else if volume.MCPVolumeSource.HostPath != nil {
			hostPathVolume := &corev1.HostPathVolumeSource{
				Path: volume.MCPVolumeSource.HostPath.Path,
			}
			// Convert HostPathType if present
			if volume.MCPVolumeSource.HostPath.Type != nil {
				hostPathType := corev1.HostPathType(string(*volume.MCPVolumeSource.HostPath.Type))
				hostPathVolume.Type = &hostPathType
			}
			podVolume.HostPath = hostPathVolume
		}

		podVolumes = append(podVolumes, podVolume)
	}

	return podVolumes
}

// buildVolumeMounts builds volume mounts for the container
func (r *DefaultResourceBuilderService) buildVolumeMounts(volumeMounts []mcpv1.VolumeMount) []corev1.VolumeMount {
	containerVolumeMounts := []corev1.VolumeMount{}

	for _, volumeMount := range volumeMounts {
		containerVolumeMount := corev1.VolumeMount{
			Name:      volumeMount.Name,
			MountPath: volumeMount.MountPath,
			ReadOnly:  volumeMount.ReadOnly,
		}

		if volumeMount.SubPath != "" {
			containerVolumeMount.SubPath = volumeMount.SubPath
		}

		containerVolumeMounts = append(containerVolumeMounts, containerVolumeMount)
	}

	return containerVolumeMounts
}

// Helper utility methods

// convertKeyToPathItems converts custom KeyToPath items to Kubernetes KeyToPath items
func (r *DefaultResourceBuilderService) convertKeyToPathItems(items []mcpv1.KeyToPath) []corev1.KeyToPath {
	if len(items) == 0 {
		return nil
	}

	k8sItems := make([]corev1.KeyToPath, len(items))
	for i, item := range items {
		k8sItems[i] = corev1.KeyToPath{
			Key:  item.Key,
			Path: item.Path,
			Mode: item.Mode,
		}
	}
	return k8sItems
}

// isSystemLabel checks if a label is a system label
func (r *DefaultResourceBuilderService) isSystemLabel(key string) bool {
	systemPrefixes := []string{
		"app.kubernetes.io/",
		"mcp.allbeone.io/",
		"kubernetes.io/",
		"k8s.io/",
	}

	for _, prefix := range systemPrefixes {
		if len(key) >= len(prefix) && key[:len(prefix)] == prefix {
			return true
		}
	}

	return false
}

// isResourceRequirementsEmpty checks if resource requirements are empty
func (r *DefaultResourceBuilderService) isResourceRequirementsEmpty(resources mcpv1.ResourceRequirements) bool {
	return len(resources.Limits) == 0 && len(resources.Requests) == 0
}

// buildDeploymentStrategy builds deployment strategy configuration
func (r *DefaultResourceBuilderService) buildDeploymentStrategy(mcpServer *mcpv1.MCPServer) appsv1.DeploymentStrategy {
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

// BuildVirtualService builds Istio VirtualService for MCPServer
func (r *DefaultResourceBuilderService) BuildVirtualService(mcpServer *mcpv1.MCPServer) *unstructured.Unstructured {
	logger := log.Log.WithValues("mcpserver", mcpServer.Name, "namespace", mcpServer.Namespace)
	logger.V(1).Info("Building VirtualService")

	// Check if gateway.istio is enabled
	if mcpServer.Spec.Gateway == nil || mcpServer.Spec.Gateway.Istio == nil || !mcpServer.Spec.Gateway.Istio.Enabled {
		return nil
	}

	istioConfig := mcpServer.Spec.Gateway.Istio

	// Host is required
	if istioConfig.Host == "" {
		logger.V(1).Info("Skipping VirtualService - host not specified")
		return nil
	}

	// Set default path from transport.path or "/mcp"
	path := "/mcp" // Default path
	if mcpServer.Spec.Transport != nil && mcpServer.Spec.Transport.Path != "" {
		path = mcpServer.Spec.Transport.Path
	}

	// Set default gateway if not specified
	gateway := istioConfig.GatewayRef
	if gateway == "" {
		gateway = "default"
	}

	// Determine destination port: gateway.port or first server port if gateway not enabled
	var destinationPort int32
	if mcpServer.Spec.Gateway != nil && mcpServer.Spec.Gateway.Enabled && mcpServer.Spec.Gateway.Port > 0 {
		destinationPort = mcpServer.Spec.Gateway.Port
	} else {
		// Use first server port
		containerPorts := r.collectContainerPorts(mcpServer)
		if len(containerPorts) > 0 {
			destinationPort = containerPorts[0].ContainerPort
		} else {
			destinationPort = 8080 // fallback
		}
	}

	// Build VirtualService spec
	spec := map[string]interface{}{
		"hosts":    []string{istioConfig.Host},
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
								"number": destinationPort,
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

// BuildDestinationRule builds Istio DestinationRule for MCPServer
func (r *DefaultResourceBuilderService) BuildDestinationRule(mcpServer *mcpv1.MCPServer) *unstructured.Unstructured {
	logger := log.Log.WithValues("mcpserver", mcpServer.Name, "namespace", mcpServer.Namespace)
	logger.V(1).Info("Building DestinationRule")

	// Check if gateway.istio is enabled - DestinationRule is optional
	if mcpServer.Spec.Gateway == nil || mcpServer.Spec.Gateway.Istio == nil || !mcpServer.Spec.Gateway.Istio.Enabled {
		return nil
	}

	// Build simple DestinationRule spec with basic subset (without mTLS for first release)
	spec := map[string]interface{}{
		"host": fmt.Sprintf("%s.%s.svc.cluster.local", mcpServer.Name, mcpServer.Namespace),
		"subsets": []map[string]interface{}{
			{
				"name": "default",
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":     "mcp-server",
					"app.kubernetes.io/instance": mcpServer.Name,
				},
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
