# Comprehensive Testing Framework

This document describes the comprehensive testing infrastructure for the Vault Auto-Unseal Operator, including expanded test coverage, resource profiling, and CI integration.

## 🎯 Overview

The testing framework provides extensive coverage across multiple dimensions:

- **Integration Tests**: Complex real-world scenarios with multi-client orchestration
- **Chaos Engineering**: Resilience testing under various failure conditions
- **Load Testing**: Performance and stress testing with resource monitoring
- **Property-Based Testing**: Random input validation and invariant checking
- **Security Testing**: Input sanitization, timing attacks, and memory safety
- **Compatibility Testing**: Cross-version compatibility with different Vault versions
- **Resource Profiling**: CPU, memory, and performance profiling integrated with CI

## 🧪 Test Categories

### 1. Extended Integration Tests (`integration_extended_test.go`)

Tests complex integration scenarios that go beyond basic functionality:

```go
// Multi-client orchestration
It("should handle multiple clients targeting different vault instances")

// Cascading failure scenarios
It("should handle cascading failures across multiple instances")

// Complex unsealing with retries
It("should handle mixed success/failure unsealing with retries")

// Resource management under high churn
It("should properly manage resources under high churn")

// Network partition and recovery
It("should handle network partition scenarios")
```

**Key Features:**
- Concurrent operations across multiple Vault instances
- Cascading failure simulation with gradual recovery
- Resource management validation under high churn
- Network partition and recovery testing
- Configuration edge case handling

### 2. Chaos Engineering Tests (`chaos_test.go`)

Resilience testing under various failure conditions:

```go
// Network chaos with random failures
It("should handle random connection failures")

// Byzantine fault tolerance
It("should handle Byzantine failure scenarios")

// Resource exhaustion
It("should handle memory pressure scenarios")

// Timing-based chaos
It("should handle clock skew scenarios")

// Configuration chaos
It("should handle dynamic configuration changes")
```

**Key Features:**
- Random connection failures and intermittent issues
- Byzantine fault scenarios with mixed behaviors
- Resource exhaustion testing (memory, goroutines)
- Race condition and timing attack simulation
- Dynamic configuration change handling

### 3. Load and Stress Tests (`load_test.go`)

Performance testing under various load conditions:

```go
// High volume operations
It("should handle high volume seal status checks")

// Memory stress testing
It("should handle large key sets efficiently")

// Concurrency stress
It("should handle extreme concurrency without deadlocks")

// Network load simulation
It("should handle varying network conditions")

// Long-running stability
It("should maintain stability over extended periods")
```

**Key Features:**
- High-volume operation testing with metrics collection
- Memory pressure simulation with large datasets
- Extreme concurrency testing without deadlocks
- Network condition variation simulation
- Long-running stability validation

### 4. Property-Based Testing (`property_test.go`)

Validates system properties with randomly generated inputs:

```go
// Base64 encoding properties
It("should satisfy roundtrip property for all valid inputs")

// Key validation invariants
It("should maintain validation invariants under random inputs")

// Threshold constraints
It("should respect threshold constraints for all valid inputs")

// Error handling consistency
It("should maintain error type consistency across versions")
```

**Key Features:**
- Roundtrip property validation for encoding/decoding
- Random input generation for boundary testing
- Invariant checking under various conditions
- Unicode and special character handling
- Concurrent access property validation

### 5. Security-Focused Tests (`security_test.go`)

Comprehensive security testing covering various attack vectors:

```go
// Input sanitization
It("should handle malicious base64 inputs safely")

// Timing attack resistance
It("should resist timing attacks on key validation")

// Memory security
It("should clear sensitive data from memory")

// Cryptographic security
It("should handle cryptographically weak keys")

// Side-channel attack resistance
It("should resist cache timing attacks")
```

**Key Features:**
- Malicious input handling without crashes
- Timing attack resistance validation
- Memory safety and sensitive data clearing
- Weak key pattern detection
- Side-channel attack resistance
- Information leakage prevention

### 6. Compatibility Tests (`compatibility_test.go`)

Cross-version compatibility testing:

```go
// API compatibility across versions
It("should handle seal status response format across versions")

// Feature compatibility
It("should handle version-specific auth requirements")

// Error handling consistency
It("should handle version-specific error formats")

// Performance consistency
It("should maintain performance characteristics across versions")
```

**Key Features:**
- API response format compatibility
- Version-specific feature handling
- Error format consistency across versions
- Performance characteristic validation
- Upgrade/downgrade scenario testing

## 🔧 Testing Infrastructure

### Test Helpers (`test_helpers.go`)

Comprehensive utilities for test execution:

- **TestMetrics**: Detailed metrics collection during test execution
- **LoadTestRunner**: Configurable load testing with operation mix
- **ChaosTestRunner**: Chaos engineering with configurable scenarios
- **PropertyTestGenerator**: Random test data generation
- **SecurityTestHelper**: Security-focused testing utilities
- **TimingAnalyzer**: Timing attack analysis tools

