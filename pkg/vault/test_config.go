package vault

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Constants for commonly used strings
const (
	trueString = "true"
)

// TestConfig holds configuration for various test scenarios
type TestConfig struct {
	// Load Testing
	LoadTestDuration     time.Duration
	LoadTestWorkers      int
	LoadTestOperationsPS int

	// Chaos Testing
	ChaosTestDuration       time.Duration
	ChaosTestClients        int
	ChaosFailureProbability float32

	// Security Testing
	SecurityTestIterations int
	SecurityTimingTests    int
	SecurityMemoryPressure bool

	// Property Testing
	PropertyTestIterations int
	PropertyTestKeyCount   int
	PropertyTestMaxKeySize int

	// Memory Testing
	MemoryTestSize       int
	MemoryTestIterations int
	MemoryLeakThreshold  int64 // bytes

	// Performance Testing
	PerformanceIterations    int
	PerformanceLatencyLimit  time.Duration
	PerformanceThroughputMin float64

	// Compatibility Testing
	CompatibilityVersions    []string
	CompatibilitySkipVersion bool

	// Resource Profiling
	ProfileCPU      bool
	ProfileMemory   bool
	ProfileBlock    bool
	ProfileMutex    bool
	ProfileTrace    bool
	ProfileDuration time.Duration

	// Reporting
	ReportVerbose         bool
	ReportMetrics         bool
	ReportMemorySnapshots bool
}

// DefaultTestConfig returns default test configuration
func DefaultTestConfig() *TestConfig {
	return &TestConfig{
		// Load Testing
		LoadTestDuration:     30 * time.Second,
		LoadTestWorkers:      10,
		LoadTestOperationsPS: 100,

		// Chaos Testing
		ChaosTestDuration:       20 * time.Second,
		ChaosTestClients:        15,
		ChaosFailureProbability: 0.2,

		// Security Testing
		SecurityTestIterations: 100,
		SecurityTimingTests:    50,
		SecurityMemoryPressure: true,

		// Property Testing
		PropertyTestIterations: 500,
		PropertyTestKeyCount:   20,
		PropertyTestMaxKeySize: 1024,

		// Memory Testing
		MemoryTestSize:       1024 * 1024, // 1MB
		MemoryTestIterations: 100,
		MemoryLeakThreshold:  50 * 1024 * 1024, // 50MB

		// Performance Testing
		PerformanceIterations:    1000,
		PerformanceLatencyLimit:  10 * time.Millisecond,
		PerformanceThroughputMin: 100.0,

		// Compatibility Testing
		CompatibilityVersions:    []string{"1.12.0", "1.13.0", "1.14.0", "1.15.0"},
		CompatibilitySkipVersion: false,

		// Resource Profiling
		ProfileCPU:      true,
		ProfileMemory:   true,
		ProfileBlock:    true,
		ProfileMutex:    true,
		ProfileTrace:    false, // Expensive, enable only when needed
		ProfileDuration: 30 * time.Second,

		// Reporting
		ReportVerbose:         true,
		ReportMetrics:         true,
		ReportMemorySnapshots: true,
	}
}

// LoadFromEnvironment loads configuration from environment variables
func (tc *TestConfig) LoadFromEnvironment() {
	tc.loadLoadTestConfig()
	tc.loadChaosTestConfig()
	tc.loadSecurityTestConfig()
	tc.loadPropertyTestConfig()
	tc.loadMemoryTestConfig()
	tc.loadPerformanceTestConfig()
	tc.loadProfilingConfig()
	tc.loadReportingConfig()
}

// Helper methods for loading different configuration categories

func (tc *TestConfig) loadLoadTestConfig() {
	if val := os.Getenv("LOAD_TEST_DURATION"); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			tc.LoadTestDuration = d
		}
	}
	if val := os.Getenv("LOAD_TEST_WORKERS"); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			tc.LoadTestWorkers = i
		}
	}
	if val := os.Getenv("LOAD_TEST_OPERATIONS_PS"); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			tc.LoadTestOperationsPS = i
		}
	}
}

