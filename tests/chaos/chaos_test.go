package integration

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/vault/api"
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

// ChaosTestSuite provides chaos engineering tests using TestContainers
type ChaosTestSuite struct {
	suite.Suite
	primaryK3s       *k3s.K3sContainer
	primaryVault     *vault.VaultContainer
	secondaryVault   *vault.VaultContainer
	primaryVaultAddr string
	secondaryVaultAddr string
	k8sClient        client.Client
	scheme           *runtime.Scheme
	reconciler       *controller.VaultUnsealConfigReconciler
	ctx              context.Context
	ctxCancel        context.CancelFunc
	vaultClients     []*api.Client
	mu               sync.Mutex
}

// SetupSuite initializes chaos testing environment with multiple containers
func (suite *ChaosTestSuite) SetupSuite() {
	suite.ctx, suite.ctxCancel = context.WithTimeout(context.Background(), 60*time.Minute)

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	rand.Seed(time.Now().UnixNano())

	// Set up multiple infrastructure components for chaos testing
	suite.setupChaosInfrastructure()
}

// setupChaosInfrastructure creates multiple containers for chaos scenarios
func (suite *ChaosTestSuite) setupChaosInfrastructure() {
	// Create K3s cluster
	suite.setupK3sCluster()

	// Create multiple Vault instances for failover testing
	suite.setupVaultClusters()

	// Set up controller
	suite.setupController()
}

// setupK3sCluster creates K3s with CRDs using TestContainers
func (suite *ChaosTestSuite) setupK3sCluster() {
	k3sContainer, err := k3s.Run(suite.ctx,
		"rancher/k3s:v1.32.1-k3s1",
		testcontainers.WithWaitStrategy(
			wait.ForLog("k3s is up and running").
				WithStartupTimeout(240*time.Second).
				WithPollInterval(10*time.Second),
		),
	)
	require.NoError(suite.T(), err, "Failed to create K3s container")
	suite.primaryK3s = k3sContainer

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

	suite.waitForK8sAPI()
}

// setupVaultClusters creates multiple Vault containers using TestContainers
func (suite *ChaosTestSuite) setupVaultClusters() {
	// Primary Vault
	primaryVault, err := vault.Run(suite.ctx,
		"hashicorp/vault:1.19.0",
		vault.WithToken("chaos-primary-token"),
		testcontainers.WithWaitStrategy(
			wait.ForHTTP("/v1/sys/health").
				WithStatusCodeMatcher(func(status int) bool {
					return status == 200 || status == 429
				}).
				WithStartupTimeout(120*time.Second),
		),
	)
	require.NoError(suite.T(), err, "Failed to create primary Vault container")
	suite.primaryVault = primaryVault

	primaryAddr, err := primaryVault.HttpHostAddress(suite.ctx)
	require.NoError(suite.T(), err)
	suite.primaryVaultAddr = primaryAddr

	// Secondary Vault for failover scenarios
	secondaryVault, err := vault.Run(suite.ctx,
		"hashicorp/vault:1.19.0",
		vault.WithToken("chaos-secondary-token"),
		testcontainers.WithWaitStrategy(
			wait.ForHTTP("/v1/sys/health").
				WithStatusCodeMatcher(func(status int) bool {
					return status == 200 || status == 429
				}).
				WithStartupTimeout(120*time.Second),
		),
	)
	require.NoError(suite.T(), err, "Failed to create secondary Vault container")
	suite.secondaryVault = secondaryVault

	secondaryAddr, err := secondaryVault.HttpHostAddress(suite.ctx)
	require.NoError(suite.T(), err)
	suite.secondaryVaultAddr = secondaryAddr
}

// setupController creates controller instance
func (suite *ChaosTestSuite) setupController() {
	suite.reconciler = &controller.VaultUnsealConfigReconciler{
		Client: suite.k8sClient,
		Log:    ctrl.Log.WithName("chaos-controller"),
		Scheme: suite.scheme,
	}
}

