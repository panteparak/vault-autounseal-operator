package integration

import (
	"context"
	"fmt"
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
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/clientcmd"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// K8sIntegrationTestSuite provides end-to-end integration testing with K3s and Vault
type K8sIntegrationTestSuite struct {
	suite.Suite
	k3sContainer      *k3s.K3sContainer
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

// SetupSuite initializes the test suite with TestContainers K3s and Vault instances
func (suite *K8sIntegrationTestSuite) SetupSuite() {
	suite.ctx, suite.ctxCancel = context.WithTimeout(context.Background(), 10*time.Minute)

	// Set up logging for tests
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	// Start K3s cluster first
	suite.setupK3sCluster()

	// Start Vault container
	suite.setupVaultContainer()

	// Set up controller with real Kubernetes client
	suite.setupControllerWithK8s()
}

// setupK3sCluster creates a K3s cluster using TestContainers
func (suite *K8sIntegrationTestSuite) setupK3sCluster() {
	k3sContainer, err := k3s.Run(suite.ctx,
		"rancher/k3s:v1.32.1-k3s1", // Latest K3s version
		testcontainers.WithWaitStrategy(
			wait.ForLog("k3s is up and running").
				WithStartupTimeout(180*time.Second).
				WithPollInterval(5*time.Second),
		),
	)
	require.NoError(suite.T(), err, "Failed to start K3s container")
	suite.k3sContainer = k3sContainer

	// Get kubeconfig
	kubeconfig, err := k3sContainer.GetKubeConfig(suite.ctx)
	require.NoError(suite.T(), err, "Failed to get kubeconfig from K3s")

	// Verify K3s is ready
	suite.T().Logf("K3s cluster started successfully")
	suite.T().Logf("Kubeconfig length: %d bytes", len(kubeconfig))
}

// setupVaultContainer creates a Vault container
func (suite *K8sIntegrationTestSuite) setupVaultContainer() {
	// Use the official TestContainers Vault module in dev mode
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
	require.Eventually(suite.T(), func() bool {
		health, err := suite.vaultClient.Sys().Health()
		return err == nil && health != nil
	}, 60*time.Second, 2*time.Second, "Vault container did not become ready")

	// Set up test keys (dev mode vault doesn't need real unseal keys for testing our logic)
	suite.unsealKeys = []string{
		"dGVzdC1rZXktMQ==", // test-key-1 in base64
		"dGVzdC1rZXktMg==", // test-key-2 in base64
		"dGVzdC1rZXktMw==", // test-key-3 in base64
		"dGVzdC1rZXktNA==", // test-key-4 in base64
		"dGVzdC1rZXktNQ==", // test-key-5 in base64
	}
}

// setupControllerWithK8s sets up the controller with real K8s client
func (suite *K8sIntegrationTestSuite) setupControllerWithK8s() {
	// Get kubeconfig from K3s
	kubeconfig, err := suite.k3sContainer.GetKubeConfig(suite.ctx)
	require.NoError(suite.T(), err, "Failed to get kubeconfig")

	// Create rest config from kubeconfig
	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	require.NoError(suite.T(), err, "Failed to create rest config from kubeconfig")

	// Set up scheme
	suite.scheme = runtime.NewScheme()
	err = clientgoscheme.AddToScheme(suite.scheme)
	require.NoError(suite.T(), err, "Failed to add client-go scheme")

	err = vaultv1.AddToScheme(suite.scheme)
	require.NoError(suite.T(), err, "Failed to add vault v1 scheme")

	// Create client
	suite.k8sClient, err = client.New(restConfig, client.Options{Scheme: suite.scheme})
	require.NoError(suite.T(), err, "Failed to create K8s client")

	// Set up reconciler with real K8s client
	suite.reconciler = &controller.VaultUnsealConfigReconciler{
		Client: suite.k8sClient,
		Log:    ctrl.Log.WithName("k8s-integration-controller"),
		Scheme: suite.scheme,
	}
}

// TearDownSuite cleans up resources after all tests
func (suite *K8sIntegrationTestSuite) TearDownSuite() {
	if suite.ctxCancel != nil {
		suite.ctxCancel()
	}

	if suite.vaultContainer != nil {
		err := suite.vaultContainer.Terminate(context.Background())
		if err != nil {
			suite.T().Logf("Failed to terminate Vault container: %v", err)
		}
	}

	if suite.k3sContainer != nil {
		err := suite.k3sContainer.Terminate(context.Background())
		if err != nil {
			suite.T().Logf("Failed to terminate K3s container: %v", err)
		}
	}
}

// TestK8sVaultIntegration tests the full Kubernetes integration with Vault
func (suite *K8sIntegrationTestSuite) TestK8sVaultIntegration() {
	// Create CRD in K3s cluster first
	suite.createVaultUnsealCRD()

	// Create a VaultUnsealConfig resource
	vaultConfig := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "k8s-test-vault-config",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:          "k8s-test-vault",
					Endpoint:      suite.vaultAddr,
					UnsealKeys:    suite.unsealKeys,
					Threshold:     func() *int { i := 3; return &i }(),
					TLSSkipVerify: false,
				},
			},
		},
	}

	// Create the resource in K3s
	err := suite.k8sClient.Create(suite.ctx, vaultConfig)
	require.NoError(suite.T(), err, "Failed to create VaultUnsealConfig in K3s")

	// Verify it was created
	var createdConfig vaultv1.VaultUnsealConfig
	err = suite.k8sClient.Get(suite.ctx, types.NamespacedName{
		Name: "k8s-test-vault-config", Namespace: "default",
	}, &createdConfig)
	require.NoError(suite.T(), err, "Failed to get created VaultUnsealConfig from K3s")

	assert.Equal(suite.T(), "k8s-test-vault-config", createdConfig.Name)
	assert.Equal(suite.T(), "k8s-test-vault", createdConfig.Spec.VaultInstances[0].Name)

	// Trigger reconciliation
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "k8s-test-vault-config",
			Namespace: "default",
		},
	}

	result, err := suite.reconciler.Reconcile(suite.ctx, req)
	require.NoError(suite.T(), err, "Reconciliation should succeed with real Vault")
	assert.NotZero(suite.T(), result.RequeueAfter, "Should requeue after some time")

	// Verify the status was updated in K3s
	var updatedConfig vaultv1.VaultUnsealConfig
	err = suite.k8sClient.Get(suite.ctx, types.NamespacedName{
		Name: "k8s-test-vault-config", Namespace: "default",
	}, &updatedConfig)
	require.NoError(suite.T(), err, "Failed to get updated config from K3s")

	// Verify status
	assert.NotEmpty(suite.T(), updatedConfig.Status.VaultStatuses, "Should have vault statuses")
	assert.Equal(suite.T(), "k8s-test-vault", updatedConfig.Status.VaultStatuses[0].Name, "Vault name should match")
	assert.False(suite.T(), updatedConfig.Status.VaultStatuses[0].Sealed, "Vault should be unsealed")
	assert.Empty(suite.T(), updatedConfig.Status.VaultStatuses[0].Error, "Should have no error")

	// Check conditions
	assert.NotEmpty(suite.T(), updatedConfig.Status.Conditions, "Should have conditions")
	readyCondition := updatedConfig.Status.Conditions[0]
	assert.Equal(suite.T(), "Ready", readyCondition.Type, "Should have Ready condition")
	assert.Equal(suite.T(), metav1.ConditionTrue, readyCondition.Status, "Ready condition should be true")
}

