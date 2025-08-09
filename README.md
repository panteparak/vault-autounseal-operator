# Vault Auto-Unseal Operator

[![GitHub Release](https://img.shields.io/github/v/release/panteparak/vault-autounseal-operator?style=flat-square)](https://github.com/panteparak/vault-autounseal-operator/releases)
[![PyPI](https://img.shields.io/pypi/v/vault-autounseal-operator?style=flat-square)](https://pypi.org/project/vault-autounseal-operator/)
[![Docker Pulls](https://img.shields.io/docker/pulls/panteparak/vault-autounseal-operator?style=flat-square)](https://hub.docker.com/r/panteparak/vault-autounseal-operator)
[![CI](https://img.shields.io/github/actions/workflow/status/panteparak/vault-autounseal-operator/ci.yml?branch=main&style=flat-square&label=CI)](https://github.com/panteparak/vault-autounseal-operator/actions/workflows/ci.yml)
[![Security](https://img.shields.io/github/actions/workflow/status/panteparak/vault-autounseal-operator/security.yml?branch=main&style=flat-square&label=Security)](https://github.com/panteparak/vault-autounseal-operator/actions/workflows/security.yml)
[![codecov](https://img.shields.io/codecov/c/github/panteparak/vault-autounseal-operator?style=flat-square)](https://codecov.io/gh/panteparak/vault-autounseal-operator)

A production-ready, security-hardened Kubernetes operator that automatically unseals HashiCorp Vault instances, with support for HCP Vault and HA configurations.

## Features

- **Custom Resource Definition (CRD)**: Define multiple vault instances and their unseal configuration
- **Pod Watching**: Monitors Kubernetes pods for sealed Vault instances
- **HA Support**: Handles High Availability Vault setups with multiple sealed instances
- **Automatic Unsealing**: Uses provided unseal keys to automatically unseal sealed Vault instances
- **Multi-Instance Support**: Configure multiple Vault clusters in a single resource
- **Secure**: Follows Kubernetes security best practices

## Installation Methods

### 🚀 Option 1: One-line Install (Recommended)

```bash
curl -sSL https://raw.githubusercontent.com/panteparak/vault-autounseal-operator/main/install.sh | bash
```

### 🐳 Option 2: Container Images

```bash
# GitHub Container Registry (Recommended)
docker pull ghcr.io/panteparak/vault-autounseal-operator:latest

# Docker Hub
docker pull panteparak/vault-autounseal-operator:latest

# Quay.io
docker pull quay.io/panteparak/vault-autounseal-operator:latest
```

### ⎈ Option 3: Manual Kubernetes Deployment

```bash
# Apply manifests directly
kubectl apply -f https://github.com/panteparak/vault-autounseal-operator/releases/latest/download/crd.yaml
kubectl apply -f https://github.com/panteparak/vault-autounseal-operator/releases/latest/download/rbac.yaml
kubectl apply -f https://github.com/panteparak/vault-autounseal-operator/releases/latest/download/deployment.yaml
```

### 📦 Option 4: Local Development

```bash
# For development/testing only
git clone https://github.com/panteparak/vault-autounseal-operator.git
cd vault-autounseal-operator
uv pip install -e .
vault-operator --help
```

## Quick Start

### 2. Generate and Deploy CRD

The CRD is defined in Python code and can be auto-generated:

```bash
# Generate CRD YAML file
vault-operator generate-crd -o manifests/crd.yaml

# Or install directly to cluster
vault-operator install-crd

# Using uv run (if not installed globally)
uv run vault-operator generate-crd -o manifests/crd.yaml
uv run vault-operator install-crd
```

### 3. Deploy the Operator

```bash
# Deploy everything
make deploy

# Or manually
kubectl apply -f manifests/crd.yaml
kubectl apply -f manifests/rbac.yaml  
kubectl apply -f manifests/deployment.yaml
```

### 4. Build Container Image

```bash
# Build and push image
make docker-build
make docker-push

# Or manually
docker build -t vault-autounseal-operator:latest .
docker push your-registry/vault-autounseal-operator:latest
```

### 5. Create a VaultUnsealConfig

#### Simple Configuration (Direct Keys)

```yaml
apiVersion: vault.io/v1
kind: VaultUnsealConfig
metadata:
  name: simple-vault
  namespace: default
spec:
  url: "https://vault.example.com:8200"
  unsealKeys:
    secret:
    - "dGVzdC11bnNlYWwta2V5LTE="  # base64 encoded unseal key 1
    - "dGVzdC11bnNlYWwta2V5LTI="  # base64 encoded unseal key 2  
    - "dGVzdC11bnNlYWwta2V5LTM="  # base64 encoded unseal key 3
  threshold: 3
  tlsSkipVerify: false
```

#### Advanced Configuration (Secret Reference)

```yaml
apiVersion: vault.io/v1
kind: VaultUnsealConfig
metadata:
  name: vault-with-secret-ref
  namespace: vault
spec:
  url: "https://vault.example.com:8200"
  unsealKeys:
    secretRef:
      name: vault-unseal-keys
      namespace: vault
      key: keys
  namespace: vault
  podSelector:
    matchLabels:
      app: vault
      component: server
  threshold: 3
  haEnabled: true
  tlsSkipVerify: false
```

## Configuration

### VaultUnsealConfig Spec

- `url`: Vault API endpoint URL (required)
- `unsealKeys`: Unseal keys configuration (required, choose one):
  - `secret`: Array of base64 encoded unseal keys
  - `secretRef`: Reference to Kubernetes secret containing keys
    - `name`: Secret name (required)
    - `namespace`: Secret namespace (defaults to resource namespace)
    - `key`: Key within secret (default: "unseal-keys")
- `namespace`: Kubernetes namespace to monitor for pods (default: "default")
- `podSelector`: Label selector to identify vault pods (for HA mode)
- `threshold`: Number of keys required to unseal (default: 3)
- `haEnabled`: Enable HA mode pod watching (default: false)
- `tlsSkipVerify`: Skip TLS verification (default: false)
- `reconcileInterval`: How often to check vault status (default: "30s")

## Development

### Setup

```bash
# Install dependencies with uv
uv pip install -e .

# Install development dependencies
uv pip install -e ".[dev]"
```

### Running Locally

```bash
# Set up kubeconfig for local cluster access
export KUBECONFIG=~/.kube/config

# Run the operator
python -m vault_autounseal_operator.main
```

### Code Quality

```bash
# Format code
black src/

# Lint code  
ruff src/

# Run tests
pytest
```

## Architecture

The operator consists of several components:

- **VaultClient**: Handles Vault API interactions for checking seal status and unsealing
- **PodWatcher**: Monitors Kubernetes pods matching selectors for sealed Vault instances  
- **Operator**: Main controller using Kopf framework for CRD lifecycle management

## Security Considerations

- Store unseal keys securely (consider using Kubernetes Secrets)
- Use TLS verification in production (`tlsSkipVerify: false`)
- Follow principle of least privilege for RBAC permissions
- Monitor operator logs for security events

## Troubleshooting

### Common Issues

1. **Operator can't connect to Vault**: Check endpoint URLs and network connectivity
2. **Unseal keys rejected**: Verify keys are base64 encoded and valid for the Vault instance
3. **Pods not detected**: Check pod selectors match your Vault pod labels
4. **Permission errors**: Verify RBAC configuration is applied correctly

### Logging

The operator provides structured logging. Key log messages include:
- Vault unseal attempts and results
- Pod discovery and monitoring events  
- Configuration changes and errors

## License

MIT License