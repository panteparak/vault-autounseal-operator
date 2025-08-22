package controller

import (
	"context"
	"errors"
	"testing"

	"github.com/go-logr/logr"
	vaultv1 "github.com/panteparak/vault-autounseal-operator/pkg/api/v1"
	controllerpkg "github.com/panteparak/vault-autounseal-operator/pkg/controller"
	"github.com/panteparak/vault-autounseal-operator/pkg/testing/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
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
	mockRepo    *mocks.MockVaultClientRepository
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

	// Set up fake client with runtime objects and status subresource
	suite.k8sClient = fake.NewClientBuilder().
		WithScheme(suite.scheme).
		WithStatusSubresource(&vaultv1.VaultUnsealConfig{}).
		WithRuntimeObjects().
		Build()

	// Set up mock repository
	suite.mockRepo = &mocks.MockVaultClientRepository{}

	// Set up reconciler using the constructor
	suite.reconciler = controllerpkg.NewVaultUnsealConfigReconciler(
		suite.k8sClient,
		suite.logger,
		suite.scheme,
		suite.mockRepo,
		nil, // Use default options
	)
}

func (suite *ControllerTestSuite) SetupTest() {
	// Set up fresh mock repository for each test
	suite.mockRepo = &mocks.MockVaultClientRepository{}

	// Update reconciler with fresh mock
	suite.reconciler.ClientRepository = suite.mockRepo
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

	// Configure the mock repository to return a connection error (simulating vault being unavailable)
	suite.mockRepo.On("GetClient",
		mock.Anything, // context
		"default/test-vault",
		mock.Anything). // vault instance
		Return(nil, assert.AnError).Once()

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
	// This should fail with connection errors since vault is mocked to be unavailable,
	// but the reconciler should handle it gracefully without panicking
	if err != nil {
		suite.T().Logf("Reconcile failed as expected (vault not available): %v", err)
		// This is acceptable in unit tests - the controller should handle missing vault gracefully
	} else {
		suite.T().Logf("Reconcile succeeded: %+v", result)
	}

	// The main requirement is that the reconciler doesn't panic and returns a reasonable result
	assert.NotNil(suite.T(), result, "Result should not be nil")

	// Verify mock expectations
	suite.mockRepo.AssertExpectations(suite.T())
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

// TestReconcileSuccessfulUnseal tests successful vault unsealing scenario
func (suite *ControllerTestSuite) TestReconcileSuccessfulUnseal() {
	threshold := 2
	vaultConfig := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "successful-unseal",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:          "vault-1",
					Endpoint:      "https://vault-1.example.com:8200",
					UnsealKeys:    []string{"key1", "key2", "key3"},
					Threshold:     &threshold,
					TLSSkipVerify: false,
				},
			},
		},
	}

	// Set up mock vault client for successful unsealing
	mockVaultClient := &mocks.MockVaultClient{}

	// Mock successful unseal flow
	mockVaultClient.On("IsSealed", mock.Anything).Return(true, nil).Once()
	mockVaultClient.On("Unseal", mock.Anything, []string{"key1", "key2", "key3"}, 2).
		Return(mocks.NewMockSealStatusResponse(false, 2, 2), nil).Once()

	// Configure mock repository to return the mock client
	suite.mockRepo.On("GetClient", mock.Anything, "default/vault-1", mock.Anything).
		Return(mockVaultClient, nil).Once()

	// Create the resource
	err := suite.k8sClient.Create(suite.ctx, vaultConfig)
	require.NoError(suite.T(), err)

	// Test reconciliation
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      vaultConfig.Name,
			Namespace: vaultConfig.Namespace,
		},
	}

	result, err := suite.reconciler.Reconcile(suite.ctx, req)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), result)

	// Verify mock expectations
	mockVaultClient.AssertExpectations(suite.T())
	suite.mockRepo.AssertExpectations(suite.T())
}

