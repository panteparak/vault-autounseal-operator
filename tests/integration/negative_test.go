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

// NegativeIntegrationTestSuite provides comprehensive negative testing scenarios
type NegativeIntegrationTestSuite struct {
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

// SetupSuite initializes the test environment
func (suite *NegativeIntegrationTestSuite) SetupSuite() {
	suite.ctx, suite.ctxCancel = context.WithTimeout(context.Background(), 30*time.Minute)
	
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	// Set up K3s and Vault for negative testing
	suite.setupEnvironment()
}

// setupEnvironment creates the test environment
func (suite *NegativeIntegrationTestSuite) setupEnvironment() {
	// Create K3s cluster
	suite.setupK3s()
	
	// Create a working Vault for comparison tests
	suite.setupVault()
	
	// Set up controller
	suite.setupController()
}

// setupK3s creates K3s cluster with CRDs
func (suite *NegativeIntegrationTestSuite) setupK3s() {
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
		// TODO: Use proper k3s manifest loading
		// k3s.WithManifest("vault-crd.yaml", crdManifest),
		testcontainers.WithWaitStrategy(
			wait.ForLog("k3s is up and running").
				WithStartupTimeout(180*time.Second).
				WithPollInterval(5*time.Second),
		),
	)
	require.NoError(suite.T(), err, "Failed to start K3s")
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

	// Wait for CRDs to be ready
	suite.waitForCRDs()
}

// setupVault creates a working Vault instance
func (suite *NegativeIntegrationTestSuite) setupVault() {
	vaultContainer, err := vault.Run(suite.ctx,
		"hashicorp/vault:1.19.0",
		vault.WithToken("test-root-token"),
		testcontainers.WithWaitStrategy(
			wait.ForHTTP("/v1/sys/health").
				WithStatusCodeMatcher(func(status int) bool {
					return status == 200 || status == 429
				}).
				WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(suite.T(), err)
	suite.vaultContainer = vaultContainer

	vaultAddr, err := vaultContainer.HttpHostAddress(suite.ctx)
	require.NoError(suite.T(), err)
	suite.vaultAddr = vaultAddr
}

// setupController creates the controller
func (suite *NegativeIntegrationTestSuite) setupController() {
	suite.reconciler = &controller.VaultUnsealConfigReconciler{
		Client: suite.k8sClient,
		Log:    ctrl.Log.WithName("negative-test-controller"),
		Scheme: suite.scheme,
	}
}

// waitForCRDs waits for CRDs to be ready
func (suite *NegativeIntegrationTestSuite) waitForCRDs() {
	require.Eventually(suite.T(), func() bool {
		testConfig := &vaultv1.VaultUnsealConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "crd-test",
				Namespace: "default",
			},
			Spec: vaultv1.VaultUnsealConfigSpec{
				VaultInstances: []vaultv1.VaultInstance{
					{Name: "test", Endpoint: "http://test", UnsealKeys: []string{"test"}},
				},
			},
		}
		
		err := suite.k8sClient.Create(suite.ctx, testConfig)
		if err == nil {
			suite.k8sClient.Delete(suite.ctx, testConfig)
			return true
		}
		return false
	}, 90*time.Second, 3*time.Second)
}

