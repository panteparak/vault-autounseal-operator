package vault

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// TestMetrics provides detailed testing metrics collection
type TestMetrics struct {
	mu                    sync.RWMutex
	operationCounts       map[string]int64
	operationDurations    map[string][]time.Duration
	errorCounts          map[string]int64
	memorySnapshots      []MemorySnapshot
	concurrentOperations int64
	testStartTime        time.Time
}

// MemorySnapshot captures memory usage at a point in time
type MemorySnapshot struct {
	Timestamp time.Time
	Alloc     uint64
	TotalAlloc uint64
	Sys       uint64
	NumGC     uint32
	Goroutines int
}

// NewTestMetrics creates a new TestMetrics instance
func NewTestMetrics() *TestMetrics {
	return &TestMetrics{
		operationCounts:    make(map[string]int64),
		operationDurations: make(map[string][]time.Duration),
		errorCounts:       make(map[string]int64),
		testStartTime:     time.Now(),
	}
}

// RecordOperation records an operation with its duration
func (tm *TestMetrics) RecordOperation(operation string, duration time.Duration, err error) {
	tm.mu.Lock()
	defer tm.mu.Unlock()

	tm.operationCounts[operation]++
	tm.operationDurations[operation] = append(tm.operationDurations[operation], duration)

	if err != nil {
		tm.errorCounts[operation]++
	}
}

// StartConcurrentOperation increments the concurrent operation counter
func (tm *TestMetrics) StartConcurrentOperation() {
	atomic.AddInt64(&tm.concurrentOperations, 1)
}

// EndConcurrentOperation decrements the concurrent operation counter
func (tm *TestMetrics) EndConcurrentOperation() {
	atomic.AddInt64(&tm.concurrentOperations, -1)
}

// TakeMemorySnapshot captures current memory usage
func (tm *TestMetrics) TakeMemorySnapshot() {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	snapshot := MemorySnapshot{
		Timestamp:  time.Now(),
		Alloc:      m.Alloc,
		TotalAlloc: m.TotalAlloc,
		Sys:        m.Sys,
		NumGC:      m.NumGC,
		Goroutines: runtime.NumGoroutine(),
	}

	tm.mu.Lock()
	tm.memorySnapshots = append(tm.memorySnapshots, snapshot)
	tm.mu.Unlock()
}

// GetSummary returns a summary of collected metrics
func (tm *TestMetrics) GetSummary() TestSummary {
	tm.mu.RLock()
	defer tm.mu.RUnlock()

	summary := TestSummary{
		TestDuration:         time.Since(tm.testStartTime),
		TotalOperations:      0,
		TotalErrors:         0,
		OperationBreakdown:  make(map[string]OperationStats),
		ConcurrentOperations: atomic.LoadInt64(&tm.concurrentOperations),
		MemoryGrowth:        0,
		PeakMemory:          0,
	}

	// Calculate operation stats
	for operation, count := range tm.operationCounts {
		durations := tm.operationDurations[operation]
		errors := tm.errorCounts[operation]

		summary.TotalOperations += count
		summary.TotalErrors += errors

		stats := OperationStats{
			Count:      count,
			Errors:     errors,
			ErrorRate:  float64(errors) / float64(count),
			TotalTime:  0,
			MinTime:    time.Hour, // Initialize with high value
			MaxTime:    0,
		}

		if len(durations) > 0 {
			for _, d := range durations {
				stats.TotalTime += d
				if d < stats.MinTime {
					stats.MinTime = d
				}
				if d > stats.MaxTime {
					stats.MaxTime = d
				}
			}
			stats.AvgTime = stats.TotalTime / time.Duration(len(durations))
		}

		summary.OperationBreakdown[operation] = stats
	}

	// Calculate memory stats
	if len(tm.memorySnapshots) > 1 {
		first := tm.memorySnapshots[0]
		last := tm.memorySnapshots[len(tm.memorySnapshots)-1]

		summary.MemoryGrowth = int64(last.Alloc) - int64(first.Alloc)

		for _, snapshot := range tm.memorySnapshots {
			if snapshot.Alloc > summary.PeakMemory {
				summary.PeakMemory = snapshot.Alloc
			}
		}
	}

	return summary
}

