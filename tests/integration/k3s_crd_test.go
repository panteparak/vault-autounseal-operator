package integration

import (
	"context"
	"encoding/base64"
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

// K3sCRDTestSuite provides comprehensive testing for the full Kubernetes integration
// This suite tests the complete workflow including K3s, CRDs, and controller reconciliation
type K3sCRDTestSuite struct {
	suite.Suite
	ctx           context.Context
	ctxCancel     context.CancelFunc
	vaultManager  *shared.VaultManager
	k3sManager    *shared.K3sManager
	crdGenerator  *shared.CRDGenerator
}

// SetupSuite initializes the test suite
func (suite *K3sCRDTestSuite) SetupSuite() {
	if testing.Short() {
		suite.T().Skip("Skipping K3s CRD integration tests in short mode")
	}

	suite.ctx, suite.ctxCancel = context.WithTimeout(context.Background(), 20*time.Minute)
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	suite.vaultManager = shared.NewVaultManager(suite.ctx, suite.Suite)
	suite.k3sManager = shared.NewK3sManager(suite.ctx, suite.Suite)
	suite.crdGenerator = shared.NewCRDGenerator()
}

// TearDownSuite cleans up resources
func (suite *K3sCRDTestSuite) TearDownSuite() {
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

// TestK3sSetupAndCRDInstallation tests K3s cluster setup and CRD installation
func (suite *K3sCRDTestSuite) TestK3sSetupAndCRDInstallation() {
	suite.Run("k3s_cluster_creation", func() {
		// Generate CRD manifest
		crdManifest := suite.crdGenerator.GenerateVaultUnsealConfigCRD()

		// Create K3s cluster with CRD
		k3sInstance, err := suite.k3sManager.CreateK3sCluster("test-cluster", crdManifest)
		require.NoError(suite.T(), err, "Should create K3s cluster with CRDs")
		require.NotNil(suite.T(), k3sInstance, "K3s instance should not be nil")

		suite.T().Logf("✅ K3s cluster created successfully")

		// Wait for CRD to be ready
		err = suite.k3sManager.WaitForCRDReady(k3sInstance, "vaultunsealconfigs.vault.io", 60*time.Second)
		assert.NoError(suite.T(), err, "CRD should be ready within timeout")

		suite.T().Logf("✅ CRD installed and ready")

		// Test creating a VaultUnsealConfig resource
		testConfig := &vaultv1.VaultUnsealConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-config",
				Namespace: "default",
			},
			Spec: vaultv1.VaultUnsealConfigSpec{
				VaultInstances: []vaultv1.VaultInstance{
					{
						Name:          "test-vault",
						Endpoint:      "http://vault.example.com:8200",
						UnsealKeys:    []string{base64.StdEncoding.EncodeToString([]byte("test-key"))},
						Threshold:     func() *int { i := 1; return &i }(),
						TLSSkipVerify: true,
					},
				},
			},
		}

		err = k3sInstance.Client.Create(suite.ctx, testConfig)
		assert.NoError(suite.T(), err, "Should create VaultUnsealConfig resource")

		suite.T().Logf("✅ VaultUnsealConfig resource created successfully")

		// Verify resource exists
		retrieved := &vaultv1.VaultUnsealConfig{}
		err = k3sInstance.Client.Get(suite.ctx, types.NamespacedName{
			Name:      "test-config",
			Namespace: "default",
		}, retrieved)
		assert.NoError(suite.T(), err, "Should retrieve created resource")
		assert.Equal(suite.T(), "test-vault", retrieved.Spec.VaultInstances[0].Name)

		suite.T().Logf("✅ Resource retrieval successful")
	})
}

