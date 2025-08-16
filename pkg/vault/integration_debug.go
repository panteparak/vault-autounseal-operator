package vault

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// DebugLevel represents the verbosity level for debugging
type DebugLevel int

const (
	DebugLevelQuiet DebugLevel = iota
	DebugLevelBasic
	DebugLevelVerbose
	DebugLevelTrace
)

func (d DebugLevel) String() string {
	switch d {
	case DebugLevelQuiet:
		return "QUIET"
	case DebugLevelBasic:
		return "BASIC"
	case DebugLevelVerbose:
		return "VERBOSE"
	case DebugLevelTrace:
		return "TRACE"
	default:
		return "UNKNOWN"
	}
}

// TestEvent represents a significant event during testing
type TestEvent struct {
	Timestamp   time.Time              `json:"timestamp"`
	Level       string                 `json:"level"`
	Event       string                 `json:"event"`
	TestName    string                 `json:"test_name,omitempty"`
	Duration    time.Duration          `json:"duration,omitempty"`
	Error       string                 `json:"error,omitempty"`
	Metadata    map[string]interface{} `json:"metadata,omitempty"`
	ClientStats map[string]interface{} `json:"client_stats,omitempty"`
}

// TestLogger provides structured logging for integration tests
type TestLogger struct {
	level      DebugLevel
	events     []TestEvent
	mutex      sync.RWMutex
	outputFile *os.File
}

// NewTestLogger creates a new test logger
func NewTestLogger(level DebugLevel) *TestLogger {
	logger := &TestLogger{
		level:  level,
		events: make([]TestEvent, 0),
	}

	// Try to open debug log file
	if logFile := os.Getenv("INTEGRATION_DEBUG_LOG"); logFile != "" {
		if file, err := os.OpenFile(logFile, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600); err == nil { //nolint:gosec // Debug log file path from environment
			logger.outputFile = file
		}
	}

	return logger
}

// Close closes the logger and any open files
func (tl *TestLogger) Close() error {
	if tl.outputFile != nil {
		return tl.outputFile.Close()
	}
	return nil
}

// Log records an event if it meets the current debug level
func (tl *TestLogger) Log(level DebugLevel, event, testName string, metadata map[string]interface{}) {
	if level > tl.level {
		return
	}

	tl.mutex.Lock()
	defer tl.mutex.Unlock()

	testEvent := TestEvent{
		Timestamp: time.Now(),
		Level:     level.String(),
		Event:     event,
		TestName:  testName,
		Metadata:  metadata,
	}

	tl.events = append(tl.events, testEvent)

	// Output to console if verbose enough
	if tl.level >= DebugLevelVerbose {
		fmt.Printf("[%s] %s: %s", testEvent.Timestamp.Format("15:04:05.000"), testEvent.Level, testEvent.Event)
		if testEvent.TestName != "" {
			fmt.Printf(" (test: %s)", testEvent.TestName)
		}
		if len(testEvent.Metadata) > 0 {
			fmt.Printf(" - %v", testEvent.Metadata)
		}
		fmt.Println()
	}

	// Write to file if available
	if tl.outputFile != nil {
		if jsonBytes, err := json.Marshal(testEvent); err == nil {
			_, _ = tl.outputFile.WriteString(string(jsonBytes) + "\n")
		}
	}
}

// LogError records an error event
func (tl *TestLogger) LogError(testName string, err error, metadata map[string]interface{}) {
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	metadata["error"] = err.Error()

	tl.Log(DebugLevelBasic, "ERROR", testName, metadata)
}

// LogTiming records timing information
func (tl *TestLogger) LogTiming(testName string, operation string, duration time.Duration, metadata map[string]interface{}) {
	if metadata == nil {
		metadata = make(map[string]interface{})
	}
	metadata["operation"] = operation
	metadata["duration_ms"] = duration.Milliseconds()

	tl.Log(DebugLevelVerbose, "TIMING", testName, metadata)
}

// LogStats records statistics
func (tl *TestLogger) LogStats(testName string, stats map[string]interface{}) {
	tl.Log(DebugLevelVerbose, "STATS", testName, stats)
}

