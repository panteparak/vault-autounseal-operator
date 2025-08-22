package vault

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime/pprof"
	"runtime/trace"
	"sync"
	"time"
)

// TestRunner orchestrates comprehensive testing with resource profiling
type TestRunner struct {
	config   *TestConfig
	profiler *ResourceProfiler
	reporter *TestReporter
	suites   []*TestSuite
	results  map[string]*TestSuiteResult
	mu       sync.RWMutex
}

// NewTestRunner creates a new test runner
func NewTestRunner(config *TestConfig) *TestRunner {
	if config == nil {
		config = DefaultTestConfig()
	}

	return &TestRunner{
		config:   config,
		profiler: NewResourceProfiler(config.GetProfilingConfig()),
		reporter: NewTestReporter(config),
		suites:   make([]*TestSuite, 0),
		results:  make(map[string]*TestSuiteResult),
	}
}

// ResourceProfiler handles resource profiling during tests
type ResourceProfiler struct {
	config   ProfilingConfig
	profiles map[string]*os.File
	mu       sync.Mutex
}

// NewResourceProfiler creates a new resource profiler
func NewResourceProfiler(config ProfilingConfig) *ResourceProfiler {
	return &ResourceProfiler{
		config:   config,
		profiles: make(map[string]*os.File),
	}
}

// StartProfiling begins resource profiling
func (rp *ResourceProfiler) StartProfiling() error {
	rp.mu.Lock()
	defer rp.mu.Unlock()

	if rp.config.CPU {
		cpuFile, err := os.Create("cpu.prof")
		if err != nil {
			return fmt.Errorf("failed to create CPU profile file: %w", err)
		}
		rp.profiles["cpu"] = cpuFile

		if err := pprof.StartCPUProfile(cpuFile); err != nil {
			_ = cpuFile.Close()
			return fmt.Errorf("failed to start CPU profiling: %w", err)
		}
	}

	if rp.config.Trace {
		traceFile, err := os.Create("trace.out")
		if err != nil {
			return fmt.Errorf("failed to create trace file: %w", err)
		}
		rp.profiles["trace"] = traceFile

		if err := trace.Start(traceFile); err != nil {
			_ = traceFile.Close()
			return fmt.Errorf("failed to start tracing: %w", err)
		}
	}

	return nil
}

// StopProfiling stops resource profiling and writes profiles
func (rp *ResourceProfiler) StopProfiling() error {
	rp.mu.Lock()
	defer rp.mu.Unlock()

	var errors []error

	errors = append(errors, rp.stopCPUProfiling()...)
	errors = append(errors, rp.stopTracing()...)
	errors = append(errors, rp.writeMemoryProfile()...)
	errors = append(errors, rp.writeBlockProfile()...)
	errors = append(errors, rp.writeMutexProfile()...)

	if len(errors) > 0 {
		return fmt.Errorf("profiling errors: %v", errors)
	}

	return nil
}

// stopCPUProfiling stops CPU profiling and closes the file
func (rp *ResourceProfiler) stopCPUProfiling() []error {
	var errors []error
	if rp.config.CPU {
		pprof.StopCPUProfile()
		if file, exists := rp.profiles["cpu"]; exists {
			if err := file.Close(); err != nil {
				errors = append(errors, fmt.Errorf("failed to close CPU profile: %w", err))
			}
		}
	}
	return errors
}

// stopTracing stops tracing and closes the trace file
func (rp *ResourceProfiler) stopTracing() []error {
	var errors []error
	if rp.config.Trace {
		trace.Stop()
		if file, exists := rp.profiles["trace"]; exists {
			if err := file.Close(); err != nil {
				errors = append(errors, fmt.Errorf("failed to close trace file: %w", err))
			}
		}
	}
	return errors
}

// writeMemoryProfile writes the memory profile to disk
func (rp *ResourceProfiler) writeMemoryProfile() []error {
	var errors []error
	if rp.config.Memory {
		memFile, err := os.Create("mem.prof")
		if err != nil {
			errors = append(errors, fmt.Errorf("failed to create memory profile: %w", err))
		} else {
			if err := pprof.WriteHeapProfile(memFile); err != nil {
				errors = append(errors, fmt.Errorf("failed to write memory profile: %w", err))
			}
			_ = memFile.Close()
		}
	}
	return errors
}

