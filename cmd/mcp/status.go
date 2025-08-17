package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"

	mcpv1 "github.com/FantasyNitroGEN/mcp_operator/api/v1"
)

func newStatusCmd() *cobra.Command {
	var (
		namespace  string
		kubeconfig string
		output     string
		watch      bool
	)

	cmd := &cobra.Command{
		Use:   "status <server-name>",
		Short: "Get the status of an MCP server",
		Long: `Get the status of a deployed MCP server from your Kubernetes cluster.
This command shows the current phase, replica counts, conditions, and other
status information for the specified MCPServer resource.`,
		Example: `  # Get status of a server
  mcp status filesystem-server

  # Get status from a specific namespace
  mcp status filesystem-server --namespace mcp-servers

  # Get status in JSON format
  mcp status filesystem-server --output json

  # Get status in YAML format
  mcp status filesystem-server --output yaml

  # Watch status changes in real-time
  mcp status filesystem-server --watch`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runStatus(args[0], namespace, kubeconfig, output, watch)
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "Kubernetes namespace to check")
	cmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig file (uses default if not specified)")
	cmd.Flags().StringVarP(&output, "output", "o", "", "Output format (json|yaml)")
	cmd.Flags().BoolVarP(&watch, "watch", "w", false, "Watch for status changes")

	return cmd
}

func runStatus(serverName, namespace, kubeconfig, output string, watch bool) error {
	// Create Kubernetes client
	k8sClient, err := createKubernetesClient(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	if watch {
		return watchMCPServerStatus(k8sClient, serverName, namespace, output)
	}

	return getMCPServerStatus(k8sClient, serverName, namespace, output)
}

func getMCPServerStatus(k8sClient client.Client, name, namespace, output string) error {
	ctx := context.Background()

	mcpServer := &mcpv1.MCPServer{}
	err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, mcpServer)
	if err != nil {
		return fmt.Errorf("MCPServer '%s' not found in namespace '%s': %w", name, namespace, err)
	}

	return printMCPServerStatus(mcpServer, output)
}

func watchMCPServerStatus(k8sClient client.Client, name, namespace, output string) error {
	fmt.Printf("Watching status of MCPServer '%s' in namespace '%s' (press Ctrl+C to stop)...\n\n", name, namespace)

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	var lastStatus string

	for range ticker.C {
		ctx := context.Background()
		mcpServer := &mcpv1.MCPServer{}
		err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, mcpServer)
		if err != nil {
			fmt.Printf("Error getting MCPServer status: %v\n", err)
			continue
		}

		// Create a simple status string to detect changes
		currentStatus := fmt.Sprintf("%s-%d-%d-%s",
			mcpServer.Status.Phase,
			mcpServer.Status.Replicas,
			mcpServer.Status.ReadyReplicas,
			mcpServer.Status.Message)

		if currentStatus != lastStatus {
			fmt.Printf("--- %s ---\n", time.Now().Format("15:04:05"))
			if err := printMCPServerStatus(mcpServer, output); err != nil {
				fmt.Printf("Error printing status: %v\n", err)
			}
			fmt.Println()
			lastStatus = currentStatus
		}
	}

	return nil // This will never be reached in normal operation
}

func printMCPServerStatus(mcpServer *mcpv1.MCPServer, output string) error {
	switch strings.ToLower(output) {
	case "json":
		return printMCPServerStatusJSON(mcpServer)
	case "yaml":
		return printMCPServerStatusYAML(mcpServer)
	default:
		return printMCPServerStatusHuman(mcpServer)
	}
}

func printMCPServerStatusJSON(mcpServer *mcpv1.MCPServer) error {
	data, err := json.MarshalIndent(mcpServer, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal to JSON: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

func printMCPServerStatusYAML(mcpServer *mcpv1.MCPServer) error {
	data, err := yaml.Marshal(mcpServer)
	if err != nil {
		return fmt.Errorf("failed to marshal to YAML: %w", err)
	}
	fmt.Print(string(data))
	return nil
}

func printMCPServerStatusHuman(mcpServer *mcpv1.MCPServer) error {
	fmt.Printf("Name: %s\n", mcpServer.Name)
	fmt.Printf("Namespace: %s\n", mcpServer.Namespace)
	fmt.Printf("Phase: %s\n", mcpServer.Status.Phase)

	if mcpServer.Status.Message != "" {
		fmt.Printf("Message: %s\n", mcpServer.Status.Message)
	}

	if mcpServer.Status.Reason != "" {
		fmt.Printf("Reason: %s\n", mcpServer.Status.Reason)
	}

	fmt.Printf("Replicas: %d/%d", mcpServer.Status.ReadyReplicas, mcpServer.Status.Replicas)
	if mcpServer.Status.AvailableReplicas > 0 {
		fmt.Printf(" (Available: %d)", mcpServer.Status.AvailableReplicas)
	}
	fmt.Println()

	if mcpServer.Status.ServiceEndpoint != "" {
		fmt.Printf("Endpoint: %s\n", mcpServer.Status.ServiceEndpoint)
	}

	// Calculate age
	age := time.Since(mcpServer.CreationTimestamp.Time)
	fmt.Printf("Age: %s\n", formatDuration(age))

	// Print registry info if available
	if mcpServer.Spec.Registry.Registry != "" {
		fmt.Printf("Registry: %s\n", mcpServer.Spec.Registry.Registry)
	}

	// Print conditions
	if len(mcpServer.Status.Conditions) > 0 {
		fmt.Println("\nConditions:")
		for _, condition := range mcpServer.Status.Conditions {
			status := "✓"
			if condition.Status != metav1.ConditionTrue {
				status = "✗"
			}
			fmt.Printf("  %s %s: %s", status, condition.Type, condition.Status)
			if condition.Message != "" {
				fmt.Printf(" - %s", condition.Message)
			}
			fmt.Println()
		}
	}

	if mcpServer.Status.LastUpdateTime != nil {
		fmt.Printf("\nLast Update: %s\n", mcpServer.Status.LastUpdateTime.Format("2006-01-02 15:04:05"))
	}

	return nil
}

func formatDuration(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.0fs", d.Seconds())
	}
	if d < time.Hour {
		return fmt.Sprintf("%.0fm", d.Minutes())
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%.0fh", d.Hours())
	}
	return fmt.Sprintf("%.0fd", d.Hours()/24)
}