func (tc *TestConfig) loadChaosTestConfig() {
	if val := os.Getenv("CHAOS_TEST_DURATION"); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			tc.ChaosTestDuration = d
		}
	}
	if val := os.Getenv("CHAOS_TEST_CLIENTS"); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			tc.ChaosTestClients = i
		}
	}
	if val := os.Getenv("CHAOS_FAILURE_PROBABILITY"); val != "" {
		if f, err := strconv.ParseFloat(val, 32); err == nil {
			tc.ChaosFailureProbability = float32(f)
		}
	}
}

func (tc *TestConfig) loadSecurityTestConfig() {
	if val := os.Getenv("SECURITY_TEST_ITERATIONS"); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			tc.SecurityTestIterations = i
		}
	}
	if val := os.Getenv("SECURITY_TIMING_TESTS"); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			tc.SecurityTimingTests = i
		}
	}
	if val := os.Getenv("SECURITY_MEMORY_PRESSURE"); val != "" {
		tc.SecurityMemoryPressure = val == trueString
	}
}

func (tc *TestConfig) loadPropertyTestConfig() {
	if val := os.Getenv("PROPERTY_TEST_ITERATIONS"); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			tc.PropertyTestIterations = i
		}
	}
	if val := os.Getenv("PROPERTY_TEST_KEY_COUNT"); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			tc.PropertyTestKeyCount = i
		}
	}
	if val := os.Getenv("PROPERTY_TEST_MAX_KEY_SIZE"); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			tc.PropertyTestMaxKeySize = i
		}
	}
}

func (tc *TestConfig) loadMemoryTestConfig() {
	if val := os.Getenv("MEMORY_TEST_SIZE"); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			tc.MemoryTestSize = i
		}
	}
	if val := os.Getenv("MEMORY_TEST_ITERATIONS"); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			tc.MemoryTestIterations = i
		}
	}
	if val := os.Getenv("MEMORY_LEAK_THRESHOLD"); val != "" {
		if i, err := strconv.ParseInt(val, 10, 64); err == nil {
			tc.MemoryLeakThreshold = i
		}
	}
}

func (tc *TestConfig) loadPerformanceTestConfig() {
	if val := os.Getenv("PERFORMANCE_ITERATIONS"); val != "" {
		if i, err := strconv.Atoi(val); err == nil {
			tc.PerformanceIterations = i
		}
	}
	if val := os.Getenv("PERFORMANCE_LATENCY_LIMIT"); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			tc.PerformanceLatencyLimit = d
		}
	}
	if val := os.Getenv("PERFORMANCE_THROUGHPUT_MIN"); val != "" {
		if f, err := strconv.ParseFloat(val, 64); err == nil {
			tc.PerformanceThroughputMin = f
		}
	}
}

func (tc *TestConfig) loadProfilingConfig() {
	if val := os.Getenv("PROFILE_CPU"); val != "" {
		tc.ProfileCPU = val == trueString
	}
	if val := os.Getenv("PROFILE_MEMORY"); val != "" {
		tc.ProfileMemory = val == trueString
	}
	if val := os.Getenv("PROFILE_BLOCK"); val != "" {
		tc.ProfileBlock = val == trueString
	}
	if val := os.Getenv("PROFILE_MUTEX"); val != "" {
		tc.ProfileMutex = val == trueString
	}
	if val := os.Getenv("PROFILE_TRACE"); val != "" {
		tc.ProfileTrace = val == trueString
	}
	if val := os.Getenv("PROFILE_DURATION"); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			tc.ProfileDuration = d
		}
	}
}

func (tc *TestConfig) loadReportingConfig() {
	if val := os.Getenv("REPORT_VERBOSE"); val != "" {
		tc.ReportVerbose = val == trueString
	}
	if val := os.Getenv("REPORT_METRICS"); val != "" {
		tc.ReportMetrics = val == trueString
	}
	if val := os.Getenv("REPORT_MEMORY_SNAPSHOTS"); val != "" {
		tc.ReportMemorySnapshots = val == trueString
	}
}

