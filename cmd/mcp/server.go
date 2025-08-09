package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcpv1 "github.com/FantasyNitroGEN/mcp_operator/api/v1"
)

func newServerCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "server",
		Short: "Manage MCP Server operations",
		Long:  `Commands for managing MCP Server resources in Kubernetes cluster.`,
	}

	cmd.AddCommand(newServerDeployCmd())
	cmd.AddCommand(newServerUpdateCmd())
	cmd.AddCommand(newServerStatusCmd())
	cmd.AddCommand(newServerListCmd())
	cmd.AddCommand(newServerDeleteCmd())
	cmd.AddCommand(newServerLogsCmd())

	return cmd
}

// Wrapper functions to integrate existing commands into server namespace
func newServerDeployCmd() *cobra.Command {
	return newDeployCmd()
}

func newServerUpdateCmd() *cobra.Command {
	return newUpdateCmd()
}

func newServerStatusCmd() *cobra.Command {
	return newStatusCmd()
}

func newServerListCmd() *cobra.Command {
	var (
		namespace     string
		format        string
		timeout       time.Duration
		kubeconfig    string
		allNamespaces bool
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List MCP servers in the cluster",
		Long: `List all MCP servers in the specified namespace or current namespace.
This command fetches the list of MCPServer resources from the Kubernetes cluster
and displays their names, status, and basic information.`,
		Example: `  # List all servers in current namespace
  mcp server list

  # List servers in specific namespace
  mcp server list --namespace mcp-system

  # List servers in all namespaces
  mcp server list --all-namespaces

  # List servers with JSON output
  mcp server list --output json

  # List servers with YAML output
  mcp server list --output yaml`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServerList(namespace, format, timeout, kubeconfig, allNamespaces)
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Namespace to list servers from (default: current namespace)")
	cmd.Flags().StringVarP(&format, "output", "o", "table", "Output format (table, json, yaml)")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "Timeout for Kubernetes operations")
	cmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig file (uses default if not specified)")
	cmd.Flags().BoolVarP(&allNamespaces, "all-namespaces", "A", false, "List servers across all namespaces")

	return cmd
}

func newServerDeleteCmd() *cobra.Command {
	var (
		namespace   string
		kubeconfig  string
		force       bool
		wait        bool
		waitTimeout time.Duration
		timeout     time.Duration
	)

	cmd := &cobra.Command{
		Use:   "delete <server-name>",
		Short: "Delete an MCP server",
		Long: `Delete a deployed MCP server from your Kubernetes cluster.
This command removes the MCPServer resource, which will cause the MCP Operator
to automatically clean up all related resources (Deployment, Service, etc.).

By default, the command will ask for confirmation before deletion. Use --force
to skip the confirmation prompt.`,
		Example: `  # Delete a server with confirmation
  mcp server delete filesystem-server

  # Delete from a specific namespace
  mcp server delete filesystem-server --namespace mcp-servers

  # Delete without confirmation
  mcp server delete filesystem-server --force

  # Delete and wait for all resources to be removed
  mcp server delete filesystem-server --wait

  # Delete with custom wait timeout
  mcp server delete filesystem-server --wait --timeout 10m`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServerDelete(args[0], namespace, kubeconfig, force, wait, waitTimeout, timeout)
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "Kubernetes namespace to delete from")
	cmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig file (uses default if not specified)")
	cmd.Flags().BoolVar(&force, "force", false, "Skip confirmation prompt")
	cmd.Flags().BoolVar(&wait, "wait", false, "Wait for all related resources to be deleted")
	cmd.Flags().DurationVar(&waitTimeout, "wait-timeout", 5*time.Minute, "Timeout for waiting for deletion to complete")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "Timeout for Kubernetes operations")

	return cmd
}