### Test Configuration (`test_config.go`)

Flexible configuration system supporting:

- Environment variable overrides
- Per-test-category configuration
- Performance thresholds and limits
- Profiling options
- Reporting preferences

### Test Runner (`test_runner.go`)

Orchestrates comprehensive testing with:

- **Resource Profiling**: CPU, memory, block, and mutex profiling
- **Test Reporting**: JSON and human-readable reports
- **Test Suites**: Organized test execution with setup/teardown
- **Result Analysis**: Comprehensive result collection and analysis

## 🚀 Usage

### Quick Start

```bash
# Run all comprehensive tests
make -f Makefile.testing test-all

# Run specific test categories
make -f Makefile.testing test-load
make -f Makefile.testing test-chaos
make -f Makefile.testing test-security

# Run with profiling
make -f Makefile.testing profile-all
```

### Environment Configuration

Configure test behavior with environment variables:

```bash
# Load testing
export LOAD_TEST_DURATION=60s
export LOAD_TEST_WORKERS=20
export LOAD_TEST_OPERATIONS_PS=200

# Chaos testing
export CHAOS_TEST_DURATION=45s
export CHAOS_TEST_CLIENTS=25
export CHAOS_FAILURE_PROBABILITY=0.3

# Security testing
export SECURITY_TEST_ITERATIONS=200
export SECURITY_TIMING_TESTS=100

# Property testing
export PROPERTY_TEST_ITERATIONS=1000
export PROPERTY_TEST_KEY_COUNT=50

# Resource profiling
export PROFILE_CPU=true
export PROFILE_MEMORY=true
export PROFILE_BLOCK=true
export PROFILE_MUTEX=true
export PROFILE_DURATION=60s

# Compatibility testing
export VAULT_VERSION=1.15.0

# Reporting
export REPORT_VERBOSE=true
export REPORT_METRICS=true
export REPORT_MEMORY_SNAPSHOTS=true
```

### Programmatic Usage

```go
// Create and configure test runner
config := DefaultTestConfig()
config.LoadTestDuration = 30 * time.Second
config.SecurityTestIterations = 100

runner := NewTestRunner(config)
runner.CreateStandardTestSuites()

// Add custom test suite
customSuite := NewTestSuite("Custom Tests", config)
customSuite.AddTest(TestCase{
    Name:        "CustomTest",
    Category:    "Custom",
    Description: "Custom test logic",
    RunFunc: func(config *TestConfig) error {
        // Your test logic here
        return nil
    },
    Timeout: 30 * time.Second,
})

runner.AddSuite(customSuite)

// Execute all tests
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
defer cancel()

if err := runner.Run(ctx); err != nil {
    log.Fatalf("Test execution failed: %v", err)
}

// Analyze results
results := runner.GetResults()
for suiteName, result := range results {
    fmt.Printf("Suite %s: %d passed, %d failed\n",
        suiteName, result.Passed, result.Failures)
}
```

## 📊 CI Integration

### GitHub Actions Workflow (`.github/workflows/resource-profiling.yml`)

The comprehensive CI workflow provides:

- **Multi-Stage Testing**: Extended integration, chaos, load, property, and security tests
- **Resource Profiling**: CPU, memory, and performance profiling during test execution
- **Memory Leak Detection**: Automated detection of memory leaks and resource issues
- **Performance Regression**: Automated performance threshold validation
- **Compatibility Matrix**: Testing across multiple Vault versions
- **Artifact Collection**: Profile files, reports, and analysis results
- **Summary Reporting**: Comprehensive test execution summaries

### Resource Monitoring

The CI pipeline includes comprehensive resource monitoring:

```yaml
# System resource monitoring during tests
- Run system monitoring with sar
- Capture memory usage, CPU utilization, and I/O metrics
- Generate resource usage reports
- Detect performance regressions automatically
```

### Profile Generation

Automated profile generation during CI:

```yaml
# Multiple profiling types
- CPU profiling with go tool pprof
- Memory profiling with heap analysis
- Block profiling for contention analysis
- Mutex profiling for synchronization issues
- Execution tracing for detailed analysis
```

## 📈 Performance Monitoring

### Metrics Collection

The testing framework collects comprehensive metrics:

- **Operation Metrics**: Count, duration, success/failure rates
- **Memory Metrics**: Allocation patterns, GC behavior, leak detection
- **Concurrency Metrics**: Goroutine counts, synchronization contention
- **System Metrics**: CPU usage, memory pressure, I/O patterns

### Performance Thresholds

Configurable performance thresholds prevent regressions:

```go
// Example performance validation
if avgLatency > config.PerformanceLatencyLimit {
    return fmt.Errorf("latency regression detected: %v > %v",
        avgLatency, config.PerformanceLatencyLimit)
}

if throughput < config.PerformanceThroughputMin {
    return fmt.Errorf("throughput regression detected: %v < %v",
        throughput, config.PerformanceThroughputMin)
}
```

