package istio

import (
	mcpv1 "github.com/FantasyNitroGEN/mcp_operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
)

const (
	// IstioSidecarInjectAnnotation is the annotation key for Istio sidecar injection
	IstioSidecarInjectAnnotation = "sidecar.istio.io/inject"
)

// AddIstioAnnotations adds Istio-related annotations to the deployment based on MCPServer configuration
func AddIstioAnnotations(deployment *appsv1.Deployment, mcpServer *mcpv1.MCPServer) {
	if !isIstioAnnotationsEnabled(mcpServer) {
		return
	}

	// Initialize annotations map if it doesn't exist
	if deployment.Spec.Template.Annotations == nil {
		deployment.Spec.Template.Annotations = make(map[string]string)
	}

	// Add sidecar injection annotation based on configuration
	injectValue := getSidecarInjectValue(mcpServer)
	if injectValue != "" {
		deployment.Spec.Template.Annotations[IstioSidecarInjectAnnotation] = injectValue
	}
}

// isIstioAnnotationsEnabled checks if Istio annotations should be added
func isIstioAnnotationsEnabled(mcpServer *mcpv1.MCPServer) bool {
	// Check if Istio is enabled via gateway.istio.enabled (as per issue requirements)
	if mcpServer.Spec.Gateway != nil && mcpServer.Spec.Gateway.Istio != nil {
		return mcpServer.Spec.Gateway.Istio.Enabled
	}

	// Fallback to spec.istio.enabled
	if mcpServer.Spec.Istio != nil {
		return mcpServer.Spec.Istio.Enabled
	}

	return false
}

// getSidecarInjectValue returns the value for the sidecar injection annotation
func getSidecarInjectValue(mcpServer *mcpv1.MCPServer) string {
	// Check gateway.istio configuration first
	if mcpServer.Spec.Gateway != nil && mcpServer.Spec.Gateway.Istio != nil && mcpServer.Spec.Gateway.Istio.Enabled {
		// For gateway.istio.enabled, always inject sidecar as per issue requirements
		return "true"
	}

	// Check spec.istio.sidecarInject configuration
	if mcpServer.Spec.Istio != nil && mcpServer.Spec.Istio.Enabled {
		if mcpServer.Spec.Istio.SidecarInject != nil {
			if *mcpServer.Spec.Istio.SidecarInject {
				return "true"
			} else {
				return "false"
			}
		}
		// Default to true if sidecarInject is not specified but Istio is enabled
		return "true"
	}

	return ""
}

// RemoveIstioAnnotations removes Istio-related annotations from the deployment
func RemoveIstioAnnotations(deployment *appsv1.Deployment) {
	if deployment.Spec.Template.Annotations == nil {
		return
	}

	delete(deployment.Spec.Template.Annotations, IstioSidecarInjectAnnotation)
}
