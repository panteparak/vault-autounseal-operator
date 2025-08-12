package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/panteparak/vault-autounseal-operator/test/integration/framework"
	"github.com/panteparak/vault-autounseal-operator/test/integration/suites"
)

func main() {
	var (
		configPath = flag.String("config", "", "Path to test configuration file")
		scenario   = flag.String("scenario", "", "Test scenario to run (basic, failover, multi-vault, or all)")
		timeout    = flag.Duration("timeout", 30*time.Minute, "Test execution timeout")
		verbose    = flag.Bool("verbose", false, "Enable verbose logging")
	)
	flag.Parse()

	// Create context with timeout
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()

	// Initialize test framework
	fmt.Println("🚀 Initializing Vault Auto-Unseal Operator Integration Test Framework")

	testFramework, err := framework.NewTestFramework(ctx, *configPath)
	if err != nil {
		fmt.Printf("❌ Failed to initialize test framework: %v\n", err)
		os.Exit(1)
	}

	// Ensure cleanup happens
	defer func() {
		fmt.Println("🧹 Cleaning up test framework...")
		if err := testFramework.Cleanup(); err != nil {
			fmt.Printf("⚠️ Warning: cleanup failed: %v\n", err)
		}
	}()

	// Load test scenarios
	scenarios, err := framework.LoadTestScenarios("")
	if err != nil {
		fmt.Printf("❌ Failed to load test scenarios: %v\n", err)
		os.Exit(1)
	}

	// Filter scenarios if specific scenario requested
	if *scenario != "" && *scenario != "all" {
		scenarios = filterScenarios(scenarios, *scenario)
		if len(scenarios) == 0 {
			fmt.Printf("❌ No scenarios found matching: %s\n", *scenario)
			os.Exit(1)
		}
	}

	// Run test scenarios
	allPassed := true
	totalResults := make([]*framework.TestResult, 0)

	for _, scenarioConfig := range scenarios {
		fmt.Printf("\n📋 Running scenario: %s - %s\n", scenarioConfig.Name, scenarioConfig.Description)

		// Create test suite for this scenario
		testSuite := suites.NewIntegrationTestSuite(scenarioConfig.Name, scenarioConfig.VaultSetup)

		// Run the test suite
		results, err := testFramework.RunTestSuite(testSuite)
		if err != nil {
			fmt.Printf("❌ Failed to run test suite for scenario %s: %v\n", scenarioConfig.Name, err)
			allPassed = false
			continue
		}

		// Collect results
		totalResults = append(totalResults, results...)

		// Check if any tests failed
		scenarioPassed := true
		for _, result := range results {
			if !result.Success {
				scenarioPassed = false
				allPassed = false
			}
		}

		if scenarioPassed {
			fmt.Printf("✅ Scenario %s completed successfully\n", scenarioConfig.Name)
		} else {
			fmt.Printf("❌ Scenario %s had failures\n", scenarioConfig.Name)
		}

		// Print detailed results if verbose
		if *verbose {
			printDetailedResults(results)
		}
	}

	// Print summary
	printSummary(totalResults, allPassed)

	// Generate test report
	if testFramework.Config.TestSettings.GenerateReports {
		if err := testFramework.Reporter.GenerateReport(); err != nil {
			fmt.Printf("⚠️ Warning: failed to generate test report: %v\n", err)
		} else {
			fmt.Println("📊 Test report generated successfully")
		}
	}

	if !allPassed {
		os.Exit(1)
	}

	fmt.Println("🎉 All integration tests passed!")
}

func filterScenarios(scenarios []framework.TestScenario, targetScenario string) []framework.TestScenario {
	var filtered []framework.TestScenario
	for _, scenario := range scenarios {
		if scenario.Name == targetScenario {
			filtered = append(filtered, scenario)
		}
	}
	return filtered
}

func printDetailedResults(results []*framework.TestResult) {
	fmt.Println("\n📝 Detailed Test Results:")
	fmt.Println(strings.Repeat("=", 60))

	for _, result := range results {
		status := "✅ PASS"
		if !result.Success {
			status = "❌ FAIL"
		}

		fmt.Printf("%s %s (Duration: %v)\n", status, result.TestName, result.Duration)

		if result.Error != nil {
			fmt.Printf("   Error: %v\n", result.Error)
		}

		// Print key details
		if len(result.Details) > 0 {
			fmt.Println("   Details:")
			for key, value := range result.Details {
				fmt.Printf("     %s: %v\n", key, value)
			}
		}

		// Print recent logs if there are failures
		if !result.Success && len(result.Logs) > 0 {
			fmt.Println("   Recent Logs:")
			logCount := len(result.Logs)
			start := 0
			if logCount > 5 {
				start = logCount - 5 // Show last 5 logs
			}
			for i := start; i < logCount; i++ {
				fmt.Printf("     %s\n", result.Logs[i])
			}
		}

		fmt.Println()
	}
}

func printSummary(results []*framework.TestResult, allPassed bool) {
	fmt.Println("\n📊 Test Summary:")
	fmt.Println(strings.Repeat("=", 40))

	totalTests := len(results)
	passedTests := 0
	failedTests := 0
	var totalDuration time.Duration

	for _, result := range results {
		totalDuration += result.Duration
		if result.Success {
			passedTests++
		} else {
			failedTests++
		}
	}

	fmt.Printf("Total Tests: %d\n", totalTests)
	fmt.Printf("Passed: %d\n", passedTests)
	fmt.Printf("Failed: %d\n", failedTests)
	fmt.Printf("Total Duration: %v\n", totalDuration)
	fmt.Printf("Average Duration: %v\n", totalDuration/time.Duration(totalTests))

	if allPassed {
		fmt.Println("🎯 Overall Result: ALL TESTS PASSED")
	} else {
		fmt.Println("💥 Overall Result: SOME TESTS FAILED")
	}
}
