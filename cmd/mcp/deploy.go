package main

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	mcpv1 "github.com/FantasyNitroGEN/mcp_operator/api/v1"
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
		registry    string
		runtimeType string
		image       string
		// Autoscaling flags
		autoscale   bool
		minReplicas int32
		maxReplicas int32
		targetCPU   int32
		// Environment variable flags
		envVars        []string
		envFromSecrets []string
		// Transport flags
		transport string
		httpPath  string
		// Port flags
		ports []string
		port  int32 // deprecated
		// Gateway flags
		gateway      bool
		gatewayImage string
		gatewayPort  int32
		gatewayArgs  []string
		// Istio flags
		istio        bool
		istioHost    string
		istioGateway string
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

  # Deploy with environment variables
  mcp deploy postgres --env POSTGRES_URL=postgresql://user:pass@host:5432/db --env DEBUG=true

  # Deploy with environment variables from secrets
  mcp deploy postgres --env-from-secret postgres-credentials --env-from-secret app-config

  # Deploy with both direct env vars and secret references
  mcp deploy postgres --env POSTGRES_URL=postgresql://host:5432/db --env-from-secret postgres-secret

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
			return runDeploy(args[0], registry, namespace, kubeconfig, timeout, replicas, dryRun, wait, waitTimeout, autoscale, minReplicas, maxReplicas, targetCPU, envVars, envFromSecrets, runtimeType, image, transport, httpPath, ports, port, gateway, gatewayImage, gatewayPort, gatewayArgs, istio, istioHost, istioGateway)
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "Kubernetes namespace to deploy to")
	cmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig file (uses default if not specified)")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "Timeout for Kubernetes operations")
	cmd.Flags().Int32Var(&replicas, "replicas", 1, "Number of replicas to deploy (ignored when autoscaling is enabled)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Print the resource that would be created without actually creating it")
	cmd.Flags().BoolVar(&wait, "wait", false, "Wait for the deployment to be ready")
	cmd.Flags().DurationVar(&waitTimeout, "wait-timeout", 5*time.Minute, "Timeout for waiting for deployment to be ready")
	cmd.Flags().StringVar(&registry, "registry", "", "Registry name to deploy server from (required)")
	cmd.Flags().StringVar(&runtimeType, "runtime", "docker", "Runtime type: docker|stdio")
	cmd.Flags().StringVar(&image, "image", "", "Docker image for runtime=docker (e.g. mcp/postgres)")

	// Transport flags
	cmd.Flags().StringVar(&transport, "transport", "stdio", "Transport type: stdio|http|streamable-http")
	cmd.Flags().StringVar(&httpPath, "http-path", "/mcp", "HTTP path for transport=http|streamable-http")

	// Port flags
	cmd.Flags().StringArrayVar(&ports, "ports", []string{}, "Port configuration name:port[:targetPort[:protocol[:appProtocol]]] (can be repeated)")
	cmd.Flags().Int32Var(&port, "port", 0, "Port number for MCP server (sets MCPServer.Spec.Runtime.Port)")

	// Gateway flags
	cmd.Flags().BoolVar(&gateway, "gateway", false, "Enable gateway")
	cmd.Flags().StringVar(&gatewayImage, "gateway-image", "", "Gateway container image")
	cmd.Flags().Int32Var(&gatewayPort, "gateway-port", 0, "Gateway port")
	cmd.Flags().StringArrayVar(&gatewayArgs, "gateway-arg", []string{}, "Gateway container arguments (can be repeated)")

	// Istio flags
	cmd.Flags().BoolVar(&istio, "istio", false, "Enable Istio integration")
	cmd.Flags().StringVar(&istioHost, "istio-host", "", "Istio host for VirtualService")
	cmd.Flags().StringVar(&istioGateway, "istio-gateway", "", "Istio gateway reference (namespace/name)")

	// Autoscaling flags
	cmd.Flags().BoolVar(&autoscale, "autoscale", false, "Enable horizontal pod autoscaling")
	cmd.Flags().Int32Var(&minReplicas, "min", 1, "Minimum number of replicas for autoscaling")
	cmd.Flags().Int32Var(&maxReplicas, "max", 10, "Maximum number of replicas for autoscaling")
	cmd.Flags().Int32Var(&targetCPU, "target-cpu", 80, "Target CPU utilization percentage for autoscaling")

	// Environment variable flags
	cmd.Flags().StringArrayVar(&envVars, "env", []string{}, "Environment variables in KEY=VALUE format (can be repeated)")
	cmd.Flags().StringArrayVar(&envFromSecrets, "env-from-secret", []string{}, "Load environment variables from secret (can be repeated)")

	return cmd
}

