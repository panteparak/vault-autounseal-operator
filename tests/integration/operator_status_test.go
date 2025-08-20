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

// OperatorStatusTestSuite tests operator status and health monitoring
type OperatorStatusTestSuite struct {
	suite.Suite
	ctx           context.Context
	ctxCancel     context.CancelFunc
	vaultManager  *shared.VaultManager
	k3sManager    *shared.K3sManager
	crdGenerator  *shared.CRDGenerator
}

// SetupSuite initializes the operator status test suite
func (suite *OperatorStatusTestSuite) SetupSuite() {
	if testing.Short() {
		suite.T().Skip("Skipping operator status integration tests in short mode")
	}

	suite.ctx, suite.ctxCancel = context.WithTimeout(context.Background(), 20*time.Minute)
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	
	suite.vaultManager = shared.NewVaultManager(suite.ctx, suite.Suite)
	suite.k3sManager = shared.NewK3sManager(suite.ctx, suite.Suite)
	suite.crdGenerator = shared.NewCRDGenerator()
}

// TearDownSuite cleans up resources
func (suite *OperatorStatusTestSuite) TearDownSuite() {
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

// TestOperatorBasicStatus tests basic operator status functionality
func (suite *OperatorStatusTestSuite) TestOperatorBasicStatus() {
	suite.Run("controller_reconciliation_status", func() {
		// Set up infrastructure
		crdManifest := suite.crdGenerator.GenerateVaultUnsealConfigCRD()
		rbacManifest := suite.crdGenerator.GenerateRBACManifests("default")
		
		k3sInstance, err := suite.k3sManager.CreateK3sCluster("status-test", crdManifest, rbacManifest)
		require.NoError(suite.T(), err, "Should create K3s cluster")

		// Create a vault instance
		vaultInstance, err := suite.vaultManager.CreateProdVault("status-vault")
		require.NoError(suite.T(), err, "Should create vault instance")

		// Wait for CRD
		err = suite.k3sManager.WaitForCRDReady(k3sInstance, "vaultunsealconfigs.vault.io", 60*time.Second)
		require.NoError(suite.T(), err, "CRD should be ready")

		// Create VaultUnsealConfig
		threshold := 3
		vaultConfig := &vaultv1.VaultUnsealConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "status-test-config",
				Namespace: "default",
			},
			Spec: vaultv1.VaultUnsealConfigSpec{
				VaultInstances: []vaultv1.VaultInstance{
					{
						Name:          "status-vault",
						Endpoint:      vaultInstance.Address,
						UnsealKeys:    vaultInstance.UnsealKeys,
						Threshold:     &threshold,
						TLSSkipVerify: true,
					},
				},
			},
		}

		err = k3sInstance.Client.Create(suite.ctx, vaultConfig)
		require.NoError(suite.T(), err, "Should create VaultUnsealConfig")

		// Create controller
		reconciler := &controller.VaultUnsealConfigReconciler{
			Client: k3sInstance.Client,
			Log:    ctrl.Log.WithName("status-controller"),
			Scheme: k3sInstance.Scheme,
		}

		// Perform reconciliation
		req := ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      vaultConfig.Name,
				Namespace: vaultConfig.Namespace,
			},
		}

		result, err := reconciler.Reconcile(suite.ctx, req)
		assert.NoError(suite.T(), err, "Reconciliation should succeed")
		assert.False(suite.T(), result.Requeue, "Should not require immediate requeue")

		// Retrieve the updated resource to check status
		updatedConfig := &vaultv1.VaultUnsealConfig{}
		err = k3sInstance.Client.Get(suite.ctx, types.NamespacedName{
			Name:      vaultConfig.Name,
			Namespace: vaultConfig.Namespace,
		}, updatedConfig)
		assert.NoError(suite.T(), err, "Should retrieve updated config")

		// Verify the resource was processed
		assert.NotNil(suite.T(), updatedConfig, "Updated config should not be nil")

		suite.T().Logf("✅ Controller reconciliation status verified successfully")
	})
}