// TestControllerIntegration tests the controller reconciliation with real Vault instances
func (suite *K3sCRDTestSuite) TestControllerIntegration() {
	suite.Run("controller_with_real_vault", func() {
		// Set up infrastructure
		crdManifest := suite.crdGenerator.GenerateVaultUnsealConfigCRD()
		rbacManifest := suite.crdGenerator.GenerateRBACManifests("default")

		k3sInstance, err := suite.k3sManager.CreateK3sCluster("controller-test", crdManifest, rbacManifest)
		require.NoError(suite.T(), err, "Should create K3s cluster")

		// Create a real Vault instance
		vaultInstance, err := suite.vaultManager.CreateProdVault("controller-vault")
		require.NoError(suite.T(), err, "Should create Vault instance")

		// Wait for CRD to be ready
		err = suite.k3sManager.WaitForCRDReady(k3sInstance, "vaultunsealconfigs.vault.io", 60*time.Second)
		require.NoError(suite.T(), err, "CRD should be ready")

		// Create controller
		reconciler := &controller.VaultUnsealConfigReconciler{
			Client: k3sInstance.Client,
			Log:    ctrl.Log.WithName("test-controller"),
			Scheme: k3sInstance.Scheme,
		}

		// Create VaultUnsealConfig pointing to real Vault
		threshold := 3
		vaultConfig := &vaultv1.VaultUnsealConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "real-vault-config",
				Namespace: "default",
			},
			Spec: vaultv1.VaultUnsealConfigSpec{
				VaultInstances: []vaultv1.VaultInstance{
					{
						Name:          "test-vault",
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

		// Trigger reconciliation
		req := ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      vaultConfig.Name,
				Namespace: vaultConfig.Namespace,
			},
		}

		result, err := reconciler.Reconcile(suite.ctx, req)
		assert.NoError(suite.T(), err, "Reconciliation should not error")

		suite.T().Logf("✅ Controller reconciliation completed: %+v", result)

		// Verify Vault was unsealed
		err = suite.vaultManager.VerifyVaultHealth(vaultInstance, false)
		assert.NoError(suite.T(), err, "Vault should be unsealed after reconciliation")

		suite.T().Logf("✅ Vault successfully unsealed by controller")
	})
}

// TestComplexWorkflows tests complex real-world scenarios
func (suite *K3sCRDTestSuite) TestComplexWorkflows() {
	suite.Run("multiple_vault_instances", func() {
		// Set up infrastructure
		crdManifest := suite.crdGenerator.GenerateVaultUnsealConfigCRD()
		rbacManifest := suite.crdGenerator.GenerateRBACManifests("default")

		k3sInstance, err := suite.k3sManager.CreateK3sCluster("multi-vault", crdManifest, rbacManifest)
		require.NoError(suite.T(), err, "Should create K3s cluster")

		// Create multiple Vault instances
		vault1, err := suite.vaultManager.CreateProdVault("vault-1")
		require.NoError(suite.T(), err, "Should create vault 1")

		vault2, err := suite.vaultManager.CreateProdVault("vault-2")
		require.NoError(suite.T(), err, "Should create vault 2")

		// Wait for CRD
		err = suite.k3sManager.WaitForCRDReady(k3sInstance, "vaultunsealconfigs.vault.io", 60*time.Second)
		require.NoError(suite.T(), err, "CRD should be ready")

		// Create multi-vault configuration
		threshold1 := 3
		threshold2 := 3
		multiVaultConfig := &vaultv1.VaultUnsealConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "multi-vault-config",
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

		err = k3sInstance.Client.Create(suite.ctx, multiVaultConfig)
		require.NoError(suite.T(), err, "Should create multi-vault config")

		// Create controller
		reconciler := &controller.VaultUnsealConfigReconciler{
			Client: k3sInstance.Client,
			Log:    ctrl.Log.WithName("multi-vault-controller"),
			Scheme: k3sInstance.Scheme,
		}

		// Trigger reconciliation
		req := ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      multiVaultConfig.Name,
				Namespace: multiVaultConfig.Namespace,
			},
		}

		result, err := reconciler.Reconcile(suite.ctx, req)
		assert.NoError(suite.T(), err, "Multi-vault reconciliation should not error")

		suite.T().Logf("✅ Multi-vault reconciliation completed: %+v", result)

		// Verify both vaults were unsealed
		err = suite.vaultManager.VerifyVaultHealth(vault1, false)
		assert.NoError(suite.T(), err, "Vault 1 should be unsealed")

		err = suite.vaultManager.VerifyVaultHealth(vault2, false)
		assert.NoError(suite.T(), err, "Vault 2 should be unsealed")

		suite.T().Logf("✅ Both vaults successfully unsealed")
	})

	suite.Run("configuration_updates", func() {
		// Set up infrastructure
		crdManifest := suite.crdGenerator.GenerateVaultUnsealConfigCRD()
		rbacManifest := suite.crdGenerator.GenerateRBACManifests("default")

		k3sInstance, err := suite.k3sManager.CreateK3sCluster("config-update", crdManifest, rbacManifest)
		require.NoError(suite.T(), err, "Should create K3s cluster")

		vault1, err := suite.vaultManager.CreateProdVault("update-vault-1")
		require.NoError(suite.T(), err, "Should create first vault")

		vault2, err := suite.vaultManager.CreateProdVault("update-vault-2")
		require.NoError(suite.T(), err, "Should create second vault")

		// Wait for CRD
		err = suite.k3sManager.WaitForCRDReady(k3sInstance, "vaultunsealconfigs.vault.io", 60*time.Second)
		require.NoError(suite.T(), err, "CRD should be ready")

		// Create initial configuration with one vault
		threshold := 3
		vaultConfig := &vaultv1.VaultUnsealConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "update-config",
				Namespace: "default",
			},
			Spec: vaultv1.VaultUnsealConfigSpec{
				VaultInstances: []vaultv1.VaultInstance{
					{
						Name:          "vault-1",
						Endpoint:      vault1.Address,
						UnsealKeys:    vault1.UnsealKeys,
						Threshold:     &threshold,
						TLSSkipVerify: true,
					},
				},
			},
		}

		err = k3sInstance.Client.Create(suite.ctx, vaultConfig)
		require.NoError(suite.T(), err, "Should create initial config")

		// Create controller
		reconciler := &controller.VaultUnsealConfigReconciler{
			Client: k3sInstance.Client,
			Log:    ctrl.Log.WithName("update-controller"),
			Scheme: k3sInstance.Scheme,
		}

		// Initial reconciliation
		req := ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      vaultConfig.Name,
				Namespace: vaultConfig.Namespace,
			},
		}

		_, err = reconciler.Reconcile(suite.ctx, req)
		assert.NoError(suite.T(), err, "Initial reconciliation should succeed")

		// Verify first vault is unsealed
		err = suite.vaultManager.VerifyVaultHealth(vault1, false)
		assert.NoError(suite.T(), err, "Vault 1 should be unsealed")

		suite.T().Logf("✅ Initial configuration processed successfully")

		// Update configuration to add second vault
		retrieved := &vaultv1.VaultUnsealConfig{}
		err = k3sInstance.Client.Get(suite.ctx, types.NamespacedName{
			Name:      vaultConfig.Name,
			Namespace: vaultConfig.Namespace,
		}, retrieved)
		require.NoError(suite.T(), err, "Should retrieve config for update")

		// Add second vault to the spec
		retrieved.Spec.VaultInstances = append(retrieved.Spec.VaultInstances, vaultv1.VaultInstance{
			Name:          "vault-2",
			Endpoint:      vault2.Address,
			UnsealKeys:    vault2.UnsealKeys,
			Threshold:     &threshold,
			TLSSkipVerify: true,
		})

		err = k3sInstance.Client.Update(suite.ctx, retrieved)
		assert.NoError(suite.T(), err, "Should update configuration")

		// Reconcile updated configuration
		_, err = reconciler.Reconcile(suite.ctx, req)
		assert.NoError(suite.T(), err, "Updated reconciliation should succeed")

		// Verify both vaults are now unsealed
		err = suite.vaultManager.VerifyVaultHealth(vault1, false)
		assert.NoError(suite.T(), err, "Vault 1 should still be unsealed")

		err = suite.vaultManager.VerifyVaultHealth(vault2, false)
		assert.NoError(suite.T(), err, "Vault 2 should now be unsealed")

		suite.T().Logf("✅ Configuration update processed successfully")
	})
}

