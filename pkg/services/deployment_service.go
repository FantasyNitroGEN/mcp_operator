package services

import (
	"context"
	"fmt"

	mcpv1 "github.com/FantasyNitroGEN/mcp_operator/api/v1"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// DefaultDeploymentService implements DeploymentService interface
type DefaultDeploymentService struct {
	client           client.Client
	resourceBuilder  ResourceBuilderService
	kubernetesClient KubernetesClientService
}

// NewDefaultDeploymentService creates a new DefaultDeploymentService
func NewDefaultDeploymentService(client client.Client, resourceBuilder ResourceBuilderService, kubernetesClient KubernetesClientService) *DefaultDeploymentService {
	return &DefaultDeploymentService{
		client:           client,
		resourceBuilder:  resourceBuilder,
		kubernetesClient: kubernetesClient,
	}
}

// CreateOrUpdateDeployment creates or updates a Kubernetes deployment
func (d *DefaultDeploymentService) CreateOrUpdateDeployment(ctx context.Context, mcpServer *mcpv1.MCPServer) (*appsv1.Deployment, error) {
	logger := log.FromContext(ctx).WithValues("mcpserver", mcpServer.Name, "namespace", mcpServer.Namespace)
	logger.Info("Creating or updating deployment")

	// Build the desired deployment
	deployment := d.resourceBuilder.BuildDeployment(mcpServer)

	// Set controller reference
	if err := controllerutil.SetControllerReference(mcpServer, deployment, d.client.Scheme()); err != nil {
		return nil, fmt.Errorf("failed to set controller reference: %w", err)
	}

	// Check if deployment already exists
	existing := &appsv1.Deployment{}
	key := types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}

	err := d.kubernetesClient.Get(ctx, key, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			// Create new deployment
			logger.Info("Creating new deployment")
			if err := d.kubernetesClient.Create(ctx, deployment); err != nil {
				return nil, fmt.Errorf("failed to create deployment: %w", err)
			}
			return deployment, nil
		}
		return nil, fmt.Errorf("failed to get existing deployment: %w", err)
	}

	// Update existing deployment
	logger.Info("Updating existing deployment")
	deployment.ResourceVersion = existing.ResourceVersion
	if err := d.kubernetesClient.Update(ctx, deployment); err != nil {
		return nil, fmt.Errorf("failed to update deployment: %w", err)
	}

	return deployment, nil
}

// CreateOrUpdateService creates or updates a Kubernetes service
func (d *DefaultDeploymentService) CreateOrUpdateService(ctx context.Context, mcpServer *mcpv1.MCPServer) (*corev1.Service, error) {
	logger := log.FromContext(ctx).WithValues("mcpserver", mcpServer.Name, "namespace", mcpServer.Namespace)
	logger.Info("Creating or updating service")

	// Build the desired service
	service := d.resourceBuilder.BuildService(mcpServer)

	// Set controller reference
	if err := controllerutil.SetControllerReference(mcpServer, service, d.client.Scheme()); err != nil {
		return nil, fmt.Errorf("failed to set controller reference: %w", err)
	}

	// Check if service already exists
	existing := &corev1.Service{}
	key := types.NamespacedName{Name: service.Name, Namespace: service.Namespace}

	err := d.kubernetesClient.Get(ctx, key, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			// Create new service
			logger.Info("Creating new service")
			if err := d.kubernetesClient.Create(ctx, service); err != nil {
				return nil, fmt.Errorf("failed to create service: %w", err)
			}
			return service, nil
		}
		return nil, fmt.Errorf("failed to get existing service: %w", err)
	}

	// Update existing service (preserve ClusterIP)
	logger.Info("Updating existing service")
	service.ResourceVersion = existing.ResourceVersion
	service.Spec.ClusterIP = existing.Spec.ClusterIP
	if err := d.kubernetesClient.Update(ctx, service); err != nil {
		return nil, fmt.Errorf("failed to update service: %w", err)
	}

	return service, nil
}

