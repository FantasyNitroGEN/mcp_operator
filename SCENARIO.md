# End-to-End Scenario: MCP Operator with Registry Integration

This document demonstrates a complete end-to-end scenario of using the MCP Operator with the enhanced registry integration and CLI tools.

## Scenario Overview

We'll walk through the complete process:
1. Installing the MCP Operator
2. Using the CLI to explore available servers
3. Deploying a server using the CLI
4. Deploying a server using YAML with registry integration
5. Monitoring and troubleshooting

## Prerequisites

- Kubernetes cluster (1.19+)
- kubectl configured
- Helm 3.2.0+
- Internet access for registry operations

## Step 1: Install MCP Operator

### Using Helm (Recommended)

```bash
# Add the Helm repository
helm repo add mcp-operator https://fantasynitrogen.github.io/mcp_operator/
helm repo update

# Install the operator
helm install mcp-operator mcp-operator/mcp-operator

# Verify installation
kubectl get pods -l app.kubernetes.io/name=mcp-operator
```

Expected output:
```
NAME                            READY   STATUS    RESTARTS   AGE
mcp-operator-7d4b8c9f8d-xyz12   1/1     Running   0          30s
```

## Step 2: Install and Use MCP CLI

### Build the CLI

```bash
# Clone the repository
git clone https://github.com/FantasyNitroGEN/mcp_operator.git
cd mcp-operator

# Build the CLI
go build -o bin/mcp ./cmd/mcp

# Make it available in PATH (optional)
sudo mv bin/mcp /usr/local/bin/
```

### Explore Available Servers

```bash
# List all available MCP servers from the registry
mcp registry list
```

Expected output:
```
Fetching MCP servers from registry...
NAME                    UPDATED              SIZE
----                    -------              ----
filesystem-server       2024-01-15 10:30:45  2048 bytes
database-server         2024-01-14 15:22:10  3072 bytes
web-scraper-server      2024-01-13 09:15:30  2560 bytes
```

```bash
# Search for specific servers
mcp registry search filesystem
```

Expected output:
```
Searching for 'filesystem' in MCP registry...
NAME                    UPDATED              SIZE
----                    -------              ----
filesystem-server       2024-01-15 10:30:45  2048 bytes
```

## Step 3: Deploy Using CLI

### Simple Deployment

```bash
# Deploy a filesystem server using CLI
mcp deploy filesystem-server
```

Expected output:
```
Verifying server 'filesystem-server' exists in registry...
✓ Server 'filesystem-server' found in registry
Creating MCPServer resource 'filesystem-server' in namespace 'default'...
✓ MCPServer 'filesystem-server' created successfully

To check the status of your deployment, run:
  kubectl get mcpserver filesystem-server -n default
  kubectl describe mcpserver filesystem-server -n default
```

### Advanced Deployment with Options

```bash
# Deploy with custom options and wait for readiness
mcp deploy database-server \
  --namespace mcp-servers \
  --replicas 2 \
  --wait \
  --wait-timeout 5m
```

Expected output:
```
Verifying server 'database-server' exists in registry...
✓ Server 'database-server' found in registry
Creating MCPServer resource 'database-server' in namespace 'mcp-servers'...
✓ MCPServer 'database-server' created successfully
Waiting for MCPServer 'database-server' to be ready (timeout: 5m0s)...
⏳ MCPServer status: Pending - Fetching registry data
⏳ MCPServer status: Pending - Creating deployment
⏳ MCPServer status: Running - MCP server is running
✓ MCPServer 'database-server' is ready!
  Service endpoint: database-server.mcp-servers.svc.cluster.local:8080
```

### Dry Run

```bash
# Preview what would be created
mcp deploy web-scraper-server --dry-run
```

Expected output:
```
Verifying server 'web-scraper-server' exists in registry...
✓ Server 'web-scraper-server' found in registry

--- MCPServer Resource (dry-run) ---
apiVersion: mcp.allbeone.io/v1
kind: MCPServer
metadata:
  name: web-scraper-server
  namespace: default
  labels:
    app.kubernetes.io/name: mcp-server
    app.kubernetes.io/instance: web-scraper-server
    app.kubernetes.io/component: mcp-server
    app.kubernetes.io/created-by: mcp-cli
  annotations:
    mcp.allbeone.io/deployed-by: mcp-cli
    mcp.allbeone.io/deployed-at: 2024-01-15T12:30:45Z
spec:
  registry:
    name: web-scraper-server
  runtime:
    type: docker
  replicas: 1
```

## Step 4: Deploy Using YAML with Registry Integration

### Create a Registry-Integrated MCPServer

Create `mcpserver-registry.yaml`:

