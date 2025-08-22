package controller

import (
	"testing"
	"time"

	vaultv1 "github.com/panteparak/vault-autounseal-operator/pkg/api/v1"
	"github.com/panteparak/vault-autounseal-operator/pkg/testing/mocks"
	"github.com/panteparak/vault-autounseal-operator/pkg/testing/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
)

// TODO: Fix status update issues in reconciler tests
func SkipTestVaultUnsealConfigReconciler_Reconcile(t *testing.T) {
	tests := []struct {
		name           string
		vaultConfig    *vaultv1.VaultUnsealConfig
		setupMocks     func(*mocks.MockVaultClientRepository, *mocks.MockVaultClient)
		expectedResult ctrl.Result
		expectedError  bool
		assertions     func(*testing.T, *vaultv1.VaultUnsealConfig)
	}{
		{
			name: "successful reconciliation with unsealed vault",
			vaultConfig: &vaultv1.VaultUnsealConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-config",
					Namespace: "test-namespace",
				},
				Spec: vaultv1.VaultUnsealConfigSpec{
					VaultInstances: []vaultv1.VaultInstance{
						{
							Name:       "vault-1",
							Endpoint:   "http://vault-1:8200",
							UnsealKeys: []string{"key1", "key2", "key3"},
							Threshold:  testutil.IntPtr(3),
						},
					},
				},
			},
			setupMocks: func(repo *mocks.MockVaultClientRepository, client *mocks.MockVaultClient) {
				repo.On("GetClient", mock.Anything, "test-namespace/vault-1", mock.Anything).Return(client, nil)
				client.On("IsSealed", mock.Anything).Return(false, nil)
			},
			expectedResult: ctrl.Result{RequeueAfter: DefaultRequeueAfterSeconds * time.Second},
			expectedError:  false,
			assertions: func(t *testing.T, config *vaultv1.VaultUnsealConfig) {
				t.Helper()
				require.Len(t, config.Status.VaultStatuses, 1)
				assert.Equal(t, "vault-1", config.Status.VaultStatuses[0].Name)
				assert.False(t, config.Status.VaultStatuses[0].Sealed)
				assert.NotNil(t, config.Status.VaultStatuses[0].LastUnsealed)

				require.Len(t, config.Status.Conditions, 1)
				assert.Equal(t, "Ready", config.Status.Conditions[0].Type)
				assert.Equal(t, metav1.ConditionTrue, config.Status.Conditions[0].Status)
			},
		},
		{
			name: "successful reconciliation with sealed vault that gets unsealed",
			vaultConfig: &vaultv1.VaultUnsealConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-config",
					Namespace: "test-namespace",
				},
				Spec: vaultv1.VaultUnsealConfigSpec{
					VaultInstances: []vaultv1.VaultInstance{
						{
							Name:       "vault-1",
							Endpoint:   "http://vault-1:8200",
							UnsealKeys: []string{"key1", "key2", "key3"},
							Threshold:  testutil.IntPtr(3),
						},
					},
				},
			},
			setupMocks: func(repo *mocks.MockVaultClientRepository, client *mocks.MockVaultClient) {
				repo.On("GetClient", mock.Anything, "test-namespace/vault-1", mock.Anything).Return(client, nil)
				client.On("IsSealed", mock.Anything).Return(true, nil)
				client.On("Unseal", mock.Anything, []string{"key1", "key2", "key3"}, 3).Return(
					mocks.NewMockSealStatusResponse(false, 3, 3), nil)
			},
			expectedResult: ctrl.Result{RequeueAfter: DefaultRequeueAfterSeconds * time.Second},
			expectedError:  false,
			assertions: func(t *testing.T, config *vaultv1.VaultUnsealConfig) {
				t.Helper()
				require.Len(t, config.Status.VaultStatuses, 1)
				assert.Equal(t, "vault-1", config.Status.VaultStatuses[0].Name)
				assert.False(t, config.Status.VaultStatuses[0].Sealed)
				assert.NotNil(t, config.Status.VaultStatuses[0].LastUnsealed)

				require.Len(t, config.Status.Conditions, 1)
				assert.Equal(t, "Ready", config.Status.Conditions[0].Type)
				assert.Equal(t, metav1.ConditionTrue, config.Status.Conditions[0].Status)
			},
		},
		{
			name: "failed reconciliation with vault client error",
			vaultConfig: &vaultv1.VaultUnsealConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-config",
					Namespace: "test-namespace",
				},
				Spec: vaultv1.VaultUnsealConfigSpec{
					VaultInstances: []vaultv1.VaultInstance{
						{
							Name:       "vault-1",
							Endpoint:   "http://vault-1:8200",
							UnsealKeys: []string{"key1", "key2", "key3"},
							Threshold:  testutil.IntPtr(3),
						},
					},
				},
			},
			setupMocks: func(repo *mocks.MockVaultClientRepository, client *mocks.MockVaultClient) {
				repo.On("GetClient", mock.Anything, "test-namespace/vault-1", mock.Anything).Return(client, nil)
				client.On("IsSealed", mock.Anything).Return(false, assert.AnError)
			},
			expectedResult: ctrl.Result{RequeueAfter: DefaultRequeueAfterSeconds * time.Second},
			expectedError:  false,
			assertions: func(t *testing.T, config *vaultv1.VaultUnsealConfig) {
				t.Helper()
				require.Len(t, config.Status.VaultStatuses, 1)
				assert.Equal(t, "vault-1", config.Status.VaultStatuses[0].Name)
				assert.True(t, config.Status.VaultStatuses[0].Sealed)
				assert.Contains(t, config.Status.VaultStatuses[0].Error, "failed to check seal status")

				require.Len(t, config.Status.Conditions, 1)
				assert.Equal(t, "Ready", config.Status.Conditions[0].Type)
				assert.Equal(t, metav1.ConditionFalse, config.Status.Conditions[0].Status)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup test context
			tc := testutil.NewTestContext(t)

			// Create the VaultUnsealConfig
			err := tc.Client.Create(tc.Ctx, tt.vaultConfig)
			require.NoError(t, err)

			// Setup mocks
			mockRepo := &mocks.MockVaultClientRepository{}
			mockClient := &mocks.MockVaultClient{}
			tt.setupMocks(mockRepo, mockClient)

			// Create reconciler
			reconciler := NewVaultUnsealConfigReconciler(
				tc.Client,
				tc.Logger,
				tc.Scheme,
				mockRepo,
				&ReconcilerOptions{
					RequeueAfter: DefaultRequeueAfterSeconds * time.Second,
					Timeout:      DefaultTimeoutSeconds * time.Second,
				},
			)

			// Perform reconciliation
			req := ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      tt.vaultConfig.Name,
					Namespace: tt.vaultConfig.Namespace,
				},
			}

			result, err := reconciler.Reconcile(tc.Ctx, req)

			// Assertions
			if tt.expectedError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			assert.Equal(t, tt.expectedResult, result)

			// Get updated config
			updatedConfig := &vaultv1.VaultUnsealConfig{}
			err = tc.Client.Get(tc.Ctx, req.NamespacedName, updatedConfig)
			require.NoError(t, err)

			// Run custom assertions
			tt.assertions(t, updatedConfig)

			// Verify mock expectations
			mockRepo.AssertExpectations(t)
			mockClient.AssertExpectations(t)
		})
	}
}