// GetEvents returns all recorded events
func (tl *TestLogger) GetEvents() []TestEvent {
	tl.mutex.RLock()
	defer tl.mutex.RUnlock()

	// Return copy
	events := make([]TestEvent, len(tl.events))
	copy(events, tl.events)
	return events
}

// GenerateReport creates a comprehensive debug report
func (tl *TestLogger) GenerateReport() string {
	tl.mutex.RLock()
	defer tl.mutex.RUnlock()

	var report strings.Builder
	report.WriteString("=== Integration Test Debug Report ===\n\n")

	// Summary
	totalEvents := len(tl.events)
	errorCount := 0
	testsByName := make(map[string][]TestEvent)

	for _, event := range tl.events {
		if event.Event == "ERROR" {
			errorCount++
		}

		if event.TestName != "" {
			testsByName[event.TestName] = append(testsByName[event.TestName], event)
		}
	}

	report.WriteString(fmt.Sprintf("Total Events: %d\n", totalEvents))
	report.WriteString(fmt.Sprintf("Errors: %d\n", errorCount))
	report.WriteString(fmt.Sprintf("Tests: %d\n", len(testsByName)))
	report.WriteString("\n")

	// Error summary
	if errorCount > 0 {
		report.WriteString("=== ERRORS ===\n")
		for _, event := range tl.events {
			if event.Event == "ERROR" {
				report.WriteString(fmt.Sprintf("[%s] %s: %s\n",
					event.Timestamp.Format("15:04:05.000"),
					event.TestName,
					event.Metadata["error"]))
			}
		}
		report.WriteString("\n")
	}

	// Timing analysis
	report.WriteString("=== TIMING ANALYSIS ===\n")
	var testNames []string
	for testName := range testsByName {
		testNames = append(testNames, testName)
	}
	sort.Strings(testNames)

	for _, testName := range testNames {
		events := testsByName[testName]

		var totalDuration time.Duration
		var operations []string

		for _, event := range events {
			if event.Event == "TIMING" {
				if duration, ok := event.Metadata["duration_ms"].(int64); ok {
					totalDuration += time.Duration(duration) * time.Millisecond
				}
				if op, ok := event.Metadata["operation"].(string); ok {
					operations = append(operations, op)
				}
			}
		}

		report.WriteString(fmt.Sprintf("%s: %v (%d operations)\n",
			testName, totalDuration, len(operations)))

		if tl.level >= DebugLevelTrace {
			for _, op := range operations {
				report.WriteString(fmt.Sprintf("  - %s\n", op))
			}
		}
	}
	report.WriteString("\n")

	// Detailed timeline (only for trace level)
	if tl.level >= DebugLevelTrace {
		report.WriteString("=== DETAILED TIMELINE ===\n")
		for _, event := range tl.events {
			report.WriteString(fmt.Sprintf("[%s] %s:%s",
				event.Timestamp.Format("15:04:05.000"),
				event.Level,
				event.Event))

			if event.TestName != "" {
				report.WriteString(fmt.Sprintf(" (%s)", event.TestName))
			}

			if len(event.Metadata) > 0 {
				if jsonBytes, err := json.Marshal(event.Metadata); err == nil {
					report.WriteString(fmt.Sprintf(" %s", string(jsonBytes)))
				}
			}
			report.WriteString("\n")
		}
	}

	return report.String()
}

// EnhancedIntegrationTestRunner extends the basic runner with debugging capabilities
type EnhancedIntegrationTestRunner struct {
	*IntegrationTestRunner
	logger        *TestLogger
	debugLevel    DebugLevel
	testStartTime map[string]time.Time
	mutex         sync.RWMutex
}

// NewEnhancedIntegrationTestRunner creates a new enhanced runner with debugging
func NewEnhancedIntegrationTestRunner(config *IntegrationTestConfig, debugLevel DebugLevel) *EnhancedIntegrationTestRunner {
	baseRunner := NewIntegrationTestRunner(config)

	return &EnhancedIntegrationTestRunner{
		IntegrationTestRunner: baseRunner,
		logger:                NewTestLogger(debugLevel),
		debugLevel:            debugLevel,
		testStartTime:         make(map[string]time.Time),
	}
}

