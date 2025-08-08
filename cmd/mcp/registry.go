package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

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
  mcp registry list --format json

  # List servers with custom timeout
  mcp registry list --timeout 60s`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRegistryList(timeout, format)
		},
	}

	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "Timeout for registry operations")
	cmd.Flags().StringVar(&format, "format", "table", "Output format (table, json, yaml)")

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
  mcp registry search database --format json`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRegistrySearch(args[0], timeout, format)
		},
	}

	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "Timeout for registry operations")
	cmd.Flags().StringVar(&format, "format", "table", "Output format (table, json, yaml)")

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

func printServersTable(servers []registry.MCPServerInfo) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	defer w.Flush()

	fmt.Fprintln(w, "NAME\tUPDATED\tSIZE")
	fmt.Fprintln(w, "----\t-------\t----")

	for _, server := range servers {
		fmt.Fprintf(w, "%s\t%s\t%d bytes\n",
			server.Name,
			server.UpdatedAt.Format("2006-01-02 15:04:05"),
			server.Size,
		)
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
	defer encoder.Close()
	return encoder.Encode(servers)
}