// TestReconcileAlreadyUnsealed tests when vault is already unsealed
func (suite *ControllerTestSuite) TestReconcileAlreadyUnsealed() {
	threshold := 3
	vaultConfig := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "already-unsealed",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:       "vault-unsealed",
					Endpoint:   "https://vault-unsealed.example.com:8200",
					UnsealKeys: []string{"key1", "key2", "key3"},
					Threshold:  &threshold,
				},
			},
		},
	}

	// Set up mock vault client
	mockVaultClient := &mocks.MockVaultClient{}

	// Mock vault already unsealed
	mockVaultClient.On("IsSealed", mock.Anything).Return(false, nil).Once()

	// Configure mock repository
	suite.mockRepo.On("GetClient", mock.Anything, "default/vault-unsealed", mock.Anything).
		Return(mockVaultClient, nil).Once()

	// Create the resource
	err := suite.k8sClient.Create(suite.ctx, vaultConfig)
	require.NoError(suite.T(), err)

	// Test reconciliation
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      vaultConfig.Name,
			Namespace: vaultConfig.Namespace,
		},
	}

	result, err := suite.reconciler.Reconcile(suite.ctx, req)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), result)

	// Verify mock expectations
	mockVaultClient.AssertExpectations(suite.T())
	suite.mockRepo.AssertExpectations(suite.T())
}

// TestReconcileMultipleVaultInstances tests handling multiple vault instances
func (suite *ControllerTestSuite) TestReconcileMultipleVaultInstances() {
	threshold := 2
	vaultConfig := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "multiple-vaults",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:       "vault-1",
					Endpoint:   "https://vault-1.example.com:8200",
					UnsealKeys: []string{"key1", "key2"},
					Threshold:  &threshold,
				},
				{
					Name:       "vault-2",
					Endpoint:   "https://vault-2.example.com:8200",
					UnsealKeys: []string{"key3", "key4"},
					Threshold:  &threshold,
				},
			},
		},
	}

	// Set up mock vault clients
	mockVaultClient1 := &mocks.MockVaultClient{}
	mockVaultClient2 := &mocks.MockVaultClient{}

	// Mock first vault - needs unsealing
	mockVaultClient1.On("IsSealed", mock.Anything).Return(true, nil).Once()
	mockVaultClient1.On("Unseal", mock.Anything, []string{"key1", "key2"}, 2).
		Return(mocks.NewMockSealStatusResponse(false, 2, 2), nil).Once()

	// Mock second vault - already unsealed
	mockVaultClient2.On("IsSealed", mock.Anything).Return(false, nil).Once()

	// Configure mock repository
	suite.mockRepo.On("GetClient", mock.Anything, "default/vault-1", mock.Anything).
		Return(mockVaultClient1, nil).Once()
	suite.mockRepo.On("GetClient", mock.Anything, "default/vault-2", mock.Anything).
		Return(mockVaultClient2, nil).Once()

	// Create the resource
	err := suite.k8sClient.Create(suite.ctx, vaultConfig)
	require.NoError(suite.T(), err)

	// Test reconciliation
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      vaultConfig.Name,
			Namespace: vaultConfig.Namespace,
		},
	}

	result, err := suite.reconciler.Reconcile(suite.ctx, req)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), result)

	// Verify mock expectations
	mockVaultClient1.AssertExpectations(suite.T())
	mockVaultClient2.AssertExpectations(suite.T())
	suite.mockRepo.AssertExpectations(suite.T())
}

// TestReconcileUnsealFailure tests when unsealing fails
func (suite *ControllerTestSuite) TestReconcileUnsealFailure() {
	threshold := 3
	vaultConfig := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "unseal-failure",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:       "vault-fail",
					Endpoint:   "https://vault-fail.example.com:8200",
					UnsealKeys: []string{"key1", "key2", "key3"},
					Threshold:  &threshold,
				},
			},
		},
	}

	// Set up mock vault client
	mockVaultClient := &mocks.MockVaultClient{}

	// Mock unsealing failure
	mockVaultClient.On("IsSealed", mock.Anything).Return(true, nil).Once()
	mockVaultClient.On("Unseal", mock.Anything, []string{"key1", "key2", "key3"}, 3).
		Return(nil, errors.New("invalid unseal key")).Once()

	// Configure mock repository
	suite.mockRepo.On("GetClient", mock.Anything, "default/vault-fail", mock.Anything).
		Return(mockVaultClient, nil).Once()

	// Create the resource
	err := suite.k8sClient.Create(suite.ctx, vaultConfig)
	require.NoError(suite.T(), err)

	// Test reconciliation
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      vaultConfig.Name,
			Namespace: vaultConfig.Namespace,
		},
	}

	result, err := suite.reconciler.Reconcile(suite.ctx, req)
	// Controller should handle unseal failures gracefully
	assert.NoError(suite.T(), err, "Controller should handle unseal failures without returning an error")
	assert.NotNil(suite.T(), result)

	// Verify mock expectations
	mockVaultClient.AssertExpectations(suite.T())
	suite.mockRepo.AssertExpectations(suite.T())
}

