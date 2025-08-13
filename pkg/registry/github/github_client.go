package github

import (
	"context"
	"fmt"
	"net/url"
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
	var repo *registry.GitHubRepository
	var err error

	// Handle spec.url configuration
	if mcpRegistry.Spec.URL != "" {
		repo, err = g.parseGitHubAPIURL(mcpRegistry.Spec.URL)
		if err != nil {
			return nil, fmt.Errorf("failed to parse GitHub API URL: %w", err)
		}
	} else if mcpRegistry.Spec.Source != nil && mcpRegistry.Spec.Source.Github != nil {
		// Handle spec.source configuration
		repo, err = g.parseRepositoryURL(mcpRegistry.Spec.Source.Github.Repo)
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
	} else {
		return nil, fmt.Errorf("neither spec.url nor spec.source.github configuration provided")
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

// parseGitHubAPIURL parses GitHub API URL and extracts repository components
// Example: https://api.github.com/repos/docker/mcp-registry/contents/servers?ref=main
func (g *GitHubRegistryClient) parseGitHubAPIURL(apiURL string) (*registry.GitHubRepository, error) {
	parsedURL, err := url.Parse(apiURL)
	if err != nil {
		return nil, fmt.Errorf("invalid URL format: %w", err)
	}

	// Check if it's a GitHub API URL
	if parsedURL.Host != "api.github.com" {
		return nil, fmt.Errorf("only GitHub API URLs are supported")
	}

	// Parse path: /repos/{owner}/{repo}/contents/{path}
	pathParts := strings.Split(strings.Trim(parsedURL.Path, "/"), "/")
	if len(pathParts) < 4 || pathParts[0] != "repos" || pathParts[3] != "contents" {
		return nil, fmt.Errorf("invalid GitHub API URL format, expected /repos/{owner}/{repo}/contents/{path}")
	}

	owner := pathParts[1]
	name := pathParts[2]

	// Extract path from URL (everything after /contents/)
	var repoPath string
	if len(pathParts) > 4 {
		repoPath = strings.Join(pathParts[4:], "/")
	}

	// Extract branch from query parameters (ref parameter)
	branch := parsedURL.Query().Get("ref")
	if branch == "" {
		branch = "main" // Default branch
	}

	return &registry.GitHubRepository{
		Owner:  owner,
		Name:   name,
		Path:   repoPath,
		Branch: branch,
	}, nil
}

// getTokenFromContext retrieves the auth token from context
func (g *GitHubRegistryClient) getTokenFromContext(ctx context.Context) string {
	if token, ok := ctx.Value(registry.AuthTokenContextKey).(string); ok {
		return token
	}
	return ""
}