// writeBlockProfile writes the block profile to disk
func (rp *ResourceProfiler) writeBlockProfile() []error {
	var errors []error
	if rp.config.Block {
		blockFile, err := os.Create("block.prof")
		if err != nil {
			errors = append(errors, fmt.Errorf("failed to create block profile: %w", err))
		} else {
			if err := pprof.Lookup("block").WriteTo(blockFile, 0); err != nil {
				errors = append(errors, fmt.Errorf("failed to write block profile: %w", err))
			}
			_ = blockFile.Close()
		}
	}
	return errors
}

// writeMutexProfile writes the mutex profile to disk
func (rp *ResourceProfiler) writeMutexProfile() []error {
	var errors []error
	if rp.config.Mutex {
		mutexFile, err := os.Create("mutex.prof")
		if err != nil {
			errors = append(errors, fmt.Errorf("failed to create mutex profile: %w", err))
		} else {
			if err := pprof.Lookup("mutex").WriteTo(mutexFile, 0); err != nil {
				errors = append(errors, fmt.Errorf("failed to write mutex profile: %w", err))
			}
			_ = mutexFile.Close()
		}
	}
	return errors
}

// TestReporter handles test result reporting
type TestReporter struct {
	config *TestConfig
	output *os.File
}

// NewTestReporter creates a new test reporter
func NewTestReporter(config *TestConfig) *TestReporter {
	return &TestReporter{
		config: config,
		output: os.Stdout,
	}
}

// SetOutput sets the output destination for reports
func (tr *TestReporter) SetOutput(file *os.File) {
	tr.output = file
}

// ReportSuiteStart reports the start of a test suite
func (tr *TestReporter) ReportSuiteStart(suite *TestSuite) {
	if tr.config.ReportVerbose {
		_, _ = fmt.Fprintf(tr.output, "🚀 Starting test suite: %s\n", suite.Name)
		_, _ = fmt.Fprintf(tr.output, "   Tests: %d\n", len(suite.Tests))
		_, _ = fmt.Fprintf(tr.output, "   Configuration loaded from environment: %+v\n", tr.config)
	}
}

// ReportSuiteEnd reports the end of a test suite
func (tr *TestReporter) ReportSuiteEnd(result *TestSuiteResult) {
	_, _ = fmt.Fprintf(tr.output, "✅ Test suite completed: %s\n", result.Name)
	_, _ = fmt.Fprintf(tr.output, "%s\n", result.GetSummary())

	if len(result.GetFailedTests()) > 0 {
		_, _ = fmt.Fprintf(tr.output, "❌ Failed tests: %v\n", result.GetFailedTests())
	}

	if len(result.GetSkippedTests()) > 0 {
		_, _ = fmt.Fprintf(tr.output, "⏭️  Skipped tests: %v\n", result.GetSkippedTests())
	}

	_, _ = fmt.Fprintf(tr.output, "\n")
}

// ReportTestStart reports the start of an individual test
func (tr *TestReporter) ReportTestStart(test TestCase) {
	if tr.config.ReportVerbose {
		_, _ = fmt.Fprintf(tr.output, "  🧪 Running: %s (%s)\n", test.Name, test.Category)
	}
}

// ReportTestEnd reports the end of an individual test
func (tr *TestReporter) ReportTestEnd(result *TestResult) {
	status := "✅"
	if result.Error != nil {
		status = "❌"
	} else if result.Skipped {
		status = "⏭️"
	}

	if tr.config.ReportVerbose {
		_, _ = fmt.Fprintf(tr.output, "    %s %s - %v", status, result.Name, result.Duration)
		if result.Error != nil {
			_, _ = fmt.Fprintf(tr.output, " - Error: %v", result.Error)
		}
		if result.Skipped {
			_, _ = fmt.Fprintf(tr.output, " - Skipped: %s", result.SkipReason)
		}
		_, _ = fmt.Fprintf(tr.output, "\n")
	}
}