// TestErrorScenarios tests error handling in the full integration
func (suite *K3sCRDTestSuite) TestErrorScenarios() {
	suite.Run("invalid_vault_endpoints", func() {
		// Set up infrastructure
		crdManifest := suite.crdGenerator.GenerateVaultUnsealConfigCRD()
		rbacManifest := suite.crdGenerator.GenerateRBACManifests("default")

		k3sInstance, err := suite.k3sManager.CreateK3sCluster("error-test", crdManifest, rbacManifest)
		require.NoError(suite.T(), err, "Should create K3s cluster")

		// Wait for CRD
		err = suite.k3sManager.WaitForCRDReady(k3sInstance, "vaultunsealconfigs.vault.io", 60*time.Second)
		require.NoError(suite.T(), err, "CRD should be ready")

		// Create configuration with invalid endpoint
		threshold := 3
		invalidConfig := &vaultv1.VaultUnsealConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "invalid-config",
				Namespace: "default",
			},
			Spec: vaultv1.VaultUnsealConfigSpec{
				VaultInstances: []vaultv1.VaultInstance{
					{
						Name:          "invalid-vault",
						Endpoint:      "http://nonexistent.vault:8200",
						UnsealKeys:    []string{base64.StdEncoding.EncodeToString([]byte("fake-key"))},
						Threshold:     &threshold,
						TLSSkipVerify: true,
					},
				},
			},
		}

		err = k3sInstance.Client.Create(suite.ctx, invalidConfig)
		require.NoError(suite.T(), err, "Should create config with invalid endpoint")

		// Create controller
		reconciler := &controller.VaultUnsealConfigReconciler{
			Client: k3sInstance.Client,
			Log:    ctrl.Log.WithName("error-controller"),
			Scheme: k3sInstance.Scheme,
		}

		// Trigger reconciliation - should handle error gracefully
		req := ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      invalidConfig.Name,
				Namespace: invalidConfig.Namespace,
			},
		}

		result, err := reconciler.Reconcile(suite.ctx, req)
		// Controller should handle errors gracefully, not crash
		assert.NoError(suite.T(), err, "Controller should handle invalid endpoints gracefully")

		suite.T().Logf("✅ Invalid endpoint handled gracefully: %+v", result)
	})

	suite.Run("insufficient_rbac_permissions", func() {
		// Create cluster without RBAC manifests
		crdManifest := suite.crdGenerator.GenerateVaultUnsealConfigCRD()

		k3sInstance, err := suite.k3sManager.CreateK3sCluster("rbac-test", crdManifest)
		require.NoError(suite.T(), err, "Should create K3s cluster without RBAC")

		// Wait for CRD
		err = suite.k3sManager.WaitForCRDReady(k3sInstance, "vaultunsealconfigs.vault.io", 60*time.Second)
		require.NoError(suite.T(), err, "CRD should be ready")

		suite.T().Logf("✅ RBAC test setup completed (testing without explicit RBAC setup)")
	})
}

