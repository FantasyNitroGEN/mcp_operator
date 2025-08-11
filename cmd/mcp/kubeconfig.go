package main

import (
	"os"

	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// getKubernetesConfigWithPriority implements the proper kubeconfig resolution priority:
// 1. --kubeconfig flag (if specified)
// 2. $KUBECONFIG env var (if file exists)
// 3. ~/.kube/config (default location)
// 4. InClusterConfig (as final fallback)
func getKubernetesConfigWithPriority(kubeconfig string) (*rest.Config, error) {
	// Priority 1: If kubeconfig path is specified via --kubeconfig flag, use it
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}

	// Priority 2: Check KUBECONFIG environment variable
	if kubeconfigEnv := os.Getenv("KUBECONFIG"); kubeconfigEnv != "" {
		// Check if the file exists before trying to use it
		if _, err := os.Stat(kubeconfigEnv); err == nil {
			return clientcmd.BuildConfigFromFlags("", kubeconfigEnv)
		}
	}

	// Priority 3: Try default kubeconfig location (~/.kube/config)
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	configOverrides := &clientcmd.ConfigOverrides{}
	kubeConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, configOverrides)

	if config, err := kubeConfig.ClientConfig(); err == nil {
		return config, nil
	}

	// Priority 4: Fall back to in-cluster config (last resort)
	return rest.InClusterConfig()
}
