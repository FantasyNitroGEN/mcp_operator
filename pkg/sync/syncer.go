package sync

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/FantasyNitroGEN/mcp_operator/pkg/registry"
)

// Syncer отвечает за синхронизацию MCP серверов из реестра
type Syncer struct {
	client     *registry.Client
	baseDir    string
	yamalsDir  string
	sourcesDir string
}

// NewSyncer создает новый синхронизатор
func NewSyncer(client *registry.Client, baseDir string) *Syncer {
	return &Syncer{
		client:     client,
		baseDir:    baseDir,
		yamalsDir:  filepath.Join(baseDir, "yamls"),
		sourcesDir: filepath.Join(baseDir, "sources"),
	}
}

// SyncAllServers синхронизирует все серверы из реестра
func (s *Syncer) SyncAllServers(ctx context.Context) error {
	servers, err := s.client.ListServers(ctx)
	if err != nil {
		return fmt.Errorf("failed to list servers: %w", err)
	}

	fmt.Printf("Найдено %d серверов для синхронизации\n", len(servers))

	successCount := 0
	errorCount := 0
	rateLimitCount := 0

	for i, server := range servers {
		fmt.Printf("Синхронизация %d/%d: %s\n", i+1, len(servers), server.Name)

		if err := s.downloadServerSpecWithRetry(ctx, server.Name); err != nil {
			if strings.Contains(err.Error(), "429") {
				rateLimitCount++
				fmt.Printf("⚠️ Rate limit для %s: %v\n", server.Name, err)
				// Делаем паузу при rate limiting
				time.Sleep(2 * time.Second)
			} else {
				errorCount++
				fmt.Printf("⚠️ Ошибка загрузки спецификации %s: %v\n", server.Name, err)
			}
			continue
		}

		successCount++
		fmt.Printf("✅ %s - спецификация загружена\n", server.Name)

		// Небольшая пауза между запросами для предотвращения rate limiting
		time.Sleep(100 * time.Millisecond)
	}

	fmt.Printf("\n📊 Результаты синхронизации:\n")
	fmt.Printf("   ✅ Успешно: %d\n", successCount)
	fmt.Printf("   ❌ Ошибки: %d\n", errorCount)
	fmt.Printf("   ⏳ Rate limit: %d\n", rateLimitCount)

	return nil
}

// downloadServerSpecWithRetry загружает спецификацию с повторными попытками
func (s *Syncer) downloadServerSpecWithRetry(ctx context.Context, serverName string) error {
	maxRetries := 3
	baseDelay := time.Second

	for attempt := 0; attempt < maxRetries; attempt++ {
		err := s.DownloadServerSpec(ctx, serverName)
		if err == nil {
			return nil
		}

		// При rate limiting увеличиваем задержку
		if strings.Contains(err.Error(), "429") {
			delay := baseDelay * time.Duration(1<<uint(attempt)) // exponential backoff
			if attempt < maxRetries-1 {
				fmt.Printf("   Rate limit, повтор через %v...\n", delay)
				time.Sleep(delay)
				continue
			}
		}

		// При других ошибках прекращаем попытки
		if attempt == maxRetries-1 {
			return err
		}

		time.Sleep(baseDelay)
	}

	return fmt.Errorf("exceeded max retries for %s", serverName)
}

// DownloadServerSpec загружает YAML спецификацию сервера
func (s *Syncer) DownloadServerSpec(ctx context.Context, serverName string) error {
	if err := s.ensureDir(s.yamalsDir); err != nil {
		return fmt.Errorf("failed to create yamls directory: %w", err)
	}

	spec, err := s.client.GetServerSpec(ctx, serverName)
	if err != nil {
		return fmt.Errorf("failed to get server spec: %w", err)
	}

	// Создаем папку для сервера в servers/yamls/
	serverDir := filepath.Join(s.yamalsDir, serverName)
	if err := os.MkdirAll(serverDir, 0755); err != nil {
		return fmt.Errorf("failed to create server directory: %w", err)
	}

	// Сохраняем спецификацию в YAML файл
	specPath := filepath.Join(serverDir, "server.yaml")

	yamlData, err := yaml.Marshal(spec)
	if err != nil {
		return fmt.Errorf("failed to marshal spec to YAML: %w", err)
	}

	if err := os.WriteFile(specPath, yamlData, 0644); err != nil {
		return fmt.Errorf("failed to write spec file: %w", err)
	}

	return nil
}