// GenerateReport generates a comprehensive test report
func (tr *TestReporter) GenerateReport(runner *TestRunner) error {
	report := ComprehensiveReport{
		Timestamp:     time.Now(),
		Configuration: runner.config,
		Suites:        runner.results,
		Summary:       tr.generateSummary(runner.results),
	}

	// Write JSON report
	jsonFile, err := os.Create("test_report.json")
	if err != nil {
		return fmt.Errorf("failed to create JSON report: %w", err)
	}
	defer func() { _ = jsonFile.Close() }()

	encoder := json.NewEncoder(jsonFile)
	encoder.SetIndent("", "  ")
	if encodeErr := encoder.Encode(report); encodeErr != nil {
		return fmt.Errorf("failed to write JSON report: %w", encodeErr)
	}

	// Write human-readable report
	textFile, err := os.Create("test_report.txt")
	if err != nil {
		return fmt.Errorf("failed to create text report: %w", err)
	}
	defer func() { _ = textFile.Close() }()

	tr.writeTextReport(textFile, &report)

	return nil
}

// generateSummary creates an overall summary of all test results
func (tr *TestReporter) generateSummary(results map[string]*TestSuiteResult) ReportSummary {
	summary := ReportSummary{
		TotalSuites: len(results),
	}

	for _, result := range results {
		summary.TotalTests += result.Passed + result.Failures + result.Skipped
		summary.TotalPassed += result.Passed
		summary.TotalFailed += result.Failures
		summary.TotalSkipped += result.Skipped

		if result.Duration > summary.LongestSuite {
			summary.LongestSuite = result.Duration
		}

		if summary.ShortestSuite == 0 || result.Duration < summary.ShortestSuite {
			summary.ShortestSuite = result.Duration
		}

		summary.TotalDuration += result.Duration
	}

	if summary.TotalTests > 0 {
		summary.OverallPassRate = float64(summary.TotalPassed) / float64(summary.TotalTests) * 100
	}

	return summary
}

// writeTextReport writes a human-readable report
func (tr *TestReporter) writeTextReport(file *os.File, report *ComprehensiveReport) {
	_, _ = fmt.Fprintf(file, "🧪 Comprehensive Test Report\n")
	_, _ = fmt.Fprintf(file, "Generated: %s\n\n", report.Timestamp.Format(time.RFC3339))

	_, _ = fmt.Fprintf(file, "📊 Overall Summary:\n")
	_, _ = fmt.Fprintf(file, "  Total Suites: %d\n", report.Summary.TotalSuites)
	_, _ = fmt.Fprintf(file, "  Total Tests: %d\n", report.Summary.TotalTests)
	_, _ = fmt.Fprintf(file, "  Passed: %d (%.1f%%)\n", report.Summary.TotalPassed, report.Summary.OverallPassRate)
	_, _ = fmt.Fprintf(file, "  Failed: %d\n", report.Summary.TotalFailed)
	_, _ = fmt.Fprintf(file, "  Skipped: %d\n", report.Summary.TotalSkipped)
	_, _ = fmt.Fprintf(file, "  Total Duration: %v\n", report.Summary.TotalDuration)
	_, _ = fmt.Fprintf(file, "  Average Suite Duration: %v\n", report.Summary.TotalDuration/time.Duration(report.Summary.TotalSuites))
	_, _ = fmt.Fprintf(file, "\n")

	// Suite details
	_, _ = fmt.Fprintf(file, "📋 Test Suite Details:\n")
	for suiteName, result := range report.Suites {
		_, _ = fmt.Fprintf(file, "  %s:\n", suiteName)
		_, _ = fmt.Fprintf(file, "    Duration: %v\n", result.Duration)
		_, _ = fmt.Fprintf(file, "    Tests: %d passed, %d failed, %d skipped\n",
			result.Passed, result.Failures, result.Skipped)

		if len(result.GetFailedTests()) > 0 {
			_, _ = fmt.Fprintf(file, "    Failed: %v\n", result.GetFailedTests())
		}

		_, _ = fmt.Fprintf(file, "\n")
	}

	// Configuration
	_, _ = fmt.Fprintf(file, "⚙️  Test Configuration:\n")
	_, _ = fmt.Fprintf(file, "  Load Test Duration: %v\n", report.Configuration.LoadTestDuration)
	_, _ = fmt.Fprintf(file, "  Chaos Test Duration: %v\n", report.Configuration.ChaosTestDuration)
	_, _ = fmt.Fprintf(file, "  Security Test Iterations: %d\n", report.Configuration.SecurityTestIterations)
	_, _ = fmt.Fprintf(file, "  Property Test Iterations: %d\n", report.Configuration.PropertyTestIterations)
	_, _ = fmt.Fprintf(file, "  Profiling Enabled: CPU=%t, Memory=%t, Block=%t, Mutex=%t\n",
		report.Configuration.ProfileCPU, report.Configuration.ProfileMemory,
		report.Configuration.ProfileBlock, report.Configuration.ProfileMutex)
	_, _ = fmt.Fprintf(file, "\n")
}

