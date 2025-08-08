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

	// Default values
	replicas := int32(1)
	if mcpServer.Spec.Replicas != nil {
		replicas = *mcpServer.Spec.Replicas
	}

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
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: selector,
			},
			Template: podTemplate,
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RollingUpdateDeploymentStrategyType,
				RollingUpdate: &appsv1.RollingUpdateDeployment{
					MaxUnavailable: &intstr.IntOrString{Type: intstr.String, StrVal: "25%"},
					MaxSurge:       &intstr.IntOrString{Type: intstr.String, StrVal: "25%"},
				},
			},
		},
	}

	logger.V(1).Info("Deployment built successfully", "replicas", replicas)
	return deployment
}

// BuildService builds a service for MCPServer
func (r *DefaultResourceBuilderService) BuildService(mcpServer *mcpv1.MCPServer) *corev1.Service {
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

	// Add registry information
	if mcpServer.Spec.Registry.Name != "" {
		labels["mcp.allbeone.io/server-name"] = mcpServer.Spec.Registry.Name
	}
	if mcpServer.Spec.Registry.Version != "" {
		labels["mcp.allbeone.io/server-version"] = mcpServer.Spec.Registry.Version
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

	return corev1.PodTemplateSpec{
		ObjectMeta: metav1.ObjectMeta{
			Labels: labels,
		},
		Spec: podSpec,
	}
}

// buildContainer builds the main container for the pod
func (r *DefaultResourceBuilderService) buildContainer(mcpServer *mcpv1.MCPServer) corev1.Container {
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

	// Add volume mounts
	if len(mcpServer.Spec.VolumeMounts) > 0 {
		container.VolumeMounts = r.buildVolumeMounts(mcpServer.Spec.VolumeMounts)
	}

	// Add security context
	if mcpServer.Spec.SecurityContext != nil {
		container.SecurityContext = mcpServer.Spec.SecurityContext
	}

	// Add health checks (liveness and readiness probes)
	container.LivenessProbe = &corev1.Probe{
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
	}

	container.ReadinessProbe = &corev1.Probe{
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
	}

	return container
}

// buildEnvironmentVariables builds environment variables for the container
func (r *DefaultResourceBuilderService) buildEnvironmentVariables(mcpServer *mcpv1.MCPServer) []corev1.EnvVar {
	envVars := []corev1.EnvVar{}

	// Add runtime environment variables
	for k, v := range mcpServer.Spec.Runtime.Env {
		envVars = append(envVars, corev1.EnvVar{
			Name:  k,
			Value: v,
		})
	}

	// Add general environment variables
	for k, v := range mcpServer.Spec.Environment {
		envVars = append(envVars, corev1.EnvVar{
			Name:  k,
			Value: v,
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
		{
			Name:  "MCP_SERVER_PORT",
			Value: strconv.Itoa(int(mcpServer.Spec.Runtime.Port)),
		},
	}...)

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

	return metrics
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
