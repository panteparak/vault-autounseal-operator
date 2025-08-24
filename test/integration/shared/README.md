# Shared Integration Test Utilities

This package provides a comprehensive set of shared utilities for integration testing of the vault-autounseal-operator. It standardizes TestContainer setup, provides reusable test suites, and eliminates code duplication across integration tests.

## Overview

The shared utilities package includes:

- **`IntegrationTestSuite`**: Base test suite with configurable component setup
- **Specialized Test Suites**: Pre-configured suites for common testing scenarios
- **Managers**: `VaultManager` and `K3sManager` for TestContainer lifecycle management
- **CRD Generator**: Utilities for generating Kubernetes manifests
- **Configuration**: Centralized configuration management with environment overrides

## Quick Start

### Basic Vault-Only Test

```go
package integration

import (
    "testing"
    "github.com/panteparak/vault-autounseal-operator/tests/integration/shared"
    "github.com/stretchr/testify/require"
)

type MyVaultTest struct {
    shared.VaultOnlyTestSuite
}

func (suite *MyVaultTest) TestVaultOperations() {
    vault := suite.GetDefaultVaultInstance()
    require.NotNil(suite.T(), vault)

    // Your test logic here
    health, err := vault.Client.Sys().Health()
    require.NoError(suite.T(), err)
}

func TestMyVaultTest(t *testing.T) {
    shared.RunVaultOnlyTests(t, new(MyVaultTest))
}
```

### Full Integration Test

```go
type MyIntegrationTest struct {
    shared.FullIntegrationTestSuite
}

func (suite *MyIntegrationTest) TestOperatorWorkflow() {
    vault := suite.GetDefaultVaultInstance()
    k3s, exists := suite.GetK3sInstance()
    require.True(suite.T(), exists)

    // Test end-to-end operator functionality
}

func TestMyIntegrationTest(t *testing.T) {
    shared.RunFullIntegrationTests(t, new(MyIntegrationTest))
}
```

## Available Test Suites

### 1. VaultOnlyTestSuite

Use when you only need Vault containers without Kubernetes components.

**Features:**
- Automatic Vault container setup
- Dev mode (unsealed) by default
- No K8s cluster overhead
- Fast execution

**Example Use Cases:**
- Vault API testing
- Vault client library validation
- Vault version compatibility testing

### 2. K3sOnlyTestSuite

Use when you only need Kubernetes clusters without Vault containers.

**Features:**
- K3s cluster with CRDs installed
- Real Kubernetes client
- RBAC manifests applied
- No Vault overhead

**Example Use Cases:**
- CRD validation testing
- Kubernetes controller testing (without Vault)
- RBAC testing

### 3. FullIntegrationTestSuite

Use for complete end-to-end integration testing.

**Features:**
- Both Vault and K3s clusters
- Controller with real K8s client
- CRDs and RBAC installed
- Complete operator workflow testing

**Example Use Cases:**
- End-to-end operator testing
- Production scenario simulation
- Complete workflow validation

### 4. ControllerOnlyTestSuite

Use for fast controller testing with fake clients.

**Features:**
- Controller reconciler setup
- Fake Kubernetes client (fast)
- CRD schemas available
- No container overhead

**Example Use Cases:**
- Unit testing for controllers
- Reconciliation logic testing
- Fast feedback during development

### 5. MultiVaultTestSuite

Use for testing scenarios with multiple Vault instances.

**Features:**
- 3 Vault instances by default
- Named instances (vault-primary, vault-secondary, vault-tertiary)
- Controller setup for multi-vault scenarios
- Failover testing support

**Example Use Cases:**
- Vault failover testing
- Load balancing scenarios
- Multi-region simulation

### 6. CompatibilityTestSuite

Use for testing against multiple component versions.

**Features:**
- Custom Vault and K3s versions
- Full integration setup
- Version-specific testing
- CI compatibility testing

**Example Use Cases:**
- Version compatibility validation
- Upgrade testing
- Legacy version support

## Custom Configuration

For advanced use cases, use the base `IntegrationTestSuite` with custom options:

```go
type MyCustomTest struct {
    shared.IntegrationTestSuite
}

func (suite *MyCustomTest) SetupSuite() {
    options := &shared.IntegrationSetupOptions{
        RequiresVault:       true,
        RequiresK3s:        true,
        RequiresController: true,

        VaultMode:           shared.ProdMode,
        VaultVersion:        "1.17.0",
        NumVaultInstances:   2,
        VaultInstanceNames:  []string{"primary", "backup"},

        K3sVersion:          "v1.29.0-k3s1",
        K3sNamespace:        "vault-system",

        UseRealK8sClient:    true,
        CustomTimeout:       20 * time.Minute,
    }

    suite.SetupIntegrationSuite(options)
}
```

## Configuration Management

The shared utilities use centralized configuration from `tests/config/versions.yaml`:

