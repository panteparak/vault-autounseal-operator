package shared

import (
	"context"
	"fmt"
	"time"

	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/k3s"
	"github.com/testcontainers/testcontainers-go/wait"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vaultv1 "github.com/panteparak/vault-autounseal-operator/pkg/api/v1"
	"github.com/panteparak/vault-autounseal-operator/test/config"
)

// K3sInstance represents a configured K3s cluster
type K3sInstance struct {
	Container  *k3s.K3sContainer
	Client     client.Client
	Scheme     *runtime.Scheme
	KubeConfig []byte
}

// K3sManager manages K3s clusters for testing
type K3sManager struct {
	ctx       context.Context
	instances map[string]*K3sInstance
	suite     suite.Suite
	config    *config.Config
}

// NewK3sManager creates a new K3s manager for tests
func NewK3sManager(ctx context.Context, testSuite suite.Suite) *K3sManager {
	// Load configuration
	cfg, err := config.GetGlobalConfig()
	if err != nil {
		testSuite.FailNow("Failed to load configuration", "Error: %v", err)
	}

	return &K3sManager{
		ctx:       ctx,
		instances: make(map[string]*K3sInstance),
		suite:     testSuite,
		config:    cfg,
	}
}

// CreateK3sCluster creates a new K3s cluster with CRDs installed
func (km *K3sManager) CreateK3sCluster(name string, crdManifests ...string) (*K3sInstance, error) {
	// Create K3s container without manifests first
	k3sContainer, err := k3s.Run(km.ctx,
		km.config.GetK3sImage(),
		testcontainers.WithWaitStrategy(
			wait.ForLog("k3s is up and running").
				WithStartupTimeout(km.config.StartupTimeout).
				WithPollInterval(km.config.ReadinessPollInterval),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start K3s cluster: %w", err)
	}

	// Get kubeconfig
	kubeConfig, err := k3sContainer.GetKubeConfig(km.ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	// Create Kubernetes client
	config, err := clientcmd.RESTConfigFromKubeConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create REST config: %w", err)
	}

	// Set up scheme
	scheme := runtime.NewScheme()
	err = clientgoscheme.AddToScheme(scheme)
	if err != nil {
		return nil, fmt.Errorf("failed to add client-go scheme: %w", err)
	}

	err = vaultv1.AddToScheme(scheme)
	if err != nil {
		return nil, fmt.Errorf("failed to add vault scheme: %w", err)
	}

	// Create controller-runtime client
	k8sClient, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	instance := &K3sInstance{
		Container:  k3sContainer,
		Client:     k8sClient,
		Scheme:     scheme,
		KubeConfig: kubeConfig,
	}

	// Apply CRD manifests after cluster is ready
	for _, manifest := range crdManifests {
		if err := km.ApplyManifest(instance, manifest); err != nil {
			return nil, fmt.Errorf("failed to apply CRD manifest: %w", err)
		}
	}

	km.instances[name] = instance
	return instance, nil
}

// CreateK3sClusterWithVersion creates a new K3s cluster with a specific version
func (km *K3sManager) CreateK3sClusterWithVersion(name, version string, crdManifests ...string) (*K3sInstance, error) {
	// Create K3s container with specific version
	k3sContainer, err := k3s.Run(km.ctx,
		km.config.GetK3sImageForVersion(version),
		testcontainers.WithWaitStrategy(
			wait.ForLog("k3s is up and running").
				WithStartupTimeout(km.config.StartupTimeout).
				WithPollInterval(km.config.ReadinessPollInterval),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start K3s cluster %s with version %s: %w", name, version, err)
	}

	// Get kubeconfig
	kubeConfig, err := k3sContainer.GetKubeConfig(km.ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	// Create Kubernetes client
	config, err := clientcmd.RESTConfigFromKubeConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create REST config: %w", err)
	}

	// Set up scheme
	scheme := runtime.NewScheme()
	err = clientgoscheme.AddToScheme(scheme)
	if err != nil {
		return nil, fmt.Errorf("failed to add client-go scheme: %w", err)
	}

	err = vaultv1.AddToScheme(scheme)
	if err != nil {
		return nil, fmt.Errorf("failed to add vault scheme: %w", err)
	}

	// Create controller-runtime client
	k8sClient, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create Kubernetes client: %w", err)
	}

	instance := &K3sInstance{
		Container:  k3sContainer,
		Client:     k8sClient,
		Scheme:     scheme,
		KubeConfig: kubeConfig,
	}

	// Apply CRD manifests after cluster is ready
	for _, manifest := range crdManifests {
		if err := km.ApplyManifest(instance, manifest); err != nil {
			return nil, fmt.Errorf("failed to apply CRD manifest: %w", err)
		}
	}

	km.instances[name] = instance
	return instance, nil
}

// GetInstance returns a K3s instance by name
func (km *K3sManager) GetInstance(name string) (*K3sInstance, bool) {
	instance, exists := km.instances[name]
	return instance, exists
}

// WaitForCRDReady waits for CRDs to be ready in the cluster with proper health checks
func (km *K3sManager) WaitForCRDReady(instance *K3sInstance, crdName string, timeout time.Duration) error {
	ctx, cancel := context.WithTimeout(km.ctx, timeout)
	defer cancel()

	// Use configured backoff settings
	backoff := km.config.RetryBackoff
	maxBackoff := km.config.MaxBackoff

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout waiting for CRD %s to be ready", crdName)
		default:
			// First check if the CRD is installed and API server recognizes it
			if km.checkCRDAvailability(instance, crdName) {
				// Then verify we can actually create resources
				if km.verifyCRDFunctional(instance) {
					return nil
				}
			}

			// Wait with exponential backoff
			time.Sleep(backoff)
			if backoff < maxBackoff {
				backoff *= 2
				if backoff > maxBackoff {
					backoff = maxBackoff
				}
			}
		}
	}
}

