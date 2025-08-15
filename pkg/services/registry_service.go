package services

import (
	"context"
	"fmt"
	"os"

	mcpv1 "github.com/FantasyNitroGEN/mcp_operator/api/v1"
	"github.com/FantasyNitroGEN/mcp_operator/pkg/registry"
	"github.com/FantasyNitroGEN/mcp_operator/pkg/retry"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// DefaultRegistryService implements RegistryService interface
type DefaultRegistryService struct {
	registryClient *registry.Client
	client         client.Client
}

// getGitHubTokenFromSecret reads GitHub token from Kubernetes secret
// Supports both "token" and "GITHUB_TOKEN" keys for compatibility
func (r *DefaultRegistryService) getGitHubTokenFromSecret(ctx context.Context, secretRef *mcpv1.SecretReference, namespace string) (string, error) {
	if secretRef == nil {
		return "", nil
	}

	logger := log.FromContext(ctx).WithValues("secret", secretRef.Name, "namespace", namespace)

	secret := &corev1.Secret{}
	err := r.client.Get(ctx, types.NamespacedName{
		Name:      secretRef.Name,
		Namespace: namespace,
	}, secret)

	if err != nil {
		if errors.IsNotFound(err) && secretRef.Optional {
			logger.Info("Optional secret not found, continuing without token")
			return "", nil
		}
		return "", fmt.Errorf("failed to get secret %s/%s: %w", namespace, secretRef.Name, err)
	}

	// Try to get token using the specified key first
	if secretRef.Key != "" {
		if tokenBytes, exists := secret.Data[secretRef.Key]; exists {
			logger.Info("Found GitHub token using specified key", "key", secretRef.Key)
			return string(tokenBytes), nil
		}
	}

	// Try both "token" and "GITHUB_TOKEN" keys for compatibility
	supportedKeys := []string{"token", "GITHUB_TOKEN"}
	for _, key := range supportedKeys {
		if tokenBytes, exists := secret.Data[key]; exists {
			logger.Info("Found GitHub token using key", "key", key)
			return string(tokenBytes), nil
		}
	}

	if !secretRef.Optional {
		return "", fmt.Errorf("GitHub token not found in secret %s/%s, tried keys: %v", namespace, secretRef.Name, append([]string{secretRef.Key}, supportedKeys...))
	}

	logger.Info("GitHub token not found in optional secret, continuing without token")
	return "", nil
}

// getGitHubToken gets GitHub token with fallback logic:
// 1. Try to get from MCPRegistry auth.secretRef
// 2. Fall back to environment variable GITHUB_TOKEN
// 3. Return empty string if nothing found
func (r *DefaultRegistryService) getGitHubToken(ctx context.Context, mcpRegistry *mcpv1.MCPRegistry) string {
	logger := log.FromContext(ctx).WithValues("registry", mcpRegistry.Name)

	// First, try direct token from spec (not recommended for production)
	if mcpRegistry.Spec.Auth != nil && mcpRegistry.Spec.Auth.Token != "" {
		logger.Info("Using direct token from MCPRegistry spec (not recommended for production)")
		return mcpRegistry.Spec.Auth.Token
	}

	// Try to get token from secret reference
	if mcpRegistry.Spec.Auth != nil && mcpRegistry.Spec.Auth.SecretRef != nil {
		token, err := r.getGitHubTokenFromSecret(ctx, mcpRegistry.Spec.Auth.SecretRef, mcpRegistry.Namespace)
		if err != nil {
			logger.Error(err, "Failed to get GitHub token from secret, falling back to environment variable")
		} else if token != "" {
			logger.Info("Successfully retrieved GitHub token from secret")
			return token
		}
	}

	// Fall back to environment variable
	if envToken := os.Getenv("GITHUB_TOKEN"); envToken != "" {
		logger.Info("Using GitHub token from GITHUB_TOKEN environment variable")
		return envToken
	}

	logger.Info("No GitHub token found, proceeding without authentication")
	return ""
}

// getRegistryClientForRegistry creates a registry client with authentication from MCPRegistry
func (r *DefaultRegistryService) getRegistryClientForRegistry(ctx context.Context, mcpRegistry *mcpv1.MCPRegistry) *registry.Client {
	token := r.getGitHubToken(ctx, mcpRegistry)
	if token != "" {
		return registry.NewClientWithToken(token)
	}
	return registry.NewClient()
}

// NewDefaultRegistryService creates a new DefaultRegistryService
func NewDefaultRegistryService(client client.Client) *DefaultRegistryService {
	return &DefaultRegistryService{
		registryClient: registry.NewClient(),
		client:         client,
	}
}

// NewDefaultRegistryServiceWithRetryConfig creates a new DefaultRegistryService with custom GitHub retry configuration
func NewDefaultRegistryServiceWithRetryConfig(client client.Client, retryConfig retry.GitHubRetryConfig) *DefaultRegistryService {
	return &DefaultRegistryService{
		registryClient: registry.NewClientWithRetryConfig(retryConfig),
		client:         client,
	}
}

// FetchServerSpec fetches server specification from registry
func (r *DefaultRegistryService) FetchServerSpec(ctx context.Context, registryName, serverName string) (*registry.MCPServerSpec, error) {
	logger := log.FromContext(ctx).WithValues("registry", registryName, "server", serverName)
	logger.Info("Fetching server specification from registry")

	if r.registryClient == nil {
		return nil, fmt.Errorf("registry client is not initialized")
	}

	spec, err := r.registryClient.GetServerSpec(ctx, serverName)
	if err != nil {
		logger.Error(err, "Failed to fetch server specification")
		return nil, fmt.Errorf("failed to fetch server spec for %s: %w", serverName, err)
	}

	logger.Info("Successfully fetched server specification",
		"version", spec.Version,
		"image", spec.Runtime.Image)

	return spec, nil
}

// EnrichMCPServer enriches MCPServer with registry data
func (r *DefaultRegistryService) EnrichMCPServer(ctx context.Context, mcpServer *mcpv1.MCPServer, registryName string) error {
	logger := log.FromContext(ctx).WithValues("mcpserver", mcpServer.Name, "registry", registryName)
	logger.Info("Enriching MCPServer with registry data")

	// Initialize annotations if nil
	if mcpServer.Annotations == nil {
		mcpServer.Annotations = make(map[string]string)
	}

	// Skip if already enriched (check annotations instead of non-existent fields)
	if mcpServer.Annotations["mcp.allbeone.io/registry-version"] != "" && mcpServer.Spec.Runtime.Image != "" {
		logger.Info("MCPServer already enriched with registry data")
		return nil
	}

	// Fetch server specification - use ServerName first, fallback to deprecated Server field
	serverName := mcpServer.Spec.Registry.ServerName
	if serverName == "" {
		//nolint:staticcheck
		serverName = mcpServer.Spec.Registry.Server // fallback to deprecated field
	}
	spec, err := r.FetchServerSpec(ctx, registryName, serverName)
	if err != nil {
		return fmt.Errorf("failed to fetch server spec for enrichment: %w", err)
	}

	// Store registry information in annotations
	if mcpServer.Annotations["mcp.allbeone.io/registry-version"] == "" {
		mcpServer.Annotations["mcp.allbeone.io/registry-version"] = spec.Version
	}
	if mcpServer.Annotations["mcp.allbeone.io/registry-description"] == "" {
		mcpServer.Annotations["mcp.allbeone.io/registry-description"] = spec.Description
	}
	if mcpServer.Annotations["mcp.allbeone.io/registry-repository"] == "" {
		mcpServer.Annotations["mcp.allbeone.io/registry-repository"] = spec.Repository
	}
	if mcpServer.Annotations["mcp.allbeone.io/registry-license"] == "" {
		mcpServer.Annotations["mcp.allbeone.io/registry-license"] = spec.License
	}
	if mcpServer.Annotations["mcp.allbeone.io/registry-author"] == "" {
		mcpServer.Annotations["mcp.allbeone.io/registry-author"] = spec.Author
	}

	// Enrich runtime information
	if mcpServer.Spec.Runtime.Type == "" {
		mcpServer.Spec.Runtime.Type = spec.Runtime.Type
	}
	if mcpServer.Spec.Runtime.Image == "" {
		mcpServer.Spec.Runtime.Image = spec.Runtime.Image
	}
	if len(mcpServer.Spec.Runtime.Command) == 0 {
		mcpServer.Spec.Runtime.Command = spec.Runtime.Command
	}
	if len(mcpServer.Spec.Runtime.Args) == 0 {
		mcpServer.Spec.Runtime.Args = spec.Runtime.Args
	}

	// Enrich environment variables
	if mcpServer.Spec.Runtime.Env == nil {
		mcpServer.Spec.Runtime.Env = make(map[string]string)
	}
	for k, v := range spec.Runtime.Env {
		if _, exists := mcpServer.Spec.Runtime.Env[k]; !exists {
			mcpServer.Spec.Runtime.Env[k] = v
		}
	}

	// Store template digest in annotations
	if mcpServer.Annotations == nil {
		mcpServer.Annotations = make(map[string]string)
	}
	if spec.TemplateDigest != "" {
		mcpServer.Annotations["mcp.allbeone.io/template-digest"] = spec.TemplateDigest
	}

	logger.Info("Successfully enriched MCPServer with registry data",
		"version", spec.Version,
		"image", spec.Runtime.Image,
		"template_digest", spec.TemplateDigest)

	return nil
}

// ForceEnrichMCPServer enriches MCPServer with registry data, bypassing "already enriched" check
func (r *DefaultRegistryService) ForceEnrichMCPServer(ctx context.Context, mcpServer *mcpv1.MCPServer, registryName string) error {
	logger := log.FromContext(ctx).WithValues("mcpserver", mcpServer.Name, "registry", registryName)
	logger.Info("Force enriching MCPServer with registry data")

	// Initialize annotations if nil
	if mcpServer.Annotations == nil {
		mcpServer.Annotations = make(map[string]string)
	}

	// Fetch server specification (always fetch, no skip check) - use ServerName first, fallback to deprecated Server field
	serverName := mcpServer.Spec.Registry.ServerName
	if serverName == "" {
		//nolint:staticcheck
		serverName = mcpServer.Spec.Registry.Server // fallback to deprecated field
	}
	spec, err := r.FetchServerSpec(ctx, registryName, serverName)
	if err != nil {
		return fmt.Errorf("failed to fetch server spec for force enrichment: %w", err)
	}

	// Always store registry information in annotations (overwrite existing values)
	mcpServer.Annotations["mcp.allbeone.io/registry-version"] = spec.Version
	mcpServer.Annotations["mcp.allbeone.io/registry-description"] = spec.Description
	mcpServer.Annotations["mcp.allbeone.io/registry-repository"] = spec.Repository
	mcpServer.Annotations["mcp.allbeone.io/registry-license"] = spec.License
	mcpServer.Annotations["mcp.allbeone.io/registry-author"] = spec.Author

	// Always enrich runtime information (overwrite existing values)
	mcpServer.Spec.Runtime.Type = spec.Runtime.Type
	mcpServer.Spec.Runtime.Image = spec.Runtime.Image
	mcpServer.Spec.Runtime.Command = spec.Runtime.Command
	mcpServer.Spec.Runtime.Args = spec.Runtime.Args

	// Always enrich environment variables (overwrite existing values)
	if mcpServer.Spec.Runtime.Env == nil {
		mcpServer.Spec.Runtime.Env = make(map[string]string)
	}
	// Clear existing env vars and set new ones
	for k := range mcpServer.Spec.Runtime.Env {
		delete(mcpServer.Spec.Runtime.Env, k)
	}
	for k, v := range spec.Runtime.Env {
		mcpServer.Spec.Runtime.Env[k] = v
	}

	// Store template digest in annotations
	if mcpServer.Annotations == nil {
		mcpServer.Annotations = make(map[string]string)
	}
	if spec.TemplateDigest != "" {
		mcpServer.Annotations["mcp.allbeone.io/template-digest"] = spec.TemplateDigest
	}

	logger.Info("Successfully force enriched MCPServer with registry data",
		"version", spec.Version,
		"image", spec.Runtime.Image,
		"template_digest", spec.TemplateDigest)

	return nil
}

// SyncRegistry synchronizes registry data
func (r *DefaultRegistryService) SyncRegistry(ctx context.Context, registry *mcpv1.MCPRegistry) error {
	logger := log.FromContext(ctx).WithValues("registry", registry.Name)
	logger.Info("Synchronizing registry data")

	// Get authenticated registry client for this specific registry
	registryClient := r.getRegistryClientForRegistry(ctx, registry)

	// List available servers in the registry using the authenticated client
	servers, err := r.listAvailableServersWithClient(ctx, registry.Name, registryClient)
	if err != nil {
		return fmt.Errorf("failed to list available servers: %w", err)
	}

	// Update registry status with server information
	registry.Status.AvailableServers = int32(len(servers))

	// Convert to MCPServerInfo format for status
	serverInfos := make([]mcpv1.MCPServerInfo, len(servers))
	for i, server := range servers {
		serverInfos[i] = mcpv1.MCPServerInfo{
			Name:        server.Name,
			Description: fmt.Sprintf("Server from %s", server.Path),
		}
		if !server.UpdatedAt.IsZero() {
			lastUpdated := metav1.NewTime(server.UpdatedAt)
			serverInfos[i].LastUpdated = &lastUpdated
		}
	}
	registry.Status.ServerList = serverInfos

	logger.Info("Successfully synchronized registry data",
		"availableServers", len(servers))

	return nil
}

// listAvailableServersWithClient lists all available servers in a registry using the provided client
func (r *DefaultRegistryService) listAvailableServersWithClient(ctx context.Context, registryName string, registryClient *registry.Client) ([]registry.MCPServerInfo, error) {
	logger := log.FromContext(ctx).WithValues("registry", registryName)
	logger.Info("Listing available servers in registry")

	if registryClient == nil {
		return nil, fmt.Errorf("registry client is not initialized")
	}

	servers, err := registryClient.ListServers(ctx)
	if err != nil {
		logger.Error(err, "Failed to list servers from registry")
		return nil, fmt.Errorf("failed to list servers: %w", err)
	}

	logger.Info("Successfully listed servers from registry", "count", len(servers))
	return servers, nil
}

// ListAvailableServers lists all available servers in a registry using the default client (for backward compatibility)
func (r *DefaultRegistryService) ListAvailableServers(ctx context.Context, registryName string) ([]registry.MCPServerInfo, error) {
	return r.listAvailableServersWithClient(ctx, registryName, r.registryClient)
}

// ValidateRegistryConnection validates connection to registry
func (r *DefaultRegistryService) ValidateRegistryConnection(ctx context.Context, registry *mcpv1.MCPRegistry) error {
	logger := log.FromContext(ctx).WithValues("registry", registry.Name)
	logger.Info("Validating registry connection")

	// Get authenticated registry client for this specific registry
	registryClient := r.getRegistryClientForRegistry(ctx, registry)

	// Test connection by attempting to list servers
	_, err := registryClient.ListServers(ctx)
	if err != nil {
		logger.Error(err, "Registry connection validation failed")
		return fmt.Errorf("registry connection validation failed: %w", err)
	}

	logger.Info("Registry connection validation successful")
	return nil
}

// SetRegistryClient sets the registry client (useful for testing)
func (r *DefaultRegistryService) SetRegistryClient(client *registry.Client) {
	r.registryClient = client
}

// GetRegistryClient returns the current registry client
func (r *DefaultRegistryService) GetRegistryClient() *registry.Client {
	return r.registryClient
}

// EnrichMCPServerFromCache enriches MCPServer with registry data from ConfigMap cache
func (r *DefaultRegistryService) EnrichMCPServerFromCache(ctx context.Context, mcpServer *mcpv1.MCPServer, namespace string) error {
	logger := log.FromContext(ctx).WithValues(
		"mcpserver", mcpServer.Name,
		"registryName", mcpServer.Spec.Registry.Registry,
		"serverName", mcpServer.Spec.Registry.ServerName,
	)
	logger.Info("Enriching MCPServer from cache")

	// Validate required fields
	if mcpServer.Spec.Registry.Registry == "" {
		return fmt.Errorf("registry name is required for cache enrichment")
	}

	// Build ConfigMap name according to the specification
	reg := mcpServer.Spec.Registry
	server := reg.ServerName
	if server == "" {
		server = mcpServer.Name
	}

	// Find ConfigMap mcpregistry-<registryName>-<serverName>
	configMapName := fmt.Sprintf("mcpregistry-%s-%s", reg.Registry, server)
	configMap := &corev1.ConfigMap{}

	// Use the provided namespace parameter instead of mcpServer.Namespace
	ns := namespace
	if ns == "" {
		ns = mcpServer.Namespace
	}

	if err := r.client.Get(ctx, client.ObjectKey{
		Name:      configMapName,
		Namespace: ns,
	}, configMap); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("ConfigMap not found for server", "configMap", configMapName, "namespace", ns)
			return fmt.Errorf("registry cache not found: ConfigMap %s not found in namespace %s", configMapName, ns)
		}
		return fmt.Errorf("failed to get ConfigMap %s: %w", configMapName, err)
	}

	// Parse server.yaml from ConfigMap
	serverYAMLData, exists := configMap.Data["server.yaml"]
	if !exists {
		return fmt.Errorf("server.yaml not found in ConfigMap %s", configMapName)
	}

	// Parse the YAML data
	var spec registry.MCPServerSpec
	if err := yaml.Unmarshal([]byte(serverYAMLData), &spec); err != nil {
		return fmt.Errorf("failed to parse server.yaml from ConfigMap %s: %w", configMapName, err)
	}

	// Initialize Runtime if nil
	if mcpServer.Spec.Runtime == nil {
		mcpServer.Spec.Runtime = &mcpv1.RuntimeSpec{}
	}

	// Initialize annotations if nil
	if mcpServer.Annotations == nil {
		mcpServer.Annotations = make(map[string]string)
	}

	// Store registry information in annotations (without overwriting user-specified values)
	if mcpServer.Annotations["mcp.allbeone.io/registry-version"] == "" {
		mcpServer.Annotations["mcp.allbeone.io/registry-version"] = spec.Version
	}
	if mcpServer.Annotations["mcp.allbeone.io/registry-description"] == "" {
		mcpServer.Annotations["mcp.allbeone.io/registry-description"] = spec.Description
	}
	if mcpServer.Annotations["mcp.allbeone.io/registry-repository"] == "" {
		mcpServer.Annotations["mcp.allbeone.io/registry-repository"] = spec.Repository
	}
	if mcpServer.Annotations["mcp.allbeone.io/registry-license"] == "" {
		mcpServer.Annotations["mcp.allbeone.io/registry-license"] = spec.License
	}
	if mcpServer.Annotations["mcp.allbeone.io/registry-author"] == "" {
		mcpServer.Annotations["mcp.allbeone.io/registry-author"] = spec.Author
	}

	// Enrich runtime information (without overwriting user-specified values)
	if mcpServer.Spec.Runtime.Type == "" && spec.Runtime.Type != "" {
		mcpServer.Spec.Runtime.Type = spec.Runtime.Type
	}
	// For docker runtime, ensure image is populated from registry cache when not specified by user
	if (mcpServer.Spec.Runtime.Type == "docker" || mcpServer.Spec.Runtime.Type == "Docker") &&
		mcpServer.Spec.Runtime.Image == "" && spec.Runtime.Image != "" {
		mcpServer.Spec.Runtime.Image = spec.Runtime.Image
	}
	if len(mcpServer.Spec.Runtime.Command) == 0 {
		mcpServer.Spec.Runtime.Command = spec.Runtime.Command
	}
	if len(mcpServer.Spec.Runtime.Args) == 0 {
		mcpServer.Spec.Runtime.Args = spec.Runtime.Args
	}

	// Enrich environment variables (without overwriting user-specified values)
	if mcpServer.Spec.Runtime.Env == nil {
		mcpServer.Spec.Runtime.Env = make(map[string]string)
	}
	for k, v := range spec.Runtime.Env {
		if _, exists := mcpServer.Spec.Runtime.Env[k]; !exists {
			mcpServer.Spec.Runtime.Env[k] = v
		}
	}

	// Enrich transport information (without overwriting user-specified values)
	if mcpServer.Spec.Transport == nil && spec.Transport != nil {
		mcpServer.Spec.Transport = &mcpv1.TransportSpec{
			Type: spec.Transport.Type,
			Path: spec.Transport.Path,
		}
	} else if mcpServer.Spec.Transport != nil && spec.Transport != nil {
		// Enrich individual transport fields if they're empty
		if mcpServer.Spec.Transport.Type == "" {
			mcpServer.Spec.Transport.Type = spec.Transport.Type
		}
		if mcpServer.Spec.Transport.Path == "" {
			mcpServer.Spec.Transport.Path = spec.Transport.Path
		}
	}

	// Auto-detect transport type based on ports presence if not specified
	if mcpServer.Spec.Transport == nil || mcpServer.Spec.Transport.Type == "" {
		// Initialize transport if nil
		if mcpServer.Spec.Transport == nil {
			mcpServer.Spec.Transport = &mcpv1.TransportSpec{}
		}

		// Auto-detect transport type based on ports presence
		if len(spec.Ports) == 0 {
			// No ports in server.yaml → STDIO transport
			if mcpServer.Spec.Transport.Type == "" {
				mcpServer.Spec.Transport.Type = "stdio"
				logger.Info("Auto-detected STDIO transport (no ports in registry)", "serverName", mcpServer.Spec.Registry.ServerName)
			}
		} else {
			// Ports exist in server.yaml → HTTP transport
			if mcpServer.Spec.Transport.Type == "" {
				mcpServer.Spec.Transport.Type = "http"
				logger.Info("Auto-detected HTTP transport (ports found in registry)", "serverName", mcpServer.Spec.Registry.ServerName, "portsCount", len(spec.Ports))
			}
		}

		// Fill transport path from registry if available and not specified
		if mcpServer.Spec.Transport.Path == "" && spec.Transport != nil && spec.Transport.Path != "" {
			mcpServer.Spec.Transport.Path = spec.Transport.Path
		}
	}

	// Enrich ports information (without overwriting user-specified values)
	if len(mcpServer.Spec.Ports) == 0 && len(spec.Ports) > 0 {
		mcpServer.Spec.Ports = make([]mcpv1.PortSpec, len(spec.Ports))
		for i, port := range spec.Ports {
			mcpServer.Spec.Ports[i] = mcpv1.PortSpec{
				Name:        port.Name,
				Port:        port.Port,
				TargetPort:  port.TargetPort,
				Protocol:    port.Protocol,
				AppProtocol: port.AppProtocol,
			}
		}
	}

	// Store template digest in annotations
	if mcpServer.Annotations == nil {
		mcpServer.Annotations = make(map[string]string)
	}
	if spec.TemplateDigest != "" {
		mcpServer.Annotations["mcp.allbeone.io/template-digest"] = spec.TemplateDigest
	}

	logger.Info("Successfully enriched MCPServer from cache",
		"version", spec.Version,
		"image", spec.Runtime.Image,
		"template_digest", spec.TemplateDigest,
		"configMap", configMapName)

	return nil
}
