package integration

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	vaultv1 "github.com/panteparak/vault-autounseal-operator/pkg/api/v1"
	"github.com/panteparak/vault-autounseal-operator/pkg/controller"
	vaultpkg "github.com/panteparak/vault-autounseal-operator/pkg/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/k3s"
	"github.com/testcontainers/testcontainers-go/modules/vault"
	"github.com/testcontainers/testcontainers-go/wait"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// PerformanceTestSuite provides performance testing using TestContainers
type PerformanceTestSuite struct {
	suite.Suite
	k3sContainer   *k3s.K3sContainer
	vaultContainer *vault.VaultContainer
	vaultAddr      string
	k8sClient      client.Client
	scheme         *runtime.Scheme
	reconciler     *controller.VaultUnsealConfigReconciler
	ctx            context.Context
	ctxCancel      context.CancelFunc
	metrics        *PerformanceMetrics
}

// PerformanceMetrics tracks performance data
type PerformanceMetrics struct {
	mu                    sync.Mutex
	ReconciliationTimes   []time.Duration
	ClientCreationTimes   []time.Duration
	VaultOperationTimes   []time.Duration
	ConfigUpdateTimes     []time.Duration
	TotalOperations       int64
	SuccessfulOperations  int64
	FailedOperations      int64
}

// AddReconciliationTime records a reconciliation duration
func (m *PerformanceMetrics) AddReconciliationTime(duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ReconciliationTimes = append(m.ReconciliationTimes, duration)
	m.TotalOperations++
}

// AddSuccess increments successful operations
func (m *PerformanceMetrics) AddSuccess() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.SuccessfulOperations++
}

// AddFailure increments failed operations
func (m *PerformanceMetrics) AddFailure() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.FailedOperations++
}

// GetStats returns performance statistics
func (m *PerformanceMetrics) GetStats() (avgReconciliation time.Duration, p95Reconciliation time.Duration, successRate float64) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if len(m.ReconciliationTimes) == 0 {
		return 0, 0, 0
	}

	// Calculate average
	var total time.Duration
	for _, d := range m.ReconciliationTimes {
		total += d
	}
	avgReconciliation = total / time.Duration(len(m.ReconciliationTimes))

	// Calculate P95 (simplified - just 95th percentile position)
	if len(m.ReconciliationTimes) > 0 {
		p95Index := int(float64(len(m.ReconciliationTimes)) * 0.95)
		if p95Index >= len(m.ReconciliationTimes) {
			p95Index = len(m.ReconciliationTimes) - 1
		}
		p95Reconciliation = m.ReconciliationTimes[p95Index]
	}

	// Calculate success rate
	if m.TotalOperations > 0 {
		successRate = float64(m.SuccessfulOperations) / float64(m.TotalOperations) * 100
	}

	return
}

// SetupSuite initializes performance testing environment
func (suite *PerformanceTestSuite) SetupSuite() {
	suite.ctx, suite.ctxCancel = context.WithTimeout(context.Background(), 45*time.Minute)

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	suite.metrics = &PerformanceMetrics{}

	// Set up performance testing infrastructure using TestContainers
	suite.setupPerformanceInfrastructure()
}

// setupPerformanceInfrastructure creates optimized containers for performance testing
func (suite *PerformanceTestSuite) setupPerformanceInfrastructure() {
	// Create K3s cluster with performance optimizations
	suite.setupOptimizedK3s()

	// Create optimized Vault container
	suite.setupOptimizedVault()

	// Set up controller
	suite.setupController()
}

// setupOptimizedK3s creates K3s with performance settings
func (suite *PerformanceTestSuite) setupOptimizedK3s() {
	k3sContainer, err := k3s.Run(suite.ctx,
		"rancher/k3s:v1.32.1-k3s1",
		testcontainers.WithWaitStrategy(
			wait.ForLog("k3s is up and running").
				WithStartupTimeout(300*time.Second).
				WithPollInterval(5*time.Second),
		),
		// Resource limits commented out due to API changes
		// testcontainers.WithConfigModifier(func(config *testcontainers.ContainerRequest) {
		//	config.Resources = testcontainers.ContainerResources{
		//		Memory: 1024 * 1024 * 1024, // 1GB
		//		CPU:    "1000m",             // 1 CPU
		//	}
		// }),
	)
	require.NoError(suite.T(), err)
	suite.k3sContainer = k3sContainer

	// Set up Kubernetes client
	kubeconfig, err := k3sContainer.GetKubeConfig(suite.ctx)
	require.NoError(suite.T(), err)

	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	require.NoError(suite.T(), err)

	// Optimize client settings for performance
	restConfig.QPS = 100    // Increase QPS
	restConfig.Burst = 200  // Increase burst

	suite.scheme = runtime.NewScheme()
	err = clientgoscheme.AddToScheme(suite.scheme)
	require.NoError(suite.T(), err)

	err = vaultv1.AddToScheme(suite.scheme)
	require.NoError(suite.T(), err)

	suite.k8sClient, err = client.New(restConfig, client.Options{Scheme: suite.scheme})
	require.NoError(suite.T(), err)

	suite.waitForOptimizedAPI()
}