// CreateOrUpdateHPA creates or updates horizontal pod autoscaler
func (d *DefaultDeploymentService) CreateOrUpdateHPA(ctx context.Context, mcpServer *mcpv1.MCPServer) (*autoscalingv2.HorizontalPodAutoscaler, error) {
	logger := log.FromContext(ctx).WithValues("mcpserver", mcpServer.Name, "namespace", mcpServer.Namespace)
	logger.Info("Creating or updating HPA")

	// Build the desired HPA
	hpa := d.resourceBuilder.BuildHPA(mcpServer)
	if hpa == nil {
		return nil, fmt.Errorf("HPA configuration is not available")
	}

	// Set controller reference
	if err := controllerutil.SetControllerReference(mcpServer, hpa, d.client.Scheme()); err != nil {
		return nil, fmt.Errorf("failed to set controller reference: %w", err)
	}

	// Check if HPA already exists
	existing := &autoscalingv2.HorizontalPodAutoscaler{}
	key := types.NamespacedName{Name: hpa.Name, Namespace: hpa.Namespace}

	err := d.kubernetesClient.Get(ctx, key, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			// Create new HPA
			logger.Info("Creating new HPA")
			if err := d.kubernetesClient.Create(ctx, hpa); err != nil {
				return nil, fmt.Errorf("failed to create HPA: %w", err)
			}
			return hpa, nil
		}
		return nil, fmt.Errorf("failed to get existing HPA: %w", err)
	}

	// Update existing HPA
	logger.Info("Updating existing HPA")
	hpa.ResourceVersion = existing.ResourceVersion
	if err := d.kubernetesClient.Update(ctx, hpa); err != nil {
		return nil, fmt.Errorf("failed to update HPA: %w", err)
	}

	return hpa, nil
}

// CreateOrUpdateVPA creates or updates vertical pod autoscaler
func (d *DefaultDeploymentService) CreateOrUpdateVPA(ctx context.Context, mcpServer *mcpv1.MCPServer) (*unstructured.Unstructured, error) {
	logger := log.FromContext(ctx).WithValues("mcpserver", mcpServer.Name, "namespace", mcpServer.Namespace)
	logger.Info("Creating or updating VPA")

	// Build the desired VPA
	vpa := d.resourceBuilder.BuildVPA(mcpServer)
	if vpa == nil {
		return nil, fmt.Errorf("VPA configuration is not available")
	}

	// Set controller reference
	if err := controllerutil.SetControllerReference(mcpServer, vpa, d.client.Scheme()); err != nil {
		return nil, fmt.Errorf("failed to set controller reference: %w", err)
	}

	// Check if VPA already exists
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(vpa.GroupVersionKind())
	key := types.NamespacedName{Name: vpa.GetName(), Namespace: vpa.GetNamespace()}

	err := d.kubernetesClient.Get(ctx, key, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			// Create new VPA
			logger.Info("Creating new VPA")
			if err := d.kubernetesClient.Create(ctx, vpa); err != nil {
				return nil, fmt.Errorf("failed to create VPA: %w", err)
			}
			return vpa, nil
		}
		return nil, fmt.Errorf("failed to get existing VPA: %w", err)
	}

	// Update existing VPA
	logger.Info("Updating existing VPA")
	vpa.SetResourceVersion(existing.GetResourceVersion())
	if err := d.kubernetesClient.Update(ctx, vpa); err != nil {
		return nil, fmt.Errorf("failed to update VPA: %w", err)
	}

	return vpa, nil
}

