package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcpv1 "github.com/FantasyNitroGEN/mcp_operator/api/v1"
	"github.com/FantasyNitroGEN/mcp_operator/pkg/registry"
)

func newRegistryCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "registry",
		Short: "Manage MCP Registry operations",
		Long:  `Commands for interacting with the MCP Registry to list and search available MCP servers.`,
	}

	cmd.AddCommand(newRegistryListCmd())
	cmd.AddCommand(newRegistryServersCmd())
	cmd.AddCommand(newRegistrySearchCmd())
	cmd.AddCommand(newRegistryInspectCmd())
	cmd.AddCommand(newRegistryRefreshCmd())

	return cmd
}

func newRegistryListCmd() *cobra.Command {
	var (
		timeout time.Duration
		format  string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available MCP servers from the registry",
		Long: `List all available MCP servers from the Docker MCP Registry.
This command fetches the list of servers from the public registry and displays
their names and basic information.`,
		Example: `  # List all available servers
  mcp registry list

  # List servers with JSON output
  mcp registry list --output json

  # List servers with custom timeout
  mcp registry list --timeout 60s`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRegistryList(timeout, format)
		},
	}

	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "Timeout for registry operations")
	cmd.Flags().StringVarP(&format, "output", "o", "table", "Output format (table, json, yaml)")

	return cmd
}