func newServerLogsCmd() *cobra.Command {
	var (
		namespace  string
		follow     bool
		tail       int64
		container  string
		timeout    time.Duration
		kubeconfig string
	)

	cmd := &cobra.Command{
		Use:   "logs <server-name>",
		Short: "View logs from MCP server pods",
		Long: `View logs from all pods associated with the specified MCP server.
This command finds all pods with labels app.kubernetes.io/instance=<server-name> 
and app.kubernetes.io/component=server, then displays their logs.

When multiple pods exist, logs from each pod are displayed with a header 
indicating the pod name.`,
		Example: `  # View logs from a server
  mcp server logs filesystem-server

  # View logs from a specific namespace
  mcp server logs filesystem-server --namespace mcp-system

  # Follow logs (like kubectl logs -f)
  mcp server logs filesystem-server --follow

  # Show last 50 lines
  mcp server logs filesystem-server --tail 50

  # Specify container name (if pod has multiple containers)
  mcp server logs filesystem-server --container mcp-server`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runServerLogs(args[0], namespace, follow, tail, container, timeout, kubeconfig)
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "", "Namespace to get logs from (default: current namespace)")
	cmd.Flags().BoolVarP(&follow, "follow", "f", false, "Follow log output (like kubectl logs -f)")
	cmd.Flags().Int64Var(&tail, "tail", 100, "Number of lines to show from the end of the logs")
	cmd.Flags().StringVar(&container, "container", "", "Container name (if pod has multiple containers)")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "Timeout for Kubernetes operations")
	cmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig file (uses default if not specified)")

	return cmd
}

func runServerLogs(serverName, namespace string, follow bool, tail int64, container string, timeout time.Duration, kubeconfig string) error {
	// Use current namespace if not specified
	if namespace == "" {
		namespace = getCurrentNamespace()
	}

	// Create controller-runtime client to check if MCPServer exists
	k8sClient, err := createServerKubernetesClientWithConfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// Check if MCPServer exists
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	mcpServer := &mcpv1.MCPServer{}
	err = k8sClient.Get(ctx, client.ObjectKey{Name: serverName, Namespace: namespace}, mcpServer)
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("MCPServer '%s' not found in namespace '%s'", serverName, namespace)
		}
		return fmt.Errorf("failed to get MCPServer: %w", err)
	}

	// Create standard Kubernetes client for logs API
	config, err := getKubernetesConfig()
	if err != nil {
		return fmt.Errorf("failed to get Kubernetes config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes clientset: %w", err)
	}

	// Find pods with the required labels
	labelSelector := labels.Set{
		"app.kubernetes.io/instance":  serverName,
		"app.kubernetes.io/component": "server",
	}.AsSelector()

	pods, err := clientset.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector.String(),
	})
	if err != nil {
		return fmt.Errorf("failed to list pods: %w", err)
	}

	if len(pods.Items) == 0 {
		return fmt.Errorf("no pods found for MCPServer '%s' in namespace '%s'", serverName, namespace)
	}

	// If follow is enabled and there are multiple pods, we need to handle them concurrently
	if follow && len(pods.Items) > 1 {
		return followLogsFromMultiplePods(clientset, pods.Items, container, tail)
	}

	// For non-follow mode or single pod, process sequentially
	for i, pod := range pods.Items {
		if len(pods.Items) > 1 {
			fmt.Printf("*** Logs from pod %s ***\n", pod.Name)
		}

		err := getPodLogs(clientset, pod, container, follow, tail)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error getting logs from pod %s: %v\n", pod.Name, err)
			continue
		}

		// Add separator between pods (except for the last one)
		if i < len(pods.Items)-1 && len(pods.Items) > 1 {
			fmt.Println()
		}
	}

	return nil
}

func followLogsFromMultiplePods(clientset *kubernetes.Clientset, pods []corev1.Pod, container string, tail int64) error {
	var wg sync.WaitGroup

	for _, pod := range pods {
		wg.Add(1)
		go func(p corev1.Pod) {
			defer wg.Done()

			fmt.Printf("*** Following logs from pod %s ***\n", p.Name)
			err := getPodLogs(clientset, p, container, true, tail)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error following logs from pod %s: %v\n", p.Name, err)
			}
		}(pod)
	}

	wg.Wait()
	return nil
}

