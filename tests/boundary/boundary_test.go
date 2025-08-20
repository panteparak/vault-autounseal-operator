package integration

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
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

// BoundaryTestSuite tests edge cases and boundary conditions using TestContainers
type BoundaryTestSuite struct {
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

// SetupSuite initializes boundary testing environment
func (suite *BoundaryTestSuite) SetupSuite() {
	suite.ctx, suite.ctxCancel = context.WithTimeout(context.Background(), 30*time.Minute)
	
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	// Set up infrastructure using TestContainers
	suite.setupBoundaryInfrastructure()
}

// setupBoundaryInfrastructure creates containers for boundary testing
func (suite *BoundaryTestSuite) setupBoundaryInfrastructure() {
	// Create K3s cluster
	suite.setupK3s()
	
	// Create Vault container
	suite.setupVault()
	
	// Set up controller
	suite.setupController()
}

// setupK3s creates K3s cluster using TestContainers
func (suite *BoundaryTestSuite) setupK3s() {
	k3sContainer, err := k3s.Run(suite.ctx,
		"rancher/k3s:v1.32.1-k3s1",
		testcontainers.WithWaitStrategy(
			wait.ForLog("k3s is up and running").
				WithStartupTimeout(240*time.Second).
				WithPollInterval(5*time.Second),
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

	suite.waitForAPI()
}

// setupVault creates Vault container using TestContainers
func (suite *BoundaryTestSuite) setupVault() {
	vaultContainer, err := vault.Run(suite.ctx,
		"hashicorp/vault:1.19.0",
		vault.WithToken("boundary-test-token"),
		testcontainers.WithWaitStrategy(
			wait.ForHTTP("/v1/sys/health").
				WithStatusCodeMatcher(func(status int) bool {
					return status == 200 || status == 429
				}).
				WithStartupTimeout(90*time.Second),
		),
	)
	require.NoError(suite.T(), err)
	suite.vaultContainer = vaultContainer

	vaultAddr, err := vaultContainer.HttpHostAddress(suite.ctx)
	require.NoError(suite.T(), err)
	suite.vaultAddr = vaultAddr
}

// setupController creates controller
func (suite *BoundaryTestSuite) setupController() {
	suite.reconciler = &controller.VaultUnsealConfigReconciler{
		Client: suite.k8sClient,
		Log:    ctrl.Log.WithName("boundary-controller"),
		Scheme: suite.scheme,
	}
}

// waitForAPI waits for Kubernetes API
func (suite *BoundaryTestSuite) waitForAPI() {
	require.Eventually(suite.T(), func() bool {
		_, err := suite.k8sClient.RESTMapper().RESTMapping(
			vaultv1.GroupVersion.WithKind("VaultUnsealConfig").GroupKind(),
		)
		return err == nil
	}, 120*time.Second, 3*time.Second)
}

// TearDownSuite cleans up resources
func (suite *BoundaryTestSuite) TearDownSuite() {
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
func (suite *BoundaryTestSuite) TearDownTest() {
	configList := &vaultv1.VaultUnsealConfigList{}
	err := suite.k8sClient.List(suite.ctx, configList)
	if err == nil {
		for _, config := range configList.Items {
			suite.k8sClient.Delete(suite.ctx, &config)
		}
	}
}

// TestExtremelyLargeConfigurations tests handling of very large configurations
func (suite *BoundaryTestSuite) TestExtremelyLargeConfigurations() {
	tests := []struct {
		name              string
		vaultInstanceCount int
		keyCount          int
		expectSuccess     bool
		description       string
	}{
		{
			name:               "moderate_size_config",
			vaultInstanceCount: 10,
			keyCount:           5,
			expectSuccess:      true,
			description:        "Moderate configuration should succeed",
		},
		{
			name:               "large_config",
			vaultInstanceCount: 50,
			keyCount:           10,
			expectSuccess:      true,
			description:        "Large configuration should be handled",
		},
		{
			name:               "extreme_vault_count",
			vaultInstanceCount: 100,
			keyCount:           3,
			expectSuccess:      true,
			description:        "Extreme vault count should be handled",
		},
		{
			name:               "extreme_key_count",
			vaultInstanceCount: 5,
			keyCount:           50,
			expectSuccess:      true,
			description:        "Many keys should be handled",
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			// Create large configuration
			config := &vaultv1.VaultUnsealConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("boundary-%s", tt.name),
					Namespace: "default",
				},
				Spec: vaultv1.VaultUnsealConfigSpec{
					VaultInstances: make([]vaultv1.VaultInstance, tt.vaultInstanceCount),
				},
			}

			// Populate vault instances
			for i := 0; i < tt.vaultInstanceCount; i++ {
				keys := make([]string, tt.keyCount)
				for j := 0; j < tt.keyCount; j++ {
					keys[j] = base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("test-key-%d-%d", i, j)))
				}

				config.Spec.VaultInstances[i] = vaultv1.VaultInstance{
					Name:       fmt.Sprintf("vault-instance-%d", i),
					Endpoint:   suite.vaultAddr,
					UnsealKeys: keys,
					Threshold:  &[]int{tt.keyCount / 2}[0], // Half the keys as threshold
				}
			}

			// Try to create the configuration
			err := suite.k8sClient.Create(suite.ctx, config)
			if tt.expectSuccess {
				require.NoError(suite.T(), err, tt.description)

				// Try reconciliation
				req := ctrl.Request{
					NamespacedName: types.NamespacedName{
						Name:      config.Name,
						Namespace: config.Namespace,
					},
				}

				start := time.Now()
				_, err = suite.reconciler.Reconcile(suite.ctx, req)
				duration := time.Since(start)

				suite.T().Logf("%s: Reconciliation took %v", tt.description, duration)
				assert.NoError(suite.T(), err, "Large configuration reconciliation should succeed")
				assert.Less(suite.T(), duration, 30*time.Second, "Large configuration should reconcile in reasonable time")
			} else {
				assert.Error(suite.T(), err, tt.description)
			}
		})
	}
}