// TestSummary provides a summary of test execution metrics
type TestSummary struct {
	TestDuration         time.Duration
	TotalOperations      int64
	TotalErrors         int64
	OperationBreakdown  map[string]OperationStats
	ConcurrentOperations int64
	MemoryGrowth        int64
	PeakMemory          uint64
}

// OperationStats provides statistics for a specific operation type
type OperationStats struct {
	Count     int64
	Errors    int64
	ErrorRate float64
	TotalTime time.Duration
	AvgTime   time.Duration
	MinTime   time.Duration
	MaxTime   time.Duration
}

// String returns a formatted string representation of the summary
func (ts TestSummary) String() string {
	return fmt.Sprintf(`Test Summary:
  Duration: %v
  Operations: %d (%.2f ops/sec)
  Errors: %d (%.2f%% error rate)
  Concurrent Operations: %d
  Memory Growth: %d KB
  Peak Memory: %d KB`,
		ts.TestDuration,
		ts.TotalOperations,
		float64(ts.TotalOperations)/ts.TestDuration.Seconds(),
		ts.TotalErrors,
		float64(ts.TotalErrors)/float64(ts.TotalOperations)*100,
		ts.ConcurrentOperations,
		ts.MemoryGrowth/1024,
		ts.PeakMemory/1024)
}

// LoadTestRunner provides utilities for running load tests
type LoadTestRunner struct {
	metrics      *TestMetrics
	factory      *MockClientFactory
	numWorkers   int
	duration     time.Duration
	operationMix map[string]float32 // operation name -> probability
}

// NewLoadTestRunner creates a new load test runner
func NewLoadTestRunner(numWorkers int, duration time.Duration) *LoadTestRunner {
	return &LoadTestRunner{
		metrics:    NewTestMetrics(),
		factory:    NewMockClientFactory(),
		numWorkers: numWorkers,
		duration:   duration,
		operationMix: map[string]float32{
			"IsSealed":    0.4,
			"HealthCheck": 0.3,
			"GetSealStatus": 0.2,
			"Unseal":      0.1,
		},
	}
}

// SetOperationMix sets the probability distribution for operations
func (ltr *LoadTestRunner) SetOperationMix(mix map[string]float32) {
	ltr.operationMix = mix
}

// Run executes the load test
func (ltr *LoadTestRunner) Run(ctx context.Context) TestSummary {
	var wg sync.WaitGroup
	ctx, cancel := context.WithTimeout(ctx, ltr.duration)
	defer cancel()

	// Start memory monitoring
	go ltr.memoryMonitor(ctx)

	// Start workers
	for i := 0; i < ltr.numWorkers; i++ {
		wg.Add(1)
		go ltr.worker(ctx, &wg, i)
	}

	wg.Wait()
	return ltr.metrics.GetSummary()
}

// worker runs operations according to the operation mix
func (ltr *LoadTestRunner) worker(ctx context.Context, wg *sync.WaitGroup, workerID int) {
	defer wg.Done()

	// Create client for this worker
	client, err := ltr.factory.NewClient(
		fmt.Sprintf("http://load-test-%d:8200", workerID),
		false,
		100*time.Millisecond,
	)
	if err != nil {
		return
	}
	defer client.Close()

	mockClient := ltr.factory.GetClient(fmt.Sprintf("http://load-test-%d:8200", workerID))

	// Configure mock client
	mockClient.SetSealed(workerID%2 == 0)
	mockClient.SetHealthy(true)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			operation := ltr.selectOperation()
			ltr.executeOperation(ctx, client, mockClient, operation)
		}
	}
}

// selectOperation chooses an operation based on the probability distribution
func (ltr *LoadTestRunner) selectOperation() string {
	r := float32(rand.Intn(1000)) / 1000.0
	cumulative := float32(0.0)

	for operation, probability := range ltr.operationMix {
		cumulative += probability
		if r <= cumulative {
			return operation
		}
	}

	return "IsSealed" // fallback
}

