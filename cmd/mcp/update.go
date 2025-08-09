package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcpv1 "github.com/FantasyNitroGEN/mcp_operator/api/v1"
)

func newUpdateCmd() *cobra.Command {
	var (
		namespace   string
		kubeconfig  string
		timeout     time.Duration
		image       string
		replicas    int32
		wait        bool
		waitTimeout time.Duration
		dryRun      bool
	)

	cmd := &cobra.Command{
		Use:   "update <server-name>",
		Short: "Update an existing MCP server configuration",
		Long: `Update the configuration of an already deployed MCP server.
This command allows you to modify server parameters like container image or replica count
without manual YAML editing or redeployment.`,
		Example: `  # Update server image
  mcp update filesystem-server --image myregistry/filesystem-server:v2.0

  # Update replica count
  mcp update filesystem-server --replicas 3

  # Update both image and replicas
  mcp update filesystem-server --image myregistry/filesystem-server:v2.0 --replicas 3

  # Update and wait for rollout to complete
  mcp update filesystem-server --image myregistry/filesystem-server:v2.0 --wait --wait-timeout 10m

  # Update server in specific namespace
  mcp update filesystem-server --namespace mcp-servers --replicas 2

  # Dry run to see what would be updated
  mcp update filesystem-server --image myregistry/filesystem-server:v2.0 --dry-run`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runUpdate(args[0], namespace, kubeconfig, timeout, image, replicas, wait, waitTimeout, dryRun)
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "Kubernetes namespace where the server is deployed")
	cmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig file (uses default if not specified)")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "Timeout for Kubernetes operations")
	cmd.Flags().StringVar(&image, "image", "", "Update container image")
	cmd.Flags().Int32Var(&replicas, "replicas", 0, "Update number of replicas (0 means no change)")
	cmd.Flags().BoolVar(&wait, "wait", false, "Wait for the update to be ready")
	cmd.Flags().DurationVar(&waitTimeout, "wait-timeout", 5*time.Minute, "Timeout for waiting for update to be ready")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print the updated resource without applying changes")

	return cmd
}

func runUpdate(serverName, namespace, kubeconfig string, timeout time.Duration, image string, replicas int32, wait bool, waitTimeout time.Duration, dryRun bool) error {
	// Validate that at least one update parameter is provided
	if image == "" && replicas == 0 {
		return fmt.Errorf("at least one update parameter must be specified (--image or --replicas)")
	}

	// Validate incompatible flag combinations
	if dryRun && wait {
		return fmt.Errorf("--dry-run and --wait flags are incompatible: cannot wait for changes that are not applied")
	}

	// Create Kubernetes client
	k8sClient, err := createKubernetesClient(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// Get current MCPServer resource
	fmt.Printf("Получение текущей конфигурации MCPServer '%s'...\n", serverName)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	mcpServer := &mcpv1.MCPServer{}
	err = k8sClient.Get(ctx, client.ObjectKey{Name: serverName, Namespace: namespace}, mcpServer)
	if err != nil {
		return fmt.Errorf("failed to get MCPServer '%s' in namespace '%s': %w", serverName, namespace, err)
	}

	fmt.Printf("✓ MCPServer '%s' найден\n", serverName)

	// Track what we're updating
	var updates []string

	// Update image if specified
	if image != "" {
		fmt.Printf("Обновление образа: %s -> %s\n", mcpServer.Spec.Runtime.Image, image)
		mcpServer.Spec.Runtime.Image = image
		updates = append(updates, fmt.Sprintf("образ: %s", image))
	}

	// Update replicas if specified
	if replicas > 0 {
		currentReplicas := int32(1) // default
		if mcpServer.Spec.Replicas != nil {
			currentReplicas = *mcpServer.Spec.Replicas
		}
		fmt.Printf("Обновление количества реплик: %d -> %d\n", currentReplicas, replicas)
		mcpServer.Spec.Replicas = &replicas
		updates = append(updates, fmt.Sprintf("реплики: %d", replicas))
	}

	// Update the annotation to track the update
	if mcpServer.Annotations == nil {
		mcpServer.Annotations = make(map[string]string)
	}
	mcpServer.Annotations["mcp.allbeone.io/updated-by"] = "mcp-cli"
	mcpServer.Annotations["mcp.allbeone.io/updated-at"] = time.Now().Format(time.RFC3339)

	// Handle dry-run mode
	if dryRun {
		fmt.Printf("Dry-run режим: показываем обновленный ресурс без применения изменений\n")
		fmt.Printf("Обновления (%s):\n", fmt.Sprintf("%v", updates))
		fmt.Println("---")

		// Print the updated MCPServer YAML
		return printMCPServerYAML(mcpServer)
	}

	// Apply the update
	fmt.Printf("Применение обновлений (%s)...\n", fmt.Sprintf("%v", updates))

	ctx, cancel = context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err = k8sClient.Update(ctx, mcpServer)
	if err != nil {
		return fmt.Errorf("failed to update MCPServer: %w", err)
	}

	fmt.Printf("✓ MCPServer '%s' успешно обновлен\n", serverName)

	if wait {
		fmt.Printf("Ожидание готовности MCPServer '%s' (таймаут: %v)...\n", serverName, waitTimeout)
		return waitForMCPServerReady(k8sClient, serverName, namespace, waitTimeout)
	}

	fmt.Printf("\nДля проверки статуса обновления выполните:\n")
	fmt.Printf("  kubectl get mcpserver %s -n %s\n", serverName, namespace)
	fmt.Printf("  kubectl describe mcpserver %s -n %s\n", serverName, namespace)

	return nil
}
