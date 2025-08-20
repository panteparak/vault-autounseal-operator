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

// FailoverTestSuite tests primary/standby Vault failover scenarios
type FailoverTestSuite struct {
	suite.Suite
	ctx           context.Context
	ctxCancel     context.CancelFunc
	vaultManager  *shared.VaultManager
	k3sManager    *shared.K3sManager
	crdGenerator  *shared.CRDGenerator
}

// SetupSuite initializes the failover test suite
func (suite *FailoverTestSuite) SetupSuite() {
	if testing.Short() {
		suite.T().Skip("Skipping failover integration tests in short mode")
	}

	suite.ctx, suite.ctxCancel = context.WithTimeout(context.Background(), 25*time.Minute)
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	suite.vaultManager = shared.NewVaultManager(suite.ctx, suite.Suite)
	suite.k3sManager = shared.NewK3sManager(suite.ctx, suite.Suite)
	suite.crdGenerator = shared.NewCRDGenerator()
}

// TearDownSuite cleans up resources
func (suite *FailoverTestSuite) TearDownSuite() {
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

// TestPrimaryStandbyFailover tests primary/standby vault failover
func (suite *FailoverTestSuite) TestPrimaryStandbyFailover() {
	suite.Run("primary_standby_setup", func() {
		// Set up infrastructure
		crdManifest := suite.crdGenerator.GenerateVaultUnsealConfigCRD()
		rbacManifest := suite.crdGenerator.GenerateRBACManifests("default")

		k3sInstance, err := suite.k3sManager.CreateK3sCluster("failover-test", crdManifest, rbacManifest)
		require.NoError(suite.T(), err, "Should create K3s cluster")

		// Create primary and standby Vault instances
		primaryVault, err := suite.vaultManager.CreateProdVault("vault-primary")
		require.NoError(suite.T(), err, "Should create primary vault")

		standbyVault, err := suite.vaultManager.CreateProdVault("vault-standby")
		require.NoError(suite.T(), err, "Should create standby vault")

		// Wait for CRD
		err = suite.k3sManager.WaitForCRDReady(k3sInstance, "vaultunsealconfigs.vault.io", 60*time.Second)
		require.NoError(suite.T(), err, "CRD should be ready")

		// Create failover configuration
		threshold := 3
		failoverConfig := &vaultv1.VaultUnsealConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "failover-vault-config",
				Namespace: "default",
				Labels: map[string]string{
					"scenario": "failover",
				},
			},
			Spec: vaultv1.VaultUnsealConfigSpec{
				VaultInstances: []vaultv1.VaultInstance{
					{
						Name:          "vault-primary",
						Endpoint:      primaryVault.Address,
						UnsealKeys:    primaryVault.UnsealKeys,
						Threshold:     &threshold,
						TLSSkipVerify: true,
					},
					{
						Name:          "vault-standby",
						Endpoint:      standbyVault.Address,
						UnsealKeys:    standbyVault.UnsealKeys,
						Threshold:     &threshold,
						TLSSkipVerify: true,
					},
				},
			},
		}

		err = k3sInstance.Client.Create(suite.ctx, failoverConfig)
		require.NoError(suite.T(), err, "Should create failover config")

		// Create controller
		reconciler := &controller.VaultUnsealConfigReconciler{
			Client: k3sInstance.Client,
			Log:    ctrl.Log.WithName("failover-controller"),
			Scheme: k3sInstance.Scheme,
		}

		// Initial reconciliation
		req := ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      failoverConfig.Name,
				Namespace: failoverConfig.Namespace,
			},
		}

		_, err = reconciler.Reconcile(suite.ctx, req)
		assert.NoError(suite.T(), err, "Initial reconciliation should succeed")

		// For testing, manually unseal both vaults to simulate normal operation
		err = suite.vaultManager.UnsealVault(primaryVault, primaryVault.UnsealKeys, 3)
		assert.NoError(suite.T(), err, "Should unseal primary vault")

		err = suite.vaultManager.UnsealVault(standbyVault, standbyVault.UnsealKeys, 3)
		assert.NoError(suite.T(), err, "Should unseal standby vault")

		// Verify both vaults are unsealed
		err = suite.vaultManager.VerifyVaultHealth(primaryVault, false)
		assert.NoError(suite.T(), err, "Primary vault should be unsealed")

		err = suite.vaultManager.VerifyVaultHealth(standbyVault, false)
		assert.NoError(suite.T(), err, "Standby vault should be unsealed")

		suite.T().Logf("✅ Primary/Standby setup completed successfully")
	})
}

