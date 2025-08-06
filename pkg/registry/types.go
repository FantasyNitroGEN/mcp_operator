package registry

import (
	"encoding/json"
	"time"
)

// MCPServerSpec представляет спецификацию MCP сервера из реестра
type MCPServerSpec struct {
	Name         string            `json:"name" yaml:"name"`
	Version      string            `json:"version" yaml:"version"`
	Description  string            `json:"description" yaml:"description"`
	Repository   string            `json:"repository" yaml:"repository"`
	License      string            `json:"license" yaml:"license"`
	Author       string            `json:"author" yaml:"author"`
	Homepage     string            `json:"homepage" yaml:"homepage"`
	Keywords     []string          `json:"keywords" yaml:"keywords"`
	Runtime      RuntimeSpec       `json:"runtime" yaml:"runtime"`
	Config       ConfigSpec        `json:"config" yaml:"config"`
	Capabilities []string          `json:"capabilities" yaml:"capabilities"`
	Environment  map[string]string `json:"environment" yaml:"environment"`
}

// RuntimeSpec описывает среду выполнения MCP сервера
type RuntimeSpec struct {
	Type    string            `json:"type" yaml:"type"`       // "docker", "node", "python", etc.
	Image   string            `json:"image" yaml:"image"`     // Docker image
	Command []string          `json:"command" yaml:"command"` // Команда запуска
	Args    []string          `json:"args" yaml:"args"`       // Аргументы команды
	Env     map[string]string `json:"env" yaml:"env"`         // Переменные окружения
}

// ConfigSpec описывает конфигурацию MCP сервера
type ConfigSpec struct {
	Schema     json.RawMessage        `json:"schema" yaml:"schema"`         // JSON Schema для конфигурации
	Required   []string               `json:"required" yaml:"required"`     // Обязательные параметры
	Properties map[string]interface{} `json:"properties" yaml:"properties"` // Свойства конфигурации
}

// MCPServerInfo содержит метаинформацию о сервере из реестра
type MCPServerInfo struct {
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	UpdatedAt   time.Time `json:"updated_at"`
	Size        int64     `json:"size"`
	DownloadURL string    `json:"download_url"`
	HTMLURL     string    `json:"html_url"`
}

// ServerYAML представляет структуру server.yaml файла из реестра
type ServerYAML struct {
	Name   string     `yaml:"name"`
	Image  string     `yaml:"image"`
	Type   string     `yaml:"type"`
	Meta   MetaInfo   `yaml:"meta"`
	About  AboutInfo  `yaml:"about"`
	Source SourceInfo `yaml:"source"`
}

// MetaInfo содержит метаинформацию о сервере
type MetaInfo struct {
	Category string   `yaml:"category"`
	Tags     []string `yaml:"tags"`
}

// AboutInfo содержит описательную информацию о сервере
type AboutInfo struct {
	Title       string `yaml:"title"`
	Description string `yaml:"description"`
	Icon        string `yaml:"icon"`
}

// SourceInfo содержит информацию об исходном коде сервера
type SourceInfo struct {
	Project   string `yaml:"project"`
	Directory string `yaml:"directory"`
}

// RegistryResponse представляет ответ от GitHub API
type RegistryResponse struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	SHA         string `json:"sha"`
	Size        int64  `json:"size"`
	URL         string `json:"url"`
	HTMLURL     string `json:"html_url"`
	GitURL      string `json:"git_url"`
	DownloadURL string `json:"download_url"`
	Type        string `json:"type"`
	Content     string `json:"content,omitempty"`
	Encoding    string `json:"encoding,omitempty"`
}
