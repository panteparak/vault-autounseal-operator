package framework

import (
	"context"
	"fmt"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

// NewTestFramework creates a new test framework instance
func NewTestFramework(ctx context.Context, configPath string) (*TestFramework, error) {
	// Load test configuration
	testConfig, err := LoadTestConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to load test config: %w", err)
	}

	// Get Kubernetes configuration
	kubeConfig, err := config.GetConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to get kubernetes config: %w", err)
	}

	// Create Kubernetes client
	kubeClient, err := client.New(kubeConfig, client.Options{})
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Create Kubernetes clientset
	kubernetesClient, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create kubernetes clientset: %w", err)
	}

	// Create infrastructure manager
	infrastructure, err := NewDockerInfrastructure(testConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create infrastructure manager: %w", err)
	}

	// Create test reporter
	reporter := NewJSONReporter(testConfig.TestSettings.GenerateReports)

	framework := &TestFramework{
		KubeClient:       kubeClient,
		KubernetesClient: kubernetesClient,
		Context:          ctx,
		Namespace:        "vault-operator-system",
		Config:           testConfig,
		Infrastructure:   infrastructure,
		Reporter:         reporter,
	}

	return framework, nil
}

// RunTestSuite executes a test suite with the framework
func (f *TestFramework) RunTestSuite(suite TestSuite) ([]*TestResult, error) {
	var results []*TestResult

	f.Reporter.StartSuite(suite)
	defer func() {
		f.Reporter.EndSuite(suite, results)
	}()

	// Setup suite
	if err := suite.Setup(f.Context, f); err != nil {
		return nil, fmt.Errorf("failed to setup test suite %s: %w", suite.Name(), err)
	}

	// Ensure cleanup happens
	defer func() {
		if err := suite.Teardown(f.Context, f); err != nil {
			fmt.Printf("Warning: failed to teardown test suite %s: %v\n", suite.Name(), err)
		}
	}()

	testCases := suite.TestCases()

	if f.Config.TestSettings.Parallel {
		results = f.runTestCasesParallel(testCases)
	} else {
		results = f.runTestCasesSequential(testCases)
	}

	return results, nil
}

// runTestCasesSequential executes test cases one by one
func (f *TestFramework) runTestCasesSequential(testCases []TestCase) []*TestResult {
	var results []*TestResult

	for _, testCase := range testCases {
		result := f.executeTestCase(testCase)
		results = append(results, result)

		// Stop on first failure if fail-fast is enabled
		if f.Config.TestSettings.FailFast && !result.Success {
			break
		}
	}

	return results
}

// runTestCasesParallel executes test cases in parallel
func (f *TestFramework) runTestCasesParallel(testCases []TestCase) []*TestResult {
	maxConcurrency := f.Config.TestSettings.MaxConcurrency
	if maxConcurrency <= 0 {
		maxConcurrency = len(testCases)
	}

	sem := make(chan struct{}, maxConcurrency)
	resultChan := make(chan *TestResult, len(testCases))

	// Start all test cases
	for _, testCase := range testCases {
		go func(tc TestCase) {
			sem <- struct{}{}        // Acquire semaphore
			defer func() { <-sem }() // Release semaphore

			result := f.executeTestCase(tc)
			resultChan <- result
		}(testCase)
	}

	// Collect results
	var results []*TestResult
	for i := 0; i < len(testCases); i++ {
		result := <-resultChan
		results = append(results, result)
	}

	return results
}

// executeTestCase executes a single test case and handles cleanup
func (f *TestFramework) executeTestCase(testCase TestCase) *TestResult {
	f.Reporter.StartTest(testCase)

	start := time.Now()
	result := &TestResult{
		TestName:  testCase.Name(),
		Timestamp: start,
		Details:   make(map[string]interface{}),
		Logs:      make([]string, 0),
	}

	defer func() {
		result.Duration = time.Since(start)
		f.Reporter.EndTest(testCase, result)
	}()

	// Check prerequisites
	if err := f.checkPrerequisites(testCase.Prerequisites()); err != nil {
		result.Success = false
		result.Error = fmt.Errorf("prerequisites not met: %w", err)
		return result
	}

	// Execute test case
	result = testCase.Execute(f.Context, f)
	result.Duration = time.Since(start)

	// Cleanup test case resources
	if err := testCase.Cleanup(f.Context, f); err != nil {
		fmt.Printf("Warning: failed to cleanup test case %s: %v\n", testCase.Name(), err)
		// Don't fail the test due to cleanup issues unless it's a critical failure
	}

	return result
}

// checkPrerequisites validates that all prerequisites are met
func (f *TestFramework) checkPrerequisites(prerequisites []string) error {
	for _, prereq := range prerequisites {
		if err := f.validatePrerequisite(prereq); err != nil {
			return fmt.Errorf("prerequisite %s not met: %w", prereq, err)
		}
	}
	return nil
}

// validatePrerequisite checks a specific prerequisite
func (f *TestFramework) validatePrerequisite(prerequisite string) error {
	switch prerequisite {
	case "vault-available":
		return f.checkVaultAvailability()
	case "operator-deployed":
		return f.checkOperatorDeployment()
	case "kubernetes-ready":
		return f.checkKubernetesReady()
	default:
		return fmt.Errorf("unknown prerequisite: %s", prerequisite)
	}
}

// checkVaultAvailability verifies that Vault instances are available
func (f *TestFramework) checkVaultAvailability() error {
	// Implementation to check if Vault instances are running and accessible
	return nil
}

// checkOperatorDeployment verifies that the operator is deployed and ready
func (f *TestFramework) checkOperatorDeployment() error {
	// Implementation to check if the operator is deployed and ready
	return nil
}

// checkKubernetesReady verifies that Kubernetes cluster is ready
func (f *TestFramework) checkKubernetesReady() error {
	// Implementation to check if Kubernetes cluster is ready
	return nil
}

// Cleanup performs framework-level cleanup
func (f *TestFramework) Cleanup() error {
	if f.Infrastructure != nil {
		return f.Infrastructure.Cleanup(f.Context)
	}
	return nil
}

// GetVaultInstance retrieves a Vault instance by name
func (f *TestFramework) GetVaultInstance(name string) (*VaultInstance, error) {
	// This would be implemented to track and retrieve Vault instances
	// For now, return a placeholder
	return nil, fmt.Errorf("vault instance %s not found", name)
}

// WaitForCondition waits for a condition to be true with timeout
func (f *TestFramework) WaitForCondition(condition func() bool, timeout time.Duration, message string) error {
	ctx, cancel := context.WithTimeout(f.Context, timeout)
	defer cancel()

	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for condition: %s", message)
		case <-ticker.C:
			if condition() {
				return nil
			}
		}
	}
}