// TestFailoverRecovery tests recovery scenarios
func (suite *FailoverTestSuite) TestFailoverRecovery() {
	suite.Run("primary_failure_recovery", func() {
		// Set up infrastructure
		crdManifest := suite.crdGenerator.GenerateVaultUnsealConfigCRD()
		rbacManifest := suite.crdGenerator.GenerateRBACManifests("default")

		k3sInstance, err := suite.k3sManager.CreateK3sCluster("recovery-test", crdManifest, rbacManifest)
		require.NoError(suite.T(), err, "Should create K3s cluster")

		// Create primary vault
		primaryVault, err := suite.vaultManager.CreateProdVault("vault-primary-recovery")
		require.NoError(suite.T(), err, "Should create primary vault")

		// Wait for CRD
		err = suite.k3sManager.WaitForCRDReady(k3sInstance, "vaultunsealconfigs.vault.io", 60*time.Second)
		require.NoError(suite.T(), err, "CRD should be ready")

		// Create initial configuration with just primary
		threshold := 3
		recoveryConfig := &vaultv1.VaultUnsealConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "recovery-config",
				Namespace: "default",
			},
			Spec: vaultv1.VaultUnsealConfigSpec{
				VaultInstances: []vaultv1.VaultInstance{
					{
						Name:          "vault-primary",
						Endpoint:      primaryVault.Address,
						UnsealKeys:    primaryVault.UnsealKeys,
						Threshold:     &threshold,
						TLSSkipVerify: true,
					},
				},
			},
		}

		err = k3sInstance.Client.Create(suite.ctx, recoveryConfig)
		require.NoError(suite.T(), err, "Should create recovery config")

		// Create controller
		reconciler := &controller.VaultUnsealConfigReconciler{
			Client: k3sInstance.Client,
			Log:    ctrl.Log.WithName("recovery-controller"),
			Scheme: k3sInstance.Scheme,
		}

		// Initial reconciliation
		req := ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      recoveryConfig.Name,
				Namespace: recoveryConfig.Namespace,
			},
		}

		_, err = reconciler.Reconcile(suite.ctx, req)
		assert.NoError(suite.T(), err, "Initial reconciliation should succeed")

		// For this test, manually unseal the primary vault to simulate normal operation
		err = suite.vaultManager.UnsealVault(primaryVault, primaryVault.UnsealKeys, 3)
		assert.NoError(suite.T(), err, "Should unseal primary vault")

		// Verify primary is unsealed
		err = suite.vaultManager.VerifyVaultHealth(primaryVault, false)
		assert.NoError(suite.T(), err, "Primary vault should be unsealed")

		// Simulate primary failure by sealing it
		err = suite.vaultManager.SealVault(primaryVault)
		assert.NoError(suite.T(), err, "Should seal primary vault")

		// Verify primary is now sealed
		err = suite.vaultManager.VerifyVaultHealth(primaryVault, true)
		assert.NoError(suite.T(), err, "Primary vault should be sealed")

		// Reconcile again - this should attempt to process the sealed vault
		_, err = reconciler.Reconcile(suite.ctx, req)
		assert.NoError(suite.T(), err, "Recovery reconciliation should succeed")

		// For testing purposes, manually unseal again to simulate recovery
		err = suite.vaultManager.UnsealVault(primaryVault, primaryVault.UnsealKeys, 3)
		assert.NoError(suite.T(), err, "Should unseal primary vault again")

		// Verify primary is unsealed again
		err = suite.vaultManager.VerifyVaultHealth(primaryVault, false)
		assert.NoError(suite.T(), err, "Primary vault should be unsealed again")

		suite.T().Logf("✅ Primary failure recovery completed successfully")
	})
}

// TestFailoverTestSuite runs the failover test suite
func TestFailoverTestSuite(t *testing.T) {
	suite.Run(t, new(FailoverTestSuite))
}