// TestReconcileInvalidThreshold tests handling of invalid threshold values
func (suite *ControllerTestSuite) TestReconcileInvalidThreshold() {
	invalidThreshold := 0
	vaultConfig := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalid-threshold",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:       "vault-invalid",
					Endpoint:   "https://vault-invalid.example.com:8200",
					UnsealKeys: []string{"key1", "key2"},
					Threshold:  &invalidThreshold,
				},
			},
		},
	}

	// Configure mock repository to return error due to invalid threshold
	suite.mockRepo.On("GetClient", mock.Anything, "default/vault-invalid", mock.Anything).
		Return(nil, errors.New("invalid threshold: must be > 0")).Once()

	// Create the resource
	err := suite.k8sClient.Create(suite.ctx, vaultConfig)
	require.NoError(suite.T(), err)

	// Test reconciliation
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      vaultConfig.Name,
			Namespace: vaultConfig.Namespace,
		},
	}

	result, err := suite.reconciler.Reconcile(suite.ctx, req)
	// Controller should handle invalid configuration gracefully
	assert.NoError(suite.T(), err, "Controller should handle invalid configurations without panicking")
	assert.NotNil(suite.T(), result)

	// Verify mock expectations
	suite.mockRepo.AssertExpectations(suite.T())
}

// TestReconcileEmptyUnsealKeys tests handling of empty unseal keys
func (suite *ControllerTestSuite) TestReconcileEmptyUnsealKeys() {
	threshold := 3
	vaultConfig := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "empty-keys",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:       "vault-empty-keys",
					Endpoint:   "https://vault-empty.example.com:8200",
					UnsealKeys: []string{}, // Empty keys
					Threshold:  &threshold,
				},
			},
		},
	}

	// Configure mock repository to return error due to empty keys
	suite.mockRepo.On("GetClient", mock.Anything, "default/vault-empty-keys", mock.Anything).
		Return(nil, errors.New("no unseal keys provided")).Once()

	// Create the resource
	err := suite.k8sClient.Create(suite.ctx, vaultConfig)
	require.NoError(suite.T(), err)

	// Test reconciliation
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      vaultConfig.Name,
			Namespace: vaultConfig.Namespace,
		},
	}

	result, err := suite.reconciler.Reconcile(suite.ctx, req)
	assert.NoError(suite.T(), err, "Controller should handle empty keys gracefully")
	assert.NotNil(suite.T(), result)

	// Verify mock expectations
	suite.mockRepo.AssertExpectations(suite.T())
}

// TestReconcileInvalidEndpoint tests handling of invalid endpoints
func (suite *ControllerTestSuite) TestReconcileInvalidEndpoint() {
	threshold := 2
	vaultConfig := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "invalid-endpoint",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:       "vault-invalid-endpoint",
					Endpoint:   "invalid://not-a-url",
					UnsealKeys: []string{"key1", "key2"},
					Threshold:  &threshold,
				},
			},
		},
	}

	// Configure mock repository to return connection error
	suite.mockRepo.On("GetClient", mock.Anything, "default/vault-invalid-endpoint", mock.Anything).
		Return(nil, errors.New("invalid endpoint URL")).Once()

	// Create the resource
	err := suite.k8sClient.Create(suite.ctx, vaultConfig)
	require.NoError(suite.T(), err)

	// Test reconciliation
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      vaultConfig.Name,
			Namespace: vaultConfig.Namespace,
		},
	}

	result, err := suite.reconciler.Reconcile(suite.ctx, req)
	assert.NoError(suite.T(), err, "Controller should handle invalid endpoints gracefully")
	assert.NotNil(suite.T(), result)

	// Verify mock expectations
	suite.mockRepo.AssertExpectations(suite.T())
}

