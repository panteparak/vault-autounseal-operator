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

// StressTestSuite provides stress testing and performance validation
type StressTestSuite struct {
	suite.Suite
	k3sContainer   *k3s.K3sContainer
	vaultContainer *vault.VaultContainer
	vaultAddr      string
	k8sClient      client.Client
	scheme         *runtime.Scheme
	reconciler     *controller.VaultUnsealConfigReconciler
	ctx            context.Context
	ctxCancel      context.CancelFunc
}

// SetupSuite initializes the stress test environment
func (suite *StressTestSuite) SetupSuite() {
	suite.ctx, suite.ctxCancel = context.WithTimeout(context.Background(), 45*time.Minute)
	
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	// Set up infrastructure for stress testing
	suite.setupInfrastructure()
}

// setupInfrastructure creates the test infrastructure
func (suite *StressTestSuite) setupInfrastructure() {
	// Create K3s cluster
	suite.setupK3s()
	
	// Create Vault
	suite.setupVault()
	
	// Set up controller
	suite.setupController()
}

// setupK3s creates K3s cluster
func (suite *StressTestSuite) setupK3s() {
	_ = `apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: vaultunsealconfigs.vault.io
spec:
  group: vault.io
  versions:
  - name: v1
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        type: object
        properties:
          spec:
            type: object
            properties:
              vaultInstances:
                type: array
                items:
                  type: object
                  properties:
                    name:
                      type: string
                    endpoint:
                      type: string
                    unsealKeys:
                      type: array
                      items:
                        type: string
                    threshold:
                      type: integer
                  required:
                  - name
                  - endpoint
                  - unsealKeys
            required:
            - vaultInstances
          status:
            type: object
        required:
        - spec
  scope: Namespaced
  names:
    plural: vaultunsealconfigs
    singular: vaultunsealconfig
    kind: VaultUnsealConfig
    shortNames:
    - vuc`

	k3sContainer, err := k3s.Run(suite.ctx,
		"rancher/k3s:v1.32.1-k3s1",
		testcontainers.WithWaitStrategy(
			wait.ForLog("k3s is up and running").
				WithStartupTimeout(300*time.Second).
				WithPollInterval(10*time.Second),
		),
	)
	require.NoError(suite.T(), err)
	suite.k3sContainer = k3sContainer

	// Set up Kubernetes client
	kubeconfig, err := k3sContainer.GetKubeConfig(suite.ctx)
	require.NoError(suite.T(), err)

	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	require.NoError(suite.T(), err)

	suite.scheme = runtime.NewScheme()
	err = clientgoscheme.AddToScheme(suite.scheme)
	require.NoError(suite.T(), err)
	
	err = vaultv1.AddToScheme(suite.scheme)
	require.NoError(suite.T(), err)

	suite.k8sClient, err = client.New(restConfig, client.Options{Scheme: suite.scheme})
	require.NoError(suite.T(), err)

	// Wait for API server
	suite.waitForAPI()
}

// setupVault creates Vault container
func (suite *StressTestSuite) setupVault() {
	vaultContainer, err := vault.Run(suite.ctx,
		"hashicorp/vault:1.19.0",
		vault.WithToken("stress-test-token"),
		testcontainers.WithWaitStrategy(
			wait.ForHTTP("/v1/sys/health").
				WithStatusCodeMatcher(func(status int) bool {
					return status == 200 || status == 429
				}).
				WithStartupTimeout(120*time.Second),
		),
	)
	require.NoError(suite.T(), err)
	suite.vaultContainer = vaultContainer

	vaultAddr, err := vaultContainer.HttpHostAddress(suite.ctx)
	require.NoError(suite.T(), err)
	suite.vaultAddr = vaultAddr
}

// setupController creates controller
func (suite *StressTestSuite) setupController() {
	suite.reconciler = &controller.VaultUnsealConfigReconciler{
		Client: suite.k8sClient,
		Log:    ctrl.Log.WithName("stress-test-controller"),
		Scheme: suite.scheme,
	}
}

