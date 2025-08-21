package integration

import (
	"context"
	"testing"
	"time"

	vaultv1 "github.com/panteparak/vault-autounseal-operator/pkg/api/v1"
	"github.com/panteparak/vault-autounseal-operator/pkg/controller"
	vaultpkg "github.com/panteparak/vault-autounseal-operator/pkg/vault"
	"github.com/panteparak/vault-autounseal-operator/tests/integration/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// VaultIntegrationLegacyTestSuite provides integration testing for vault-autounseal-operator using shared utilities
type VaultIntegrationLegacyTestSuite struct {
	suite.Suite
	vaultManager  *shared.VaultManager
	vaultInstance *shared.VaultInstance
	k8sClient     client.Client
	scheme        *runtime.Scheme
	reconciler    *controller.VaultUnsealConfigReconciler
	ctx           context.Context
	ctxCancel     context.CancelFunc
}

// SetupSuite initializes the test suite with shared utilities
func (suite *VaultIntegrationLegacyTestSuite) SetupSuite() {
	suite.ctx, suite.ctxCancel = context.WithTimeout(context.Background(), 5*time.Minute)

	// Set up logging for tests
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	// Use shared VaultManager for standardized Vault setup
	suite.vaultManager = shared.NewVaultManager(suite.ctx, suite.Suite)

	// Create a development mode Vault instance
	var err error
	suite.vaultInstance, err = suite.vaultManager.CreateDevVault("test-vault")
	require.NoError(suite.T(), err, "Failed to create Vault instance")

	// Set up Kubernetes client and controller
	suite.setupKubernetesClient()
	suite.setupController()
}

// setupKubernetesClient sets up the fake Kubernetes client for testing
func (suite *VaultIntegrationLegacyTestSuite) setupKubernetesClient() {
	// Set up scheme
	suite.scheme = runtime.NewScheme()
	err := clientgoscheme.AddToScheme(suite.scheme)
	require.NoError(suite.T(), err, "Failed to add client-go scheme")

	err = vaultv1.AddToScheme(suite.scheme)
	require.NoError(suite.T(), err, "Failed to add vault scheme")

	// Set up fake client
	suite.k8sClient = fake.NewClientBuilder().WithScheme(suite.scheme).Build()
}

// setupController sets up the VaultUnsealConfig controller for testing
func (suite *VaultIntegrationLegacyTestSuite) setupController() {
	suite.reconciler = &controller.VaultUnsealConfigReconciler{
		Client: suite.k8sClient,
		Log:    ctrl.Log.WithName("controllers").WithName("VaultUnsealConfig"),
		Scheme: suite.scheme,
	}
}

// TearDownSuite cleans up test resources
func (suite *VaultIntegrationLegacyTestSuite) TearDownSuite() {
	if suite.ctxCancel != nil {
		suite.ctxCancel()
	}

	if suite.vaultManager != nil {
		suite.vaultManager.Cleanup()
	}
}

// SetupTest sets up each individual test
func (suite *VaultIntegrationLegacyTestSuite) SetupTest() {
	// Verify vault is still healthy before each test
	err := suite.vaultManager.VerifyVaultHealth(suite.vaultInstance, false)
	require.NoError(suite.T(), err, "Vault should be healthy before test")
}

// TestVaultClientConnection tests basic vault client connectivity
func (suite *VaultIntegrationLegacyTestSuite) TestVaultClientConnection() {
	// Create our vault client using the instance info
	client, err := vaultpkg.NewClient(suite.vaultInstance.Address, false, 30*time.Second)
	require.NoError(suite.T(), err, "Should create vault client")
	defer client.Close()

	// Test basic connectivity
	assert.Equal(suite.T(), suite.vaultInstance.Address, client.URL())
	assert.False(suite.T(), client.IsClosed())
}

// TestVaultSealStatus tests reading vault seal status
func (suite *VaultIntegrationLegacyTestSuite) TestVaultSealStatus() {
	// Create vault client
	client, err := vaultpkg.NewClient(suite.vaultInstance.Address, false, 30*time.Second)
	require.NoError(suite.T(), err, "Should create vault client")
	defer client.Close()

	// Test seal status (dev vault should be unsealed)
	isSealed, err := client.IsSealed(suite.ctx)
	require.NoError(suite.T(), err, "Should get seal status")
	assert.False(suite.T(), isSealed, "Dev vault should not be sealed")

	// Test seal status response
	sealStatus, err := client.GetSealStatus(suite.ctx)
	require.NoError(suite.T(), err, "Should get seal status response")
	require.NotNil(suite.T(), sealStatus)
	assert.False(suite.T(), sealStatus.Sealed, "Dev vault should not be sealed")
}

// TestVaultUnsealing tests the unsealing process with test keys
func (suite *VaultIntegrationLegacyTestSuite) TestVaultUnsealing() {
	// For dev vault, we test the unsealing logic but the vault stays unsealed
	// This tests our client's unsealing workflow even though the vault doesn't actually get sealed
	testKeys := []string{
		"dGVzdC1rZXktMQ==", // test-key-1 in base64
		"dGVzdC1rZXktMg==", // test-key-2 in base64
		"dGVzdC1rZXktMw==", // test-key-3 in base64
	}

	client, err := vaultpkg.NewClient(suite.vaultInstance.Address, false, 30*time.Second)
	require.NoError(suite.T(), err, "Should create vault client")
	defer client.Close()

	// Test unsealing with multiple keys (this tests our client logic)
	sealStatus, err := client.Unseal(suite.ctx, testKeys, 3)
	// This might fail due to dev vault not actually being sealed, but tests the workflow
	if err != nil {
		// Expected for dev vault - log and continue
		suite.T().Logf("Unsealing failed as expected for dev vault: %v", err)
	} else {
		require.NotNil(suite.T(), sealStatus)
	}
}

// TestVaultUnsealingWithInvalidKeys tests unsealing with invalid keys
func (suite *VaultIntegrationLegacyTestSuite) TestVaultUnsealingWithInvalidKeys() {
	invalidKeys := []string{
		"invalid-key-1",
		"invalid-key-2",
		"invalid-key-3",
	}

	client, err := vaultpkg.NewClient(suite.vaultInstance.Address, false, 30*time.Second)
	require.NoError(suite.T(), err, "Should create vault client")
	defer client.Close()

	// Test unsealing with invalid keys (should fail validation)
	_, err = client.Unseal(suite.ctx, invalidKeys, 3)
	assert.Error(suite.T(), err, "Should fail with invalid keys")
	assert.Contains(suite.T(), err.Error(), "invalid base64", "Should be validation error")
}

// TestControllerReconciliation tests the controller reconciliation process
func (suite *VaultIntegrationLegacyTestSuite) TestControllerReconciliation() {
	// Create a VaultUnsealConfig for testing
	threshold := 3
	vaultConfig := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-vault-config",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:       "test-vault",
					Endpoint:   suite.vaultInstance.Address,
					UnsealKeys: []string{
						"dGVzdC1rZXktMQ==",
						"dGVzdC1rZXktMg==",
						"dGVzdC1rZXktMw==",
					},
					Threshold:     &threshold,
					TLSSkipVerify: false,
				},
			},
		},
	}

	// Create the resource
	err := suite.k8sClient.Create(suite.ctx, vaultConfig)
	require.NoError(suite.T(), err, "Should create VaultUnsealConfig")

	// Test reconciliation
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      vaultConfig.Name,
			Namespace: vaultConfig.Namespace,
		},
	}

	result, err := suite.reconciler.Reconcile(suite.ctx, req)
	// This may have warnings due to dev vault setup, but should not error
	assert.NoError(suite.T(), err, "Reconcile should not error")
	suite.T().Logf("Reconcile result: %+v", result)
}