// waitForK8sAPI waits for Kubernetes API to be ready
func (suite *ChaosTestSuite) waitForK8sAPI() {
	require.Eventually(suite.T(), func() bool {
		_, err := suite.k8sClient.RESTMapper().RESTMapping(
			vaultv1.GroupVersion.WithKind("VaultUnsealConfig").GroupKind(),
		)
		return err == nil
	}, 180*time.Second, 5*time.Second, "Kubernetes API should become ready")
}

// TearDownSuite cleans up all containers
func (suite *ChaosTestSuite) TearDownSuite() {
	suite.mu.Lock()
	defer suite.mu.Unlock()

	// Close all vault clients
	for _, client := range suite.vaultClients {
		if client != nil {
			// Note: hashicorp vault client doesn't have a Close() method
			// This is just for our custom client wrapper
		}
	}

	if suite.ctxCancel != nil {
		suite.ctxCancel()
	}

	if suite.secondaryVault != nil {
		suite.secondaryVault.Terminate(context.Background())
	}

	if suite.primaryVault != nil {
		suite.primaryVault.Terminate(context.Background())
	}

	if suite.primaryK3s != nil {
		suite.primaryK3s.Terminate(context.Background())
	}
}

// TearDownTest cleans up after each test
func (suite *ChaosTestSuite) TearDownTest() {
	configList := &vaultv1.VaultUnsealConfigList{}
	err := suite.k8sClient.List(suite.ctx, configList)
	if err == nil {
		for _, config := range configList.Items {
			suite.k8sClient.Delete(suite.ctx, &config)
		}
	}
}

// TestContainerTerminationChaos tests random container termination scenarios
func (suite *ChaosTestSuite) TestContainerTerminationChaos() {
	// Create a configuration that uses both vaults
	config := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "chaos-termination-config",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:       "primary-vault",
					Endpoint:   suite.primaryVaultAddr,
					UnsealKeys: []string{"dGVzdDE=", "dGVzdDI=", "dGVzdDM="},
				},
				{
					Name:       "secondary-vault",
					Endpoint:   suite.secondaryVaultAddr,
					UnsealKeys: []string{"dGVzdDE=", "dGVzdDI=", "dGVzdDM="},
				},
			},
		},
	}

	err := suite.k8sClient.Create(suite.ctx, config)
	require.NoError(suite.T(), err)

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "chaos-termination-config",
			Namespace: "default",
		},
	}

	// Initial reconciliation to establish baseline
	_, err = suite.reconciler.Reconcile(suite.ctx, req)
	require.NoError(suite.T(), err, "Initial reconciliation should succeed")

	// Simulate chaos by randomly terminating and restarting containers
	chaosRounds := 5
	for round := 0; round < chaosRounds; round++ {
		suite.T().Logf("Chaos round %d: Terminating random container", round+1)

		// Randomly choose which container to terminate
		if rand.Float32() < 0.5 {
			// Terminate primary vault
			suite.T().Log("Terminating primary vault")
			suite.primaryVault.Terminate(suite.ctx)

			// Wait a bit
			time.Sleep(2 * time.Second)

			// Recreate primary vault
			newPrimaryVault, err := vault.Run(suite.ctx,
				"hashicorp/vault:1.19.0",
				vault.WithToken("chaos-primary-token-new"),
				testcontainers.WithWaitStrategy(
					wait.ForHTTP("/v1/sys/health").
						WithStatusCodeMatcher(func(status int) bool {
							return status == 200 || status == 429
						}).
						WithStartupTimeout(120*time.Second),
				),
			)
			if err == nil {
				suite.primaryVault = newPrimaryVault
				newPrimaryAddr, _ := newPrimaryVault.HttpHostAddress(suite.ctx)
				suite.primaryVaultAddr = newPrimaryAddr

				// Update config with new address
				var currentConfig vaultv1.VaultUnsealConfig
				suite.k8sClient.Get(suite.ctx, req.NamespacedName, &currentConfig)
				currentConfig.Spec.VaultInstances[0].Endpoint = suite.primaryVaultAddr
				suite.k8sClient.Update(suite.ctx, &currentConfig)
			}
		} else {
			// Terminate secondary vault
			suite.T().Log("Terminating secondary vault")
			suite.secondaryVault.Terminate(suite.ctx)

			time.Sleep(2 * time.Second)

			// Recreate secondary vault
			newSecondaryVault, err := vault.Run(suite.ctx,
				"hashicorp/vault:1.19.0",
				vault.WithToken("chaos-secondary-token-new"),
				testcontainers.WithWaitStrategy(
					wait.ForHTTP("/v1/sys/health").
						WithStatusCodeMatcher(func(status int) bool {
							return status == 200 || status == 429
						}).
						WithStartupTimeout(120*time.Second),
				),
			)
			if err == nil {
				suite.secondaryVault = newSecondaryVault
				newSecondaryAddr, _ := newSecondaryVault.HttpHostAddress(suite.ctx)
				suite.secondaryVaultAddr = newSecondaryAddr

				// Update config with new address
				var currentConfig vaultv1.VaultUnsealConfig
				suite.k8sClient.Get(suite.ctx, req.NamespacedName, &currentConfig)
				currentConfig.Spec.VaultInstances[1].Endpoint = suite.secondaryVaultAddr
				suite.k8sClient.Update(suite.ctx, &currentConfig)
			}
		}

		// Try reconciliation after chaos
		_, err = suite.reconciler.Reconcile(suite.ctx, req)
		suite.T().Logf("Post-chaos reconciliation result: %v", err)
	}

	suite.T().Log("Container termination chaos test completed")
}