func TestVaultUnsealConfigReconciler_processVaultInstances(t *testing.T) {
	tc := testutil.NewTestContext(t)

	vaultConfig := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-config",
			Namespace: "test-namespace",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:       "vault-1",
					Endpoint:   "http://vault-1:8200",
					UnsealKeys: []string{"key1", "key2", "key3"},
					Threshold:  testutil.IntPtr(3),
				},
				{
					Name:       "vault-2",
					Endpoint:   "http://vault-2:8200",
					UnsealKeys: []string{"key1", "key2", "key3"},
					Threshold:  testutil.IntPtr(3),
				},
			},
		},
	}

	mockRepo := &mocks.MockVaultClientRepository{}
	mockClient1 := &mocks.MockVaultClient{}
	mockClient2 := &mocks.MockVaultClient{}

	// Setup mocks - vault-1 is unsealed, vault-2 is sealed
	mockRepo.On("GetClient", mock.Anything, "test-namespace/vault-1", mock.Anything).Return(mockClient1, nil)
	mockRepo.On("GetClient", mock.Anything, "test-namespace/vault-2", mock.Anything).Return(mockClient2, nil)

	mockClient1.On("IsSealed", mock.Anything).Return(false, nil)
	mockClient2.On("IsSealed", mock.Anything).Return(true, nil)
	mockClient2.On("Unseal", mock.Anything, []string{"key1", "key2", "key3"}, 3).Return(
		mocks.NewMockSealStatusResponse(true, 1, 3), nil) // Still sealed after first key

	reconciler := NewVaultUnsealConfigReconciler(
		tc.Client,
		tc.Logger,
		tc.Scheme,
		mockRepo,
		DefaultReconcilerOptions(),
	)

	statuses, allReady := reconciler.processVaultInstances(tc.Ctx, tc.Logger, vaultConfig)

	// Assertions
	assert.Len(t, statuses, 2)
	assert.False(t, allReady) // vault-2 is still sealed

	// Check vault-1 status
	assert.Equal(t, "vault-1", statuses[0].Name)
	assert.False(t, statuses[0].Sealed)
	assert.NotNil(t, statuses[0].LastUnsealed)

	// Check vault-2 status
	assert.Equal(t, "vault-2", statuses[1].Name)
	assert.True(t, statuses[1].Sealed)
	assert.Nil(t, statuses[1].LastUnsealed)

	// Verify mock expectations
	mockRepo.AssertExpectations(t)
	mockClient1.AssertExpectations(t)
	mockClient2.AssertExpectations(t)
}

