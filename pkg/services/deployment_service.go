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

	// Delete Istio resources if they exist
	if err := d.DeleteIstioResources(ctx, mcpServer); err != nil {
		logger.Error(err, "Failed to delete Istio resources")
		// Continue with deletion even if Istio resource deletion fails
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

// CreateOrUpdateVirtualService creates or updates Istio VirtualService
func (d *DefaultDeploymentService) CreateOrUpdateVirtualService(ctx context.Context, mcpServer *mcpv1.MCPServer) (*unstructured.Unstructured, error) {
	logger := log.FromContext(ctx).WithValues("mcpserver", mcpServer.Name, "namespace", mcpServer.Namespace)
	logger.Info("Creating or updating Istio VirtualService")

	// Check if Istio is enabled for this MCPServer
	if mcpServer.Spec.Istio == nil || !mcpServer.Spec.Istio.Enabled || mcpServer.Spec.Istio.VirtualService == nil {
		return nil, fmt.Errorf("Istio VirtualService configuration is not available or not enabled")
	}

	// Check if Istio CRDs are available
	available, err := d.IsIstioAvailable(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check Istio availability: %w", err)
	}
	if !available {
		logger.Info("Istio CRDs not available, skipping VirtualService creation")
		return nil, fmt.Errorf("Istio CRDs are not available in the cluster")
	}

	// Build the desired VirtualService
	virtualService := d.resourceBuilder.BuildVirtualService(mcpServer)
	if virtualService == nil {
		return nil, fmt.Errorf("VirtualService configuration is not available")
	}

	// Set controller reference
	if err := controllerutil.SetControllerReference(mcpServer, virtualService, d.client.Scheme()); err != nil {
		return nil, fmt.Errorf("failed to set controller reference: %w", err)
	}

	// Check if VirtualService already exists
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(virtualService.GroupVersionKind())
	key := types.NamespacedName{Name: virtualService.GetName(), Namespace: virtualService.GetNamespace()}

	err = d.kubernetesClient.Get(ctx, key, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			// Create new VirtualService
			logger.Info("Creating new VirtualService")
			if err := d.kubernetesClient.Create(ctx, virtualService); err != nil {
				return nil, fmt.Errorf("failed to create VirtualService: %w", err)
			}
			return virtualService, nil
		}
		return nil, fmt.Errorf("failed to get existing VirtualService: %w", err)
	}

	// Update existing VirtualService
	logger.Info("Updating existing VirtualService")
	virtualService.SetResourceVersion(existing.GetResourceVersion())
	if err := d.kubernetesClient.Update(ctx, virtualService); err != nil {
		return nil, fmt.Errorf("failed to update VirtualService: %w", err)
	}

	return virtualService, nil
}

// CreateOrUpdateDestinationRule creates or updates Istio DestinationRule
func (d *DefaultDeploymentService) CreateOrUpdateDestinationRule(ctx context.Context, mcpServer *mcpv1.MCPServer) (*unstructured.Unstructured, error) {
	logger := log.FromContext(ctx).WithValues("mcpserver", mcpServer.Name, "namespace", mcpServer.Namespace)
	logger.Info("Creating or updating Istio DestinationRule")

	// Check if Istio is enabled for this MCPServer
	if mcpServer.Spec.Istio == nil || !mcpServer.Spec.Istio.Enabled || mcpServer.Spec.Istio.DestinationRule == nil {
		return nil, fmt.Errorf("Istio DestinationRule configuration is not available or not enabled")
	}

	// Check if Istio CRDs are available
	available, err := d.IsIstioAvailable(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to check Istio availability: %w", err)
	}
	if !available {
		logger.Info("Istio CRDs not available, skipping DestinationRule creation")
		return nil, fmt.Errorf("Istio CRDs are not available in the cluster")
	}

	// Build the desired DestinationRule
	destinationRule := d.resourceBuilder.BuildDestinationRule(mcpServer)
	if destinationRule == nil {
		return nil, fmt.Errorf("DestinationRule configuration is not available")
	}

	// Set controller reference
	if err := controllerutil.SetControllerReference(mcpServer, destinationRule, d.client.Scheme()); err != nil {
		return nil, fmt.Errorf("failed to set controller reference: %w", err)
	}

	// Check if DestinationRule already exists
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(destinationRule.GroupVersionKind())
	key := types.NamespacedName{Name: destinationRule.GetName(), Namespace: destinationRule.GetNamespace()}

	err = d.kubernetesClient.Get(ctx, key, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			// Create new DestinationRule
			logger.Info("Creating new DestinationRule")
			if err := d.kubernetesClient.Create(ctx, destinationRule); err != nil {
				return nil, fmt.Errorf("failed to create DestinationRule: %w", err)
			}
			return destinationRule, nil
		}
		return nil, fmt.Errorf("failed to get existing DestinationRule: %w", err)
	}

	// Update existing DestinationRule
	logger.Info("Updating existing DestinationRule")
	destinationRule.SetResourceVersion(existing.GetResourceVersion())
	if err := d.kubernetesClient.Update(ctx, destinationRule); err != nil {
		return nil, fmt.Errorf("failed to update DestinationRule: %w", err)
	}

	return destinationRule, nil
}

