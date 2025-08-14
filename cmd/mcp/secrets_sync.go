package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

func newSecretsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "secrets",
		Short: "Manage Kubernetes secrets",
		Long:  "Commands for managing Kubernetes secrets",
	}

	cmd.AddCommand(newSecretsSyncCmd())
	return cmd
}

func newSecretsSyncCmd() *cobra.Command {
	var namespace string
	var name string
	var fromFiles []string
	var prefix string
	var kubeconfig string
	var timeout time.Duration

	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Sync secrets from environment files to Kubernetes",
		Long: `Sync key-value pairs from environment files to a Kubernetes secret.
Loads key=value pairs, ignores comments and empty lines, and creates/updates 
a Kubernetes secret without logging secret values.

Example:
  mcp secrets sync --namespace mcp --name my-secret --from-file .env --from-file .env.prod --prefix POSTGRES_`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if name == "" {
				return fmt.Errorf("--name flag is required")
			}
			if len(fromFiles) == 0 {
				return fmt.Errorf("at least one --from-file flag is required")
			}
			return runSecretsSync(namespace, name, fromFiles, prefix, kubeconfig, timeout)
		},
	}

	cmd.Flags().StringVarP(&namespace, "namespace", "n", "default", "Kubernetes namespace")
	cmd.Flags().StringVar(&name, "name", "", "Secret name (required)")
	cmd.Flags().StringSliceVar(&fromFiles, "from-file", []string{}, "Environment file paths (can be specified multiple times)")
	cmd.Flags().StringVar(&prefix, "prefix", "", "Only sync keys with this prefix")
	cmd.Flags().StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig file")
	cmd.Flags().DurationVar(&timeout, "timeout", 30*time.Second, "Timeout for operations")

	_ = cmd.MarkFlagRequired("name")

	return cmd
}

func runSecretsSync(namespace, name string, fromFiles []string, prefix, kubeconfig string, timeout time.Duration) error {
	// Parse environment files
	envData, err := parseEnvFiles(fromFiles, prefix)
	if err != nil {
		return fmt.Errorf("failed to parse environment files: %w", err)
	}

	if len(envData) == 0 {
		fmt.Printf("No key-value pairs found with prefix '%s'\n", prefix)
		return nil
	}

	// Create Kubernetes client
	clientset, err := createSecretsKubernetesClient(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	// Create or update secret
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	err = createOrUpdateSecret(ctx, clientset, namespace, name, envData)
	if err != nil {
		return fmt.Errorf("failed to create/update secret: %w", err)
	}

	fmt.Printf("Secret '%s' successfully synced in namespace '%s' with %d keys\n", name, namespace, len(envData))
	return nil
}

func parseEnvFiles(filePaths []string, prefix string) (map[string][]byte, error) {
	envData := make(map[string][]byte)

	for _, filePath := range filePaths {
		file, err := os.Open(filePath)
		if err != nil {
			return nil, fmt.Errorf("failed to open file %s: %w", filePath, err)
		}
		defer func() { _ = file.Close() }()

		scanner := bufio.NewScanner(file)
		lineNum := 0

		for scanner.Scan() {
			lineNum++
			line := strings.TrimSpace(scanner.Text())

			// Skip empty lines and comments
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}

			// Parse key=value
			parts := strings.SplitN(line, "=", 2)
			if len(parts) != 2 {
				fmt.Printf("Warning: skipping invalid line %d in %s: %s\n", lineNum, filePath, line)
				continue
			}

			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])

			// Apply prefix filter if specified
			if prefix != "" && !strings.HasPrefix(key, prefix) {
				continue
			}

			// Store as bytes for Kubernetes secret
			envData[key] = []byte(value)
		}

		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
		}
	}

	return envData, nil
}

func createSecretsKubernetesClient(kubeconfig string) (*kubernetes.Clientset, error) {
	var config *rest.Config
	var err error

	if kubeconfig != "" {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
	} else {
		// Try in-cluster config first, then fall back to default kubeconfig
		config, err = rest.InClusterConfig()
		if err != nil {
			config, err = clientcmd.BuildConfigFromFlags("", clientcmd.RecommendedHomeFile)
		}
	}

	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return clientset, nil
}

func createOrUpdateSecret(ctx context.Context, clientset *kubernetes.Clientset, namespace, name string, data map[string][]byte) error {
	secretsClient := clientset.CoreV1().Secrets(namespace)

	// Try to get existing secret
	existingSecret, err := secretsClient.Get(ctx, name, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			// Create new secret
			secret := &corev1.Secret{
				ObjectMeta: metav1.ObjectMeta{
					Name:      name,
					Namespace: namespace,
				},
				Type: corev1.SecretTypeOpaque,
				Data: data,
			}

			_, err = secretsClient.Create(ctx, secret, metav1.CreateOptions{})
			if err != nil {
				return fmt.Errorf("failed to create secret: %w", err)
			}
			fmt.Printf("Secret '%s' created\n", name)
			return nil
		}
		return fmt.Errorf("failed to get secret: %w", err)
	}

	// Update existing secret
	existingSecret.Data = data
	_, err = secretsClient.Update(ctx, existingSecret, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update secret: %w", err)
	}
	fmt.Printf("Secret '%s' updated\n", name)
	return nil
}
