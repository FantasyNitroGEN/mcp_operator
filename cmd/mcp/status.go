package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
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

	for {
		select {
		case <-ticker.C:
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
	}
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
	if mcpServer.Spec.Registry.Name != "" {
		fmt.Printf("Registry: %s\n", mcpServer.Spec.Registry.Name)
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

func newDeleteCmd() *cobra.Command {
	var (
		namespace   string
		kubeconfig  string
		wait        bool
		waitTimeout time.Duration
	)

	cmd := &cobra.Command{
		Use:   "delete <server-name>",
		Short: "Delete an MCP server",
		Long: `Delete a deployed MCP server from your Kubernetes cluster.
This command removes the MCPServer resource, which will cause the MCP Operator
to automatically clean up all related resources (Deployment, Service, etc.).`,
		Example: `  # Delete a server
  mcp delete filesystem-server

  # Delete from a specific namespace
  mcp delete filesystem-server --namespace mcp-servers

  # Delete and wait for all resources to be removed
  mcp delete filesystem-server --wait

  # Delete with custom wait timeout
  mcp delete filesystem-server --wait --wait-timeout 10m`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDelete(args[0], namespace, kubeconfig, wait, waitTimeout)
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "Kubernetes namespace to delete from")
	cmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig file (uses default if not specified)")
	cmd.Flags().BoolVar(&wait, "wait", false, "Wait for all related resources to be deleted")
	cmd.Flags().DurationVar(&waitTimeout, "wait-timeout", 5*time.Minute, "Timeout for waiting for deletion to complete")

	return cmd
}

func runDelete(serverName, namespace, kubeconfig string, wait bool, waitTimeout time.Duration) error {
	// Create Kubernetes client
	k8sClient, err := createKubernetesClient(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// First, check if the MCPServer exists
	ctx := context.Background()
	mcpServer := &mcpv1.MCPServer{}
	err = k8sClient.Get(ctx, client.ObjectKey{Name: serverName, Namespace: namespace}, mcpServer)
	if err != nil {
		return fmt.Errorf("MCPServer '%s' not found in namespace '%s': %w", serverName, namespace, err)
	}

	fmt.Printf("Deleting MCPServer '%s' in namespace '%s'...\n", serverName, namespace)

	// Delete the MCPServer resource
	err = k8sClient.Delete(ctx, mcpServer)
	if err != nil {
		return fmt.Errorf("failed to delete MCPServer: %w", err)
	}

	fmt.Printf("✓ MCPServer '%s' deletion initiated\n", serverName)

	if wait {
		fmt.Printf("Waiting for all related resources to be deleted (timeout: %v)...\n", waitTimeout)
		return waitForMCPServerDeletion(k8sClient, serverName, namespace, waitTimeout)
	}

	fmt.Printf("\nTo check if deletion is complete, run:\n")
	fmt.Printf("  kubectl get mcpserver %s -n %s\n", serverName, namespace)
	fmt.Printf("  kubectl get deployment %s -n %s\n", serverName, namespace)

	return nil
}

func waitForMCPServerDeletion(k8sClient client.Client, name, namespace string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for MCPServer and related resources to be deleted")
		case <-ticker.C:
			// Check if MCPServer still exists
			mcpServer := &mcpv1.MCPServer{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, mcpServer)
			if err != nil {
				// MCPServer is gone, now check if Deployment is also gone
				if err := checkDeploymentDeleted(k8sClient, ctx, name, namespace); err != nil {
					fmt.Printf("⏳ MCPServer deleted, waiting for Deployment cleanup...\n")
					continue
				}

				fmt.Printf("✓ MCPServer '%s' and all related resources have been deleted\n", name)
				return nil
			}

			// MCPServer still exists, show its phase
			if mcpServer.Status.Phase == mcpv1.MCPServerPhaseTerminating {
				fmt.Printf("⏳ MCPServer is terminating...\n")
			} else {
				fmt.Printf("⏳ MCPServer deletion in progress (Phase: %s)...\n", mcpServer.Status.Phase)
			}
		}
	}
}

func checkDeploymentDeleted(k8sClient client.Client, ctx context.Context, name, namespace string) error {
	// Check if the Deployment still exists
	deployment := &appsv1.Deployment{}
	err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, deployment)
	if err != nil {
		if errors.IsNotFound(err) {
			// Deployment is deleted, which is what we want
			return nil
		}
		// Some other error occurred
		return fmt.Errorf("error checking deployment status: %w", err)
	}
	// Deployment still exists
	return fmt.Errorf("deployment still exists")
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
