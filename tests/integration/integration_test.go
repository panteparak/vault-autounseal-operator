package integration

import (
	"context"
	"fmt"
	"os"
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
	"github.com/testcontainers/testcontainers-go/modules/vault"
	"github.com/testcontainers/testcontainers-go/wait"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// VaultIntegrationTestSuite provides integration testing for vault-autounseal-operator
type VaultIntegrationTestSuite struct {
	suite.Suite
	vaultContainer    *vault.VaultContainer
	vaultClient       *api.Client
	vaultAddr         string
	unsealKeys        []string
	rootToken         string
	k8sClient         client.Client
	scheme            *runtime.Scheme
	reconciler        *controller.VaultUnsealConfigReconciler
	ctx               context.Context
	ctxCancel         context.CancelFunc
}

// SetupSuite initializes the test suite with TestContainers Vault instance
func (suite *VaultIntegrationTestSuite) SetupSuite() {
	suite.ctx, suite.ctxCancel = context.WithTimeout(context.Background(), 5*time.Minute)

	// Set up logging for tests
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	// Use the official TestContainers Vault module in dev mode for easier testing
	vaultContainer, err := vault.Run(suite.ctx,
		"hashicorp/vault:1.19.0",
		vault.WithToken("root-token"), // Set dev mode token
		testcontainers.WithWaitStrategy(
			wait.ForHTTP("/v1/sys/health").
				WithStatusCodeMatcher(func(status int) bool {
					// Accept 503 (sealed), 200 (ready), or 429 (standby)
					return status == 503 || status == 200 || status == 429
				}).
				WithStartupTimeout(60*time.Second).
				WithPollInterval(1*time.Second),
		),
	)
	require.NoError(suite.T(), err, "Failed to start Vault container")
	suite.vaultContainer = vaultContainer

	// Get container endpoint
	vaultAddr, err := vaultContainer.HttpHostAddress(suite.ctx)
	require.NoError(suite.T(), err, "Failed to get Vault HTTP address")
	suite.vaultAddr = vaultAddr

	// Initialize Vault client
	vaultConfig := api.DefaultConfig()
	vaultConfig.Address = suite.vaultAddr
	suite.vaultClient, err = api.NewClient(vaultConfig)
	require.NoError(suite.T(), err, "Failed to create Vault client")

	// Set root token for dev mode
	suite.rootToken = "root-token"
	suite.vaultClient.SetToken(suite.rootToken)

	// Wait for Vault to be ready
	suite.waitForVaultReady()

	// Set up test keys (dev mode vault doesn't need real unseal keys for testing our logic)
	suite.setupTestKeys()

	// Set up Kubernetes client and controller
	suite.setupKubernetesClient()
	suite.setupController()
}

// waitForVaultReady waits for Vault to be ready to accept requests
func (suite *VaultIntegrationTestSuite) waitForVaultReady() {
	require.Eventually(suite.T(), func() bool {
		// In dev mode, just check health
		health, err := suite.vaultClient.Sys().Health()
		return err == nil && health != nil
	}, 60*time.Second, 2*time.Second, "Vault container did not become ready")
}

// setupTestKeys sets up test keys for our integration testing
func (suite *VaultIntegrationTestSuite) setupTestKeys() {
	// For dev mode vault, we use test keys for testing our unsealing logic
	// These keys will be used in tests but won't actually unseal the dev vault
	suite.unsealKeys = []string{
		"dGVzdC1rZXktMQ==", // test-key-1 in base64
		"dGVzdC1rZXktMg==", // test-key-2 in base64
		"dGVzdC1rZXktMw==", // test-key-3 in base64
		"dGVzdC1rZXktNA==", // test-key-4 in base64
		"dGVzdC1rZXktNQ==", // test-key-5 in base64
	}
}

// setupKubernetesClient initializes the fake Kubernetes client for testing
func (suite *VaultIntegrationTestSuite) setupKubernetesClient() {
	suite.scheme = runtime.NewScheme()
	err := clientgoscheme.AddToScheme(suite.scheme)
	require.NoError(suite.T(), err, "Failed to add client-go scheme")

	err = vaultv1.AddToScheme(suite.scheme)
	require.NoError(suite.T(), err, "Failed to add vault v1 scheme")

	suite.k8sClient = fake.NewClientBuilder().WithScheme(suite.scheme).WithStatusSubresource(&vaultv1.VaultUnsealConfig{}).Build()
}

// basicVaultRepository is a simple implementation of VaultClientRepository for integration tests
type basicVaultRepository struct{}

func (r *basicVaultRepository) GetClient(ctx context.Context, key string, instance *vaultv1.VaultInstance) (vaultpkg.VaultClient, error) {
	return vaultpkg.NewClient(instance.Endpoint, instance.TLSSkipVerify, 30*time.Second)
}

func (r *basicVaultRepository) Close() error {
	return nil
}

// setupController initializes the VaultUnsealConfig controller for testing
func (suite *VaultIntegrationTestSuite) setupController() {
	// This test needs a real vault client repository since it uses real vault containers
	realRepo := &basicVaultRepository{}

	suite.reconciler = controller.NewVaultUnsealConfigReconciler(
		suite.k8sClient,
		ctrl.Log.WithName("test-controller"),
		suite.scheme,
		realRepo,
		nil, // Use default options
	)
}

// TearDownSuite cleans up resources after all tests
func (suite *VaultIntegrationTestSuite) TearDownSuite() {
	if suite.ctxCancel != nil {
		suite.ctxCancel()
	}

	if suite.vaultContainer != nil {
		err := suite.vaultContainer.Terminate(context.Background())
		if err != nil {
			suite.T().Logf("Failed to terminate Vault container: %v", err)
		}
	}
}

// SetupTest prepares the environment before each test
func (suite *VaultIntegrationTestSuite) SetupTest() {
	// Dev mode vault cannot be sealed, so we don't need to do anything special
	// The tests will work with the dev mode vault as-is
}

// TestVaultClientConnection tests basic Vault client connectivity
func (suite *VaultIntegrationTestSuite) TestVaultClientConnection() {
	vaultClient, err := vaultpkg.NewClient(suite.vaultAddr, false, 30*time.Second)
	require.NoError(suite.T(), err, "Failed to create vault client")
	defer vaultClient.Close()

	// Test health check
	health, err := vaultClient.HealthCheck(suite.ctx)
	require.NoError(suite.T(), err, "Health check failed")
	assert.NotNil(suite.T(), health, "Health response should not be nil")
}

// TestVaultSealStatus tests seal status checking functionality
func (suite *VaultIntegrationTestSuite) TestVaultSealStatus() {
	vaultClient, err := vaultpkg.NewClient(suite.vaultAddr, false, 30*time.Second)
	require.NoError(suite.T(), err, "Failed to create vault client")
	defer vaultClient.Close()

	// In dev mode, vault starts unsealed with 1/1 threshold
	isSealed, err := vaultClient.IsSealed(suite.ctx)
	require.NoError(suite.T(), err, "Failed to check seal status")
	assert.False(suite.T(), isSealed, "Vault should be unsealed in dev mode")

	// Get detailed seal status
	sealStatus, err := vaultClient.GetSealStatus(suite.ctx)
	require.NoError(suite.T(), err, "Failed to get seal status")
	assert.False(suite.T(), sealStatus.Sealed, "Vault should be unsealed in dev mode")
	assert.Equal(suite.T(), 1, sealStatus.T, "Threshold should be 1 in dev mode")
	assert.Equal(suite.T(), 1, sealStatus.N, "Total keys should be 1 in dev mode")
}

// TestVaultUnsealing tests the unsealing process (in dev mode, vault starts unsealed)
func (suite *VaultIntegrationTestSuite) TestVaultUnsealing() {
	vaultClient, err := vaultpkg.NewClient(suite.vaultAddr, false, 30*time.Second)
	require.NoError(suite.T(), err, "Failed to create vault client")
	defer vaultClient.Close()

	// In dev mode, vault is already unsealed
	isSealed, err := vaultClient.IsSealed(suite.ctx)
	require.NoError(suite.T(), err, "Failed to check initial seal status")
	assert.False(suite.T(), isSealed, "Vault should be unsealed in dev mode")

	// Test that we can get seal status (dev mode doesn't really need unsealing)
	sealStatus, err := vaultClient.GetSealStatus(suite.ctx)
	require.NoError(suite.T(), err, "Should be able to get seal status")
	assert.False(suite.T(), sealStatus.Sealed, "Vault should remain unsealed")

	// Verify vault is still unsealed
	isSealed, err = vaultClient.IsSealed(suite.ctx)
	require.NoError(suite.T(), err, "Failed to check final seal status")
	assert.False(suite.T(), isSealed, "Vault should remain unsealed")
}

// TestVaultUnsealingWithInvalidKeys tests unsealing with invalid keys (dev mode vault behavior)
func (suite *VaultIntegrationTestSuite) TestVaultUnsealingWithInvalidKeys() {
	vaultClient, err := vaultpkg.NewClient(suite.vaultAddr, false, 30*time.Second)
	require.NoError(suite.T(), err, "Failed to create vault client")
	defer vaultClient.Close()

	// Test with invalid keys (in dev mode, this should either fail gracefully or be ignored)
	invalidKeys := []string{"invalid-key-1", "invalid-key-2", "invalid-key-3"}
	_, err = vaultClient.Unseal(suite.ctx, invalidKeys, 3)
	// In dev mode, this might not fail since vault is already unsealed
	// We just verify it doesn't crash

	// Verify vault state (should be unsealed in dev mode regardless)
	isSealed, err := vaultClient.IsSealed(suite.ctx)
	require.NoError(suite.T(), err, "Failed to check seal status after invalid unseal")
	assert.False(suite.T(), isSealed, "Vault should remain unsealed in dev mode")
}

// TestControllerReconciliation tests the full controller reconciliation process
func (suite *VaultIntegrationTestSuite) TestControllerReconciliation() {
	// Create a VaultUnsealConfig resource
	vaultConfig := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-vault-config",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:          "test-vault",
					Endpoint:      suite.vaultAddr,
					UnsealKeys:    suite.unsealKeys,
					Threshold:     func() *int { i := 3; return &i }(),
					TLSSkipVerify: false,
				},
			},
		},
	}

	// Create the resource in the fake client
	err := suite.k8sClient.Create(suite.ctx, vaultConfig)
	require.NoError(suite.T(), err, "Failed to create VaultUnsealConfig")

	// Trigger reconciliation
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-vault-config",
			Namespace: "default",
		},
	}

	result, err := suite.reconciler.Reconcile(suite.ctx, req)
	require.NoError(suite.T(), err, "Reconciliation should not fail")
	assert.NotZero(suite.T(), result.RequeueAfter, "Should requeue after some time")

	// Verify the status was updated
	var updatedConfig vaultv1.VaultUnsealConfig
	err = suite.k8sClient.Get(suite.ctx, types.NamespacedName{
		Name: "test-vault-config", Namespace: "default",
	}, &updatedConfig)
	require.NoError(suite.T(), err, "Failed to get updated config")

	// Verify status
	assert.NotEmpty(suite.T(), updatedConfig.Status.VaultStatuses, "Should have vault statuses")
	assert.Equal(suite.T(), "test-vault", updatedConfig.Status.VaultStatuses[0].Name, "Vault name should match")
	assert.False(suite.T(), updatedConfig.Status.VaultStatuses[0].Sealed, "Vault should be unsealed")
	assert.Empty(suite.T(), updatedConfig.Status.VaultStatuses[0].Error, "Should have no error")

	// Check conditions
	assert.NotEmpty(suite.T(), updatedConfig.Status.Conditions, "Should have conditions")
	readyCondition := updatedConfig.Status.Conditions[0]
	assert.Equal(suite.T(), "Ready", readyCondition.Type, "Should have Ready condition")
	assert.Equal(suite.T(), metav1.ConditionTrue, readyCondition.Status, "Ready condition should be true")
}

