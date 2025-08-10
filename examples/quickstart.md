# Quick Start Guide

## Running the Vault Auto-Unseal Operator

The operator is designed to run inside a Kubernetes cluster. If you try to run it outside of a cluster without proper configuration, it will exit with an error.

### Option 1: Deploy with Helm (Recommended)

```bash
# Add the Helm repository
helm repo add vault-operator https://panteparak.github.io/vault-autounseal-operator/helm/
helm repo update

# Install the operator
helm install vault-autounseal-operator vault-operator/vault-autounseal-operator \
  --namespace vault-system \
  --create-namespace
```

### Option 2: Deploy with kubectl

```bash
# Apply CRDs
kubectl apply -f https://github.com/panteparak/vault-autounseal-operator/releases/download/v0.4.2/vault.io_vaultunsealconfigs.yaml

# Deploy the operator (example manifest)
kubectl apply -f - <<EOF
apiVersion: apps/v1
kind: Deployment
metadata:
  name: vault-autounseal-operator
  namespace: vault-system
spec:
  replicas: 1
  selector:
    matchLabels:
      app: vault-autounseal-operator
  template:
    metadata:
      labels:
        app: vault-autounseal-operator
    spec:
      serviceAccountName: vault-autounseal-operator
      containers:
      - name: manager
        image: ghcr.io/panteparak/vault-autounseal-operator:0.4.2
        args:
        - --leader-elect
        ports:
        - containerPort: 8080
          name: metrics
        - containerPort: 8081
          name: health
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8081
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
EOF
```

### Option 3: Local Development with kubeconfig

If you want to run the operator locally for development:

```bash
# Ensure you have a valid kubeconfig
export KUBECONFIG=~/.kube/config

# Run the operator locally
docker run --rm \
  -v ~/.kube/config:/kubeconfig:ro \
  -e KUBECONFIG=/kubeconfig \
  ghcr.io/panteparak/vault-autounseal-operator:0.4.2 \
  --kubeconfig=/kubeconfig
```

## Common Issues

### Exit Code 1 (Not 128)
If you see the operator exit with code 1, this is normal behavior when:
- Running outside of a Kubernetes cluster without proper kubeconfig
- Kubernetes API is unreachable
- Missing required RBAC permissions

**Error message:**
```
unable to get kubernetes config - ensure operator is running in cluster or has valid kubeconfig
```

**Solution:** Deploy the operator inside a Kubernetes cluster using one of the methods above.

## Creating a VaultUnsealConfig

Once the operator is running, create a configuration:

```yaml
apiVersion: vault.io/v1
kind: VaultUnsealConfig
metadata:
  name: my-vault-cluster
  namespace: vault-system
spec:
  vaultInstances:
  - name: vault-0
    endpoint: "https://vault-0.vault.svc.cluster.local:8200"
    tlsSkipVerify: false
    unsealKeys:
    - "base64-encoded-unseal-key-1"
    - "base64-encoded-unseal-key-2"
    - "base64-encoded-unseal-key-3"
    threshold: 3
```

Apply it:
```bash
kubectl apply -f vault-config.yaml
```