// DownloadServerSources загружает исходный код сервера
func (s *Syncer) DownloadServerSources(ctx context.Context, serverName string) error {
	if err := s.ensureDir(s.sourcesDir); err != nil {
		return fmt.Errorf("failed to create sources directory: %w", err)
	}

	// Пробуем разные стратегии загрузки

	// 1. Сначала пытаемся получить спецификацию и найти репозиторий
	if spec, err := s.client.GetServerSpec(ctx, serverName); err == nil && spec.Repository != "" {
		fmt.Printf("   Клонируем из репозитория: %s\n", spec.Repository)
		if err := s.cloneRepository(spec.Repository, serverName); err == nil {
			return nil
		}
		fmt.Printf("   Ошибка клонирования репозитория: %v\n", err)
	}

	// 2. Fallback: загружаем из основного реестра Docker
	fmt.Printf("   Загружаем из Docker MCP Registry...\n")
	if err := s.downloadFromDockerRegistry(ctx, serverName); err == nil {
		return nil
	}

	// 3. Последняя попытка: создаем базовую структуру на основе известных паттернов
	fmt.Printf("   Создаем базовую структуру сервера...\n")
	return s.createBasicServerStructure(serverName)
}

// downloadFromDockerRegistry загружает исходники из Docker MCP Registry
func (s *Syncer) downloadFromDockerRegistry(ctx context.Context, serverName string) error {
	sourcePath := filepath.Join(s.sourcesDir, serverName)

	// Удаляем существующую директорию если есть
	if err := os.RemoveAll(sourcePath); err != nil {
		return fmt.Errorf("failed to remove existing source directory: %w", err)
	}

	// Клонируем весь репозиторий Docker MCP Registry
	repoURL := "https://github.com/docker/mcp-registry.git"
	tempDir := filepath.Join(os.TempDir(), "mcp-registry-temp")

	// Удаляем временную директорию если есть
	os.RemoveAll(tempDir)

	cmd := exec.CommandContext(ctx, "git", "clone", "--depth", "1", repoURL, tempDir)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to clone registry: %w", err)
	}
	defer os.RemoveAll(tempDir)

	// Копируем папку сервера
	serverDir := filepath.Join(tempDir, "servers", serverName)
	if _, err := os.Stat(serverDir); os.IsNotExist(err) {
		return fmt.Errorf("server %s not found in registry", serverName)
	}

	cmd = exec.Command("cp", "-r", serverDir, sourcePath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to copy server sources: %w", err)
	}

	return nil
}

// cloneRepository клонирует репозиторий сервера
func (s *Syncer) cloneRepository(repoURL, serverName string) error {
	sourcePath := filepath.Join(s.sourcesDir, serverName)

	// Удаляем существующую директорию если есть
	if err := os.RemoveAll(sourcePath); err != nil {
		return fmt.Errorf("failed to remove existing source directory: %w", err)
	}

	cmd := exec.Command("git", "clone", repoURL, sourcePath)
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to clone repository: %w", err)
	}

	return nil
}

// ensureDir создает директорию если её нет
func (s *Syncer) ensureDir(dir string) error {
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		return os.MkdirAll(dir, 0755)
	}
	return nil
}

// GetLocalServers возвращает список локально сохраненных серверов
func (s *Syncer) GetLocalServers() ([]string, error) {
	var servers []string

	if _, err := os.Stat(s.yamalsDir); os.IsNotExist(err) {
		return servers, nil
	}

	entries, err := os.ReadDir(s.yamalsDir)
	if err != nil {
		return nil, fmt.Errorf("failed to read yamls directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".yaml") {
			serverName := strings.TrimSuffix(entry.Name(), ".yaml")
			servers = append(servers, serverName)
		}
	}

	return servers, nil
}

// HasLocalSources проверяет, есть ли локальные исходники для сервера
func (s *Syncer) HasLocalSources(serverName string) bool {
	sourcePath := filepath.Join(s.sourcesDir, serverName)
	_, err := os.Stat(sourcePath)
	return err == nil
}

