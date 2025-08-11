package registry

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/FantasyNitroGEN/mcp_operator/pkg/retry"
	"gopkg.in/yaml.v3"
)

const (
	// DefaultRegistryURL - URL Docker MCP Registry на GitHub
	DefaultRegistryURL = "https://api.github.com/repos/docker/mcp-registry/contents/servers"

	// DefaultTimeout - таймаут по умолчанию для HTTP запросов
	DefaultTimeout = 30 * time.Second
)

// Client представляет клиент для работы с Docker MCP Registry
type Client struct {
	baseURL       string
	httpClient    *http.Client
	userAgent     string
	githubToken   string
	githubRetrier *retry.GitHubRetrier
}

// NewClient создает новый клиент для работы с реестром
func NewClient() *Client {
	// Configure GitHub retry with default settings
	retryConfig := retry.DefaultGitHubRetryConfig()
	retryConfig.OnRetry = func(attempt int, err error, errorType retry.GitHubErrorType, delay time.Duration) {
		// Enhanced logging for retry attempts
		fmt.Printf("GitHub API retry attempt %d for error type %s, delay: %v, error: %v\n",
			attempt, errorType.String(), delay, err)
	}

	return &Client{
		baseURL: DefaultRegistryURL,
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
		userAgent:     "mcp-operator/1.0",
		githubToken:   os.Getenv("GITHUB_TOKEN"),
		githubRetrier: retry.NewGitHubRetrier(retryConfig),
	}
}

// NewClientWithURL создает клиент с кастомным URL
func NewClientWithURL(baseURL string) *Client {
	client := NewClient()
	client.baseURL = baseURL
	return client
}

// NewClientWithToken создает клиент с GitHub токеном
func NewClientWithToken(token string) *Client {
	client := NewClient()
	client.githubToken = token
	return client
}

// NewClientWithRetryConfig создает клиент с кастомной конфигурацией повторных попыток
func NewClientWithRetryConfig(retryConfig retry.GitHubRetryConfig) *Client {
	return &Client{
		baseURL: DefaultRegistryURL,
		httpClient: &http.Client{
			Timeout: DefaultTimeout,
		},
		userAgent:     "mcp-operator/1.0",
		githubToken:   os.Getenv("GITHUB_TOKEN"),
		githubRetrier: retry.NewGitHubRetrier(retryConfig),
	}
}

// ListServers получает список всех MCP серверов из реестра
func (c *Client) ListServers(ctx context.Context) ([]MCPServerInfo, error) {
	var servers []MCPServerInfo

	err := c.githubRetrier.DoWithGitHubRetry(ctx, "list_servers", func(ctx context.Context) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("User-Agent", c.userAgent)
		req.Header.Set("Accept", "application/vnd.github.v3+json")

		if c.githubToken != "" {
			req.Header.Set("Authorization", "token "+c.githubToken)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return resp, fmt.Errorf("failed to execute request: %w", err)
		}

		if resp.StatusCode != http.StatusOK {
			return resp, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
		}

		var registryResponse []RegistryResponse
		if err := json.NewDecoder(resp.Body).Decode(&registryResponse); err != nil {
			if closeErr := resp.Body.Close(); closeErr != nil {
				// Log the close error but don't override the original decode error
				fmt.Fprintf(os.Stderr, "Failed to close response body: %v\n", closeErr)
			}
			return resp, fmt.Errorf("failed to decode response: %w", err)
		}

		for _, item := range registryResponse {
			if item.Type == "dir" {
				servers = append(servers, MCPServerInfo{
					Name:        item.Name,
					Path:        item.Path,
					Size:        item.Size,
					DownloadURL: item.DownloadURL,
					HTMLURL:     item.HTMLURL,
				})
			}
		}

		if err := resp.Body.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to close response body: %v\n", err)
		}
		return resp, nil
	})

	if err != nil {
		return nil, err
	}

	return servers, nil
}

