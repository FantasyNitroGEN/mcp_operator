package services

import (
	"context"
	"fmt"

	mcpv1 "github.com/FantasyNitroGEN/mcp_operator/api/v1"
	"github.com/FantasyNitroGEN/mcp_operator/pkg/registry"
	"github.com/FantasyNitroGEN/mcp_operator/pkg/retry"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// DefaultRegistryService implements RegistryService interface
type DefaultRegistryService struct {
	registryClient *registry.Client
	client         client.Client
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

	// Skip if already enriched
	if mcpServer.Spec.Registry.Version != "" && mcpServer.Spec.Runtime.Image != "" {
		logger.Info("MCPServer already enriched with registry data")
		return nil
	}

	// Fetch server specification
	spec, err := r.FetchServerSpec(ctx, registryName, mcpServer.Spec.Registry.Name)
	if err != nil {
		return fmt.Errorf("failed to fetch server spec for enrichment: %w", err)
	}

	// Enrich registry information
	if mcpServer.Spec.Registry.Version == "" {
		mcpServer.Spec.Registry.Version = spec.Version
	}
	if mcpServer.Spec.Registry.Description == "" {
		mcpServer.Spec.Registry.Description = spec.Description
	}
	if mcpServer.Spec.Registry.Repository == "" {
		mcpServer.Spec.Registry.Repository = spec.Repository
	}
	if mcpServer.Spec.Registry.License == "" {
		mcpServer.Spec.Registry.License = spec.License
	}
	if mcpServer.Spec.Registry.Author == "" {
		mcpServer.Spec.Registry.Author = spec.Author
	}
	if len(mcpServer.Spec.Registry.Keywords) == 0 {
		mcpServer.Spec.Registry.Keywords = spec.Keywords
	}
	if len(mcpServer.Spec.Registry.Capabilities) == 0 {
		mcpServer.Spec.Registry.Capabilities = spec.Capabilities
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

	// Fetch server specification (always fetch, no skip check)
	spec, err := r.FetchServerSpec(ctx, registryName, mcpServer.Spec.Registry.Name)
	if err != nil {
		return fmt.Errorf("failed to fetch server spec for force enrichment: %w", err)
	}

	// Always enrich registry information (overwrite existing values)
	mcpServer.Spec.Registry.Version = spec.Version
	mcpServer.Spec.Registry.Description = spec.Description
	mcpServer.Spec.Registry.Repository = spec.Repository
	mcpServer.Spec.Registry.License = spec.License
	mcpServer.Spec.Registry.Author = spec.Author
	mcpServer.Spec.Registry.Keywords = spec.Keywords
	mcpServer.Spec.Registry.Capabilities = spec.Capabilities

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

	if r.registryClient == nil {
		return fmt.Errorf("registry client is not initialized")
	}

	// List available servers in the registry
	servers, err := r.ListAvailableServers(ctx, registry.Name)
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

// ListAvailableServers lists all available servers in a registry
func (r *DefaultRegistryService) ListAvailableServers(ctx context.Context, registryName string) ([]registry.MCPServerInfo, error) {
	logger := log.FromContext(ctx).WithValues("registry", registryName)
	logger.Info("Listing available servers in registry")

	if r.registryClient == nil {
		return nil, fmt.Errorf("registry client is not initialized")
	}

	servers, err := r.registryClient.ListServers(ctx)
	if err != nil {
		logger.Error(err, "Failed to list servers from registry")
		return nil, fmt.Errorf("failed to list servers: %w", err)
	}

	logger.Info("Successfully listed servers from registry", "count", len(servers))
	return servers, nil
}

// ValidateRegistryConnection validates connection to registry
func (r *DefaultRegistryService) ValidateRegistryConnection(ctx context.Context, registry *mcpv1.MCPRegistry) error {
	logger := log.FromContext(ctx).WithValues("registry", registry.Name)
	logger.Info("Validating registry connection")

	if r.registryClient == nil {
		return fmt.Errorf("registry client is not initialized")
	}

	// Test connection by attempting to list servers
	_, err := r.registryClient.ListServers(ctx)
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
func (r *DefaultRegistryService) EnrichMCPServerFromCache(ctx context.Context, mcpServer *mcpv1.MCPServer) error {
	logger := log.FromContext(ctx).WithValues(
		"mcpserver", mcpServer.Name,
		"registryName", mcpServer.Spec.Registry.RegistryName,
		"serverName", mcpServer.Spec.Registry.ServerName,
	)
	logger.Info("Enriching MCPServer from cache")

	// Validate required fields
	if mcpServer.Spec.Registry.RegistryName == "" {
		return fmt.Errorf("registry name is required for cache enrichment")
	}
	if mcpServer.Spec.Registry.ServerName == "" {
		return fmt.Errorf("server name is required for cache enrichment")
	}

	// Skip if already enriched
	if mcpServer.Spec.Registry.Version != "" && mcpServer.Spec.Runtime.Image != "" {
		logger.Info("MCPServer already enriched with registry data")
		return nil
	}

	// Find ConfigMap mcpregistry-<registryName>-<serverName>
	configMapName := fmt.Sprintf("mcpregistry-%s-%s", mcpServer.Spec.Registry.RegistryName, mcpServer.Spec.Registry.ServerName)
	configMap := &corev1.ConfigMap{}
	if err := r.client.Get(ctx, client.ObjectKey{
		Name:      configMapName,
		Namespace: mcpServer.Namespace,
	}, configMap); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("ConfigMap not found for server", "configMap", configMapName)
			return fmt.Errorf("registry cache not found: ConfigMap %s not found in namespace %s", configMapName, mcpServer.Namespace)
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

	// Enrich registry information (without overwriting user-specified values)
	if mcpServer.Spec.Registry.Version == "" {
		mcpServer.Spec.Registry.Version = spec.Version
	}
	if mcpServer.Spec.Registry.Description == "" {
		mcpServer.Spec.Registry.Description = spec.Description
	}
	if mcpServer.Spec.Registry.Repository == "" {
		mcpServer.Spec.Registry.Repository = spec.Repository
	}
	if mcpServer.Spec.Registry.License == "" {
		mcpServer.Spec.Registry.License = spec.License
	}
	if mcpServer.Spec.Registry.Author == "" {
		mcpServer.Spec.Registry.Author = spec.Author
	}
	if len(mcpServer.Spec.Registry.Keywords) == 0 {
		mcpServer.Spec.Registry.Keywords = spec.Keywords
	}
	if len(mcpServer.Spec.Registry.Capabilities) == 0 {
		mcpServer.Spec.Registry.Capabilities = spec.Capabilities
	}

	// Enrich runtime information (without overwriting user-specified values)
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

	// Enrich environment variables (without overwriting user-specified values)
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

	logger.Info("Successfully enriched MCPServer from cache",
		"version", spec.Version,
		"image", spec.Runtime.Image,
		"template_digest", spec.TemplateDigest,
		"configMap", configMapName)

	return nil
}