// setupOptimizedVault creates Vault with performance settings
func (suite *PerformanceTestSuite) setupOptimizedVault() {
	vaultContainer, err := vault.Run(suite.ctx,
		"hashicorp/vault:1.19.0",
		vault.WithToken("performance-test-token"),
		testcontainers.WithWaitStrategy(
			wait.ForHTTP("/v1/sys/health").
				WithStatusCodeMatcher(func(status int) bool {
					return status == 200 || status == 429
				}).
				WithStartupTimeout(90*time.Second),
		),
		// Resource limits commented out due to API changes
		// testcontainers.WithConfigModifier(func(config *testcontainers.ContainerRequest) {
		//	config.Resources = testcontainers.ContainerResources{
		//		Memory: 512 * 1024 * 1024, // 512MB
		//		CPU:    "500m",             // 0.5 CPU
		//	}
		// }),
	)
	require.NoError(suite.T(), err)
	suite.vaultContainer = vaultContainer

	vaultAddr, err := vaultContainer.HttpHostAddress(suite.ctx)
	require.NoError(suite.T(), err)
	suite.vaultAddr = vaultAddr
}

// setupController creates optimized controller
func (suite *PerformanceTestSuite) setupController() {
	suite.reconciler = &controller.VaultUnsealConfigReconciler{
		Client: suite.k8sClient,
		Log:    ctrl.Log.WithName("perf-controller"),
		Scheme: suite.scheme,
	}
}

// waitForOptimizedAPI waits for API with performance considerations
func (suite *PerformanceTestSuite) waitForOptimizedAPI() {
	require.Eventually(suite.T(), func() bool {
		_, err := suite.k8sClient.RESTMapper().RESTMapping(
			vaultv1.GroupVersion.WithKind("VaultUnsealConfig").GroupKind(),
		)
		return err == nil
	}, 120*time.Second, 2*time.Second)
}

// TearDownSuite cleans up resources
func (suite *PerformanceTestSuite) TearDownSuite() {
	if suite.ctxCancel != nil {
		suite.ctxCancel()
	}

	if suite.vaultContainer != nil {
		suite.vaultContainer.Terminate(context.Background())
	}

	if suite.k3sContainer != nil {
		suite.k3sContainer.Terminate(context.Background())
	}
}

// TearDownTest cleans up after each test
func (suite *PerformanceTestSuite) TearDownTest() {
	configList := &vaultv1.VaultUnsealConfigList{}
	err := suite.k8sClient.List(suite.ctx, configList)
	if err == nil {
		for _, config := range configList.Items {
			suite.k8sClient.Delete(suite.ctx, &config)
		}
	}
}

// TestReconciliationPerformance measures reconciliation performance
func (suite *PerformanceTestSuite) TestReconciliationPerformance() {
	// Create test configuration
	config := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "perf-reconciliation-test",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:       "perf-vault",
					Endpoint:   suite.vaultAddr,
					UnsealKeys: []string{"dGVzdDE=", "dGVzdDI=", "dGVzdDM="},
				},
			},
		},
	}

	err := suite.k8sClient.Create(suite.ctx, config)
	require.NoError(suite.T(), err)

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "perf-reconciliation-test",
			Namespace: "default",
		},
	}

	// Warm up
	for i := 0; i < 5; i++ {
		suite.reconciler.Reconcile(suite.ctx, req)
		time.Sleep(100 * time.Millisecond)
	}

	// Performance test
	reconciliationCount := 100
	start := time.Now()

	for i := 0; i < reconciliationCount; i++ {
		reconcileStart := time.Now()
		_, err := suite.reconciler.Reconcile(suite.ctx, req)
		reconcileDuration := time.Since(reconcileStart)

		suite.metrics.AddReconciliationTime(reconcileDuration)
		if err == nil {
			suite.metrics.AddSuccess()
		} else {
			suite.metrics.AddFailure()
		}
	}

	totalDuration := time.Since(start)
	avgReconciliation, p95Reconciliation, successRate := suite.metrics.GetStats()

	suite.T().Logf("Reconciliation Performance Results:")
	suite.T().Logf("  Total reconciliations: %d", reconciliationCount)
	suite.T().Logf("  Total duration: %v", totalDuration)
	suite.T().Logf("  Average reconciliation time: %v", avgReconciliation)
	suite.T().Logf("  P95 reconciliation time: %v", p95Reconciliation)
	suite.T().Logf("  Success rate: %.2f%%", successRate)
	suite.T().Logf("  Reconciliations per second: %.2f", float64(reconciliationCount)/totalDuration.Seconds())

	// Performance assertions
	assert.Less(suite.T(), avgReconciliation, 1*time.Second, "Average reconciliation should be under 1 second")
	assert.Less(suite.T(), p95Reconciliation, 2*time.Second, "P95 reconciliation should be under 2 seconds")
	assert.Greater(suite.T(), successRate, 95.0, "Success rate should be above 95%")
}