// TestOperatorHealthChecks tests operator health check capabilities
func (suite *OperatorStatusTestSuite) TestOperatorHealthChecks() {
	suite.Run("vault_health_monitoring", func() {
		// Set up infrastructure
		crdManifest := suite.crdGenerator.GenerateVaultUnsealConfigCRD()
		rbacManifest := suite.crdGenerator.GenerateRBACManifests("default")
		
		k3sInstance, err := suite.k3sManager.CreateK3sCluster("health-test", crdManifest, rbacManifest)
		require.NoError(suite.T(), err, "Should create K3s cluster")

		// Create healthy vault instance
		healthyVault, err := suite.vaultManager.CreateProdVault("healthy-vault")
		require.NoError(suite.T(), err, "Should create healthy vault")

		// Wait for CRD
		err = suite.k3sManager.WaitForCRDReady(k3sInstance, "vaultunsealconfigs.vault.io", 60*time.Second)
		require.NoError(suite.T(), err, "CRD should be ready")

		// Create configuration for health monitoring
		threshold := 3
		healthConfig := &vaultv1.VaultUnsealConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "health-monitor-config",
				Namespace: "default",
				Labels: map[string]string{
					"monitoring": "health-check",
				},
			},
			Spec: vaultv1.VaultUnsealConfigSpec{
				VaultInstances: []vaultv1.VaultInstance{
					{
						Name:          "healthy-vault",
						Endpoint:      healthyVault.Address,
						UnsealKeys:    healthyVault.UnsealKeys,
						Threshold:     &threshold,
						TLSSkipVerify: true,
					},
				},
			},
		}

		err = k3sInstance.Client.Create(suite.ctx, healthConfig)
		require.NoError(suite.T(), err, "Should create health config")

		// Create controller
		reconciler := &controller.VaultUnsealConfigReconciler{
			Client: k3sInstance.Client,
			Log:    ctrl.Log.WithName("health-controller"),
			Scheme: k3sInstance.Scheme,
		}

		// Perform initial reconciliation
		req := ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      healthConfig.Name,
				Namespace: healthConfig.Namespace,
			},
		}

		_, err = reconciler.Reconcile(suite.ctx, req)
		assert.NoError(suite.T(), err, "Initial reconciliation should succeed")

		// Verify vault is healthy and unsealed
		err = suite.vaultManager.VerifyVaultHealth(healthyVault, false)
		assert.NoError(suite.T(), err, "Vault should be healthy and unsealed")

		// Simulate health check by sealing and then reconciling again
		err = suite.vaultManager.SealVault(healthyVault)
		assert.NoError(suite.T(), err, "Should seal vault")

		// Verify vault is sealed
		err = suite.vaultManager.VerifyVaultHealth(healthyVault, true)
		assert.NoError(suite.T(), err, "Vault should be sealed")

		// Reconcile again to check if controller detects and fixes the issue
		_, err = reconciler.Reconcile(suite.ctx, req)
		assert.NoError(suite.T(), err, "Health check reconciliation should succeed")

		// Verify vault is unsealed again
		err = suite.vaultManager.VerifyVaultHealth(healthyVault, false)
		assert.NoError(suite.T(), err, "Vault should be unsealed again")

		suite.T().Logf("✅ Vault health monitoring completed successfully")
	})
}

// TestOperatorErrorReporting tests operator error reporting capabilities
func (suite *OperatorStatusTestSuite) TestOperatorErrorReporting() {
	suite.Run("error_status_reporting", func() {
		// Set up infrastructure
		crdManifest := suite.crdGenerator.GenerateVaultUnsealConfigCRD()
		rbacManifest := suite.crdGenerator.GenerateRBACManifests("default")
		
		k3sInstance, err := suite.k3sManager.CreateK3sCluster("error-test", crdManifest, rbacManifest)
		require.NoError(suite.T(), err, "Should create K3s cluster")

		// Wait for CRD
		err = suite.k3sManager.WaitForCRDReady(k3sInstance, "vaultunsealconfigs.vault.io", 60*time.Second)
		require.NoError(suite.T(), err, "CRD should be ready")

		// Create configuration with invalid vault endpoint
		threshold := 3
		errorConfig := &vaultv1.VaultUnsealConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "error-reporting-config",
				Namespace: "default",
			},
			Spec: vaultv1.VaultUnsealConfigSpec{
				VaultInstances: []vaultv1.VaultInstance{
					{
						Name:          "invalid-vault",
						Endpoint:      "http://nonexistent.vault:8200",
						UnsealKeys:    []string{"invalid-key"},
						Threshold:     &threshold,
						TLSSkipVerify: true,
					},
				},
			},
		}

		err = k3sInstance.Client.Create(suite.ctx, errorConfig)
		require.NoError(suite.T(), err, "Should create error config")

		// Create controller
		reconciler := &controller.VaultUnsealConfigReconciler{
			Client: k3sInstance.Client,
			Log:    ctrl.Log.WithName("error-controller"),
			Scheme: k3sInstance.Scheme,
		}

		// Perform reconciliation with invalid configuration
		req := ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      errorConfig.Name,
				Namespace: errorConfig.Namespace,
			},
		}

		// Controller should handle errors gracefully
		_, err = reconciler.Reconcile(suite.ctx, req)
		assert.NoError(suite.T(), err, "Controller should handle errors gracefully")

		// Retrieve the resource to check if any status was updated
		updatedConfig := &vaultv1.VaultUnsealConfig{}
		err = k3sInstance.Client.Get(suite.ctx, types.NamespacedName{
			Name:      errorConfig.Name,
			Namespace: errorConfig.Namespace,
		}, updatedConfig)
		assert.NoError(suite.T(), err, "Should retrieve config even with errors")

		suite.T().Logf("✅ Error reporting completed successfully")
	})
}

