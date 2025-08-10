# Configuration Examples

This document provides practical examples for configuring the Vault Auto-Unseal Operator in various scenarios.

## Basic Single Instance

The simplest configuration for a single Vault instance:

```yaml
apiVersion: vault.io/v1
kind: VaultUnsealConfig
metadata:
  name: single-vault
  namespace: vault-system
spec:
  vaultInstances:
  - name: vault-primary
    endpoint: http://vault:8200
    unsealKeys:
    - "dGVzdC1rZXktMQ=="  # base64: test-key-1
    - "dGVzdC1rZXktMg=="  # base64: test-key-2
    - "dGVzdC1rZXktMw=="  # base64: test-key-3
    threshold: 3
```

## Production HTTPS with TLS Verification

For production deployments with proper TLS:

```yaml
apiVersion: vault.io/v1
kind: VaultUnsealConfig
metadata:
  name: production-vault
  namespace: vault-system
spec:
  vaultInstances:
  - name: vault-prod
    endpoint: https://vault.company.com:8200
    unsealKeys:
    - "YWN0dWFsLXVuc2VhbC1rZXktMQ=="
    - "YWN0dWFsLXVuc2VhbC1rZXktMg=="
    - "YWN0dWFsLXVuc2VhbC1rZXktMw=="
    - "YWN0dWFsLXVuc2VhbC1rZXktNA=="
    - "YWN0dWFsLXVuc2VhbC1rZXktNQ=="
    threshold: 3  # Only need 3 out of 5 keys
    tlsSkipVerify: false  # Verify TLS certificates (default)
```

## Development with Self-Signed Certificates

For development environments with self-signed certificates:

```yaml
apiVersion: vault.io/v1
kind: VaultUnsealConfig
metadata:
  name: dev-vault
  namespace: vault-system
spec:
  vaultInstances:
  - name: vault-dev
    endpoint: https://vault.dev.local:8200
    unsealKeys:
    - "ZGV2LWtleS0x"
    - "ZGV2LWtleS0y"
    - "ZGV2LWtleS0z"
    threshold: 2
    tlsSkipVerify: true  # Skip TLS verification for dev
```

## High Availability Vault Cluster

For HA Vault deployments with pod monitoring:

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
    unsealKeys:
    - "aGEta2V5LTE="
    - "aGEta2V5LTI="
    - "aGEta2V5LTM="
    - "aGEta2V5LTQ="
    - "aGEta2V5LTU="
    threshold: 3
    haEnabled: true
    podSelector:
      app.kubernetes.io/name: vault
      app.kubernetes.io/component: server
    namespace: vault  # Monitor pods in vault namespace
```

## Multiple Vault Instances

Managing multiple independent Vault instances:

```yaml
apiVersion: vault.io/v1
kind: VaultUnsealConfig
metadata:
  name: multi-vault-setup
  namespace: vault-system
spec:
  vaultInstances:
  # Production Vault
  - name: vault-production
    endpoint: https://vault-prod.company.com:8200
    unsealKeys:
    - "cHJvZC1rZXktMQ=="
    - "cHJvZC1rZXktMg=="
    - "cHJvZC1rZXktMw=="
    threshold: 3

  # Staging Vault
  - name: vault-staging
    endpoint: https://vault-staging.company.com:8200
    unsealKeys:
    - "c3RhZ2luZy1rZXktMQ=="
    - "c3RhZ2luZy1rZXktMg=="
    - "c3RhZ2luZy1rZXktMw=="
    threshold: 2

  # Development Vault (less secure)
  - name: vault-development
    endpoint: http://vault-dev.company.com:8200
    unsealKeys:
    - "ZGV2LWtleS0x"
    - "ZGV2LWtleS0y"
    - "ZGV2LWtleS0z"
    threshold: 2
    tlsSkipVerify: true
```

## Cross-Namespace Monitoring

Monitoring Vault pods in a different namespace:

```yaml
apiVersion: vault.io/v1
kind: VaultUnsealConfig
metadata:
  name: cross-namespace-vault
  namespace: vault-system
spec:
  vaultInstances:
  - name: app-vault
    endpoint: https://vault.apps.svc.cluster.local:8200
    unsealKeys:
    - "YXBwLWtleS0x"
    - "YXBwLWtleS0y"
    - "YXBwLWtleS0z"
    threshold: 2
    haEnabled: true
    podSelector:
      app: vault
      release: vault-app
    namespace: applications  # Monitor pods in applications namespace
