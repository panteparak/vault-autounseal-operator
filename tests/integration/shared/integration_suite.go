package shared

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	vaultv1 "github.com/panteparak/vault-autounseal-operator/pkg/api/v1"
	"github.com/panteparak/vault-autounseal-operator/pkg/controller"
	"github.com/panteparak/vault-autounseal-operator/pkg/testing/mocks"
	"github.com/panteparak/vault-autounseal-operator/tests/config"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// IntegrationTestSuite is a base test suite that provides common setup and utilities
// for all integration tests. It automatically sets up Vault, K3s, and controller
// components based on the test requirements.
type IntegrationTestSuite struct {
	suite.Suite

	// Core components
	ctx       context.Context
	ctxCancel context.CancelFunc
	config    *config.Config

	// Managers for TestContainers
	vaultManager *VaultManager
	k3sManager   *K3sManager
	crdGenerator *CRDGenerator

	// Kubernetes components
	scheme        *runtime.Scheme
	k8sClient     client.Client
	reconciler    *controller.VaultUnsealConfigReconciler

	// Test configuration
	testTimeout   time.Duration
	setupOptions  *IntegrationSetupOptions
}

// IntegrationSetupOptions configures what components to set up for a test
type IntegrationSetupOptions struct {
	// Which components to initialize
	RequiresVault          bool
	RequiresK3s           bool
	RequiresController    bool
	RequiresCRDs          bool

	// Vault configuration
	VaultMode            VaultMode
	VaultVersion         string
	NumVaultInstances    int
	VaultInstanceNames   []string

	// K3s configuration
	K3sVersion           string
	K3sNamespace         string

	// Controller configuration
	UseRealK8sClient     bool  // Use real K3s client vs fake client
	EnableLeaderElection bool

	// Test behavior
	SkipInShortMode      bool
	CustomTimeout        time.Duration
}

// DefaultIntegrationSetupOptions returns sensible defaults for integration tests
func DefaultIntegrationSetupOptions() *IntegrationSetupOptions {
	return &IntegrationSetupOptions{
		RequiresVault:       true,
		RequiresK3s:        false,
		RequiresController: true,
		RequiresCRDs:       true,

		VaultMode:          DevMode,
		NumVaultInstances:  1,
		VaultInstanceNames: []string{"default"},

		K3sNamespace:       "default",
		UseRealK8sClient:   false,
		SkipInShortMode:    true,
		CustomTimeout:      15 * time.Minute,
	}
}

// VaultOnlyOptions returns options for Vault-only tests
func VaultOnlyOptions() *IntegrationSetupOptions {
	opts := DefaultIntegrationSetupOptions()
	opts.RequiresK3s = false
	opts.RequiresController = false
	opts.RequiresCRDs = false
	opts.UseRealK8sClient = false
	return opts
}

// K3sOnlyOptions returns options for K3s-only tests
func K3sOnlyOptions() *IntegrationSetupOptions {
	opts := DefaultIntegrationSetupOptions()
	opts.RequiresVault = false
	opts.RequiresK3s = true
	opts.RequiresController = false
	opts.UseRealK8sClient = true
	return opts
}

// FullIntegrationOptions returns options for complete integration tests
func FullIntegrationOptions() *IntegrationSetupOptions {
	opts := DefaultIntegrationSetupOptions()
	opts.RequiresVault = true
	opts.RequiresK3s = true
	opts.RequiresController = true
	opts.RequiresCRDs = true
	opts.UseRealK8sClient = true
	return opts
}

// SetupIntegrationSuite initializes the integration test suite with the given options
func (suite *IntegrationTestSuite) SetupIntegrationSuite(options *IntegrationSetupOptions) {
	if options == nil {
		options = DefaultIntegrationSetupOptions()
	}
	suite.setupOptions = options

	// Skip in short mode if requested
	if options.SkipInShortMode && testing.Short() {
		suite.T().Skip("Skipping integration tests in short mode")
	}

	// Set up timeout
	suite.testTimeout = options.CustomTimeout
	suite.ctx, suite.ctxCancel = context.WithTimeout(context.Background(), suite.testTimeout)

	// Set up logging
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	// Load configuration
	suite.setupConfiguration()

	// Set up Kubernetes scheme
	suite.setupKubernetesScheme()

	// Set up components based on options
	if options.RequiresVault {
		suite.setupVaultManager()
		suite.setupVaultInstances()
	}

	if options.RequiresK3s {
		suite.setupK3sManager()
		suite.setupK3sCluster()
	}

	if options.RequiresCRDs {
		suite.setupCRDGenerator()
	}

	if options.RequiresController {
		suite.setupController()
	}
}

// setupConfiguration loads the global test configuration
func (suite *IntegrationTestSuite) setupConfiguration() {
	var err error
	suite.config, err = config.GetGlobalConfig()
	require.NoError(suite.T(), err, "Failed to load configuration")

	// Validate configuration
	require.NoError(suite.T(), suite.config.Validate(), "Configuration validation failed")

	suite.T().Logf("Loaded test configuration: %s", suite.config.String())
}