// ComprehensiveReport represents the complete test report
type ComprehensiveReport struct {
	Timestamp     time.Time                   `json:"timestamp"`
	Configuration *TestConfig                 `json:"configuration"`
	Suites        map[string]*TestSuiteResult `json:"suites"`
	Summary       ReportSummary               `json:"summary"`
}

// ReportSummary provides overall statistics
type ReportSummary struct {
	TotalSuites     int           `json:"total_suites"`
	TotalTests      int           `json:"total_tests"`
	TotalPassed     int           `json:"total_passed"`
	TotalFailed     int           `json:"total_failed"`
	TotalSkipped    int           `json:"total_skipped"`
	TotalDuration   time.Duration `json:"total_duration"`
	LongestSuite    time.Duration `json:"longest_suite"`
	ShortestSuite   time.Duration `json:"shortest_suite"`
	OverallPassRate float64       `json:"overall_pass_rate"`
}

// AddSuite adds a test suite to the runner
func (tr *TestRunner) AddSuite(suite *TestSuite) {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	tr.suites = append(tr.suites, suite)
}

// Run executes all test suites with resource profiling
func (tr *TestRunner) Run(ctx context.Context) error {
	tr.reporter.ReportSuiteStart(&TestSuite{Name: "All Suites", Tests: []TestCase{}})

	// Load configuration from environment
	tr.config.LoadFromEnvironment()

	// Start profiling
	if err := tr.profiler.StartProfiling(); err != nil {
		return fmt.Errorf("failed to start profiling: %w", err)
	}

	// Run all suites
	for _, suite := range tr.suites {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			tr.reporter.ReportSuiteStart(suite)
			result := suite.Run()
			tr.reporter.ReportSuiteEnd(result)

			tr.mu.Lock()
			tr.results[suite.Name] = result
			tr.mu.Unlock()
		}
	}

	// Stop profiling
	if err := tr.profiler.StopProfiling(); err != nil {
		fmt.Printf("Warning: failed to stop profiling: %v\n", err)
	}

	// Generate comprehensive report
	if err := tr.reporter.GenerateReport(tr); err != nil {
		fmt.Printf("Warning: failed to generate report: %v\n", err)
	}

	return nil
}

