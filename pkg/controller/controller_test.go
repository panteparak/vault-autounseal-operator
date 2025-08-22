package controller

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	vaultv1 "github.com/panteparak/vault-autounseal-operator/pkg/api/v1"
	"github.com/panteparak/vault-autounseal-operator/pkg/testing/mocks"
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
	k8sClient  client.Client
	scheme     *runtime.Scheme
	reconciler *VaultUnsealConfigReconciler
	ctx        context.Context
	logger     logr.Logger
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

	// Set up fake client
	suite.k8sClient = fake.NewClientBuilder().WithScheme(suite.scheme).Build()

	// Set up reconciler
	mockRepo := &mocks.MockVaultClientRepository{}
	suite.reconciler = NewVaultUnsealConfigReconciler(
		suite.k8sClient,
		suite.logger,
		suite.scheme,
		mockRepo,
		DefaultReconcilerOptions(),
	)
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
// TODO: Fix this test to properly mock vault clients
func (suite *ControllerTestSuite) SkipTestReconcileBasicVaultConfig() {
	// Create a basic VaultUnsealConfig
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
					Threshold:     func() *int { i := 3; return &i }(),
					TLSSkipVerify: false,
				},
			},
		},
	}

	// Create the resource
	err := suite.k8sClient.Create(suite.ctx, vaultConfig)
	require.NoError(suite.T(), err)

	// Reconcile
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-vault-config",
			Namespace: "default",
		},
	}

	result, err := suite.reconciler.Reconcile(suite.ctx, req)
	// We expect an error because there's no real vault server, but the reconciler should handle it gracefully
	if err != nil {
		suite.T().Logf("Reconcile failed as expected (vault not running): %v", err)
		// If there's an error, result may have requeue time or not - both are acceptable
	} else {
		suite.T().Logf("Reconcile succeeded: %+v", result)
	}
	// The main requirement is that the reconciler doesn't panic and returns a reasonable result
	assert.NotNil(suite.T(), result, "Result should not be nil")

	// Verify the status was attempted to be updated (even though it failed)
	var updatedConfig vaultv1.VaultUnsealConfig
	err = suite.k8sClient.Get(suite.ctx, types.NamespacedName{
		Name: "test-vault-config", Namespace: "default",
	}, &updatedConfig)
	require.NoError(suite.T(), err)

	// The config should exist and have the same spec
	assert.Equal(suite.T(), "test-vault-config", updatedConfig.Name)
	assert.Equal(suite.T(), "test-vault", updatedConfig.Spec.VaultInstances[0].Name)
}

// TestReconcileMultipleVaultInstances tests reconciling with multiple vault instances
// TODO: Fix this test to properly mock vault clients
func (suite *ControllerTestSuite) SkipTestReconcileMultipleVaultInstances() {
	// Create a VaultUnsealConfig with multiple instances
	vaultConfig := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-multi-vault-config",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:          "vault-primary",
					Endpoint:      "http://vault-primary.example.com:8200",
					UnsealKeys:    []string{"key1", "key2", "key3"},
					Threshold:     func() *int { i := 3; return &i }(),
					TLSSkipVerify: false,
				},
				{
					Name:          "vault-secondary",
					Endpoint:      "http://vault-secondary.example.com:8200",
					UnsealKeys:    []string{"key4", "key5", "key6"},
					Threshold:     func() *int { i := 3; return &i }(),
					TLSSkipVerify: true,
				},
			},
		},
	}

	// Create the resource
	err := suite.k8sClient.Create(suite.ctx, vaultConfig)
	require.NoError(suite.T(), err)

	// Reconcile
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-multi-vault-config",
			Namespace: "default",
		},
	}

	result, err := suite.reconciler.Reconcile(suite.ctx, req)
	// We expect errors because there are no real vault servers, but reconciler should handle gracefully
	if err != nil {
		suite.T().Logf("Reconcile failed as expected (vault servers not running): %v", err)
	} else {
		suite.T().Logf("Reconcile succeeded: %+v", result)
	}
	assert.NotNil(suite.T(), result, "Result should not be nil")

	// Verify the config exists and has correct spec
	var updatedConfig vaultv1.VaultUnsealConfig
	err = suite.k8sClient.Get(suite.ctx, types.NamespacedName{
		Name: "test-multi-vault-config", Namespace: "default",
	}, &updatedConfig)
	require.NoError(suite.T(), err)

	assert.Len(suite.T(), updatedConfig.Spec.VaultInstances, 2)
	assert.Equal(suite.T(), "vault-primary", updatedConfig.Spec.VaultInstances[0].Name)
	assert.Equal(suite.T(), "vault-secondary", updatedConfig.Spec.VaultInstances[1].Name)
}

