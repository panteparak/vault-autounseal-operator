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

// MultiVaultTestSuite tests multiple Vault instances coordination scenarios
type MultiVaultTestSuite struct {
	suite.Suite
	ctx           context.Context
	ctxCancel     context.CancelFunc
	vaultManager  *shared.VaultManager
	k3sManager    *shared.K3sManager
	crdGenerator  *shared.CRDGenerator
}

// SetupSuite initializes the multi-vault test suite
func (suite *MultiVaultTestSuite) SetupSuite() {
	if testing.Short() {
		suite.T().Skip("Skipping multi-vault integration tests in short mode")
	}

	suite.ctx, suite.ctxCancel = context.WithTimeout(context.Background(), 30*time.Minute)
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	suite.vaultManager = shared.NewVaultManager(suite.ctx, suite.Suite)
	suite.k3sManager = shared.NewK3sManager(suite.ctx, suite.Suite)
	suite.crdGenerator = shared.NewCRDGenerator()
}

// TearDownSuite cleans up resources
func (suite *MultiVaultTestSuite) TearDownSuite() {
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

// TestMultiVaultCoordination tests coordination of multiple vault instances
func (suite *MultiVaultTestSuite) TestMultiVaultCoordination() {
	suite.Run("three_vault_departments", func() {
		// Set up infrastructure
		crdManifest := suite.crdGenerator.GenerateVaultUnsealConfigCRD()
		rbacManifest := suite.crdGenerator.GenerateRBACManifests("default")

		k3sInstance, err := suite.k3sManager.CreateK3sCluster("multi-vault", crdManifest, rbacManifest)
		require.NoError(suite.T(), err, "Should create K3s cluster")

		// Create three Vault instances representing different departments
		financeVault, err := suite.vaultManager.CreateProdVault("vault-finance")
		require.NoError(suite.T(), err, "Should create finance vault")

		engineeringVault, err := suite.vaultManager.CreateProdVault("vault-engineering")
		require.NoError(suite.T(), err, "Should create engineering vault")

		operationsVault, err := suite.vaultManager.CreateProdVault("vault-operations")
		require.NoError(suite.T(), err, "Should create operations vault")

		// Wait for CRD
		err = suite.k3sManager.WaitForCRDReady(k3sInstance, "vaultunsealconfigs.vault.io", 60*time.Second)
		require.NoError(suite.T(), err, "CRD should be ready")

		// Create multi-vault configuration
		threshold := 3
		multiVaultConfig := &vaultv1.VaultUnsealConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "multi-department-vaults",
				Namespace: "default",
				Labels: map[string]string{
					"scenario": "multi-vault",
					"environment": "enterprise",
				},
			},
			Spec: vaultv1.VaultUnsealConfigSpec{
				VaultInstances: []vaultv1.VaultInstance{
					{
						Name:          "vault-finance",
						Endpoint:      financeVault.Address,
						UnsealKeys:    financeVault.UnsealKeys,
						Threshold:     &threshold,
						TLSSkipVerify: true,
					},
					{
						Name:          "vault-engineering",
						Endpoint:      engineeringVault.Address,
						UnsealKeys:    engineeringVault.UnsealKeys,
						Threshold:     &threshold,
						TLSSkipVerify: true,
					},
					{
						Name:          "vault-operations",
						Endpoint:      operationsVault.Address,
						UnsealKeys:    operationsVault.UnsealKeys,
						Threshold:     &threshold,
						TLSSkipVerify: true,
					},
				},
			},
		}

		err = k3sInstance.Client.Create(suite.ctx, multiVaultConfig)
		require.NoError(suite.T(), err, "Should create multi-vault config")

		// Create controller
		reconciler := &controller.VaultUnsealConfigReconciler{
			Client: k3sInstance.Client,
			Log:    ctrl.Log.WithName("multi-vault-controller"),
			Scheme: k3sInstance.Scheme,
		}

		// Reconcile the multi-vault configuration
		req := ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      multiVaultConfig.Name,
				Namespace: multiVaultConfig.Namespace,
			},
		}

		_, err = reconciler.Reconcile(suite.ctx, req)
		assert.NoError(suite.T(), err, "Multi-vault reconciliation should succeed")

		// Verify all three vaults are unsealed
		err = suite.vaultManager.VerifyVaultHealth(financeVault, false)
		assert.NoError(suite.T(), err, "Finance vault should be unsealed")

		err = suite.vaultManager.VerifyVaultHealth(engineeringVault, false)
		assert.NoError(suite.T(), err, "Engineering vault should be unsealed")

		err = suite.vaultManager.VerifyVaultHealth(operationsVault, false)
		assert.NoError(suite.T(), err, "Operations vault should be unsealed")

		suite.T().Logf("✅ All three department vaults unsealed successfully")
	})
}