// parsePortSpec parses port specification string in format:
// name:port[:targetPort[:protocol[:appProtocol]]]
// Supports short forms: 8080, name:8080, 8080:8080
func parsePortSpec(portStr string) (mcpv1.PortSpec, error) {
	parts := strings.Split(portStr, ":")
	portSpec := mcpv1.PortSpec{
		Protocol: "TCP", // default protocol
	}

	switch len(parts) {
	case 1:
		// Format: "8080"
		port, err := strconv.ParseInt(parts[0], 10, 32)
		if err != nil {
			return portSpec, fmt.Errorf("invalid port number: %s", parts[0])
		}
		portSpec.Name = fmt.Sprintf("port-%d", port)
		portSpec.Port = int32(port)
		portSpec.TargetPort = int32(port)

	case 2:
		// Format: "name:8080" or "8080:8081"
		port, err := strconv.ParseInt(parts[1], 10, 32)
		if err != nil {
			return portSpec, fmt.Errorf("invalid port number: %s", parts[1])
		}

		// Check if first part is a port number (8080:8081 format)
		if firstPort, err := strconv.ParseInt(parts[0], 10, 32); err == nil {
			// Format: "8080:8081"
			portSpec.Name = fmt.Sprintf("port-%d", firstPort)
			portSpec.Port = int32(firstPort)
			portSpec.TargetPort = int32(port)
		} else {
			// Format: "name:8080"
			portSpec.Name = parts[0]
			portSpec.Port = int32(port)
			portSpec.TargetPort = int32(port)
		}

	case 3:
		// Format: "name:8080:8081"
		port, err := strconv.ParseInt(parts[1], 10, 32)
		if err != nil {
			return portSpec, fmt.Errorf("invalid port number: %s", parts[1])
		}
		targetPort, err := strconv.ParseInt(parts[2], 10, 32)
		if err != nil {
			return portSpec, fmt.Errorf("invalid target port number: %s", parts[2])
		}
		portSpec.Name = parts[0]
		portSpec.Port = int32(port)
		portSpec.TargetPort = int32(targetPort)

	case 4:
		// Format: "name:8080:8081:TCP"
		port, err := strconv.ParseInt(parts[1], 10, 32)
		if err != nil {
			return portSpec, fmt.Errorf("invalid port number: %s", parts[1])
		}
		targetPort, err := strconv.ParseInt(parts[2], 10, 32)
		if err != nil {
			return portSpec, fmt.Errorf("invalid target port number: %s", parts[2])
		}
		if parts[3] != "" && parts[3] != "TCP" && parts[3] != "UDP" {
			return portSpec, fmt.Errorf("invalid protocol: %s (must be TCP or UDP)", parts[3])
		}
		portSpec.Name = parts[0]
		portSpec.Port = int32(port)
		portSpec.TargetPort = int32(targetPort)
		if parts[3] != "" {
			portSpec.Protocol = parts[3]
		}

	case 5:
		// Format: "name:8080:8081:TCP:http"
		port, err := strconv.ParseInt(parts[1], 10, 32)
		if err != nil {
			return portSpec, fmt.Errorf("invalid port number: %s", parts[1])
		}
		targetPort, err := strconv.ParseInt(parts[2], 10, 32)
		if err != nil {
			return portSpec, fmt.Errorf("invalid target port number: %s", parts[2])
		}
		if parts[3] != "" && parts[3] != "TCP" && parts[3] != "UDP" {
			return portSpec, fmt.Errorf("invalid protocol: %s (must be TCP or UDP)", parts[3])
		}
		portSpec.Name = parts[0]
		portSpec.Port = int32(port)
		portSpec.TargetPort = int32(targetPort)
		if parts[3] != "" {
			portSpec.Protocol = parts[3]
		}
		if parts[4] != "" {
			portSpec.AppProtocol = parts[4]
		}

	default:
		return portSpec, fmt.Errorf("invalid port format, expected name:port[:targetPort[:protocol[:appProtocol]]]")
	}

	// Validate port ranges
	if portSpec.Port < 1 || portSpec.Port > 65535 {
		return portSpec, fmt.Errorf("port must be between 1 and 65535, got %d", portSpec.Port)
	}
	if portSpec.TargetPort < 1 || portSpec.TargetPort > 65535 {
		return portSpec, fmt.Errorf("target port must be between 1 and 65535, got %d", portSpec.TargetPort)
	}

	return portSpec, nil
}

