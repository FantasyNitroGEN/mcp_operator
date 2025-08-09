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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"

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
	client := registry.NewClient()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	fmt.Println("Fetching MCP servers from registry...")

	servers, err := client.ListServers(ctx)
	if err != nil {
		return fmt.Errorf("failed to list servers from registry: %w", err)
	}

	if len(servers) == 0 {
		fmt.Println("No servers found in the registry.")
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

	// Initialize Kubernetes client
	var cfg *rest.Config
	var err error
	if kubeconfig != "" {
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	} else {
		cfg, err = config.GetConfig()
	}
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