func getPodLogs(clientset *kubernetes.Clientset, pod corev1.Pod, container string, follow bool, tail int64) error {
	// Determine container name
	containerName := container
	if containerName == "" {
		// Use the first container if not specified
		if len(pod.Spec.Containers) > 0 {
			containerName = pod.Spec.Containers[0].Name
		} else {
			return fmt.Errorf("no containers found in pod %s", pod.Name)
		}
	}

	// Validate container exists
	containerExists := false
	for _, c := range pod.Spec.Containers {
		if c.Name == containerName {
			containerExists = true
			break
		}
	}
	if !containerExists {
		return fmt.Errorf("container '%s' not found in pod %s", containerName, pod.Name)
	}

	// Prepare log options
	logOptions := &corev1.PodLogOptions{
		Container: containerName,
		Follow:    follow,
		TailLines: &tail,
	}

	// Get logs
	req := clientset.CoreV1().Pods(pod.Namespace).GetLogs(pod.Name, logOptions)
	podLogs, err := req.Stream(context.Background())
	if err != nil {
		return fmt.Errorf("failed to get log stream: %w", err)
	}
	defer func() {
		if closeErr := podLogs.Close(); closeErr != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close log stream: %v\n", closeErr)
		}
	}()

	// Stream logs to stdout
	_, err = io.Copy(os.Stdout, podLogs)
	if err != nil {
		return fmt.Errorf("failed to copy logs: %w", err)
	}

	return nil
}

func runServerDelete(serverName, namespace, kubeconfig string, force, wait bool, waitTimeout, timeout time.Duration) error {
	// Create Kubernetes client
	k8sClient, err := createServerKubernetesClient()
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// First, check if the MCPServer exists
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	mcpServer := &mcpv1.MCPServer{}
	err = k8sClient.Get(ctx, client.ObjectKey{Name: serverName, Namespace: namespace}, mcpServer)
	if err != nil {
		if errors.IsNotFound(err) {
			return fmt.Errorf("MCPServer '%s' не найден в namespace '%s'", serverName, namespace)
		}
		return fmt.Errorf("failed to get MCPServer '%s' in namespace '%s': %w", serverName, namespace, err)
	}

	fmt.Printf("Найден MCPServer '%s' в namespace '%s'\n", serverName, namespace)

	// Ask for confirmation unless --force is used
	if !force {
		fmt.Printf("\nВы уверены, что хотите удалить MCPServer '%s'?\n", serverName)
		fmt.Printf("Это действие удалит сервер и все связанные ресурсы (Deployment, Service, ConfigMap и т.д.)\n")
		fmt.Print("Продолжить? (y/N): ")

		reader := bufio.NewReader(os.Stdin)
		response, err := reader.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read user input: %w", err)
		}

		response = strings.TrimSpace(strings.ToLower(response))
		if response != "y" && response != "yes" && response != "да" {
			fmt.Println("Удаление отменено пользователем")
			return nil
		}
	}

	fmt.Printf("Удаление MCPServer '%s' в namespace '%s'...\n", serverName, namespace)

	// Delete the MCPServer resource
	ctx, cancel = context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err = k8sClient.Delete(ctx, mcpServer)
	if err != nil {
		return fmt.Errorf("failed to delete MCPServer: %w", err)
	}

	fmt.Printf("✓ MCPServer '%s' удаление инициировано\n", serverName)

	if wait {
		fmt.Printf("Ожидание удаления всех связанных ресурсов (таймаут: %v)...\n", waitTimeout)
		return waitForMCPServerDeletion(k8sClient, serverName, namespace, waitTimeout)
	}

	fmt.Printf("\nДля проверки завершения удаления выполните:\n")
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