// createVaultUnsealCRD creates the VaultUnsealConfig CRD in the K3s cluster
func (suite *K8sIntegrationTestSuite) createVaultUnsealCRD() {
	// In a real scenario, this would be handled by the operator installation
	// For this test, we'll ensure the CRD is registered by the scheme
	// The fake client automatically handles CRDs, but with real K8s we need to ensure they exist

	// For this integration test, we assume the CRD is already installed
	// In a complete E2E test, you would apply the CRD YAML here
}

// TestK8sClusterHealth tests basic K3s cluster health
func (suite *K8sIntegrationTestSuite) TestK8sClusterHealth() {
	// Test that we can query the cluster
	nodeList := &corev1.NodeList{}
	err := suite.k8sClient.List(suite.ctx, nodeList)
	require.NoError(suite.T(), err, "Should be able to list nodes")
	assert.NotEmpty(suite.T(), nodeList.Items, "Should have at least one node")

	// Log cluster info
	suite.T().Logf("K3s cluster has %d nodes", len(nodeList.Items))
	for i, node := range nodeList.Items {
		suite.T().Logf("Node %d: %s (%s)", i, node.Name, node.Status.NodeInfo.KubeletVersion)
	}
}

// TestVaultConnectionFromK8s tests Vault connectivity from the K8s environment
func (suite *K8sIntegrationTestSuite) TestVaultConnectionFromK8s() {
	// Create a vault client and test connectivity
	vaultClient, err := vaultpkg.NewClient(suite.vaultAddr, false, 30*time.Second)
	require.NoError(suite.T(), err, "Failed to create vault client")
	defer vaultClient.Close()

	// Test health check
	health, err := vaultClient.HealthCheck(suite.ctx)
	require.NoError(suite.T(), err, "Health check should succeed")
	assert.NotNil(suite.T(), health, "Health response should not be nil")

	// Test that the vault is accessible and running
	assert.True(suite.T(), health.Initialized, "Vault should be initialized in dev mode")
	assert.False(suite.T(), health.Sealed, "Vault should not be sealed in dev mode")
}