// setupKubernetesScheme sets up the Kubernetes runtime scheme
func (suite *IntegrationTestSuite) setupKubernetesScheme() {
	suite.scheme = runtime.NewScheme()
	require.NoError(suite.T(), clientgoscheme.AddToScheme(suite.scheme), "Failed to add client-go scheme")
	require.NoError(suite.T(), vaultv1.AddToScheme(suite.scheme), "Failed to add vault v1 scheme")
}

// setupVaultManager initializes the Vault manager
func (suite *IntegrationTestSuite) setupVaultManager() {
	suite.vaultManager = NewVaultManager(suite.ctx, suite.Suite)
	require.NotNil(suite.T(), suite.vaultManager, "VaultManager should be created successfully")
}

// setupVaultInstances creates the required Vault instances
func (suite *IntegrationTestSuite) setupVaultInstances() {
	opts := suite.setupOptions

	for i := 0; i < opts.NumVaultInstances; i++ {
		var name string
		if i < len(opts.VaultInstanceNames) {
			name = opts.VaultInstanceNames[i]
		} else {
			name = fmt.Sprintf("vault-%d", i)
		}

		var instance *VaultInstance
		var err error

		if opts.VaultVersion != "" {
			instance, err = suite.vaultManager.CreateVaultWithVersion(name, opts.VaultVersion, opts.VaultMode)
		} else if opts.VaultMode == DevMode {
			instance, err = suite.vaultManager.CreateDevVault(name)
		} else {
			instance, err = suite.vaultManager.CreateProdVault(name)
		}

		require.NoError(suite.T(), err, "Failed to create vault instance %s", name)
		require.NotNil(suite.T(), instance, "Vault instance %s should be created", name)

		suite.T().Logf("Created vault instance '%s' at %s (mode: %v)", name, instance.Address, instance.Mode)
	}
}

// setupK3sManager initializes the K3s manager
func (suite *IntegrationTestSuite) setupK3sManager() {
	suite.k3sManager = NewK3sManager(suite.ctx, suite.Suite)
	require.NotNil(suite.T(), suite.k3sManager, "K3sManager should be created successfully")
}

// setupK3sCluster creates a K3s cluster with CRDs if required
func (suite *IntegrationTestSuite) setupK3sCluster() {
	opts := suite.setupOptions

	var crdManifests []string
	if opts.RequiresCRDs && suite.crdGenerator != nil {
		crdManifests = append(crdManifests, suite.crdGenerator.GenerateVaultUnsealConfigCRD())
		crdManifests = append(crdManifests, suite.crdGenerator.GenerateRBACManifests(opts.K3sNamespace))
	}

	var instance *K3sInstance
	var err error

	if opts.K3sVersion != "" {
		instance, err = suite.k3sManager.CreateK3sClusterWithVersion("default", opts.K3sVersion, crdManifests...)
	} else {
		instance, err = suite.k3sManager.CreateK3sCluster("default", crdManifests...)
	}

	require.NoError(suite.T(), err, "Failed to create K3s cluster")
	require.NotNil(suite.T(), instance, "K3s instance should be created")

	if opts.UseRealK8sClient {
		suite.k8sClient = instance.Client
	}

	// Wait for CRDs to be ready if they were installed
	if opts.RequiresCRDs {
		err = suite.k3sManager.WaitForCRDReady(instance, "vaultunsealconfigs.vault.io", 60*time.Second)
		require.NoError(suite.T(), err, "CRDs should become ready")
	}

	suite.T().Log("K3s cluster ready with CRDs installed")
}

// setupCRDGenerator initializes the CRD generator
func (suite *IntegrationTestSuite) setupCRDGenerator() {
	suite.crdGenerator = NewCRDGenerator()
}

// setupController initializes the Vault controller
func (suite *IntegrationTestSuite) setupController() {
	if suite.k8sClient == nil {
		// Use fake client if no real K8s client is set up
		suite.k8sClient = fake.NewClientBuilder().
			WithScheme(suite.scheme).
			Build()
	}

	// Create controller with mock repository
	mockRepo := &mocks.MockVaultClientRepository{}
	// Set up mock to return error when trying to connect (since we don't have real vault)
	mockRepo.On("GetClient", mock.Anything, mock.AnythingOfType("string"), mock.Anything).
		Return(nil, errors.New("vault connection failed - expected in integration test")).Maybe()

	suite.reconciler = controller.NewVaultUnsealConfigReconciler(
		suite.k8sClient,
		ctrl.Log.WithName("controllers").WithName("VaultUnsealConfig"),
		suite.scheme,
		mockRepo,
		nil, // Use default options
	)

	suite.T().Log("Controller initialized successfully")
}

// TearDownIntegrationSuite cleans up all resources
func (suite *IntegrationTestSuite) TearDownIntegrationSuite() {
	if suite.vaultManager != nil {
		suite.vaultManager.Cleanup()
	}

	if suite.k3sManager != nil {
		suite.k3sManager.Cleanup()
	}

	if suite.ctxCancel != nil {
		suite.ctxCancel()
	}
}

