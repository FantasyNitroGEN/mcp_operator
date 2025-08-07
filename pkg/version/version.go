package version

import (
	"context"
	"fmt"
	"runtime/debug"
	"strings"

	"k8s.io/client-go/discovery"
	"k8s.io/client-go/rest"
)

var (
	// These variables can be set at build time using ldflags
	// Example: go build -ldflags "-X github.com/FantasyNitroGEN/mcp_operator/pkg/version.Version=v1.0.0"
	Version   = "dev"     // Operator version
	GitCommit = "unknown" // Git commit hash
	BuildDate = "unknown" // Build date
)

// BuildInfo contains build-time information about the operator
type BuildInfo struct {
	Version   string
	GitCommit string
	BuildDate string
	GoVersion string
}

// ClusterInfo contains information about the Kubernetes cluster
type ClusterInfo struct {
	Version    string
	GitVersion string
	Platform   string
}

// GetBuildInfo returns build information about the operator
func GetBuildInfo() BuildInfo {
	info := BuildInfo{
		Version:   Version,
		GitCommit: GitCommit,
		BuildDate: BuildDate,
		GoVersion: "unknown",
	}

	// Try to get additional info from runtime/debug if available
	if buildInfo, ok := debug.ReadBuildInfo(); ok {
		info.GoVersion = buildInfo.GoVersion

		// If version is still "dev", try to get it from build info
		if info.Version == "dev" {
			for _, setting := range buildInfo.Settings {
				switch setting.Key {
				case "vcs.revision":
					if info.GitCommit == "unknown" {
						// Use first 7 characters of commit hash
						if len(setting.Value) >= 7 {
							info.GitCommit = setting.Value[:7]
						} else {
							info.GitCommit = setting.Value
						}
					}
				case "vcs.time":
					if info.BuildDate == "unknown" {
						info.BuildDate = setting.Value
					}
				}
			}
		}
	}

	return info
}

// GetOperatorVersion returns the operator version string
// This function prioritizes build-time version, falls back to VCS info
func GetOperatorVersion() string {
	buildInfo := GetBuildInfo()

	// If we have a proper version set at build time, use it
	if buildInfo.Version != "dev" {
		return buildInfo.Version
	}

	// Otherwise, create a version from git commit if available
	if buildInfo.GitCommit != "unknown" {
		return fmt.Sprintf("dev-%s", buildInfo.GitCommit)
	}

	// Final fallback
	return "dev-unknown"
}

// GetClusterVersion returns the Kubernetes cluster version using REST config
func GetClusterVersion(ctx context.Context, config *rest.Config) (string, error) {
	// Create a discovery client using the REST config
	discoveryClient, err := discovery.NewDiscoveryClientForConfig(config)
	if err != nil {
		return "", fmt.Errorf("failed to create discovery client: %w", err)
	}

	// Get server version
	serverVersion, err := discoveryClient.ServerVersion()
	if err != nil {
		return "", fmt.Errorf("failed to get server version: %w", err)
	}

	return serverVersion.GitVersion, nil
}

// GetClusterVersionSimple returns a simplified cluster version (e.g., "1.28.0")
func GetClusterVersionSimple(ctx context.Context, config *rest.Config) (string, error) {
	fullVersion, err := GetClusterVersion(ctx, config)
	if err != nil {
		return "", err
	}

	// Extract just the version number (remove 'v' prefix and any suffixes)
	version := strings.TrimPrefix(fullVersion, "v")

	// Split by '-' to remove any suffixes like "-gke.1000"
	parts := strings.Split(version, "-")
	if len(parts) > 0 {
		return parts[0], nil
	}

	return version, nil
}

// GetClusterInfo returns comprehensive cluster information
func GetClusterInfo(ctx context.Context, config *rest.Config) (ClusterInfo, error) {
	info := ClusterInfo{}

	// Get server version
	fullVersion, err := GetClusterVersion(ctx, config)
	if err != nil {
		return info, err
	}

	info.GitVersion = fullVersion

	// Extract simple version
	simpleVersion, err := GetClusterVersionSimple(ctx, config)
	if err != nil {
		return info, err
	}
	info.Version = simpleVersion

	// Try to determine platform from version string
	if strings.Contains(fullVersion, "gke") {
		info.Platform = "GKE"
	} else if strings.Contains(fullVersion, "eks") {
		info.Platform = "EKS"
	} else if strings.Contains(fullVersion, "aks") {
		info.Platform = "AKS"
	} else if strings.Contains(fullVersion, "k3s") {
		info.Platform = "K3s"
	} else if strings.Contains(fullVersion, "kind") {
		info.Platform = "kind"
	} else if strings.Contains(fullVersion, "minikube") {
		info.Platform = "minikube"
	} else {
		info.Platform = "kubernetes"
	}

	return info, nil
}