```yaml
apiVersion: mcp.allbeone.io/v1
kind: MCPServer
metadata:
  name: my-filesystem-server
  namespace: default
spec:
  registry:
    name: filesystem-server  # Server name from MCP Registry
  runtime:
    type: python  # Will be enriched from registry
  replicas: 2
  # Operator will automatically populate:
  # - registry.version, registry.description, etc.
  # - runtime.image, runtime.command, runtime.args
  # - runtime.env
```

Apply the configuration:

```bash
kubectl apply --server-side -f mcpserver-registry.yaml
```

## Step 5: Monitor and Verify Deployment

### Check MCPServer Status

```bash
# List all MCPServers
kubectl get mcpserver
```

Expected output:
```
NAME                   PHASE     REPLICAS   READY   AGE
filesystem-server      Running   1          1       5m
database-server        Running   2          2       3m
my-filesystem-server   Running   2          2       1m
```

### Check Registry Integration Status

```bash
# Check if registry data was fetched successfully
kubectl get mcpserver my-filesystem-server -o jsonpath='{.status.conditions[?(@.type=="RegistryFetched")].status}'
```

Expected output: `True`

```bash
# Get registry fetch message
kubectl get mcpserver my-filesystem-server -o jsonpath='{.status.conditions[?(@.type=="RegistryFetched")].message}'
```

Expected output: `Successfully fetched and applied server specification from registry`

### Detailed Status Information

```bash
# Get detailed information about the MCPServer
kubectl describe mcpserver my-filesystem-server
```

Expected output (partial):
```
Name:         my-filesystem-server
Namespace:    default
Labels:       <none>
Annotations:  <none>
API Version:  mcp.allbeone.io/v1
Kind:         MCPServer
Spec:
  Registry:
    Author:         MCP Team
    Capabilities:   filesystem read write
    Description:    A filesystem server for MCP
    Keywords:       filesystem file system
    License:        MIT
    Name:           filesystem-server
    Repository:     https://github.com/docker/mcp-registry
    Version:        1.0.0
  Runtime:
    Args:
      --port
      8080
    Command:
      python
      -m
      filesystem_server
    Env:
      MCP_PORT:  8080
    Image:       mcpregistry/filesystem-server:latest
    Type:        python
  Replicas:      2
Status:
  Available Replicas:  2
  Conditions:
    Last Transition Time:  2024-01-15T12:35:00Z
    Message:               Successfully fetched and applied server specification from registry
    Reason:                RegistryFetchSuccessful
    Status:                True
    Type:                  RegistryFetched
    Last Transition Time:  2024-01-15T12:35:30Z
    Message:               MCP server is running
    Reason:                DeploymentReady
    Status:                True
    Type:                  Ready
  Phase:                   Running
  Ready Replicas:          2
  Replicas:                2
  Service Endpoint:        my-filesystem-server.default.svc.cluster.local:8080
```

### Check Kubernetes Resources

```bash
# Check the created Deployment
kubectl get deployment my-filesystem-server

# Check the created Service
kubectl get service my-filesystem-server

# Check the pods
kubectl get pods -l app.kubernetes.io/instance=my-filesystem-server
```

## Step 6: Test Server Connectivity

### Port Forward to Test

```bash
# Port forward to test the MCP server
kubectl port-forward service/my-filesystem-server 8080:8080
```

In another terminal:
```bash
# Test the MCP server (example)
curl http://localhost:8080/health
```

## Step 7: Troubleshooting

### Common Issues and Solutions

#### Registry Fetch Failed

```bash
# Check registry condition
kubectl get mcpserver my-filesystem-server -o jsonpath='{.status.conditions[?(@.type=="RegistryFetched")]}'
```

If status is `False`, check:
1. Internet connectivity
2. GitHub API rate limits (set GITHUB_TOKEN environment variable)
3. Server name exists in registry

#### Deployment Not Ready

```bash
# Check pod logs
kubectl logs -l app.kubernetes.io/instance=my-filesystem-server

# Check events
kubectl get events --field-selector involvedObject.name=my-filesystem-server
```

#### CLI Issues

```bash
# Check CLI version
mcp version

# Get help
mcp --help
mcp deploy --help
```

## Step 8: Cleanup

```bash
# Delete MCPServers
kubectl delete mcpserver filesystem-server database-server my-filesystem-server

# Uninstall operator (if needed)
helm uninstall mcp-operator
```

## Summary

This scenario demonstrated:

1. ✅ **Easy Installation** - One-command Helm installation
2. ✅ **CLI Usage** - Simple commands to list and deploy servers
3. ✅ **Registry Integration** - Automatic enrichment from MCP Registry
4. ✅ **Status Tracking** - Detailed status conditions and monitoring
5. ✅ **Kubernetes Native** - Full integration with Kubernetes ecosystem
6. ✅ **Troubleshooting** - Clear debugging and monitoring capabilities

The enhanced MCP Operator provides a seamless experience for deploying and managing MCP servers in Kubernetes, with powerful registry integration that eliminates manual configuration while maintaining full transparency and control.