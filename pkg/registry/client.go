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

// parseYAMLContent парсит YAML содержимое (простая реализация)
func (c *Client) parseYAMLContent(content []byte) (*MCPServerSpec, error) {
	// Простой парсинг основных полей из YAML
	spec := &MCPServerSpec{}

	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "name:") {
			spec.Name = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
		} else if strings.HasPrefix(line, "version:") {
			spec.Version = strings.TrimSpace(strings.TrimPrefix(line, "version:"))
		} else if strings.HasPrefix(line, "description:") {
			spec.Description = strings.TrimSpace(strings.TrimPrefix(line, "description:"))
		}
		// Добавить парсинг других полей по необходимости
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