// waitForAPI waits for Kubernetes API to be ready
func (suite *StressTestSuite) waitForAPI() {
	require.Eventually(suite.T(), func() bool {
		_, err := suite.k8sClient.RESTMapper().RESTMapping(
			vaultv1.GroupVersion.WithKind("VaultUnsealConfig").GroupKind(),
		)
		return err == nil
	}, 120*time.Second, 5*time.Second)
}

// TearDownSuite cleans up resources
func (suite *StressTestSuite) TearDownSuite() {
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
func (suite *StressTestSuite) TearDownTest() {
	configList := &vaultv1.VaultUnsealConfigList{}
	err := suite.k8sClient.List(suite.ctx, configList)
	if err == nil {
		for _, config := range configList.Items {
			suite.k8sClient.Delete(suite.ctx, &config)
		}
	}
}

// TestHighVolumeReconciliation tests reconciliation under high volume
func (suite *StressTestSuite) TestHighVolumeReconciliation() {
	configCount := 50
	reconciliationsPerConfig := 20
	
	// Create many VaultUnsealConfigs
	configs := make([]*vaultv1.VaultUnsealConfig, configCount)
	for i := 0; i < configCount; i++ {
		config := &vaultv1.VaultUnsealConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("stress-config-%d", i),
				Namespace: "default",
			},
			Spec: vaultv1.VaultUnsealConfigSpec{
				VaultInstances: []vaultv1.VaultInstance{
					{
						Name:       fmt.Sprintf("vault-%d", i),
						Endpoint:   suite.vaultAddr,
						UnsealKeys: []string{"dGVzdDE=", "dGVzdDI=", "dGVzdDM="},
					},
				},
			},
		}
		
		err := suite.k8sClient.Create(suite.ctx, config)
		require.NoError(suite.T(), err)
		configs[i] = config
	}

	// Perform many reconciliations concurrently
	var wg sync.WaitGroup
	errors := make(chan error, configCount*reconciliationsPerConfig)
	successes := make(chan bool, configCount*reconciliationsPerConfig)

	start := time.Now()

	for i := 0; i < configCount; i++ {
		for j := 0; j < reconciliationsPerConfig; j++ {
			wg.Add(1)
			go func(configIndex, reconcileIndex int) {
				defer wg.Done()
				
				req := ctrl.Request{
					NamespacedName: types.NamespacedName{
						Name:      fmt.Sprintf("stress-config-%d", configIndex),
						Namespace: "default",
					},
				}

				_, err := suite.reconciler.Reconcile(suite.ctx, req)
				if err != nil {
					errors <- err
				} else {
					successes <- true
				}
			}(i, j)
		}
	}

	wg.Wait()
	duration := time.Since(start)
	
	close(errors)
	close(successes)

	_ = len(errors)
	successCount := len(successes)
	totalOps := configCount * reconciliationsPerConfig

	suite.T().Logf("High volume reconciliation: %d operations in %v", totalOps, duration)
	suite.T().Logf("Success rate: %d/%d (%.2f%%)", successCount, totalOps, float64(successCount)/float64(totalOps)*100)
	
	// Allow some failures but expect majority to succeed
	assert.Greater(suite.T(), successCount, totalOps/2, "At least half of reconciliations should succeed")
	assert.Less(suite.T(), duration, 5*time.Minute, "High volume reconciliation should complete in reasonable time")
}

// TestConcurrentVaultClientConnections tests many concurrent vault client connections
func (suite *StressTestSuite) TestConcurrentVaultClientConnections() {
	clientCount := 100
	operationsPerClient := 10

	var wg sync.WaitGroup
	errors := make(chan error, clientCount*operationsPerClient)
	successes := make(chan bool, clientCount*operationsPerClient)

	start := time.Now()

	for i := 0; i < clientCount; i++ {
		wg.Add(1)
		go func(clientID int) {
			defer wg.Done()

			client, err := vaultpkg.NewClient(suite.vaultAddr, false, 30*time.Second)
			if err != nil {
				errors <- fmt.Errorf("client %d creation failed: %w", clientID, err)
				return
			}
			defer client.Close()

			// Perform multiple operations per client
			for j := 0; j < operationsPerClient; j++ {
				_, err := client.HealthCheck(suite.ctx)
				if err != nil {
					errors <- fmt.Errorf("client %d operation %d failed: %w", clientID, j, err)
				} else {
					successes <- true
				}
			}
		}(i)
	}

	wg.Wait()
	duration := time.Since(start)

	close(errors)
	close(successes)

	_ = len(errors)
	successCount := len(successes)
	totalOps := clientCount * operationsPerClient

	suite.T().Logf("Concurrent vault clients: %d operations in %v", totalOps, duration)
	suite.T().Logf("Success rate: %d/%d (%.2f%%)", successCount, totalOps, float64(successCount)/float64(totalOps)*100)

	// Expect high success rate for basic operations
	assert.Greater(suite.T(), successCount, totalOps*3/4, "At least 75% of operations should succeed")
}