func runDeploy(serverName, registryName, namespace, kubeconfig string, timeout time.Duration, replicas int32, dryRun, wait bool, waitTimeout time.Duration, autoscale bool, minReplicas, maxReplicas, targetCPU int32, envVars []string, envFromSecrets []string, runtimeType, image, transport, httpPath string, ports []string, port int32, gateway bool, gatewayImage string, gatewayPort int32, gatewayArgs []string, istio bool, istioHost string, istioGateway string) error {
	logger := log.Log.WithName("mcp-deploy").WithValues(
		"server", serverName,
		"registry", registryName,
		"namespace", namespace,
		"runtime", runtimeType,
	)

	logger.Info("Starting MCP server deployment")

	// Validate that registry name is provided
	if registryName == "" {
		logger.Error(nil, "Registry name is required")
		return fmt.Errorf("registry name is required, use --registry flag")
	}

	// Validate that image is provided when runtime is docker
	if strings.EqualFold(runtimeType, "docker") && strings.TrimSpace(image) == "" {
		logger.Error(nil, "Image is required when runtime is docker", "runtime", runtimeType)
		return fmt.Errorf("--image is required when --runtime=docker")
	}

	// Validate transport type
	validTransports := map[string]bool{"stdio": true, "http": true, "streamable-http": true}
	if !validTransports[transport] {
		logger.Error(nil, "Invalid transport type", "transport", transport, "validOptions", "stdio, http, streamable-http")
		return fmt.Errorf("unsupported transport type: %s. Valid options: stdio, http, streamable-http", transport)
	}

	// Handle --port flag - this will set MCPServer.Spec.Runtime.Port
	// The --ports flag is handled separately and takes priority in the operator

	// Show notice for --http-path with stdio transport
	if transport == "stdio" && httpPath != "/mcp" {
		fmt.Fprintf(os.Stderr, "NOTICE: --http-path is ignored for stdio transport\n")
	}

	// Validate gateway configuration
	if gateway {
		logger.V(1).Info("Validating gateway configuration")
		if gatewayImage == "" {
			logger.Error(nil, "Gateway image is required when gateway is enabled")
			return fmt.Errorf("--gateway-image is required when --gateway is enabled")
		}
		if gatewayPort == 0 {
			logger.Error(nil, "Gateway port is required when gateway is enabled")
			return fmt.Errorf("--gateway-port is required when --gateway is enabled")
		}
	}

	// Validate Istio configuration
	if istio && !gateway {
		logger.Error(nil, "Istio requires gateway to be enabled")
		return fmt.Errorf("--istio requires --gateway to be enabled")
	}

	// Parse and validate ports
	logger.V(1).Info("Parsing port specifications", "portsCount", len(ports))
	var parsedPorts []mcpv1.PortSpec
	for _, portStr := range ports {
		portSpec, err := parsePortSpec(portStr)
		if err != nil {
			logger.Error(err, "Failed to parse port specification", "portSpec", portStr)
			return fmt.Errorf("invalid --ports entry '%s': %w", portStr, err)
		}
		parsedPorts = append(parsedPorts, portSpec)
	}

	fmt.Printf("Creating MCPServer '%s' from registry '%s'...\n", serverName, registryName)

	// Parse environment variables
	envMap := make(map[string]string)
	for _, envVar := range envVars {
		parts := strings.SplitN(envVar, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid environment variable format '%s', expected KEY=VALUE", envVar)
		}
		envMap[parts[0]] = parts[1]
	}

	// Parse envFrom secrets
	var envFromSources []mcpv1.EnvFromSource
	for _, secretName := range envFromSecrets {
		envFromSources = append(envFromSources, mcpv1.EnvFromSource{
			SecretRef: &mcpv1.SecretEnvSource{
				Name: secretName,
			},
		})
	}

	// Create transport spec
	var transportSpec *mcpv1.TransportSpec
	if transport != "" {
		transportSpec = &mcpv1.TransportSpec{
			Type: transport,
		}
		// Only set path for http/streamable-http transports
		if transport == "http" || transport == "streamable-http" {
			transportSpec.Path = httpPath
		}
	}

	// Create gateway spec
	var gatewaySpec *mcpv1.GatewaySpec
	if gateway {
		gatewaySpec = &mcpv1.GatewaySpec{
			Enabled: true,
			Image:   gatewayImage,
			Port:    gatewayPort,
			Args:    gatewayArgs,
		}

		// Add Istio configuration if enabled
		if istio {
			gatewaySpec.Istio = &mcpv1.GatewayIstioSpec{
				Enabled:    true,
				Host:       istioHost,
				GatewayRef: istioGateway,
			}
		}
	}

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
			Registry: &mcpv1.RegistryRef{
				RegistryName: registryName,
				ServerName:   serverName,
			},
			Runtime: &mcpv1.RuntimeSpec{
				Type:  runtimeType,
				Image: image,
				Env:   envMap,
				Port:  port,
			},
			Transport: transportSpec,
			Ports:     parsedPorts,
			Gateway:   gatewaySpec,
			EnvFrom:   envFromSources,
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
	logger.V(1).Info("Creating Kubernetes client", "kubeconfig", kubeconfig)
	k8sClient, err := createKubernetesClient(kubeconfig)
	if err != nil {
		logger.Error(err, "Failed to create Kubernetes client", "kubeconfig", kubeconfig)
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}
	logger.V(1).Info("Successfully created Kubernetes client")

	// Create the MCPServer resource
	fmt.Printf("Creating MCPServer resource '%s' in namespace '%s'...\n", serverName, namespace)
	logger.Info("Creating MCPServer resource", "name", serverName, "namespace", namespace)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err = k8sClient.Create(ctx, mcpServer)
	if err != nil {
		logger.Error(err, "Failed to create MCPServer resource", "name", serverName, "namespace", namespace)
		return fmt.Errorf("create MCPServer %s/%s: %w", mcpServer.Namespace, mcpServer.Name, err)
	}

	fmt.Printf("✓ MCPServer '%s' created successfully\n", serverName)
	logger.Info("Successfully created MCPServer resource", "name", serverName, "namespace", namespace)

	if wait {
		fmt.Printf("Waiting for MCPServer '%s' to be ready (timeout: %v)...\n", serverName, waitTimeout)
		logger.Info("Waiting for MCPServer to be ready", "name", serverName, "timeout", waitTimeout)
		return waitForMCPServerReady(k8sClient, serverName, namespace, waitTimeout)
	}

	fmt.Printf("\nTo check the status of your deployment, run:\n")
	fmt.Printf("  kubectl get mcpserver %s -n %s\n", serverName, namespace)
	fmt.Printf("  kubectl describe mcpserver %s -n %s\n", serverName, namespace)

	return nil
}