// GetLoadTestConfig returns configuration for load testing
func (tc *TestConfig) GetLoadTestConfig() LoadTestConfig {
	return LoadTestConfig{
		Duration:     tc.LoadTestDuration,
		Workers:      tc.LoadTestWorkers,
		OperationsPS: tc.LoadTestOperationsPS,
	}
}

// GetChaosTestConfig returns configuration for chaos testing
func (tc *TestConfig) GetChaosTestConfig() ChaosTestConfig {
	return ChaosTestConfig{
		Duration:           tc.ChaosTestDuration,
		Clients:            tc.ChaosTestClients,
		FailureProbability: tc.ChaosFailureProbability,
	}
}

// GetSecurityTestConfig returns configuration for security testing
func (tc *TestConfig) GetSecurityTestConfig() SecurityTestConfig {
	return SecurityTestConfig{
		Iterations:     tc.SecurityTestIterations,
		TimingTests:    tc.SecurityTimingTests,
		MemoryPressure: tc.SecurityMemoryPressure,
	}
}

// GetPropertyTestConfig returns configuration for property-based testing
func (tc *TestConfig) GetPropertyTestConfig() PropertyTestConfig {
	return PropertyTestConfig{
		Iterations: tc.PropertyTestIterations,
		KeyCount:   tc.PropertyTestKeyCount,
		MaxKeySize: tc.PropertyTestMaxKeySize,
	}
}

// GetMemoryTestConfig returns configuration for memory testing
func (tc *TestConfig) GetMemoryTestConfig() MemoryTestConfig {
	return MemoryTestConfig{
		Size:          tc.MemoryTestSize,
		Iterations:    tc.MemoryTestIterations,
		LeakThreshold: tc.MemoryLeakThreshold,
	}
}

// GetPerformanceTestConfig returns configuration for performance testing
func (tc *TestConfig) GetPerformanceTestConfig() PerformanceTestConfig {
	return PerformanceTestConfig{
		Iterations:    tc.PerformanceIterations,
		LatencyLimit:  tc.PerformanceLatencyLimit,
		ThroughputMin: tc.PerformanceThroughputMin,
	}
}

// GetProfilingConfig returns configuration for resource profiling
func (tc *TestConfig) GetProfilingConfig() ProfilingConfig {
	return ProfilingConfig{
		CPU:      tc.ProfileCPU,
		Memory:   tc.ProfileMemory,
		Block:    tc.ProfileBlock,
		Mutex:    tc.ProfileMutex,
		Trace:    tc.ProfileTrace,
		Duration: tc.ProfileDuration,
	}
}

// Configuration types for different test categories

type LoadTestConfig struct {
	Duration     time.Duration
	Workers      int
	OperationsPS int
}

type ChaosTestConfig struct {
	Duration           time.Duration
	Clients            int
	FailureProbability float32
}

type SecurityTestConfig struct {
	Iterations     int
	TimingTests    int
	MemoryPressure bool
}

type PropertyTestConfig struct {
	Iterations int
	KeyCount   int
	MaxKeySize int
}

type MemoryTestConfig struct {
	Size          int
	Iterations    int
	LeakThreshold int64
}

type PerformanceTestConfig struct {
	Iterations    int
	LatencyLimit  time.Duration
	ThroughputMin float64
}

type ProfilingConfig struct {
	CPU      bool
	Memory   bool
	Block    bool
	Mutex    bool
	Trace    bool
	Duration time.Duration
}

// TestSuite represents a collection of tests with shared configuration
type TestSuite struct {
	Name         string
	Config       *TestConfig
	Tests        []TestCase
	SetupFunc    func() error
	TeardownFunc func() error
}

// TestCase represents an individual test case
type TestCase struct {
	Name        string
	Category    string
	Description string
	RunFunc     func(*TestConfig) error
	SkipFunc    func(*TestConfig) bool
	Timeout     time.Duration
}

