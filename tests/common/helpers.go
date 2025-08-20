package common

import (
	"context"
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	vaultv1 "github.com/panteparak/vault-autounseal-operator/pkg/api/v1"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// TestConfig holds common test configuration
type TestConfig struct {
	Timeout        time.Duration
	VaultToken     string
	TestNamespace  string
	EnableVerbose  bool
	ParallelismMax int
}

// DefaultTestConfig returns default test configuration
func DefaultTestConfig() *TestConfig {
	return &TestConfig{
		Timeout:        30 * time.Minute,
		VaultToken:     "test-root-token",
		TestNamespace:  "default",
		EnableVerbose:  false,
		ParallelismMax: 4,
	}
}

// K8sClientBuilder helps build Kubernetes clients for tests
type K8sClientBuilder struct {
	scheme *runtime.Scheme
}

// NewK8sClientBuilder creates a new client builder
func NewK8sClientBuilder() *K8sClientBuilder {
	return &K8sClientBuilder{
		scheme: runtime.NewScheme(),
	}
}

// WithScheme adds a scheme to the builder
func (b *K8sClientBuilder) WithScheme(addToScheme func(*runtime.Scheme) error) *K8sClientBuilder {
	if err := addToScheme(b.scheme); err != nil {
		panic(fmt.Sprintf("failed to add scheme: %v", err))
	}
	return b
}

// BuildFromKubeconfig builds a client from kubeconfig bytes
func (b *K8sClientBuilder) BuildFromKubeconfig(t *testing.T, kubeconfig []byte) client.Client {
	// Add default schemes
	err := clientgoscheme.AddToScheme(b.scheme)
	require.NoError(t, err)

	err = vaultv1.AddToScheme(b.scheme)
	require.NoError(t, err)

	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	require.NoError(t, err)

	// Optimize for testing
	restConfig.QPS = 100
	restConfig.Burst = 200

	k8sClient, err := client.New(restConfig, client.Options{Scheme: b.scheme})
	require.NoError(t, err)

	return k8sClient
}

// VaultUnsealConfigBuilder helps build VaultUnsealConfig objects for tests
type VaultUnsealConfigBuilder struct {
	config *vaultv1.VaultUnsealConfig
}

// NewVaultUnsealConfigBuilder creates a new config builder
func NewVaultUnsealConfigBuilder(name, namespace string) *VaultUnsealConfigBuilder {
	return &VaultUnsealConfigBuilder{
		config: &vaultv1.VaultUnsealConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Spec: vaultv1.VaultUnsealConfigSpec{
				VaultInstances: []vaultv1.VaultInstance{},
			},
		},
	}
}

// WithVaultInstance adds a vault instance
func (b *VaultUnsealConfigBuilder) WithVaultInstance(name, endpoint string, unsealKeys []string) *VaultUnsealConfigBuilder {
	instance := vaultv1.VaultInstance{
		Name:       name,
		Endpoint:   endpoint,
		UnsealKeys: unsealKeys,
	}
	b.config.Spec.VaultInstances = append(b.config.Spec.VaultInstances, instance)
	return b
}

// WithVaultInstanceAndThreshold adds a vault instance with threshold
func (b *VaultUnsealConfigBuilder) WithVaultInstanceAndThreshold(name, endpoint string, unsealKeys []string, threshold int) *VaultUnsealConfigBuilder {
	instance := vaultv1.VaultInstance{
		Name:       name,
		Endpoint:   endpoint,
		UnsealKeys: unsealKeys,
		Threshold:  &threshold,
	}
	b.config.Spec.VaultInstances = append(b.config.Spec.VaultInstances, instance)
	return b
}

// WithLabels adds labels to the config
func (b *VaultUnsealConfigBuilder) WithLabels(labels map[string]string) *VaultUnsealConfigBuilder {
	if b.config.Labels == nil {
		b.config.Labels = make(map[string]string)
	}
	for k, v := range labels {
		b.config.Labels[k] = v
	}
	return b
}

// WithAnnotations adds annotations to the config
func (b *VaultUnsealConfigBuilder) WithAnnotations(annotations map[string]string) *VaultUnsealConfigBuilder {
	if b.config.Annotations == nil {
		b.config.Annotations = make(map[string]string)
	}
	for k, v := range annotations {
		b.config.Annotations[k] = v
	}
	return b
}

// Build returns the built configuration
func (b *VaultUnsealConfigBuilder) Build() *vaultv1.VaultUnsealConfig {
	return b.config.DeepCopy()
}

// WaitForAPI waits for Kubernetes API to be ready
func WaitForAPI(ctx context.Context, t *testing.T, k8sClient client.Client, timeout time.Duration) {
	require.Eventually(t, func() bool {
		_, err := k8sClient.RESTMapper().RESTMapping(
			vaultv1.GroupVersion.WithKind("VaultUnsealConfig").GroupKind(),
		)
		return err == nil
	}, timeout, 2*time.Second, "Kubernetes API should become ready")
}

// GenerateTestKeys generates base64 encoded test keys
func GenerateTestKeys(count int) []string {
	keys := make([]string, count)
	for i := 0; i < count; i++ {
		keys[i] = base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("test-key-%d", i)))
	}
	return keys
}

// CleanupConfigs removes all VaultUnsealConfig resources in a namespace
func CleanupConfigs(ctx context.Context, k8sClient client.Client, namespace string) error {
	configList := &vaultv1.VaultUnsealConfigList{}
	listOpts := []client.ListOption{}
	if namespace != "" {
		listOpts = append(listOpts, client.InNamespace(namespace))
	}

	err := k8sClient.List(ctx, configList, listOpts...)
	if err != nil {
		return err
	}

	for _, config := range configList.Items {
		if err := k8sClient.Delete(ctx, &config); err != nil {
			// Continue cleanup even if some deletions fail
			continue
		}
	}

	return nil
}