// Helper Methods for Test Cases

// GetVaultInstance returns a Vault instance by name
func (suite *IntegrationTestSuite) GetVaultInstance(name string) (*VaultInstance, bool) {
	if suite.vaultManager == nil {
		return nil, false
	}
	return suite.vaultManager.GetInstance(name)
}

// GetDefaultVaultInstance returns the first Vault instance (commonly used in tests)
func (suite *IntegrationTestSuite) GetDefaultVaultInstance() *VaultInstance {
	if suite.vaultManager == nil {
		return nil
	}

	instanceName := "default"
	if len(suite.setupOptions.VaultInstanceNames) > 0 {
		instanceName = suite.setupOptions.VaultInstanceNames[0]
	}

	instance, exists := suite.vaultManager.GetInstance(instanceName)
	if !exists {
		// Try fallback names
		for _, name := range []string{"vault-0", "vault"} {
			if instance, exists = suite.vaultManager.GetInstance(name); exists {
				break
			}
		}
	}

	return instance
}

// GetK3sInstance returns the K3s instance
func (suite *IntegrationTestSuite) GetK3sInstance() (*K3sInstance, bool) {
	if suite.k3sManager == nil {
		return nil, false
	}
	return suite.k3sManager.GetInstance("default")
}

// CreateTestVaultUnsealConfig creates a test VaultUnsealConfig resource
func (suite *IntegrationTestSuite) CreateTestVaultUnsealConfig(name, namespace string, vaultConfigs []VaultInstanceConfig) *vaultv1.VaultUnsealConfig {
	require.NotNil(suite.T(), suite.crdGenerator, "CRD generator not available - ensure RequiresCRDs is true")

	// Generate YAML manifest
	manifest := suite.crdGenerator.GenerateTestVaultUnsealConfig(name, namespace, vaultConfigs)

	// For integration tests, we might want to create the actual resource
	// This is a placeholder for now - implement based on test needs
	suite.T().Logf("Generated VaultUnsealConfig manifest for %s/%s (%d bytes)", namespace, name, len(manifest))

	return nil // TODO: Parse YAML and return the resource
}

// ReconcileVaultUnsealConfig performs a reconciliation cycle for the given resource
func (suite *IntegrationTestSuite) ReconcileVaultUnsealConfig(namespacedName types.NamespacedName) (reconcile.Result, error) {
	require.NotNil(suite.T(), suite.reconciler, "Controller not available - ensure RequiresController is true")

	req := reconcile.Request{NamespacedName: namespacedName}
	return suite.reconciler.Reconcile(suite.ctx, req)
}

// WaitForCondition waits for a specific condition on a resource with timeout
func (suite *IntegrationTestSuite) WaitForCondition(namespacedName types.NamespacedName, conditionType string, timeout time.Duration) error {
	require.NotNil(suite.T(), suite.k8sClient, "K8s client not available")

	ctx, cancel := context.WithTimeout(suite.ctx, timeout)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for condition %s on %s", conditionType, namespacedName)
		default:
			var resource vaultv1.VaultUnsealConfig
			err := suite.k8sClient.Get(suite.ctx, namespacedName, &resource)
			if err != nil {
				time.Sleep(1 * time.Second)
				continue
			}

			// Check if condition exists
			for _, condition := range resource.Status.Conditions {
				if condition.Type == conditionType && condition.Status == "True" {
					return nil
				}
			}

			time.Sleep(1 * time.Second)
		}
	}
}

// AssertVaultHealth verifies that a Vault instance is healthy
func (suite *IntegrationTestSuite) AssertVaultHealth(instanceName string, expectSealed bool) {
	instance, exists := suite.GetVaultInstance(instanceName)
	require.True(suite.T(), exists, "Vault instance %s should exist", instanceName)

	err := suite.vaultManager.VerifyVaultHealth(instance, expectSealed)
	require.NoError(suite.T(), err, "Vault instance %s should be healthy", instanceName)
}

// Config returns the test configuration
func (suite *IntegrationTestSuite) Config() *config.Config {
	return suite.config
}

// Context returns the test context
func (suite *IntegrationTestSuite) Context() context.Context {
	return suite.ctx
}

// VaultManager returns the vault manager
func (suite *IntegrationTestSuite) VaultManager() *VaultManager {
	return suite.vaultManager
}

// K3sManager returns the K3s manager
func (suite *IntegrationTestSuite) K3sManager() *K3sManager {
	return suite.k3sManager
}

// CRDGenerator returns the CRD generator
func (suite *IntegrationTestSuite) CRDGenerator() *CRDGenerator {
	return suite.crdGenerator
}

// Reconciler returns the controller reconciler
func (suite *IntegrationTestSuite) Reconciler() *controller.VaultUnsealConfigReconciler {
	return suite.reconciler
}

// K8sClient returns the Kubernetes client
func (suite *IntegrationTestSuite) K8sClient() client.Client {
	return suite.k8sClient
}