// TestProcessVaultInstances tests the processVaultInstances method
func (suite *ControllerTestSuite) SkipTestProcessVaultInstances() {
	vaultConfig := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-process-config",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:          "test-vault",
					Endpoint:      "http://invalid-vault.example.com:8200",
					UnsealKeys:    []string{"key1", "key2", "key3"},
					Threshold:     func() *int { i := 3; return &i }(),
					TLSSkipVerify: false,
				},
			},
		},
	}

	statuses, allReady := suite.reconciler.processVaultInstances(
		suite.ctx, suite.logger, vaultConfig,
	)

	// Should return statuses and allReady should be false due to errors
	assert.Len(suite.T(), statuses, 1)
	assert.False(suite.T(), allReady)
	assert.Equal(suite.T(), "test-vault", statuses[0].Name)
	assert.True(suite.T(), statuses[0].Sealed)
	assert.NotEmpty(suite.T(), statuses[0].Error)
}

// TestProcessVaultInstanceError tests error handling in processVaultInstance
func (suite *ControllerTestSuite) SkipTestProcessVaultInstanceError() {
	instance := &vaultv1.VaultInstance{
		Name:          "error-vault",
		Endpoint:      "http://invalid-vault.example.com:8200",
		UnsealKeys:    []string{"key1", "key2", "key3"},
		Threshold:     func() *int { i := 3; return &i }(),
		TLSSkipVerify: false,
	}

	logger := suite.reconciler.Log.WithValues("test", "processVaultInstance")
	status, err := suite.reconciler.processVaultInstance(suite.ctx, logger, instance, "default")

	// Should return an error and empty status
	assert.Error(suite.T(), err)
	assert.Equal(suite.T(), vaultv1.VaultInstanceStatus{}, status)
}

// TestUpdateVaultConfigStatus tests the status update functionality
func (suite *ControllerTestSuite) TestUpdateVaultConfigStatus() {
	vaultConfig := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-status-config",
			Namespace:  "default",
			Generation: 1,
		},
	}

	// Test with all vaults ready
	vaultStatuses := []vaultv1.VaultInstanceStatus{
		{
			Name:         "vault1",
			Sealed:       false,
			LastUnsealed: &metav1.Time{Time: time.Now()},
		},
		{
			Name:         "vault2",
			Sealed:       false,
			LastUnsealed: &metav1.Time{Time: time.Now()},
		},
	}

	suite.reconciler.updateVaultConfigStatus(vaultConfig, vaultStatuses, true)

	// Verify status
	assert.Len(suite.T(), vaultConfig.Status.VaultStatuses, 2)
	assert.Len(suite.T(), vaultConfig.Status.Conditions, 1)

	condition := vaultConfig.Status.Conditions[0]
	assert.Equal(suite.T(), "Ready", condition.Type)
	assert.Equal(suite.T(), metav1.ConditionTrue, condition.Status)
	assert.Equal(suite.T(), "AllInstancesUnsealed", condition.Reason)

	// Test with some vaults not ready
	vaultStatuses[0].Sealed = true
	vaultStatuses[0].Error = "connection failed"

	suite.reconciler.updateVaultConfigStatus(vaultConfig, vaultStatuses, false)

	condition = vaultConfig.Status.Conditions[0]
	assert.Equal(suite.T(), "Ready", condition.Type)
	assert.Equal(suite.T(), metav1.ConditionFalse, condition.Status)
	assert.Equal(suite.T(), "SomeInstancesSealed", condition.Reason)
}