// GetServerSpec получает спецификацию конкретного MCP сервера
func (c *Client) GetServerSpec(ctx context.Context, serverName string) (*MCPServerSpec, error) {
	var spec *MCPServerSpec

	err := c.githubRetrier.DoWithGitHubRetry(ctx, "get_server_spec", func(ctx context.Context) (*http.Response, error) {
		// URL для получения server.yaml файла сервера
		specURL := fmt.Sprintf("%s/%s/server.yaml", c.baseURL, serverName)

		req, err := http.NewRequestWithContext(ctx, "GET", specURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("User-Agent", c.userAgent)
		req.Header.Set("Accept", "application/vnd.github.v3+json")

		// Добавляем GitHub токен если доступен
		if c.githubToken != "" {
			req.Header.Set("Authorization", "token "+c.githubToken)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return resp, fmt.Errorf("failed to execute request: %w", err)
		}

		if resp.StatusCode == http.StatusNotFound {
			return resp, fmt.Errorf("server %s not found in registry", serverName)
		}

		if resp.StatusCode != http.StatusOK {
			return resp, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
		}

		var fileResponse RegistryResponse
		if err := json.NewDecoder(resp.Body).Decode(&fileResponse); err != nil {
			if closeErr := resp.Body.Close(); closeErr != nil {
				fmt.Fprintf(os.Stderr, "Failed to close response body: %v\n", closeErr)
			}
			return resp, fmt.Errorf("failed to decode response: %w", err)
		}

		// Декодируем содержимое файла из base64
		content, err := base64.StdEncoding.DecodeString(fileResponse.Content)
		if err != nil {
			if closeErr := resp.Body.Close(); closeErr != nil {
				fmt.Fprintf(os.Stderr, "Failed to close response body: %v\n", closeErr)
			}
			return resp, fmt.Errorf("failed to decode file content: %w", err)
		}

		// Вычисляем SHA256 digest от raw содержимого server.yaml
		hash := sha256.Sum256(content)
		templateDigest := hex.EncodeToString(hash[:])

		// Парсим YAML содержимое
		var parsedSpec MCPServerSpec
		if err := json.Unmarshal(content, &parsedSpec); err != nil {
			// Если это YAML, попробуем другой подход
			if strings.Contains(string(content), "name:") {
				spec, err = c.parseYAMLContent(content)
				if err != nil {
					if closeErr := resp.Body.Close(); closeErr != nil {
						fmt.Fprintf(os.Stderr, "Failed to close response body: %v\n", closeErr)
					}
					return resp, err
				}
			} else {
				if closeErr := resp.Body.Close(); closeErr != nil {
					fmt.Fprintf(os.Stderr, "Failed to close response body: %v\n", closeErr)
				}
				return resp, fmt.Errorf("failed to parse server spec: %w", err)
			}
		} else {
			spec = &parsedSpec
		}

		// Устанавливаем digest в спецификацию
		if spec != nil {
			spec.TemplateDigest = templateDigest
		}

		if err := resp.Body.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to close response body: %v\n", err)
		}
		return resp, nil
	})

	if err != nil {
		return nil, err
	}

	return spec, nil
}

// parseYAMLContent парсит YAML содержимое используя proper YAML unmarshaling
func (c *Client) parseYAMLContent(content []byte) (*MCPServerSpec, error) {
	spec := &MCPServerSpec{}

	if err := yaml.Unmarshal(content, spec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal YAML content: %w", err)
	}

	return spec, nil
}

// extractRateLimitInfo извлекает информацию о rate limit из GitHub API response headers
func (c *Client) extractRateLimitInfo(resp *http.Response) *RateLimitInfo {
	if resp == nil {
		return nil
	}

	limitHeader := resp.Header.Get("X-RateLimit-Limit")
	remainingHeader := resp.Header.Get("X-RateLimit-Remaining")
	resetHeader := resp.Header.Get("X-RateLimit-Reset")

	if limitHeader == "" || remainingHeader == "" || resetHeader == "" {
		return nil
	}

	limit, err := strconv.ParseInt(limitHeader, 10, 32)
	if err != nil {
		return nil
	}

	remaining, err := strconv.ParseInt(remainingHeader, 10, 32)
	if err != nil {
		return nil
	}

	reset, err := strconv.ParseInt(resetHeader, 10, 64)
	if err != nil {
		return nil
	}

	return &RateLimitInfo{
		Limit:     int32(limit),
		Remaining: int32(remaining),
		Reset:     time.Unix(reset, 0),
	}
}

// listServersWithRateLimit получает список серверов и извлекает rate limit информацию
func (c *Client) listServersWithRateLimit(ctx context.Context, rateLimitInfo **RateLimitInfo) ([]MCPServerInfo, error) {
	var servers []MCPServerInfo

	err := c.githubRetrier.DoWithGitHubRetry(ctx, "list_servers", func(ctx context.Context) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("User-Agent", c.userAgent)
		req.Header.Set("Accept", "application/vnd.github.v3+json")

		if c.githubToken != "" {
			req.Header.Set("Authorization", "token "+c.githubToken)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return resp, fmt.Errorf("failed to execute request: %w", err)
		}

		// Extract rate limit information
		if *rateLimitInfo == nil {
			*rateLimitInfo = c.extractRateLimitInfo(resp)
		}

		if resp.StatusCode != http.StatusOK {
			return resp, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
		}

		var registryResponse []RegistryResponse
		if err := json.NewDecoder(resp.Body).Decode(&registryResponse); err != nil {
			if closeErr := resp.Body.Close(); closeErr != nil {
				fmt.Fprintf(os.Stderr, "Failed to close response body: %v\n", closeErr)
			}
			return resp, fmt.Errorf("failed to decode response: %w", err)
		}

		for _, item := range registryResponse {
			if item.Type == "dir" {
				servers = append(servers, MCPServerInfo{
					Name:        item.Name,
					Path:        item.Path,
					Size:        item.Size,
					DownloadURL: item.DownloadURL,
					HTMLURL:     item.HTMLURL,
				})
			}
		}

		if err := resp.Body.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to close response body: %v\n", err)
		}
		return resp, nil
	})

	if err != nil {
		return nil, err
	}

	return servers, nil
}

