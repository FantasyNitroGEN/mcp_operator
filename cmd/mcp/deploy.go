package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	mcpv1 "github.com/FantasyNitroGEN/mcp_operator/api/v1"
	"github.com/FantasyNitroGEN/mcp_operator/pkg/registry"
)

func newDeployCmd() *cobra.Command {
	var (
		namespace   string
		kubeconfig  string
		timeout     time.Duration
		replicas    int32
		dryRun      bool
		wait        bool
		waitTimeout time.Duration
		// Autoscaling flags
		autoscale   bool
		minReplicas int32
		maxReplicas int32
		targetCPU   int32
	)

	cmd := &cobra.Command{
		Use:   "deploy <server-name>",
		Short: "Deploy an MCP server from the registry to Kubernetes",
		Long: `Deploy an MCP server from the Docker MCP Registry to your Kubernetes cluster.
This command creates an MCPServer resource that the MCP Operator will automatically
enrich with registry data and deploy as a running server.`,
		Example: `  # Deploy a filesystem server
  mcp deploy filesystem-server

  # Deploy to a specific namespace
  mcp deploy filesystem-server --namespace mcp-servers

  # Deploy with custom replicas
  mcp deploy filesystem-server --replicas 3

  # Deploy with autoscaling enabled
  mcp deploy filesystem-server --autoscale --min 1 --max 5 --target-cpu 70

  # Deploy with autoscaling using default values (min=1, max=10, target-cpu=80%)
  mcp deploy filesystem-server --autoscale

  # Dry run to see what would be created
  mcp deploy filesystem-server --dry-run

  # Deploy and wait for it to be ready
  mcp deploy filesystem-server --wait --wait-timeout 5m`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runDeploy(args[0], namespace, kubeconfig, timeout, replicas, dryRun, wait, waitTimeout, autoscale, minReplicas, maxReplicas, targetCPU)
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "Kubernetes namespace to deploy to")
	cmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig file (uses default if not specified)")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "Timeout for Kubernetes operations")
	cmd.Flags().Int32Var(&replicas, "replicas", 1, "Number of replicas to deploy (ignored when autoscaling is enabled)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print the resource that would be created without actually creating it")
	cmd.Flags().BoolVar(&wait, "wait", false, "Wait for the deployment to be ready")
	cmd.Flags().DurationVar(&waitTimeout, "wait-timeout", 5*time.Minute, "Timeout for waiting for deployment to be ready")

	// Autoscaling flags
	cmd.Flags().BoolVar(&autoscale, "autoscale", false, "Enable horizontal pod autoscaling")
	cmd.Flags().Int32Var(&minReplicas, "min", 1, "Minimum number of replicas for autoscaling")
	cmd.Flags().Int32Var(&maxReplicas, "max", 10, "Maximum number of replicas for autoscaling")
	cmd.Flags().Int32Var(&targetCPU, "target-cpu", 80, "Target CPU utilization percentage for autoscaling")

	return cmd
}

func runDeploy(serverName, namespace, kubeconfig string, timeout time.Duration, replicas int32, dryRun, wait bool, waitTimeout time.Duration, autoscale bool, minReplicas, maxReplicas, targetCPU int32) error {
	// First, verify the server exists in the registry
	fmt.Printf("Verifying server '%s' exists in registry...\n", serverName)

	registryClient := registry.NewClient()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	// Try to get server spec to verify it exists
	_, err := registryClient.GetServerSpec(ctx, serverName)
	if err != nil {
		return fmt.Errorf("server '%s' not found in registry: %w", serverName, err)
	}

	fmt.Printf("✓ Server '%s' found in registry\n", serverName)

	// Create MCPServer resource
	mcpServer := &mcpv1.MCPServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serverName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/name":       "mcp-server",
				"app.kubernetes.io/instance":   serverName,
				"app.kubernetes.io/component":  "mcp-server",
				"app.kubernetes.io/created-by": "mcp-cli",
			},
			Annotations: map[string]string{
				"mcp.allbeone.io/deployed-by": "mcp-cli",
				"mcp.allbeone.io/deployed-at": time.Now().Format(time.RFC3339),
			},
		},
		Spec: mcpv1.MCPServerSpec{
			Registry: mcpv1.MCPRegistryInfo{
				Name: serverName,
			},
			Runtime: mcpv1.MCPRuntimeSpec{
				Type: "docker", // Default to docker, will be enriched from registry
			},
		},
	}

	// Configure autoscaling if enabled
	if autoscale {
		fmt.Printf("Configuring autoscaling: min=%d, max=%d, target-cpu=%d%%\n", minReplicas, maxReplicas, targetCPU)
		mcpServer.Spec.Autoscaling = &mcpv1.AutoscalingSpec{
			HPA: &mcpv1.HPASpec{
				Enabled:                        true,
				MinReplicas:                    &minReplicas,
				MaxReplicas:                    maxReplicas,
				TargetCPUUtilizationPercentage: &targetCPU,
			},
		}
		// When autoscaling is enabled, don't set static replicas
		// The HPA will manage replica count dynamically
	} else {
		// Only set replicas when autoscaling is not enabled
		mcpServer.Spec.Replicas = &replicas
	}

	if dryRun {
		fmt.Println("\n--- MCPServer Resource (dry-run) ---")
		return printMCPServerYAML(mcpServer)
	}

	// Create Kubernetes client
	k8sClient, err := createKubernetesClient(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// Create the MCPServer resource
	fmt.Printf("Creating MCPServer resource '%s' in namespace '%s'...\n", serverName, namespace)

	ctx, cancel = context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err = k8sClient.Create(ctx, mcpServer)
	if err != nil {
		return fmt.Errorf("failed to create MCPServer resource: %w", err)
	}

	fmt.Printf("✓ MCPServer '%s' created successfully\n", serverName)

	if wait {
		fmt.Printf("Waiting for MCPServer '%s' to be ready (timeout: %v)...\n", serverName, waitTimeout)
		return waitForMCPServerReady(k8sClient, serverName, namespace, waitTimeout)
	}

	fmt.Printf("\nTo check the status of your deployment, run:\n")
	fmt.Printf("  kubectl get mcpserver %s -n %s\n", serverName, namespace)
	fmt.Printf("  kubectl describe mcpserver %s -n %s\n", serverName, namespace)

	return nil
}