// DeleteIstioResources deletes Istio resources associated with MCPServer
func (d *DefaultDeploymentService) DeleteIstioResources(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	logger := log.FromContext(ctx).WithValues("mcpserver", mcpServer.Name, "namespace", mcpServer.Namespace)
	logger.Info("Deleting Istio resources associated with MCPServer")

	// Check if Istio CRDs are available
	available, err := d.IsIstioAvailable(ctx)
	if err != nil {
		logger.Error(err, "Failed to check Istio availability, skipping Istio resource deletion")
		return nil // Don't fail the deletion if we can't check Istio availability
	}
	if !available {
		logger.Info("Istio CRDs not available, skipping Istio resource deletion")
		return nil
	}

	// Delete VirtualService
	virtualService := &unstructured.Unstructured{}
	virtualService.SetGroupVersionKind(d.getVirtualServiceGroupVersionKind())
	virtualService.SetName(mcpServer.Name)
	virtualService.SetNamespace(mcpServer.Namespace)

	if err := d.kubernetesClient.Delete(ctx, virtualService); err != nil {
		if !errors.IsNotFound(err) {
			logger.Error(err, "Failed to delete VirtualService")
		}
	} else {
		logger.Info("Successfully deleted VirtualService")
	}

	// Delete DestinationRule
	destinationRule := &unstructured.Unstructured{}
	destinationRule.SetGroupVersionKind(d.getDestinationRuleGroupVersionKind())
	destinationRule.SetName(mcpServer.Name)
	destinationRule.SetNamespace(mcpServer.Namespace)

	if err := d.kubernetesClient.Delete(ctx, destinationRule); err != nil {
		if !errors.IsNotFound(err) {
			logger.Error(err, "Failed to delete DestinationRule")
		}
	} else {
		logger.Info("Successfully deleted DestinationRule")
	}

	return nil
}

// IsIstioAvailable checks if Istio CRDs are available in the cluster
func (d *DefaultDeploymentService) IsIstioAvailable(ctx context.Context) (bool, error) {
	logger := log.FromContext(ctx)
	logger.V(1).Info("Checking if Istio CRDs are available")

	// Check for VirtualService CRD
	virtualServiceCRD := &unstructured.Unstructured{}
	virtualServiceCRD.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apiextensions.k8s.io",
		Version: "v1",
		Kind:    "CustomResourceDefinition",
	})
	virtualServiceCRD.SetName("virtualservices.networking.istio.io")

	if err := d.kubernetesClient.Get(ctx, types.NamespacedName{Name: "virtualservices.networking.istio.io"}, virtualServiceCRD); err != nil {
		if errors.IsNotFound(err) {
			logger.V(1).Info("VirtualService CRD not found, Istio not available")
			return false, nil
		}
		return false, fmt.Errorf("failed to check VirtualService CRD: %w", err)
	}

	// Check for DestinationRule CRD
	destinationRuleCRD := &unstructured.Unstructured{}
	destinationRuleCRD.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "apiextensions.k8s.io",
		Version: "v1",
		Kind:    "CustomResourceDefinition",
	})
	destinationRuleCRD.SetName("destinationrules.networking.istio.io")

	if err := d.kubernetesClient.Get(ctx, types.NamespacedName{Name: "destinationrules.networking.istio.io"}, destinationRuleCRD); err != nil {
		if errors.IsNotFound(err) {
			logger.V(1).Info("DestinationRule CRD not found, Istio not available")
			return false, nil
		}
		return false, fmt.Errorf("failed to check DestinationRule CRD: %w", err)
	}

	logger.V(1).Info("Istio CRDs are available")
	return true, nil
}

// getVPAGroupVersionKind returns the GroupVersionKind for VPA
func (d *DefaultDeploymentService) getVPAGroupVersionKind() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   "autoscaling.k8s.io",
		Version: "v1",
		Kind:    "VerticalPodAutoscaler",
	}
}

// getVirtualServiceGroupVersionKind returns the GroupVersionKind for Istio VirtualService
func (d *DefaultDeploymentService) getVirtualServiceGroupVersionKind() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   "networking.istio.io",
		Version: "v1beta1",
		Kind:    "VirtualService",
	}
}

// getDestinationRuleGroupVersionKind returns the GroupVersionKind for Istio DestinationRule
func (d *DefaultDeploymentService) getDestinationRuleGroupVersionKind() schema.GroupVersionKind {
	return schema.GroupVersionKind{
		Group:   "networking.istio.io",
		Version: "v1beta1",
		Kind:    "DestinationRule",
	}
}