// NewTestSuite creates a new test suite
func NewTestSuite(name string, config *TestConfig) *TestSuite {
	return &TestSuite{
		Name:   name,
		Config: config,
		Tests:  make([]TestCase, 0),
	}
}

// AddTest adds a test case to the suite
func (ts *TestSuite) AddTest(test TestCase) {
	ts.Tests = append(ts.Tests, test)
}

// Run executes all tests in the suite
func (ts *TestSuite) Run() *TestSuiteResult {
	result := &TestSuiteResult{
		Name:      ts.Name,
		StartTime: time.Now(),
		Results:   make(map[string]*TestResult),
	}

	// Setup
	if ts.SetupFunc != nil {
		if err := ts.SetupFunc(); err != nil {
			result.SetupError = err
			return result
		}
	}

	// Run tests
	for _, test := range ts.Tests {
		testResult := ts.runTest(test)
		result.Results[test.Name] = testResult

		switch {
		case testResult.Error != nil:
			result.Failures++
		case testResult.Skipped:
			result.Skipped++
		default:
			result.Passed++
		}
	}

	// Teardown
	if ts.TeardownFunc != nil {
		if err := ts.TeardownFunc(); err != nil {
			result.TeardownError = err
		}
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	return result
}

// runTest executes a single test case
func (ts *TestSuite) runTest(test TestCase) *TestResult {
	result := &TestResult{
		Name:      test.Name,
		Category:  test.Category,
		StartTime: time.Now(),
	}

	// Check if test should be skipped
	if test.SkipFunc != nil && test.SkipFunc(ts.Config) {
		result.Skipped = true
		result.SkipReason = "Skipped by skip function"
		return result
	}

	// Run test with timeout
	done := make(chan error, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				done <- fmt.Errorf("panic: %v", r)
			}
		}()
		done <- test.RunFunc(ts.Config)
	}()

	timeout := test.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute // Default timeout
	}

	select {
	case err := <-done:
		result.Error = err
	case <-time.After(timeout):
		result.Error = fmt.Errorf("test timed out after %v", timeout)
	}

	result.EndTime = time.Now()
	result.Duration = result.EndTime.Sub(result.StartTime)

	return result
}

// TestSuiteResult represents the result of running a test suite
type TestSuiteResult struct {
	Name          string
	StartTime     time.Time
	EndTime       time.Time
	Duration      time.Duration
	Results       map[string]*TestResult
	Passed        int
	Failures      int
	Skipped       int
	SetupError    error
	TeardownError error
}

// TestResult represents the result of running a single test
type TestResult struct {
	Name       string
	Category   string
	StartTime  time.Time
	EndTime    time.Time
	Duration   time.Duration
	Error      error
	Skipped    bool
	SkipReason string
}

// GetSummary returns a summary of the test suite results
func (tsr *TestSuiteResult) GetSummary() string {
	total := tsr.Passed + tsr.Failures + tsr.Skipped
	passRate := float64(tsr.Passed) / float64(total) * 100

	return fmt.Sprintf(`Test Suite: %s
Duration: %v
Total: %d, Passed: %d (%.1f%%), Failed: %d, Skipped: %d
Setup Error: %v
Teardown Error: %v`,
		tsr.Name,
		tsr.Duration,
		total,
		tsr.Passed,
		passRate,
		tsr.Failures,
		tsr.Skipped,
		tsr.SetupError,
		tsr.TeardownError)
}

// GetFailedTests returns a list of failed test names
func (tsr *TestSuiteResult) GetFailedTests() []string {
	var failed []string
	for name, result := range tsr.Results {
		if result.Error != nil {
			failed = append(failed, name)
		}
	}
	return failed
}

// GetSkippedTests returns a list of skipped test names
func (tsr *TestSuiteResult) GetSkippedTests() []string {
	var skipped []string
	for name, result := range tsr.Results {
		if result.Skipped {
			skipped = append(skipped, name)
		}
	}
	return skipped
}
