# Testing Infrastructure

This directory contains the testing infrastructure for the Vault Auto-Unseal Operator.

## Structure

```
test/
├── environments/
│   ├── ci/          # CI-specific Docker Compose environments
│   └── local/       # Local development environments
└── integration/     # Integration test framework (Go-based)
```

## Integration Tests

The project uses **Go-based integration tests** with **Testcontainers** for reliable, fast testing.

### Key Features

- **Testcontainers-based**: Uses latest Testcontainers Go (v0.38.0)
- **Testify test framework**: Structured test suites with setup/teardown
- **Fast execution**: ~8x faster than Docker Compose based tests
- **CI/CD ready**: Works seamlessly in GitHub Actions
- **Local development**: Easy to run locally with Docker

### Running Integration Tests

#### Local Development

```bash
# Quick run
make test-integration

# Verbose output
make test-integration-verbose

# Using the script directly
./scripts/run-integration-tests.sh -v

# With coverage
./scripts/run-integration-tests.sh -c

# Specific test
./scripts/run-integration-tests.sh -r "TestBasicVaultOperations"
```

#### CI Environment

The integration tests run automatically in GitHub Actions using the same Go test suite.

### Test Coverage

The integration test suite covers:

- ✅ **Basic Vault Operations** (dev mode)
- ✅ **Sealed Vault Operations** (production mode)
- ✅ **Client Unsealing** (through operator client)
- ✅ **Multi-Vault Scenarios** (multiple instances)
- ✅ **Failover Testing** (primary/standby)
- ✅ **Performance Testing** (concurrent operations)
- ✅ **VaultUnsealConfig Scenarios** (CRD-like operations)

### Requirements

- Go 1.21+
- Docker (for Testcontainers)
- Make (optional)

## Legacy Docker Compose Environments

Legacy Docker Compose environments are available in `environments/` for specialized testing:

### CI Environments (`test/environments/ci/`)

- `docker-compose.fast-integration.yml` - Fast integration testing
- `docker-compose.test.yml` - Full test environment
- `docker-compose.scenario*.yml` - Specific test scenarios

### Local Environments (`test/environments/local/`)

- `docker-compose.yml` - Full local development environment

#### Usage

```bash
# Start local environment
docker-compose -f test/environments/local/docker-compose.yml up -d

# Run legacy integration tests
make test-docker-local

# CI environment testing
make test-docker-ci
```

## Migration from Docker Compose

The project has migrated from Docker Compose-based integration tests to Go-based Testcontainers for:

- **Faster execution** (8x improvement)
- **Better CI integration**
- **Simplified dependencies**
- **Improved reliability**
- **Modern Go testing patterns**

The Docker Compose environments remain available for specialized use cases and local development.
