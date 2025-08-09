package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	rootCmd := &cobra.Command{
		Use:   "mcp",
		Short: "MCP Operator CLI - Manage MCP servers with Kubernetes",
		Long: `MCP CLI is a command-line tool for managing MCP (Model Context Protocol) servers
using the MCP Operator in Kubernetes clusters.

This tool allows you to:
- List available MCP servers from the registry
- Deploy MCP servers to your Kubernetes cluster
- Manage MCP server configurations`,
		Version: fmt.Sprintf("%s (commit: %s, built: %s)", version, commit, date),
	}

	// Add subcommands
	rootCmd.AddCommand(newRegistryCmd())
	rootCmd.AddCommand(newServerCmd())
	rootCmd.AddCommand(newVersionCmd())

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("mcp version %s\n", version)
			fmt.Printf("commit: %s\n", commit)
			fmt.Printf("built: %s\n", date)
		},
	}
}