// TestNetworkPartitionSimulation tests behavior during network issues
func (suite *ChaosTestSuite) TestNetworkPartitionSimulation() {
	// Create configurations that will experience "network partitions"
	configCount := 10
	var configs []*vaultv1.VaultUnsealConfig

	for i := 0; i < configCount; i++ {
		config := &vaultv1.VaultUnsealConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("network-chaos-%d", i),
				Namespace: "default",
			},
			Spec: vaultv1.VaultUnsealConfigSpec{
				VaultInstances: []vaultv1.VaultInstance{
					{
						Name:       fmt.Sprintf("vault-%d", i),
						Endpoint:   suite.primaryVaultAddr,
						UnsealKeys: []string{"dGVzdDE=", "dGVzdDI=", "dGVzdDM="},
					},
				},
			},
		}

		err := suite.k8sClient.Create(suite.ctx, config)
		require.NoError(suite.T(), err)
		configs = append(configs, config)
	}

	// Simulate network partitions by pointing configs to invalid endpoints
	partitionRounds := 3
	for round := 0; round < partitionRounds; round++ {
		suite.T().Logf("Network partition round %d", round+1)

		// Randomly partition some configs
		partitionedConfigs := rand.Intn(configCount/2) + 1

		for i := 0; i < partitionedConfigs; i++ {
			configIndex := rand.Intn(len(configs))
			configs[configIndex].Spec.VaultInstances[0].Endpoint = "http://10.255.255.255:8200" // Unreachable
			err := suite.k8sClient.Update(suite.ctx, configs[configIndex])
			require.NoError(suite.T(), err)
		}

		// Try reconciliations during partition
		var wg sync.WaitGroup
		results := make(chan error, len(configs))

		for i, config := range configs {
			wg.Add(1)
			go func(configIndex int, cfg *vaultv1.VaultUnsealConfig) {
				defer wg.Done()

				req := ctrl.Request{
					NamespacedName: types.NamespacedName{
						Name:      cfg.Name,
						Namespace: cfg.Namespace,
					},
				}

				_, err := suite.reconciler.Reconcile(suite.ctx, req)
				results <- err
			}(i, config)
		}

		wg.Wait()
		close(results)

		// Count results
		errorCount := 0
		for err := range results {
			if err != nil {
				errorCount++
			}
		}

		suite.T().Logf("Network partition round %d: %d/%d reconciliations had errors", round+1, errorCount, len(configs))

		// Heal partitions
		for i, config := range configs {
			config.Spec.VaultInstances[0].Endpoint = suite.primaryVaultAddr
			suite.k8sClient.Update(suite.ctx, config)
			configs[i] = config
		}

		// Recovery reconciliations
		recoverySuccesses := 0
		for _, config := range configs {
			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      config.Name,
					Namespace: config.Namespace,
				},
			}

			_, err := suite.reconciler.Reconcile(suite.ctx, req)
			if err == nil {
				recoverySuccesses++
			}
		}

		suite.T().Logf("Recovery: %d/%d reconciliations succeeded", recoverySuccesses, len(configs))
	}
}