func runServerList(namespace, format string, timeout time.Duration, kubeconfig string, allNamespaces bool) error {
	// Create Kubernetes client
	k8sClient, err := createServerKubernetesClientWithConfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Handle namespace logic
	var listOpts client.ListOptions
	var namespaceMsg string

	if allNamespaces {
		namespaceMsg = "all namespaces"
		// No namespace restriction for all namespaces
	} else {
		if namespace == "" {
			namespace = getCurrentNamespace()
		}
		namespaceMsg = fmt.Sprintf("namespace '%s'", namespace)
		listOpts.Namespace = namespace
	}

	fmt.Printf("Fetching MCP servers from %s...\n", namespaceMsg)

	// List MCPServer resources
	var serverList mcpv1.MCPServerList
	if err := k8sClient.List(ctx, &serverList, &listOpts); err != nil {
		return fmt.Errorf("failed to list MCPServer resources: %w", err)
	}

	if len(serverList.Items) == 0 {
		fmt.Printf("No MCPServer found in %s.\n", namespaceMsg)
		return nil
	}

	switch format {
	case "json":
		return printMCPServersJSON(serverList.Items)
	case "yaml":
		return printMCPServersYAML(serverList.Items)
	default:
		return printMCPServersTable(serverList.Items)
	}
}

func createServerKubernetesClient() (client.Client, error) {
	return createServerKubernetesClientWithConfig("")
}

func createServerKubernetesClientWithConfig(kubeconfig string) (client.Client, error) {
	// Register MCPServer types with the scheme
	if err := mcpv1.AddToScheme(scheme.Scheme); err != nil {
		return nil, fmt.Errorf("failed to add MCPServer types to scheme: %w", err)
	}

	// Get Kubernetes config
	config, err := getKubernetesConfigWithPath(kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to get Kubernetes config: %w", err)
	}

	// Create controller-runtime client
	k8sClient, err := client.New(config, client.Options{
		Scheme: scheme.Scheme,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	return k8sClient, nil
}

func getKubernetesConfig() (*rest.Config, error) {
	return getKubernetesConfigWithPath("")
}

func getKubernetesConfigWithPath(kubeconfig string) (*rest.Config, error) {
	// If kubeconfig path is specified, use it
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}

	// Try in-cluster config first
	if config, err := rest.InClusterConfig(); err == nil {
		return config, nil
	}

	// Fall back to default kubeconfig
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	return kubeConfig.ClientConfig()
}

func getCurrentNamespace() string {
	// Try to get current namespace from kubeconfig context
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	if namespace, _, err := kubeConfig.Namespace(); err == nil && namespace != "" {
		return namespace
	}

	// Default to "default" namespace
	return "default"
}

func printMCPServersTable(servers []mcpv1.MCPServer) error {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	defer func() {
		if err := w.Flush(); err != nil {
			fmt.Fprintf(os.Stderr, "Error flushing output: %v\n", err)
		}
	}()

	// Print header
	if _, err := fmt.Fprintln(w, "NAME\tPHASE\tREPLICAS\tREADY\tAGE"); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}
	if _, err := fmt.Fprintln(w, "----\t-----\t--------\t-----\t---"); err != nil {
		return fmt.Errorf("failed to write separator: %w", err)
	}

	// Print server information
	for _, server := range servers {
		name := server.Name
		phase := string(server.Status.Phase)
		if phase == "" {
			phase = "Unknown"
		}

		replicas := fmt.Sprintf("%d", server.Status.Replicas)
		ready := fmt.Sprintf("%d", server.Status.ReadyReplicas)
		age := calculateAge(server.CreationTimestamp.Time)

		if _, err := fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n",
			name, phase, replicas, ready, age); err != nil {
			return fmt.Errorf("failed to write server info: %w", err)
		}
	}

	return nil
}

func printMCPServersJSON(servers []mcpv1.MCPServer) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(servers)
}

func printMCPServersYAML(servers []mcpv1.MCPServer) error {
	encoder := yaml.NewEncoder(os.Stdout)
	defer func() {
		if err := encoder.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Error closing YAML encoder: %v\n", err)
		}
	}()
	return encoder.Encode(servers)
}

func calculateAge(creationTime time.Time) string {
	now := time.Now()
	duration := now.Sub(creationTime)

	if duration < time.Minute {
		return fmt.Sprintf("%ds", int(duration.Seconds()))
	} else if duration < time.Hour {
		return fmt.Sprintf("%dm", int(duration.Minutes()))
	} else if duration < 24*time.Hour {
		return fmt.Sprintf("%dh", int(duration.Hours()))
	} else {
		days := int(duration.Hours() / 24)
		return fmt.Sprintf("%dd", days)
	}
}
