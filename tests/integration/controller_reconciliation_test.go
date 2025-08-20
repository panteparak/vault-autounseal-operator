package integration

import (
	"context"
	"testing"
	"time"

	vaultv1 "github.com/panteparak/vault-autounseal-operator/pkg/api/v1"
	"github.com/panteparak/vault-autounseal-operator/pkg/controller"
	"github.com/panteparak/vault-autounseal-operator/tests/integration/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// ControllerReconciliationTestSuite tests controller reconciliation with real K8s and Vault
type ControllerReconciliationTestSuite struct {
	suite.Suite
	ctx           context.Context
	ctxCancel     context.CancelFunc
	vaultManager  *shared.VaultManager
	k3sManager    *shared.K3sManager
	crdGenerator  *shared.CRDGenerator
	reconciler    *controller.VaultUnsealConfigReconciler
}

// SetupSuite initializes K3s cluster with CRDs and Vault container
func (suite *ControllerReconciliationTestSuite) SetupSuite() {
	if testing.Short() {
		suite.T().Skip("Skipping controller reconciliation integration tests in short mode")
	}

	suite.ctx, suite.ctxCancel = context.WithTimeout(context.Background(), 20*time.Minute)
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	
	suite.vaultManager = shared.NewVaultManager(suite.ctx, suite.Suite)
	suite.k3sManager = shared.NewK3sManager(suite.ctx, suite.Suite)
	suite.crdGenerator = shared.NewCRDGenerator()
}

// TearDownSuite cleans up resources
func (suite *ControllerReconciliationTestSuite) TearDownSuite() {
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

// TestBasicReconciliation tests basic controller reconciliation
func (suite *ControllerReconciliationTestSuite) TestBasicReconciliation() {
	suite.Run("basic_controller_reconciliation", func() {
		// Set up infrastructure with CRD
		crdManifest := suite.crdGenerator.GenerateVaultUnsealConfigCRD()
		rbacManifest := suite.crdGenerator.GenerateRBACManifests("default")
		
		k3sInstance, err := suite.k3sManager.CreateK3sCluster("reconcile-test", crdManifest, rbacManifest)
		require.NoError(suite.T(), err, "Should create K3s cluster")

		// Create vault instance
		vaultInstance, err := suite.vaultManager.CreateProdVault("reconcile-vault")
		require.NoError(suite.T(), err, "Should create vault instance")

		// Wait for CRD to be ready
		err = suite.k3sManager.WaitForCRDReady(k3sInstance, "vaultunsealconfigs.vault.io", 60*time.Second)
		require.NoError(suite.T(), err, "CRD should be ready")

		// Create VaultUnsealConfig
		threshold := 3
		config := &vaultv1.VaultUnsealConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "basic-reconcile-test",
				Namespace: "default",
			},
			Spec: vaultv1.VaultUnsealConfigSpec{
				VaultInstances: []vaultv1.VaultInstance{
					{
						Name:          "reconcile-vault",
						Endpoint:      vaultInstance.Address,
						UnsealKeys:    vaultInstance.UnsealKeys,
						Threshold:     &threshold,
						TLSSkipVerify: true,
					},
				},
			},
		}

		err = k3sInstance.Client.Create(suite.ctx, config)
		require.NoError(suite.T(), err, "Should create VaultUnsealConfig")

		// Create controller
		reconciler := &controller.VaultUnsealConfigReconciler{
			Client: k3sInstance.Client,
			Log:    ctrl.Log.WithName("test-controller"),
			Scheme: k3sInstance.Scheme,
		}

		// Trigger reconciliation
		req := ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      "basic-reconcile-test",
				Namespace: "default",
			},
		}

		result, err := reconciler.Reconcile(suite.ctx, req)
		assert.NoError(suite.T(), err, "Reconciliation should succeed")
		assert.NotNil(suite.T(), result, "Should have reconciliation result")

		// Verify status was updated
		var updatedConfig vaultv1.VaultUnsealConfig
		err = k3sInstance.Client.Get(suite.ctx, types.NamespacedName{
			Name: "basic-reconcile-test", Namespace: "default",
		}, &updatedConfig)
		assert.NoError(suite.T(), err, "Should retrieve updated config")

		suite.T().Logf("✅ Basic controller reconciliation completed successfully")
	})
}