// TearDownSuite cleans up resources
func (suite *NegativeIntegrationTestSuite) TearDownSuite() {
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
func (suite *NegativeIntegrationTestSuite) TearDownTest() {
	configList := &vaultv1.VaultUnsealConfigList{}
	err := suite.k8sClient.List(suite.ctx, configList)
	if err == nil {
		for _, config := range configList.Items {
			suite.k8sClient.Delete(suite.ctx, &config)
		}
	}
}

// TestInvalidVaultEndpoints tests various invalid Vault endpoint scenarios
func (suite *NegativeIntegrationTestSuite) TestInvalidVaultEndpoints() {
	tests := []struct {
		name        string
		endpoint    string
		description string
	}{
		{
			name:        "nonexistent host",
			endpoint:    "http://nonexistent.vault.local:8200",
			description: "should handle DNS resolution failure",
		},
		{
			name:        "wrong port",
			endpoint:    "http://localhost:9999",
			description: "should handle connection refused",
		},
		{
			name:        "invalid scheme",
			endpoint:    "ftp://localhost:8200",
			description: "should handle invalid URL scheme",
		},
		{
			name:        "malformed URL",
			endpoint:    "not-a-valid-url",
			description: "should handle malformed URL",
		},
		{
			name:        "unreachable https",
			endpoint:    "https://192.168.255.255:8200",
			description: "should handle unreachable HTTPS endpoint",
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			config := &vaultv1.VaultUnsealConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-endpoint-test",
					Namespace: "default",
				},
				Spec: vaultv1.VaultUnsealConfigSpec{
					VaultInstances: []vaultv1.VaultInstance{
						{
							Name:       "invalid-vault",
							Endpoint:   tt.endpoint,
							UnsealKeys: []string{"dGVzdA==", "dGVzdA==", "dGVzdA=="},
						},
					},
				},
			}

			err := suite.k8sClient.Create(suite.ctx, config)
			require.NoError(suite.T(), err)

			// Reconcile should not error, but should record the failure
			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "invalid-endpoint-test",
					Namespace: "default",
				},
			}

			_, err = suite.reconciler.Reconcile(suite.ctx, req)
			require.NoError(suite.T(), err, "Reconciler should handle endpoint failures gracefully")

			// Verify error is recorded in status
			var updatedConfig vaultv1.VaultUnsealConfig
			err = suite.k8sClient.Get(suite.ctx, req.NamespacedName, &updatedConfig)
			require.NoError(suite.T(), err)

			assert.NotEmpty(suite.T(), updatedConfig.Status.VaultStatuses)
			assert.True(suite.T(), updatedConfig.Status.VaultStatuses[0].Sealed)
			assert.NotEmpty(suite.T(), updatedConfig.Status.VaultStatuses[0].Error)
			
			suite.T().Logf("%s: Error recorded: %s", tt.description, updatedConfig.Status.VaultStatuses[0].Error)
		})
	}
}

// TestInvalidUnsealKeys tests various invalid unseal key scenarios
func (suite *NegativeIntegrationTestSuite) TestInvalidUnsealKeys() {
	tests := []struct {
		name        string
		keys        []string
		threshold   *int
		description string
	}{
		{
			name:        "empty keys",
			keys:        []string{},
			threshold:   nil,
			description: "should handle empty unseal keys",
		},
		{
			name:        "invalid base64 keys",
			keys:        []string{"not-base64", "also-not-base64"},
			threshold:   nil,
			description: "should handle malformed base64 keys",
		},
		{
			name:        "threshold too high",
			keys:        []string{"dGVzdA==", "dGVzdA=="},
			threshold:   func() *int { i := 5; return &i }(),
			description: "should handle threshold exceeding key count",
		},
		{
			name:        "zero threshold",
			keys:        []string{"dGVzdA==", "dGVzdA==", "dGVzdA=="},
			threshold:   func() *int { i := 0; return &i }(),
			description: "should handle zero threshold",
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			config := &vaultv1.VaultUnsealConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-keys-test",
					Namespace: "default",
				},
				Spec: vaultv1.VaultUnsealConfigSpec{
					VaultInstances: []vaultv1.VaultInstance{
						{
							Name:       "test-vault",
							Endpoint:   suite.vaultAddr,
							UnsealKeys: tt.keys,
							Threshold:  tt.threshold,
						},
					},
				},
			}

			err := suite.k8sClient.Create(suite.ctx, config)
			require.NoError(suite.T(), err)

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      "invalid-keys-test",
					Namespace: "default",
				},
			}

			_, err = suite.reconciler.Reconcile(suite.ctx, req)
			require.NoError(suite.T(), err, "Reconciler should handle invalid keys gracefully")

			suite.T().Logf("%s: Handled gracefully", tt.description)
		})
	}
}

// TestTimeoutScenarios tests timeout and resource exhaustion scenarios
func (suite *NegativeIntegrationTestSuite) TestTimeoutScenarios() {
	// Test with extremely slow/unresponsive endpoint using a non-routable IP
	config := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "timeout-test",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:       "timeout-vault",
					Endpoint:   "http://10.255.255.1:8200", // Non-routable IP for timeout
					UnsealKeys: []string{"dGVzdA==", "dGVzdA==", "dGVzdA=="},
				},
			},
		},
	}

	err := suite.k8sClient.Create(suite.ctx, config)
	require.NoError(suite.T(), err)

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "timeout-test",
			Namespace: "default",
		},
	}

	// Reconciliation should handle timeout gracefully
	start := time.Now()
	_, err = suite.reconciler.Reconcile(suite.ctx, req)
	duration := time.Since(start)

	require.NoError(suite.T(), err, "Should handle timeouts gracefully")
	assert.Less(suite.T(), duration, 2*time.Minute, "Should not hang indefinitely")

	suite.T().Logf("Timeout scenario handled in %v", duration)
}

