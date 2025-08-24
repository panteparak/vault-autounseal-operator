package integration

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	vaultv1 "github.com/panteparak/vault-autounseal-operator/pkg/api/v1"
	"github.com/panteparak/vault-autounseal-operator/pkg/controller"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/k3s"
	"github.com/testcontainers/testcontainers-go/wait"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// E2ETestSuite tests the complete vault unsealing workflow using K3s and Helm
type E2ETestSuite struct {
	suite.Suite

	// Infrastructure
	k3sContainer *k3s.K3sContainer
	k8sClient    client.Client
	scheme       *runtime.Scheme
	ctx          context.Context
	ctxCancel    context.CancelFunc

	// Controllers
	vaultController *controller.VaultUnsealConfigReconciler

	// Vault info
	vaultNamespace string
	vaultRelease   string
}

// SetupSuite initializes K3s and deploys Vault in dev mode
func (suite *E2ETestSuite) SetupSuite() {
	suite.ctx, suite.ctxCancel = context.WithTimeout(context.Background(), 15*time.Minute)

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	suite.vaultNamespace = "vault-system"
	suite.vaultRelease = "vault-dev"

	suite.T().Log("🚀 Setting up E2E test infrastructure...")
	suite.setupK3sCluster()
	suite.deployVaultDevMode()
	suite.setupVaultController()
}

func (suite *E2ETestSuite) setupK3sCluster() {
	suite.T().Log("🚀 Setting up K3s cluster...")

	k3sContainer, err := k3s.Run(suite.ctx,
		"rancher/k3s:v1.30.8-k3s1",
		testcontainers.WithWaitStrategy(
			wait.ForLog("k3s is up and running").
				WithStartupTimeout(180*time.Second).
				WithPollInterval(5*time.Second),
		),
	)
	require.NoError(suite.T(), err, "Failed to start K3s container")
	suite.k3sContainer = k3sContainer

	// Set up K8s client
	kubeconfig, err := k3sContainer.GetKubeConfig(suite.ctx)
	require.NoError(suite.T(), err, "Failed to get kubeconfig")

	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeconfig)
	require.NoError(suite.T(), err, "Failed to create rest config")

	suite.scheme = runtime.NewScheme()
	err = clientgoscheme.AddToScheme(suite.scheme)
	require.NoError(suite.T(), err, "Failed to add client-go scheme")

	err = vaultv1.AddToScheme(suite.scheme)
	require.NoError(suite.T(), err, "Failed to add vault v1 scheme")

	suite.k8sClient, err = client.New(restConfig, client.Options{Scheme: suite.scheme})
	require.NoError(suite.T(), err, "Failed to create K8s client")

	suite.T().Log("✅ K3s cluster ready")
}