```

## Using Kubernetes Secrets for Keys

**Recommended approach** for production:

1. Create the secret:
   ```bash
   kubectl create secret generic vault-keys \
     --from-literal=key1="$(echo -n 'actual-unseal-key-1' | base64)" \
     --from-literal=key2="$(echo -n 'actual-unseal-key-2' | base64)" \
     --from-literal=key3="$(echo -n 'actual-unseal-key-3' | base64)" \
     --namespace vault-system
   ```

2. Reference in VaultUnsealConfig:
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

## External Vault with Custom Port

Accessing Vault on a non-standard port:

```yaml
apiVersion: vault.io/v1
kind: VaultUnsealConfig
metadata:
  name: custom-port-vault
  namespace: vault-system
spec:
  vaultInstances:
  - name: vault-custom
    endpoint: https://vault.external.com:9200
    unsealKeys:
    - "Y3VzdG9tLWtleS0x"
    - "Y3VzdG9tLWtleS0y"
    - "Y3VzdG9tLWtleS0z"
    threshold: 2
```

## Vault with Load Balancer

When Vault is behind a load balancer:

```yaml
apiVersion: vault.io/v1
kind: VaultUnsealConfig
metadata:
  name: lb-vault
  namespace: vault-system
spec:
  vaultInstances:
  - name: vault-cluster
    endpoint: https://vault-lb.company.com:443
    unsealKeys:
    - "bGItdmF1bHQta2V5LTE="
    - "bGItdmF1bHQta2V5LTI="
    - "bGItdmF1bHQta2V5LTM="
    threshold: 3
    # Note: HA monitoring might not work well with load balancers
    # as you can't predict which pod you'll hit
```

## Vault in Different Kubernetes Cluster

Accessing Vault running in a different Kubernetes cluster:

```yaml
apiVersion: vault.io/v1
kind: VaultUnsealConfig
metadata:
  name: external-k8s-vault
  namespace: vault-system
spec:
  vaultInstances:
  - name: remote-vault
    endpoint: https://vault.cluster2.company.com:8200
    unsealKeys:
    - "cmVtb3RlLWtleS0x"
    - "cmVtb3RlLWtleS0y"
    - "cmVtb3RlLWtleS0z"
    threshold: 2
    # HA monitoring won't work across clusters
    haEnabled: false
```

## Minimal Configuration

The absolute minimum required configuration:

```yaml
apiVersion: vault.io/v1
kind: VaultUnsealConfig
metadata:
  name: minimal-vault
  namespace: vault-system
spec:
  vaultInstances:
  - name: vault
    endpoint: http://vault:8200
    unsealKeys: ["dGVzdA=="]
    # threshold defaults to 3, but we only have 1 key
    threshold: 1
```

## Testing Configuration

For testing and development:

```yaml
apiVersion: vault.io/v1
kind: VaultUnsealConfig
metadata:
  name: test-vault
  namespace: default
spec:
  vaultInstances:
  - name: vault-test
    endpoint: http://localhost:8200
    unsealKeys:
    - "dGVzdC1rZXktMQ=="  # test-key-1
    - "dGVzdC1rZXktMg=="  # test-key-2
    - "dGVzdC1rZXktMw=="  # test-key-3
    threshold: 1  # For testing, only require 1 key
    tlsSkipVerify: true
```

## Regional Deployment

Managing Vault instances across regions:

```yaml
apiVersion: vault.io/v1
kind: VaultUnsealConfig
metadata:
  name: multi-region-vault
  namespace: vault-system
spec:
  vaultInstances:
  # US East
  - name: vault-us-east
    endpoint: https://vault-us-east.company.com:8200
    unsealKeys: ["dXMtZWFzdC1rZXktMQ==", "dXMtZWFzdC1rZXktMg==", "dXMtZWFzdC1rZXktMw=="]
    threshold: 2

  # US West
  - name: vault-us-west
    endpoint: https://vault-us-west.company.com:8200
    unsealKeys: ["dXMtd2VzdC1rZXktMQ==", "dXMtd2VzdC1rZXktMg==", "dXMtd2VzdC1rZXktMw=="]
    threshold: 2

  # Europe
  - name: vault-eu
    endpoint: https://vault-eu.company.com:8200
    unsealKeys: ["ZXUtcmVnaW9uLWtleS0x", "ZXUtcmVnaW9uLWtleS0y", "ZXUtcmVnaW9uLWtleS0z"]
    threshold: 2
```

## Notes

- Always use properly base64-encoded unseal keys
- Store sensitive keys in Kubernetes secrets, not in the YAML directly
- Use `tlsSkipVerify: false` (default) in production
- Set appropriate `threshold` values based on your security requirements
- Test configurations in development before applying to production
- Monitor operator logs for any unsealing issues