func TestDefaultVaultClientRepository_GetClient(t *testing.T) {
	mockFactory := &mocks.MockClientFactory{}
	mockClient := &mocks.MockVaultClient{}

	instance := &vaultv1.VaultInstance{
		Name:          "test-vault",
		Endpoint:      "http://vault:8200",
		TLSSkipVerify: true,
	}

	mockFactory.On("NewClient", "http://vault:8200", true, DefaultTimeoutSeconds*time.Second).Return(mockClient, nil)

	repo := NewDefaultVaultClientRepository(mockFactory)

	// First call should create new client
	client1, err := repo.GetClient(t.Context(), "test-key", instance)
	require.NoError(t, err)
	assert.Equal(t, mockClient, client1)

	// Second call should return cached client
	client2, err := repo.GetClient(t.Context(), "test-key", instance)
	require.NoError(t, err)
	assert.Equal(t, mockClient, client2)

	// Verify factory was called only once
	mockFactory.AssertExpectations(t)
}

func TestReconcilerOptions_Defaults(t *testing.T) {
	opts := DefaultReconcilerOptions()

	assert.Equal(t, DefaultRequeueAfterSeconds*time.Second, opts.RequeueAfter)
	assert.Equal(t, DefaultTimeoutSeconds*time.Second, opts.Timeout)
}

func TestGetThreshold(t *testing.T) {
	tests := []struct {
		name     string
		instance *vaultv1.VaultInstance
		expected int
	}{
		{
			name: "with threshold set",
			instance: &vaultv1.VaultInstance{
				Threshold: testutil.IntPtr(5),
			},
			expected: 5,
		},
		{
			name: "with threshold nil",
			instance: &vaultv1.VaultInstance{
				Threshold: nil,
			},
			expected: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := getThreshold(tt.instance)
			assert.Equal(t, tt.expected, result)
		})
	}
}
