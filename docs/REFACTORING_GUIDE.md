# Refactoring Guide: Layer-Based to Feature-Based Architecture

## Overview

This document provides a comprehensive guide for the architectural refactoring of the Vault Autounseal Operator from a layer-based to a feature-based package structure.

## Migration Summary

### Before (Layer-Based)
```
pkg/
├── api/v1/          # API types and schemas
├── controller/      # Kubernetes controllers
└── vault/           # All vault-related functionality
```

### After (Feature-Based)
```
pkg/
├── core/            # Core domain types
├── unsealing/       # Vault unsealing feature
├── operator/        # Kubernetes operator feature
└── testing/         # Shared testing utilities
```

## Detailed File Mapping

### Core Types Migration

| Old Location | New Location | Purpose |
|--------------|--------------|---------|
| `pkg/vault/interfaces.go` | `pkg/core/types/interfaces.go` | Core domain interfaces |
| `pkg/vault/errors.go` | `pkg/core/types/errors.go` | Error types and handling |

### Unsealing Feature Migration

| Old Location | New Location | Purpose |
|--------------|--------------|---------|
| `pkg/vault/client.go` | `pkg/unsealing/client/client.go` | Vault HTTP client |
| `pkg/vault/client_test.go` | `pkg/unsealing/client/client_test.go` | Client unit tests |
| `pkg/vault/strategy.go` | `pkg/unsealing/strategy/strategy.go` | Unsealing strategies |
| `pkg/vault/strategy_test.go` | `pkg/unsealing/strategy/strategy_test.go` | Strategy tests |
| `pkg/vault/validator.go` | `pkg/unsealing/validation/validator.go` | Key validation |
| `pkg/vault/validator_test.go` | `pkg/unsealing/validation/validator_test.go` | Validation tests |

### Testing Infrastructure Migration

| Old Location | New Location | Purpose |
|--------------|--------------|---------|
| `pkg/vault/mocks.go` | `pkg/testing/mocks/mocks.go` | Mock implementations |
| `pkg/vault/integration_suite_test.go` | `pkg/testing/integration/testcontainers_suite.go` | Integration test suite |

### Files Consolidated/Removed

The following redundant test files were identified for removal:
- `pkg/vault/fast_integration_test.go` - Replaced by Testcontainers suite
- `pkg/vault/comprehensive_test.go` - Functionality merged into feature-specific tests
- `pkg/vault/load_test.go` - Replaced by benchmark tests
- `pkg/vault/client_refactored_test.go` - Duplicate of client_test.go
- `pkg/vault/security_edge_cases_test.go` - Minor edge cases, not core functionality
- `pkg/vault/integration_extended_test.go` - Superseded by new integration suite
- `pkg/vault/extended_integration_positive_test.go` - Redundant test cases
- `pkg/vault/extended_integration_negative_test.go` - Redundant test cases

## Code Changes Required

### 1. Import Statement Updates

**Before:**
```go
import "github.com/panteparak/vault-autounseal-operator/pkg/vault"
```

**After:**
```go
import (
    "github.com/panteparak/vault-autounseal-operator/pkg/core/types"
    "github.com/panteparak/vault-autounseal-operator/pkg/unsealing/client"
    "github.com/panteparak/vault-autounseal-operator/pkg/unsealing/validation"
    "github.com/panteparak/vault-autounseal-operator/pkg/unsealing/strategy"
)
```

### 2. Type References

**Before:**
```go
var client *vault.Client
var validator vault.KeyValidator
var strategy vault.UnsealStrategy
```

**After:**
```go
var vaultClient types.VaultClient
var validator types.KeyValidator
var strategy types.UnsealStrategy
```

### 3. Constructor Calls

**Before:**
```go
client := vault.NewClient(endpoint, tlsSkip, timeout)
validator := vault.NewDefaultKeyValidator()
strategy := vault.NewDefaultUnsealStrategy(validator, metrics)
```

**After:**
```go
vaultClient := client.NewClient(endpoint, tlsSkip, timeout)
validator := validation.NewDefaultKeyValidator()
strategy := strategy.NewDefaultUnsealStrategy(validator, metrics)
```

### 4. Error Handling

**Before:**
```go
if vault.IsRetryableError(err) {
    // retry logic
}
```

**After:**
```go
if types.IsRetryableError(err) {
    // retry logic
}
```

## Testing Migration

### Mock Usage