### Memory Leak Detection

Automated memory leak detection:

```go
// Memory growth analysis
if summary.MemoryGrowth > config.MemoryLeakThreshold {
    return fmt.Errorf("potential memory leak: growth %d bytes exceeds threshold",
        summary.MemoryGrowth)
}
```

## 🔍 Analysis and Debugging

### Profile Analysis

Generated profiles can be analyzed with:

```bash
# CPU profile analysis
go tool pprof profiles/cpu.prof

# Memory profile analysis
go tool pprof profiles/mem.prof

# Web UI for interactive analysis
go tool pprof -http=:8080 profiles/cpu.prof

# Block contention analysis
go tool pprof profiles/block.prof

# Mutex contention analysis
go tool pprof profiles/mutex.prof
```

### Report Generation

Comprehensive reports include:

- **Test Execution Summary**: Pass/fail rates, execution times
- **Performance Metrics**: Throughput, latency, resource usage
- **Memory Analysis**: Allocation patterns, leak detection results
- **Security Analysis**: Timing attack resistance, input sanitization results
- **Compatibility Results**: Cross-version compatibility validation

### Debugging Failed Tests

Debug failed tests with enhanced output:

```bash
# Run with debug output
make -f Makefile.testing test-debug

# Run specific failing test with verbose output
go test -v -run=TestSpecificFailingTest ./pkg/vault/

# Generate profiles for failed test analysis
make -f Makefile.testing profile-all
```

## 🎛️ Customization

### Custom Test Scenarios

Add custom chaos scenarios:

```go
chaosRunner.AddChaosScenario(ChaosScenario{
    Name:        "CustomFailure",
    Description: "Custom failure scenario",
    Probability: 0.1,
    ApplyFunc: func(c *MockVaultClient) {
        // Apply custom failure logic
    },
    RecoverFunc: func(c *MockVaultClient) {
        // Recovery logic
    },
})
```

### Custom Load Patterns

Configure custom load patterns:

```go
loadRunner.SetOperationMix(map[string]float32{
    "IsSealed":      0.4,
    "HealthCheck":   0.3,
    "GetSealStatus": 0.2,
    "Unseal":        0.1,
})
```

### Custom Security Tests

Add custom security validations:

```go
helper := NewSecurityTestHelper()
helper.AddSensitivePattern("custom-sensitive-data")

// Test custom malicious inputs
customInputs := []string{"custom-malicious-input"}
for _, input := range customInputs {
    // Custom security test logic
}
```

## 📋 Best Practices

### Test Organization

- **Categorize Tests**: Use categories (Performance, Security, Resilience, etc.)
- **Descriptive Names**: Use clear, descriptive test names
- **Focused Tests**: Keep individual tests focused on specific scenarios
- **Proper Timeouts**: Set appropriate timeouts for different test types

### Performance Testing

- **Baseline Establishment**: Establish performance baselines for comparison
- **Realistic Load**: Use realistic load patterns based on production usage
- **Resource Monitoring**: Always monitor resource usage during performance tests
- **Regression Detection**: Implement automated regression detection

### Security Testing

- **Comprehensive Coverage**: Test all input vectors and attack surfaces
- **Timing Analysis**: Validate timing consistency for security-sensitive operations
- **Memory Safety**: Verify sensitive data clearing and memory safety
- **Error Analysis**: Ensure errors don't leak sensitive information

### CI Integration

- **Parallel Execution**: Run test categories in parallel where possible
- **Artifact Retention**: Retain profiles and reports for analysis
- **Failure Analysis**: Provide detailed failure analysis and debugging information
- **Performance Tracking**: Track performance trends over time

## 🚨 Troubleshooting

### Common Issues

**Test Timeouts:**
- Increase timeout values for slow environments
- Check resource constraints (CPU, memory)
- Verify network connectivity in test environment

**Memory Issues:**
- Monitor memory usage during tests
- Check for memory leaks in test code
- Adjust memory limits for test environment

**Profile Generation:**
- Ensure sufficient permissions for profile file creation
- Check disk space for profile storage
- Verify profiling tools are available

**CI Pipeline Issues:**
- Check environment variable configuration
- Verify test dependencies are installed
- Review CI runner resource limits

### Getting Help

- Check test logs for detailed error information
- Analyze generated profiles for performance issues
- Review comprehensive test reports for failure patterns
- Use debug mode for additional diagnostic information

## 🔗 References

- [Go Testing Package](https://pkg.go.dev/testing)
- [Go Profiling](https://go.dev/blog/pprof)
- [Ginkgo Testing Framework](https://onsi.github.io/ginkgo/)
- [Gomega Matchers](https://onsi.github.io/gomega/)
- [Property-Based Testing](https://en.wikipedia.org/wiki/Property_testing)
- [Chaos Engineering Principles](https://principlesofchaos.org/)

This comprehensive testing framework ensures the Vault Auto-Unseal Operator maintains high quality, performance, and security standards across various operating conditions and Vault versions.
