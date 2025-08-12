# Integration Test Framework

This directory contains a comprehensive, modular integration test framework for the Vault Auto-Unseal Operator. The framework is designed for maintainability, scalability, and ease of use.

## 🏗️ Architecture

The test framework follows a clean architecture with separated concerns:

```
test/integration/
├── framework/           # Core testing framework
│   ├── types.go        # Type definitions and interfaces
│   ├── framework.go    # Main framework implementation
│   ├── config.go       # Configuration management
│   ├── infrastructure.go # Infrastructure management (Docker, K8s)
│   └── reporter.go     # Test reporting and metrics
├── testcases/          # Individual test case implementations
│   ├── vault_unsealing_test.go
│   ├── operator_status_test.go
│   └── crd_validation_test.go
├── suites/             # Test suite compositions
│   └── integration_suite.go
├── main.go             # Test runner entry point
├── go.mod              # Go module definition
└── README.md           # This file
```

## 🚀 Key Features

### 1. **Modular Test Infrastructure**
- **Separation of Concerns**: Infrastructure setup, test cases, and reporting are completely separated
- **Pluggable Architecture**: Easy to add new test cases, infrastructure providers, or reporters
- **Reusable Components**: Framework components can be reused across different test scenarios

### 2. **Comprehensive Test Coverage**
- **API Validation**: Direct Vault API testing for seal/unseal status
- **CRD Status Monitoring**: Kubernetes resource status validation
- **Operator Behavior**: End-to-end operator functionality testing
- **Edge Case Handling**: Tests for already-unsealed vaults, multiple configurations, etc.

### 3. **Advanced Configuration Management**
- **YAML-based Configuration**: Flexible test configuration with defaults
- **Environment-specific Settings**: Different configs for different environments
- **Scenario-based Testing**: Support for basic, failover, and multi-vault scenarios

### 4. **Rich Reporting and Metrics**
- **JSON Reports**: Structured test results for CI/CD integration
- **HTML Reports**: Human-readable test reports with detailed logs
- **Performance Metrics**: Response times, resource usage, API call counts
- **Test Artifacts**: Automatic collection of logs and diagnostic information

### 5. **Docker-based Infrastructure**
- **Isolated Testing**: Each test runs in isolated Docker containers
- **Realistic Scenarios**: Production-like Vault instances with proper initialization
- **Easy Cleanup**: Automatic cleanup of test resources

## 📋 Test Scenarios

### Basic Scenario
- Single Vault instance in dev mode
- Tests fundamental unsealing functionality
- Validates basic operator behavior

### Failover Scenario
- Primary + Standby Vault instances
- Tests failover mechanisms
- Validates multi-instance coordination

### Multi-Vault Scenario
- Three independent Vault clusters (Finance, Engineering, Operations)
- Tests complex multi-tenant scenarios
- Validates operator scalability

## 🔧 Usage

### Running All Tests
```bash
cd test/integration
go run main.go
```

### Running Specific Scenario
```bash
go run main.go -scenario=basic
go run main.go -scenario=failover
go run main.go -scenario=multi-vault
```

### Custom Configuration
```bash
go run main.go -config=custom-config.yaml -scenario=basic -verbose=true
```

### CI/CD Integration
The framework integrates seamlessly with GitHub Actions:

```yaml
- name: Run Integration Tests
  run: |
    cd test/integration
    go run main.go -scenario=basic -timeout=20m
```

## ⚙️ Configuration

### Example Configuration (test-config.yaml)
```yaml
vaultVersion: "1.20.0"
testScenarios: ["basic", "failover", "multi-vault"]
timeouts:
  vaultStartup: 60s
  operatorReady: 120s
  vaultUnseal: 30s
  statusUpdate: 45s
  testExecution: 300s
  cleanupTimeout: 60s
vaultConfig:
  devMode: true
  initializeVaults: true
  unsealThreshold: 3
  secretShares: 3
  tlsConfig:
    skipVerify: true
operatorConfig:
  image: "vault-autounseal-operator"
  tag: "test"
  logLevel: "debug"
testSettings:
  parallel: false
  maxConcurrency: 3
  failFast: false
  verboseLogging: true
  collectLogs: true
  generateReports: true
```

## 🧪 Adding New Test Cases