**Before:**
```go
mockClient := &vault.MockVaultClient{}
```

**After:**
```go
import "github.com/panteparak/vault-autounseal-operator/pkg/testing/mocks"

mockClient := &mocks.MockVaultClient{}
```

### Integration Tests

**Before:**
```go
// Multiple different integration test files with varying approaches
```

**After:**
```go
import "github.com/panteparak/vault-autounseal-operator/pkg/testing/integration"

// Single, unified Testcontainers-based suite
func TestVaultIntegration(t *testing.T) {
    suite.Run(t, new(integration.VaultTestSuite))
}
```

## Dependency Management

### go.mod Updates

Ensure the module path is correct for all new packages:

```go
module github.com/panteparak/vault-autounseal-operator

// No additional dependencies required for the refactoring itself
// Existing dependencies remain the same
```

### Internal Dependencies

The new architecture creates clear dependency flow:

```
core/types (no internal dependencies)
    ↓
unsealing/validation → core/types
    ↓
unsealing/strategy → core/types, unsealing/validation
    ↓
unsealing/client → core/types, unsealing/strategy
    ↓
operator/controller → core/types, unsealing/*
    ↓
testing/mocks → core/types
testing/integration → unsealing/client
```

## Build and Test Configuration

### Makefile Updates

Update build targets to reflect new structure:

```makefile
# Test specific features
test-core:
	go test ./pkg/core/...

test-unsealing:
	go test ./pkg/unsealing/...

test-operator:
	go test ./pkg/operator/...

# Integration tests
test-integration:
	go test -tags=integration ./pkg/testing/integration/...

# All tests
test-all:
	go test ./pkg/...
```

### CI/CD Pipeline Updates

Update pipeline configurations to run tests for specific features:

```yaml
# GitHub Actions example
- name: Test Core
  run: go test ./pkg/core/...

- name: Test Unsealing
  run: go test ./pkg/unsealing/...

- name: Test Integration
  run: go test -tags=integration ./pkg/testing/integration/...
```

## Migration Checklist

### Phase 1: Core Infrastructure
- [ ] Create new package structure
- [ ] Move interfaces to `core/types`
- [ ] Move error types to `core/types`
- [ ] Update import statements in moved files

### Phase 2: Feature Migration
- [ ] Move client code to `unsealing/client`
- [ ] Move strategy code to `unsealing/strategy`
- [ ] Move validation code to `unsealing/validation`
- [ ] Update all internal imports

### Phase 3: Testing Infrastructure
- [ ] Move mocks to `testing/mocks`
- [ ] Consolidate integration tests
- [ ] Remove redundant test files
- [ ] Update test imports

### Phase 4: Documentation
- [ ] Update architecture documentation
- [ ] Update API documentation
- [ ] Update README files
- [ ] Create migration guide

### Phase 5: Validation
- [ ] Run all tests to ensure nothing is broken
- [ ] Verify build process works
- [ ] Check import paths are correct
- [ ] Validate CI/CD pipeline

## Benefits Realized

### 1. **Improved Code Organization**
- Related functionality is co-located
- Clear feature boundaries
- Easier to understand system architecture

### 2. **Enhanced Testability**
- Feature-specific testing
- Shared test utilities
- Better mock organization

### 3. **Reduced Technical Debt**
- Removed redundant test files (8 files eliminated)
- Consolidated overlapping functionality
- Cleaner dependency structure

### 4. **Better Maintainability**
- Changes are localized to specific features
- Easier to add new functionality
- Clear ownership boundaries

### 5. **Improved Developer Experience**
- Faster test execution (feature-specific tests)
- Clearer code navigation
- Better IDE support

## Rollback Plan

If issues are discovered after migration:

1. **Immediate rollback**: Revert to previous commit
2. **Selective rollback**: Keep new structure but revert specific changes
3. **Fix-forward**: Address issues while maintaining new structure

## Future Enhancements

The new architecture enables:

1. **Plugin system**: Easy addition of new unsealing strategies
2. **Multi-backend support**: Support for different Vault configurations
3. **Enhanced observability**: Feature-specific metrics and monitoring
4. **Microservice extraction**: Features can be extracted as separate services
5. **Team ownership**: Clear boundaries for team responsibilities

## Conclusion

This refactoring improves the codebase's maintainability, testability, and extensibility while reducing technical debt. The feature-based architecture provides a solid foundation for future development and scaling.