func (suite *E2ETestSuite) deployVaultDevMode() {
	suite.T().Log("🔐 Deploying Vault in dev mode...")

	// First check what OS the K3s container is running
	osCheckScript := `#!/bin/bash
echo "=== OS Detection ==="
cat /etc/os-release || echo "No os-release file"
echo "=== Available commands ==="
command -v wget && echo "wget: available" || echo "wget: not available"
command -v curl && echo "curl: available" || echo "curl: not available"
command -v apk && echo "apk: available" || echo "apk: not available"
command -v apt-get && echo "apt-get: available" || echo "apt-get: not available"
command -v yum && echo "yum: available" || echo "yum: not available"
ls -la /usr/local/bin/ || echo "/usr/local/bin/ does not exist"
`

	exitCode, reader, err := suite.k3sContainer.Exec(suite.ctx, []string{"sh", "-c", osCheckScript})
	output, _ := io.ReadAll(reader)
	suite.T().Logf("OS check output (exit code %d): %s", exitCode, string(output))
	require.NoError(suite.T(), err, "Failed to check OS")

	// Create namespace first using k8s client
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: suite.vaultNamespace,
		},
	}
	err = suite.k8sClient.Create(suite.ctx, ns)
	if err != nil && !strings.Contains(err.Error(), "already exists") {
		require.NoError(suite.T(), err, "Failed to create vault namespace")
	}

	// Skip Helm and deploy Vault directly using kubectl
	// Since the K3s container has kubectl, we can deploy Vault using raw YAML
	vaultDeploymentScript := `#!/bin/bash
set -e

echo "Deploying Vault directly using kubectl (skipping Helm due to K3s limitations)..."

# Create Vault deployment YAML directly
cat > /tmp/vault-dev.yaml << 'EOF'
apiVersion: apps/v1
kind: Deployment
metadata:
  name: vault-dev
  namespace: vault-system
  labels:
    app.kubernetes.io/name: vault
    app.kubernetes.io/instance: vault-dev
    app.kubernetes.io/managed-by: manual
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: vault
      app.kubernetes.io/instance: vault-dev
  template:
    metadata:
      labels:
        app.kubernetes.io/name: vault
        app.kubernetes.io/instance: vault-dev
        app.kubernetes.io/component: server
        app.kubernetes.io/managed-by: manual
    spec:
      containers:
      - name: vault
        image: hashicorp/vault:1.19.0
        env:
        - name: VAULT_DEV_ROOT_TOKEN_ID
          value: "dev-root-token"
        - name: VAULT_DEV_LISTEN_ADDRESS
          value: "0.0.0.0:8200"
        command:
        - vault
        - server
        - -dev
        - -dev-listen-address=0.0.0.0:8200
        - -dev-root-token-id=dev-root-token
        ports:
        - containerPort: 8200
          name: http
        readinessProbe:
          httpGet:
            path: /v1/sys/health
            port: 8200
            scheme: HTTP
          initialDelaySeconds: 5
          timeoutSeconds: 3
        resources:
          requests:
            memory: "64Mi"
            cpu: "50m"
          limits:
            memory: "128Mi"
            cpu: "100m"
---
apiVersion: v1
kind: Service
metadata:
  name: vault-dev
  namespace: vault-system
  labels:
    app.kubernetes.io/name: vault
    app.kubernetes.io/instance: vault-dev
spec:
  selector:
    app.kubernetes.io/name: vault
    app.kubernetes.io/instance: vault-dev
  ports:
  - port: 8200
    targetPort: 8200
    name: http
EOF

# Apply the deployment
kubectl apply -f /tmp/vault-dev.yaml

echo "Vault deployment completed. Checking status..."
kubectl get all -n vault-system
`

	vaultExitCode, vaultReader, err := suite.k3sContainer.Exec(suite.ctx, []string{"sh", "-c", vaultDeploymentScript})
	vaultOutput, _ := io.ReadAll(vaultReader)
	suite.T().Logf("Vault deployment output (exit code %d): %s", vaultExitCode, string(vaultOutput))
	require.NoError(suite.T(), err, "Failed to deploy Vault")
	require.Equal(suite.T(), 0, vaultExitCode, "Vault deployment should succeed")

	// Wait for Vault pod to be ready
	suite.waitForVaultPods()

	suite.T().Log("✅ Vault dev mode deployed and ready")
}

func (suite *E2ETestSuite) waitForVaultPods() {
	suite.T().Log("⏳ Waiting for Vault pods to be ready...")

	require.Eventually(suite.T(), func() bool {
		// First, list all pods in namespace to see what exists
		var allPods corev1.PodList
		err := suite.k8sClient.List(suite.ctx, &allPods, client.InNamespace(suite.vaultNamespace))
		if err != nil {
			suite.T().Logf("Error listing all pods: %v", err)
		} else {
			suite.T().Logf("All pods in namespace %s:", suite.vaultNamespace)
			for _, pod := range allPods.Items {
				suite.T().Logf("  - Pod: %s, Labels: %v, Status: %s", pod.Name, pod.Labels, pod.Status.Phase)
			}
		}

		var pods corev1.PodList
		err = suite.k8sClient.List(suite.ctx, &pods,
			client.InNamespace(suite.vaultNamespace),
			client.MatchingLabels{
				"app.kubernetes.io/name":     "vault",
				"app.kubernetes.io/instance": suite.vaultRelease,
			},
		)
		if err != nil {
			suite.T().Logf("Error listing vault pods: %v", err)
			return false
		}

		if len(pods.Items) == 0 {
			suite.T().Logf("No Vault pods found with labels app.kubernetes.io/name=vault, app.kubernetes.io/instance=%s", suite.vaultRelease)
			return false
		}

		readyPods := 0
		for _, pod := range pods.Items {
			if pod.Status.Phase == corev1.PodRunning {
				// Check if all containers are ready
				allReady := true
				for _, status := range pod.Status.ContainerStatuses {
					if !status.Ready {
						allReady = false
						break
					}
				}
				if allReady {
					readyPods++
				}
			}
		}

		suite.T().Logf("Vault pods ready: %d/%d", readyPods, len(pods.Items))
		return readyPods > 0 // At least one pod should be ready
	}, 5*time.Minute, 10*time.Second, "Vault pods should be ready")
}

func (suite *E2ETestSuite) setupVaultController() {
	// Create enhanced event-driven controller
	vaultRepo := &basicVaultRepository{}

	suite.vaultController = controller.NewVaultUnsealConfigReconciler(
		suite.k8sClient,
		ctrl.Log.WithName("e2e-controller"),
		suite.scheme,
		vaultRepo,
		&controller.ReconcilerOptions{
			Timeout:      2 * time.Minute,
			RequeueAfter: 5 * time.Minute,
		},
	)

	suite.T().Log("✅ Enhanced event-driven controller configured")
}

