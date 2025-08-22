package shared

import (
	"testing"

	"github.com/stretchr/testify/suite"
)

// VaultOnlyTestSuite is a specialized test suite for Vault-only integration tests
// Use this when you only need Vault containers without Kubernetes or controllers
type VaultOnlyTestSuite struct {
	IntegrationTestSuite
}

// SetupSuite initializes the Vault-only test suite
func (suite *VaultOnlyTestSuite) SetupSuite() {
	suite.SetupIntegrationSuite(VaultOnlyOptions())
}

// TearDownSuite cleans up resources
func (suite *VaultOnlyTestSuite) TearDownSuite() {
	suite.TearDownIntegrationSuite()
}

// K3sOnlyTestSuite is a specialized test suite for K3s-only integration tests
// Use this when you only need Kubernetes clusters without Vault containers
type K3sOnlyTestSuite struct {
	IntegrationTestSuite
}

// SetupSuite initializes the K3s-only test suite
func (suite *K3sOnlyTestSuite) SetupSuite() {
	suite.SetupIntegrationSuite(K3sOnlyOptions())
}

// TearDownSuite cleans up resources
func (suite *K3sOnlyTestSuite) TearDownSuite() {
	suite.TearDownIntegrationSuite()
}

// FullIntegrationTestSuite is a specialized test suite for complete integration tests
// Use this when you need both Vault containers and Kubernetes clusters with controllers
type FullIntegrationTestSuite struct {
	IntegrationTestSuite
}

// SetupSuite initializes the full integration test suite
func (suite *FullIntegrationTestSuite) SetupSuite() {
	suite.SetupIntegrationSuite(FullIntegrationOptions())
}

// TearDownSuite cleans up resources
func (suite *FullIntegrationTestSuite) TearDownSuite() {
	suite.TearDownIntegrationSuite()
}

// ControllerOnlyTestSuite is a specialized test suite for controller-only tests
// Use this when you want to test the controller with fake clients (unit test style)
type ControllerOnlyTestSuite struct {
	IntegrationTestSuite
}

// SetupSuite initializes the controller-only test suite
func (suite *ControllerOnlyTestSuite) SetupSuite() {
	options := DefaultIntegrationSetupOptions()
	options.RequiresVault = false
	options.RequiresK3s = false
	options.RequiresController = true
	options.RequiresCRDs = true
	options.UseRealK8sClient = false // Use fake client for fast testing

	suite.SetupIntegrationSuite(options)
}

// TearDownSuite cleans up resources
func (suite *ControllerOnlyTestSuite) TearDownSuite() {
	suite.TearDownIntegrationSuite()
}

// MultiVaultTestSuite is a specialized test suite for testing multiple Vault instances
// Use this for failover, load balancing, and multi-vault scenarios
type MultiVaultTestSuite struct {
	IntegrationTestSuite
}

// SetupSuite initializes the multi-vault test suite
func (suite *MultiVaultTestSuite) SetupSuite() {
	options := DefaultIntegrationSetupOptions()
	options.NumVaultInstances = 3
	options.VaultInstanceNames = []string{"vault-primary", "vault-secondary", "vault-tertiary"}
	options.RequiresK3s = false // Focus on Vault scenarios
	options.RequiresController = true

	suite.SetupIntegrationSuite(options)
}

// TearDownSuite cleans up resources
func (suite *MultiVaultTestSuite) TearDownSuite() {
	suite.TearDownIntegrationSuite()
}

// CompatibilityTestSuite is a specialized test suite for version compatibility testing
// Use this to test against multiple versions of Vault and K3s
type CompatibilityTestSuite struct {
	IntegrationTestSuite
}

// SetupSuite initializes the compatibility test suite with default versions
// Call SetupWithVersions() for specific version testing
func (suite *CompatibilityTestSuite) SetupSuite() {
	suite.SetupIntegrationSuite(FullIntegrationOptions())
}

// SetupWithVersions initializes the suite with specific component versions
func (suite *CompatibilityTestSuite) SetupWithVersions(vaultVersion, k3sVersion string) {
	options := FullIntegrationOptions()
	options.VaultVersion = vaultVersion
	options.K3sVersion = k3sVersion

	suite.SetupIntegrationSuite(options)
}

// TearDownSuite cleans up resources
func (suite *CompatibilityTestSuite) TearDownSuite() {
	suite.TearDownIntegrationSuite()
}

// Convenience functions for running test suites

// RunVaultOnlyTests runs a test suite that only requires Vault containers
func RunVaultOnlyTests(t *testing.T, testSuite suite.TestingSuite) {
	suite.Run(t, testSuite)
}

// RunK3sOnlyTests runs a test suite that only requires K3s clusters
func RunK3sOnlyTests(t *testing.T, testSuite suite.TestingSuite) {
	suite.Run(t, testSuite)
}

// RunFullIntegrationTests runs a complete integration test suite
func RunFullIntegrationTests(t *testing.T, testSuite suite.TestingSuite) {
	if testing.Short() {
		t.Skip("Skipping full integration tests in short mode")
	}
	suite.Run(t, testSuite)
}

// RunControllerOnlyTests runs controller tests with fake clients (fast)
func RunControllerOnlyTests(t *testing.T, testSuite suite.TestingSuite) {
	suite.Run(t, testSuite)
}

// RunMultiVaultTests runs tests against multiple Vault instances
func RunMultiVaultTests(t *testing.T, testSuite suite.TestingSuite) {
	if testing.Short() {
		t.Skip("Skipping multi-vault tests in short mode")
	}
	suite.Run(t, testSuite)
}

// RunCompatibilityTests runs compatibility tests (usually in CI only)
func RunCompatibilityTests(t *testing.T, testSuite suite.TestingSuite) {
	if testing.Short() {
		t.Skip("Skipping compatibility tests in short mode")
	}

	// Only run compatibility tests if explicitly enabled
	if !suite.suite.(*CompatibilityTestSuite).Config().IsCompatibilityTestingEnabled() {
		t.Skip("Compatibility testing not enabled - set ENABLE_COMPATIBILITY_TESTING=true")
	}

	suite.Run(t, testSuite)
}