func newRegistrySearchCmd() *cobra.Command {
	var (
		timeout time.Duration
		format  string
	)

	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search for MCP servers in the registry",
		Long: `Search for MCP servers in the registry using a query string.
The search will match server names and descriptions.`,
		Example: `  # Search for filesystem-related servers
  mcp registry search filesystem

  # Search with JSON output
  mcp registry search database --output json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRegistrySearch(args[0], timeout, format)
		},
	}

	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "Timeout for registry operations")
	cmd.Flags().StringVarP(&format, "output", "o", "table", "Output format (table, json, yaml)")

	return cmd
}

func newRegistryServersCmd() *cobra.Command {
	var (
		timeout time.Duration
		format  string
	)

	cmd := &cobra.Command{
		Use:   "servers <registry>",
		Short: "List servers from a specific registry cache",
		Long: `List servers available in a specific MCP registry from the cached ConfigMaps in the cluster.
This command reads from the registry cache ConfigMaps instead of accessing GitHub directly.`,
		Example: `  # List servers from default registry
  mcp registry servers default-registry

  # List servers with JSON output
  mcp registry servers my-registry --output json

  # List servers with custom timeout
  mcp registry servers my-registry --timeout 60s`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRegistryServers(args[0], timeout, format)
		},
	}

	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "Timeout for Kubernetes operations")
	cmd.Flags().StringVarP(&format, "output", "o", "table", "Output format (table, yaml, json)")

	return cmd
}

func newRegistryInspectCmd() *cobra.Command {
	var (
		timeout time.Duration
		format  string
	)

	cmd := &cobra.Command{
		Use:   "inspect <server>",
		Short: "Inspect a specific MCP server template from the registry",
		Long: `Inspect a specific MCP server template from the registry to view its detailed specification.
This command fetches the server.yaml file for the specified server and displays
its complete configuration including description, runtime, environment variables,
capabilities, and configuration schema.`,
		Example: `  # Inspect a server with YAML output (default)
  mcp registry inspect filesystem-server

  # Inspect a server with JSON output
  mcp registry inspect filesystem-server --output json

  # Inspect with custom timeout
  mcp registry inspect database-server --timeout 60s`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRegistryInspect(args[0], timeout, format)
		},
	}

	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "Timeout for registry operations")
	cmd.Flags().StringVarP(&format, "output", "o", "yaml", "Output format (yaml, json)")

	return cmd
}

func runRegistryList(timeout time.Duration, format string) error {
	// Create Kubernetes client
	k8sClient, err := createKubernetesClient("")
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	fmt.Println("Fetching MCPRegistry resources from cluster...")

	// List all MCPRegistry resources
	registryList := &mcpv1.MCPRegistryList{}
	err = k8sClient.List(ctx, registryList)
	if err != nil {
		return fmt.Errorf("failed to list MCPRegistry resources: %w", err)
	}

	if len(registryList.Items) == 0 {
		fmt.Println("No MCPRegistry resources found in the cluster.")
		return nil
	}

	switch format {
	case "json":
		return printRegistriesJSON(registryList.Items)
	case "yaml":
		return printRegistriesYAML(registryList.Items)
	default:
		return printRegistriesTable(registryList.Items)
	}
}

func runRegistrySearch(query string, timeout time.Duration, format string) error {
	client := registry.NewClient()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	fmt.Printf("Searching for '%s' in MCP registry...\n", query)

	servers, err := client.SearchServers(ctx, query)
	if err != nil {
		return fmt.Errorf("failed to search servers in registry: %w", err)
	}

	if len(servers) == 0 {
		fmt.Printf("No servers found matching '%s'.\n", query)
		return nil
	}

	switch format {
	case "json":
		return printServersJSON(servers)
	case "yaml":
		return printServersYAML(servers)
	default:
		return printServersTable(servers)
	}
}

func runRegistryServers(registryName string, timeout time.Duration, format string) error {
	// Create Kubernetes client
	k8sClient, err := createKubernetesClient("")
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	fmt.Printf("Fetching servers from registry '%s' cache...\n", registryName)

	// First, find the MCPRegistry to get its namespace
	registryList := &mcpv1.MCPRegistryList{}
	err = k8sClient.List(ctx, registryList)
	if err != nil {
		return fmt.Errorf("failed to list MCPRegistry resources: %w", err)
	}

	var targetRegistry *mcpv1.MCPRegistry
	for _, registry := range registryList.Items {
		if registry.Name == registryName {
			targetRegistry = &registry
			break
		}
	}

	if targetRegistry == nil {
		return fmt.Errorf("registry '%s' not found in cluster", registryName)
	}

	// List ConfigMaps with registry label in the target namespace
	configMaps := &corev1.ConfigMapList{}
	err = k8sClient.List(ctx, configMaps,
		client.InNamespace(targetRegistry.Namespace),
		client.MatchingLabels{
			"mcp.allbeone.io/registry": registryName,
		})
	if err != nil {
		return fmt.Errorf("failed to list ConfigMaps for registry '%s': %w", registryName, err)
	}

	// Extract server information from ConfigMaps
	var servers []string
	var serverDetails []CachedServerInfo
	for _, cm := range configMaps.Items {
		if serverName, exists := cm.Labels["mcp.allbeone.io/server"]; exists {
			servers = append(servers, serverName)

			// Extract additional info if available
			serverInfo := CachedServerInfo{
				Name:          serverName,
				Registry:      registryName,
				Namespace:     cm.Namespace,
				ConfigMapName: cm.Name,
				LastUpdated:   cm.CreationTimestamp,
			}

			// Try to parse server.yaml content for description
			if yamlContent, exists := cm.Data["server.yaml"]; exists && len(yamlContent) > 0 {
				serverInfo.HasSpec = true
			}

			serverDetails = append(serverDetails, serverInfo)
		}
	}

	if len(servers) == 0 {
		fmt.Printf("No servers found in registry '%s' cache.\n", registryName)
		return nil
	}

	switch format {
	case "json":
		return printCachedServersJSON(serverDetails)
	case "yaml":
		return printCachedServersYAML(serverDetails)
	default:
		return printCachedServersTable(serverDetails)
	}
}

// CachedServerInfo represents server information from cache
type CachedServerInfo struct {
	Name          string      `json:"name"`
	Registry      string      `json:"registry"`
	Namespace     string      `json:"namespace"`
	ConfigMapName string      `json:"configMapName"`
	HasSpec       bool        `json:"hasSpec"`
	LastUpdated   metav1.Time `json:"lastUpdated"`
}

func printCachedServersTable(servers []CachedServerInfo) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	defer func() {
		if err := w.Flush(); err != nil {
			fmt.Fprintf(os.Stderr, "Error flushing output: %v\n", err)
		}
	}()

	if _, err := fmt.Fprintln(w, "NAME\tREGISTRY\tNAMESPACE\tHAS-SPEC\tLAST-UPDATED"); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}
	if _, err := fmt.Fprintln(w, "----\t--------\t---------\t--------\t------------"); err != nil {
		return fmt.Errorf("failed to write separator: %w", err)
	}

	for _, server := range servers {
		hasSpecStr := "false"
		if server.HasSpec {
			hasSpecStr = "true"
		}

		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			server.Name,
			server.Registry,
			server.Namespace,
			hasSpecStr,
			server.LastUpdated.Format("2006-01-02 15:04:05"),
		); err != nil {
			return fmt.Errorf("failed to write server info: %w", err)
		}
	}

	return nil
}

func printCachedServersJSON(servers []CachedServerInfo) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(servers)
}

func printCachedServersYAML(servers []CachedServerInfo) error {
	encoder := yaml.NewEncoder(os.Stdout)
	defer func() {
		if err := encoder.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing YAML encoder: %v\n", err)
		}
	}()
	return encoder.Encode(servers)
}

func runRegistryInspect(serverName string, timeout time.Duration, format string) error {
	client := registry.NewClient()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	fmt.Printf("Fetching server specification for '%s'...\n", serverName)

	spec, err := client.GetServerSpec(ctx, serverName)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return fmt.Errorf("server '%s' not found in registry", serverName)
		}
		return fmt.Errorf("failed to get server specification: %w", err)
	}

	switch format {
	case "json":
		return printServerSpecJSON(spec)
	default: // yaml is default
		return printServerSpecYAML(spec)
	}
}

func printServerSpecJSON(spec *registry.MCPServerSpec) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(spec)
}

func printServerSpecYAML(spec *registry.MCPServerSpec) error {
	encoder := yaml.NewEncoder(os.Stdout)
	defer func() {
		if err := encoder.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing YAML encoder: %v\n", err)
		}
	}()
	return encoder.Encode(spec)
}

func printServersTable(servers []registry.MCPServerInfo) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	defer func() {
		if err := w.Flush(); err != nil {
			fmt.Fprintf(os.Stderr, "Error flushing output: %v\n", err)
		}
	}()

	if _, err := fmt.Fprintln(w, "NAME\tUPDATED\tSIZE"); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}
	if _, err := fmt.Fprintln(w, "----\t-------\t----"); err != nil {
		return fmt.Errorf("failed to write separator: %w", err)
	}

	for _, server := range servers {
		if _, err := fmt.Fprintf(w, "%s\t%s\t%d bytes\n",
			server.Name,
			server.UpdatedAt.Format("2006-01-02 15:04:05"),
			server.Size,
		); err != nil {
			return fmt.Errorf("failed to write server info: %w", err)
		}
	}

	return nil
}

func printRegistriesTable(registries []mcpv1.MCPRegistry) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	defer func() {
		if err := w.Flush(); err != nil {
			fmt.Fprintf(os.Stderr, "Error flushing output: %v\n", err)
		}
	}()

	if _, err := fmt.Fprintln(w, "NAME\tNAMESPACE\tPHASE\tSERVERS\tLAST SYNC"); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}
	if _, err := fmt.Fprintln(w, "----\t---------\t-----\t-------\t---------"); err != nil {
		return fmt.Errorf("failed to write separator: %w", err)
	}

	for _, registry := range registries {
		lastSync := "Never"
		if registry.Status.LastSyncTime != nil {
			lastSync = registry.Status.LastSyncTime.Format("2006-01-02 15:04:05")
		}

		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\n",
			registry.Name,
			registry.Namespace,
			registry.Status.Phase,
			registry.Status.ServersDiscovered,
			lastSync,
		); err != nil {
			return fmt.Errorf("failed to write registry info: %w", err)
		}
	}

	return nil
}

func printRegistriesJSON(registries []mcpv1.MCPRegistry) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(registries)
}

func printRegistriesYAML(registries []mcpv1.MCPRegistry) error {
	encoder := yaml.NewEncoder(os.Stdout)
	defer func() {
		if err := encoder.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing YAML encoder: %v\n", err)
		}
	}()
	return encoder.Encode(registries)
}

func printServersJSON(servers []registry.MCPServerInfo) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(servers)
}

func printServersYAML(servers []registry.MCPServerInfo) error {
	encoder := yaml.NewEncoder(os.Stdout)
	defer func() {
		if err := encoder.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing YAML encoder: %v\n", err)
		}
	}()
	return encoder.Encode(servers)
}

func newRegistryRefreshCmd() *cobra.Command {
	var (
		timeout    time.Duration
		namespace  string
		name       string
		kubeconfig string
	)

	cmd := &cobra.Command{
		Use:   "refresh",
		Short: "Refresh the MCP registry cache",
		Long: `Refresh the local cache of MCP templates by triggering synchronization 
with the remote registry. This ensures that 'mcp registry list' shows the most 
up-to-date information about available MCP servers.

The command finds the MCPRegistry resource in the cluster and triggers its 
reconciliation, which will update the cached server list.`,
		Example: `  # Refresh the default registry
  mcp registry refresh

  # Refresh a specific registry by name
  mcp registry refresh --name myregistry

  # Refresh registry in a specific namespace
  mcp registry refresh --namespace mcp-system

  # Refresh with custom timeout
  mcp registry refresh --timeout 60s`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRegistryRefresh(timeout, namespace, name, kubeconfig)
		},
	}

	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "Timeout for registry operations")
	cmd.Flags().StringVar(&namespace, "namespace", "", "Namespace to search for MCPRegistry (empty for all namespaces)")
	cmd.Flags().StringVar(&name, "name", "", "Name of the MCPRegistry to refresh (empty to find any)")
	cmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig file (uses default if not specified)")

	return cmd
}

func runRegistryRefresh(timeout time.Duration, namespace, name, kubeconfig string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Initialize Kubernetes client using unified kubeconfig priority logic
	cfg, err := getKubernetesConfigWithPriority(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	// Add our scheme to the default scheme
	if err := mcpv1.AddToScheme(scheme.Scheme); err != nil {
		return fmt.Errorf("failed to add MCP scheme: %w", err)
	}

	k8sClient, err := client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	fmt.Println("Looking for MCPRegistry resources...")

	// Find MCPRegistry resource(s)
	var registry *mcpv1.MCPRegistry
	if name != "" {
		// Look for specific registry by name
		registry = &mcpv1.MCPRegistry{}
		key := types.NamespacedName{
			Name:      name,
			Namespace: namespace,
		}
		if namespace == "" {
			// If no namespace specified, try default namespace first
			key.Namespace = "default"
		}

		err = k8sClient.Get(ctx, key, registry)
		if err != nil {
			if namespace == "" {
				// Try mcp-system namespace as fallback
				key.Namespace = "mcp-system"
				err = k8sClient.Get(ctx, key, registry)
			}
			if err != nil {
				return fmt.Errorf("MCPRegistry '%s' not found: %w", name, err)
			}
		}
	} else {
		// Find any MCPRegistry
		registryList := &mcpv1.MCPRegistryList{}
		listOpts := []client.ListOption{}
		if namespace != "" {
			listOpts = append(listOpts, client.InNamespace(namespace))
		}

		if err := k8sClient.List(ctx, registryList, listOpts...); err != nil {
			return fmt.Errorf("failed to list MCPRegistry resources: %w", err)
		}

		if len(registryList.Items) == 0 {
			// No MCPRegistry found, create a default one
			return createDefaultRegistry(ctx, k8sClient, namespace)
		}

		if len(registryList.Items) > 1 && name == "" {
			fmt.Println("Multiple MCPRegistry resources found:")
			for _, reg := range registryList.Items {
				fmt.Printf("  - %s/%s\n", reg.Namespace, reg.Name)
			}
			return fmt.Errorf("multiple registries found, please specify --name")
		}

		registry = &registryList.Items[0]
	}

	fmt.Printf("Found MCPRegistry: %s/%s\n", registry.Namespace, registry.Name)

	// Trigger registry sync by updating annotation
	if registry.Annotations == nil {
		registry.Annotations = make(map[string]string)
	}
	registry.Annotations["mcp.allbeone.io/sync-trigger"] = time.Now().Format(time.RFC3339)

	if err := k8sClient.Update(ctx, registry); err != nil {
		return fmt.Errorf("failed to trigger registry sync: %w", err)
	}

	fmt.Println("✓ Registry cache refresh triggered successfully")
	fmt.Printf("The registry controller will sync with the remote registry and update the cache.\n")
	fmt.Printf("You can check the status with: kubectl get mcpregistry %s -n %s\n", registry.Name, registry.Namespace)

	return nil
}

func createDefaultRegistry(ctx context.Context, k8sClient client.Client, namespace string) error {
	if namespace == "" {
		namespace = "default"
	}

	fmt.Printf("No MCPRegistry found. Creating default registry in namespace '%s'...\n", namespace)

	registry := &mcpv1.MCPRegistry{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default",
			Namespace: namespace,
			Annotations: map[string]string{
				"mcp.allbeone.io/sync-trigger": time.Now().Format(time.RFC3339),
			},
		},
		Spec: mcpv1.MCPRegistrySpec{
			URL:  "https://api.github.com/repos/modelcontextprotocol/servers/contents/src",
			Type: "github",
			SyncInterval: &metav1.Duration{
				Duration: 30 * time.Minute,
			},
		},
	}

	if err := k8sClient.Create(ctx, registry); err != nil {
		return fmt.Errorf("failed to create default MCPRegistry: %w", err)
	}

	fmt.Printf("✓ Default MCPRegistry created: %s/%s\n", registry.Namespace, registry.Name)
	fmt.Println("✓ Registry cache refresh triggered successfully")
	fmt.Printf("You can check the status with: kubectl get mcpregistry %s -n %s\n", registry.Name, registry.Namespace)

	return nil
}
