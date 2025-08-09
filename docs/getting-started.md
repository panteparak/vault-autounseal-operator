# Getting Started with Vault Auto-Unseal Operator

This guide will help you get started with the Vault Auto-Unseal Operator, a production-ready Kubernetes operator for automatically unsealing HashiCorp Vault instances.

## Table of Contents

- [Prerequisites](#prerequisites)
- [Installation](#installation)
- [Configuration](#configuration)
- [Basic Usage](#basic-usage)
- [Advanced Configuration](#advanced-configuration)
- [Monitoring](#monitoring)
- [Troubleshooting](#troubleshooting)

## Prerequisites

Before installing the operator, ensure you have:

- **Kubernetes cluster** (v1.25+) with admin access
- **Helm** (v3.8+) installed
- **kubectl** configured to access your cluster
- **HashiCorp Vault** instance(s) deployed and initialized
- **Unseal keys** for your Vault instances

### Vault Requirements

- Vault must be initialized but sealed
- Network access from Kubernetes cluster to Vault instances
- (Optional) TLS certificates if using HTTPS

## Installation

### Option 1: Using Helm (Recommended)

1. **Add the Helm repository** (if publishing to a Helm repo):
   ```bash
   helm repo add vault-operator https://panteparak.github.io/vault-autounseal-operator
   helm repo update
   ```

2. **Create a namespace**:
   ```bash
   kubectl create namespace vault-system
   ```

3. **Install the operator**:
   ```bash
   helm install vault-autounseal-operator vault-operator/vault-autounseal-operator \
     --namespace vault-system \
     --set image.repository=ghcr.io/panteparak/vault-autounseal-operator \
     --set image.tag=latest
   ```

### Option 2: From Local Chart

1. **Clone the repository**:
   ```bash
   git clone https://github.com/panteparak/vault-autounseal-operator.git
   cd vault-autounseal-operator
   ```

2. **Create a namespace**:
   ```bash
   kubectl create namespace vault-system
   ```

3. **Install using local chart**:
   ```bash
   helm install vault-autounseal-operator ./helm/vault-autounseal-operator/ \
     --namespace vault-system \
     --set image.repository=ghcr.io/panteparak/vault-autounseal-operator \
     --set image.tag=latest
   ```

### Option 3: Using kubectl (Manual)

1. **Apply CRDs**:
   ```bash
   kubectl apply -f helm/vault-autounseal-operator/templates/crd.yaml
   ```

2. **Apply RBAC**:
   ```bash
   kubectl apply -f helm/vault-autounseal-operator/templates/rbac.yaml
   ```

3. **Apply Deployment**:
   ```bash
   kubectl apply -f helm/vault-autounseal-operator/templates/deployment.yaml
   ```

## Configuration

### Basic Configuration

Create a `VaultUnsealConfig` resource to manage your Vault instances:

```yaml
apiVersion: vault.io/v1
kind: VaultUnsealConfig
metadata:
  name: my-vault-config
  namespace: vault-system
spec:
  vaultInstances:
  - name: vault-primary
    endpoint: https://vault.example.com:8200
    unsealKeys:
    - "base64-encoded-key-1"
    - "base64-encoded-key-2" 
    - "base64-encoded-key-3"
    threshold: 3
    tlsSkipVerify: false
```

### Secure Key Management

**⚠️ Important**: Never store unseal keys in plain text. Use Kubernetes secrets:

1. **Create a secret with your unseal keys**:
   ```bash
   kubectl create secret generic vault-unseal-keys \
     --from-literal=key1="your-base64-encoded-key-1" \
     --from-literal=key2="your-base64-encoded-key-2" \
     --from-literal=key3="your-base64-encoded-key-3" \
     --namespace vault-system
   ```

2. **Reference the secret in your configuration**:
   ```yaml
   apiVersion: vault.io/v1
   kind: VaultUnsealConfig
   metadata:
     name: my-vault-config
     namespace: vault-system
   spec:
     vaultInstances:
     - name: vault-primary
       endpoint: https://vault.example.com:8200
       unsealKeysFromSecret:
         name: vault-unseal-keys
         keys: ["key1", "key2", "key3"]
       threshold: 3
       tlsSkipVerify: false
   ```

## Basic Usage

### Deploy Configuration

Apply your configuration:

```bash
kubectl apply -f vault-config.yaml
```

### Check Status

Monitor the operator and your Vault instances:

```bash
# Check operator logs
kubectl logs -n vault-system deployment/vault-autounseal-operator

# Check VaultUnsealConfig status
kubectl get vaultunsealconfigs -n vault-system
kubectl describe vaultunsealconfig my-vault-config -n vault-system

# View detailed status
kubectl get vaultunsealconfig my-vault-config -n vault-system -o yaml
```

### Verify Unsealing

The operator will:
1. Check if Vault is sealed every 30 seconds
2. Automatically unseal using the provided keys
3. Update the status with unsealing results
4. Log all operations for monitoring

## Advanced Configuration

### High Availability (HA) Setup

For HA Vault deployments, enable pod monitoring:

```yaml
apiVersion: vault.io/v1
kind: VaultUnsealConfig
metadata:
  name: vault-ha-config
  namespace: vault-system
spec:
  vaultInstances:
  - name: vault-cluster
    endpoint: https://vault.example.com:8200
    unsealKeys:
    - "base64-key-1"
    - "base64-key-2"
    - "base64-key-3"
    threshold: 3
    haEnabled: true
    podSelector:
      app: vault
      component: server
    namespace: vault-namespace
```

### Multiple Vault Instances

Manage multiple Vault instances in one configuration:

```yaml
apiVersion: vault.io/v1
kind: VaultUnsealConfig
metadata:
  name: multi-vault-config
  namespace: vault-system
spec:
  vaultInstances:
  - name: vault-primary
    endpoint: https://vault-primary.example.com:8200
    unsealKeys: ["key1", "key2", "key3"]
    threshold: 3
  - name: vault-secondary
    endpoint: https://vault-secondary.example.com:8200
    unsealKeys: ["key4", "key5", "key6"]
    threshold: 3
    tlsSkipVerify: true  # Only for development
```

### Custom Helm Values

Customize the operator deployment:

```yaml
# values.yaml
image:
  repository: ghcr.io/panteparak/vault-autounseal-operator
  tag: "v1.0.0"

resources:
  requests:
    memory: "128Mi"
    cpu: "100m"
  limits:
    memory: "256Mi"
    cpu: "500m"

monitoring:
  serviceMonitor:
    enabled: true
    labels:
      prometheus: kube-prometheus

nodeSelector:
  kubernetes.io/os: linux

tolerations:
- key: "vault"
  operator: "Equal"
  value: "true"
  effect: "NoSchedule"
```

Install with custom values:

```bash
helm install vault-autounseal-operator ./helm/vault-autounseal-operator/ \
  --namespace vault-system \
  --values values.yaml
```

## Monitoring

### Prometheus Metrics

The operator exposes metrics on `:8080/metrics`:

- `vault_unseal_attempts_total` - Total unseal attempts
- `vault_unseal_successes_total` - Successful unseals
- `vault_unseal_failures_total` - Failed unseal attempts
- `vault_instances_sealed` - Number of sealed instances

### Enable ServiceMonitor

```yaml
monitoring:
  serviceMonitor:
    enabled: true
    labels:
      prometheus: kube-prometheus
    interval: 30s
```

### Grafana Dashboard

Import the provided Grafana dashboard from `examples/grafana-dashboard.json`.

### Health Checks

The operator provides health endpoints:

- `:8081/healthz` - Liveness probe
- `:8081/readyz` - Readiness probe

## Troubleshooting

### Common Issues

1. **Operator not starting**:
   ```bash
   kubectl logs -n vault-system deployment/vault-autounseal-operator
   ```

2. **CRD not found**:
   ```bash
   kubectl get crd vaultunsealconfigs.vault.io
   ```

3. **RBAC issues**:
   ```bash
   kubectl auth can-i get pods --as=system:serviceaccount:vault-system:vault-autounseal-operator
   ```

4. **Network connectivity**:
   ```bash
   kubectl run debug --rm -it --image=curlimages/curl -- curl -k https://vault.example.com:8200/v1/sys/health
   ```

### Debug Mode

Enable debug logging:

```yaml
operator:
  logLevel: debug
```

### Validation

Check your configuration:

```bash
# Validate YAML syntax
kubectl apply --dry-run=client -f vault-config.yaml

# Check operator events
kubectl get events -n vault-system --sort-by='.lastTimestamp'
```

### Support

For issues and support:

1. Check the [troubleshooting guide](troubleshooting.md)
2. Review [GitHub issues](https://github.com/panteparak/vault-autounseal-operator/issues)
3. Create a new issue with:
   - Operator version
   - Kubernetes version
   - Configuration YAML
   - Operator logs
   - Error messages

## Next Steps

- Learn about [security best practices](security.md)
- Explore [advanced configuration](advanced-config.md)
- Set up [monitoring and alerting](monitoring.md)
- Review [production deployment guide](production.md)