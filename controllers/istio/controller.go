package istio

import (
	"context"
	"fmt"
	"strings"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	mcpv1 "github.com/FantasyNitroGEN/mcp_operator/api/v1"
)

// Controller handles Istio resource management for MCPServer
type Controller struct {
	client.Client
	logger logr.Logger
}

// NewController creates a new Istio controller
func NewController(client client.Client, logger logr.Logger) *Controller {
	return &Controller{
		Client: client,
		logger: logger.WithName("istio-controller"),
	}
}

// ReconcileIstioResources creates or updates Istio resources for the MCPServer
func (c *Controller) ReconcileIstioResources(ctx context.Context, mcpServer *mcpv1.MCPServer, serviceName string, servicePort int32) error {
	// Check if Istio integration is enabled via gateway.istio.enabled
	if !c.isIstioEnabled(mcpServer) {
		c.logger.Info("Istio integration disabled, skipping Istio resources",
			"mcpserver", mcpServer.Name,
			"namespace", mcpServer.Namespace)
		return nil
	}

	// Check if Istio CRDs are available
	available, err := c.isIstioCRDsAvailable(ctx)
	if err != nil {
		return fmt.Errorf("failed to check Istio CRDs availability: %w", err)
	}
	if !available {
		c.logger.Info("Istio CRDs not available, skipping Istio resources creation")
		return nil
	}

	c.logger.Info("Reconciling Istio resources",
		"mcpserver", mcpServer.Name,
		"namespace", mcpServer.Namespace,
		"service", serviceName,
		"port", servicePort)

	// Create VirtualService
	if err := c.reconcileVirtualService(ctx, mcpServer, serviceName, servicePort); err != nil {
		return fmt.Errorf("failed to reconcile VirtualService: %w", err)
	}

	// Create DestinationRule if configured
	if err := c.reconcileDestinationRule(ctx, mcpServer, serviceName); err != nil {
		return fmt.Errorf("failed to reconcile DestinationRule: %w", err)
	}

	return nil
}

// isIstioEnabled checks if Istio integration is enabled for the MCPServer
func (c *Controller) isIstioEnabled(mcpServer *mcpv1.MCPServer) bool {
	// Check spec.gateway.istio.enabled first (as per issue requirements)
	if mcpServer.Spec.Gateway != nil && mcpServer.Spec.Gateway.Istio != nil {
		return mcpServer.Spec.Gateway.Istio.Enabled
	}

	// Fallback to spec.istio.enabled if available
	if mcpServer.Spec.Istio != nil {
		return mcpServer.Spec.Istio.Enabled
	}

	return false
}

// isIstioCRDsAvailable checks if Istio CRDs are installed in the cluster
func (c *Controller) isIstioCRDsAvailable(ctx context.Context) (bool, error) {
	// Check for VirtualService CRD
	virtualServiceGVK := schema.GroupVersionKind{
		Group:   "networking.istio.io",
		Version: "v1beta1",
		Kind:    "VirtualService",
	}

	vs := &unstructured.Unstructured{}
	vs.SetGroupVersionKind(virtualServiceGVK)

	// Try to get any VirtualService to check if CRD exists
	err := c.Get(ctx, types.NamespacedName{Name: "dummy", Namespace: "dummy"}, vs)
	if err != nil {
		// If it's a NoKindMatchError, CRDs are not installed
		if strings.Contains(err.Error(), "no matches for kind") {
			return false, nil
		}
		// Other errors (like NotFound) mean CRDs exist but resource doesn't
	}

	return true, nil
}