// CreateOrUpdateNetworkPolicy creates or updates network policy
func (d *DefaultDeploymentService) CreateOrUpdateNetworkPolicy(ctx context.Context, mcpServer *mcpv1.MCPServer) (*networkingv1.NetworkPolicy, error) {
	logger := log.FromContext(ctx).WithValues("mcpserver", mcpServer.Name, "namespace", mcpServer.Namespace)
	logger.Info("Creating or updating network policy")

	// Build the desired network policy
	networkPolicy := d.resourceBuilder.BuildNetworkPolicy(mcpServer)
	if networkPolicy == nil {
		return nil, fmt.Errorf("NetworkPolicy configuration is not available")
	}

	// Set controller reference
	if err := controllerutil.SetControllerReference(mcpServer, networkPolicy, d.client.Scheme()); err != nil {
		return nil, fmt.Errorf("failed to set controller reference: %w", err)
	}

	// Check if network policy already exists
	existing := &networkingv1.NetworkPolicy{}
	key := types.NamespacedName{Name: networkPolicy.Name, Namespace: networkPolicy.Namespace}

	err := d.kubernetesClient.Get(ctx, key, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			// Create new network policy
			logger.Info("Creating new network policy")
			if err := d.kubernetesClient.Create(ctx, networkPolicy); err != nil {
				return nil, fmt.Errorf("failed to create network policy: %w", err)
			}
			return networkPolicy, nil
		}
		return nil, fmt.Errorf("failed to get existing network policy: %w", err)
	}

	// Update existing network policy
	logger.Info("Updating existing network policy")
	networkPolicy.ResourceVersion = existing.ResourceVersion
	if err := d.kubernetesClient.Update(ctx, networkPolicy); err != nil {
		return nil, fmt.Errorf("failed to update network policy: %w", err)
	}

	return networkPolicy, nil
}

// DeleteResources deletes all resources associated with MCPServer
func (d *DefaultDeploymentService) DeleteResources(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	logger := log.FromContext(ctx).WithValues("mcpserver", mcpServer.Name, "namespace", mcpServer.Namespace)
	logger.Info("Deleting all resources associated with MCPServer")

	// List of resources to delete
	resources := []client.Object{
		&appsv1.Deployment{},
		&corev1.Service{},
		&autoscalingv2.HorizontalPodAutoscaler{},
		&networkingv1.NetworkPolicy{},
	}

	// Delete each resource type
	for _, resource := range resources {
		resource.SetName(mcpServer.Name)
		resource.SetNamespace(mcpServer.Namespace)

		if err := d.kubernetesClient.Delete(ctx, resource); err != nil {
			if !errors.IsNotFound(err) {
				logger.Error(err, "Failed to delete resource", "type", fmt.Sprintf("%T", resource))
				// Continue with other resources even if one fails
			}
		} else {
			logger.Info("Successfully deleted resource", "type", fmt.Sprintf("%T", resource))
		}
	}

	// Delete VPA if it exists
	vpa := &unstructured.Unstructured{}
	vpa.SetGroupVersionKind(d.getVPAGroupVersionKind())
	vpa.SetName(mcpServer.Name)
	vpa.SetNamespace(mcpServer.Namespace)

	if err := d.kubernetesClient.Delete(ctx, vpa); err != nil {
		if !errors.IsNotFound(err) {
			logger.Error(err, "Failed to delete VPA")
		}
	} else {
		logger.Info("Successfully deleted VPA")
	}

	return nil
}

// GetDeploymentStatus gets the current status of deployment
func (d *DefaultDeploymentService) GetDeploymentStatus(ctx context.Context, mcpServer *mcpv1.MCPServer) (*appsv1.Deployment, error) {
	logger := log.FromContext(ctx).WithValues("mcpserver", mcpServer.Name, "namespace", mcpServer.Namespace)
	logger.V(1).Info("Getting deployment status")

	deployment := &appsv1.Deployment{}
	key := types.NamespacedName{Name: mcpServer.Name, Namespace: mcpServer.Namespace}

	if err := d.kubernetesClient.Get(ctx, key, deployment); err != nil {
		return nil, fmt.Errorf("failed to get deployment status: %w", err)
	}

	return deployment, nil
}

// getVPAGroupVersionKind returns the GroupVersionKind for VPA
func (d *DefaultDeploymentService) getVPAGroupVersionKind() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   "autoscaling.k8s.io",
		Version: "v1",
		Kind:    "VerticalPodAutoscaler",
	}
}