// TestSelectiveVaultOperations tests operating on specific vaults in a multi-vault setup
func (suite *MultiVaultTestSuite) TestSelectiveVaultOperations() {
	suite.Run("selective_vault_management", func() {
		// Set up infrastructure
		crdManifest := suite.crdGenerator.GenerateVaultUnsealConfigCRD()
		rbacManifest := suite.crdGenerator.GenerateRBACManifests("default")

		k3sInstance, err := suite.k3sManager.CreateK3sCluster("selective-vault", crdManifest, rbacManifest)
		require.NoError(suite.T(), err, "Should create K3s cluster")

		// Create multiple vault instances
		devVault, err := suite.vaultManager.CreateProdVault("vault-dev")
		require.NoError(suite.T(), err, "Should create dev vault")

		stagingVault, err := suite.vaultManager.CreateProdVault("vault-staging")
		require.NoError(suite.T(), err, "Should create staging vault")

		prodVault, err := suite.vaultManager.CreateProdVault("vault-prod")
		require.NoError(suite.T(), err, "Should create prod vault")

		// Wait for CRD
		err = suite.k3sManager.WaitForCRDReady(k3sInstance, "vaultunsealconfigs.vault.io", 60*time.Second)
		require.NoError(suite.T(), err, "CRD should be ready")

		// Create separate configurations for different environments
		threshold := 3

		// Development environment config
		devConfig := &vaultv1.VaultUnsealConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "dev-vault-config",
				Namespace: "default",
				Labels: map[string]string{
					"environment": "development",
				},
			},
			Spec: vaultv1.VaultUnsealConfigSpec{
				VaultInstances: []vaultv1.VaultInstance{
					{
						Name:          "vault-dev",
						Endpoint:      devVault.Address,
						UnsealKeys:    devVault.UnsealKeys,
						Threshold:     &threshold,
						TLSSkipVerify: true,
					},
				},
			},
		}

		// Production environment config
		prodConfig := &vaultv1.VaultUnsealConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "prod-vault-config",
				Namespace: "default",
				Labels: map[string]string{
					"environment": "production",
				},
			},
			Spec: vaultv1.VaultUnsealConfigSpec{
				VaultInstances: []vaultv1.VaultInstance{
					{
						Name:          "vault-staging",
						Endpoint:      stagingVault.Address,
						UnsealKeys:    stagingVault.UnsealKeys,
						Threshold:     &threshold,
						TLSSkipVerify: true,
					},
					{
						Name:          "vault-prod",
						Endpoint:      prodVault.Address,
						UnsealKeys:    prodVault.UnsealKeys,
						Threshold:     &threshold,
						TLSSkipVerify: true,
					},
				},
			},
		}

		// Create both configurations
		err = k3sInstance.Client.Create(suite.ctx, devConfig)
		require.NoError(suite.T(), err, "Should create dev config")

		err = k3sInstance.Client.Create(suite.ctx, prodConfig)
		require.NoError(suite.T(), err, "Should create prod config")

		// Create controller
		reconciler := &controller.VaultUnsealConfigReconciler{
			Client: k3sInstance.Client,
			Log:    ctrl.Log.WithName("selective-controller"),
			Scheme: k3sInstance.Scheme,
		}

		// Reconcile dev environment
		devReq := ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      devConfig.Name,
				Namespace: devConfig.Namespace,
			},
		}

		_, err = reconciler.Reconcile(suite.ctx, devReq)
		assert.NoError(suite.T(), err, "Dev reconciliation should succeed")

		// Reconcile prod environment
		prodReq := ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      prodConfig.Name,
				Namespace: prodConfig.Namespace,
			},
		}

		_, err = reconciler.Reconcile(suite.ctx, prodReq)
		assert.NoError(suite.T(), err, "Prod reconciliation should succeed")

		// Verify all vaults are unsealed
		err = suite.vaultManager.VerifyVaultHealth(devVault, false)
		assert.NoError(suite.T(), err, "Dev vault should be unsealed")

		err = suite.vaultManager.VerifyVaultHealth(stagingVault, false)
		assert.NoError(suite.T(), err, "Staging vault should be unsealed")

		err = suite.vaultManager.VerifyVaultHealth(prodVault, false)
		assert.NoError(suite.T(), err, "Prod vault should be unsealed")

		suite.T().Logf("✅ Selective vault management completed successfully")
	})
}