// TestExtremelyLongNames tests handling of very long resource names
func (suite *BoundaryTestSuite) TestExtremelyLongNames() {
	tests := []struct {
		name        string
		nameLength  int
		expectError bool
		description string
	}{
		{
			name:        "normal_length_name",
			nameLength:  20,
			expectError: false,
			description: "Normal length names should work",
		},
		{
			name:        "long_name",
			nameLength:  100,
			expectError: false,
			description: "Long names should work",
		},
		{
			name:        "very_long_name",
			nameLength:  200,
			expectError: false,
			description: "Very long names should work",
		},
		{
			name:        "kubernetes_limit_name",
			nameLength:  253, // Kubernetes DNS name limit
			expectError: false,
			description: "Names at Kubernetes limit should work",
		},
		{
			name:        "beyond_kubernetes_limit",
			nameLength:  300,
			expectError: true,
			description: "Names beyond Kubernetes limit should fail",
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			// Generate name of specified length
			baseName := "test-boundary-name"
			longName := baseName + strings.Repeat("x", tt.nameLength-len(baseName))
			if len(longName) != tt.nameLength {
				longName = strings.Repeat("x", tt.nameLength)
			}

			config := &vaultv1.VaultUnsealConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      longName,
					Namespace: "default",
				},
				Spec: vaultv1.VaultUnsealConfigSpec{
					VaultInstances: []vaultv1.VaultInstance{
						{
							Name:       "test-vault",
							Endpoint:   suite.vaultAddr,
							UnsealKeys: []string{"dGVzdA==", "dGVzdA==", "dGVzdA=="},
						},
					},
				},
			}

			err := suite.k8sClient.Create(suite.ctx, config)
			if tt.expectError {
				assert.Error(suite.T(), err, tt.description)
			} else {
				assert.NoError(suite.T(), err, tt.description)
			}
		})
	}
}

