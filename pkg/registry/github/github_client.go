package github

import (
	"context"
	"fmt"
	"strings"

	mcpv1 "github.com/FantasyNitroGEN/mcp_operator/api/v1"
	"github.com/FantasyNitroGEN/mcp_operator/pkg/registry"
)

// GitHubRegistryClient wraps the generic registry client for GitHub-specific operations
type GitHubRegistryClient struct {
	client *registry.Client
}

// NewGitHubRegistryClient creates a new GitHub registry client
func NewGitHubRegistryClient(client *registry.Client) *GitHubRegistryClient {
	return &GitHubRegistryClient{
		client: client,
	}
}

// SyncRegistry synchronizes servers from a GitHub registry
func (g *GitHubRegistryClient) SyncRegistry(ctx context.Context, mcpRegistry *mcpv1.MCPRegistry) (*registry.SyncResult, error) {
	repo, err := g.parseRepositoryURL(mcpRegistry.Spec.Source.Github.Repo)
	if err != nil {
		return nil, fmt.Errorf("failed to parse repository URL: %w", err)
	}

	// Set branch if specified
	if mcpRegistry.Spec.Source.Github.Branch != "" {
		repo.Branch = mcpRegistry.Spec.Source.Github.Branch
	}

	// Set path if specified
	if mcpRegistry.Spec.Source.Path != "" {
		repo.Path = mcpRegistry.Spec.Source.Path
	}

	// Set authentication token if available
	if mcpRegistry.Spec.Auth != nil && mcpRegistry.Spec.Auth.SecretRef != nil {
		// Token should be injected by the controller
		if token := g.getTokenFromContext(ctx); token != "" {
			repo.AuthToken = token
		}
	}

	return g.client.SyncRepository(ctx, repo)
}

// parseRepositoryURL parses GitHub repository URL and extracts components
func (g *GitHubRegistryClient) parseRepositoryURL(repoURL string) (*registry.GitHubRepository, error) {
	// Handle different GitHub URL formats
	repoURL = strings.TrimPrefix(repoURL, "https://github.com/")
	repoURL = strings.TrimPrefix(repoURL, "git@github.com:")
	repoURL = strings.TrimSuffix(repoURL, ".git")

	parts := strings.Split(repoURL, "/")
	if len(parts) < 2 {
		return nil, fmt.Errorf("invalid repository URL format")
	}

	return &registry.GitHubRepository{
		Owner:  parts[0],
		Name:   parts[1],
		Branch: "main", // Default branch
	}, nil
}

// getTokenFromContext retrieves the auth token from context
func (g *GitHubRegistryClient) getTokenFromContext(ctx context.Context) string {
	if token, ok := ctx.Value("auth_token").(string); ok {
		return token
	}
	return ""
}
