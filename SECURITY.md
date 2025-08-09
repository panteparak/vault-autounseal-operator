# Security Overview

This document outlines the security features, considerations, and best practices for the Vault Auto-Unseal Operator.

## Security Features

### 1. Input Validation and Sanitization

The operator implements comprehensive input validation:

- **URL validation**: Prevents injection attacks and validates schemes, hostnames
- **Kubernetes name validation**: DNS-compliant naming with length limits
- **Base64 validation**: Ensures unseal keys are properly encoded
- **Threshold validation**: Prevents integer overflow and logical errors
- **Resource limits**: Protects against DoS via oversized inputs

### 2. Secret Management

- **Kubernetes Secrets**: Support for storing unseal keys in K8s secrets
- **Memory protection**: Sensitive data cleared immediately after use
- **Log sanitization**: Automatic redaction of sensitive fields in logs
- **No key persistence**: Keys not stored permanently in operator memory

### 3. Network Security

- **TLS verification**: Enabled by default, with warnings when disabled
- **Security headers**: HTTP security headers added to all requests
- **Retry strategy**: Exponential backoff with jitter to prevent thundering herd
- **Timeout configuration**: Request timeouts to prevent hanging connections

### 4. Kubernetes Security

- **RBAC**: Minimal required permissions following principle of least privilege
- **Security context**: Non-root user, read-only filesystem, no privilege escalation
- **Namespace isolation**: Resources scoped to specific namespaces
- **Pod security standards**: Compatible with restricted security profiles

## Threat Model

### Protected Against

1. **Injection Attacks**
   - URL injection and path traversal
   - Command injection via parameters
   - Log injection via malicious input
   - YAML/JSON injection in CRDs

2. **Resource Exhaustion**
   - DoS via oversized inputs
   - Memory exhaustion from large keys
   - CPU exhaustion from regex DoS
   - Network resource exhaustion

3. **Information Disclosure**
   - Unseal keys in logs
   - Sensitive data in error messages
   - Debug information leakage
   - Stack trace exposure

4. **Privilege Escalation**
   - Container breakout prevention
   - File system access restrictions
   - Network policy compliance
   - RBAC boundary enforcement

### Considerations

1. **Vault Security**: Operator inherits Vault's security model
2. **Network Security**: Secure network policies recommended
3. **Secret Storage**: Kubernetes secret encryption at rest
4. **Audit Logging**: Enable Kubernetes audit logging for compliance
5. **Image Security**: Use distroless/minimal base images in production

## Configuration Security

### Secure Configuration

```yaml
apiVersion: vault.io/v1
kind: VaultUnsealConfig
metadata:
  name: secure-vault
  namespace: vault
spec:
  url: "https://vault.example.com:8200"  # HTTPS only
  unsealKeys:
    secretRef:                           # Use secret reference
      name: vault-unseal-keys
      namespace: vault-secrets            # Dedicated secrets namespace
      key: keys
  threshold: 3
  tlsSkipVerify: false                   # Always verify TLS
  reconcileInterval: "30s"
```

### Secret Creation

```bash
# Create secret with proper encoding
keys='["key1", "key2", "key3"]'
kubectl create secret generic vault-unseal-keys \
  --namespace=vault-secrets \
  --from-literal=keys="$(echo -n "$keys" | base64)"
```

## Deployment Security

### RBAC Configuration

The operator requires minimal permissions:

```yaml
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: vault-autounseal-operator
rules:
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "list", "watch"]  # Read-only pod access
- apiGroups: [""]
  resources: ["secrets"]
  verbs: ["get"]                   # Read-only secret access
- apiGroups: ["vault.io"]
  resources: ["vaultunsealconfigs"]
  verbs: ["get", "list", "watch", "update"]
```

### Pod Security

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: vault-autounseal-operator
spec:
  template:
    spec:
      securityContext:
        runAsNonRoot: true
        runAsUser: 65534
        fsGroup: 65534
      containers:
      - name: operator
        securityContext:
          allowPrivilegeEscalation: false
          capabilities:
            drop: ["ALL"]
          readOnlyRootFilesystem: true
          runAsNonRoot: true
          runAsUser: 65534
```

### Network Policies

```yaml
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: vault-operator-netpol
  namespace: vault-operator
spec:
  podSelector:
    matchLabels:
      app: vault-autounseal-operator
  policyTypes:
  - Egress
  egress:
  - to: []  # Vault endpoints
    ports:
    - protocol: TCP
      port: 8200
  - to: []  # Kubernetes API
    ports:
    - protocol: TCP
      port: 443
```

## Monitoring and Alerting

### Security Events to Monitor

1. **Authentication failures** to Vault
2. **TLS verification bypassed** (tlsSkipVerify: true)
3. **Excessive unseal attempts** (potential brute force)
4. **Secret access failures** (missing or inaccessible secrets)
5. **Pod selection mismatches** (unexpected pods targeted)

### Recommended Alerts

```yaml
# Example Prometheus alert rules
groups:
- name: vault-operator-security
  rules:
  - alert: VaultOperatorTLSSkipped
    expr: vault_operator_tls_skip_verify > 0
    labels:
      severity: warning
    annotations:
      summary: "Vault operator bypassing TLS verification"

  - alert: VaultOperatorUnsealFailures
    expr: rate(vault_operator_unseal_failures_total[5m]) > 0.1
    labels:
      severity: critical
    annotations:
      summary: "High vault unseal failure rate"
```

## Compliance

### SOC 2 / ISO 27001

- Audit logging enabled
- Access controls implemented
- Data encryption in transit
- Incident response procedures

### PCI DSS

- Network segmentation supported
- Secure development practices
- Regular security testing
- Vulnerability management

## Incident Response

### Security Incident Procedures

1. **Identify**: Monitor logs and alerts
2. **Contain**: Disable operator if compromised
3. **Investigate**: Review audit logs and configurations
4. **Recover**: Rotate secrets, update configurations
5. **Learn**: Update security controls and procedures

### Recovery Steps

1. Rotate all Vault unseal keys
2. Update Kubernetes secrets
3. Restart operator with new configuration
4. Review and update RBAC permissions
5. Validate security controls

## Security Testing

The operator includes comprehensive security tests:

```bash
# Run security-focused tests
make test-security

# Run static security analysis
make security-scan

# Run all tests with coverage
make test-coverage
```

## Reporting Security Issues

To report security vulnerabilities:

1. **Do not** open public GitHub issues
2. Email security concerns to: security@your-domain.com
3. Include detailed reproduction steps
4. Allow reasonable time for patching before disclosure

## Updates and Patches

- Monitor for security updates
- Subscribe to security advisories
- Test updates in staging environment
- Apply patches promptly
- Review configuration after updates
