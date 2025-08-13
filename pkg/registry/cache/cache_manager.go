package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	mcpv1 "github.com/FantasyNitroGEN/mcp_operator/api/v1"
	"github.com/FantasyNitroGEN/mcp_operator/pkg/registry"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// DNS-1123 compliant name sanitizer
var dns1123Re = regexp.MustCompile(`[^a-z0-9\-\.]`)

func dns1123Name(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, "_", "-")
	s = strings.ReplaceAll(s, " ", "-")
	s = dns1123Re.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-.")
	if s == "" {
		s = "server"
	}
	if len(s) > 253 {
		s = s[:253]
	}
	return s
}

type CacheManager struct {
	client.Client
}

// NewCacheManager creates a new cache manager
func NewCacheManager(client client.Client) *CacheManager {
	return &CacheManager{
		Client: client,
	}
}

// CacheServer creates or updates a ConfigMap for a server
func (c *CacheManager) CacheServer(ctx context.Context, registryName string, serverInfo *registry.MCPServerInfo, serverSpec *registry.MCPServerSpec, mcpRegistry *mcpv1.MCPRegistry) error {
	logger := log.FromContext(ctx)

	// Marshal server.yaml content
	yamlContent, err := yaml.Marshal(serverSpec)
	if err != nil {
		return fmt.Errorf("failed to marshal server spec: %w", err)
	}

	orig := serverInfo.Name   // original name from directory
	safe := dns1123Name(orig) // DNS-1123 compliant name for resource
	configMapName := fmt.Sprintf("mcpregistry-%s-%s", registryName, safe)

	configMap := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{ // Required for SSA - fixes "invalid object type" error
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: mcpRegistry.Namespace,
			Labels: map[string]string{
				"mcp.allbeone.io/registry":  registryName,
				"mcp.allbeone.io/server":    safe, // use safe name for selectors
				"app.kubernetes.io/name":    "mcp-operator",
				"app.kubernetes.io/part-of": "mcp-registry",
			},
			Annotations: map[string]string{
				"mcp.allbeone.io/server-original": orig, // preserve human-readable original name
			},
		},
		Data: map[string]string{
			"server.yaml": string(yamlContent),
		},
	}

	// Set controller reference (owner reference)
	if err := controllerutil.SetControllerReference(mcpRegistry, configMap, c.Scheme()); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	logger.Info("Applying server ConfigMap using server-side apply",
		"configMap", configMapName,
		"registry", registryName,
		"server", orig,
		"namespace", mcpRegistry.Namespace)

	// Use server-side apply with Patch
	if err := c.Patch(ctx, configMap, client.Apply, client.FieldOwner("mcp-operator"), client.ForceOwnership); err != nil {
		return fmt.Errorf("failed to apply ConfigMap: %w", err)
	}

	return nil
}

// CacheIndex creates or updates the registry index ConfigMap (optional)
func (c *CacheManager) CacheIndex(ctx context.Context, registryName string, index *registry.RegistryIndex, mcpRegistry *mcpv1.MCPRegistry) error {
	logger := log.FromContext(ctx)

	// Marshal index content
	indexContent, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal registry index: %w", err)
	}

	configMapName := fmt.Sprintf("mcpregistry-%s-index", registryName)
	configMap := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{ // Required for SSA - fixes "invalid object type" error
			APIVersion: "v1",
			Kind:       "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: mcpRegistry.Namespace,
			Labels: map[string]string{
				"mcp.allbeone.io/registry":  registryName,
				"mcp.allbeone.io/type":      "index",
				"app.kubernetes.io/name":    "mcp-operator",
				"app.kubernetes.io/part-of": "mcp-registry",
			},
		},
		Data: map[string]string{
			"index.json": string(indexContent),
		},
	}

	// Set controller reference
	if err := controllerutil.SetControllerReference(mcpRegistry, configMap, c.Scheme()); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	logger.Info("Applying index ConfigMap using server-side apply",
		"configMap", configMapName,
		"registry", registryName,
		"namespace", mcpRegistry.Namespace)

	// Use server-side apply with Patch
	if err := c.Patch(ctx, configMap, client.Apply, client.FieldOwner("mcp-operator"), client.ForceOwnership); err != nil {
		return fmt.Errorf("failed to apply index ConfigMap: %w", err)
	}

	return nil
}