func createKubernetesClient(kubeconfig string) (client.Client, error) {
	// Load kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return nil, fmt.Errorf("failed to load kubeconfig: %w", err)
	}

	// Create scheme and add MCP types
	sch := runtime.NewScheme()
	if err := scheme.AddToScheme(sch); err != nil {
		return nil, fmt.Errorf("failed to add core types to scheme: %w", err)
	}
	if err := mcpv1.AddToScheme(sch); err != nil {
		return nil, fmt.Errorf("failed to add MCP types to scheme: %w", err)
	}

	// Create client
	k8sClient, err := client.New(config, client.Options{Scheme: sch})
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	return k8sClient, nil
}

func printMCPServerYAML(mcpServer *mcpv1.MCPServer) error {
	// Simple YAML-like output for dry-run
	fmt.Printf(`apiVersion: %s
kind: %s
metadata:
  name: %s
  namespace: %s
  labels:
    app.kubernetes.io/name: %s
    app.kubernetes.io/instance: %s
    app.kubernetes.io/component: %s
    app.kubernetes.io/created-by: %s
  annotations:
    mcp.allbeone.io/deployed-by: %s
    mcp.allbeone.io/deployed-at: %s
spec:
  registry:
    name: %s
  runtime:
    type: %s
  replicas: %d
`,
		"mcp.allbeone.io/v1",
		"MCPServer",
		mcpServer.Name,
		mcpServer.Namespace,
		mcpServer.Labels["app.kubernetes.io/name"],
		mcpServer.Labels["app.kubernetes.io/instance"],
		mcpServer.Labels["app.kubernetes.io/component"],
		mcpServer.Labels["app.kubernetes.io/created-by"],
		mcpServer.Annotations["mcp.allbeone.io/deployed-by"],
		mcpServer.Annotations["mcp.allbeone.io/deployed-at"],
		mcpServer.Spec.Registry.Name,
		mcpServer.Spec.Runtime.Type,
		*mcpServer.Spec.Replicas,
	)
	return nil
}

func waitForMCPServerReady(k8sClient client.Client, name, namespace string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for MCPServer to be ready")
		case <-ticker.C:
			mcpServer := &mcpv1.MCPServer{}
			err := k8sClient.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, mcpServer)
			if err != nil {
				fmt.Printf("⏳ Waiting for MCPServer to be created...\n")
				continue
			}

			fmt.Printf("⏳ MCPServer status: %s", mcpServer.Status.Phase)
			if mcpServer.Status.Message != "" {
				fmt.Printf(" - %s", mcpServer.Status.Message)
			}
			fmt.Println()

			if mcpServer.Status.Phase == mcpv1.MCPServerPhaseRunning && mcpServer.Status.ReadyReplicas > 0 {
				fmt.Printf("✓ MCPServer '%s' is ready!\n", name)
				if mcpServer.Status.ServiceEndpoint != "" {
					fmt.Printf("  Service endpoint: %s\n", mcpServer.Status.ServiceEndpoint)
				}
				return nil
			}

			if mcpServer.Status.Phase == mcpv1.MCPServerPhaseFailed {
				return fmt.Errorf("MCPServer deployment failed: %s", mcpServer.Status.Message)
			}
		}
	}
}