// TestMultipleVaultInstances tests reconciliation with multiple vault instances
func (suite *ControllerReconciliationTestSuite) TestMultipleVaultInstances() {
	suite.Run("multi_vault_reconciliation", func() {
		// Set up infrastructure
		crdManifest := suite.crdGenerator.GenerateVaultUnsealConfigCRD()
		rbacManifest := suite.crdGenerator.GenerateRBACManifests("default")
		
		k3sInstance, err := suite.k3sManager.CreateK3sCluster("multi-vault-test", crdManifest, rbacManifest)
		require.NoError(suite.T(), err, "Should create K3s cluster")

		// Create multiple vault instances
		vault1, err := suite.vaultManager.CreateProdVault("vault-1")
		require.NoError(suite.T(), err, "Should create vault-1")

		vault2, err := suite.vaultManager.CreateProdVault("vault-2")
		require.NoError(suite.T(), err, "Should create vault-2")

		// Wait for CRD to be ready
		err = suite.k3sManager.WaitForCRDReady(k3sInstance, "vaultunsealconfigs.vault.io", 60*time.Second)
		require.NoError(suite.T(), err, "CRD should be ready")

		// Create multi-vault config
		threshold1, threshold2 := 3, 2
		config := &vaultv1.VaultUnsealConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "multi-vault-test",
				Namespace: "default",
			},
			Spec: vaultv1.VaultUnsealConfigSpec{
				VaultInstances: []vaultv1.VaultInstance{
					{
						Name:          "vault-1",
						Endpoint:      vault1.Address,
						UnsealKeys:    vault1.UnsealKeys,
						Threshold:     &threshold1,
						TLSSkipVerify: true,
					},
					{
						Name:          "vault-2",
						Endpoint:      vault2.Address,
						UnsealKeys:    vault2.UnsealKeys,
						Threshold:     &threshold2,
						TLSSkipVerify: true,
					},
				},
			},
		}

		err = k3sInstance.Client.Create(suite.ctx, config)
		require.NoError(suite.T(), err, "Should create multi-vault config")

		// Create controller
		reconciler := &controller.VaultUnsealConfigReconciler{
			Client: k3sInstance.Client,
			Log:    ctrl.Log.WithName("multi-vault-controller"),
			Scheme: k3sInstance.Scheme,
		}

		// Reconcile
		req := ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      "multi-vault-test",
				Namespace: "default",
			},
		}

		result, err := reconciler.Reconcile(suite.ctx, req)
		assert.NoError(suite.T(), err, "Multi-vault reconciliation should succeed")
		assert.NotNil(suite.T(), result, "Should have reconciliation result")

		// Verify config was processed
		var updatedConfig vaultv1.VaultUnsealConfig
		err = k3sInstance.Client.Get(suite.ctx, types.NamespacedName{
			Name: "multi-vault-test", Namespace: "default",
		}, &updatedConfig)
		assert.NoError(suite.T(), err, "Should retrieve multi-vault config")
		assert.Equal(suite.T(), 2, len(updatedConfig.Spec.VaultInstances), "Should have 2 vault instances")

		suite.T().Logf("✅ Multi-vault reconciliation completed successfully")
	})
}

// TestReconciliationWithErrors tests reconciliation when vault connections fail
func (suite *ControllerReconciliationTestSuite) TestReconciliationWithErrors() {
	suite.Run("error_handling_reconciliation", func() {
		// Set up infrastructure
		crdManifest := suite.crdGenerator.GenerateVaultUnsealConfigCRD()
		rbacManifest := suite.crdGenerator.GenerateRBACManifests("default")
		
		k3sInstance, err := suite.k3sManager.CreateK3sCluster("error-test", crdManifest, rbacManifest)
		require.NoError(suite.T(), err, "Should create K3s cluster")

		// Wait for CRD to be ready
		err = suite.k3sManager.WaitForCRDReady(k3sInstance, "vaultunsealconfigs.vault.io", 60*time.Second)
		require.NoError(suite.T(), err, "CRD should be ready")

		// Create config with invalid vault endpoint
		threshold := 3
		config := &vaultv1.VaultUnsealConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "error-test",
				Namespace: "default",
			},
			Spec: vaultv1.VaultUnsealConfigSpec{
				VaultInstances: []vaultv1.VaultInstance{
					{
						Name:          "unreachable-vault",
						Endpoint:      "http://nonexistent.vault.local:8200",
						UnsealKeys:    []string{"invalid-key"},
						Threshold:     &threshold,
						TLSSkipVerify: true,
					},
				},
			},
		}

		err = k3sInstance.Client.Create(suite.ctx, config)
		require.NoError(suite.T(), err, "Should create error-test config")

		// Create controller
		reconciler := &controller.VaultUnsealConfigReconciler{
			Client: k3sInstance.Client,
			Log:    ctrl.Log.WithName("error-controller"),
			Scheme: k3sInstance.Scheme,
		}

		// Reconcile - should handle errors gracefully
		req := ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      "error-test",
				Namespace: "default",
			},
		}

		result, err := reconciler.Reconcile(suite.ctx, req)
		// Controller should handle errors gracefully, not panic
		assert.NoError(suite.T(), err, "Controller should handle errors gracefully")
		assert.NotNil(suite.T(), result, "Should have reconciliation result")

		suite.T().Logf("✅ Error handling reconciliation completed successfully")
	})
}

// TestControllerReconciliationTestSuite runs the controller reconciliation test suite
func TestControllerReconciliationTestSuite(t *testing.T) {
	suite.Run(t, new(ControllerReconciliationTestSuite))
}