// TestRandomizedResourceManipulation tests chaotic resource changes
func (suite *ChaosTestSuite) TestRandomizedResourceManipulation() {
	baseConfig := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "chaos-manipulation",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:       "base-vault",
					Endpoint:   suite.primaryVaultAddr,
					UnsealKeys: []string{"dGVzdDE=", "dGVzdDI=", "dGVzdDM="},
				},
			},
		},
	}

	err := suite.k8sClient.Create(suite.ctx, baseConfig)
	require.NoError(suite.T(), err)

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "chaos-manipulation",
			Namespace: "default",
		},
	}

	manipulationRounds := 20
	successfulReconciliations := 0

	for round := 0; round < manipulationRounds; round++ {
		// Get current config
		var currentConfig vaultv1.VaultUnsealConfig
		err := suite.k8sClient.Get(suite.ctx, req.NamespacedName, &currentConfig)
		require.NoError(suite.T(), err)

		// Randomly manipulate the configuration
		manipulation := rand.Intn(6)
		switch manipulation {
		case 0:
			// Change endpoint
			if rand.Float32() < 0.3 {
				currentConfig.Spec.VaultInstances[0].Endpoint = "http://chaos.invalid:8200"
			} else {
				currentConfig.Spec.VaultInstances[0].Endpoint = suite.primaryVaultAddr
			}
		case 1:
			// Modify unseal keys
			if rand.Float32() < 0.3 {
				currentConfig.Spec.VaultInstances[0].UnsealKeys = []string{"invalid"}
			} else {
				currentConfig.Spec.VaultInstances[0].UnsealKeys = []string{"dGVzdDE=", "dGVzdDI=", "dGVzdDM="}
			}
		case 2:
			// Add/remove vault instances
			if len(currentConfig.Spec.VaultInstances) == 1 && rand.Float32() < 0.5 {
				// Add instance
				currentConfig.Spec.VaultInstances = append(currentConfig.Spec.VaultInstances, vaultv1.VaultInstance{
					Name:       "added-vault",
					Endpoint:   suite.secondaryVaultAddr,
					UnsealKeys: []string{"dGVzdDE=", "dGVzdDI=", "dGVzdDM="},
				})
			} else if len(currentConfig.Spec.VaultInstances) > 1 {
				// Remove instance
				currentConfig.Spec.VaultInstances = currentConfig.Spec.VaultInstances[:1]
			}
		case 3:
			// Modify threshold
			threshold := rand.Intn(5) + 1
			currentConfig.Spec.VaultInstances[0].Threshold = &threshold
		case 4:
			// Toggle TLS settings
			tlsSkip := rand.Float32() < 0.5
			currentConfig.Spec.VaultInstances[0].TLSSkipVerify = tlsSkip
		case 5:
			// Modify labels/annotations
			if currentConfig.Labels == nil {
				currentConfig.Labels = make(map[string]string)
			}
			currentConfig.Labels[fmt.Sprintf("chaos-round")] = fmt.Sprintf("%d", round)
		}

		// Apply the manipulation
		err = suite.k8sClient.Update(suite.ctx, &currentConfig)
		require.NoError(suite.T(), err)

		// Try reconciliation
		_, err = suite.reconciler.Reconcile(suite.ctx, req)
		if err == nil {
			successfulReconciliations++
		}

		// Brief pause
		time.Sleep(time.Duration(rand.Intn(100)) * time.Millisecond)
	}

	suite.T().Logf("Random manipulation: %d/%d reconciliations succeeded", successfulReconciliations, manipulationRounds)
	assert.Greater(suite.T(), successfulReconciliations, manipulationRounds/3, "At least 1/3 of chaotic reconciliations should succeed")
}