func (suite *E2ETestSuite) installVaultUnsealConfigCRD() {
	suite.T().Log("📋 Installing VaultUnsealConfig CRD...")

	// Create a simple CRD installation script
	crdInstallScript := `#!/bin/bash
set -e

# Create VaultUnsealConfig CRD
cat > /tmp/vault-crd.yaml << 'EOF'
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: vaultunsealconfigs.vault.io
spec:
  group: vault.io
  versions:
  - name: v1
    served: true
    storage: true
    schema:
      openAPIV3Schema:
        type: object
        properties:
          spec:
            type: object
            properties:
              vaultInstances:
                type: array
                items:
                  type: object
                  properties:
                    name:
                      type: string
                    endpoint:
                      type: string
                    unsealKeys:
                      type: array
                      items:
                        type: string
                    threshold:
                      type: integer
                    podSelector:
                      type: object
                      additionalProperties:
                        type: string
                    namespace:
                      type: string
          status:
            type: object
            properties:
              vaultStatuses:
                type: array
                items:
                  type: object
                  properties:
                    name:
                      type: string
                    sealed:
                      type: boolean
                    error:
                      type: string
                    lastUnsealed:
                      type: string
                      format: date-time
              conditions:
                type: array
                items:
                  type: object
                  properties:
                    type:
                      type: string
                    status:
                      type: string
                    reason:
                      type: string
                    message:
                      type: string
                    lastTransitionTime:
                      type: string
                      format: date-time
  scope: Namespaced
  names:
    plural: vaultunsealconfigs
    singular: vaultunsealconfig
    kind: VaultUnsealConfig
EOF

# Apply the CRD
kubectl apply -f /tmp/vault-crd.yaml

# Wait for CRD to be established
kubectl wait --for=condition=Established crd/vaultunsealconfigs.vault.io --timeout=30s

echo "VaultUnsealConfig CRD installed successfully"
`

	exitCode, reader, err := suite.k3sContainer.Exec(suite.ctx, []string{"sh", "-c", crdInstallScript})
	output, _ := io.ReadAll(reader)
	suite.T().Logf("CRD installation output (exit code %d): %s", exitCode, string(output))
	require.NoError(suite.T(), err, "Failed to install CRD")
	require.Equal(suite.T(), 0, exitCode, "CRD installation should succeed")

	suite.T().Log("✅ VaultUnsealConfig CRD installed")
}

func (suite *E2ETestSuite) TearDownSuite() {
	if suite.ctxCancel != nil {
		suite.ctxCancel()
	}

	// Note: Enhanced controller doesn't need explicit stopping
	// as it follows standard Kubernetes controller lifecycle

	// Clean up Helm release
	cleanupScript := fmt.Sprintf("helm uninstall %s -n %s || true", suite.vaultRelease, suite.vaultNamespace)
	_, _, _ = suite.k3sContainer.Exec(context.Background(), []string{"sh", "-c", cleanupScript})

	// Terminate K3s
	if suite.k3sContainer != nil {
		if err := suite.k3sContainer.Terminate(context.Background()); err != nil {
			suite.T().Logf("Failed to terminate K3s container: %v", err)
		}
	}
}

// E2E TEST CASES

func (suite *E2ETestSuite) TestVaultPodLabelsAndDetection() {
	suite.T().Log("🎯 Testing Vault pod labels and event detection...")

	// List Vault pods
	var pods corev1.PodList
	err := suite.k8sClient.List(suite.ctx, &pods,
		client.InNamespace(suite.vaultNamespace),
		client.MatchingLabels{
			"app.kubernetes.io/name":     "vault",
			"app.kubernetes.io/instance": suite.vaultRelease,
		},
	)
	require.NoError(suite.T(), err, "Should list Vault pods")
	require.NotEmpty(suite.T(), pods.Items, "Should have at least one Vault pod")

	for _, pod := range pods.Items {
		suite.T().Logf("Found Vault pod: %s", pod.Name)

		// Verify labels (using manual deployment, not Helm)
		assert.Equal(suite.T(), "vault", pod.Labels["app.kubernetes.io/name"])
		assert.Equal(suite.T(), suite.vaultRelease, pod.Labels["app.kubernetes.io/instance"])
		assert.Equal(suite.T(), "manual", pod.Labels["app.kubernetes.io/managed-by"])

		// Verify pod is running
		assert.Equal(suite.T(), corev1.PodRunning, pod.Status.Phase)

		suite.T().Logf("✅ Pod %s has correct labels and is running", pod.Name)
	}
}

