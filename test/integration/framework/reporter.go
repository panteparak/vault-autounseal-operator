package framework

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// JSONReporter implements TestReporter interface for JSON output
type JSONReporter struct {
	generateReports bool
	results        []*TestResult
	suiteResults   []SuiteResult
	startTime      time.Time
}

// SuiteResult holds results for a complete test suite
type SuiteResult struct {
	SuiteName   string         `json:"suiteName"`
	StartTime   time.Time      `json:"startTime"`
	EndTime     time.Time      `json:"endTime"`
	Duration    time.Duration  `json:"duration"`
	TestResults []*TestResult  `json:"testResults"`
	Success     bool           `json:"success"`
}

// TestReport holds the complete test report
type TestReport struct {
	GeneratedAt   time.Time     `json:"generatedAt"`
	TotalDuration time.Duration `json:"totalDuration"`
	Summary       TestSummary   `json:"summary"`
	Suites        []SuiteResult `json:"suites"`
	Environment   map[string]string `json:"environment"`
}

// TestSummary provides high-level test execution summary
type TestSummary struct {
	TotalTests   int `json:"totalTests"`
	PassedTests  int `json:"passedTests"`
	FailedTests  int `json:"failedTests"`
	SkippedTests int `json:"skippedTests"`
	SuccessRate  float64 `json:"successRate"`
}

// NewJSONReporter creates a new JSON reporter
func NewJSONReporter(generateReports bool) TestReporter {
	return &JSONReporter{
		generateReports: generateReports,
		results:        make([]*TestResult, 0),
		suiteResults:   make([]SuiteResult, 0),
		startTime:      time.Now(),
	}
}

// StartSuite is called when a test suite starts
func (r *JSONReporter) StartSuite(suite TestSuite) {
	fmt.Printf("🚀 Starting test suite: %s - %s\n", suite.Name(), suite.Description())
}

// StartTest is called when an individual test starts
func (r *JSONReporter) StartTest(testCase TestCase) {
	fmt.Printf("   ▶️ Running test: %s\n", testCase.Name())
}

// EndTest is called when an individual test completes
func (r *JSONReporter) EndTest(testCase TestCase, result *TestResult) {
	status := "✅ PASS"
	if !result.Success {
		status = "❌ FAIL"
	}

	fmt.Printf("   %s %s (Duration: %v)\n", status, testCase.Name(), result.Duration)

	if !result.Success && result.Error != nil {
		fmt.Printf("      Error: %v\n", result.Error)
	}

	r.results = append(r.results, result)
}

// EndSuite is called when a test suite completes
func (r *JSONReporter) EndSuite(suite TestSuite, results []*TestResult) {
	endTime := time.Now()
	duration := endTime.Sub(r.startTime)

	success := true
	for _, result := range results {
		if !result.Success {
			success = false
			break
		}
	}

	suiteResult := SuiteResult{
		SuiteName:   suite.Name(),
		StartTime:   r.startTime,
		EndTime:     endTime,
		Duration:    duration,
		TestResults: results,
		Success:     success,
	}

	r.suiteResults = append(r.suiteResults, suiteResult)

	status := "✅ PASSED"
	if !success {
		status = "❌ FAILED"
	}

	fmt.Printf("📊 Suite %s completed: %s (Duration: %v)\n", suite.Name(), status, duration)
}

// GenerateReport generates the final test report
func (r *JSONReporter) GenerateReport() error {
	if !r.generateReports {
		return nil
	}

	// Create reports directory if it doesn't exist
	reportsDir := "test-reports"
	if err := os.MkdirAll(reportsDir, 0755); err != nil {
		return fmt.Errorf("failed to create reports directory: %w", err)
	}

	// Generate test report
	report := r.buildTestReport()

	// Write JSON report
	jsonFilePath := filepath.Join(reportsDir, fmt.Sprintf("integration-test-report-%s.json",
		time.Now().Format("2006-01-02-15-04-05")))

	if err := r.writeJSONReport(report, jsonFilePath); err != nil {
		return fmt.Errorf("failed to write JSON report: %w", err)
	}

	// Write HTML report
	htmlFilePath := filepath.Join(reportsDir, fmt.Sprintf("integration-test-report-%s.html",
		time.Now().Format("2006-01-02-15-04-05")))

	if err := r.writeHTMLReport(report, htmlFilePath); err != nil {
		return fmt.Errorf("failed to write HTML report: %w", err)
	}

	fmt.Printf("📄 Test reports generated:\n")
	fmt.Printf("   JSON: %s\n", jsonFilePath)
	fmt.Printf("   HTML: %s\n", htmlFilePath)

	return nil
}