// TestResourceExhaustionScenarios tests various resource exhaustion scenarios
func (suite *StressTestSuite) TestResourceExhaustionScenarios() {
	suite.Run("memory_exhaustion", func() {
		// Create configs with large amounts of data
		largeConfig := &vaultv1.VaultUnsealConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "large-config",
				Namespace: "default",
				Labels:    make(map[string]string),
			},
			Spec: vaultv1.VaultUnsealConfigSpec{
				VaultInstances: make([]vaultv1.VaultInstance, 100), // Many instances
			},
		}

		// Add many labels
		for i := 0; i < 50; i++ {
			largeConfig.Labels[fmt.Sprintf("label-%d", i)] = fmt.Sprintf("value-%d", i)
		}

		// Add many vault instances
		for i := 0; i < 100; i++ {
			largeConfig.Spec.VaultInstances[i] = vaultv1.VaultInstance{
				Name:       fmt.Sprintf("vault-instance-%d", i),
				Endpoint:   fmt.Sprintf("https://vault-%d.example.com:8200", i),
				UnsealKeys: []string{"dGVzdDE=", "dGVzdDI=", "dGVzdDM="},
			}
		}

		err := suite.k8sClient.Create(suite.ctx, largeConfig)
		assert.NoError(suite.T(), err, "Should handle large configurations")
	})

	suite.Run("connection_exhaustion", func() {
		// Test many connections to overwhelm connection pools
		connectionCount := 200
		var wg sync.WaitGroup
		
		for i := 0; i < connectionCount; i++ {
			wg.Add(1)
			go func(id int) {
				defer wg.Done()
				
				client, err := vaultpkg.NewClient(suite.vaultAddr, false, 5*time.Second)
				if err != nil {
					return // Expected to fail under load
				}
				defer client.Close()
				
				// Hold connection briefly
				time.Sleep(100 * time.Millisecond)
				client.HealthCheck(suite.ctx)
			}(i)
		}
		
		wg.Wait()
		suite.T().Log("Connection exhaustion test completed")
	})
}

// TestLongRunningOperations tests operations that run for extended periods
func (suite *StressTestSuite) TestLongRunningOperations() {
	config := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "long-running-config",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:       "persistent-vault",
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
			Name:      "long-running-config",
			Namespace: "default",
		},
	}

	// Perform reconciliations over an extended period
	reconciliationCount := 100
	interval := 2 * time.Second
	
	start := time.Now()
	successCount := 0
	
	for i := 0; i < reconciliationCount; i++ {
		_, err := suite.reconciler.Reconcile(suite.ctx, req)
		if err == nil {
			successCount++
		}
		
		if i < reconciliationCount-1 {
			time.Sleep(interval)
		}
	}
	
	duration := time.Since(start)
	
	suite.T().Logf("Long running operations: %d reconciliations over %v", reconciliationCount, duration)
	suite.T().Logf("Success rate: %d/%d", successCount, reconciliationCount)
	
	assert.Greater(suite.T(), successCount, reconciliationCount*3/4, "Most long-running operations should succeed")
}

