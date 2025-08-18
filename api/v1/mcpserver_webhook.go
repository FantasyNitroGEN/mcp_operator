package v1

import (
	"strings"
)

// Default sets default values for MCPServer
func (r *MCPServer) Default() {
	if r.Spec.Registry == nil {
		r.Spec.Registry = &RegistryRef{}
	}
	// алиас с "registry" -> "name", если вдруг такой есть
	if r.Spec.Registry.Name == "" && r.Spec.Registry.Registry != "" {
		r.Spec.Registry.Name = r.Spec.Registry.Registry
	}
	if strings.TrimSpace(r.Spec.Registry.Name) == "" {
		r.Spec.Registry.Name = "default-registry"
	}
	if strings.TrimSpace(r.Spec.Registry.ServerName) == "" {
		r.Spec.Registry.ServerName = r.Name
	}
}

// ValidateCreate validates MCPServer on creation
func (r *MCPServer) ValidateCreate() error {
	// Only check runtime.image for docker, don't require ports for STDIO
	if r.Spec.Runtime != nil && r.Spec.Runtime.Type == "docker" {
		if r.Spec.Runtime.Image == "" {
			return nil // Allow empty image for non-docker runtimes
		}
	}
	return nil
}