// TestConcurrentChaosOperations tests multiple chaos scenarios simultaneously
func (suite *ChaosTestSuite) TestConcurrentChaosOperations() {
	configCount := 20
	var configs []*vaultv1.VaultUnsealConfig

	// Create multiple configs
	for i := 0; i < configCount; i++ {
		config := &vaultv1.VaultUnsealConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("concurrent-chaos-%d", i),
				Namespace: "default",
			},
			Spec: vaultv1.VaultUnsealConfigSpec{
				VaultInstances: []vaultv1.VaultInstance{
					{
						Name:       fmt.Sprintf("concurrent-vault-%d", i),
						Endpoint:   suite.primaryVaultAddr,
						UnsealKeys: []string{"dGVzdDE=", "dGVzdDI=", "dGVzdDM="},
					},
				},
			},
		}

		err := suite.k8sClient.Create(suite.ctx, config)
		require.NoError(suite.T(), err)
		configs = append(configs, config)
	}

	// Launch concurrent chaos operations
	var wg sync.WaitGroup
	results := make(chan bool, configCount*10) // 10 operations per config

	for i, config := range configs {
		wg.Add(1)
		go func(configIndex int, cfg *vaultv1.VaultUnsealConfig) {
			defer wg.Done()

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      cfg.Name,
					Namespace: cfg.Namespace,
				},
			}

			// Perform multiple chaotic operations
			for j := 0; j < 10; j++ {
				// Random chaos
				chaosType := rand.Intn(4)

				switch chaosType {
				case 0:
					// Reconciliation attempt
					_, err := suite.reconciler.Reconcile(suite.ctx, req)
					results <- (err == nil)
				case 1:
					// Config update
					var currentConfig vaultv1.VaultUnsealConfig
					err := suite.k8sClient.Get(suite.ctx, req.NamespacedName, &currentConfig)
					if err == nil {
						currentConfig.Spec.VaultInstances[0].Endpoint = suite.secondaryVaultAddr
						err = suite.k8sClient.Update(suite.ctx, &currentConfig)
						results <- (err == nil)
					} else {
						results <- false
					}
				case 2:
					// Client creation chaos
					client, err := vaultpkg.NewClient(suite.primaryVaultAddr, false, 5*time.Second)
					if err == nil {
						_, healthErr := client.HealthCheck(suite.ctx)
						client.Close()
						results <- (healthErr == nil)
					} else {
						results <- false
					}
				case 3:
					// Status update simulation
					var currentConfig vaultv1.VaultUnsealConfig
					err := suite.k8sClient.Get(suite.ctx, req.NamespacedName, &currentConfig)
					if err == nil {
						currentConfig.Status.VaultStatuses = []vaultv1.VaultInstanceStatus{
							{
								Name:   fmt.Sprintf("concurrent-vault-%d", configIndex),
								Sealed: rand.Float32() < 0.5,
							},
						}
						err = suite.k8sClient.Status().Update(suite.ctx, &currentConfig)
						results <- (err == nil)
					} else {
						results <- false
					}
				}

				// Random delay
				time.Sleep(time.Duration(rand.Intn(50)) * time.Millisecond)
			}
		}(i, config)
	}

	wg.Wait()
	close(results)

	// Analyze results
	successCount := 0
	totalOps := 0
	for success := range results {
		totalOps++
		if success {
			successCount++
		}
	}

	suite.T().Logf("Concurrent chaos operations: %d/%d succeeded (%.2f%%)",
		successCount, totalOps, float64(successCount)/float64(totalOps)*100)

	assert.Greater(suite.T(), successCount, totalOps/4, "At least 25% of concurrent chaotic operations should succeed")
}

// TestChaosTestSuite runs the chaos engineering test suite
func TestChaosTestSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping chaos tests in short mode")
	}

	suite.Run(t, new(ChaosTestSuite))
}
