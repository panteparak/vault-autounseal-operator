package testutil

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr"
	vaultv1 "github.com/panteparak/vault-autounseal-operator/pkg/api/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	// TestTimeoutPollingInterval is the polling interval for test assertions.
	TestTimeoutPollingInterval = 100 * time.Millisecond
)

// TestContext provides common testing utilities.
type TestContext struct {
	T      *testing.T
	Client client.Client
	Scheme *runtime.Scheme
	Logger logr.Logger
	Ctx    context.Context
}

// NewTestContext creates a new test context with common setup.
func NewTestContext(t *testing.T) *TestContext {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, vaultv1.AddToScheme(scheme))

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	logger := zap.New(zap.UseDevMode(true))

	return &TestContext{
		T:      t,
		Client: client,
		Scheme: scheme,
		Logger: logger,
		Ctx:    t.Context(),
	}
}

// CreateVaultUnsealConfig creates a test VaultUnsealConfig.
func (tc *TestContext) CreateVaultUnsealConfig(
	name, namespace string,
	instances []vaultv1.VaultInstance,
) *vaultv1.VaultUnsealConfig {
	config := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: instances,
		},
	}

	err := tc.Client.Create(tc.Ctx, config)
	require.NoError(tc.T, err)

	return config
}

// CreateVaultInstance creates a test VaultInstance.
func CreateVaultInstance(name, endpoint string, keys []string, threshold *int) vaultv1.VaultInstance {
	return vaultv1.VaultInstance{
		Name:       name,
		Endpoint:   endpoint,
		UnsealKeys: keys,
		Threshold:  threshold,
	}
}

// AssertCondition asserts that a condition exists and has the expected status.
func (tc *TestContext) AssertCondition(
	config *vaultv1.VaultUnsealConfig,
	conditionType string,
	status metav1.ConditionStatus,
) {
	tc.T.Helper()

	for _, condition := range config.Status.Conditions {
		if condition.Type == conditionType {
			assert.Equal(tc.T, status, condition.Status, "condition %s should have status %s", conditionType, status)

			return
		}
	}

	tc.T.Errorf("condition %s not found", conditionType)
}

// AssertEventuallyReady waits for a VaultUnsealConfig to become ready.
func (tc *TestContext) AssertEventuallyReady(config *vaultv1.VaultUnsealConfig, timeout time.Duration) {
	tc.T.Helper()

	assert.Eventually(tc.T, func() bool {
		err := tc.Client.Get(tc.Ctx, client.ObjectKeyFromObject(config), config)
		if err != nil {
			return false
		}

		for _, condition := range config.Status.Conditions {
			if condition.Type == "Ready" && condition.Status == metav1.ConditionTrue {
				return true
			}
		}

		return false
	}, timeout, TestTimeoutPollingInterval, "VaultUnsealConfig should become ready")
}

// AssertInstanceStatus asserts the status of a specific vault instance.
func (tc *TestContext) AssertInstanceStatus(
	config *vaultv1.VaultUnsealConfig,
	instanceName string,
	expectedSealed bool,
) {
	tc.T.Helper()

	for _, status := range config.Status.VaultStatuses {
		if status.Name == instanceName {
			assert.Equal(tc.T, expectedSealed, status.Sealed, "instance %s sealed status", instanceName)

			return
		}
	}

	tc.T.Errorf("instance status for %s not found", instanceName)
}

// IntPtr returns a pointer to an int value.
func IntPtr(i int) *int {
	return &i
}

// TimePtr returns a pointer to a metav1.Time value.
func TimePtr(t time.Time) *metav1.Time {
	mt := metav1.NewTime(t)

	return &mt
}

// StringSlice returns a slice of strings.
func StringSlice(s ...string) []string {
	return s
}

// MockVaultEndpoint returns a mock vault endpoint URL.
func MockVaultEndpoint(id string) string {
	return "http://vault-" + id + ":8200"
}

// MockUnsealKeys returns mock unseal keys.
func MockUnsealKeys(count int) []string {
	keys := make([]string, count)
	for i := 0; i < count; i++ {
		keys[i] = "key" + string(rune('1'+i))
	}

	return keys
}

// WithTimeout creates a context with timeout for testing.
func WithTimeout(t *testing.T, timeout time.Duration) (context.Context, context.CancelFunc) {
	t.Helper()

	return context.WithTimeout(t.Context(), timeout)
}