// getServerSpecWithRateLimit получает спецификацию сервера и обновляет rate limit информацию
func (c *Client) getServerSpecWithRateLimit(ctx context.Context, serverName string, rateLimitInfo **RateLimitInfo) (*MCPServerSpec, error) {
	var spec *MCPServerSpec

	err := c.githubRetrier.DoWithGitHubRetry(ctx, "get_server_spec", func(ctx context.Context) (*http.Response, error) {
		specURL := fmt.Sprintf("%s/%s/server.yaml", c.baseURL, serverName)

		req, err := http.NewRequestWithContext(ctx, "GET", specURL, nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create request: %w", err)
		}

		req.Header.Set("User-Agent", c.userAgent)
		req.Header.Set("Accept", "application/vnd.github.v3+json")

		if c.githubToken != "" {
			req.Header.Set("Authorization", "token "+c.githubToken)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return resp, fmt.Errorf("failed to execute request: %w", err)
		}

		// Update rate limit information with most recent data
		*rateLimitInfo = c.extractRateLimitInfo(resp)

		if resp.StatusCode == http.StatusNotFound {
			return resp, fmt.Errorf("server %s not found in registry", serverName)
		}

		if resp.StatusCode != http.StatusOK {
			return resp, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
		}

		var fileResponse RegistryResponse
		if err := json.NewDecoder(resp.Body).Decode(&fileResponse); err != nil {
			if closeErr := resp.Body.Close(); closeErr != nil {
				fmt.Fprintf(os.Stderr, "Failed to close response body: %v\n", closeErr)
			}
			return resp, fmt.Errorf("failed to decode response: %w", err)
		}

		content, err := base64.StdEncoding.DecodeString(fileResponse.Content)
		if err != nil {
			if closeErr := resp.Body.Close(); closeErr != nil {
				fmt.Fprintf(os.Stderr, "Failed to close response body: %v\n", closeErr)
			}
			return resp, fmt.Errorf("failed to decode file content: %w", err)
		}

		hash := sha256.Sum256(content)
		templateDigest := hex.EncodeToString(hash[:])

		var parsedSpec MCPServerSpec
		if err := json.Unmarshal(content, &parsedSpec); err != nil {
			if strings.Contains(string(content), "name:") {
				spec, err = c.parseYAMLContent(content)
				if err != nil {
					if closeErr := resp.Body.Close(); closeErr != nil {
						fmt.Fprintf(os.Stderr, "Failed to close response body: %v\n", closeErr)
					}
					return resp, err
				}
			} else {
				if closeErr := resp.Body.Close(); closeErr != nil {
					fmt.Fprintf(os.Stderr, "Failed to close response body: %v\n", closeErr)
				}
				return resp, fmt.Errorf("failed to parse server spec: %w", err)
			}
		} else {
			spec = &parsedSpec
		}

		if spec != nil {
			spec.TemplateDigest = templateDigest
		}

		if err := resp.Body.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to close response body: %v\n", err)
		}
		return resp, nil
	})

	if err != nil {
		return nil, err
	}

	return spec, nil
}

// SearchServers ищет серверы по ключевым словам
func (c *Client) SearchServers(ctx context.Context, query string) ([]MCPServerInfo, error) {
	servers, err := c.ListServers(ctx)
	if err != nil {
		return nil, err
	}

	var filtered []MCPServerInfo
	query = strings.ToLower(query)

	for _, server := range servers {
		if strings.Contains(strings.ToLower(server.Name), query) {
			filtered = append(filtered, server)
		}
	}

	return filtered, nil
}

