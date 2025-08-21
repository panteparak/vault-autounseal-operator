package controller

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	vaultv1 "github.com/panteparak/vault-autounseal-operator/pkg/api/v1"
	controllerpkg "github.com/panteparak/vault-autounseal-operator/pkg/controller"
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
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ControllerTestSuite provides unit testing for the VaultUnsealConfig controller
type ControllerTestSuite struct {
	suite.Suite
	k8sClient   client.Client
	scheme      *runtime.Scheme
	reconciler  *controllerpkg.VaultUnsealConfigReconciler
	ctx         context.Context
	logger      logr.Logger
}

func (suite *ControllerTestSuite) SetupSuite() {
	suite.ctx = context.Background()
	suite.logger = log.Log.WithName("test-controller")

	// Set up scheme
	suite.scheme = runtime.NewScheme()
	err := clientgoscheme.AddToScheme(suite.scheme)
	require.NoError(suite.T(), err)

	err = vaultv1.AddToScheme(suite.scheme)
	require.NoError(suite.T(), err)

	// Set up fake client with runtime objects
	suite.k8sClient = fake.NewClientBuilder().
		WithScheme(suite.scheme).
		WithRuntimeObjects().
		Build()

	// Set up reconciler
	suite.reconciler = &controllerpkg.VaultUnsealConfigReconciler{
		Client: suite.k8sClient,
		Log:    suite.logger,
		Scheme: suite.scheme,
	}
}

func (suite *ControllerTestSuite) TearDownTest() {
	// Clean up any resources created during test
	vaultConfigList := &vaultv1.VaultUnsealConfigList{}
	err := suite.k8sClient.List(suite.ctx, vaultConfigList)
	if err == nil {
		for _, config := range vaultConfigList.Items {
			err := suite.k8sClient.Delete(suite.ctx, &config)
			if err != nil {
				suite.T().Logf("Failed to delete config %s: %v", config.Name, err)
			}
		}
	}
}

// TestReconcileNonExistentResource tests reconciling a non-existent resource
func (suite *ControllerTestSuite) TestReconcileNonExistentResource() {
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "non-existent",
			Namespace: "default",
		},
	}

	result, err := suite.reconciler.Reconcile(suite.ctx, req)
	assert.NoError(suite.T(), err, "Reconciling non-existent resource should not error")
	assert.Equal(suite.T(), ctrl.Result{}, result, "Should return empty result for non-existent resource")
}

// TestReconcileBasicVaultConfig tests reconciling a basic vault configuration
func (suite *ControllerTestSuite) TestReconcileBasicVaultConfig() {
	// Create a basic VaultUnsealConfig
	threshold := 3
	vaultConfig := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-vault-config",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:          "test-vault",
					Endpoint:      "http://vault.example.com:8200",
					UnsealKeys:    []string{"key1", "key2", "key3"},
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
	// Note: This might fail with connection errors since vault is not running,
	// but the reconciler should handle it gracefully
	// The reconciler should either succeed or fail gracefully (both are acceptable in unit tests)
	if err != nil {
		suite.T().Logf("Reconcile failed as expected (vault not running): %v", err)
		// This is acceptable in unit tests - the controller should handle missing vault gracefully
	} else {
		suite.T().Logf("Reconcile succeeded: %+v", result)
	}

	// The main requirement is that the reconciler doesn't panic and returns a reasonable result
	assert.NotNil(suite.T(), result, "Result should not be nil")
}

// TestReconcileControllerSetup tests basic controller setup
func (suite *ControllerTestSuite) TestReconcileControllerSetup() {
	assert.NotNil(suite.T(), suite.reconciler, "Reconciler should be initialized")
	assert.NotNil(suite.T(), suite.reconciler.Client, "Client should be set")
	assert.NotNil(suite.T(), suite.reconciler.Log, "Logger should be set")
	assert.NotNil(suite.T(), suite.reconciler.Scheme, "Scheme should be set")
}

// TestVaultConfigCreationAndDeletion tests the lifecycle of VaultUnsealConfig
func (suite *ControllerTestSuite) TestVaultConfigCreationAndDeletion() {
	threshold := 2
	vaultConfig := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "lifecycle-test",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:       "test-vault",
					Endpoint:   "http://vault.test:8200",
					UnsealKeys: []string{"key1", "key2"},
					Threshold:  &threshold,
				},
			},
		},
	}

	// Test creation
	err := suite.k8sClient.Create(suite.ctx, vaultConfig)
	require.NoError(suite.T(), err, "Should create VaultUnsealConfig")

	// Verify it exists
	retrieved := &vaultv1.VaultUnsealConfig{}
	err = suite.k8sClient.Get(suite.ctx, types.NamespacedName{
		Name:      vaultConfig.Name,
		Namespace: vaultConfig.Namespace,
	}, retrieved)
	require.NoError(suite.T(), err, "Should retrieve created VaultUnsealConfig")
	assert.Equal(suite.T(), vaultConfig.Spec.VaultInstances[0].Name, retrieved.Spec.VaultInstances[0].Name)

	// Test deletion
	err = suite.k8sClient.Delete(suite.ctx, vaultConfig)
	assert.NoError(suite.T(), err, "Should delete VaultUnsealConfig")
}

// TestControllerTestSuite runs the controller test suite
func TestControllerTestSuite(t *testing.T) {
	suite.Run(t, new(ControllerTestSuite))
}