// LoadServerSpec загружает спецификацию сервера из локального файла
func (s *Syncer) LoadServerSpec(serverName string) (*registry.MCPServerSpec, error) {
	specPath := filepath.Join(s.yamalsDir, serverName, "server.yaml")

	data, err := os.ReadFile(specPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read spec file: %w", err)
	}

	var spec registry.MCPServerSpec
	if err := yaml.Unmarshal(data, &spec); err != nil {
		return nil, fmt.Errorf("failed to unmarshal spec: %w", err)
	}

	return &spec, nil
}

// createBasicServerStructure создает базовую структуру сервера если не удалось загрузить исходники
func (s *Syncer) createBasicServerStructure(serverName string) error {
	sourcePath := filepath.Join(s.sourcesDir, serverName)

	if err := os.MkdirAll(sourcePath, 0755); err != nil {
		return fmt.Errorf("failed to create server directory: %w", err)
	}

	// Создаем базовые файлы на основе популярных паттернов
	spec := s.getDefaultServerSpec(serverName)

	// Создаем спецификацию
	specData, err := yaml.Marshal(spec)
	if err != nil {
		return fmt.Errorf("failed to marshal default spec: %w", err)
	}

	// Создаем папку для сервера в servers/yamls/
	serverDir := filepath.Join(s.yamalsDir, serverName)
	if err := os.MkdirAll(serverDir, 0755); err != nil {
		return fmt.Errorf("failed to create server directory: %w", err)
	}

	specPath := filepath.Join(serverDir, "server.yaml")
	if err := os.WriteFile(specPath, specData, 0644); err != nil {
		return fmt.Errorf("failed to write default spec: %w", err)
	}

	// Создаем базовый Dockerfile
	dockerfilePath := filepath.Join(sourcePath, "Dockerfile")
	dockerfileContent := s.generateDockerfile(serverName, spec.Runtime.Type)

	if err := os.WriteFile(dockerfilePath, []byte(dockerfileContent), 0644); err != nil {
		return fmt.Errorf("failed to write Dockerfile: %w", err)
	}

	// Создаем README
	readmePath := filepath.Join(sourcePath, "README.md")
	readmeContent := fmt.Sprintf("# MCP Server: %s\n\nБазовая структура сервера %s, созданная автоматически.\n\nДля получения исходного кода см. официальный репозиторий: https://github.com/docker/mcp-registry/tree/main/servers/%s\n", serverName, serverName, serverName)

	if err := os.WriteFile(readmePath, []byte(readmeContent), 0644); err != nil {
		return fmt.Errorf("failed to write README: %w", err)
	}

	fmt.Printf("   ✅ Создана базовая структура для %s\n", serverName)
	return nil
}

// getDefaultServerSpec возвращает спецификацию сервера по умолчанию
func (s *Syncer) getDefaultServerSpec(serverName string) *registry.MCPServerSpec {
	// Определяем тип runtime на основе имени сервера
	runtimeType := "python" // по умолчанию
	image := "python:3.11-slim"
	command := []string{"python", "-m", fmt.Sprintf("mcp_server_%s", serverName)}

	if strings.Contains(serverName, "node") || strings.Contains(serverName, "js") {
		runtimeType = "node"
		image = "node:18-alpine"
		command = []string{"npm", "start"}
	} else if strings.Contains(serverName, "go") {
		runtimeType = "go"
		image = "golang:1.21-alpine"
		command = []string{"./main"}
	}

	return &registry.MCPServerSpec{
		Name:        serverName,
		Version:     "1.0.0",
		Description: fmt.Sprintf("MCP server for %s", serverName),
		Runtime: registry.RuntimeSpec{
			Type:    runtimeType,
			Image:   image,
			Command: command,
		},
		Capabilities: []string{
			"tools",
			"resources",
		},
	}
}