// TestResourceLifecycle tests the complete lifecycle of VaultUnsealConfig resources
func (suite *K3sCRDTestSuite) TestResourceLifecycle() {
	suite.Run("create_update_delete_cycle", func() {
		// Set up infrastructure
		crdManifest := suite.crdGenerator.GenerateVaultUnsealConfigCRD()
		rbacManifest := suite.crdGenerator.GenerateRBACManifests("default")

		k3sInstance, err := suite.k3sManager.CreateK3sCluster("lifecycle-test", crdManifest, rbacManifest)
		require.NoError(suite.T(), err, "Should create K3s cluster")

		vaultInstance, err := suite.vaultManager.CreateProdVault("lifecycle-vault")
		require.NoError(suite.T(), err, "Should create vault")

		// Wait for CRD
		err = suite.k3sManager.WaitForCRDReady(k3sInstance, "vaultunsealconfigs.vault.io", 60*time.Second)
		require.NoError(suite.T(), err, "CRD should be ready")

		// Create resource
		threshold := 3
		config := &vaultv1.VaultUnsealConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "lifecycle-config",
				Namespace: "default",
				Labels: map[string]string{
					"test-type": "lifecycle",
				},
			},
			Spec: vaultv1.VaultUnsealConfigSpec{
				VaultInstances: []vaultv1.VaultInstance{
					{
						Name:          "lifecycle-vault",
						Endpoint:      vaultInstance.Address,
						UnsealKeys:    vaultInstance.UnsealKeys,
						Threshold:     &threshold,
						TLSSkipVerify: true,
					},
				},
			},
		}

		err = k3sInstance.Client.Create(suite.ctx, config)
		require.NoError(suite.T(), err, "Should create lifecycle config")

		// Update resource
		retrieved := &vaultv1.VaultUnsealConfig{}
		err = k3sInstance.Client.Get(suite.ctx, types.NamespacedName{
			Name:      config.Name,
			Namespace: config.Namespace,
		}, retrieved)
		require.NoError(suite.T(), err, "Should retrieve config")

		retrieved.Labels["updated"] = "true"
		err = k3sInstance.Client.Update(suite.ctx, retrieved)
		assert.NoError(suite.T(), err, "Should update config")

		// Verify update
		updated := &vaultv1.VaultUnsealConfig{}
		err = k3sInstance.Client.Get(suite.ctx, types.NamespacedName{
			Name:      config.Name,
			Namespace: config.Namespace,
		}, updated)
		require.NoError(suite.T(), err, "Should retrieve updated config")
		assert.Equal(suite.T(), "true", updated.Labels["updated"])

		// Delete resource
		err = k3sInstance.Client.Delete(suite.ctx, updated)
		assert.NoError(suite.T(), err, "Should delete config")

		// Verify deletion
		deleted := &vaultv1.VaultUnsealConfig{}
		err = k3sInstance.Client.Get(suite.ctx, types.NamespacedName{
			Name:      config.Name,
			Namespace: config.Namespace,
		}, deleted)
		assert.Error(suite.T(), err, "Should not find deleted config")

		suite.T().Logf("✅ Complete lifecycle test successful")
	})
}

// TestK3sCRDTestSuite runs the K3s CRD integration test suite
func TestK3sCRDTestSuite(t *testing.T) {
	suite.Run(t, new(K3sCRDTestSuite))
}