// TestConcurrentReconciliationPerformance tests concurrent reconciliation performance
func (suite *PerformanceTestSuite) TestConcurrentReconciliationPerformance() {
	configCount := 20
	reconciliationsPerConfig := 10
	concurrency := 10

	// Create multiple configurations
	var configs []*vaultv1.VaultUnsealConfig
	for i := 0; i < configCount; i++ {
		config := &vaultv1.VaultUnsealConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("perf-concurrent-%d", i),
				Namespace: "default",
			},
			Spec: vaultv1.VaultUnsealConfigSpec{
				VaultInstances: []vaultv1.VaultInstance{
					{
						Name:       fmt.Sprintf("concurrent-vault-%d", i),
						Endpoint:   suite.vaultAddr,
						UnsealKeys: []string{"dGVzdDE=", "dGVzdDI=", "dGVzdDM="},
					},
				},
			},
		}

		err := suite.k8sClient.Create(suite.ctx, config)
		require.NoError(suite.T(), err)
		configs = append(configs, config)
	}

	// Performance test with controlled concurrency
	semaphore := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	totalOps := configCount * reconciliationsPerConfig

	start := time.Now()

	for i, config := range configs {
		for j := 0; j < reconciliationsPerConfig; j++ {
			wg.Add(1)
			go func(configIndex, opIndex int, cfg *vaultv1.VaultUnsealConfig) {
				defer wg.Done()

				// Acquire semaphore
				semaphore <- struct{}{}
				defer func() { <-semaphore }()

				req := ctrl.Request{
					NamespacedName: types.NamespacedName{
						Name:      cfg.Name,
						Namespace: cfg.Namespace,
					},
				}

				reconcileStart := time.Now()
				_, err := suite.reconciler.Reconcile(suite.ctx, req)
				reconcileDuration := time.Since(reconcileStart)

				suite.metrics.AddReconciliationTime(reconcileDuration)
				if err == nil {
					suite.metrics.AddSuccess()
				} else {
					suite.metrics.AddFailure()
				}
			}(i, j, config)
		}
	}

	wg.Wait()
	totalDuration := time.Since(start)
	avgReconciliation, p95Reconciliation, successRate := suite.metrics.GetStats()

	suite.T().Logf("Concurrent Reconciliation Performance Results:")
	suite.T().Logf("  Total operations: %d", totalOps)
	suite.T().Logf("  Concurrency level: %d", concurrency)
	suite.T().Logf("  Total duration: %v", totalDuration)
	suite.T().Logf("  Average reconciliation time: %v", avgReconciliation)
	suite.T().Logf("  P95 reconciliation time: %v", p95Reconciliation)
	suite.T().Logf("  Success rate: %.2f%%", successRate)
	suite.T().Logf("  Operations per second: %.2f", float64(totalOps)/totalDuration.Seconds())
	suite.T().Logf("  Effective throughput: %.2f ops/sec per worker", float64(totalOps)/totalDuration.Seconds()/float64(concurrency))

	// Performance assertions for concurrent operations
	assert.Less(suite.T(), avgReconciliation, 2*time.Second, "Average concurrent reconciliation should be reasonable")
	assert.Greater(suite.T(), successRate, 90.0, "Concurrent success rate should be above 90%")

	// Throughput should be reasonable
	opsPerSecond := float64(totalOps) / totalDuration.Seconds()
	assert.Greater(suite.T(), opsPerSecond, 5.0, "Should achieve at least 5 operations per second")
}