// TestRapidConfigurationChanges tests rapid updates to configurations
func (suite *StressTestSuite) TestRapidConfigurationChanges() {
	config := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "rapid-change-config",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:       "changeable-vault",
					Endpoint:   suite.vaultAddr,
					UnsealKeys: []string{"dGVzdDE=", "dGVzdDI=", "dGVzdDM="},
				},
			},
		},
	}

	err := suite.k8sClient.Create(suite.ctx, config)
	require.NoError(suite.T(), err)

	// Perform rapid updates
	updateCount := 50
	for i := 0; i < updateCount; i++ {
		// Get current version
		var currentConfig vaultv1.VaultUnsealConfig
		err := suite.k8sClient.Get(suite.ctx, types.NamespacedName{
			Name: "rapid-change-config", Namespace: "default",
		}, &currentConfig)
		require.NoError(suite.T(), err)

		// Update endpoint
		currentConfig.Spec.VaultInstances[0].Endpoint = fmt.Sprintf("%s?version=%d", suite.vaultAddr, i)
		
		err = suite.k8sClient.Update(suite.ctx, &currentConfig)
		assert.NoError(suite.T(), err, "Rapid update %d should succeed", i)
		
		// Brief pause to allow processing
		time.Sleep(10 * time.Millisecond)
	}

	suite.T().Logf("Completed %d rapid configuration changes", updateCount)
}

// TestCascadingFailureRecovery tests recovery from cascading failures
func (suite *StressTestSuite) TestCascadingFailureRecovery() {
	// Create multiple configs that depend on the same vault
	configCount := 20
	configs := make([]*vaultv1.VaultUnsealConfig, configCount)
	
	for i := 0; i < configCount; i++ {
		config := &vaultv1.VaultUnsealConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("cascade-config-%d", i),
				Namespace: "default",
			},
			Spec: vaultv1.VaultUnsealConfigSpec{
				VaultInstances: []vaultv1.VaultInstance{
					{
						Name:       fmt.Sprintf("cascade-vault-%d", i),
						Endpoint:   suite.vaultAddr, // All point to same vault
						UnsealKeys: []string{"dGVzdDE=", "dGVzdDI=", "dGVzdDM="},
					},
				},
			},
		}
		
		err := suite.k8sClient.Create(suite.ctx, config)
		require.NoError(suite.T(), err)
		configs[i] = config
	}

	// Simulate initial reconciliations
	var wg sync.WaitGroup
	for i := 0; i < configCount; i++ {
		wg.Add(1)
		go func(configIndex int) {
			defer wg.Done()
			
			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      fmt.Sprintf("cascade-config-%d", configIndex),
					Namespace: "default",
				},
			}
			
			suite.reconciler.Reconcile(suite.ctx, req)
		}(i)
	}
	
	wg.Wait()
	
	// Now update all configs to point to invalid endpoint (simulate vault failure)
	for i := 0; i < configCount; i++ {
		configs[i].Spec.VaultInstances[0].Endpoint = "http://invalid-vault.local:8200"
		err := suite.k8sClient.Update(suite.ctx, configs[i])
		require.NoError(suite.T(), err)
	}

	// Reconcile with failures
	failureReconciliations := 0
	for i := 0; i < configCount; i++ {
		req := ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      fmt.Sprintf("cascade-config-%d", i),
				Namespace: "default",
			},
		}
		
		_, err := suite.reconciler.Reconcile(suite.ctx, req)
		if err == nil {
			failureReconciliations++
		}
	}

	suite.T().Logf("Cascading failure handled: %d/%d reconciliations succeeded", failureReconciliations, configCount)
	
	// Recovery - fix endpoints
	for i := 0; i < configCount; i++ {
		configs[i].Spec.VaultInstances[0].Endpoint = suite.vaultAddr
		err := suite.k8sClient.Update(suite.ctx, configs[i])
		require.NoError(suite.T(), err)
	}

	// Verify recovery
	recoveryReconciliations := 0
	for i := 0; i < configCount; i++ {
		req := ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      fmt.Sprintf("cascade-config-%d", i),
				Namespace: "default",
			},
		}
		
		_, err := suite.reconciler.Reconcile(suite.ctx, req)
		if err == nil {
			recoveryReconciliations++
		}
	}

	suite.T().Logf("Recovery: %d/%d reconciliations succeeded", recoveryReconciliations, configCount)
	assert.Greater(suite.T(), recoveryReconciliations, failureReconciliations, "Recovery should improve success rate")
}

// TestStressTestSuite runs the stress test suite
func TestStressTestSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping stress tests in short mode")
	}

	suite.Run(t, new(StressTestSuite))
}