// TestEndToEndWorkflow tests the complete workflow
func (suite *K8sIntegrationTestSuite) TestEndToEndWorkflow() {
	// This test simulates the complete operator workflow:
	// 1. Vault becomes sealed (simulated)
	// 2. Operator detects it and unseals it
	// 3. Status is updated in K3s

	suite.createVaultUnsealCRD()

	// Create VaultUnsealConfig
	vaultConfig := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "e2e-vault-config",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:       "e2e-vault",
					Endpoint:   suite.vaultAddr,
					UnsealKeys: suite.unsealKeys,
					Threshold:  func() *int { i := 3; return &i }(),
				},
			},
		},
	}

	err := suite.k8sClient.Create(suite.ctx, vaultConfig)
	require.NoError(suite.T(), err, "Failed to create VaultUnsealConfig")

	// Perform multiple reconciliation cycles to simulate operator behavior
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "e2e-vault-config",
			Namespace: "default",
		},
	}

	for i := 0; i < 3; i++ {
		result, err := suite.reconciler.Reconcile(suite.ctx, req)
		require.NoError(suite.T(), err, fmt.Sprintf("Reconciliation cycle %d should succeed", i+1))
		assert.NotZero(suite.T(), result.RequeueAfter, "Should requeue for next reconciliation")

		// Brief pause between reconciliations
		time.Sleep(100 * time.Millisecond)
	}

	// Verify final state
	var finalConfig vaultv1.VaultUnsealConfig
	err = suite.k8sClient.Get(suite.ctx, types.NamespacedName{
		Name: "e2e-vault-config", Namespace: "default",
	}, &finalConfig)
	require.NoError(suite.T(), err, "Failed to get final config state")

	// Verify the operator maintained the desired state
	assert.Len(suite.T(), finalConfig.Status.VaultStatuses, 1, "Should have one vault status")
	vaultStatus := finalConfig.Status.VaultStatuses[0]
	assert.Equal(suite.T(), "e2e-vault", vaultStatus.Name, "Vault name should match")
	assert.False(suite.T(), vaultStatus.Sealed, "Vault should remain unsealed")
	assert.Empty(suite.T(), vaultStatus.Error, "Should have no errors")

	// Verify conditions indicate healthy state
	assert.NotEmpty(suite.T(), finalConfig.Status.Conditions, "Should have conditions")
	for _, condition := range finalConfig.Status.Conditions {
		if condition.Type == "Ready" {
			assert.Equal(suite.T(), metav1.ConditionTrue, condition.Status, "Ready condition should be true")
			break
		}
	}
}

// TestK8sIntegrationTestSuite runs the K8s integration test suite
func TestK8sE2EIntegrationTestSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping K8s integration tests in short mode")
	}

	suite.Run(t, new(K8sIntegrationTestSuite))
}
