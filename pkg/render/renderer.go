package render

import (
	"crypto/sha256"
	"fmt"
	"strings"

	mcpv1 "github.com/FantasyNitroGEN/mcp_operator/api/v1"
	"github.com/FantasyNitroGEN/mcp_operator/pkg/services"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

// RendererService defines the interface for rendering MCPServer resources
type RendererService interface {
	// RenderResources renders all resources for an MCPServer to YAML
	RenderResources(mcpServer *mcpv1.MCPServer) ([]client.Object, string, error)
	// CalculateHash calculates SHA256 hash of the rendered YAML
	CalculateHash(yamlContent string) string
	// AddRequiredLabels adds the required labels to a resource
	AddRequiredLabels(obj client.Object, mcpServer *mcpv1.MCPServer)
}

// DefaultRendererService implements the RendererService interface
type DefaultRendererService struct {
	resourceBuilder services.ResourceBuilderService
}

// NewDefaultRendererService creates a new renderer service
func NewDefaultRendererService(resourceBuilder services.ResourceBuilderService) *DefaultRendererService {
	return &DefaultRendererService{
		resourceBuilder: resourceBuilder,
	}
}

// RenderResources renders all resources for an MCPServer and returns them as objects and YAML
func (r *DefaultRendererService) RenderResources(mcpServer *mcpv1.MCPServer) ([]client.Object, string, error) {
	var resources []client.Object
	var yamlDocs []string

	// Build core resources

	// 1. Deployment (always created)
	deployment := r.resourceBuilder.BuildDeployment(mcpServer)
	if deployment != nil {
		r.AddRequiredLabels(deployment, mcpServer)
		resources = append(resources, deployment)

		yamlDoc, err := r.objectToYAML(deployment)
		if err != nil {
			return nil, "", fmt.Errorf("failed to serialize deployment to YAML: %w", err)
		}
		yamlDocs = append(yamlDocs, yamlDoc)
	}

	// 2. Service (conditional creation based on transport type and gateway settings)
	// Skip Service creation if transport.type==stdio and gateway.enabled!=true
	shouldSkipService := false
	if mcpServer.Spec.Transport != nil && mcpServer.Spec.Transport.Type == "stdio" {
		if mcpServer.Spec.Gateway == nil || !mcpServer.Spec.Gateway.Enabled {
			shouldSkipService = true
		}
	}

	if !shouldSkipService {
		service := r.resourceBuilder.BuildService(mcpServer)
		if service != nil {
			r.AddRequiredLabels(service, mcpServer)
			resources = append(resources, service)

			yamlDoc, err := r.objectToYAML(service)
			if err != nil {
				return nil, "", fmt.Errorf("failed to serialize service to YAML: %w", err)
			}
			yamlDocs = append(yamlDocs, yamlDoc)
		}
	}

	// 3. ConfigMap (if needed)
	configMap := r.resourceBuilder.BuildConfigMap(mcpServer)
	if configMap != nil {
		r.AddRequiredLabels(configMap, mcpServer)
		resources = append(resources, configMap)

		yamlDoc, err := r.objectToYAML(configMap)
		if err != nil {
			return nil, "", fmt.Errorf("failed to serialize configMap to YAML: %w", err)
		}
		yamlDocs = append(yamlDocs, yamlDoc)
	}

	// 4. Secret (if needed)
	secret := r.resourceBuilder.BuildSecret(mcpServer)
	if secret != nil {
		r.AddRequiredLabels(secret, mcpServer)
		resources = append(resources, secret)

		yamlDoc, err := r.objectToYAML(secret)
		if err != nil {
			return nil, "", fmt.Errorf("failed to serialize secret to YAML: %w", err)
		}
		yamlDocs = append(yamlDocs, yamlDoc)
	}

	// 3. HPA (if autoscaling is enabled)
	if mcpServer.Spec.Autoscaling != nil && mcpServer.Spec.Autoscaling.HPA != nil {
		hpa := r.resourceBuilder.BuildHPA(mcpServer)
		if hpa != nil {
			r.AddRequiredLabels(hpa, mcpServer)
			resources = append(resources, hpa)

			yamlDoc, err := r.objectToYAML(hpa)
			if err != nil {
				return nil, "", fmt.Errorf("failed to serialize HPA to YAML: %w", err)
			}
			yamlDocs = append(yamlDocs, yamlDoc)
		}
	}

	// 4. VPA (if autoscaling is enabled)
	if mcpServer.Spec.Autoscaling != nil && mcpServer.Spec.Autoscaling.VPA != nil {
		vpa := r.resourceBuilder.BuildVPA(mcpServer)
		if vpa != nil {
			r.AddRequiredLabels(vpa, mcpServer)
			resources = append(resources, vpa)

			yamlDoc, err := r.objectToYAML(vpa)
			if err != nil {
				return nil, "", fmt.Errorf("failed to serialize VPA to YAML: %w", err)
			}
			yamlDocs = append(yamlDocs, yamlDoc)
		}
	}

	// 5. NetworkPolicy (if multi-tenancy is enabled)
	if mcpServer.Spec.Tenancy != nil && mcpServer.Spec.Tenancy.NetworkPolicy != nil {
		networkPolicy := r.resourceBuilder.BuildNetworkPolicy(mcpServer)
		if networkPolicy != nil {
			r.AddRequiredLabels(networkPolicy, mcpServer)
			resources = append(resources, networkPolicy)

			yamlDoc, err := r.objectToYAML(networkPolicy)
			if err != nil {
				return nil, "", fmt.Errorf("failed to serialize NetworkPolicy to YAML: %w", err)
			}
			yamlDocs = append(yamlDocs, yamlDoc)
		}
	}

	// 6. Istio VirtualService (if gateway.istio is enabled)
	virtualService := r.resourceBuilder.BuildVirtualService(mcpServer)
	if virtualService != nil {
		r.AddRequiredLabels(virtualService, mcpServer)
		resources = append(resources, virtualService)

		yamlDoc, err := r.objectToYAML(virtualService)
		if err != nil {
			return nil, "", fmt.Errorf("failed to serialize VirtualService to YAML: %w", err)
		}
		yamlDocs = append(yamlDocs, yamlDoc)
	}

	// 7. Istio DestinationRule (if gateway.istio is enabled - optional)
	destinationRule := r.resourceBuilder.BuildDestinationRule(mcpServer)
	if destinationRule != nil {
		r.AddRequiredLabels(destinationRule, mcpServer)
		resources = append(resources, destinationRule)

		yamlDoc, err := r.objectToYAML(destinationRule)
		if err != nil {
			return nil, "", fmt.Errorf("failed to serialize DestinationRule to YAML: %w", err)
		}
		yamlDocs = append(yamlDocs, yamlDoc)
	}

	// Join all YAML documents with separator
	combinedYAML := strings.Join(yamlDocs, "\n---\n")

	return resources, combinedYAML, nil
}

// AddRequiredLabels adds the required labels to a resource
func (r *DefaultRendererService) AddRequiredLabels(obj client.Object, mcpServer *mcpv1.MCPServer) {
	labels := obj.GetLabels()
	if labels == nil {
		labels = make(map[string]string)
	}

	// Add required labels from the issue description
	labels["mcp.allbeone.io/name"] = mcpServer.Name
	labels["mcp.allbeone.io/registry"] = mcpServer.Spec.Registry.Registry
	labels["mcp.allbeone.io/server"] = mcpServer.Spec.Registry.ServerName

	obj.SetLabels(labels)
}

// CalculateHash calculates SHA256 hash of the rendered YAML
func (r *DefaultRendererService) CalculateHash(yamlContent string) string {
	hash := sha256.Sum256([]byte(yamlContent))
	return fmt.Sprintf("%x", hash)
}

// objectToYAML converts a Kubernetes object to YAML
func (r *DefaultRendererService) objectToYAML(obj runtime.Object) (string, error) {
	yamlBytes, err := yaml.Marshal(obj)
	if err != nil {
		return "", err
	}
	return string(yamlBytes), nil
}
