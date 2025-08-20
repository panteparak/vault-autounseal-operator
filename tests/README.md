# Test Modules

This directory contains all test modules for the vault-autounseal-operator, organized by test type and scope.

## Directory Structure

```
tests/
├── unit/           # Unit tests for individual components
├── integration/    # Integration tests using TestContainers
├── e2e/           # End-to-end workflow tests
├── performance/   # Performance and load testing
├── chaos/         # Chaos engineering tests
├── boundary/      # Boundary and edge case tests
├── common/        # Shared test utilities and helpers
└── fixtures/      # Test data and fixtures
```

## Test Categories

### Unit Tests (`unit/`)
- Fast, isolated tests for individual functions and components
- No external dependencies or containers
- Mock-based testing for dependencies

### Integration Tests (`integration/`)
- Tests using real TestContainers (Vault, K3s)
- Component integration testing
- API compatibility testing

### End-to-End Tests (`e2e/`)
- Complete workflow testing
- Multi-component scenarios
- Real environment simulation

### Performance Tests (`performance/`)
- Load testing and benchmarks
- Resource usage analysis
- Scalability testing

### Chaos Tests (`chaos/`)
- Fault injection testing
- Container termination scenarios
- Network partition simulation

### Boundary Tests (`boundary/`)
- Edge case testing
- Limit testing (large configs, extreme values)
- Error condition boundaries

## Running Tests

### All Tests
```bash
make test-all
```

### By Category
```bash
make test-unit          # Unit tests only
make test-integration   # Integration tests only  
make test-e2e          # E2E tests only
make test-performance  # Performance tests only
make test-chaos        # Chaos tests only
make test-boundary     # Boundary tests only
```

### Individual Test Suites
```bash
cd tests/integration
go test -v ./validation/...
go test -v ./controller/...
```

## Test Configuration

Each test module can be configured via environment variables:
- `TEST_TIMEOUT`: Test timeout (default: 30m)
- `TEST_CONTAINERS`: Enable TestContainers (default: true)  
- `TEST_PARALLELISM`: Number of parallel tests (default: 4)
- `TEST_VERBOSE`: Verbose output (default: false)

## Test Requirements

### Dependencies
- Docker (for TestContainers)
- Go 1.23.1+
- Kubernetes cluster access (for some integration tests)

### Resources
- Minimum 4GB RAM
- Docker with 2GB+ memory limit
- Network access for pulling container images

## CI/CD Integration

Tests are organized to support different CI/CD scenarios:
- **PR validation**: Unit + Integration tests
- **Nightly builds**: All test categories
- **Release validation**: E2E + Performance tests
- **Chaos testing**: Scheduled chaos runs