// TestResourceExhaustion tests resource exhaustion scenarios
func (suite *NegativeIntegrationTestSuite) TestResourceExhaustion() {
	// Create many concurrent clients to test resource limits
	concurrency := 50
	results := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		go func(id int) {
			// Try to create vault client to non-existent endpoint
			client, err := vaultpkg.NewClient("http://localhost:9999", false, 5*time.Second)
			if err != nil {
				results <- err
				return
			}
			defer client.Close()

			// Try to perform health check (should fail)
			_, err = client.HealthCheck(suite.ctx)
			results <- err
		}(i)
	}

	// Collect results
	errorCount := 0
	for i := 0; i < concurrency; i++ {
		select {
		case err := <-results:
			if err != nil {
				errorCount++
			}
		case <-time.After(30 * time.Second):
			suite.T().Fatal("Timeout waiting for resource exhaustion test")
		}
	}

	// Should handle resource exhaustion gracefully (most connections should fail)
	assert.Greater(suite.T(), errorCount, concurrency/2, "Should handle resource limits")
	suite.T().Logf("Resource exhaustion test: %d/%d operations failed as expected", errorCount, concurrency)
}

// TestInvalidConfigurations tests invalid configuration combinations
func (suite *NegativeIntegrationTestSuite) TestInvalidConfigurations() {
	tests := []struct {
		name   string
		config *vaultv1.VaultUnsealConfig
		reason string
	}{
		{
			name: "conflicting instance names",
			config: &vaultv1.VaultUnsealConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "conflicting-names",
					Namespace: "default",
				},
				Spec: vaultv1.VaultUnsealConfigSpec{
					VaultInstances: []vaultv1.VaultInstance{
						{
							Name:       "duplicate",
							Endpoint:   suite.vaultAddr,
							UnsealKeys: []string{"dGVzdA=="},
						},
						{
							Name:       "duplicate",
							Endpoint:   "http://vault2.com",
							UnsealKeys: []string{"dGVzdA=="},
						},
					},
				},
			},
			reason: "duplicate instance names should be handled",
		},
		{
			name: "mixed secure and insecure endpoints",
			config: &vaultv1.VaultUnsealConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "mixed-security",
					Namespace: "default",
				},
				Spec: vaultv1.VaultUnsealConfigSpec{
					VaultInstances: []vaultv1.VaultInstance{
						{
							Name:       "secure",
							Endpoint:   "https://vault-secure.com:8200",
							UnsealKeys: []string{"dGVzdA=="},
						},
						{
							Name:       "insecure",
							Endpoint:   "http://vault-insecure.com:8200",
							UnsealKeys: []string{"dGVzdA=="},
						},
					},
				},
			},
			reason: "mixed security configuration should be handled",
		},
	}

	for _, tt := range tests {
		suite.Run(tt.name, func() {
			err := suite.k8sClient.Create(suite.ctx, tt.config)
			require.NoError(suite.T(), err)

			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      tt.config.Name,
					Namespace: tt.config.Namespace,
				},
			}

			_, err = suite.reconciler.Reconcile(suite.ctx, req)
			require.NoError(suite.T(), err, "Should handle configuration issues gracefully")

			suite.T().Logf("%s: %s", tt.name, tt.reason)
		})
	}
}

// TestConcurrentFailures tests concurrent failure scenarios
func (suite *NegativeIntegrationTestSuite) TestConcurrentFailures() {
	// Create multiple configs that will fail concurrently
	concurrency := 10
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			config := &vaultv1.VaultUnsealConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      fmt.Sprintf("concurrent-fail-%d", id),
					Namespace: "default",
				},
				Spec: vaultv1.VaultUnsealConfigSpec{
					VaultInstances: []vaultv1.VaultInstance{
						{
							Name:       fmt.Sprintf("fail-vault-%d", id),
							Endpoint:   fmt.Sprintf("http://nonexistent-%d.vault.local:8200", id),
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

			_, err = suite.reconciler.Reconcile(suite.ctx, req)
			require.NoError(suite.T(), err, "Concurrent failure should be handled")
		}(i)
	}

	wg.Wait()
	suite.T().Log("All concurrent failure scenarios handled successfully")
}

// TestNegativeIntegrationTestSuite runs the negative integration test suite
func TestNegativeIntegrationTestSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping negative integration tests in short mode")
	}

	suite.Run(t, new(NegativeIntegrationTestSuite))
}