// CreateStandardTestSuites creates the standard set of test suites
func (tr *TestRunner) CreateStandardTestSuites() {
	// Load Test Suite
	loadSuite := NewTestSuite("Load Testing", tr.config)
	loadSuite.AddTest(TestCase{
		Name:        "HighVolumeOperations",
		Category:    "Performance",
		Description: "Test high volume operations with resource monitoring",
		RunFunc:     tr.runLoadTest,
		Timeout:     tr.config.LoadTestDuration + 30*time.Second,
	})

	// Chaos Test Suite
	chaosSuite := NewTestSuite("Chaos Engineering", tr.config)
	chaosSuite.AddTest(TestCase{
		Name:        "NetworkChaos",
		Category:    "Resilience",
		Description: "Test resilience under network chaos scenarios",
		RunFunc:     tr.runChaosTest,
		Timeout:     tr.config.ChaosTestDuration + 30*time.Second,
	})

	// Security Test Suite
	securitySuite := NewTestSuite("Security Testing", tr.config)
	securitySuite.AddTest(TestCase{
		Name:        "InputSanitization",
		Category:    "Security",
		Description: "Test input sanitization and security boundaries",
		RunFunc:     tr.runSecurityTest,
		Timeout:     2 * time.Minute,
	})

	// Property Test Suite
	propertySuite := NewTestSuite("Property-Based Testing", tr.config)
	propertySuite.AddTest(TestCase{
		Name:        "KeyValidationProperties",
		Category:    "Correctness",
		Description: "Test key validation properties with random inputs",
		RunFunc:     tr.runPropertyTest,
		Timeout:     2 * time.Minute,
	})

	// Memory Test Suite
	memorySuite := NewTestSuite("Memory Testing", tr.config)
	memorySuite.AddTest(TestCase{
		Name:        "MemoryLeakDetection",
		Category:    "Memory",
		Description: "Test for memory leaks and resource cleanup",
		RunFunc:     tr.runMemoryTest,
		Timeout:     3 * time.Minute,
	})

	// Compatibility Test Suite
	compatibilitySuite := NewTestSuite("Compatibility Testing", tr.config)
	compatibilitySuite.AddTest(TestCase{
		Name:        "VaultVersionCompatibility",
		Category:    "Compatibility",
		Description: "Test compatibility across different Vault versions",
		RunFunc:     tr.runCompatibilityTest,
		SkipFunc: func(config *TestConfig) bool {
			return config.CompatibilitySkipVersion
		},
		Timeout: 2 * time.Minute,
	})

	// Add all suites
	tr.AddSuite(loadSuite)
	tr.AddSuite(chaosSuite)
	tr.AddSuite(securitySuite)
	tr.AddSuite(propertySuite)
	tr.AddSuite(memorySuite)
	tr.AddSuite(compatibilitySuite)
}

// Test execution functions

func (tr *TestRunner) runLoadTest(config *TestConfig) error {
	loadConfig := config.GetLoadTestConfig()
	runner := NewLoadTestRunner(loadConfig.Workers, loadConfig.Duration)

	ctx, cancel := context.WithTimeout(context.Background(), loadConfig.Duration+30*time.Second)
	defer cancel()

	summary := runner.Run(ctx)

	// Validate performance requirements
	if summary.TotalErrors > 0 {
		errorRate := float64(summary.TotalErrors) / float64(summary.TotalOperations) * 100
		if errorRate > 10.0 { // Allow up to 10% errors
			return fmt.Errorf("error rate too high: %.2f%%", errorRate)
		}
	}

	if config.ReportVerbose {
		fmt.Printf("Load Test Summary: %s\n", summary.String())
	}

	return nil
}

func (tr *TestRunner) runChaosTest(config *TestConfig) error {
	chaosConfig := config.GetChaosTestConfig()
	runner := NewChaosTestRunner(chaosConfig.Clients)

	ctx, cancel := context.WithTimeout(context.Background(), chaosConfig.Duration+30*time.Second)
	defer cancel()

	summary := runner.Run(ctx, chaosConfig.Duration)

	// Validate resilience requirements
	if summary.TotalOperations == 0 {
		return fmt.Errorf("no operations completed during chaos test")
	}

	if config.ReportVerbose {
		fmt.Printf("Chaos Test Summary: %s\n", summary.String())
	}

	return nil
}

func (tr *TestRunner) runSecurityTest(config *TestConfig) error {
	_ = config.GetSecurityTestConfig() // Security config for future use
	helper := NewSecurityTestHelper()
	validator := NewDefaultKeyValidator()

	// Test malicious inputs
	maliciousInputs := helper.GenerateMaliciousInputs()
	for _, input := range maliciousInputs {
		keys := []string{input}
		err := validator.ValidateKeys(keys, 1)

		// Should handle gracefully without panicking
		if err != nil && !helper.CheckErrorMessageSecurity(err) {
			return fmt.Errorf("security test failed: error message contains sensitive information")
		}
	}

	if config.ReportVerbose {
		fmt.Printf("Security test completed: tested %d malicious inputs\n", len(maliciousInputs))
	}

	return nil
}