// Close cleans up the enhanced runner
func (eitr *EnhancedIntegrationTestRunner) Close() error {
	return eitr.logger.Close()
}

// RunTestWithDebug executes a test with comprehensive debugging
func (eitr *EnhancedIntegrationTestRunner) RunTestWithDebug(ctx context.Context, testName string, testFunc func(context.Context) error) error {
	eitr.mutex.Lock()
	eitr.testStartTime[testName] = time.Now()
	eitr.mutex.Unlock()

	eitr.logger.Log(DebugLevelVerbose, "TEST_START", testName, map[string]interface{}{
		"start_time": time.Now(),
		"config":     eitr.config,
	})

	// Log pre-test health status
	if eitr.debugLevel >= DebugLevelVerbose {
		healthyClients := eitr.healthChecker.GetHealthyClients()
		eitr.logger.LogStats(testName, map[string]interface{}{
			"healthy_clients": healthyClients,
			"circuit_state":   eitr.circuitBreaker.GetState().String(),
		})
	}

	// Enhanced test execution with timing
	start := time.Now()
	err := eitr.IntegrationTestRunner.RunTest(ctx, testName, func(testCtx context.Context) error {
		deadline, hasDeadline := testCtx.Deadline()
		eitr.logger.Log(DebugLevelTrace, "TEST_EXECUTE", testName, map[string]interface{}{
			"context_deadline": deadline,
			"has_deadline":     hasDeadline,
		})

		return testFunc(testCtx)
	})
	duration := time.Since(start)

	// Log results
	if err != nil {
		eitr.logger.LogError(testName, err, map[string]interface{}{
			"duration_ms": duration.Milliseconds(),
			"failed":      true,
		})
		eitr.logger.Log(DebugLevelBasic, "TEST_FAILED", testName, map[string]interface{}{
			"duration": duration,
			"error":    err.Error(),
		})
	} else {
		eitr.logger.LogTiming(testName, "test_execution", duration, map[string]interface{}{
			"success": true,
		})
		eitr.logger.Log(DebugLevelVerbose, "TEST_PASSED", testName, map[string]interface{}{
			"duration": duration,
		})
	}

	// Log post-test stats
	if eitr.debugLevel >= DebugLevelVerbose {
		stats := eitr.GetStats()
		eitr.logger.LogStats(testName, stats)
	}

	return err
}