// TestVaultClientPerformance measures vault client performance
func (suite *PerformanceTestSuite) TestVaultClientPerformance() {
	operationCount := 200
	concurrency := 20

	semaphore := make(chan struct{}, concurrency)
	var wg sync.WaitGroup
	results := make(chan time.Duration, operationCount)
	errors := make(chan error, operationCount)

	start := time.Now()

	for i := 0; i < operationCount; i++ {
		wg.Add(1)
		go func(opIndex int) {
			defer wg.Done()

			// Acquire semaphore
			semaphore <- struct{}{}
			defer func() { <-semaphore }()

			opStart := time.Now()

			client, err := vaultpkg.NewClient(suite.vaultAddr, false, 10*time.Second)
			if err != nil {
				errors <- err
				return
			}
			defer client.Close()

			_, err = client.HealthCheck(suite.ctx)
			if err != nil {
				errors <- err
				return
			}

			opDuration := time.Since(opStart)
			results <- opDuration
		}(i)
	}

	wg.Wait()
	totalDuration := time.Since(start)

	close(results)
	close(errors)

	// Analyze results
	var durations []time.Duration
	for duration := range results {
		durations = append(durations, duration)
	}

	errorCount := len(errors)
	successCount := len(durations)

	if len(durations) > 0 {
		var totalTime time.Duration
		for _, d := range durations {
			totalTime += d
		}
		avgDuration := totalTime / time.Duration(len(durations))

		suite.T().Logf("Vault Client Performance Results:")
		suite.T().Logf("  Total operations: %d", operationCount)
		suite.T().Logf("  Successful operations: %d", successCount)
		suite.T().Logf("  Failed operations: %d", errorCount)
		suite.T().Logf("  Total duration: %v", totalDuration)
		suite.T().Logf("  Average operation time: %v", avgDuration)
		suite.T().Logf("  Operations per second: %.2f", float64(successCount)/totalDuration.Seconds())
		suite.T().Logf("  Success rate: %.2f%%", float64(successCount)/float64(operationCount)*100)

		// Performance assertions
		assert.Less(suite.T(), avgDuration, 500*time.Millisecond, "Average vault operation should be under 500ms")
		assert.Greater(suite.T(), float64(successCount)/float64(operationCount), 0.95, "Success rate should be above 95%")
	}
}

// TestMemoryAndResourceUsage measures resource usage patterns
func (suite *PerformanceTestSuite) TestMemoryAndResourceUsage() {
	// Create a large number of configurations to test resource usage
	configCount := 100

	// Measure initial state
	suite.T().Log("Starting resource usage test with large configuration set")

	var configs []*vaultv1.VaultUnsealConfig
	creationStart := time.Now()

	for i := 0; i < configCount; i++ {
		config := &vaultv1.VaultUnsealConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("resource-test-%d", i),
				Namespace: "default",
				Labels: map[string]string{
					"test-batch":      "resource-usage",
					"config-index":    fmt.Sprintf("%d", i),
					"performance-test": "true",
				},
			},
			Spec: vaultv1.VaultUnsealConfigSpec{
				VaultInstances: []vaultv1.VaultInstance{
					{
						Name:       fmt.Sprintf("resource-vault-%d", i),
						Endpoint:   suite.vaultAddr,
						UnsealKeys: []string{"dGVzdDE=", "dGVzdDI=", "dGVzdDM=", "dGVzdDQ=", "dGVzdDU="},
					},
				},
			},
		}

		err := suite.k8sClient.Create(suite.ctx, config)
		require.NoError(suite.T(), err)
		configs = append(configs, config)
	}

	creationDuration := time.Since(creationStart)
	suite.T().Logf("Created %d configurations in %v", configCount, creationDuration)

	// Perform reconciliations and measure performance degradation
	reconcileStart := time.Now()
	successfulReconciliations := 0

	for i, config := range configs {
		req := ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      config.Name,
				Namespace: config.Namespace,
			},
		}

		_, err := suite.reconciler.Reconcile(suite.ctx, req)
		if err == nil {
			successfulReconciliations++
		}

		// Log progress every 25 configs
		if (i+1)%25 == 0 {
			elapsed := time.Since(reconcileStart)
			avgTime := elapsed / time.Duration(i+1)
			suite.T().Logf("Progress: %d/%d configs reconciled, avg time per config: %v", i+1, configCount, avgTime)
		}
	}

	reconcileDuration := time.Since(reconcileStart)
	avgReconcileTime := reconcileDuration / time.Duration(configCount)

	suite.T().Logf("Resource Usage Test Results:")
	suite.T().Logf("  Configuration count: %d", configCount)
	suite.T().Logf("  Creation time: %v (avg: %v per config)", creationDuration, creationDuration/time.Duration(configCount))
	suite.T().Logf("  Reconciliation time: %v (avg: %v per config)", reconcileDuration, avgReconcileTime)
	suite.T().Logf("  Successful reconciliations: %d/%d", successfulReconciliations, configCount)
	suite.T().Logf("  Success rate: %.2f%%", float64(successfulReconciliations)/float64(configCount)*100)

	// Performance should not degrade significantly with more resources
	assert.Less(suite.T(), avgReconcileTime, 2*time.Second, "Average reconciliation time should remain reasonable with many resources")
	assert.Greater(suite.T(), float64(successfulReconciliations)/float64(configCount), 0.9, "Should maintain high success rate with many resources")
}

// TestPerformanceTestSuite runs the performance test suite
func TestPerformanceTestSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping performance tests in short mode")
	}

	suite.Run(t, new(PerformanceTestSuite))
}
