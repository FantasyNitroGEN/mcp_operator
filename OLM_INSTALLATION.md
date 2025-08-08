# OLM Installation Guide for MCP Operator

This guide explains how to install and manage the MCP Operator using the Operator Lifecycle Manager (OLM).

## Prerequisites

- Kubernetes cluster with OLM installed
- `kubectl` configured to access your cluster
- Docker or Podman for building bundle images

## Installing OLM (if not already installed)

If your cluster doesn't have OLM installed, you can install it using:

```bash
# Install OLM
curl -sL https://github.com/operator-framework/operator-lifecycle-manager/releases/download/v0.28.0/install.sh | bash -s v0.28.0

# Verify OLM installation
kubectl get csv -A
```

## Building and Publishing the Bundle

### 1. Build the Bundle Image

```bash
# Build the bundle image
make bundle-build BUNDLE_IMG=your-registry/mcp-operator-bundle:v1.0.0

# Push the bundle image to your registry
make bundle-push BUNDLE_IMG=your-registry/mcp-operator-bundle:v1.0.0
```

### 2. Validate the Bundle

```bash
# Validate bundle structure
make bundle-validate

# Build and validate using operator-sdk (if available)
operator-sdk bundle validate ./bundle
```

## Installing via OLM

### Method 1: Using operator-sdk run bundle

```bash
# Install the operator using the bundle
operator-sdk run bundle your-registry/mcp-operator-bundle:v1.0.0

# Check installation status
kubectl get csv -n operators

# Check operator pods
kubectl get pods -n operators
```

### Method 2: Manual Installation with CatalogSource

Create a CatalogSource to make the operator available in OperatorHub:

```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: mcp-operator-catalog
  namespace: olm
spec:
  sourceType: grpc
  image: your-registry/mcp-operator-bundle:v1.0.0
  displayName: MCP Operator Catalog
  publisher: FantasyNitroGEN
```

Apply the CatalogSource:

```bash
kubectl apply -f catalogsource.yaml
```

Then create a Subscription:

```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: mcp-operator
  namespace: operators
spec:
  channel: stable
  name: mcp-operator
  source: mcp-operator-catalog
  sourceNamespace: olm
```

Apply the Subscription:

```bash
kubectl apply -f subscription.yaml
```

## Verifying Installation

### Check Operator Status

```bash
# Check ClusterServiceVersion
kubectl get csv -n operators

# Check operator deployment
kubectl get deployment -n operators

# Check operator logs
kubectl logs -n operators deployment/mcp-operator-controller-manager
```

### Verify CRDs Installation

```bash
# Check if MCPServer CRD is installed
kubectl get crd mcpservers.mcp.allbeone.io

# Check if MCPServerBackup CRD is installed
kubectl get crd mcpserverbackups.mcp.allbeone.io
```

## Using the Operator

Once installed via OLM, you can create MCPServer resources:

```yaml
apiVersion: mcp.allbeone.io/v1
kind: MCPServer
metadata:
  name: example-mcpserver
  namespace: default
spec:
  registry:
    name: filesystem-server
  runtime:
    type: python
  replicas: 1
```

Apply the resource:

```bash
kubectl apply -f mcpserver.yaml

# Check status
kubectl get mcpserver
kubectl describe mcpserver example-mcpserver
```

## Upgrading the Operator

### Using operator-sdk

```bash
# Build new bundle version
make bundle-build BUNDLE_IMG=your-registry/mcp-operator-bundle:v1.1.0
make bundle-push BUNDLE_IMG=your-registry/mcp-operator-bundle:v1.1.0

# Upgrade using operator-sdk
operator-sdk run bundle-upgrade your-registry/mcp-operator-bundle:v1.1.0
```

### Using OLM Subscription

Update the CatalogSource with the new bundle image and OLM will automatically upgrade the operator based on the subscription configuration.

## Uninstalling the Operator

### Using operator-sdk

```bash
# Cleanup operator installation
operator-sdk cleanup mcp-operator
```

### Manual Cleanup

```bash
# Delete all MCPServer resources first
kubectl delete mcpserver --all -A

# Delete the subscription
kubectl delete subscription mcp-operator -n operators

# Delete the ClusterServiceVersion
kubectl delete csv mcp-operator.v1.0.0 -n operators

# Delete CRDs (optional, be careful as this removes all custom resources)
kubectl delete crd mcpservers.mcp.allbeone.io
kubectl delete crd mcpserverbackups.mcp.allbeone.io
```

## Troubleshooting

### Common Issues

1. **Bundle validation fails**
   ```bash
   # Check bundle structure
   make bundle-validate
   
   # Validate with operator-sdk
   operator-sdk bundle validate ./bundle
   ```

2. **Operator fails to start**
   ```bash
   # Check operator logs
   kubectl logs -n operators deployment/mcp-operator-controller-manager
   
   # Check events
   kubectl get events -n operators --sort-by='.lastTimestamp'
   ```

3. **CRDs not installed**
   ```bash
   # Check if CRDs are in the bundle
   ls -la bundle/manifests/
   
   # Manually install CRDs if needed
   kubectl apply -f bundle/manifests/mcp.allbeone.io_mcpservers.yaml
   kubectl apply -f bundle/manifests/mcp.allbeone.io_mcpserverbackups.yaml
   ```

### Getting Help

- Check the [MCP Operator GitHub repository](https://github.com/FantasyNitroGEN/mcp_operator)
- Review operator logs for detailed error messages
- Validate bundle structure using the provided Makefile targets

## Bundle Structure

The OLM bundle includes:

```
bundle/
├── manifests/
│   ├── mcp-operator.clusterserviceversion.yaml  # Main CSV
│   ├── mcp.allbeone.io_mcpservers.yaml         # MCPServer CRD
│   └── mcp.allbeone.io_mcpserverbackups.yaml   # MCPServerBackup CRD
├── metadata/
│   └── annotations.yaml                         # Bundle metadata
└── bundle.Dockerfile                            # Bundle image definition
```

This structure follows OLM standards and enables the operator to be distributed through OperatorHub or private catalogs.