// checkCRDAvailability checks if the CRD is available via API discovery
func (km *K3sManager) checkCRDAvailability(instance *K3sInstance, crdName string) bool {
	// Use kubectl to check if the CRD is available
	exitCode, _, err := instance.Container.Exec(km.ctx, []string{
		"kubectl", "api-resources", "--api-group=vault.io", "--no-headers",
	})
	return err == nil && exitCode == 0
}

// verifyCRDFunctional verifies that we can actually work with the CRD
func (km *K3sManager) verifyCRDFunctional(instance *K3sInstance) bool {
	// Try to list resources of the CRD type to check if it's functional
	list := &vaultv1.VaultUnsealConfigList{}
	err := instance.Client.List(km.ctx, list)
	return err == nil
}

// ApplyManifest applies a YAML manifest to the cluster
func (km *K3sManager) ApplyManifest(instance *K3sInstance, manifest string) error {
	// For simplicity, we'll use kubectl through the container
	// In a real implementation, you might want to parse YAML and apply via client-go
	exitCode, reader, err := instance.Container.Exec(km.ctx, []string{
		"sh", "-c", "echo '" + manifest + "' | kubectl apply -f -",
	})
	if err != nil {
		return fmt.Errorf("failed to apply manifest: %w", err)
	}

	if exitCode != 0 {
		// Read error output
		output := make([]byte, 1024)
		n, _ := reader.Read(output)
		return fmt.Errorf("kubectl apply failed with exit code %d: %s", exitCode, string(output[:n]))
	}

	return nil
}

// DeleteManifest deletes a YAML manifest from the cluster
func (km *K3sManager) DeleteManifest(instance *K3sInstance, manifest string) error {
	exitCode, reader, err := instance.Container.Exec(km.ctx, []string{
		"sh", "-c", "echo '" + manifest + "' | kubectl delete -f - --ignore-not-found",
	})
	if err != nil {
		return fmt.Errorf("failed to delete manifest: %w", err)
	}

	if exitCode != 0 {
		// Read error output
		output := make([]byte, 1024)
		n, _ := reader.Read(output)
		return fmt.Errorf("kubectl delete failed with exit code %d: %s", exitCode, string(output[:n]))
	}

	return nil
}

// Cleanup cleans up all K3s instances
func (km *K3sManager) Cleanup() {
	for name, instance := range km.instances {
		if instance.Container != nil {
			if err := testcontainers.TerminateContainer(instance.Container); err != nil {
				fmt.Printf("Failed to cleanup K3s instance %s: %v\n", name, err)
			}
		}
	}
	km.instances = make(map[string]*K3sInstance)
}