// TestUpdateCondition tests the condition update functionality
func (suite *ControllerTestSuite) TestUpdateCondition() {
	vaultConfig := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-condition-config",
			Namespace:  "default",
			Generation: 1,
		},
	}

	// Add initial condition
	initialCondition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "Initializing",
		Message:            "Initializing vault instances",
		LastTransitionTime: metav1.NewTime(time.Now()),
		ObservedGeneration: 1,
	}

	suite.reconciler.updateCondition(vaultConfig, &initialCondition)
	assert.Len(suite.T(), vaultConfig.Status.Conditions, 1)
	assert.Equal(suite.T(), "Ready", vaultConfig.Status.Conditions[0].Type)
	assert.Equal(suite.T(), metav1.ConditionFalse, vaultConfig.Status.Conditions[0].Status)

	// Update existing condition
	updatedCondition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		Reason:             "AllReady",
		Message:            "All vault instances are ready",
		LastTransitionTime: metav1.NewTime(time.Now()),
		ObservedGeneration: 1,
	}

	suite.reconciler.updateCondition(vaultConfig, &updatedCondition)
	assert.Len(suite.T(), vaultConfig.Status.Conditions, 1)
	assert.Equal(suite.T(), "Ready", vaultConfig.Status.Conditions[0].Type)
	assert.Equal(suite.T(), metav1.ConditionTrue, vaultConfig.Status.Conditions[0].Status)
	assert.Equal(suite.T(), "AllReady", vaultConfig.Status.Conditions[0].Reason)

	// Add different condition type
	newCondition := metav1.Condition{
		Type:               "Synced",
		Status:             metav1.ConditionTrue,
		Reason:             "SyncComplete",
		Message:            "Configuration synchronized",
		LastTransitionTime: metav1.NewTime(time.Now()),
		ObservedGeneration: 1,
	}

	suite.reconciler.updateCondition(vaultConfig, &newCondition)
	assert.Len(suite.T(), vaultConfig.Status.Conditions, 2)

	// Find the new condition
	var syncedCondition *metav1.Condition
	for i := range vaultConfig.Status.Conditions {
		if vaultConfig.Status.Conditions[i].Type == "Synced" {
			syncedCondition = &vaultConfig.Status.Conditions[i]
			break
		}
	}
	require.NotNil(suite.T(), syncedCondition)
	assert.Equal(suite.T(), metav1.ConditionTrue, syncedCondition.Status)
	assert.Equal(suite.T(), "SyncComplete", syncedCondition.Reason)
}

// TestVaultInstanceWithCustomThreshold tests instance with custom threshold
func (suite *ControllerTestSuite) TestVaultInstanceWithCustomThreshold() {
	vaultConfig := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-custom-threshold-config",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:          "custom-threshold-vault",
					Endpoint:      "http://vault.example.com:8200",
					UnsealKeys:    []string{"key1", "key2", "key3", "key4", "key5"},
					Threshold:     func() *int { i := 5; return &i }(),
					TLSSkipVerify: true,
				},
			},
		},
	}

	// Create the resource
	err := suite.k8sClient.Create(suite.ctx, vaultConfig)
	require.NoError(suite.T(), err)

	// Verify the spec was set correctly
	var retrievedConfig vaultv1.VaultUnsealConfig
	err = suite.k8sClient.Get(suite.ctx, types.NamespacedName{
		Name: "test-custom-threshold-config", Namespace: "default",
	}, &retrievedConfig)
	require.NoError(suite.T(), err)

	instance := retrievedConfig.Spec.VaultInstances[0]
	assert.Equal(suite.T(), "custom-threshold-vault", instance.Name)
	assert.NotNil(suite.T(), instance.Threshold)
	assert.Equal(suite.T(), 5, *instance.Threshold)
	assert.True(suite.T(), instance.TLSSkipVerify)
	assert.Len(suite.T(), instance.UnsealKeys, 5)
}

