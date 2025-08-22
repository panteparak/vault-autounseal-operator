package shared

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// ExampleVaultOnlyTest demonstrates how to write a Vault-only integration test
type ExampleVaultOnlyTest struct {
	VaultOnlyTestSuite
}

// TestVaultBasicOperations demonstrates basic Vault operations testing
func (suite *ExampleVaultOnlyTest) TestVaultBasicOperations() {
	// Get the default Vault instance (automatically created by the suite)
	vault := suite.GetDefaultVaultInstance()
	require.NotNil(suite.T(), vault, "Default vault instance should be available")

	// Verify vault is healthy and unsealed (dev mode vaults start unsealed)
	suite.AssertVaultHealth("default", false)

	// Test basic Vault client operations
	_, err := vault.Client.Sys().Health()
	require.NoError(suite.T(), err, "Vault health check should succeed")

	// Test writing and reading a secret
	secretData := map[string]interface{}{
		"data": map[string]interface{}{
			"username": "testuser",
			"password": "testpass",
		},
	}

	_, err = vault.Client.Logical().Write("secret/data/test", secretData)
	require.NoError(suite.T(), err, "Should be able to write secret")

	secret, err := vault.Client.Logical().Read("secret/data/test")
	require.NoError(suite.T(), err, "Should be able to read secret")
	require.NotNil(suite.T(), secret, "Secret should exist")

	// Verify the data
	data := secret.Data["data"].(map[string]interface{})
	assert.Equal(suite.T(), "testuser", data["username"])
	assert.Equal(suite.T(), "testpass", data["password"])
}

// TestVaultProdModeSimulation demonstrates production mode testing
func (suite *ExampleVaultOnlyTest) TestVaultProdModeSimulation() {
	// Create a production mode vault
	prodVault, err := suite.VaultManager().CreateProdVault("prod-test")
	require.NoError(suite.T(), err, "Should create production vault")

	// Verify it starts sealed
	suite.AssertVaultHealth("prod-test", true)

	// Unseal the vault using the generated keys
	err = suite.VaultManager().UnsealVault(prodVault, prodVault.UnsealKeys, 3)
	require.NoError(suite.T(), err, "Should unseal vault with valid keys")

	// Verify it's now unsealed
	suite.AssertVaultHealth("prod-test", false)
}

// ExampleControllerTest demonstrates controller-only testing
type ExampleControllerTest struct {
	ControllerOnlyTestSuite
}

// TestControllerReconciliation demonstrates controller testing with fake clients
func (suite *ExampleControllerTest) TestControllerReconciliation() {
	// This test runs fast because it uses fake Kubernetes clients
	// No actual containers are started

	require.NotNil(suite.T(), suite.Reconciler(), "Controller should be available")
	require.NotNil(suite.T(), suite.K8sClient(), "K8s client should be available")

	// TODO: Add actual controller reconciliation tests
	// This is a placeholder showing the structure
	suite.T().Log("Controller reconciliation test would go here")
}

// ExampleFullIntegrationTest demonstrates complete integration testing
type ExampleFullIntegrationTest struct {
	FullIntegrationTestSuite
}

// TestOperatorWorkflow demonstrates end-to-end operator workflow
func (suite *ExampleFullIntegrationTest) TestOperatorWorkflow() {
	// This test has both Vault and K3s available, plus controllers
	vault := suite.GetDefaultVaultInstance()
	require.NotNil(suite.T(), vault, "Vault should be available")

	k3s, exists := suite.GetK3sInstance()
	require.True(suite.T(), exists, "K3s should be available")
	require.NotNil(suite.T(), k3s, "K3s instance should not be nil")

	// Create a VaultUnsealConfig resource
	vaultConfigs := []VaultInstanceConfig{
		{
			Name:         "test-vault",
			Endpoint:     vault.Address,
			UnsealKeys:   []string{"ZGVmYXVsdC11bnNlYWwta2V5LTE="}, // base64 encoded test key
			Threshold:    1,
			TLSSkipVerify: true,
		},
	}

	config := suite.CreateTestVaultUnsealConfig("test-config", "default", vaultConfigs)
	require.NotNil(suite.T(), config, "VaultUnsealConfig should be created")

	// TODO: Add actual reconciliation and verification
	suite.T().Log("Full integration test would verify end-to-end workflow here")
}

// ExampleMultiVaultTest demonstrates testing with multiple Vault instances
type ExampleMultiVaultTest struct {
	MultiVaultTestSuite
}