// SyncFromLocalRegistry копирует server.yaml файлы из клонированного реестра в servers/yamls/
func (s *Syncer) SyncFromLocalRegistry(registryPath string) error {
	if err := s.ensureDir(s.yamalsDir); err != nil {
		return fmt.Errorf("failed to create yamls directory: %w", err)
	}

	serversDir := filepath.Join(registryPath, "servers")
	if _, err := os.Stat(serversDir); err != nil {
		return fmt.Errorf("local registry servers directory not found at %s: %w", serversDir, err)
	}

	entries, err := os.ReadDir(serversDir)
	if err != nil {
		return fmt.Errorf("failed to read servers directory: %w", err)
	}

	successCount := 0
	errorCount := 0

	fmt.Printf("🔄 Копирование server.yaml файлов в servers/yamls/...\n")

	for _, entry := range entries {
		if entry.IsDir() {
			serverName := entry.Name()
			serverFile := filepath.Join(serversDir, serverName, "server.yaml")

			// Проверяем наличие server.yaml
			if _, err := os.Stat(serverFile); err == nil {
				// Читаем server.yaml
				content, err := os.ReadFile(serverFile)
				if err != nil {
					fmt.Printf("⚠️ Ошибка чтения %s: %v\n", serverName, err)
					errorCount++
					continue
				}

				// Парсим server.yaml для получения структурированных данных
				var serverYAML registry.ServerYAML
				if err := yaml.Unmarshal(content, &serverYAML); err != nil {
					fmt.Printf("⚠️ Ошибка парсинга %s: %v\n", serverName, err)
					errorCount++
					continue
				}

				// Конвертируем в MCPServerSpec формат
				spec := &registry.MCPServerSpec{
					Name:        serverYAML.Name,
					Description: serverYAML.About.Description,
					Repository:  serverYAML.Source.Project,
					Runtime: registry.RuntimeSpec{
						Type:  serverYAML.Type,
						Image: serverYAML.Image,
					},
					Keywords: serverYAML.Meta.Tags,
				}

				// Создаем папку для сервера в servers/yamls/
				serverDir := filepath.Join(s.yamalsDir, serverName)
				if err := os.MkdirAll(serverDir, 0755); err != nil {
					fmt.Printf("⚠️ Ошибка создания папки %s: %v\n", serverName, err)
					errorCount++
					continue
				}

				specPath := filepath.Join(serverDir, "server.yaml")
				yamlData, err := yaml.Marshal(spec)
				if err != nil {
					fmt.Printf("⚠️ Ошибка маршалинга %s: %v\n", serverName, err)
					errorCount++
					continue
				}

				if err := os.WriteFile(specPath, yamlData, 0644); err != nil {
					fmt.Printf("⚠️ Ошибка записи %s: %v\n", serverName, err)
					errorCount++
					continue
				}

				successCount++
				if successCount <= 10 || successCount%50 == 0 {
					fmt.Printf("✅ %s скопирован в servers/yamls/\n", serverName)
				}
			}
		}
	}

	fmt.Printf("\n📊 Результаты копирования:\n")
	fmt.Printf("   ✅ Успешно скопировано: %d файлов\n", successCount)
	if errorCount > 0 {
		fmt.Printf("   ❌ Ошибки: %d файлов\n", errorCount)
	}
	fmt.Printf("   📁 Файлы сохранены в: %s\n", s.yamalsDir)

	return nil
}

// generateDockerfile создает Dockerfile для сервера
func (s *Syncer) generateDockerfile(serverName, runtimeType string) string {
	switch runtimeType {
	case "node":
		return fmt.Sprintf(`FROM node:18-alpine

WORKDIR /app

# Устанавливаем зависимости
COPY package*.json ./
RUN npm ci --only=production

# Копируем исходный код
COPY . .

EXPOSE 8080

CMD ["npm", "start"]
`)

	case "go":
		return fmt.Sprintf(`FROM golang:1.21-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o main .

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/

COPY --from=builder /app/main .

EXPOSE 8080
CMD ["./main"]
`)

	default: // python
		return fmt.Sprintf(`FROM python:3.11-slim

WORKDIR /app

# Устанавливаем системные зависимости
RUN apt-get update && apt-get install -y \\
    git \\
    && rm -rf /var/lib/apt/lists/*

# Устанавливаем Python зависимости
COPY requirements.txt .
RUN pip install --no-cache-dir -r requirements.txt

# Копируем исходный код
COPY . .

EXPOSE 8080

CMD ["python", "-m", "mcp_server_%s"]
`, serverName)
	}
}