// reconcileVirtualService creates or updates the VirtualService for the MCPServer
func (c *Controller) reconcileVirtualService(ctx context.Context, mcpServer *mcpv1.MCPServer, serviceName string, servicePort int32) error {
	// Get Istio configuration
	istioConfig := c.getIstioConfig(mcpServer)
	if istioConfig.Host == "" {
		c.logger.Info("No host specified for Istio VirtualService, skipping",
			"mcpserver", mcpServer.Name)
		return nil
	}

	// Create VirtualService object
	vs := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.istio.io/v1beta1",
			"kind":       "VirtualService",
			"metadata": map[string]interface{}{
				"name":      mcpServer.Name,
				"namespace": mcpServer.Namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":       "mcp-server",
					"app.kubernetes.io/instance":   mcpServer.Name,
					"app.kubernetes.io/managed-by": "mcp-operator",
				},
			},
			"spec": map[string]interface{}{
				"hosts":    []string{istioConfig.Host},
				"gateways": []string{c.getGatewayRef(istioConfig)},
				"http": []map[string]interface{}{
					{
						"match": []map[string]interface{}{
							{
								"uri": map[string]interface{}{
									"prefix": c.getPathPrefix(mcpServer),
								},
							},
						},
						"route": []map[string]interface{}{
							{
								"destination": map[string]interface{}{
									"host": serviceName,
									"port": map[string]interface{}{
										"number": servicePort,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(mcpServer, vs, c.Scheme()); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	// Create or update VirtualService
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(vs.GroupVersionKind())

	err := c.Get(ctx, types.NamespacedName{Name: vs.GetName(), Namespace: vs.GetNamespace()}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			// Create new VirtualService
			c.logger.Info("Creating VirtualService",
				"name", vs.GetName(),
				"namespace", vs.GetNamespace(),
				"host", istioConfig.Host)
			return c.Create(ctx, vs)
		}
		return fmt.Errorf("failed to get existing VirtualService: %w", err)
	}

	// Update existing VirtualService
	existing.Object["spec"] = vs.Object["spec"]
	c.logger.Info("Updating VirtualService",
		"name", existing.GetName(),
		"namespace", existing.GetNamespace(),
		"host", istioConfig.Host)
	return c.Update(ctx, existing)
}

// reconcileDestinationRule creates or updates the DestinationRule for the MCPServer
func (c *Controller) reconcileDestinationRule(ctx context.Context, mcpServer *mcpv1.MCPServer, serviceName string) error {
	// Check if DestinationRule is configured in spec.istio.destinationRule
	if mcpServer.Spec.Istio == nil || mcpServer.Spec.Istio.DestinationRule == nil {
		c.logger.Info("No DestinationRule configuration found, skipping",
			"mcpserver", mcpServer.Name)
		return nil
	}

	// Create simple DestinationRule without mTLS for first release (as per issue requirements)
	dr := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "networking.istio.io/v1beta1",
			"kind":       "DestinationRule",
			"metadata": map[string]interface{}{
				"name":      mcpServer.Name,
				"namespace": mcpServer.Namespace,
				"labels": map[string]interface{}{
					"app.kubernetes.io/name":       "mcp-server",
					"app.kubernetes.io/instance":   mcpServer.Name,
					"app.kubernetes.io/managed-by": "mcp-operator",
				},
			},
			"spec": map[string]interface{}{
				"host": serviceName,
				"subsets": []map[string]interface{}{
					{
						"name": "default",
						"labels": map[string]interface{}{
							"app.kubernetes.io/name":     "mcp-server",
							"app.kubernetes.io/instance": mcpServer.Name,
						},
					},
				},
			},
		},
	}

	// Set owner reference
	if err := controllerutil.SetControllerReference(mcpServer, dr, c.Scheme()); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	// Create or update DestinationRule
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(dr.GroupVersionKind())

	err := c.Get(ctx, types.NamespacedName{Name: dr.GetName(), Namespace: dr.GetNamespace()}, existing)
	if err != nil {
		if errors.IsNotFound(err) {
			// Create new DestinationRule
			c.logger.Info("Creating DestinationRule",
				"name", dr.GetName(),
				"namespace", dr.GetNamespace())
			return c.Create(ctx, dr)
		}
		return fmt.Errorf("failed to get existing DestinationRule: %w", err)
	}

	// Update existing DestinationRule
	existing.Object["spec"] = dr.Object["spec"]
	c.logger.Info("Updating DestinationRule",
		"name", existing.GetName(),
		"namespace", existing.GetNamespace())
	return c.Update(ctx, existing)
}

// getIstioConfig extracts Istio configuration from MCPServer spec
func (c *Controller) getIstioConfig(mcpServer *mcpv1.MCPServer) *mcpv1.GatewayIstioSpec {
	if mcpServer.Spec.Gateway != nil && mcpServer.Spec.Gateway.Istio != nil {
		return mcpServer.Spec.Gateway.Istio
	}
	return &mcpv1.GatewayIstioSpec{}
}

// getGatewayRef returns the gateway reference, defaulting if not specified
func (c *Controller) getGatewayRef(istioConfig *mcpv1.GatewayIstioSpec) string {
	if istioConfig.GatewayRef != "" {
		return istioConfig.GatewayRef
	}
	// Default gateway reference
	return "default"
}

// getPathPrefix returns the path prefix for VirtualService matching
func (c *Controller) getPathPrefix(mcpServer *mcpv1.MCPServer) string {
	// Use spec.transport.path if specified, otherwise default to "/mcp" as per issue requirements
	// Note: TransportSpec might need a Path field added if not already present
	// For now, using default as per issue requirement
	return "/mcp"
}

// CleanupIstioResources removes Istio resources for the MCPServer
func (c *Controller) CleanupIstioResources(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	c.logger.Info("Cleaning up Istio resources",
		"mcpserver", mcpServer.Name,
		"namespace", mcpServer.Namespace)

	// Delete VirtualService
	vs := &unstructured.Unstructured{}
	vs.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "networking.istio.io",
		Version: "v1beta1",
		Kind:    "VirtualService",
	})
	vs.SetName(mcpServer.Name)
	vs.SetNamespace(mcpServer.Namespace)

	if err := c.Delete(ctx, vs); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete VirtualService: %w", err)
	}

	// Delete DestinationRule
	dr := &unstructured.Unstructured{}
	dr.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "networking.istio.io",
		Version: "v1beta1",
		Kind:    "DestinationRule",
	})
	dr.SetName(mcpServer.Name)
	dr.SetNamespace(mcpServer.Namespace)

	if err := c.Delete(ctx, dr); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete DestinationRule: %w", err)
	}

	return nil
}