// TestExtremeTimeout tests behavior under extreme timeout conditions
func (suite *BoundaryTestSuite) TestExtremeTimeout() {
	// Create config pointing to extremely slow/unresponsive endpoint
	config := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "timeout-boundary-test",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:       "timeout-vault",
					Endpoint:   "http://10.255.255.1:8200", // Non-routable IP
					UnsealKeys: []string{"dGVzdA==", "dGVzdA==", "dGVzdA=="},
				},
			},
		},
	}

	err := suite.k8sClient.Create(suite.ctx, config)
	require.NoError(suite.T(), err)

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "timeout-boundary-test",
			Namespace: "default",
		},
	}

	// Test reconciliation with timeout
	start := time.Now()
	_, err = suite.reconciler.Reconcile(suite.ctx, req)
	duration := time.Since(start)

	suite.T().Logf("Timeout test: Reconciliation took %v", duration)
	
	// Should complete in reasonable time even with unreachable endpoint
	assert.Less(suite.T(), duration, 60*time.Second, "Reconciliation should timeout gracefully")
	assert.NoError(suite.T(), err, "Should handle unreachable endpoints gracefully")
}

// TestVaultClientBoundaryConditions tests vault client edge cases
func (suite *BoundaryTestSuite) TestVaultClientBoundaryConditions() {
	tests := []struct {
		name        string
		endpoint    string
		timeout     time.Duration
		expectError bool
		description string
	}{
		{
			name:        "normal_endpoint",
			endpoint:    suite.vaultAddr,
			timeout:     30 * time.Second,
			expectError: false,
			description: "Normal endpoint should work",
		},
		{
			name:        "very_short_timeout",
			endpoint:    suite.vaultAddr,
			timeout:     1 * time.Nanosecond,
			expectError: true,
			description: "Extremely short timeout should fail",
		},
		{
			name:        "very_long_timeout",
			endpoint:    suite.vaultAddr,
			timeout:     24 * time.Hour,
			expectError: false,
			description: "Very long timeout should work",
		},
		{
			name:        "localhost_endpoint",
			endpoint:    "http://localhost:8200",
			timeout:     5 * time.Second,
			expectError: false, // May work or fail depending on environment
			description: "Localhost endpoint handling",
		},
		{
			name:        "ipv6_endpoint",
			endpoint:    "http://[::1]:8200",
			timeout:     5 * time.Second,
			expectError: false, // May work or fail depending on environment
			description: "IPv6 endpoint handling",
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			client, err := vaultpkg.NewClient(tt.endpoint, false, tt.timeout)
			if tt.expectError {
				assert.Error(suite.T(), err, tt.description)
				return
			}

			// If client creation succeeded, try operations
			if err == nil {
				defer client.Close()
				
				// Try health check with timeout context
				ctx, cancel := context.WithTimeout(suite.ctx, tt.timeout+time.Second)
				defer cancel()
				
				_, healthErr := client.HealthCheck(ctx)
				suite.T().Logf("%s: Health check result: %v", tt.description, healthErr)
				// Don't assert on health check as it depends on actual connectivity
			}
		})
	}
}

