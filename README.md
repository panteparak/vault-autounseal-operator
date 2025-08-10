# Vault Auto-Unseal Operator

[![CI Pipeline](https://github.com/panteparak/vault-autounseal-operator/actions/workflows/ci.yaml/badge.svg)](https://github.com/panteparak/vault-autounseal-operator/actions/workflows/ci.yaml)
[![Go Report Card](https://goreportcard.com/badge/github.com/panteparak/vault-autounseal-operator)](https://goreportcard.com/report/github.com/panteparak/vault-autounseal-operator)
[![License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)
[![Go Version](https://img.shields.io/badge/go-1.21+-blue.svg)](https://golang.org/dl/)
[![Docker Pulls](https://img.shields.io/docker/pulls/ghcr.io/panteparak/vault-autounseal-operator)](https://github.com/panteparak/vault-autounseal-operator/pkgs/container/vault-autounseal-operator)

A **production-ready Kubernetes operator** for automatically unsealing HashiCorp Vault instances. Built with Go and controller-runtime for high performance, security, and reliability.

## 🚀 Features

- **🔐 Automatic Unsealing**: Continuously monitors and unseals Vault instances with configurable reconciliation
- **🏗️ High Availability**: Full support for HA Vault clusters with intelligent pod monitoring
- **🛡️ Security First**: Secure key handling, comprehensive TLS support, input validation, and audit logging
- **📊 Production Ready**: Built-in monitoring, Prometheus metrics, health checks, and observability
- **⚡ High Performance**: Efficient Go implementation with minimal resource footprint
- **🔄 Complete CI/CD**: Automated testing, building, packaging, and releasing
- **📚 Comprehensive Docs**: Detailed documentation with real-world examples

## 📋 Quick Start

### Prerequisites

- **Kubernetes**: v1.25+ with admin access
- **Helm**: v3.8+ installed
- **Vault**: Initialized HashiCorp Vault instance(s)

### 🏃‍♂️ Installation (60 seconds)

1. **Install the operator**:
   ```bash
   helm install vault-autounseal-operator \
     oci://ghcr.io/panteparak/vault-autounseal-operator \
     --namespace vault-system --create-namespace
   ```

2. **Create configuration**:
   ```bash
   cat <<EOF | kubectl apply -f -
   apiVersion: vault.io/v1
   kind: VaultUnsealConfig
   metadata:
     name: my-vault
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
   EOF
   ```

3. **Verify it's working**:
   ```bash
   kubectl get vaultunsealconfigs -n vault-system
   kubectl logs -n vault-system deployment/vault-autounseal-operator
   ```

## 🏗️ Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    Vault Auto-Unseal Operator                   │
│                                                                 │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐             │
│  │ Controller  │  │ Vault Client│  │ Pod Watcher │             │
│  │             │  │             │  │             │             │
│  │ • Reconcile │  │ • TLS/mTLS  │  │ • HA Support│             │
│  │ • Status    │  │ • Security  │  │ • Pod Events│             │
│  │ • Events    │  │ • Unsealing │  │ • Monitoring│             │
│  └─────────────┘  └─────────────┘  └─────────────┘             │
│                                                                 │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐             │
│  │ Metrics     │  │ Health      │  │ Logging     │             │
│  │ Prometheus  │  │ Liveness    │  │ Structured  │             │
│  └─────────────┘  └─────────────┘  └─────────────┘             │
└─────────────────────────────────────────────────────────────────┘
                              │
                              ▼
         ┌─────────────────────────────────────────┐
         │              Vault Instances            │
         │                                         │
         │  ┌─────────┐ ┌─────────┐ ┌─────────┐    │
         │  │Vault #1 │ │Vault #2 │ │Vault #N │    │
         │  └─────────┘ └─────────┘ └─────────┘    │
         └─────────────────────────────────────────┘
```

## 📖 Configuration Examples

<details>
<summary><b>💡 Single Vault Instance</b></summary>

```yaml
apiVersion: vault.io/v1
kind: VaultUnsealConfig
metadata:
  name: simple-vault
  namespace: vault-system
spec:
  vaultInstances:
  - name: vault
    endpoint: https://vault.company.com:8200
    unsealKeys:
    - "dGVzdC1rZXktMQ=="  # base64 encoded
    - "dGVzdC1rZXktMg=="
    - "dGVzdC1rZXktMw=="
    threshold: 3
```
</details>

<details>
<summary><b>🏗️ High Availability Cluster</b></summary>

```yaml
apiVersion: vault.io/v1
kind: VaultUnsealConfig
metadata:
  name: vault-ha-cluster
  namespace: vault-system
spec:
  vaultInstances:
  - name: vault-cluster
    endpoint: https://vault-active.vault.svc.cluster.local:8200
    unsealKeys: ["key1", "key2", "key3", "key4", "key5"]
    threshold: 3
    haEnabled: true
    podSelector:
      app.kubernetes.io/name: vault
    namespace: vault
```
</details>

<details>
<summary><b>🌐 Multiple Environments</b></summary>

```yaml
apiVersion: vault.io/v1
kind: VaultUnsealConfig
metadata:
  name: multi-env-vault
  namespace: vault-system
spec:
  vaultInstances:
  - name: vault-prod
    endpoint: https://vault-prod.company.com:8200
    unsealKeys: ["prod-key-1", "prod-key-2", "prod-key-3"]
    threshold: 3
  - name: vault-staging
    endpoint: https://vault-staging.company.com:8200
    unsealKeys: ["staging-key-1", "staging-key-2"]
    threshold: 2
    tlsSkipVerify: true
```
</details>

<details>
<summary><b>🔐 Using Kubernetes Secrets (Recommended)</b></summary>

```bash
# Create secret with unseal keys
kubectl create secret generic vault-keys \
  --from-literal=key1="$(echo -n 'unseal-key-1' | base64)" \
  --from-literal=key2="$(echo -n 'unseal-key-2' | base64)" \
  --from-literal=key3="$(echo -n 'unseal-key-3' | base64)" \
  --namespace vault-system
```

```yaml
apiVersion: vault.io/v1
kind: VaultUnsealConfig
metadata:
  name: secure-vault
  namespace: vault-system
spec:
  vaultInstances:
  - name: vault-secure
    endpoint: https://vault.company.com:8200
    unsealKeysRef:
      secretName: vault-keys
      keys: ["key1", "key2", "key3"]
    threshold: 3
```
</details>

## 📊 Monitoring & Observability

### Prometheus Metrics
The operator exposes comprehensive metrics on `:8080/metrics`:

| Metric | Description |
|--------|-------------|
| `vault_unseal_attempts_total` | Total unseal attempts |
| `vault_unseal_successes_total` | Successful unseals |
| `vault_unseal_failures_total` | Failed unseal attempts |
| `vault_instances_sealed` | Currently sealed instances |
| `vault_reconcile_duration_seconds` | Reconciliation duration |

### Health Checks
- **Liveness**: `:8081/healthz` - Operator health
- **Readiness**: `:8081/readyz` - Ready to serve requests

### Grafana Dashboard
Import our pre-built dashboard from `examples/grafana-dashboard.json`.

## 🛡️ Security

Security is our **top priority**:

- ✅ **Secure Key Storage**: Kubernetes secrets integration
- ✅ **Input Validation**: Comprehensive config validation
- ✅ **TLS Support**: Full certificate verification
- ✅ **Non-root Execution**: Runs as UID 65532
- ✅ **Read-only Filesystem**: Immutable container filesystem
- ✅ **Audit Logging**: Complete operation audit trail
- ✅ **Minimal RBAC**: Least-privilege permissions
- ✅ **Security Scanning**: Automated vulnerability detection

### 🔒 Security Best Practices

1. **Never store unseal keys in plain YAML**
2. **Always use Kubernetes secrets**
3. **Enable TLS verification in production**
4. **Monitor all operator activities**
5. **Use network policies to restrict access**
6. **Regularly rotate unseal keys**

## 🛠️ Development

### Local Development Setup

```bash
# Clone and setup
git clone https://github.com/panteparak/vault-autounseal-operator.git
cd vault-autounseal-operator

# Install dependencies
go mod download

# Run tests
make test

# Build binary
make build

# Run locally (requires kubeconfig)
./bin/manager --metrics-bind-address=:8080 --health-probe-bind-address=:8081
```

### 🧪 Testing

```bash
# Unit tests
make test

# Integration tests
make test-integration

# Security scan
make security-scan

# Coverage report
make test-coverage
```

## 🚀 CI/CD Pipeline

Our automated pipeline handles:

- **🧹 Code Quality**: `gofmt`, `goimports`, `go vet`, `staticcheck`
- **🔒 Security**: `gosec`, `trivy`, vulnerability scanning
- **🧪 Testing**: Unit tests, integration tests, race detection
- **🏗️ Building**: Multi-arch Docker images (amd64/arm64)
- **📦 Packaging**: Automated Helm chart packaging with CRDs
- **🚢 Releases**: Semantic versioning with conventional commits
- **🏷️ Tagging**: Auto-generated tags and changelogs

## 🔄 Release Process

Releases are **fully automated** using semantic versioning:

### Commit Types → Release Types
- `feat:` → **Minor** release (0.1.0 → 0.2.0)
- `fix:` → **Patch** release (0.1.0 → 0.1.1)
- `feat!:` or `BREAKING CHANGE:` → **Major** release (0.1.0 → 1.0.0)

### Automated Release Features
- 🏷️ **Auto-versioning** based on conventional commits
- 📝 **Generated changelogs** with categorized changes
- 🐳 **Tagged Docker images** (`latest` + version tags)
- 📦 **Helm chart packaging** with embedded CRDs
- 🔒 **Security scanning** of release artifacts
- 🚀 **GitHub releases** with installation scripts

### Making a Release
Simply push commits with conventional commit messages to `main`:

```bash
# Feature release (0.1.0 → 0.2.0)
git commit -m "feat: add new unsealing strategy for HA clusters"

# Bug fix release (0.1.0 → 0.1.1)
git commit -m "fix: resolve memory leak in vault client"

# Breaking change release (0.1.0 → 1.0.0)
git commit -m "feat!: redesign API for better performance"

# Push to trigger release
git push origin main
```

## 📚 Documentation

| Document | Description |
|----------|-------------|
| [Getting Started](docs/getting-started.md) | Complete setup and configuration guide |
| [Configuration Examples](docs/examples.md) | Real-world configuration examples |
| [Security Guide](docs/security.md) | Security best practices |
| [Monitoring Guide](docs/monitoring.md) | Observability and alerting |
| [Troubleshooting](docs/troubleshooting.md) | Common issues and solutions |
| [API Reference](docs/api-reference.md) | Complete API documentation |

## ❓ FAQ

<details>
<summary><b>How does the operator handle HA Vault clusters?</b></summary>

The operator monitors individual pods in HA clusters using label selectors and automatically unseals new pods as they start, ensuring seamless failover.
</details>

<details>
<summary><b>What happens if unseal keys are incorrect?</b></summary>

The operator logs detailed error messages and continues attempting according to the reconciliation schedule. Status is updated in the VaultUnsealConfig resource.
</details>

<details>
<summary><b>Can I manage Vault instances across namespaces?</b></summary>

Yes! The operator supports cross-namespace monitoring with proper RBAC configuration.
</details>

<details>
<summary><b>Is this production-ready?</b></summary>

Absolutely. The operator includes comprehensive security, monitoring, error handling, and has been designed following production best practices.
</details>

<details>
<summary><b>How do I migrate from the Python version?</b></summary>

The Go version is a complete rewrite with the same API. Simply update your Helm deployment and your existing VaultUnsealConfig resources will work unchanged.
</details>

## 🤝 Contributing

We welcome contributions! Please see our [Contributing Guide](CONTRIBUTING.md).

### Development Workflow
1. **Fork** the repository
2. **Create** a feature branch
3. **Add** tests for new functionality
4. **Run** `make test` and ensure everything passes
5. **Submit** a pull request

## 📄 License

This project is licensed under the **MIT License** - see the [LICENSE](LICENSE) file for details.

## 🙏 Acknowledgments

- [HashiCorp](https://www.hashicorp.com/) for creating Vault
- [Kubernetes SIG](https://github.com/kubernetes-sigs/controller-runtime) for controller-runtime
- The amazing Go community for excellent tooling and libraries

---

<div align="center">

**⭐ If this project helps you, please give it a star! ⭐**

[Report Bug](https://github.com/panteparak/vault-autounseal-operator/issues) ·
[Request Feature](https://github.com/panteparak/vault-autounseal-operator/issues) ·
[Documentation](docs/) ·
[Discussions](https://github.com/panteparak/vault-autounseal-operator/discussions)

</div>
