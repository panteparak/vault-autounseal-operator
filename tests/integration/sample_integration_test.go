package integration

import (
	"context"
	"testing"

	"github.com/panteparak/vault-autounseal-operator/tests/common"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// SampleIntegrationTestSuite demonstrates the new modular test structure
type SampleIntegrationTestSuite struct {
	suite.Suite
	containerManager *common.ContainerManager
	config           *common.TestConfig
	ctx              context.Context
	ctxCancel        context.CancelFunc
}

// SetupSuite initializes the test suite using common utilities
func (suite *SampleIntegrationTestSuite) SetupSuite() {
	suite.config = common.DefaultTestConfig()
	suite.ctx, suite.ctxCancel = context.WithTimeout(context.Background(), suite.config.Timeout)
	
	// Use common container manager
	suite.containerManager = common.NewContainerManager(suite.ctx)
}

// TearDownSuite cleans up resources
func (suite *SampleIntegrationTestSuite) TearDownSuite() {
	if suite.containerManager != nil {
		suite.containerManager.Cleanup()
	}
	
	if suite.ctxCancel != nil {
		suite.ctxCancel()
	}
}

// TestVaultContainerStartup tests starting Vault using common utilities
func (suite *SampleIntegrationTestSuite) TestVaultContainerStartup() {
	// Start Vault using common container manager
	vaultContainer, vaultAddr, err := suite.containerManager.StartVault(suite.config.VaultToken)
	require.NoError(suite.T(), err, "Should start Vault container")
	require.NotNil(suite.T(), vaultContainer, "Vault container should not be nil")
	require.NotEmpty(suite.T(), vaultAddr, "Vault address should not be empty")

	suite.T().Logf("✅ Vault container started at: %s", vaultAddr)
}

// TestK3sContainerStartup tests starting K3s using common utilities
func (suite *SampleIntegrationTestSuite) TestK3sContainerStartup() {
	// Start K3s using common container manager
	k3sContainer, err := suite.containerManager.StartK3s()
	require.NoError(suite.T(), err, "Should start K3s container")
	require.NotNil(suite.T(), k3sContainer, "K3s container should not be nil")

	// Get kubeconfig
	kubeconfig, err := k3sContainer.GetKubeConfig(suite.ctx)
	require.NoError(suite.T(), err, "Should get kubeconfig")
	require.NotEmpty(suite.T(), kubeconfig, "Kubeconfig should not be empty")

	suite.T().Logf("✅ K3s container started with kubeconfig size: %d bytes", len(kubeconfig))
}

// TestVaultUnsealConfigBuilder tests the configuration builder utility
func (suite *SampleIntegrationTestSuite) TestVaultUnsealConfigBuilder() {
	// Use common builder to create test configuration
	config := common.NewVaultUnsealConfigBuilder("sample-config", "default").
		WithVaultInstance("test-vault", "http://vault.example.com:8200", common.GenerateTestKeys(3)).
		WithVaultInstanceAndThreshold("test-vault-2", "http://vault2.example.com:8200", common.GenerateTestKeys(5), 3).
		WithLabels(map[string]string{
			"test-type": "integration",
			"module":    "sample",
		}).
		WithAnnotations(map[string]string{
			"test.example.com/created-by": "sample-integration-test",
		}).
		Build()

	require.NotNil(suite.T(), config, "Config should not be nil")
	require.Equal(suite.T(), "sample-config", config.Name, "Config name should match")
	require.Equal(suite.T(), "default", config.Namespace, "Config namespace should match")
	require.Len(suite.T(), config.Spec.VaultInstances, 2, "Should have 2 vault instances")
	require.Equal(suite.T(), "integration", config.Labels["test-type"], "Should have test-type label")

	suite.T().Logf("✅ VaultUnsealConfig built with %d vault instances", len(config.Spec.VaultInstances))
}

// TestModularStructure demonstrates the benefits of the new modular structure
func (suite *SampleIntegrationTestSuite) TestModularStructure() {
	suite.T().Log("🎯 Demonstrating modular test structure benefits:")
	suite.T().Log("  ✅ Common utilities for container management")
	suite.T().Log("  ✅ Shared configuration builders")
	suite.T().Log("  ✅ Reusable test helpers")
	suite.T().Log("  ✅ Organized by test category (unit/integration/e2e/performance/chaos/boundary)")
	suite.T().Log("  ✅ Makefile targets for different test categories")
	suite.T().Log("  ✅ Similar to Java's main/test structure")
}

// TestSampleIntegrationTestSuite runs the sample integration test suite
func TestSampleIntegrationTestSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration tests in short mode")
	}

	suite.Run(t, new(SampleIntegrationTestSuite))
}