```yaml
versions:
  vault:
    default: "1.19.0"
    compatibility: ["1.16.0", "1.17.0", "1.18.0", "1.19.0"]
  k3s:
    default: "v1.30.8-k3s1"
    compatibility: ["v1.28.0-k3s1", "v1.29.0-k3s1", "v1.30.8-k3s1"]

test_settings:
  startup_timeout: "180s"
  health_check_timeout: "60s"
  readiness_poll_interval: "5s"
  max_retries: 3
```

### Environment Variable Overrides

Override versions and settings using environment variables:

```bash
export VAULT_VERSION=1.18.0
export K3S_VERSION=v1.29.0-k3s1
export TEST_MAX_RETRIES=5
export ENABLE_COMPATIBILITY_TESTING=true
```

## Helper Methods

### Vault Operations

```go
// Get Vault instances
vault := suite.GetDefaultVaultInstance()
primary, exists := suite.GetVaultInstance("primary")

// Health checks
suite.AssertVaultHealth("default", false) // unsealed
suite.AssertVaultHealth("prod", true)     // sealed

// Create additional instances
prodVault, err := suite.VaultManager().CreateProdVault("prod")
```

### K3s Operations

```go
// Get K3s cluster
k3s, exists := suite.GetK3sInstance()

// Apply manifests
err := suite.K3sManager().ApplyManifest(k3s, yamlManifest)

// Wait for CRDs
err := suite.K3sManager().WaitForCRDReady(k3s, "vaultunsealconfigs.vault.io", 60*time.Second)
```

### Controller Operations

```go
// Reconcile resources
result, err := suite.ReconcileVaultUnsealConfig(namespacedName)

// Wait for conditions
err := suite.WaitForCondition(namespacedName, "Ready", 30*time.Second)
```

## Migration Guide

### From Legacy Integration Tests

1. **Replace manual TestContainer setup:**
   ```go
   // OLD
   vaultContainer, err := vault.Run(ctx, "hashicorp/vault:1.19.0", ...)

   // NEW
   type MyTest struct {
       shared.VaultOnlyTestSuite
   }
   vault := suite.GetDefaultVaultInstance()
   ```

2. **Replace manual K8s setup:**
   ```go
   // OLD
   k3sContainer, err := k3s.Run(ctx, "rancher/k3s:v1.30.8-k3s1", ...)

   // NEW
   type MyTest struct {
       shared.K3sOnlyTestSuite
   }
   k3s, _ := suite.GetK3sInstance()
   ```

3. **Replace manual controller setup:**
   ```go
   // OLD
   scheme := runtime.NewScheme()
   clientgoscheme.AddToScheme(scheme)
   vaultv1.AddToScheme(scheme)
   client := fake.NewClientBuilder().WithScheme(scheme).Build()
   reconciler := &controller.VaultUnsealConfigReconciler{...}

   // NEW
   type MyTest struct {
       shared.ControllerOnlyTestSuite
   }
   reconciler := suite.Reconciler()
   ```

## Performance Optimization

### Choose the Right Suite

- **Fastest**: `ControllerOnlyTestSuite` (no containers)
- **Fast**: `VaultOnlyTestSuite` (Vault containers only)
- **Medium**: `K3sOnlyTestSuite` (K3s containers only)
- **Slowest**: `FullIntegrationTestSuite` (all components)

### Parallel Execution

The shared utilities support parallel test execution:

```go
func TestParallelVaultTests(t *testing.T) {
    t.Parallel() // Safe to run in parallel
    shared.RunVaultOnlyTests(t, new(MyVaultTest))
}
```

### CI Optimization

For CI environments, use appropriate timeouts and skip long-running tests in short mode:

```bash
# Fast CI run
go test -short ./tests/integration/...

# Full CI run
export ENABLE_COMPATIBILITY_TESTING=true
go test ./tests/integration/...
```

## Best Practices

1. **Use specific test suites** for your use case instead of always using `FullIntegrationTestSuite`
2. **Leverage parallel execution** for independent tests
3. **Use environment variables** for CI configuration
4. **Implement proper cleanup** by calling the appropriate `TearDownSuite` methods
5. **Add proper test timeouts** for long-running tests
6. **Use meaningful test names** and logging for debugging

## Examples

See `examples.go` for comprehensive examples of each test suite type and common testing patterns.

## Troubleshooting

### Common Issues

1. **Docker not available**: Ensure Docker is running and accessible
2. **Port conflicts**: Tests automatically use random ports
3. **Timeout issues**: Adjust `CustomTimeout` in setup options
4. **CRD installation failures**: Check K3s cluster startup logs

### Debug Mode

Enable verbose logging:

```bash
export TESTCONTAINERS_RYUK_DISABLED=true  # Keep containers for debugging
go test -v ./tests/integration/...
```

### Configuration Validation

Verify your configuration:

```go
config := suite.Config()
err := config.Validate()
require.NoError(t, err)
```