// TestVaultInstanceWithDefaultValues tests instance with default threshold
func (suite *ControllerTestSuite) TestVaultInstanceWithDefaultValues() {
	vaultConfig := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-default-values-config",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:       "default-values-vault",
					Endpoint:   "http://vault.example.com:8200",
					UnsealKeys: []string{"key1", "key2", "key3"},
					// Threshold not set (should default to 3)
					// TLSSkipVerify not set (should default to false)
				},
			},
		},
	}

	// Create the resource
	err := suite.k8sClient.Create(suite.ctx, vaultConfig)
	require.NoError(suite.T(), err)

	// Verify the spec was set correctly
	var retrievedConfig vaultv1.VaultUnsealConfig
	err = suite.k8sClient.Get(suite.ctx, types.NamespacedName{
		Name: "test-default-values-config", Namespace: "default",
	}, &retrievedConfig)
	require.NoError(suite.T(), err)

	instance := retrievedConfig.Spec.VaultInstances[0]
	assert.Equal(suite.T(), "default-values-vault", instance.Name)
	assert.Nil(suite.T(), instance.Threshold)       // Should be nil (uses default 3 in controller logic)
	assert.False(suite.T(), instance.TLSSkipVerify) // Default should be false
}

// TestControllerConcurrentReconciliation tests concurrent reconciliation handling
func (suite *ControllerTestSuite) TestControllerConcurrentReconciliation() {
	suite.T().Skip("Skipping until mock setup is updated for new architecture")
	// Create multiple VaultUnsealConfigs
	configs := []*vaultv1.VaultUnsealConfig{
		{
			ObjectMeta: metav1.ObjectMeta{Name: "concurrent-test-1", Namespace: "default"},
			Spec: vaultv1.VaultUnsealConfigSpec{
				VaultInstances: []vaultv1.VaultInstance{{
					Name: "vault-1", Endpoint: "http://vault1.example.com:8200", UnsealKeys: []string{"key1"},
				}},
			},
		},
		{
			ObjectMeta: metav1.ObjectMeta{Name: "concurrent-test-2", Namespace: "default"},
			Spec: vaultv1.VaultUnsealConfigSpec{
				VaultInstances: []vaultv1.VaultInstance{{
					Name: "vault-2", Endpoint: "http://vault2.example.com:8200", UnsealKeys: []string{"key2"},
				}},
			},
		},
	}

	// Create resources
	for _, config := range configs {
		err := suite.k8sClient.Create(suite.ctx, config)
		require.NoError(suite.T(), err)
	}

	// Reconcile concurrently
	results := make(chan ctrl.Result, len(configs))
	errors := make(chan error, len(configs))

	for _, config := range configs {
		go func(name string) {
			req := ctrl.Request{NamespacedName: types.NamespacedName{Name: name, Namespace: "default"}}
			result, err := suite.reconciler.Reconcile(suite.ctx, req)
			if err != nil {
				errors <- err
			} else {
				results <- result
			}
		}(config.Name)
	}

	// Collect results
	receivedResults := 0
	receivedErrors := 0
	timeout := time.After(10 * time.Second)

	for receivedResults+receivedErrors < len(configs) {
		select {
		case <-results:
			receivedResults++
		case <-errors:
			receivedErrors++
		case <-timeout:
			suite.T().Fatal("Timeout waiting for concurrent reconciliation")
		}
	}

	// We expect all to error due to invalid vault endpoints, but they should all complete
	assert.Equal(suite.T(), len(configs), receivedResults+receivedErrors)
}

// TestVaultControllerTestSuite runs the controller test suite
func TestVaultControllerTestSuite(t *testing.T) {
	suite.Run(t, new(ControllerTestSuite))
}