// executeOperation performs the selected operation and records metrics
func (ltr *LoadTestRunner) executeOperation(ctx context.Context, client VaultClient, mockClient *MockVaultClient, operation string) {
	ltr.metrics.StartConcurrentOperation()
	defer ltr.metrics.EndConcurrentOperation()

	start := time.Now()
	var err error

	switch operation {
	case "IsSealed":
		_, err = client.IsSealed(ctx)
	case "HealthCheck":
		_, err = client.HealthCheck(ctx)
	case "GetSealStatus":
		_, err = client.GetSealStatus(ctx)
	case "Unseal":
		keys := []string{base64.StdEncoding.EncodeToString([]byte("test-key"))}
		mockClient.SetSealed(true)
		_, err = client.Unseal(ctx, keys, 1)
	}

	duration := time.Since(start)
	ltr.metrics.RecordOperation(operation, duration, err)
}

// memoryMonitor periodically captures memory snapshots
func (ltr *LoadTestRunner) memoryMonitor(ctx context.Context) {
	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ltr.metrics.TakeMemorySnapshot()
		}
	}
}

// ChaosTestRunner provides utilities for chaos engineering tests
type ChaosTestRunner struct {
	metrics       *TestMetrics
	factory       *MockClientFactory
	clients       []*MockVaultClient
	numClients    int
	chaosScenarios []ChaosScenario
}

// ChaosScenario defines a chaos engineering scenario
type ChaosScenario struct {
	Name        string
	Description string
	Probability float32
	ApplyFunc   func(*MockVaultClient)
	RecoverFunc func(*MockVaultClient)
}

// NewChaosTestRunner creates a new chaos test runner
func NewChaosTestRunner(numClients int) *ChaosTestRunner {
	ctr := &ChaosTestRunner{
		metrics:    NewTestMetrics(),
		factory:    NewMockClientFactory(),
		numClients: numClients,
		clients:    make([]*MockVaultClient, numClients),
	}

	// Create clients
	for i := 0; i < numClients; i++ {
		client, _ := ctr.factory.NewClient(
			fmt.Sprintf("http://chaos-%d:8200", i),
			false,
			100*time.Millisecond,
		)
		ctr.clients[i] = ctr.factory.GetClient(fmt.Sprintf("http://chaos-%d:8200", i))
		_ = client // Keep reference to prevent GC
	}

	// Define default chaos scenarios
	ctr.chaosScenarios = []ChaosScenario{
		{
			Name:        "NetworkFailure",
			Description: "Simulate network connectivity issues",
			Probability: 0.2,
			ApplyFunc: func(c *MockVaultClient) {
				c.SetFailSealStatus(true)
				c.SetFailHealthCheck(true)
			},
			RecoverFunc: func(c *MockVaultClient) {
				c.SetFailSealStatus(false)
				c.SetFailHealthCheck(false)
			},
		},
		{
			Name:        "SlowResponse",
			Description: "Simulate slow network responses",
			Probability: 0.3,
			ApplyFunc: func(c *MockVaultClient) {
				c.SetResponseDelay(100 * time.Millisecond)
			},
			RecoverFunc: func(c *MockVaultClient) {
				c.SetResponseDelay(0)
			},
		},
		{
			Name:        "PartialFailure",
			Description: "Some operations fail while others succeed",
			Probability: 0.25,
			ApplyFunc: func(c *MockVaultClient) {
				c.SetFailSealStatus(true)
			},
			RecoverFunc: func(c *MockVaultClient) {
				c.SetFailSealStatus(false)
			},
		},
		{
			Name:        "StateFluctuation",
			Description: "Vault seal state changes frequently",
			Probability: 0.25,
			ApplyFunc: func(c *MockVaultClient) {
				c.SetSealed(!c.sealed)
			},
			RecoverFunc: func(c *MockVaultClient) {
				// State changes are part of the chaos, no explicit recovery needed
			},
		},
	}

	return ctr
}

// AddChaosScenario adds a custom chaos scenario
func (ctr *ChaosTestRunner) AddChaosScenario(scenario ChaosScenario) {
	ctr.chaosScenarios = append(ctr.chaosScenarios, scenario)
}

// Run executes the chaos test
func (ctr *ChaosTestRunner) Run(ctx context.Context, duration time.Duration) TestSummary {
	ctx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()

	// Start chaos injection
	go ctr.chaosInjector(ctx)

	// Start memory monitoring
	go ctr.memoryMonitor(ctx)

	// Start operations
	var wg sync.WaitGroup
	for i := 0; i < ctr.numClients; i++ {
		wg.Add(1)
		go ctr.operationWorker(ctx, &wg, i)
	}

	wg.Wait()
	return ctr.metrics.GetSummary()
}