// TestVaultFailover demonstrates failover scenario testing
func (suite *ExampleMultiVaultTest) TestVaultFailover() {
	// MultiVaultTestSuite automatically creates 3 vault instances:
	// vault-primary, vault-secondary, vault-tertiary

	primary, exists := suite.GetVaultInstance("vault-primary")
	require.True(suite.T(), exists, "Primary vault should exist")

	secondary, exists := suite.GetVaultInstance("vault-secondary")
	require.True(suite.T(), exists, "Secondary vault should exist")

	tertiary, exists := suite.GetVaultInstance("vault-tertiary")
	require.True(suite.T(), exists, "Tertiary vault should exist")

	// Verify all vaults are healthy
	suite.AssertVaultHealth("vault-primary", false)
	suite.AssertVaultHealth("vault-secondary", false)
	suite.AssertVaultHealth("vault-tertiary", false)

	// Test failover logic
	suite.T().Logf("Primary vault: %s", primary.Address)
	suite.T().Logf("Secondary vault: %s", secondary.Address)
	suite.T().Logf("Tertiary vault: %s", tertiary.Address)

	// TODO: Add actual failover testing logic
	suite.T().Log("Failover testing logic would go here")
}

// ExampleCompatibilityTest demonstrates version compatibility testing
type ExampleCompatibilityTest struct {
	CompatibilityTestSuite
}

// TestVault116Compatibility demonstrates testing specific Vault versions
func (suite *ExampleCompatibilityTest) TestVault116Compatibility() {
	// Test against Vault 1.16.0 specifically
	vault, err := suite.VaultManager().CreateVaultWithVersion("vault-116", "1.16.0", DevMode)
	require.NoError(suite.T(), err, "Should create Vault 1.16.0")

	suite.T().Logf("Testing against Vault 1.16.0 at %s", vault.Address)

	// Verify version-specific functionality
	health, err := vault.Client.Sys().Health()
	require.NoError(suite.T(), err, "Health check should work on Vault 1.16.0")
	require.NotNil(suite.T(), health, "Health response should not be nil")

	// TODO: Add version-specific compatibility tests
}

// ExampleCustomSetupTest demonstrates custom setup options
type ExampleCustomSetupTest struct {
	IntegrationTestSuite
}

// SetupSuite demonstrates custom setup configuration
func (suite *ExampleCustomSetupTest) SetupSuite() {
	// Create custom setup options
	options := &IntegrationSetupOptions{
		RequiresVault:       true,
		RequiresK3s:        true,
		RequiresController: true,
		RequiresCRDs:       true,

		VaultMode:           ProdMode,  // Use production mode
		VaultVersion:        "1.17.0", // Specific version
		NumVaultInstances:   2,         // Two vaults
		VaultInstanceNames:  []string{"vault-alpha", "vault-beta"},

		K3sVersion:          "v1.29.0-k3s1", // Specific K3s version
		K3sNamespace:        "vault-system", // Custom namespace

		UseRealK8sClient:    true,
		EnableLeaderElection: false,

		SkipInShortMode:     true,
		CustomTimeout:       20 * time.Minute, // Longer timeout
	}

	suite.SetupIntegrationSuite(options)
}

// TearDownSuite cleans up resources
func (suite *ExampleCustomSetupTest) TearDownSuite() {
	suite.TearDownIntegrationSuite()
}

// TestCustomConfiguration demonstrates using custom setup
func (suite *ExampleCustomSetupTest) TestCustomConfiguration() {
	// Verify our custom setup
	alpha, exists := suite.GetVaultInstance("vault-alpha")
	require.True(suite.T(), exists, "vault-alpha should exist")
	require.Equal(suite.T(), ProdMode, alpha.Mode, "Should be in production mode")

	beta, exists := suite.GetVaultInstance("vault-beta")
	require.True(suite.T(), exists, "vault-beta should exist")
	require.Equal(suite.T(), ProdMode, beta.Mode, "Should be in production mode")

	suite.T().Logf("Alpha vault: %s", alpha.Address)
	suite.T().Logf("Beta vault: %s", beta.Address)

	// Both should start sealed in production mode
	suite.AssertVaultHealth("vault-alpha", true)
	suite.AssertVaultHealth("vault-beta", true)
}

// Example test functions showing how to run these tests

func TestExampleVaultOnlyTest(t *testing.T) {
	RunVaultOnlyTests(t, new(ExampleVaultOnlyTest))
}

func TestExampleControllerTest(t *testing.T) {
	RunControllerOnlyTests(t, new(ExampleControllerTest))
}

func TestExampleFullIntegrationTest(t *testing.T) {
	RunFullIntegrationTests(t, new(ExampleFullIntegrationTest))
}

func TestExampleMultiVaultTest(t *testing.T) {
	RunMultiVaultTests(t, new(ExampleMultiVaultTest))
}

func TestExampleCompatibilityTest(t *testing.T) {
	RunCompatibilityTests(t, new(ExampleCompatibilityTest))
}

func TestExampleCustomSetupTest(t *testing.T) {
	suite.Run(t, new(ExampleCustomSetupTest))
}