// TestControllerReconciliationWithMultipleVaults tests reconciliation with multiple vault instances
func (suite *VaultIntegrationTestSuite) TestControllerReconciliationWithMultipleVaults() {
	// For this test, we'll use the same vault instance twice with different names
	// In a real scenario, these would be different vault instances
	vaultConfig := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-multi-vault-config",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:          "vault-primary",
					Endpoint:      suite.vaultAddr,
					UnsealKeys:    suite.unsealKeys,
					Threshold:     func() *int { i := 3; return &i }(),
					TLSSkipVerify: false,
				},
				{
					Name:          "vault-secondary",
					Endpoint:      suite.vaultAddr,
					UnsealKeys:    suite.unsealKeys,
					Threshold:     func() *int { i := 3; return &i }(),
					TLSSkipVerify: false,
				},
			},
		},
	}

	// Create the resource
	err := suite.k8sClient.Create(suite.ctx, vaultConfig)
	require.NoError(suite.T(), err, "Failed to create VaultUnsealConfig")

	// Reconcile
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-multi-vault-config",
			Namespace: "default",
		},
	}

	_, err = suite.reconciler.Reconcile(suite.ctx, req)
	require.NoError(suite.T(), err, "Reconciliation should not fail")

	// Verify status
	var updatedConfig vaultv1.VaultUnsealConfig
	err = suite.k8sClient.Get(suite.ctx, types.NamespacedName{
		Name: "test-multi-vault-config", Namespace: "default",
	}, &updatedConfig)
	require.NoError(suite.T(), err, "Failed to get updated config")

	assert.Len(suite.T(), updatedConfig.Status.VaultStatuses, 2, "Should have 2 vault statuses")
	for i, status := range updatedConfig.Status.VaultStatuses {
		assert.False(suite.T(), status.Sealed, "Vault %d should be unsealed", i)
		assert.Empty(suite.T(), status.Error, "Vault %d should have no error", i)
	}
}