// buildTestReport constructs the complete test report
func (r *JSONReporter) buildTestReport() *TestReport {
	totalTests := len(r.results)
	passedTests := 0
	failedTests := 0

	for _, result := range r.results {
		if result.Success {
			passedTests++
		} else {
			failedTests++
		}
	}

	successRate := 0.0
	if totalTests > 0 {
		successRate = float64(passedTests) / float64(totalTests) * 100
	}

	return &TestReport{
		GeneratedAt:   time.Now(),
		TotalDuration: time.Since(r.startTime),
		Summary: TestSummary{
			TotalTests:   totalTests,
			PassedTests:  passedTests,
			FailedTests:  failedTests,
			SkippedTests: 0, // Not implemented yet
			SuccessRate:  successRate,
		},
		Suites: r.suiteResults,
		Environment: map[string]string{
			"go_version": "1.21",
			"os":         "kubernetes",
		},
	}
}

// writeJSONReport writes the test report as JSON
func (r *JSONReporter) writeJSONReport(report *TestReport, filepath string) error {
	file, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")

	return encoder.Encode(report)
}

// writeHTMLReport writes the test report as HTML
func (r *JSONReporter) writeHTMLReport(report *TestReport, filepath string) error {
	file, err := os.Create(filepath)
	if err != nil {
		return err
	}
	defer file.Close()

	html := r.generateHTMLReport(report)
	_, err = file.WriteString(html)

	return err
}

// generateHTMLReport generates HTML content for the test report
func (r *JSONReporter) generateHTMLReport(report *TestReport) string {
	html := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Vault Auto-Unseal Operator Integration Test Report</title>
    <style>
        body { font-family: Arial, sans-serif; margin: 20px; }
        .header { background-color: #f5f5f5; padding: 20px; border-radius: 5px; }
        .summary { display: flex; gap: 20px; margin: 20px 0; }
        .metric { background-color: #e9f5e9; padding: 15px; border-radius: 5px; text-align: center; }
        .metric.failed { background-color: #ffe9e9; }
        .suite { margin: 20px 0; border: 1px solid #ddd; border-radius: 5px; }
        .suite-header { background-color: #f0f0f0; padding: 10px; font-weight: bold; }
        .test-case { padding: 10px; border-bottom: 1px solid #eee; }
        .test-case.passed { border-left: 4px solid #4CAF50; }
        .test-case.failed { border-left: 4px solid #f44336; }
        .details { background-color: #f9f9f9; padding: 10px; margin-top: 10px; border-radius: 3px; }
        .logs { background-color: #f5f5f5; padding: 10px; font-family: monospace; white-space: pre-wrap; }
    </style>
</head>
<body>
    <div class="header">
        <h1>🏛️ Vault Auto-Unseal Operator Integration Test Report</h1>
        <p>Generated: %s</p>
        <p>Total Duration: %v</p>
    </div>

    <div class="summary">
        <div class="metric">
            <h3>%d</h3>
            <p>Total Tests</p>
        </div>
        <div class="metric">
            <h3>%d</h3>
            <p>Passed</p>
        </div>
        <div class="metric %s">
            <h3>%d</h3>
            <p>Failed</p>
        </div>
        <div class="metric">
            <h3>%.1f%%</h3>
            <p>Success Rate</p>
        </div>
    </div>`,
		report.GeneratedAt.Format("2006-01-02 15:04:05"),
		report.TotalDuration,
		report.Summary.TotalTests,
		report.Summary.PassedTests,
		r.getFailedClass(report.Summary.FailedTests),
		report.Summary.FailedTests,
		report.Summary.SuccessRate)

	// Add test suites
	for _, suite := range report.Suites {
		html += fmt.Sprintf(`
    <div class="suite">
        <div class="suite-header">
            📋 %s (Duration: %v)
        </div>`, suite.SuiteName, suite.Duration)

		// Add test cases
		for _, testResult := range suite.TestResults {
			status := "passed"
			icon := "✅"
			if !testResult.Success {
				status = "failed"
				icon = "❌"
			}

			html += fmt.Sprintf(`
        <div class="test-case %s">
            <h4>%s %s</h4>
            <p>Duration: %v</p>`, status, icon, testResult.TestName, testResult.Duration)

			if testResult.Error != nil {
				html += fmt.Sprintf(`
            <div class="details">
                <strong>Error:</strong> %s
            </div>`, testResult.Error.Error())
			}

			if len(testResult.Details) > 0 {
				html += `<div class="details"><strong>Details:</strong><ul>`
				for key, value := range testResult.Details {
					html += fmt.Sprintf(`<li><strong>%s:</strong> %v</li>`, key, value)
				}
				html += `</ul></div>`
			}

			if len(testResult.Logs) > 0 {
				html += `<div class="details"><strong>Logs:</strong><div class="logs">`
				for _, log := range testResult.Logs {
					html += fmt.Sprintf("%s\n", log)
				}
				html += `</div></div>`
			}

			html += `</div>`
		}

		html += `</div>`
	}

	html += `
</body>
</html>`

	return html
}

// getFailedClass returns the CSS class for the failed metric
func (r *JSONReporter) getFailedClass(failedCount int) string {
	if failedCount > 0 {
		return "failed"
	}
	return ""
}