// CleanupOrphanedConfigMaps removes ConfigMaps for servers that no longer exist
func (c *CacheManager) CleanupOrphanedConfigMaps(ctx context.Context, registryName string, activeServers []string, mcpRegistry *mcpv1.MCPRegistry) error {
	logger := log.FromContext(ctx)

	// List all ConfigMaps for this registry
	configMaps := &corev1.ConfigMapList{}
	err := c.List(ctx, configMaps, client.InNamespace(mcpRegistry.Namespace), client.MatchingLabels{
		"mcp.allbeone.io/registry": registryName,
	})
	if err != nil {
		return fmt.Errorf("failed to list ConfigMaps: %w", err)
	}

	// Create a set of active server names for quick lookup
	activeServerSet := make(map[string]bool)
	for _, serverName := range activeServers {
		activeServerSet[serverName] = true
	}

	// Check each ConfigMap
	for _, cm := range configMaps.Items {
		serverName, exists := cm.Labels["mcp.allbeone.io/server"]
		if !exists {
			// Skip index ConfigMaps and other non-server ConfigMaps
			continue
		}

		if !activeServerSet[serverName] {
			// This ConfigMap is for a server that no longer exists
			logger.Info("Deleting orphaned server ConfigMap",
				"configMap", cm.Name,
				"registry", registryName,
				"server", serverName)

			if err := c.Delete(ctx, &cm); err != nil {
				logger.Error(err, "Failed to delete orphaned ConfigMap",
					"configMap", cm.Name,
					"registry", registryName,
					"server", serverName)
			}
		}
	}

	return nil
}

// ListCachedServers returns a list of cached server names for a registry
func (c *CacheManager) ListCachedServers(ctx context.Context, registryName string, namespace string) ([]string, error) {
	configMaps := &corev1.ConfigMapList{}
	err := c.List(ctx, configMaps, client.InNamespace(namespace), client.MatchingLabels{
		"mcp.allbeone.io/registry": registryName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list ConfigMaps: %w", err)
	}

	var servers []string
	for _, cm := range configMaps.Items {
		if serverName, exists := cm.Labels["mcp.allbeone.io/server"]; exists {
			servers = append(servers, serverName)
		}
	}

	return servers, nil
}

// CleanupRegistry removes all ConfigMaps associated with a registry
func (c *CacheManager) CleanupRegistry(ctx context.Context, registryName string, namespace string) error {
	logger := log.FromContext(ctx)

	// List all ConfigMaps for this registry
	configMaps := &corev1.ConfigMapList{}
	err := c.List(ctx, configMaps, client.InNamespace(namespace), client.MatchingLabels{
		"mcp.allbeone.io/registry": registryName,
	})
	if err != nil {
		return fmt.Errorf("failed to list ConfigMaps for registry %s: %w", registryName, err)
	}

	// Delete all ConfigMaps associated with this registry
	for _, cm := range configMaps.Items {
		logger.Info("Deleting registry ConfigMap",
			"configMap", cm.Name,
			"registry", registryName,
			"namespace", namespace)

		if err := c.Delete(ctx, &cm); err != nil {
			logger.Error(err, "Failed to delete ConfigMap",
				"configMap", cm.Name,
				"registry", registryName)
			// Continue deleting other ConfigMaps even if one fails
		}
	}

	logger.Info("Completed registry cleanup",
		"registry", registryName,
		"namespace", namespace,
		"deletedConfigMaps", len(configMaps.Items))

	return nil
}