// chaosInjector periodically applies chaos scenarios
func (ctr *ChaosTestRunner) chaosInjector(ctx context.Context) {
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Apply chaos to random clients
			for _, client := range ctr.clients {
				for _, scenario := range ctr.chaosScenarios {
					if float32(rand.Intn(1000))/1000.0 < scenario.Probability {
						scenario.ApplyFunc(client)

						// Schedule recovery
						if scenario.RecoverFunc != nil {
							go func(c *MockVaultClient, recover func(*MockVaultClient)) {
								time.Sleep(time.Duration(rand.Intn(200)) * time.Millisecond)
								recover(c)
							}(client, scenario.RecoverFunc)
						}
					}
				}
			}
		}
	}
}

// operationWorker performs operations on clients under chaos
func (ctr *ChaosTestRunner) operationWorker(ctx context.Context, wg *sync.WaitGroup, clientIndex int) {
	defer wg.Done()

	client := ctr.clients[clientIndex]

	for {
		select {
		case <-ctx.Done():
			return
		default:
			// Perform random operations
			operations := []string{"IsSealed", "HealthCheck", "GetSealStatus"}
			operation := operations[rand.Intn(len(operations))]

			ctr.metrics.StartConcurrentOperation()
			start := time.Now()
			var err error

			switch operation {
			case "IsSealed":
				_, err = client.IsSealed(ctx)
			case "HealthCheck":
				_, err = client.HealthCheck(ctx)
			case "GetSealStatus":
				_, err = client.GetSealStatus(ctx)
			}

			duration := time.Since(start)
			ctr.metrics.RecordOperation(operation, duration, err)
			ctr.metrics.EndConcurrentOperation()

			// Small delay between operations
			time.Sleep(time.Duration(rand.Intn(10)) * time.Millisecond)
		}
	}
}

// memoryMonitor periodically captures memory snapshots
func (ctr *ChaosTestRunner) memoryMonitor(ctx context.Context) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ctr.metrics.TakeMemorySnapshot()

			// Force GC occasionally to test memory cleanup
			if rand.Intn(10) == 0 {
				runtime.GC()
			}
		}
	}
}

// PropertyTestGenerator generates test data for property-based testing
type PropertyTestGenerator struct {
	rand *rand.Rand
}

// NewPropertyTestGenerator creates a new property test generator
func NewPropertyTestGenerator(seed int64) *PropertyTestGenerator {
	source := rand.NewSource(seed)
	return &PropertyTestGenerator{
		rand: rand.New(source),
	}
}

// GenerateRandomKeys generates random keys for testing
func (ptg *PropertyTestGenerator) GenerateRandomKeys(count int, minSize, maxSize int) []string {
	keys := make([]string, count)

	for i := 0; i < count; i++ {
		size := minSize + ptg.rand.Intn(maxSize-minSize+1)
		data := make([]byte, size)
		ptg.rand.Read(data)

		// Ensure non-zero data to avoid validation failures
		for j := range data {
			if data[j] == 0 {
				data[j] = byte(1 + ptg.rand.Intn(255))
			}
		}

		keys[i] = base64.StdEncoding.EncodeToString(data)
	}

	return keys
}

// GenerateInvalidKeys generates keys that should fail validation
func (ptg *PropertyTestGenerator) GenerateInvalidKeys(count int) []string {
	keys := make([]string, count)

	invalidPatterns := []func() string{
		// Invalid base64 characters
		func() string {
			return "invalid!@#$%^&*()"
		},
		// Empty key
		func() string {
			return ""
		},
		// Only padding
		func() string {
			return "===="
		},
		// Mixed valid/invalid
		func() string {
			valid := base64.StdEncoding.EncodeToString([]byte("valid"))
			return valid + "!@#"
		},
		// Very long invalid key
		func() string {
			invalid := make([]byte, 1000)
			for i := range invalid {
				invalid[i] = '!'
			}
			return string(invalid)
		},
	}

	for i := 0; i < count; i++ {
		pattern := invalidPatterns[ptg.rand.Intn(len(invalidPatterns))]
		keys[i] = pattern()
	}

	return keys
}

