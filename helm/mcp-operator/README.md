# MCP Operator Helm Chart

This Helm chart deploys the MCP (Model Context Protocol) Operator on a Kubernetes cluster using the Helm package manager.

## Prerequisites

- Kubernetes 1.19+
- Helm 3.2.0+
- cert-manager (optional, for webhook certificates)
- Prometheus Operator (optional, for ServiceMonitor)

## Installing the Chart

### Option 1: Install in default namespace

To install the chart with the release name `mcp-operator` in the default namespace:

```bash
helm install mcp-operator ./helm/mcp-operator
```

### Option 2: Install in a specific namespace

To install the chart in a specific namespace, first create the namespace:

```bash
kubectl create namespace mcp-system
```

Then install the chart in the created namespace:

```bash
helm install mcp-operator ./helm/mcp-operator --namespace mcp-system --create-namespace
```

Alternatively, you can use the `--create-namespace` flag to automatically create the namespace during installation:

```bash
helm install mcp-operator ./helm/mcp-operator --namespace mcp-system --create-namespace
```

The command deploys the MCP Operator on the Kubernetes cluster in the default configuration. The [Parameters](#parameters) section lists the parameters that can be configured during installation.

## Uninstalling the Chart

To uninstall/delete the `mcp-operator` deployment:

```bash
helm delete mcp-operator
```

The command removes all the Kubernetes components associated with the chart and deletes the release.

## Parameters

### Global parameters

| Name                      | Description                                     | Value |
| ------------------------- | ----------------------------------------------- | ----- |
| `nameOverride`            | String to partially override mcp-operator.name | `""`  |
| `fullnameOverride`        | String to fully override mcp-operator.fullname | `""`  |
| `namespaceOverride`       | String to override the namespace                | `""`  |
| `commonLabels`            | Labels to add to all deployed objects          | `{}`  |
| `commonAnnotations`       | Annotations to add to all deployed objects     | `{}`  |

### Operator parameters

| Name                                    | Description                                | Value                    |
| --------------------------------------- | ------------------------------------------ | ------------------------ |
| `operator.image.repository`             | MCP Operator image repository              | `mcp-operator`           |
| `operator.image.tag`                    | MCP Operator image tag                     | `""`                     |
| `operator.image.pullPolicy`             | MCP Operator image pull policy             | `IfNotPresent`           |
| `operator.imagePullSecrets`             | MCP Operator image pull secrets            | `[]`                     |
| `operator.replicaCount`                 | Number of MCP Operator replicas            | `1`                      |
| `operator.serviceAccount.create`        | Specifies whether a ServiceAccount should be created | `true`         |
| `operator.serviceAccount.annotations`   | ServiceAccount annotations                 | `{}`                     |
| `operator.serviceAccount.name`          | The name of the ServiceAccount to use      | `""`                     |
| `operator.podAnnotations`               | Annotations for MCP Operator pods          | `{}`                     |
| `operator.podSecurityContext`           | Pod Security Context                       | See values.yaml          |
| `operator.securityContext`              | Container Security Context                 | See values.yaml          |
| `operator.resources`                    | Resource limits and requests               | See values.yaml          |
| `operator.nodeSelector`                 | Node labels for pod assignment             | `{}`                     |
| `operator.tolerations`                  | Tolerations for pod assignment             | `[]`                     |
| `operator.affinity`                     | Affinity for pod assignment                | `{}`                     |
| `operator.livenessProbe`                | Liveness probe configuration               | See values.yaml          |
| `operator.readinessProbe`               | Readiness probe configuration              | See values.yaml          |

### Metrics parameters

| Name                                    | Description                                | Value         |
| --------------------------------------- | ------------------------------------------ | ------------- |
| `metrics.enabled`                       | Enable metrics                             | `true`        |
| `metrics.service.type`                  | Metrics service type                       | `ClusterIP`   |
| `metrics.service.port`                  | Metrics service port                       | `8080`        |
| `metrics.service.annotations`           | Metrics service annotations                | `{}`          |
| `metrics.serviceMonitor.enabled`        | Enable ServiceMonitor for Prometheus      | `false`       |
| `metrics.serviceMonitor.namespace`      | ServiceMonitor namespace                   | `""`          |
| `metrics.serviceMonitor.labels`         | ServiceMonitor labels                      | `{}`          |
| `metrics.serviceMonitor.annotations`    | ServiceMonitor annotations                 | `{}`          |
| `metrics.serviceMonitor.interval`       | ServiceMonitor scrape interval             | `30s`         |
| `metrics.serviceMonitor.scrapeTimeout`  | ServiceMonitor scrape timeout              | `10s`         |

### Webhook parameters

| Name                                    | Description                                | Value         |
| --------------------------------------- | ------------------------------------------ | ------------- |
| `webhook.enabled`                       | Enable admission webhooks                  | `true`        |
| `webhook.service.type`                  | Webhook service type                       | `ClusterIP`   |
| `webhook.service.port`                  | Webhook service port                       | `9443`        |
| `webhook.certManager.enabled`           | Use cert-manager for webhook certificates  | `false`       |
| `webhook.certManager.issuerRef.name`    | cert-manager issuer name                   | `selfsigned-issuer` |
| `webhook.certManager.issuerRef.kind`    | cert-manager issuer kind                   | `ClusterIssuer` |

### RBAC parameters

| Name                                    | Description                                | Value         |
| --------------------------------------- | ------------------------------------------ | ------------- |
| `rbac.create`                           | Create RBAC resources                      | `true`        |
| `rbac.rules`                            | Additional RBAC rules                      | `[]`          |

### CRD parameters

| Name                                    | Description                                | Value         |
| --------------------------------------- | ------------------------------------------ | ------------- |
| `crds.install`                          | Install CRDs                               | `true`        |
| `crds.keep`                             | Keep CRDs on uninstall                     | `true`        |

### Pod Disruption Budget parameters

| Name                                    | Description                                | Value         |
| --------------------------------------- | ------------------------------------------ | ------------- |
| `podDisruptionBudget.enabled`           | Enable PodDisruptionBudget                 | `false`       |
| `podDisruptionBudget.minAvailable`      | Minimum available pods                     | `1`           |

### Network Policy parameters

| Name                                    | Description                                | Value         |
| --------------------------------------- | ------------------------------------------ | ------------- |
| `networkPolicy.enabled`                 | Enable NetworkPolicy                       | `false`       |
| `networkPolicy.ingress`                 | Additional ingress rules                   | `[]`          |
| `networkPolicy.egress`                  | Additional egress rules                    | `[]`          |

## Configuration and installation details

### Webhook Configuration

The MCP Operator includes admission webhooks for validating and mutating MCPServer resources. You can configure webhook certificates in two ways:

#### Using cert-manager (Recommended)

```yaml
webhook:
  enabled: true
  certManager:
    enabled: true
    issuerRef:
      name: selfsigned-issuer
      kind: ClusterIssuer
```

#### Using manual certificates

```yaml
webhook:
  enabled: true
  certManager:
    enabled: false
  caBundle: "LS0tLS1CRUdJTi..." # Base64 encoded CA bundle
```

### Metrics and Monitoring

Enable Prometheus metrics and ServiceMonitor:

```yaml
metrics:
  enabled: true
  serviceMonitor:
    enabled: true
    namespace: monitoring
    labels:
      release: prometheus
```

### High Availability

For production deployments, enable PodDisruptionBudget and configure multiple replicas:

```yaml
operator:
  replicaCount: 2

podDisruptionBudget:
  enabled: true
  minAvailable: 1
```

### Network Security

Enable NetworkPolicy for enhanced security:

```yaml
networkPolicy:
  enabled: true
  ingress:
    - from:
      - namespaceSelector:
          matchLabels:
            name: monitoring
      ports:
      - protocol: TCP
        port: 8080
```

## Examples

### Basic Installation

Default namespace:
```bash
helm install mcp-operator ./helm/mcp-operator
```

Specific namespace:
```bash
helm install mcp-operator ./helm/mcp-operator --namespace mcp-system --create-namespace
```

### Production Installation with Monitoring

Create namespace and install:
```bash
kubectl create namespace mcp-system
helm install mcp-operator ./helm/mcp-operator \
  --namespace mcp-system \
  --set operator.replicaCount=2 \
  --set metrics.serviceMonitor.enabled=true \
  --set webhook.certManager.enabled=true \
  --set podDisruptionBudget.enabled=true \
  --set networkPolicy.enabled=true
```

Or use auto-create namespace:
```bash
helm install mcp-operator ./helm/mcp-operator \
  --namespace mcp-system --create-namespace \
  --set operator.replicaCount=2 \
  --set metrics.serviceMonitor.enabled=true \
  --set webhook.certManager.enabled=true \
  --set podDisruptionBudget.enabled=true \
  --set networkPolicy.enabled=true
```

### Custom Values File

Create a `values-prod.yaml` file:

```yaml
operator:
  replicaCount: 2
  resources:
    limits:
      cpu: 1000m
      memory: 1Gi
    requests:
      cpu: 200m
      memory: 256Mi

metrics:
  serviceMonitor:
    enabled: true
    namespace: monitoring

webhook:
  certManager:
    enabled: true

podDisruptionBudget:
  enabled: true
  minAvailable: 1

networkPolicy:
  enabled: true
```

Install with custom values in specific namespace:

```bash
kubectl create namespace mcp-system
helm install mcp-operator ./helm/mcp-operator \
  --namespace mcp-system \
  -f values-prod.yaml
```

## Troubleshooting

### Common Issues

1. **Webhook certificate issues**: Ensure cert-manager is installed and configured properly
2. **RBAC permissions**: Verify that the ServiceAccount has the necessary permissions
3. **Image pull issues**: Check image repository and pull secrets configuration

### Debugging

Check operator logs:

```bash
kubectl logs -l app.kubernetes.io/name=mcp-operator -f
```

Check webhook configuration:

```bash
kubectl get validatingadmissionwebhooks
kubectl get mutatingadmissionwebhooks
```

Check metrics:

```bash
kubectl port-forward svc/mcp-operator-metrics 8080:8080
curl http://localhost:8080/metrics
```

## Contributing

Please read the main project README for contribution guidelines.

## License

This project is licensed under the MIT License - see the LICENSE file for details.