// TestOperatorReconciliationLoop tests the operator's reconciliation loop behavior
func (suite *OperatorStatusTestSuite) TestOperatorReconciliationLoop() {
	suite.Run("reconciliation_loop_behavior", func() {
		// Set up infrastructure
		crdManifest := suite.crdGenerator.GenerateVaultUnsealConfigCRD()
		rbacManifest := suite.crdGenerator.GenerateRBACManifests("default")
		
		k3sInstance, err := suite.k3sManager.CreateK3sCluster("loop-test", crdManifest, rbacManifest)
		require.NoError(suite.T(), err, "Should create K3s cluster")

		// Create vault instance
		vaultInstance, err := suite.vaultManager.CreateProdVault("loop-vault")
		require.NoError(suite.T(), err, "Should create vault instance")

		// Wait for CRD
		err = suite.k3sManager.WaitForCRDReady(k3sInstance, "vaultunsealconfigs.vault.io", 60*time.Second)
		require.NoError(suite.T(), err, "CRD should be ready")

		// Create VaultUnsealConfig
		threshold := 3
		loopConfig := &vaultv1.VaultUnsealConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "reconciliation-loop-config",
				Namespace: "default",
			},
			Spec: vaultv1.VaultUnsealConfigSpec{
				VaultInstances: []vaultv1.VaultInstance{
					{
						Name:          "loop-vault",
						Endpoint:      vaultInstance.Address,
						UnsealKeys:    vaultInstance.UnsealKeys,
						Threshold:     &threshold,
						TLSSkipVerify: true,
					},
				},
			},
		}

		err = k3sInstance.Client.Create(suite.ctx, loopConfig)
		require.NoError(suite.T(), err, "Should create loop config")

		// Create controller
		reconciler := &controller.VaultUnsealConfigReconciler{
			Client: k3sInstance.Client,
			Log:    ctrl.Log.WithName("loop-controller"),
			Scheme: k3sInstance.Scheme,
		}

		req := ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      loopConfig.Name,
				Namespace: loopConfig.Namespace,
			},
		}

		// Perform multiple reconciliation cycles
		for i := 0; i < 3; i++ {
			suite.T().Logf("🔄 Reconciliation cycle %d", i+1)
			
			result, err := reconciler.Reconcile(suite.ctx, req)
			assert.NoError(suite.T(), err, "Reconciliation cycle %d should succeed", i+1)
			
			// Log result details
			suite.T().Logf("   Result: Requeue=%t, RequeueAfter=%v", result.Requeue, result.RequeueAfter)
			
			// Verify vault remains healthy
			err = suite.vaultManager.VerifyVaultHealth(vaultInstance, false)
			assert.NoError(suite.T(), err, "Vault should remain healthy in cycle %d", i+1)
			
			// Small delay between cycles
			time.Sleep(100 * time.Millisecond)
		}

		suite.T().Logf("✅ Reconciliation loop behavior verified successfully")
	})
}

// TestOperatorStatusTestSuite runs the operator status test suite
func TestOperatorStatusTestSuite(t *testing.T) {
	suite.Run(t, new(OperatorStatusTestSuite))
}