// GenerateEdgeCaseThresholds generates threshold values for edge case testing
func (ptg *PropertyTestGenerator) GenerateEdgeCaseThresholds() []int {
	return []int{
		-1000,  // Very negative
		-1,     // Negative
		0,      // Zero
		1,      // Minimum valid
		1000,   // Large positive
		int(^uint(0) >> 1), // Max int
	}
}

// SecurityTestHelper provides utilities for security-focused testing
type SecurityTestHelper struct {
	sensitivePatterns []string
}

// NewSecurityTestHelper creates a new security test helper
func NewSecurityTestHelper() *SecurityTestHelper {
	return &SecurityTestHelper{
		sensitivePatterns: []string{
			"password", "secret", "key", "token", "credential",
			"admin", "root", "auth", "login", "session",
			"/etc/passwd", "/proc/", "C:\\Windows\\",
			"127.0.0.1", "localhost", "192.168.", "10.0.0.",
		},
	}
}

// CheckErrorMessageSecurity verifies that error messages don't leak sensitive information
func (sth *SecurityTestHelper) CheckErrorMessageSecurity(err error) bool {
	if err == nil {
		return true
	}

	errMsg := strings.ToLower(err.Error())
	for _, pattern := range sth.sensitivePatterns {
		if strings.Contains(errMsg, strings.ToLower(pattern)) {
			return false
		}
	}

	return true
}

// GenerateMaliciousInputs creates inputs designed to test security boundaries
func (sth *SecurityTestHelper) GenerateMaliciousInputs() []string {
	return []string{
		// Buffer overflow attempts
		strings.Repeat("A", 100000),

		// Format string attacks
		"%s%s%s%s%s",
		"%x%x%x%x%x",

		// SQL injection (shouldn't affect base64, but test anyway)
		"'; DROP TABLE users; --",
		"1' OR '1'='1",

		// Script injection
		"<script>alert('xss')</script>",
		"javascript:alert('xss')",

		// Path traversal
		"../../../etc/passwd",
		"....//....//....//etc/passwd",

		// Null bytes and control characters
		string([]byte{0x00, 0x01, 0x02, 0x03}),
		"\x00\x01\x02\x03",

		// Unicode normalization attacks
		"A\u0300\u0301\u0302\u0303",

		// Very long strings
		strings.Repeat("malicious", 10000),

		// Binary data
		string([]byte{0xFF, 0xFE, 0xFD, 0xFC}),
	}
}

// TimingAnalyzer helps analyze timing attacks
type TimingAnalyzer struct {
	measurements []time.Duration
}

// NewTimingAnalyzer creates a new timing analyzer
func NewTimingAnalyzer() *TimingAnalyzer {
	return &TimingAnalyzer{
		measurements: make([]time.Duration, 0, 1000),
	}
}

// AddMeasurement records a timing measurement
func (ta *TimingAnalyzer) AddMeasurement(duration time.Duration) {
	ta.measurements = append(ta.measurements, duration)
}

// AnalyzeConstantTime checks if timing is consistent (constant-time property)
func (ta *TimingAnalyzer) AnalyzeConstantTime() TimingAnalysis {
	if len(ta.measurements) == 0 {
		return TimingAnalysis{}
	}

	var min, max, total time.Duration
	min = ta.measurements[0]
	max = ta.measurements[0]

	for _, d := range ta.measurements {
		total += d
		if d < min {
			min = d
		}
		if d > max {
			max = d
		}
	}

	avg := total / time.Duration(len(ta.measurements))
	variance := float64(max-min) / float64(avg)

	return TimingAnalysis{
		Count:    len(ta.measurements),
		Min:      min,
		Max:      max,
		Average:  avg,
		Variance: variance,
		IsConstantTime: variance < 2.0, // Threshold for constant-time
	}
}

// TimingAnalysis provides timing analysis results
type TimingAnalysis struct {
	Count          int
	Min            time.Duration
	Max            time.Duration
	Average        time.Duration
	Variance       float64
	IsConstantTime bool
}

// String returns a formatted string representation
func (ta TimingAnalysis) String() string {
	return fmt.Sprintf("Timing Analysis: Count=%d, Min=%v, Max=%v, Avg=%v, Variance=%.2f, ConstantTime=%t",
		ta.Count, ta.Min, ta.Max, ta.Average, ta.Variance, ta.IsConstantTime)
}