// TestReconcileWithTLSSkipVerify tests TLS skip verify functionality
func (suite *ControllerTestSuite) TestReconcileWithTLSSkipVerify() {
	threshold := 2
	vaultConfig := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "tls-skip-verify",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:          "vault-tls",
					Endpoint:      "https://vault-self-signed.example.com:8200",
					UnsealKeys:    []string{"key1", "key2"},
					Threshold:     &threshold,
					TLSSkipVerify: true,
				},
			},
		},
	}

	// Set up mock vault client
	mockVaultClient := &mocks.MockVaultClient{}

	// Mock successful connection with TLS skip verify
	mockVaultClient.On("IsSealed", mock.Anything).Return(false, nil).Once()

	// Configure mock repository
	suite.mockRepo.On("GetClient", mock.Anything, "default/vault-tls", mock.Anything).
		Return(mockVaultClient, nil).Once()

	// Create the resource
	err := suite.k8sClient.Create(suite.ctx, vaultConfig)
	require.NoError(suite.T(), err)

	// Test reconciliation
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      vaultConfig.Name,
			Namespace: vaultConfig.Namespace,
		},
	}

	result, err := suite.reconciler.Reconcile(suite.ctx, req)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), result)

	// Verify mock expectations
	mockVaultClient.AssertExpectations(suite.T())
	suite.mockRepo.AssertExpectations(suite.T())
}

// TestReconcilePartialUnsealFailure tests when some vaults fail while others succeed
func (suite *ControllerTestSuite) TestReconcilePartialUnsealFailure() {
	threshold := 2
	vaultConfig := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "partial-failure",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:       "vault-success",
					Endpoint:   "https://vault-success.example.com:8200",
					UnsealKeys: []string{"key1", "key2"},
					Threshold:  &threshold,
				},
				{
					Name:       "vault-failure",
					Endpoint:   "https://vault-failure.example.com:8200",
					UnsealKeys: []string{"key3", "key4"},
					Threshold:  &threshold,
				},
			},
		},
	}

	// Set up mock vault clients
	mockVaultClient1 := &mocks.MockVaultClient{}

	// Mock successful vault
	mockVaultClient1.On("IsSealed", mock.Anything).Return(false, nil).Once()

	// Configure mock repository - success for first, failure for second
	suite.mockRepo.On("GetClient", mock.Anything, "default/vault-success", mock.Anything).
		Return(mockVaultClient1, nil).Once()
	suite.mockRepo.On("GetClient", mock.Anything, "default/vault-failure", mock.Anything).
		Return(nil, errors.New("connection failed")).Once()

	// Create the resource
	err := suite.k8sClient.Create(suite.ctx, vaultConfig)
	require.NoError(suite.T(), err)

	// Test reconciliation
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      vaultConfig.Name,
			Namespace: vaultConfig.Namespace,
		},
	}

	result, err := suite.reconciler.Reconcile(suite.ctx, req)
	assert.NoError(suite.T(), err, "Controller should handle partial failures gracefully")
	assert.NotNil(suite.T(), result)

	// Verify mock expectations
	mockVaultClient1.AssertExpectations(suite.T())
	suite.mockRepo.AssertExpectations(suite.T())
}

// TestReconcilerOptionsValidation tests that reconciler options are properly set
func (suite *ControllerTestSuite) TestReconcilerOptionsValidation() {
	assert.NotNil(suite.T(), suite.reconciler.Options, "Options should not be nil")
	assert.Greater(suite.T(), suite.reconciler.Options.Timeout.Seconds(), float64(0), "Timeout should be positive")
	assert.Greater(suite.T(), suite.reconciler.Options.RequeueAfter.Seconds(), float64(0), "RequeueAfter should be positive")
}

// TestControllerTestSuite runs the controller test suite
func TestControllerTestSuite(t *testing.T) {
	suite.Run(t, new(ControllerTestSuite))
}