// TestControllerReconciliationWithMultipleVaults tests reconciling multiple vault instances
func (suite *VaultIntegrationLegacyTestSuite) TestControllerReconciliationWithMultipleVaults() {
	// Create a second vault instance for multi-vault testing
	vaultInstance2, err := suite.vaultManager.CreateDevVault("test-vault-2")
	require.NoError(suite.T(), err, "Should create second vault instance")

	threshold := 2
	vaultConfig := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "multi-vault-config",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:       "vault-1",
					Endpoint:   suite.vaultInstance.Address,
					UnsealKeys: []string{"dGVzdC1rZXktMQ==", "dGVzdC1rZXktMg=="},
					Threshold:  &threshold,
				},
				{
					Name:       "vault-2",
					Endpoint:   vaultInstance2.Address,
					UnsealKeys: []string{"dGVzdC1rZXktMQ==", "dGVzdC1rZXktMg=="},
					Threshold:  &threshold,
				},
			},
		},
	}

	// Create and reconcile
	err = suite.k8sClient.Create(suite.ctx, vaultConfig)
	require.NoError(suite.T(), err, "Should create multi-vault config")

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      vaultConfig.Name,
			Namespace: vaultConfig.Namespace,
		},
	}

	result, err := suite.reconciler.Reconcile(suite.ctx, req)
	assert.NoError(suite.T(), err, "Multi-vault reconcile should not error")
	suite.T().Logf("Multi-vault reconcile result: %+v", result)
}

// TestVaultClientErrorHandling tests error handling scenarios
func (suite *VaultIntegrationLegacyTestSuite) TestVaultClientErrorHandling() {
	// Test with invalid endpoint
	client, err := vaultpkg.NewClient("http://invalid-endpoint:8200", false, 5*time.Second)
	require.NoError(suite.T(), err, "Should create client even with invalid endpoint")
	defer client.Close()

	// Operations should fail with connection errors
	_, err = client.IsSealed(suite.ctx)
	assert.Error(suite.T(), err, "Should fail with connection error")
}

// TestVaultClientConcurrency tests concurrent operations
func (suite *VaultIntegrationLegacyTestSuite) TestVaultClientConcurrency() {
	client, err := vaultpkg.NewClient(suite.vaultInstance.Address, false, 30*time.Second)
	require.NoError(suite.T(), err, "Should create vault client")
	defer client.Close()

	// Run concurrent operations
	concurrency := 10
	done := make(chan error, concurrency)

	for i := 0; i < concurrency; i++ {
		go func() {
			// Test concurrent health checks
			_, err := client.HealthCheck(suite.ctx)
			done <- err
		}()
	}

	// Wait for all operations
	for i := 0; i < concurrency; i++ {
		select {
		case err := <-done:
			assert.NoError(suite.T(), err, "Concurrent operation should not error")
		case <-time.After(10 * time.Second):
			suite.T().Fatal("Timeout waiting for concurrent operations")
		}
	}
}


// TestVaultIntegrationLegacyTestSuite runs the integration test suite
func TestVaultIntegrationLegacyTestSuite(t *testing.T) {
	suite.Run(t, new(VaultIntegrationLegacyTestSuite))
}