### 1. Create Test Case
```go
// testcases/my_new_test.go
package testcases

import (
    "context"
    "github.com/panteparak/vault-autounseal-operator/test/integration/framework"
)

type MyNewTest struct {
    scenario string
}

func NewMyNewTest(scenario string) framework.TestCase {
    return &MyNewTest{scenario: scenario}
}

func (t *MyNewTest) Name() string {
    return "my-new-test"
}

func (t *MyNewTest) Execute(ctx context.Context, fw *framework.TestFramework) *framework.TestResult {
    // Test implementation
    return &framework.TestResult{
        TestName: t.Name(),
        Success:  true,
        // ... other fields
    }
}

// Implement other required methods...
```

### 2. Add to Test Suite
```go
// suites/integration_suite.go
testCases := []framework.TestCase{
    testcases.NewVaultUnsealingTest(scenario),
    testcases.NewOperatorStatusTest(scenario),
    testcases.NewMyNewTest(scenario), // Add your test
}
```

## 📊 Test Reports

The framework generates comprehensive test reports:

### JSON Report Structure
```json
{
  "generatedAt": "2024-01-15T10:30:00Z",
  "totalDuration": "5m30s",
  "summary": {
    "totalTests": 12,
    "passedTests": 11,
    "failedTests": 1,
    "successRate": 91.7
  },
  "suites": [
    {
      "suiteName": "integration-test-suite-basic",
      "duration": "2m15s",
      "success": true,
      "testResults": [...]
    }
  ]
}
```

### HTML Report Features
- Visual test status indicators
- Detailed error messages and logs
- Performance metrics and timing
- Responsive design for different devices

## 🔍 Debugging and Troubleshooting

### Verbose Logging
```bash
go run main.go -verbose=true
```

### Keep Resources on Failure
```yaml
testSettings:
  keepResourcesOnFail: true
```

### Manual Infrastructure Inspection
```bash
# Check running containers
docker ps | grep vault

# Check Kubernetes resources
kubectl get pods -n vault-operator-system
kubectl get vaultunsealconfigs -o yaml

# Check logs
kubectl logs -l app.kubernetes.io/name=vault-autounseal-operator -n vault-operator-system
```

## 🚦 Quality Gates

The framework includes built-in quality gates:

- **Test Success Rate**: Minimum 95% pass rate
- **Performance Thresholds**: Maximum response times
- **Resource Usage**: Memory and CPU limits
- **Error Rate**: Maximum allowed error count in logs

## 🔄 CI/CD Integration

### GitHub Actions Workflow
The new modular CI workflow (`.github/workflows/integration-tests-go.yml`) provides:

- **Parallel Test Execution**: Scenarios run in parallel for faster feedback
- **Infrastructure Reuse**: Optimized setup and teardown
- **Artifact Collection**: Test reports and logs preserved
- **Quality Gates**: Automated quality checks
- **Failure Diagnostics**: Automatic log collection on failures

### Key Benefits Over Shell-based Tests

| Aspect | Shell-based | Go-based Framework |
|--------|-------------|-------------------|
| **Maintainability** | Low (monolithic scripts) | High (modular components) |
| **Testability** | Hard to unit test | Unit testable components |
| **Extensibility** | Difficult to extend | Easy to add new tests |
| **Error Handling** | Basic | Comprehensive with retries |
| **Reporting** | Limited | Rich JSON/HTML reports |
| **Performance** | Sequential execution | Parallel execution support |
| **Debugging** | Print statements | Structured logging + metrics |
| **Configuration** | Hardcoded values | Flexible YAML configuration |

## 🎯 Best Practices

1. **Test Independence**: Each test should be completely independent
2. **Proper Cleanup**: Always implement cleanup methods
3. **Meaningful Assertions**: Test specific behaviors, not just "no errors"
4. **Resource Limits**: Set appropriate timeouts and resource limits
5. **Structured Logging**: Use structured logging for better debugging
6. **Configuration Management**: Use configuration files instead of hardcoded values

## 🔮 Future Enhancements

- [ ] Support for real cloud environments (AWS, GCP, Azure)
- [ ] Integration with monitoring systems (Prometheus, Grafana)
- [ ] Chaos engineering tests (network partitions, resource constraints)
- [ ] Performance benchmarking and regression detection
- [ ] Integration with security scanning tools
- [ ] Multi-cluster testing scenarios
- [ ] Automated test generation from CRD schemas