func createKubernetesClient(kubeconfig string) (client.Client, error) {
	// Load kubeconfig using the unified priority logic
	config, err := getKubernetesConfigWithPriority(kubeconfig)
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
    registryName: %s
    serverName: %s
  runtime:
    type: %s`,
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
		mcpServer.Spec.Registry.RegistryName,
		mcpServer.Spec.Registry.ServerName,
		mcpServer.Spec.Runtime.Type,
	)

	// Print image if present
	if mcpServer.Spec.Runtime.Image != "" {
		fmt.Printf("\n    image: %s", mcpServer.Spec.Runtime.Image)
	}

	// Print port if present (including 0 for verification)
	fmt.Printf("\n    port: %d", mcpServer.Spec.Runtime.Port)

	// Print environment variables if present
	if len(mcpServer.Spec.Runtime.Env) > 0 {
		fmt.Printf("\n    env:")
		for key, value := range mcpServer.Spec.Runtime.Env {
			fmt.Printf("\n      %s: %s", key, value)
		}
	}

	// Print envFrom sources if present
	if len(mcpServer.Spec.EnvFrom) > 0 {
		fmt.Printf("\n  envFrom:")
		for _, envFrom := range mcpServer.Spec.EnvFrom {
			if envFrom.SecretRef != nil {
				fmt.Printf("\n  - secretRef:")
				fmt.Printf("\n      name: %s", envFrom.SecretRef.Name)
			}
			if envFrom.ConfigMapRef != nil {
				fmt.Printf("\n  - configMapRef:")
				fmt.Printf("\n      name: %s", envFrom.ConfigMapRef.Name)
			}
		}
	}

	// Print transport if present
	if mcpServer.Spec.Transport != nil {
		fmt.Printf("\n  transport:")
		fmt.Printf("\n    type: %s", mcpServer.Spec.Transport.Type)
		if mcpServer.Spec.Transport.Path != "" {
			fmt.Printf("\n    path: %s", mcpServer.Spec.Transport.Path)
		}
	}

	// Print ports if present
	if len(mcpServer.Spec.Ports) > 0 {
		fmt.Printf("\n  ports:")
		for _, port := range mcpServer.Spec.Ports {
			fmt.Printf("\n  - name: %s", port.Name)
			fmt.Printf("\n    port: %d", port.Port)
			if port.TargetPort > 0 {
				fmt.Printf("\n    targetPort: %d", port.TargetPort)
			}
			if port.Protocol != "" {
				fmt.Printf("\n    protocol: %s", port.Protocol)
			}
			if port.AppProtocol != "" {
				fmt.Printf("\n    appProtocol: %s", port.AppProtocol)
			}
		}
	}

	// Print gateway if present
	if mcpServer.Spec.Gateway != nil {
		fmt.Printf("\n  gateway:")
		fmt.Printf("\n    enabled: %t", mcpServer.Spec.Gateway.Enabled)
		if mcpServer.Spec.Gateway.Image != "" {
			fmt.Printf("\n    image: %s", mcpServer.Spec.Gateway.Image)
		}
		if mcpServer.Spec.Gateway.Port > 0 {
			fmt.Printf("\n    port: %d", mcpServer.Spec.Gateway.Port)
		}
		if len(mcpServer.Spec.Gateway.Args) > 0 {
			fmt.Printf("\n    args:")
			for _, arg := range mcpServer.Spec.Gateway.Args {
				fmt.Printf("\n    - %s", arg)
			}
		}
		// Print Istio configuration if present
		if mcpServer.Spec.Gateway.Istio != nil {
			fmt.Printf("\n    istio:")
			fmt.Printf("\n      enabled: %t", mcpServer.Spec.Gateway.Istio.Enabled)
			if mcpServer.Spec.Gateway.Istio.Host != "" {
				fmt.Printf("\n      host: %s", mcpServer.Spec.Gateway.Istio.Host)
			}
			if mcpServer.Spec.Gateway.Istio.GatewayRef != "" {
				fmt.Printf("\n      gatewayRef: %s", mcpServer.Spec.Gateway.Istio.GatewayRef)
			}
		}
	}

	// Print replicas
	if mcpServer.Spec.Replicas != nil {
		fmt.Printf("\n  replicas: %d", *mcpServer.Spec.Replicas)
	}

	fmt.Println()
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

			// Display conditions status
			registryFetched := getConditionStatus(mcpServer, mcpv1.MCPServerConditionRegistryFetched)
			rendered := getConditionStatus(mcpServer, mcpv1.MCPServerConditionRendered)
			applied := getConditionStatus(mcpServer, mcpv1.MCPServerConditionApplied)
			ready := getConditionStatus(mcpServer, mcpv1.MCPServerConditionReady)

			fmt.Printf("⏳ MCPServer status: %s\n", mcpServer.Status.Phase)
			fmt.Printf("   RegistryFetched: %s | Rendered: %s | Applied: %s | Ready: %s\n",
				registryFetched, rendered, applied, ready)
			if mcpServer.Status.Message != "" {
				fmt.Printf("   Message: %s\n", mcpServer.Status.Message)
			}

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

// getConditionStatus returns the status of a specific condition type
func getConditionStatus(mcpServer *mcpv1.MCPServer, conditionType mcpv1.MCPServerConditionType) string {
	for _, condition := range mcpServer.Status.Conditions {
		if condition.Type == conditionType {
			switch condition.Status {
			case "True":
				return "True"
			case "False":
				return "False"
			default:
				return "Unknown"
			}
		}
	}
	return "Unknown"
}