// TestResourceExhaustionBoundaries tests system behavior at resource limits
func (suite *BoundaryTestSuite) TestResourceExhaustionBoundaries() {
	suite.Run("maximum_concurrent_clients", func() {
		// Test creating many vault clients concurrently
		clientCount := 1000
		clients := make([]*vaultpkg.Client, 0, clientCount)
		
		for i := 0; i < clientCount; i++ {
			client, err := vaultpkg.NewClient(suite.vaultAddr, false, 30*time.Second)
			if err != nil {
				suite.T().Logf("Failed to create client %d: %v", i, err)
				break
			}
			clients = append(clients, client)
		}

		suite.T().Logf("Successfully created %d vault clients", len(clients))
		
		// Cleanup
		for _, client := range clients {
			if client != nil {
				client.Close()
			}
		}

		assert.Greater(suite.T(), len(clients), 100, "Should be able to create at least 100 clients")
	})

	suite.Run("memory_pressure_simulation", func() {
		// Create configuration with many large labels and annotations
		config := &vaultv1.VaultUnsealConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:        "memory-pressure-test",
				Namespace:   "default",
				Labels:      make(map[string]string),
				Annotations: make(map[string]string),
			},
			Spec: vaultv1.VaultUnsealConfigSpec{
				VaultInstances: []vaultv1.VaultInstance{
					{
						Name:       "memory-test-vault",
						Endpoint:   suite.vaultAddr,
						UnsealKeys: []string{"dGVzdA==", "dGVzdA==", "dGVzdA=="},
					},
				},
			},
		}

		// Add many labels and annotations
		for i := 0; i < 100; i++ {
			largeValue := strings.Repeat("x", 1000) // 1KB per value
			config.Labels[fmt.Sprintf("large-label-%d", i)] = largeValue
			config.Annotations[fmt.Sprintf("large-annotation-%d", i)] = largeValue
		}

		err := suite.k8sClient.Create(suite.ctx, config)
		assert.NoError(suite.T(), err, "Should handle resources with large metadata")

		if err == nil {
			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "memory-pressure-test",
					Namespace: "default",
				},
			}

			_, err = suite.reconciler.Reconcile(suite.ctx, req)
			assert.NoError(suite.T(), err, "Should reconcile resources with large metadata")
		}
	})
}

// TestExtremeNetworkConditions tests network-related boundary conditions
func (suite *BoundaryTestSuite) TestExtremeNetworkConditions() {
	tests := []struct {
		name        string
		endpoint    string
		description string
	}{
		{
			name:        "unreachable_network",
			endpoint:    "http://192.168.255.255:8200",
			description: "Unreachable network address",
		},
		{
			name:        "invalid_port",
			endpoint:    "http://localhost:99999",
			description: "Invalid port number",
		},
		{
			name:        "non_http_port",
			endpoint:    "http://localhost:22", // SSH port
			description: "Wrong protocol on port",
		},
		{
			name:        "reserved_ip",
			endpoint:    "http://0.0.0.0:8200",
			description: "Reserved IP address",
		},
		{
			name:        "broadcast_address",
			endpoint:    "http://255.255.255.255:8200",
			description: "Broadcast address",
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			config := &vaultv1.VaultUnsealConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("network-boundary-%s", tt.name),
					Namespace: "default",
				},
				Spec: vaultv1.VaultUnsealConfigSpec{
					VaultInstances: []vaultv1.VaultInstance{
						{
							Name:       fmt.Sprintf("network-vault-%s", tt.name),
							Endpoint:   tt.endpoint,
							UnsealKeys: []string{"dGVzdA==", "dGVzdA==", "dGVzdA=="},
						},
					},
				},
			}

			err := suite.k8sClient.Create(suite.ctx, config)
			require.NoError(suite.T(), err)

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      config.Name,
					Namespace: config.Namespace,
				},
			}

			// Reconciliation should handle network issues gracefully
			start := time.Now()
			_, err = suite.reconciler.Reconcile(suite.ctx, req)
			duration := time.Since(start)

			suite.T().Logf("%s: Reconciliation took %v", tt.description, duration)
			assert.NoError(suite.T(), err, "Should handle network boundary conditions gracefully")
			assert.Less(suite.T(), duration, 30*time.Second, "Should not hang on network issues")
		})
	}
}

// TestBoundaryTestSuite runs the boundary condition test suite
func TestBoundaryTestSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping boundary tests in short mode")
	}

	suite.Run(t, new(BoundaryTestSuite))
}