// ListServersLocal получает список всех MCP серверов из локального реестра
func (c *Client) ListServersLocal(ctx context.Context, registryPath string) ([]MCPServerInfo, error) {
	serversDir := filepath.Join(registryPath, "servers")

	if _, err := os.Stat(serversDir); err != nil {
		return nil, fmt.Errorf("local registry not found at %s: %w", serversDir, err)
	}

	entries, err := os.ReadDir(serversDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read servers directory: %w", err)
	}

	var servers []MCPServerInfo
	for _, entry := range entries {
		if entry.IsDir() {
			serverPath := filepath.Join(serversDir, entry.Name())
			serverFile := filepath.Join(serverPath, "server.yaml")

			// Проверяем наличие server.yaml
			if _, err := os.Stat(serverFile); err == nil {
				info, err := entry.Info()
				if err != nil {
					continue
				}

				servers = append(servers, MCPServerInfo{
					Name:      entry.Name(),
					Path:      serverPath,
					UpdatedAt: info.ModTime(),
					Size:      info.Size(),
					HTMLURL:   "", // Локальный файл, нет URL
				})
			}
		}
	}

	return servers, nil
}

// GetServerSpecLocal получает спецификацию конкретного MCP сервера из локального реестра
func (c *Client) GetServerSpecLocal(ctx context.Context, registryPath, serverName string) (*ServerYAML, error) {
	serverFile := filepath.Join(registryPath, "servers", serverName, "server.yaml")

	if _, err := os.Stat(serverFile); err != nil {
		return nil, fmt.Errorf("server %s not found in local registry: %w", serverName, err)
	}

	content, err := os.ReadFile(serverFile)
	if err != nil {
		return nil, fmt.Errorf("failed to read server file: %w", err)
	}

	var serverSpec ServerYAML
	if err := yaml.Unmarshal(content, &serverSpec); err != nil {
		return nil, fmt.Errorf("failed to parse server spec: %w", err)
	}

	return &serverSpec, nil
}

// SearchServersLocal ищет серверы по ключевым словам в локальном реестре
func (c *Client) SearchServersLocal(ctx context.Context, registryPath, query string) ([]MCPServerInfo, error) {
	servers, err := c.ListServersLocal(ctx, registryPath)
	if err != nil {
		return nil, err
	}

	var filtered []MCPServerInfo
	query = strings.ToLower(query)

	for _, server := range servers {
		if strings.Contains(strings.ToLower(server.Name), query) {
			filtered = append(filtered, server)
		}
	}

	return filtered, nil
}

// HasLocalRegistry проверяет наличие локального реестра
func (c *Client) HasLocalRegistry(registryPath string) bool {
	serversDir := filepath.Join(registryPath, "servers")
	_, err := os.Stat(serversDir)
	return err == nil
}

// SyncRepository синхронизирует серверы из указанного GitHub репозитория
func (c *Client) SyncRepository(ctx context.Context, repo *GitHubRepository) (*SyncResult, error) {
	// Создаем новый клиент с настройками для конкретного репозитория
	repoClient := &Client{
		httpClient:    c.httpClient,
		userAgent:     c.userAgent,
		githubRetrier: c.githubRetrier,
		githubToken:   repo.AuthToken,
	}

	// Формируем URL для GitHub API
	path := "servers"
	if repo.Path != "" {
		path = strings.TrimPrefix(repo.Path, "/")
		if !strings.HasSuffix(path, "/") {
			path += "/"
		}
		path += "servers"
	}

	branch := repo.Branch
	if branch == "" {
		branch = "main"
	}

	repoClient.baseURL = fmt.Sprintf("https://api.github.com/repos/%s/%s/contents/%s?ref=%s",
		repo.Owner, repo.Name, path, branch)

	// Получаем список серверов и rate limit info
	var rateLimitInfo *RateLimitInfo
	servers, err := repoClient.listServersWithRateLimit(ctx, &rateLimitInfo)
	if err != nil {
		return &SyncResult{
			Errors:        []string{fmt.Sprintf("failed to list servers: %v", err)},
			RateLimitInfo: rateLimitInfo,
		}, err
	}

	// Получаем спецификации для каждого сервера
	serverSpecs := make(map[string]*MCPServerSpec)
	var syncErrors []string
	observedSHA := ""

	for _, server := range servers {
		spec, err := repoClient.getServerSpecWithRateLimit(ctx, server.Name, &rateLimitInfo)
		if err != nil {
			syncErrors = append(syncErrors, fmt.Sprintf("failed to get spec for %s: %v", server.Name, err))
			continue
		}
		serverSpecs[server.Name] = spec
		// Используем первый успешно полученный SHA как observedSHA
		if observedSHA == "" && spec.TemplateDigest != "" {
			observedSHA = spec.TemplateDigest
		}
	}

	result := &SyncResult{
		Servers:       servers,
		ServerSpecs:   serverSpecs,
		ObservedSHA:   observedSHA,
		ServersCount:  int32(len(servers)),
		Errors:        syncErrors,
		RateLimitInfo: rateLimitInfo,
	}

	return result, nil
}