// RunScenariosWithDebug executes scenarios with enhanced debugging
func (eitr *EnhancedIntegrationTestRunner) RunScenariosWithDebug(ctx context.Context, scenarios []TestScenario) error {
	eitr.logger.Log(DebugLevelBasic, "SCENARIOS_START", "", map[string]interface{}{
		"scenario_count": len(scenarios),
		"scenarios":      extractScenarioNames(scenarios),
	})

	overallStart := time.Now()

	for i, scenario := range scenarios {
		eitr.logger.Log(DebugLevelVerbose, "SCENARIO_START", scenario.Name, map[string]interface{}{
			"index":       i,
			"description": scenario.Description,
			"timeout":     scenario.Timeout,
		})

		// Check circuit breaker state before each scenario
		circuitState := eitr.circuitBreaker.GetState()
		if circuitState == CircuitOpen {
			err := fmt.Errorf("circuit breaker is OPEN - failing fast on scenario: %s", scenario.Name)
			eitr.logger.LogError(scenario.Name, err, map[string]interface{}{
				"circuit_state":  circuitState.String(),
				"scenario_index": i,
			})
			return err
		}

		// Execute scenario with debugging
		scenarioStart := time.Now()

		scenarioCtx := ctx
		if scenario.Timeout > 0 {
			var cancel context.CancelFunc
			scenarioCtx, cancel = context.WithTimeout(ctx, scenario.Timeout)
			defer cancel()
		}

		// Setup phase
		if scenario.Setup != nil {
			eitr.logger.Log(DebugLevelTrace, "SCENARIO_SETUP", scenario.Name, nil)
			setupStart := time.Now()
			if err := scenario.Setup(scenarioCtx); err != nil {
				setupDuration := time.Since(setupStart)
				eitr.logger.LogError(scenario.Name, err, map[string]interface{}{
					"phase":       "setup",
					"duration_ms": setupDuration.Milliseconds(),
				})
				return fmt.Errorf("scenario %s setup failed: %w", scenario.Name, err)
			}
			eitr.logger.LogTiming(scenario.Name, "setup", time.Since(setupStart), nil)
		}

		// Execute main test
		eitr.logger.Log(DebugLevelTrace, "SCENARIO_EXECUTE", scenario.Name, nil)
		executeStart := time.Now()
		err := eitr.RunTestWithDebug(scenarioCtx, scenario.Name, scenario.Execute)
		executeDuration := time.Since(executeStart)

		// Cleanup phase (always run, even on failure)
		if scenario.Cleanup != nil {
			eitr.logger.Log(DebugLevelTrace, "SCENARIO_CLEANUP", scenario.Name, nil)
			cleanupStart := time.Now()
			if cleanupErr := scenario.Cleanup(scenarioCtx); cleanupErr != nil {
				cleanupDuration := time.Since(cleanupStart)
				eitr.logger.LogError(scenario.Name, cleanupErr, map[string]interface{}{
					"phase":       "cleanup",
					"duration_ms": cleanupDuration.Milliseconds(),
				})
				if err == nil {
					err = fmt.Errorf("scenario %s cleanup failed: %w", scenario.Name, cleanupErr)
				}
			} else {
				eitr.logger.LogTiming(scenario.Name, "cleanup", time.Since(cleanupStart), nil)
			}
		}

		scenarioDuration := time.Since(scenarioStart)

		// Check if error matches expectation
		if scenario.ExpectError && err == nil {
			err = fmt.Errorf("scenario %s expected error but succeeded", scenario.Name)
			eitr.logger.LogError(scenario.Name, err, map[string]interface{}{
				"expected_error": true,
				"duration_ms":    scenarioDuration.Milliseconds(),
			})
			return err
		}
		if !scenario.ExpectError && err != nil {
			eitr.logger.LogError(scenario.Name, err, map[string]interface{}{
				"expected_success": true,
				"duration_ms":      scenarioDuration.Milliseconds(),
			})
			return fmt.Errorf("scenario %s failed: %w", scenario.Name, err)
		}

		// Log scenario completion
		eitr.logger.Log(DebugLevelVerbose, "SCENARIO_COMPLETE", scenario.Name, map[string]interface{}{
			"index":        i,
			"duration":     scenarioDuration,
			"execute_time": executeDuration,
			"success":      err == nil,
		})
	}

	overallDuration := time.Since(overallStart)
	eitr.logger.Log(DebugLevelBasic, "SCENARIOS_COMPLETE", "", map[string]interface{}{
		"total_duration": overallDuration,
		"scenario_count": len(scenarios),
		"success":        true,
	})

	return nil
}

// PrintDebugReport outputs the debug report to console
func (eitr *EnhancedIntegrationTestRunner) PrintDebugReport() {
	if eitr.debugLevel >= DebugLevelBasic {
		fmt.Println("\n" + eitr.logger.GenerateReport())
	}
}

// GetLogger returns the internal logger for external use
func (eitr *EnhancedIntegrationTestRunner) GetLogger() *TestLogger {
	return eitr.logger
}

// Helper function to extract scenario names
func extractScenarioNames(scenarios []TestScenario) []string {
	names := make([]string, len(scenarios))
	for i, scenario := range scenarios {
		names[i] = scenario.Name
	}
	return names
}

// DebugConfig returns debug configuration from environment
func DebugConfig() DebugLevel {
	debugEnv := os.Getenv("INTEGRATION_DEBUG")
	switch strings.ToUpper(debugEnv) {
	case "TRACE":
		return DebugLevelTrace
	case "VERBOSE":
		return DebugLevelVerbose
	case "BASIC":
		return DebugLevelBasic
	default:
		return DebugLevelQuiet
	}
}