// TestMultiVaultErrorHandling tests error handling in multi-vault scenarios
func (suite *MultiVaultTestSuite) TestMultiVaultErrorHandling() {
	suite.Run("partial_failure_handling", func() {
		// Set up infrastructure
		crdManifest := suite.crdGenerator.GenerateVaultUnsealConfigCRD()
		rbacManifest := suite.crdGenerator.GenerateRBACManifests("default")

		k3sInstance, err := suite.k3sManager.CreateK3sCluster("error-handling", crdManifest, rbacManifest)
		require.NoError(suite.T(), err, "Should create K3s cluster")

		// Create one working vault and one with invalid configuration
		workingVault, err := suite.vaultManager.CreateProdVault("vault-working")
		require.NoError(suite.T(), err, "Should create working vault")

		// Wait for CRD
		err = suite.k3sManager.WaitForCRDReady(k3sInstance, "vaultunsealconfigs.vault.io", 60*time.Second)
		require.NoError(suite.T(), err, "CRD should be ready")

		// Create configuration with one valid and one invalid vault
		threshold := 3
		mixedConfig := &vaultv1.VaultUnsealConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "mixed-vault-config",
				Namespace: "default",
			},
			Spec: vaultv1.VaultUnsealConfigSpec{
				VaultInstances: []vaultv1.VaultInstance{
					{
						Name:          "vault-working",
						Endpoint:      workingVault.Address,
						UnsealKeys:    workingVault.UnsealKeys,
						Threshold:     &threshold,
						TLSSkipVerify: true,
					},
					{
						Name:          "vault-invalid",
						Endpoint:      "http://nonexistent.vault:8200",
						UnsealKeys:    []string{"invalid-key"},
						Threshold:     &threshold,
						TLSSkipVerify: true,
					},
				},
			},
		}

		err = k3sInstance.Client.Create(suite.ctx, mixedConfig)
		require.NoError(suite.T(), err, "Should create mixed config")

		// Create controller
		reconciler := &controller.VaultUnsealConfigReconciler{
			Client: k3sInstance.Client,
			Log:    ctrl.Log.WithName("error-handling-controller"),
			Scheme: k3sInstance.Scheme,
		}

		// Reconcile - should handle partial failures gracefully
		req := ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      mixedConfig.Name,
				Namespace: mixedConfig.Namespace,
			},
		}

		_, err = reconciler.Reconcile(suite.ctx, req)
		// Controller should handle partial failures gracefully
		assert.NoError(suite.T(), err, "Should handle partial failures gracefully")

		// Verify working vault is unsealed despite one failure
		err = suite.vaultManager.VerifyVaultHealth(workingVault, false)
		assert.NoError(suite.T(), err, "Working vault should still be unsealed")

		suite.T().Logf("✅ Partial failure handling completed successfully")
	})
}

// TestMultiVaultTestSuite runs the multi-vault test suite
func TestMultiVaultTestSuite(t *testing.T) {
	suite.Run(t, new(MultiVaultTestSuite))
}