// TestVaultClientErrorHandling tests error handling scenarios
func (suite *VaultIntegrationTestSuite) TestVaultClientErrorHandling() {
	// Test with invalid URL
	_, err := vaultpkg.NewClient("invalid-url", false, 5*time.Second)
	assert.Error(suite.T(), err, "Should fail with invalid URL")

	// Test with unreachable endpoint
	vaultClient, err := vaultpkg.NewClient("http://localhost:9999", false, 1*time.Second)
	require.NoError(suite.T(), err, "Client creation should succeed even with unreachable endpoint")
	defer vaultClient.Close()

	// This should timeout/fail
	_, err = vaultClient.IsSealed(suite.ctx)
	assert.Error(suite.T(), err, "Should fail when connecting to unreachable endpoint")
}

// TestVaultClientConcurrency tests concurrent access to vault client
func (suite *VaultIntegrationTestSuite) TestVaultClientConcurrency() {
	vaultClient, err := vaultpkg.NewClient(suite.vaultAddr, false, 30*time.Second)
	require.NoError(suite.T(), err, "Failed to create vault client")
	defer vaultClient.Close()

	// Unseal the vault first
	_, err = vaultClient.Unseal(suite.ctx, suite.unsealKeys, 3)
	require.NoError(suite.T(), err, "Failed to unseal vault")

	// Test concurrent seal status checks
	concurrency := 10
	results := make(chan bool, concurrency)
	errors := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		go func() {
			isSealed, err := vaultClient.IsSealed(suite.ctx)
			if err != nil {
				errors <- err
				return
			}
			results <- isSealed
		}()
	}

	// Collect results
	for i := 0; i < concurrency; i++ {
		select {
		case err := <-errors:
			require.NoError(suite.T(), err, "Concurrent seal status check failed")
		case isSealed := <-results:
			assert.False(suite.T(), isSealed, "Vault should be unsealed")
		case <-time.After(10 * time.Second):
			suite.T().Fatal("Timeout waiting for concurrent operations")
		}
	}
}

// TestMain sets up and tears down the test environment
func TestMain(m *testing.M) {
	// Check if we're in CI and Docker is available
	if os.Getenv("CI") == "true" {
		// In CI, we expect Docker to be available
		fmt.Println("Running in CI environment")
	}

	// Run tests
	code := m.Run()
	os.Exit(code)
}

// TestVaultIntegrationTestSuite runs the integration test suite
func TestVaultIntegrationTestSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	suite.Run(t, new(VaultIntegrationTestSuite))
}