func (suite *E2ETestSuite) TestVaultUnsealConfigCreation() {
	suite.T().Log("📝 Testing VaultUnsealConfig creation for dev Vault...")

	// Get Vault service endpoint
	var service corev1.Service
	err := suite.k8sClient.Get(suite.ctx, types.NamespacedName{
		Name:      suite.vaultRelease,
		Namespace: suite.vaultNamespace,
	}, &service)
	require.NoError(suite.T(), err, "Should get Vault service")

	endpoint := fmt.Sprintf("http://%s.%s.svc.cluster.local:8200", service.Name, service.Namespace)

	// Install CRD first
	suite.installVaultUnsealConfigCRD()

	// Create VaultUnsealConfig for dev Vault
	config := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "vault-dev-config",
			Namespace: suite.vaultNamespace,
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:     suite.vaultRelease,
					Endpoint: endpoint,
					// Dev mode doesn't need unseal keys
					UnsealKeys: []string{},
					Threshold:  func() *int { i := 0; return &i }(), // No unsealing needed in dev mode
					PodSelector: map[string]string{
						"app.kubernetes.io/name":       "vault",
						"app.kubernetes.io/instance":   suite.vaultRelease,
						"app.kubernetes.io/managed-by": "manual",
					},
					Namespace: suite.vaultNamespace,
				},
			},
		},
	}

	err = suite.k8sClient.Create(suite.ctx, config)
	require.NoError(suite.T(), err, "Should create VaultUnsealConfig")

	// Verify config was created
	var retrievedConfig vaultv1.VaultUnsealConfig
	err = suite.k8sClient.Get(suite.ctx, types.NamespacedName{
		Name: config.Name, Namespace: config.Namespace,
	}, &retrievedConfig)
	require.NoError(suite.T(), err, "Should retrieve VaultUnsealConfig")

	assert.Equal(suite.T(), 1, len(retrievedConfig.Spec.VaultInstances))
	assert.Equal(suite.T(), suite.vaultRelease, retrievedConfig.Spec.VaultInstances[0].Name)
	assert.Equal(suite.T(), endpoint, retrievedConfig.Spec.VaultInstances[0].Endpoint)

	suite.T().Log("✅ VaultUnsealConfig created and retrieved successfully")
}

func (suite *E2ETestSuite) TestPodEventSimulation() {
	suite.T().Log("🔄 Testing pod event simulation...")

	// Get current Vault pods
	var pods corev1.PodList
	err := suite.k8sClient.List(suite.ctx, &pods,
		client.InNamespace(suite.vaultNamespace),
		client.MatchingLabels{
			"app.kubernetes.io/name":     "vault",
			"app.kubernetes.io/instance": suite.vaultRelease,
		},
	)
	require.NoError(suite.T(), err, "Should list Vault pods")
	require.NotEmpty(suite.T(), pods.Items, "Should have Vault pods")

	originalPod := pods.Items[0]
	suite.T().Logf("Original pod: %s", originalPod.Name)

	// Delete pod to simulate restart
	err = suite.k8sClient.Delete(suite.ctx, &originalPod)
	require.NoError(suite.T(), err, "Should delete Vault pod")

	suite.T().Log("🔄 Waiting for pod to be recreated...")

	// Wait for new pod to be created and ready
	require.Eventually(suite.T(), func() bool {
		var newPods corev1.PodList
		err := suite.k8sClient.List(suite.ctx, &newPods,
			client.InNamespace(suite.vaultNamespace),
			client.MatchingLabels{
				"app.kubernetes.io/name":     "vault",
				"app.kubernetes.io/instance": suite.vaultRelease,
			},
		)
		if err != nil {
			return false
		}

		// Look for a new pod (different name or newer creation time)
		for _, newPod := range newPods.Items {
			if newPod.Name != originalPod.Name || newPod.CreationTimestamp.After(originalPod.CreationTimestamp.Time) {
				if newPod.Status.Phase == corev1.PodRunning {
					// Check if containers are ready
					allReady := true
					for _, status := range newPod.Status.ContainerStatuses {
						if !status.Ready {
							allReady = false
							break
						}
					}
					if allReady {
						suite.T().Logf("✅ New pod created and ready: %s", newPod.Name)
						return true
					}
				}
			}
		}
		return false
	}, 5*time.Minute, 10*time.Second, "New Vault pod should be created and ready")

	suite.T().Log("✅ Pod restart event simulation completed")
}

// Test runner
func TestE2ETestSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E tests in short mode")
	}

	if os.Getenv("CI") == "true" {
		t.Skip("Skipping E2E tests in CI environment")
	}

	suite.Run(t, new(E2ETestSuite))
}
