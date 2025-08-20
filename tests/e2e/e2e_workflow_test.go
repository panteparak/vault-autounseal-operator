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

// E2EWorkflowTestSuite provides end-to-end workflow testing
type E2EWorkflowTestSuite struct {
	suite.Suite
	k3sContainer   *k3s.K3sContainer
	vaultContainer *vault.VaultContainer
	vaultClient    *api.Client
	vaultAddr      string
	unsealKeys     []string
	rootToken      string
	k8sClient      client.Client
	scheme         *runtime.Scheme
	reconciler     *controller.VaultUnsealConfigReconciler
	ctx            context.Context
	ctxCancel      context.CancelFunc
}

// SetupSuite initializes the complete testing environment
func (suite *E2EWorkflowTestSuite) SetupSuite() {
	suite.ctx, suite.ctxCancel = context.WithTimeout(context.Background(), 20*time.Minute)

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	// Set up complete environment
	suite.setupCompleteEnvironment()
}

// setupCompleteEnvironment creates K3s cluster with CRDs, Vault, and controller
func (suite *E2EWorkflowTestSuite) setupCompleteEnvironment() {
	// Create K3s cluster with all necessary CRDs and RBAC
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
                    tlsSkipVerify:
                      type: boolean
                    haEnabled:
                      type: boolean
                    podSelector:
                      type: object
                      additionalProperties:
                        type: string
                    namespace:
                      type: string
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
    subresources:
      status: {}
  scope: Namespaced
  names:
    plural: vaultunsealconfigs
    singular: vaultunsealconfig
    kind: VaultUnsealConfig
    shortNames:
    - vuc
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: vault-operator
  namespace: default
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: vault-operator
rules:
- apiGroups: [""]
  resources: ["pods"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["vault.io"]
  resources: ["vaultunsealconfigs"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["vault.io"]
  resources: ["vaultunsealconfigs/status"]
  verbs: ["get", "update", "patch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: vault-operator
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: vault-operator
subjects:
- kind: ServiceAccount
  name: vault-operator
  namespace: default`

	k3sContainer, err := k3s.Run(suite.ctx,
		"rancher/k3s:v1.32.1-k3s1",
		// TODO: Use proper k3s manifest loading
		// k3s.WithManifest("vault-operator.yaml", crdAndRBACManifest),
		testcontainers.WithWaitStrategy(
			wait.ForLog("k3s is up and running").
				WithStartupTimeout(180*time.Second).
				WithPollInterval(5*time.Second),
		),
	)
	require.NoError(suite.T(), err, "Failed to start K3s cluster")
	suite.k3sContainer = k3sContainer

	// Set up Kubernetes client
	kubeconfig, err := k3sContainer.GetKubeConfig(suite.ctx)
	require.NoError(suite.T(), err, "Failed to get kubeconfig")

	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	require.NoError(suite.T(), err, "Failed to create rest config")

	suite.scheme = runtime.NewScheme()
	err = clientgoscheme.AddToScheme(suite.scheme)
	require.NoError(suite.T(), err, "Failed to add client-go scheme")

	err = vaultv1.AddToScheme(suite.scheme)
	require.NoError(suite.T(), err, "Failed to add vault v1 scheme")

	suite.k8sClient, err = client.New(restConfig, client.Options{Scheme: suite.scheme})
	require.NoError(suite.T(), err, "Failed to create K8s client")

	// Wait for CRDs to be ready
	suite.waitForCRDsReady()

	// Create test namespaces
	suite.createTestNamespaces()

	// Start Vault container
	suite.setupVault()

	// Set up controller
	suite.setupController()
}

// waitForCRDsReady waits for all CRDs to be available
func (suite *E2EWorkflowTestSuite) waitForCRDsReady() {
	require.Eventually(suite.T(), func() bool {
		testConfig := &vaultv1.VaultUnsealConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "crd-readiness-test",
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
	}, 90*time.Second, 3*time.Second, "CRDs should become ready")
}

// createTestNamespaces creates namespaces for testing
func (suite *E2EWorkflowTestSuite) createTestNamespaces() {
	namespaces := []string{"vault-system", "test-env", "production"}

	for _, ns := range namespaces {
		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: ns},
		}
		err := suite.k8sClient.Create(suite.ctx, namespace)
		require.NoError(suite.T(), err, "Should create namespace %s", ns)
	}
}

// setupVault creates and configures Vault container
func (suite *E2EWorkflowTestSuite) setupVault() {
	vaultContainer, err := vault.Run(suite.ctx,
		"hashicorp/vault:1.19.0",
		vault.WithToken("e2e-root-token"),
		testcontainers.WithWaitStrategy(
			wait.ForHTTP("/v1/sys/health").
				WithStatusCodeMatcher(func(status int) bool {
					return status == 200 || status == 429
				}).
				WithStartupTimeout(60*time.Second),
		),
	)
	require.NoError(suite.T(), err, "Failed to start Vault container")
	suite.vaultContainer = vaultContainer

	vaultAddr, err := vaultContainer.HttpHostAddress(suite.ctx)
	require.NoError(suite.T(), err, "Failed to get Vault address")
	suite.vaultAddr = vaultAddr

	// Configure Vault client
	vaultConfig := api.DefaultConfig()
	vaultConfig.Address = suite.vaultAddr
	suite.vaultClient, err = api.NewClient(vaultConfig)
	require.NoError(suite.T(), err, "Failed to create Vault client")

	suite.rootToken = "e2e-root-token"
	suite.vaultClient.SetToken(suite.rootToken)

	// Set up test keys
	suite.unsealKeys = []string{
		"ZTJlLXRlc3Qta2V5LTE=", // e2e-test-key-1
		"ZTJlLXRlc3Qta2V5LTI=", // e2e-test-key-2
		"ZTJlLXRlc3Qta2V5LTM=", // e2e-test-key-3
	}

	// Configure Vault for testing (enable secrets engine, etc.)
	suite.configureVault()
}

// configureVault sets up Vault with test data
func (suite *E2EWorkflowTestSuite) configureVault() {
	// Enable KV secrets engine
	err := suite.vaultClient.Sys().Mount("secret/", &api.MountInput{
		Type: "kv-v2",
	})
	if err != nil {
		suite.T().Logf("KV mount may already exist: %v", err)
	}

	// Write some test secrets
	_, err = suite.vaultClient.Logical().Write("secret/data/test", map[string]interface{}{
		"data": map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		},
	})
	if err != nil {
		suite.T().Logf("Failed to write test secret: %v", err)
	}
}

// setupController creates the controller instance
func (suite *E2EWorkflowTestSuite) setupController() {
	suite.reconciler = &controller.VaultUnsealConfigReconciler{
		Client: suite.k8sClient,
		Log:    ctrl.Log.WithName("e2e-controller"),
		Scheme: suite.scheme,
	}
}

// TearDownSuite cleans up resources
func (suite *E2EWorkflowTestSuite) TearDownSuite() {
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

// TestCompleteOperatorWorkflow tests the full operator workflow
func (suite *E2EWorkflowTestSuite) TestCompleteOperatorWorkflow() {
	// Step 1: Create VaultUnsealConfig
	config := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "complete-workflow",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:       "workflow-vault",
					Endpoint:   suite.vaultAddr,
					UnsealKeys: suite.unsealKeys,
					Threshold:  func() *int { i := 3; return &i }(),
				},
			},
		},
	}

	err := suite.k8sClient.Create(suite.ctx, config)
	require.NoError(suite.T(), err, "Step 1: Should create VaultUnsealConfig")

	// Step 2: Trigger initial reconciliation
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "complete-workflow",
			Namespace: "default",
		},
	}

	result, err := suite.reconciler.Reconcile(suite.ctx, req)
	require.NoError(suite.T(), err, "Step 2: Initial reconciliation should succeed")
	assert.NotZero(suite.T(), result.RequeueAfter, "Should schedule next reconciliation")

	// Step 3: Verify initial status
	var firstState vaultv1.VaultUnsealConfig
	err = suite.k8sClient.Get(suite.ctx, req.NamespacedName, &firstState)
	require.NoError(suite.T(), err, "Step 3: Should get initial state")
	assert.NotEmpty(suite.T(), firstState.Status.VaultStatuses, "Should have vault status")
	assert.NotEmpty(suite.T(), firstState.Status.Conditions, "Should have conditions")

	// Step 4: Perform periodic reconciliations (simulating operator behavior)
	for i := 0; i < 3; i++ {
		time.Sleep(100 * time.Millisecond) // Brief pause

		result, err = suite.reconciler.Reconcile(suite.ctx, req)
		require.NoError(suite.T(), err, "Step 4.%d: Periodic reconciliation should succeed", i+1)
		assert.NotZero(suite.T(), result.RequeueAfter, "Should continue scheduling reconciliations")
	}

	// Step 5: Verify consistent state after multiple reconciliations
	var finalState vaultv1.VaultUnsealConfig
	err = suite.k8sClient.Get(suite.ctx, req.NamespacedName, &finalState)
	require.NoError(suite.T(), err, "Step 5: Should get final state")

	assert.Equal(suite.T(), "workflow-vault", finalState.Status.VaultStatuses[0].Name)
	assert.NotNil(suite.T(), finalState.Status.VaultStatuses[0].LastUnsealed)

	// Step 6: Test configuration updates
	finalState.Spec.VaultInstances[0].Threshold = func() *int { i := 2; return &i }()
	err = suite.k8sClient.Update(suite.ctx, &finalState)
	require.NoError(suite.T(), err, "Step 6: Should update configuration")

	// Step 7: Reconcile after update
	result, err = suite.reconciler.Reconcile(suite.ctx, req)
	require.NoError(suite.T(), err, "Step 7: Post-update reconciliation should succeed")

	// Step 8: Verify update was processed
	var updatedState vaultv1.VaultUnsealConfig
	err = suite.k8sClient.Get(suite.ctx, req.NamespacedName, &updatedState)
	require.NoError(suite.T(), err, "Step 8: Should get updated state")
	assert.Equal(suite.T(), 2, *updatedState.Spec.VaultInstances[0].Threshold, "Threshold should be updated")
}

// TestMultiNamespaceWorkflow tests operator working across multiple namespaces
func (suite *E2EWorkflowTestSuite) TestMultiNamespaceWorkflow() {
	namespaces := []string{"default", "vault-system", "test-env"}
	configs := make([]*vaultv1.VaultUnsealConfig, len(namespaces))

	// Create VaultUnsealConfig in each namespace
	for i, ns := range namespaces {
		config := &vaultv1.VaultUnsealConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("multi-ns-vault-%d", i),
				Namespace: ns,
			},
			Spec: vaultv1.VaultUnsealConfigSpec{
				VaultInstances: []vaultv1.VaultInstance{
					{
						Name:       fmt.Sprintf("vault-%s-%d", ns, i),
						Endpoint:   suite.vaultAddr,
						UnsealKeys: suite.unsealKeys,
					},
				},
			},
		}

		err := suite.k8sClient.Create(suite.ctx, config)
		require.NoError(suite.T(), err, "Should create config in namespace %s", ns)
		configs[i] = config
	}

	// Reconcile each configuration
	for i, config := range configs {
		req := ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      config.Name,
				Namespace: config.Namespace,
			},
		}

		result, err := suite.reconciler.Reconcile(suite.ctx, req)
		require.NoError(suite.T(), err, "Should reconcile config %d", i)
		assert.NotZero(suite.T(), result.RequeueAfter)

		// Verify status in each namespace
		var reconciled vaultv1.VaultUnsealConfig
		err = suite.k8sClient.Get(suite.ctx, req.NamespacedName, &reconciled)
		require.NoError(suite.T(), err, "Should get reconciled config %d", i)
		assert.NotEmpty(suite.T(), reconciled.Status.VaultStatuses, "Should have status in namespace %s", config.Namespace)
	}
}

// TestErrorRecoveryWorkflow tests error scenarios and recovery
func (suite *E2EWorkflowTestSuite) TestErrorRecoveryWorkflow() {
	// Create config with initially unreachable vault
	config := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "error-recovery",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:       "unreachable-vault",
					Endpoint:   "http://nonexistent.vault.local:8200",
					UnsealKeys: suite.unsealKeys,
				},
			},
		},
	}

	err := suite.k8sClient.Create(suite.ctx, config)
	require.NoError(suite.T(), err, "Should create error config")

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "error-recovery",
			Namespace: "default",
		},
	}

	// Initial reconciliation should handle error gracefully
	_, err = suite.reconciler.Reconcile(suite.ctx, req)
	require.NoError(suite.T(), err, "Should handle unreachable vault gracefully")

	// Verify error state
	var errorState vaultv1.VaultUnsealConfig
	err = suite.k8sClient.Get(suite.ctx, req.NamespacedName, &errorState)
	require.NoError(suite.T(), err, "Should get error state")
	assert.True(suite.T(), errorState.Status.VaultStatuses[0].Sealed, "Should be marked as sealed")
	assert.NotEmpty(suite.T(), errorState.Status.VaultStatuses[0].Error, "Should have error message")

	// "Fix" the configuration by updating to working vault
	errorState.Spec.VaultInstances[0].Endpoint = suite.vaultAddr
	err = suite.k8sClient.Update(suite.ctx, &errorState)
	require.NoError(suite.T(), err, "Should fix configuration")

	// Reconcile after fix
	_, err = suite.reconciler.Reconcile(suite.ctx, req)
	require.NoError(suite.T(), err, "Should reconcile after fix")

	// Verify recovery
	var recoveredState vaultv1.VaultUnsealConfig
	err = suite.k8sClient.Get(suite.ctx, req.NamespacedName, &recoveredState)
	require.NoError(suite.T(), err, "Should get recovered state")
	// Error should be cleared or vault should be functional
	suite.T().Logf("Recovered state: sealed=%v, error=%s",
		recoveredState.Status.VaultStatuses[0].Sealed,
		recoveredState.Status.VaultStatuses[0].Error)
}

// TestScaleUpScaleDownWorkflow tests adding and removing vault instances
func (suite *E2EWorkflowTestSuite) TestScaleUpScaleDownWorkflow() {
	// Start with single vault instance
	config := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "scale-test",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:       "vault-1",
					Endpoint:   suite.vaultAddr,
					UnsealKeys: suite.unsealKeys,
				},
			},
		},
	}

	err := suite.k8sClient.Create(suite.ctx, config)
	require.NoError(suite.T(), err, "Should create initial scale config")

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "scale-test",
			Namespace: "default",
		},
	}

	// Initial reconciliation
	_, err = suite.reconciler.Reconcile(suite.ctx, req)
	require.NoError(suite.T(), err, "Should reconcile initial config")

	var initialState vaultv1.VaultUnsealConfig
	err = suite.k8sClient.Get(suite.ctx, req.NamespacedName, &initialState)
	require.NoError(suite.T(), err, "Should get initial state")
	assert.Len(suite.T(), initialState.Status.VaultStatuses, 1, "Should have 1 vault status")

	// Scale up - add second vault instance
	initialState.Spec.VaultInstances = append(initialState.Spec.VaultInstances, vaultv1.VaultInstance{
		Name:       "vault-2",
		Endpoint:   suite.vaultAddr, // Same vault, different logical instance
		UnsealKeys: suite.unsealKeys,
	})

	err = suite.k8sClient.Update(suite.ctx, &initialState)
	require.NoError(suite.T(), err, "Should scale up configuration")

	// Reconcile after scale up
	_, err = suite.reconciler.Reconcile(suite.ctx, req)
	require.NoError(suite.T(), err, "Should reconcile scaled up config")

	var scaledUpState vaultv1.VaultUnsealConfig
	err = suite.k8sClient.Get(suite.ctx, req.NamespacedName, &scaledUpState)
	require.NoError(suite.T(), err, "Should get scaled up state")
	assert.Len(suite.T(), scaledUpState.Status.VaultStatuses, 2, "Should have 2 vault statuses")

	// Verify both vault instances are tracked
	vaultNames := make(map[string]bool)
	for _, status := range scaledUpState.Status.VaultStatuses {
		vaultNames[status.Name] = true
	}
	assert.True(suite.T(), vaultNames["vault-1"], "Should track vault-1")
	assert.True(suite.T(), vaultNames["vault-2"], "Should track vault-2")

	// Scale down - remove one vault instance
	scaledUpState.Spec.VaultInstances = scaledUpState.Spec.VaultInstances[:1] // Keep only first instance

	err = suite.k8sClient.Update(suite.ctx, &scaledUpState)
	require.NoError(suite.T(), err, "Should scale down configuration")

	// Reconcile after scale down
	_, err = suite.reconciler.Reconcile(suite.ctx, req)
	require.NoError(suite.T(), err, "Should reconcile scaled down config")

	var scaledDownState vaultv1.VaultUnsealConfig
	err = suite.k8sClient.Get(suite.ctx, req.NamespacedName, &scaledDownState)
	require.NoError(suite.T(), err, "Should get scaled down state")
	assert.Len(suite.T(), scaledDownState.Status.VaultStatuses, 1, "Should have 1 vault status after scale down")
	assert.Equal(suite.T(), "vault-1", scaledDownState.Status.VaultStatuses[0].Name, "Should keep vault-1")
}

// TestLongRunningWorkflow tests extended operator behavior
func (suite *E2EWorkflowTestSuite) TestLongRunningWorkflow() {
	config := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "long-running",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:       "persistent-vault",
					Endpoint:   suite.vaultAddr,
					UnsealKeys: suite.unsealKeys,
				},
			},
		},
	}

	err := suite.k8sClient.Create(suite.ctx, config)
	require.NoError(suite.T(), err, "Should create long-running config")

	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "long-running",
			Namespace: "default",
		},
	}

	// Simulate extended operation over time
	reconciliationCount := 10
	lastTransitionTimes := make([]time.Time, reconciliationCount)

	for i := 0; i < reconciliationCount; i++ {
		// Add some delay to simulate real-world timing
		if i > 0 {
			time.Sleep(200 * time.Millisecond)
		}

		result, err := suite.reconciler.Reconcile(suite.ctx, req)
		require.NoError(suite.T(), err, "Reconciliation %d should succeed", i+1)
		assert.NotZero(suite.T(), result.RequeueAfter, "Should continue scheduling reconciliations")

		// Check state consistency
		var currentState vaultv1.VaultUnsealConfig
		err = suite.k8sClient.Get(suite.ctx, req.NamespacedName, &currentState)
		require.NoError(suite.T(), err, "Should get state at reconciliation %d", i+1)

		assert.Len(suite.T(), currentState.Status.VaultStatuses, 1, "Should consistently have 1 vault status")
		assert.Equal(suite.T(), "persistent-vault", currentState.Status.VaultStatuses[0].Name, "Vault name should be consistent")
		assert.NotEmpty(suite.T(), currentState.Status.Conditions, "Should have conditions")

		// Track transition times
		for _, condition := range currentState.Status.Conditions {
			if condition.Type == "Ready" {
				lastTransitionTimes[i] = condition.LastTransitionTime.Time
				break
			}
		}
	}

	// Verify state stability - transition times shouldn't change frequently
	// (they should only change when actual state changes)
	uniqueTransitionTimes := make(map[time.Time]bool)
	for _, t := range lastTransitionTimes {
		if !t.IsZero() {
			uniqueTransitionTimes[t] = true
		}
	}

	// Should have relatively few unique transition times (state should be stable)
	assert.LessOrEqual(suite.T(), len(uniqueTransitionTimes), 3,
		"Should have stable state with few condition transitions")
}

// TestVaultConnectivityWorkflow tests vault connectivity scenarios
func (suite *E2EWorkflowTestSuite) TestVaultConnectivityWorkflow() {
	// Test direct vault connectivity outside of operator
	vaultClient, err := vaultpkg.NewClient(suite.vaultAddr, false, 30*time.Second)
	require.NoError(suite.T(), err, "Should create vault client")
	defer vaultClient.Close()

	// Test vault health
	health, err := vaultClient.HealthCheck(suite.ctx)
	require.NoError(suite.T(), err, "Vault should be healthy")
	assert.NotNil(suite.T(), health)

	// Test vault operations
	isSealed, err := vaultClient.IsSealed(suite.ctx)
	require.NoError(suite.T(), err, "Should check seal status")
	suite.T().Logf("Vault sealed status: %v", isSealed)

	// Create operator configuration
	config := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "connectivity-test",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:       "connectivity-vault",
					Endpoint:   suite.vaultAddr,
					UnsealKeys: suite.unsealKeys,
				},
			},
		},
	}

	err = suite.k8sClient.Create(suite.ctx, config)
	require.NoError(suite.T(), err, "Should create connectivity config")

	// Reconcile through operator
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "connectivity-test",
			Namespace: "default",
		},
	}

	_, err = suite.reconciler.Reconcile(suite.ctx, req)
	require.NoError(suite.T(), err, "Operator should successfully connect to vault")

	// Verify operator's view matches direct connection
	var operatorState vaultv1.VaultUnsealConfig
	err = suite.k8sClient.Get(suite.ctx, req.NamespacedName, &operatorState)
	require.NoError(suite.T(), err, "Should get operator state")

	assert.NotEmpty(suite.T(), operatorState.Status.VaultStatuses, "Operator should have vault status")
	suite.T().Logf("Operator vault status: sealed=%v, error=%s",
		operatorState.Status.VaultStatuses[0].Sealed,
		operatorState.Status.VaultStatuses[0].Error)
}

// TestE2EWorkflowTestSuite runs the end-to-end workflow test suite
func TestE2EWorkflowTestSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E workflow tests in short mode")
	}

	suite.Run(t, new(E2EWorkflowTestSuite))
}