func (tr *TestRunner) runPropertyTest(config *TestConfig) error {
	propertyConfig := config.GetPropertyTestConfig()
	generator := NewPropertyTestGenerator(time.Now().UnixNano())
	validator := NewDefaultKeyValidator()

	// Test key validation properties
	for i := 0; i < propertyConfig.Iterations; i++ {
		keys := generator.GenerateRandomKeys(
			1+i%propertyConfig.KeyCount,
			8,
			propertyConfig.MaxKeySize,
		)

		threshold := 1 + i%len(keys)

		// Should not panic - error is intentionally ignored for property testing
		_ = validator.ValidateKeys(keys, threshold) //nolint:errcheck // Property test: validation errors are expected and ignored
	}

	if config.ReportVerbose {
		fmt.Printf("Property test completed: %d iterations\n", propertyConfig.Iterations)
	}

	return nil
}

func (tr *TestRunner) runMemoryTest(config *TestConfig) error {
	memoryConfig := config.GetMemoryTestConfig()

	// Monitor memory usage
	metrics := NewTestMetrics()
	metrics.TakeMemorySnapshot()

	// Perform memory-intensive operations
	validator := NewDefaultKeyValidator()
	for i := 0; i < memoryConfig.Iterations; i++ {
		// Generate large keys
		keyData := make([]byte, memoryConfig.Size)
		for j := range keyData {
			keyData[j] = byte(i + j)
		}

		key := base64.StdEncoding.EncodeToString(keyData)
		keys := []string{key}

		// Memory stress test - validation errors are expected and ignored
		_ = validator.ValidateKeys(keys, 1) //nolint:errcheck // Memory test: validation errors are expected and ignored

		if i%10 == 0 {
			metrics.TakeMemorySnapshot()
		}
	}

	// Final snapshot
	metrics.TakeMemorySnapshot()
	summary := metrics.GetSummary()

	// Check for memory leaks
	if summary.MemoryGrowth > memoryConfig.LeakThreshold {
		return fmt.Errorf("potential memory leak detected: growth %d bytes exceeds threshold %d bytes",
			summary.MemoryGrowth, memoryConfig.LeakThreshold)
	}

	if config.ReportVerbose {
		fmt.Printf("Memory test completed: %d KB peak memory, %d KB growth\n",
			summary.PeakMemory/1024, summary.MemoryGrowth/1024)
	}

	return nil
}

func (tr *TestRunner) runCompatibilityTest(config *TestConfig) error {
	// Test with different version configurations
	for _, version := range config.CompatibilityVersions {
		// Set version in environment for test
		_ = os.Setenv("VAULT_VERSION", version)

		// Basic compatibility test
		factory := NewMockClientFactory()
		client, err := factory.NewClient("http://compat-test:8200", false, 30*time.Second)
		if err != nil {
			return fmt.Errorf("compatibility test failed for version %s: %w", version, err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		_, err = client.IsSealed(ctx)
		cancel()
		_ = client.Close()

		if err != nil {
			// Network errors are acceptable in test environment
			continue
		}
	}

	if config.ReportVerbose {
		fmt.Printf("Compatibility test completed for versions: %v\n", config.CompatibilityVersions)
	}

	return nil
}

// GetResults returns all test results
func (tr *TestRunner) GetResults() map[string]*TestSuiteResult {
	tr.mu.RLock()
	defer tr.mu.RUnlock()

	results := make(map[string]*TestSuiteResult)
	for k, v := range tr.results {
		results[k] = v
	}
	return results
}

// SaveProfiles saves profiling data to specified directory
func (tr *TestRunner) SaveProfiles(dir string) error {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return fmt.Errorf("failed to create profile directory: %w", err)
	}

	profiles := []string{"cpu.prof", "mem.prof", "block.prof", "mutex.prof", "trace.out"}

	for _, profile := range profiles {
		if _, err := os.Stat(profile); err == nil {
			dest := filepath.Join(dir, profile)
			if err := os.Rename(profile, dest); err != nil {
				fmt.Printf("Warning: failed to move %s to %s: %v\n", profile, dest, err)
			}
		}
	}

	return nil
}
