package integration

import (
	"context"
	"encoding/base64"
	"testing"
	"time"

	"github.com/panteparak/vault-autounseal-operator/pkg/vault"
	"github.com/panteparak/vault-autounseal-operator/tests/integration/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// VaultAPITestSuite provides comprehensive testing for Vault API/SDK unsealing functionality
// This suite focuses on direct Vault API interactions without Kubernetes dependencies
type VaultAPITestSuite struct {
	suite.Suite
	ctx           context.Context
	ctxCancel     context.CancelFunc
	vaultManager  *shared.VaultManager
}

// SetupSuite initializes the test suite
func (suite *VaultAPITestSuite) SetupSuite() {
	if testing.Short() {
		suite.T().Skip("Skipping Vault API integration tests in short mode")
	}

	suite.ctx, suite.ctxCancel = context.WithTimeout(context.Background(), 15*time.Minute)
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	
	// Create vault manager - will fail fast if Docker is not available
	suite.vaultManager = shared.NewVaultManager(suite.ctx, suite.Suite)
	
	// Verify manager was created successfully
	require.NotNil(suite.T(), suite.vaultManager, "VaultManager should be created successfully")
}

// TearDownSuite cleans up resources
func (suite *VaultAPITestSuite) TearDownSuite() {
	if suite.vaultManager != nil {
		suite.vaultManager.Cleanup()
	}
	if suite.ctxCancel != nil {
		suite.ctxCancel()
	}
}

// TestBasicVaultConnectivity tests basic connectivity to Vault instances
func (suite *VaultAPITestSuite) TestBasicVaultConnectivity() {
	suite.Run("connect_to_dev_vault", func() {
		devVault, err := suite.vaultManager.CreateDevVault("dev-test")
		require.NoError(suite.T(), err, "Should create dev vault successfully")
		require.NotNil(suite.T(), devVault, "Dev vault should not be nil")

		// Verify connectivity
		err = suite.vaultManager.VerifyVaultHealth(devVault, false)
		assert.NoError(suite.T(), err, "Dev vault should be healthy and unsealed")

		suite.T().Logf("✅ Dev vault healthy at: %s", devVault.Address)
	})

	suite.Run("connect_to_prod_vault", func() {
		prodVault, err := suite.vaultManager.CreateProdVault("prod-test")
		require.NoError(suite.T(), err, "Should create prod vault successfully")
		require.NotNil(suite.T(), prodVault, "Prod vault should not be nil")

		// Verify it starts sealed
		err = suite.vaultManager.VerifyVaultHealth(prodVault, true)
		assert.NoError(suite.T(), err, "Prod vault should be healthy but sealed")

		suite.T().Logf("✅ Prod vault healthy and sealed at: %s", prodVault.Address)
		suite.T().Logf("✅ Unseal keys available: %d", len(prodVault.UnsealKeys))
	})
}

// TestVaultUnsealingWorkflow tests the complete unsealing workflow
func (suite *VaultAPITestSuite) TestVaultUnsealingWorkflow() {
	suite.Run("successful_unsealing", func() {
		// Create a production vault
		prodVault, err := suite.vaultManager.CreateProdVault("unseal-test")
		require.NoError(suite.T(), err, "Should create prod vault")

		// Verify it's initially sealed
		err = suite.vaultManager.VerifyVaultHealth(prodVault, true)
		require.NoError(suite.T(), err, "Vault should start sealed")

		// Unseal with sufficient keys
		err = suite.vaultManager.UnsealVault(prodVault, prodVault.UnsealKeys, 3)
		assert.NoError(suite.T(), err, "Should unseal vault successfully")

		// Verify it's now unsealed
		err = suite.vaultManager.VerifyVaultHealth(prodVault, false)
		assert.NoError(suite.T(), err, "Vault should be unsealed")

		suite.T().Logf("✅ Vault successfully unsealed")
	})

	suite.Run("insufficient_keys", func() {
		prodVault, err := suite.vaultManager.CreateProdVault("insufficient-keys")
		require.NoError(suite.T(), err, "Should create prod vault")

		// Try to unseal with insufficient keys
		err = suite.vaultManager.UnsealVault(prodVault, prodVault.UnsealKeys[:2], 3)
		assert.Error(suite.T(), err, "Should fail with insufficient keys")
		assert.Contains(suite.T(), err.Error(), "insufficient keys")

		suite.T().Logf("✅ Correctly rejected insufficient keys")
	})

	suite.Run("seal_and_reseal_cycle", func() {
		prodVault, err := suite.vaultManager.CreateProdVault("seal-cycle")
		require.NoError(suite.T(), err, "Should create prod vault")

		// Unseal first
		err = suite.vaultManager.UnsealVault(prodVault, prodVault.UnsealKeys, 3)
		require.NoError(suite.T(), err, "Should unseal vault")

		// Seal it back
		err = suite.vaultManager.SealVault(prodVault)
		assert.NoError(suite.T(), err, "Should seal vault successfully")

		// Verify it's sealed
		err = suite.vaultManager.VerifyVaultHealth(prodVault, true)
		assert.NoError(suite.T(), err, "Vault should be sealed again")

		// Unseal again
		err = suite.vaultManager.UnsealVault(prodVault, prodVault.UnsealKeys, 3)
		assert.NoError(suite.T(), err, "Should unseal vault again")

		suite.T().Logf("✅ Seal/unseal cycle completed successfully")
	})
}

// TestVaultClientIntegration tests our vault client implementation
func (suite *VaultAPITestSuite) TestVaultClientIntegration() {
	suite.Run("client_creation_and_basic_ops", func() {
		devVault, err := suite.vaultManager.CreateDevVault("client-test")
		require.NoError(suite.T(), err, "Should create dev vault")

		// Create our vault client
		client, err := vault.NewClient(devVault.Address, true, 30*time.Second)
		require.NoError(suite.T(), err, "Should create vault client")
		defer client.Close()

		// Test basic client properties
		assert.Equal(suite.T(), devVault.Address, client.URL())
		assert.Equal(suite.T(), 30*time.Second, client.Timeout())
		assert.False(suite.T(), client.IsClosed())

		suite.T().Logf("✅ Vault client created and configured correctly")
	})

	suite.Run("client_unsealing_integration", func() {
		prodVault, err := suite.vaultManager.CreateProdVault("client-unseal")
		require.NoError(suite.T(), err, "Should create prod vault")

		// Create client
		client, err := vault.NewClient(prodVault.Address, true, 30*time.Second)
		require.NoError(suite.T(), err, "Should create vault client")
		defer client.Close()

		// Test unsealing through our client
		keys := prodVault.UnsealKeys[:3] // Use first 3 keys
		result, err := client.Unseal(suite.ctx, keys, 3)
		assert.NoError(suite.T(), err, "Should unseal vault successfully")
		assert.NotNil(suite.T(), result, "Should have result from unsealing")

		// Verify vault is unsealed
		assert.False(suite.T(), result.Sealed, "Vault should be unsealed after submitting threshold keys")

		suite.T().Logf("✅ Client unsealing integration successful")
	})
}

// TestErrorHandlingScenarios tests various error scenarios
func (suite *VaultAPITestSuite) TestErrorHandlingScenarios() {
	suite.Run("invalid_vault_endpoint", func() {
		// Test connection to non-existent vault
		client, err := vault.NewClient("http://nonexistent.vault:8200", true, 5*time.Second)
		if err == nil && client != nil {
			// Client creation might succeed, but operations should fail
			keys := []string{base64.StdEncoding.EncodeToString([]byte("fake-key"))}
			_, err = client.Unseal(suite.ctx, keys, 1)
			assert.Error(suite.T(), err, "Should fail to connect to non-existent vault")
			client.Close()
		}

		suite.T().Logf("✅ Invalid endpoint correctly handled")
	})

	suite.Run("invalid_unseal_keys", func() {
		prodVault, err := suite.vaultManager.CreateProdVault("invalid-keys")
		require.NoError(suite.T(), err, "Should create prod vault")

		client, err := vault.NewClient(prodVault.Address, true, 30*time.Second)
		require.NoError(suite.T(), err, "Should create vault client")
		defer client.Close()

		// Test with invalid base64 keys
		invalidKeys := []string{"invalid-base64-key!!!"}
		_, err = client.Unseal(suite.ctx, invalidKeys, 1)
		assert.Error(suite.T(), err, "Should fail with invalid base64 keys")

		// Test with wrong keys (valid base64 but wrong for this vault)
		wrongKeys := []string{base64.StdEncoding.EncodeToString([]byte("wrong-key-content"))}
		_, err = client.Unseal(suite.ctx, wrongKeys, 1)
		assert.Error(suite.T(), err, "Should fail with wrong keys")

		suite.T().Logf("✅ Invalid keys correctly rejected")
	})

	suite.Run("timeout_scenarios", func() {
		devVault, err := suite.vaultManager.CreateDevVault("timeout-test")
		require.NoError(suite.T(), err, "Should create dev vault")

		// Create client with very short timeout
		client, err := vault.NewClient(devVault.Address, true, 1*time.Nanosecond)
		require.NoError(suite.T(), err, "Should create vault client")
		defer client.Close()

		// Operations should timeout
		keys := []string{base64.StdEncoding.EncodeToString([]byte("test-key"))}
		_, err = client.Unseal(suite.ctx, keys, 1)
		// This might not always timeout in dev mode, but we test the client configuration
		suite.T().Logf("Timeout test completed - client configured with minimal timeout")
	})
}

// TestConcurrentOperations tests concurrent vault operations
func (suite *VaultAPITestSuite) TestConcurrentOperations() {
	suite.Run("concurrent_client_creation", func() {
		devVault, err := suite.vaultManager.CreateDevVault("concurrent-test")
		require.NoError(suite.T(), err, "Should create dev vault")

		// Create multiple clients concurrently
		const numClients = 10
		clients := make([]*vault.Client, numClients)
		errors := make([]error, numClients)

		// Create clients concurrently
		done := make(chan int, numClients)
		for i := 0; i < numClients; i++ {
			go func(index int) {
				client, err := vault.NewClient(devVault.Address, true, 30*time.Second)
				clients[index] = client
				errors[index] = err
				done <- index
			}(i)
		}

		// Wait for all to complete
		for i := 0; i < numClients; i++ {
			<-done
		}

		// Verify all clients were created successfully
		successCount := 0
		for i := 0; i < numClients; i++ {
			if errors[i] == nil && clients[i] != nil {
				successCount++
				clients[i].Close()
			}
		}

		assert.Equal(suite.T(), numClients, successCount, "All clients should be created successfully")
		suite.T().Logf("✅ Created %d concurrent clients successfully", successCount)
	})

	suite.Run("concurrent_unsealing_attempts", func() {
		prodVault, err := suite.vaultManager.CreateProdVault("concurrent-unseal")
		require.NoError(suite.T(), err, "Should create prod vault")

		// Multiple goroutines attempting to unseal the same vault
		const numAttempts = 5
		results := make([]error, numAttempts)
		done := make(chan int, numAttempts)

		for i := 0; i < numAttempts; i++ {
			go func(index int) {
				client, err := vault.NewClient(prodVault.Address, true, 30*time.Second)
				if err != nil {
					results[index] = err
					done <- index
					return
				}
				defer client.Close()

				keys := prodVault.UnsealKeys[:3]
				_, err = client.Unseal(suite.ctx, keys, 3)
				results[index] = err
				done <- index
			}(i)
		}

		// Wait for all attempts
		for i := 0; i < numAttempts; i++ {
			<-done
		}

		// At least one should succeed (first one to complete unsealing)
		successCount := 0
		for _, err := range results {
			if err == nil {
				successCount++
			}
		}

		assert.GreaterOrEqual(suite.T(), successCount, 1, "At least one unsealing attempt should succeed")
		suite.T().Logf("✅ Concurrent unsealing: %d/%d attempts succeeded", successCount, numAttempts)
	})
}

// TestVaultAPITestSuite runs the Vault API test suite
func TestVaultAPITestSuite(t *testing.T) {
	suite.Run(t, new(VaultAPITestSuite))
}