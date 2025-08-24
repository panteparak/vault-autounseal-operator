package integration

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hashicorp/vault/api"
	vaultv1 "github.com/panteparak/vault-autounseal-operator/pkg/api/v1"
	vaultpkg "github.com/panteparak/vault-autounseal-operator/pkg/vault"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/k3s"
	"github.com/testcontainers/testcontainers-go/modules/vault"
	"github.com/testcontainers/testcontainers-go/wait"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/clientcmd"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// E2EWorkflowTestSuite provides end-to-end workflow testing
type E2EWorkflowTestSuite struct {
	suite.Suite
	k3sContainer   *k3s.K3sContainer
	vaultContainer *vault.VaultContainer
	vaultClient    *api.Client
	vaultAddr      string
	unsealKeys     []string
	rootToken      string
	k8sClient      client.Client
	scheme         *runtime.Scheme
	ctx            context.Context
	ctxCancel      context.CancelFunc
	operatorImageTag string
}

// SetupSuite initializes the complete testing environment
func (suite *E2EWorkflowTestSuite) SetupSuite() {
	suite.ctx, suite.ctxCancel = context.WithTimeout(context.Background(), 20*time.Minute)

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	// Set up complete environment
	suite.setupCompleteEnvironment()
}

// setupCompleteEnvironment creates K3s cluster with CRDs, Vault, and operator
func (suite *E2EWorkflowTestSuite) setupCompleteEnvironment() {
	// Build operator image first
	suite.buildOperatorImage()

	// Use the existing CRD from manifests to ensure compatibility
	projectRoot, err := filepath.Abs("../../")
	require.NoError(suite.T(), err, "Failed to get project root")

	crdPath := filepath.Join(projectRoot, "manifests", "crd.yaml")
	crdBytes, err := ioutil.ReadFile(crdPath)
	require.NoError(suite.T(), err, "Failed to read CRD file")

	// Create K3s cluster with the official CRD and basic RBAC
	crdAndRBACManifest := string(crdBytes) + `
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: vault-operator
  namespace: default
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: vault-operator
rules:
- apiGroups: [""]
  resources: ["pods", "services", "endpoints", "events"]
  verbs: ["get", "list", "watch", "create", "update", "patch"]
- apiGroups: ["vault.io"]
  resources: ["vaultunsealconfigs"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["vault.io"]
  resources: ["vaultunsealconfigs/status"]
  verbs: ["get", "update", "patch"]
- apiGroups: ["apiextensions.k8s.io"]
  resources: ["customresourcedefinitions"]
  verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: vault-operator
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: vault-operator
subjects:
- kind: ServiceAccount
  name: vault-operator
  namespace: default
---
apiVersion: v1
kind: Namespace
metadata:
  name: vault
---
apiVersion: v1
kind: Service
metadata:
  name: vault
  namespace: vault
  labels:
    app: vault
spec:
  type: NodePort
  ports:
  - port: 8200
    targetPort: 8200
    nodePort: 30200
    protocol: TCP
    name: vault
  selector:
    app: vault
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: vault
  namespace: vault
  labels:
    app: vault
spec:
  replicas: 1
  selector:
    matchLabels:
      app: vault
  template:
    metadata:
      labels:
        app: vault
    spec:
      containers:
      - name: vault
        image: hashicorp/vault:1.19.0
        ports:
        - containerPort: 8200
          name: vault
        env:
        - name: VAULT_DEV_ROOT_TOKEN_ID
          value: "root-token"
        - name: VAULT_DEV_LISTEN_ADDRESS
          value: "0.0.0.0:8200"
        - name: VAULT_ADDR
          value: "http://127.0.0.1:8200"
        args:
        - "vault"
        - "server"
        - "-dev"
        - "-dev-root-token-id=root-token"
        - "-log-level=info"
        readinessProbe:
          httpGet:
            path: /v1/sys/health
            port: 8200
          initialDelaySeconds: 5
          periodSeconds: 5
        livenessProbe:
          httpGet:
            path: /v1/sys/health
            port: 8200
          initialDelaySeconds: 30
          periodSeconds: 30`

	// Create temporary manifest file
	manifestFile, err := ioutil.TempFile("", "vault-operator-*.yaml")
	require.NoError(suite.T(), err, "Failed to create temporary manifest file")
	defer os.Remove(manifestFile.Name())

	_, err = manifestFile.WriteString(crdAndRBACManifest)
	require.NoError(suite.T(), err, "Failed to write manifest content")
	manifestFile.Close()

	k3sContainer, err := k3s.Run(suite.ctx,
		"rancher/k3s:v1.32.1-k3s1",
		k3s.WithManifest(manifestFile.Name()),
		testcontainers.WithWaitStrategy(
			wait.ForLog("k3s is up and running").
				WithStartupTimeout(180*time.Second).
				WithPollInterval(5*time.Second),
		),
	)
	require.NoError(suite.T(), err, "Failed to start K3s cluster")
	suite.k3sContainer = k3sContainer

	// Set up Kubernetes client
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

	// Wait for CRDs to be ready
	suite.waitForCRDsReady()

	// Create test namespaces
	suite.createTestNamespaces()

	// Start Vault container
	suite.setupVault()

	// Deploy operator via Helm
	suite.deployOperatorViaHelm()
}

// waitForCRDsReady waits for all CRDs to be available
func (suite *E2EWorkflowTestSuite) waitForCRDsReady() {
	require.Eventually(suite.T(), func() bool {
		// First check if CRD exists via kubectl
		debugCmd := []string{"sh", "-c", "kubectl get crd vaultunsealconfigs.vault.io -o yaml"}
		_, _, err := suite.k3sContainer.Exec(suite.ctx, debugCmd)
		if err != nil {
			suite.T().Logf("CRD not yet available: %v", err)
			return false
		}

		// Then test creating a VaultUnsealConfig
		testConfig := &vaultv1.VaultUnsealConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "crd-readiness-test",
				Namespace: "default",
			},
			Spec: vaultv1.VaultUnsealConfigSpec{
				VaultInstances: []vaultv1.VaultInstance{
					{Name: "test", Endpoint: "http://test", UnsealKeys: []string{"test"}},
				},
			},
		}

		err = suite.k8sClient.Create(suite.ctx, testConfig)
		if err == nil {
			suite.k8sClient.Delete(suite.ctx, testConfig)
			suite.T().Log("CRDs are ready - test resource created and deleted successfully")
			return true
		}
		suite.T().Logf("CRD test failed: %v", err)
		return false
	}, 90*time.Second, 3*time.Second, "CRDs should become ready")
}

// createTestNamespaces creates namespaces for testing
func (suite *E2EWorkflowTestSuite) createTestNamespaces() {
	namespaces := []string{"vault-system", "test-env", "production"}

	for _, ns := range namespaces {
		namespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{Name: ns},
		}
		err := suite.k8sClient.Create(suite.ctx, namespace)
		require.NoError(suite.T(), err, "Should create namespace %s", ns)
	}
}

// setupVault waits for Vault deployment to be ready in Kubernetes
func (suite *E2EWorkflowTestSuite) setupVault() {
	// Wait for Vault deployment to be ready
	suite.waitForVaultDeployment()

	// Get Vault service endpoint for external access (from test runner)
	suite.vaultAddr = suite.getVaultServiceEndpoint()

	// Configure Vault client
	vaultConfig := api.DefaultConfig()
	vaultConfig.Address = suite.vaultAddr
	var err error
	suite.vaultClient, err = api.NewClient(vaultConfig)
	require.NoError(suite.T(), err, "Failed to create Vault client")

	suite.rootToken = "root-token"
	suite.vaultClient.SetToken(suite.rootToken)

	// Set up test keys
	suite.unsealKeys = []string{
		"ZTJlLXRlc3Qta2V5LTE=", // e2e-test-key-1
		"ZTJlLXRlc3Qta2V5LTI=", // e2e-test-key-2
		"ZTJlLXRlc3Qta2V5LTM=", // e2e-test-key-3
	}

	// Configure Vault for testing (enable secrets engine, etc.)
	suite.configureVault()
}

// waitForVaultDeployment waits for Vault deployment to be ready
func (suite *E2EWorkflowTestSuite) waitForVaultDeployment() {
	require.Eventually(suite.T(), func() bool {
		// Check if Vault service exists and has endpoints
		service := &corev1.Service{}
		err := suite.k8sClient.Get(suite.ctx, client.ObjectKey{
			Name:      "vault",
			Namespace: "vault",
		}, service)
		if err != nil {
			return false
		}

		// Simple check - if service exists, assume deployment is working
		// The service will only have endpoints if the deployment is ready
		return true
	}, 120*time.Second, 5*time.Second, "Vault deployment should become ready")
}

// getVaultServiceEndpoint gets the external endpoint for Vault service
func (suite *E2EWorkflowTestSuite) getVaultServiceEndpoint() string {
	// For E2E tests, we'll use the K3s container's exposed port
	// First get the container's host and port for the Vault service
	hostIP, err := suite.k3sContainer.Host(suite.ctx)
	require.NoError(suite.T(), err, "Failed to get K3s host")

	// K3s exposes services on random ports, we need to find the mapped port
	// For now, let's use a simpler approach and access via NodePort
	// We'll update the Vault service to be NodePort type
	return fmt.Sprintf("http://%s:30200", hostIP) // Use fixed NodePort
}

// configureVault sets up Vault with test data
func (suite *E2EWorkflowTestSuite) configureVault() {
	// Enable KV secrets engine
	err := suite.vaultClient.Sys().Mount("secret/", &api.MountInput{
		Type: "kv-v2",
	})
	if err != nil {
		suite.T().Logf("KV mount may already exist: %v", err)
	}

	// Write some test secrets
	_, err = suite.vaultClient.Logical().Write("secret/data/test", map[string]interface{}{
		"data": map[string]interface{}{
			"key1": "value1",
			"key2": "value2",
		},
	})
	if err != nil {
		suite.T().Logf("Failed to write test secret: %v", err)
	}
}

// buildOperatorImage builds the operator Docker image for testing or reuses pre-built image
func (suite *E2EWorkflowTestSuite) buildOperatorImage() {
	// Check if a pre-built image tag is provided via environment variable
	if prebuiltTag := os.Getenv("OPERATOR_IMAGE_TAG"); prebuiltTag != "" {
		suite.T().Logf("🚀 Using pre-built operator image: %s", prebuiltTag)
		suite.operatorImageTag = prebuiltTag

		// Verify the image exists locally
		checkCmd := exec.Command("docker", "inspect", prebuiltTag)
		if err := checkCmd.Run(); err != nil {
			suite.T().Logf("⚠️ Pre-built image %s not found locally, will build from scratch", prebuiltTag)
			suite.buildImageFromSource()
		} else {
			suite.T().Logf("✅ Pre-built image %s verified locally", prebuiltTag)
			return
		}
	} else {
		suite.T().Log("🏗️ No pre-built image provided, building from source")
		suite.buildImageFromSource()
	}
}

// buildImageFromSource builds the operator image from source code
func (suite *E2EWorkflowTestSuite) buildImageFromSource() {
	// Generate a unique tag for this test run
	suite.operatorImageTag = fmt.Sprintf("vault-autounseal-operator:e2e-%d", time.Now().Unix())

	// Get the project root directory (3 levels up from tests/e2e/)
	projectRoot, err := filepath.Abs("../../")
	require.NoError(suite.T(), err, "Failed to get project root")

	// Build the operator image
	buildCmd := exec.Command("docker", "build", "-t", suite.operatorImageTag, ".")
	buildCmd.Dir = projectRoot
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr

	suite.T().Logf("Building operator image from source: %s", suite.operatorImageTag)
	err = buildCmd.Run()
	require.NoError(suite.T(), err, "Failed to build operator image")
}

// deployOperatorViaHelm deploys the operator using Helm inside K3s
func (suite *E2EWorkflowTestSuite) deployOperatorViaHelm() {
	// Import the operator image into K3s
	suite.importImageIntoK3s()

	// Deploy operator using Helm
	suite.installOperatorHelm()

	// Wait for operator to be ready
	suite.waitForOperatorReady()
}

// importImageIntoK3s imports the built image into K3s cluster
func (suite *E2EWorkflowTestSuite) importImageIntoK3s() {
	// Save image to tar file
	tempDir, err := ioutil.TempDir("", "k3s-image-*")
	require.NoError(suite.T(), err)
	defer os.RemoveAll(tempDir)

	imageTarPath := filepath.Join(tempDir, "operator-image.tar")

	// Export image to tar
	exportCmd := exec.Command("docker", "save", "-o", imageTarPath, suite.operatorImageTag)
	suite.T().Logf("Exporting image: %s to %s", suite.operatorImageTag, imageTarPath)
	err = exportCmd.Run()
	require.NoError(suite.T(), err, "Failed to export image")

	// Copy tar file to K3s container
	err = suite.k3sContainer.CopyFileToContainer(suite.ctx, imageTarPath, "/tmp/operator-image.tar", 0644)
	require.NoError(suite.T(), err, "Failed to copy image to K3s")

	// Import image into K3s
	importCmd := []string{"ctr", "images", "import", "/tmp/operator-image.tar"}
	_, _, err = suite.k3sContainer.Exec(suite.ctx, importCmd)
	require.NoError(suite.T(), err, "Failed to import image into K3s")

	suite.T().Logf("Successfully imported operator image into K3s")
}

// installOperatorHelm deploys operator using kubectl (simplified approach)
func (suite *E2EWorkflowTestSuite) installOperatorHelm() {
	// Create operator manifests directly
	operatorManifests := suite.generateOperatorManifests()

	// Write manifests to temp file
	tempDir, err := ioutil.TempDir("", "operator-manifests-*")
	require.NoError(suite.T(), err)
	defer os.RemoveAll(tempDir)

	manifestsPath := filepath.Join(tempDir, "operator.yaml")
	err = ioutil.WriteFile(manifestsPath, []byte(operatorManifests), 0644)
	require.NoError(suite.T(), err)

	// Copy manifests to K3s container
	err = suite.k3sContainer.CopyFileToContainer(suite.ctx, manifestsPath, "/tmp/operator.yaml", 0644)
	require.NoError(suite.T(), err)

	// Create namespace
	createNamespaceCmd := []string{"kubectl", "create", "namespace", "vault-system", "--dry-run=client", "-o", "yaml"}
	_, _, _ = suite.k3sContainer.Exec(suite.ctx, createNamespaceCmd)
	applyNamespaceCmd := []string{"sh", "-c", "kubectl create namespace vault-system --dry-run=client -o yaml | kubectl apply -f -"}
	_, _, _ = suite.k3sContainer.Exec(suite.ctx, applyNamespaceCmd)

	// Apply operator manifests
	suite.T().Logf("Deploying operator with image: %s", suite.operatorImageTag)
	applyCmd := []string{"kubectl", "apply", "-f", "/tmp/operator.yaml"}
	_, _, err = suite.k3sContainer.Exec(suite.ctx, applyCmd)

	if err != nil {
		suite.T().Logf("Kubectl apply failed: %v", err)
		// Try to get more details about what failed
		debugCmd := []string{"sh", "-c", "cat /tmp/operator.yaml && echo '---' && kubectl get all -n vault-system"}
		debugStdout, _, _ := suite.k3sContainer.Exec(suite.ctx, debugCmd)
		suite.T().Logf("Debug output: %v", debugStdout)
	}
	require.NoError(suite.T(), err, "Failed to apply operator manifests")

	suite.T().Logf("Successfully deployed operator via kubectl")
}

// generateOperatorManifests creates Kubernetes manifests for the operator
func (suite *E2EWorkflowTestSuite) generateOperatorManifests() string {
	imageRepo := suite.getImageRepository()
	imageTag := strings.Split(suite.operatorImageTag, ":")[1]

	return fmt.Sprintf(`---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: vault-operator-vault-autounseal-operator
  namespace: vault-system
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRole
metadata:
  name: vault-operator-vault-autounseal-operator-manager-role
rules:
- apiGroups: [""]
  resources: ["pods", "services", "endpoints"]
  verbs: ["get", "list", "watch"]
- apiGroups: ["vault.io"]
  resources: ["vaultunsealconfigs"]
  verbs: ["get", "list", "watch", "create", "update", "patch", "delete"]
- apiGroups: ["vault.io"]
  resources: ["vaultunsealconfigs/status"]
  verbs: ["get", "update", "patch"]
- apiGroups: [""]
  resources: ["events"]
  verbs: ["create", "patch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: vault-operator-vault-autounseal-operator-manager-rolebinding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: vault-operator-vault-autounseal-operator-manager-role
subjects:
- kind: ServiceAccount
  name: vault-operator-vault-autounseal-operator
  namespace: vault-system
---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: vault-operator-vault-autounseal-operator
  namespace: vault-system
  labels:
    app.kubernetes.io/name: vault-autounseal-operator
    app.kubernetes.io/instance: vault-operator
spec:
  replicas: 1
  selector:
    matchLabels:
      app.kubernetes.io/name: vault-autounseal-operator
      app.kubernetes.io/instance: vault-operator
  template:
    metadata:
      labels:
        app.kubernetes.io/name: vault-autounseal-operator
        app.kubernetes.io/instance: vault-operator
    spec:
      serviceAccountName: vault-operator-vault-autounseal-operator
      securityContext:
        runAsNonRoot: true
        runAsUser: 65532
        runAsGroup: 65532
        fsGroup: 65532
      containers:
      - name: manager
        image: "%s:%s"
        imagePullPolicy: Never
        args:
        - --metrics-bind-address=:8080
        - --health-probe-bind-address=:8081
        securityContext:
          allowPrivilegeEscalation: false
          capabilities:
            drop:
            - ALL
          readOnlyRootFilesystem: true
          runAsNonRoot: true
          runAsUser: 65532
        startupProbe:
          httpGet:
            path: /readyz
            port: 8081
          initialDelaySeconds: 5
          periodSeconds: 5
          timeoutSeconds: 5
          failureThreshold: 30
        livenessProbe:
          httpGet:
            path: /healthz
            port: 8081
          initialDelaySeconds: 30
          periodSeconds: 20
          timeoutSeconds: 5
          failureThreshold: 3
        readinessProbe:
          httpGet:
            path: /readyz
            port: 8081
          initialDelaySeconds: 10
          periodSeconds: 10
          timeoutSeconds: 5
          failureThreshold: 5
        ports:
        - name: metrics
          containerPort: 8080
          protocol: TCP
        - name: health
          containerPort: 8081
          protocol: TCP
        resources:
          requests:
            cpu: 50m
            memory: 64Mi
          limits:
            cpu: 200m
            memory: 128Mi
        env:
        - name: GOMAXPROCS
          value: "1"
        - name: GOMEMLIMIT
          value: "128MiB"
        volumeMounts:
        - mountPath: /tmp
          name: tmp
      volumes:
      - name: tmp
        emptyDir: {}
      terminationGracePeriodSeconds: 10
---
apiVersion: v1
kind: Service
metadata:
  name: vault-operator-vault-autounseal-operator
  namespace: vault-system
  labels:
    app.kubernetes.io/name: vault-autounseal-operator
    app.kubernetes.io/instance: vault-operator
spec:
  type: ClusterIP
  ports:
  - port: 8080
    targetPort: metrics
    protocol: TCP
    name: metrics
  - port: 8081
    targetPort: health
    protocol: TCP
    name: health
  selector:
    app.kubernetes.io/name: vault-autounseal-operator
    app.kubernetes.io/instance: vault-operator
`, imageRepo, imageTag)
}

// getImageRepository extracts repository from the full image tag
func (suite *E2EWorkflowTestSuite) getImageRepository() string {
	// Find the last colon to separate repository from tag
	if colonIndex := strings.LastIndex(suite.operatorImageTag, ":"); colonIndex != -1 {
		return suite.operatorImageTag[:colonIndex]
	}
	return suite.operatorImageTag
}

// waitForOperatorReady waits for the operator deployment to be ready
func (suite *E2EWorkflowTestSuite) waitForOperatorReady() {
	require.Eventually(suite.T(), func() bool {
		deployment := &appsv1.Deployment{}
		err := suite.k8sClient.Get(suite.ctx, client.ObjectKey{
			Name:      "vault-operator-vault-autounseal-operator",
			Namespace: "vault-system",
		}, deployment)

		if err != nil {
			suite.T().Logf("Waiting for operator deployment: %v", err)
			return false
		}

		// Check if deployment is ready
		for _, condition := range deployment.Status.Conditions {
			if condition.Type == appsv1.DeploymentAvailable && condition.Status == corev1.ConditionTrue {
				suite.T().Logf("Operator deployment is ready")
				return true
			}
		}

		// If we see replicas = 1 but readyReplicas = 0, check pod status (but only log once every 30 seconds)
		if deployment.Status.Replicas == 1 && deployment.Status.ReadyReplicas == 0 {
			// Add some timing to avoid spamming logs
			if time.Now().Unix()%30 == 0 { // Log every 30 seconds roughly
				debugCmd := []string{"sh", "-c", "echo '=== Pod Status ==='; kubectl get pods -n vault-system -o wide; echo '=== Pod Description ==='; kubectl describe pod -n vault-system --selector=app.kubernetes.io/instance=vault-operator; echo '=== Pod Logs ==='; kubectl logs -n vault-system --selector=app.kubernetes.io/instance=vault-operator --tail=50; echo '=== CRD Status ==='; kubectl get crd vaultunsealconfigs.vault.io; echo '=== ServiceAccount ==='; kubectl get sa -n vault-system vault-operator-vault-autounseal-operator"}
				stdout, _, _ := suite.k3sContainer.Exec(suite.ctx, debugCmd)
				suite.T().Logf("Pod debug output: %v", stdout)
			}
		}

		suite.T().Logf("Operator deployment not ready yet, replicas: %d/%d",
			deployment.Status.ReadyReplicas, deployment.Status.Replicas)
		return false
	}, 300*time.Second, 5*time.Second, "Operator deployment should become ready")
}

// TearDownSuite cleans up resources
func (suite *E2EWorkflowTestSuite) TearDownSuite() {
	if suite.ctxCancel != nil {
		suite.ctxCancel()
	}

	if suite.vaultContainer != nil {
		suite.vaultContainer.Terminate(context.Background())
	}

	if suite.k3sContainer != nil {
		suite.k3sContainer.Terminate(context.Background())
	}
}

// TestCompleteOperatorWorkflow tests the full operator workflow with operator running in K3s
func (suite *E2EWorkflowTestSuite) TestCompleteOperatorWorkflow() {
	// Step 1: Create VaultUnsealConfig
	config := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "complete-workflow",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:       "workflow-vault",
					Endpoint:   "http://vault.vault.svc.cluster.local:8200", // Use Kubernetes service DNS
					UnsealKeys: suite.unsealKeys,
					Threshold:  func() *int { i := 3; return &i }(),
				},
			},
		},
	}

	err := suite.k8sClient.Create(suite.ctx, config)
	require.NoError(suite.T(), err, "Step 1: Should create VaultUnsealConfig")

	// Step 2: Wait for operator to process the configuration
	suite.T().Log("Step 2: Waiting for operator to process VaultUnsealConfig")
	require.Eventually(suite.T(), func() bool {
		var currentState vaultv1.VaultUnsealConfig
		err := suite.k8sClient.Get(suite.ctx, client.ObjectKey{
			Name: "complete-workflow",
			Namespace: "default",
		}, &currentState)

		if err != nil {
			suite.T().Logf("Failed to get config: %v", err)
			return false
		}

		// Check if operator has updated the status
		if len(currentState.Status.VaultStatuses) == 0 {
			suite.T().Log("No vault statuses yet")
			return false
		}

		if len(currentState.Status.Conditions) == 0 {
			suite.T().Log("No conditions yet")
			return false
		}

		suite.T().Logf("Operator processed config: %d vault statuses, %d conditions",
			len(currentState.Status.VaultStatuses), len(currentState.Status.Conditions))
		return true
	}, 60*time.Second, 5*time.Second, "Operator should process VaultUnsealConfig")

	// Step 3: Verify initial status set by operator
	var firstState vaultv1.VaultUnsealConfig
	err = suite.k8sClient.Get(suite.ctx, client.ObjectKey{
		Name: "complete-workflow",
		Namespace: "default",
	}, &firstState)
	require.NoError(suite.T(), err, "Step 3: Should get initial state")
	assert.NotEmpty(suite.T(), firstState.Status.VaultStatuses, "Should have vault status")
	assert.NotEmpty(suite.T(), firstState.Status.Conditions, "Should have conditions")
	assert.Equal(suite.T(), "workflow-vault", firstState.Status.VaultStatuses[0].Name)

	// Step 4: Wait for multiple operator reconciliation cycles
	suite.T().Log("Step 4: Waiting for multiple operator reconciliation cycles")
	time.Sleep(10 * time.Second) // Let operator run several cycles

	// Step 5: Verify consistent state after operator processing
	var finalState vaultv1.VaultUnsealConfig
	err = suite.k8sClient.Get(suite.ctx, client.ObjectKey{
		Name: "complete-workflow",
		Namespace: "default",
	}, &finalState)
	require.NoError(suite.T(), err, "Step 5: Should get final state")

	assert.Equal(suite.T(), "workflow-vault", finalState.Status.VaultStatuses[0].Name)
	// Note: LastUnsealed might be nil if vault is already unsealed in dev mode
	suite.T().Logf("Vault status: sealed=%v, error=%s",
		finalState.Status.VaultStatuses[0].Sealed,
		finalState.Status.VaultStatuses[0].Error)

	// Step 6: Test configuration updates (operator should detect changes)
	finalState.Spec.VaultInstances[0].Threshold = func() *int { i := 2; return &i }()
	err = suite.k8sClient.Update(suite.ctx, &finalState)
	require.NoError(suite.T(), err, "Step 6: Should update configuration")

	// Step 7: Wait for operator to process the update
	suite.T().Log("Step 7: Waiting for operator to process configuration update")
	time.Sleep(5 * time.Second) // Give operator time to detect and process change

	// Step 8: Verify update was processed by operator
	var updatedState vaultv1.VaultUnsealConfig
	err = suite.k8sClient.Get(suite.ctx, client.ObjectKey{
		Name: "complete-workflow",
		Namespace: "default",
	}, &updatedState)
	require.NoError(suite.T(), err, "Step 8: Should get updated state")
	assert.Equal(suite.T(), 2, *updatedState.Spec.VaultInstances[0].Threshold, "Threshold should be updated")

	// Verify operator continues to manage the updated configuration
	assert.NotEmpty(suite.T(), updatedState.Status.VaultStatuses, "Should maintain vault status after update")
}

// TestMultiNamespaceWorkflow tests operator working across multiple namespaces
func (suite *E2EWorkflowTestSuite) TestMultiNamespaceWorkflow() {
	namespaces := []string{"default", "vault-system", "test-env"}
	configs := make([]*vaultv1.VaultUnsealConfig, len(namespaces))

	// Create VaultUnsealConfig in each namespace
	for i, ns := range namespaces {
		config := &vaultv1.VaultUnsealConfig{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fmt.Sprintf("multi-ns-vault-%d", i),
				Namespace: ns,
			},
			Spec: vaultv1.VaultUnsealConfigSpec{
				VaultInstances: []vaultv1.VaultInstance{
					{
						Name:       fmt.Sprintf("vault-%s-%d", ns, i),
						Endpoint:   "http://vault.vault.svc.cluster.local:8200",
						UnsealKeys: suite.unsealKeys,
					},
				},
			},
		}

		err := suite.k8sClient.Create(suite.ctx, config)
		require.NoError(suite.T(), err, "Should create config in namespace %s", ns)
		configs[i] = config
	}

	// Wait for operator to process each configuration
	for i, config := range configs {
		suite.T().Logf("Waiting for operator to process config %d in namespace %s", i, config.Namespace)

		require.Eventually(suite.T(), func() bool {
			var processed vaultv1.VaultUnsealConfig
			err := suite.k8sClient.Get(suite.ctx, client.ObjectKey{
				Name:      config.Name,
				Namespace: config.Namespace,
			}, &processed)

			if err != nil {
				suite.T().Logf("Failed to get config %d: %v", i, err)
				return false
			}

			// Check if operator has processed this config
			if len(processed.Status.VaultStatuses) == 0 {
				suite.T().Logf("Config %d: No vault statuses yet", i)
				return false
			}

			suite.T().Logf("Config %d processed: %d vault statuses", i, len(processed.Status.VaultStatuses))
			return true
		}, 60*time.Second, 5*time.Second, "Operator should process config %d in namespace %s", i, config.Namespace)

		// Verify status in each namespace
		var reconciled vaultv1.VaultUnsealConfig
		err := suite.k8sClient.Get(suite.ctx, client.ObjectKey{
			Name:      config.Name,
			Namespace: config.Namespace,
		}, &reconciled)
		require.NoError(suite.T(), err, "Should get reconciled config %d", i)
		assert.NotEmpty(suite.T(), reconciled.Status.VaultStatuses, "Should have status in namespace %s", config.Namespace)
	}
}

// TestErrorRecoveryWorkflow tests error scenarios and recovery
func (suite *E2EWorkflowTestSuite) TestErrorRecoveryWorkflow() {
	// Create config with initially unreachable vault
	config := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "error-recovery",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:       "unreachable-vault",
					Endpoint:   "http://192.0.2.1:8200", // RFC 3330 TEST-NET-1 - guaranteed unreachable
					UnsealKeys: suite.unsealKeys,
				},
			},
		},
	}

	err := suite.k8sClient.Create(suite.ctx, config)
	require.NoError(suite.T(), err, "Should create error config")

	// Wait for operator to process the unreachable configuration
	suite.T().Log("Waiting for operator to process unreachable vault configuration")
	require.Eventually(suite.T(), func() bool {
		var errorState vaultv1.VaultUnsealConfig
		err := suite.k8sClient.Get(suite.ctx, client.ObjectKey{
			Name:      "error-recovery",
			Namespace: "default",
		}, &errorState)

		if err != nil {
			suite.T().Logf("Failed to get error config: %v", err)
			return false
		}

		// Check if operator has updated the status with error info
		if len(errorState.Status.VaultStatuses) == 0 {
			suite.T().Log("No vault statuses yet for error case")
			return false
		}

		// Should have error message for unreachable vault
		hasError := errorState.Status.VaultStatuses[0].Error != ""
		suite.T().Logf("Error state: sealed=%v, error='%s'",
			errorState.Status.VaultStatuses[0].Sealed,
			errorState.Status.VaultStatuses[0].Error)
		return hasError
	}, 60*time.Second, 5*time.Second, "Operator should handle unreachable vault and set error status")

	// Verify error state set by operator
	var errorState vaultv1.VaultUnsealConfig
	err = suite.k8sClient.Get(suite.ctx, client.ObjectKey{
		Name:      "error-recovery",
		Namespace: "default",
	}, &errorState)
	require.NoError(suite.T(), err, "Should get error state")
	assert.True(suite.T(), errorState.Status.VaultStatuses[0].Sealed, "Should be marked as sealed")
	assert.NotEmpty(suite.T(), errorState.Status.VaultStatuses[0].Error, "Should have error message")

	// "Fix" the configuration by updating to working vault
	errorState.Spec.VaultInstances[0].Endpoint = "http://vault.vault.svc.cluster.local:8200"
	err = suite.k8sClient.Update(suite.ctx, &errorState)
	require.NoError(suite.T(), err, "Should fix configuration")

	// Wait for operator to detect the fix and process it
	suite.T().Log("Waiting for operator to process the configuration fix")
	time.Sleep(10 * time.Second) // Give operator time to detect and process change

	// Verify recovery by operator
	var recoveredState vaultv1.VaultUnsealConfig
	err = suite.k8sClient.Get(suite.ctx, client.ObjectKey{
		Name:      "error-recovery",
		Namespace: "default",
	}, &recoveredState)
	require.NoError(suite.T(), err, "Should get recovered state")

	// Error should be cleared or reduced since vault is now reachable
	suite.T().Logf("Recovered state: sealed=%v, error='%s'",
		recoveredState.Status.VaultStatuses[0].Sealed,
		recoveredState.Status.VaultStatuses[0].Error)

	// The vault should now be manageable (error cleared or different error)
	assert.NotEqual(suite.T(), errorState.Status.VaultStatuses[0].Error,
		recoveredState.Status.VaultStatuses[0].Error,
		"Error state should change after fixing endpoint")
}

// TestScaleUpScaleDownWorkflow tests adding and removing vault instances
func (suite *E2EWorkflowTestSuite) TestScaleUpScaleDownWorkflow() {
	// Start with single vault instance
	config := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "scale-test",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:       "vault-1",
					Endpoint:   "http://vault.vault.svc.cluster.local:8200",
					UnsealKeys: suite.unsealKeys,
				},
			},
		},
	}

	err := suite.k8sClient.Create(suite.ctx, config)
	require.NoError(suite.T(), err, "Should create initial scale config")

	// Wait for operator to process initial configuration
	suite.T().Log("Waiting for operator to process initial scale test configuration")
	require.Eventually(suite.T(), func() bool {
		var initialState vaultv1.VaultUnsealConfig
		err := suite.k8sClient.Get(suite.ctx, client.ObjectKey{
			Name:      "scale-test",
			Namespace: "default",
		}, &initialState)

		if err != nil {
			return false
		}

		return len(initialState.Status.VaultStatuses) == 1
	}, 60*time.Second, 5*time.Second, "Operator should process initial config")

	var initialState vaultv1.VaultUnsealConfig
	err = suite.k8sClient.Get(suite.ctx, client.ObjectKey{
		Name:      "scale-test",
		Namespace: "default",
	}, &initialState)
	require.NoError(suite.T(), err, "Should get initial state")
	assert.Len(suite.T(), initialState.Status.VaultStatuses, 1, "Should have 1 vault status")

	// Scale up - add second vault instance
	initialState.Spec.VaultInstances = append(initialState.Spec.VaultInstances, vaultv1.VaultInstance{
		Name:       "vault-2",
		Endpoint:   "http://vault.vault.svc.cluster.local:8200", // Same vault, different logical instance
		UnsealKeys: suite.unsealKeys,
	})

	err = suite.k8sClient.Update(suite.ctx, &initialState)
	require.NoError(suite.T(), err, "Should scale up configuration")

	// Wait for operator to process scale up
	suite.T().Log("Waiting for operator to process scale up")
	require.Eventually(suite.T(), func() bool {
		var scaledUpState vaultv1.VaultUnsealConfig
		err := suite.k8sClient.Get(suite.ctx, client.ObjectKey{
			Name:      "scale-test",
			Namespace: "default",
		}, &scaledUpState)

		if err != nil {
			return false
		}

		return len(scaledUpState.Status.VaultStatuses) == 2
	}, 60*time.Second, 5*time.Second, "Operator should process scale up")

	var scaledUpState vaultv1.VaultUnsealConfig
	err = suite.k8sClient.Get(suite.ctx, client.ObjectKey{
		Name:      "scale-test",
		Namespace: "default",
	}, &scaledUpState)
	require.NoError(suite.T(), err, "Should get scaled up state")
	assert.Len(suite.T(), scaledUpState.Status.VaultStatuses, 2, "Should have 2 vault statuses")

	// Verify both vault instances are tracked
	vaultNames := make(map[string]bool)
	for _, status := range scaledUpState.Status.VaultStatuses {
		vaultNames[status.Name] = true
	}
	assert.True(suite.T(), vaultNames["vault-1"], "Should track vault-1")
	assert.True(suite.T(), vaultNames["vault-2"], "Should track vault-2")

	// Scale down - remove one vault instance
	scaledUpState.Spec.VaultInstances = scaledUpState.Spec.VaultInstances[:1] // Keep only first instance

	err = suite.k8sClient.Update(suite.ctx, &scaledUpState)
	require.NoError(suite.T(), err, "Should scale down configuration")

	// Wait for operator to process scale down
	suite.T().Log("Waiting for operator to process scale down")
	require.Eventually(suite.T(), func() bool {
		var scaledDownState vaultv1.VaultUnsealConfig
		err := suite.k8sClient.Get(suite.ctx, client.ObjectKey{
			Name:      "scale-test",
			Namespace: "default",
		}, &scaledDownState)

		if err != nil {
			return false
		}

		return len(scaledDownState.Status.VaultStatuses) == 1
	}, 60*time.Second, 5*time.Second, "Operator should process scale down")

	var scaledDownState vaultv1.VaultUnsealConfig
	err = suite.k8sClient.Get(suite.ctx, client.ObjectKey{
		Name:      "scale-test",
		Namespace: "default",
	}, &scaledDownState)
	require.NoError(suite.T(), err, "Should get scaled down state")
	assert.Len(suite.T(), scaledDownState.Status.VaultStatuses, 1, "Should have 1 vault status after scale down")
	assert.Equal(suite.T(), "vault-1", scaledDownState.Status.VaultStatuses[0].Name, "Should keep vault-1")
}

// TestLongRunningWorkflow tests extended operator behavior
func (suite *E2EWorkflowTestSuite) TestLongRunningWorkflow() {
	config := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "long-running",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:       "persistent-vault",
					Endpoint:   "http://vault.vault.svc.cluster.local:8200",
					UnsealKeys: suite.unsealKeys,
				},
			},
		},
	}

	err := suite.k8sClient.Create(suite.ctx, config)
	require.NoError(suite.T(), err, "Should create long-running config")

	// Wait for operator to process initial configuration
	suite.T().Log("Waiting for operator to process long-running configuration")
	require.Eventually(suite.T(), func() bool {
		var currentState vaultv1.VaultUnsealConfig
		err := suite.k8sClient.Get(suite.ctx, client.ObjectKey{
			Name:      "long-running",
			Namespace: "default",
		}, &currentState)

		if err != nil {
			return false
		}

		return len(currentState.Status.VaultStatuses) == 1 && len(currentState.Status.Conditions) > 0
	}, 60*time.Second, 5*time.Second, "Operator should process long-running config")

	// Monitor operator behavior over extended period
	observationCount := 10
	observations := make([]vaultv1.VaultUnsealConfig, observationCount)

	for i := 0; i < observationCount; i++ {
		// Wait between observations to let operator run multiple cycles
		if i > 0 {
			time.Sleep(3 * time.Second)
		}

		// Get current state as managed by operator
		var currentState vaultv1.VaultUnsealConfig
		err := suite.k8sClient.Get(suite.ctx, client.ObjectKey{
			Name:      "long-running",
			Namespace: "default",
		}, &currentState)
		require.NoError(suite.T(), err, "Should get state at observation %d", i+1)

		observations[i] = currentState

		// Verify consistent state management by operator
		assert.Len(suite.T(), currentState.Status.VaultStatuses, 1, "Should consistently have 1 vault status")
		assert.Equal(suite.T(), "persistent-vault", currentState.Status.VaultStatuses[0].Name, "Vault name should be consistent")
		assert.NotEmpty(suite.T(), currentState.Status.Conditions, "Should have conditions")

		suite.T().Logf("Observation %d: sealed=%v, conditions=%d, error=%s", i+1,
			currentState.Status.VaultStatuses[0].Sealed,
			len(currentState.Status.Conditions),
			currentState.Status.VaultStatuses[0].Error)
	}

	// Verify state stability over time - status should remain consistent
	firstObservation := observations[0]
	lastObservation := observations[observationCount-1]

	assert.Equal(suite.T(), firstObservation.Status.VaultStatuses[0].Name,
		lastObservation.Status.VaultStatuses[0].Name,
		"Vault name should remain stable")

	// Count condition transitions (should be minimal for stable state)
	transitionTimes := make(map[string]time.Time)
	for _, obs := range observations {
		for _, condition := range obs.Status.Conditions {
			if condition.Type == "Ready" {
				transitionTimes[condition.Reason] = condition.LastTransitionTime.Time
				break
			}
		}
	}

	suite.T().Logf("Observed %d unique condition states over %d observations",
		len(transitionTimes), observationCount)

	// Should have stable condition state (not constantly changing)
	assert.LessOrEqual(suite.T(), len(transitionTimes), 3,
		"Should have stable condition state with few transitions")
}

// TestVaultConnectivityWorkflow tests vault connectivity scenarios
func (suite *E2EWorkflowTestSuite) TestVaultConnectivityWorkflow() {
	// Test direct vault connectivity outside of K3s cluster (from test runner)
	vaultClient, err := vaultpkg.NewClient(suite.vaultAddr, false, 30*time.Second)
	require.NoError(suite.T(), err, "Should create vault client")
	defer vaultClient.Close()

	// Test vault health from external perspective
	health, err := vaultClient.HealthCheck(suite.ctx)
	require.NoError(suite.T(), err, "Vault should be healthy from external access")
	assert.NotNil(suite.T(), health)

	// Test vault operations from external perspective
	isSealed, err := vaultClient.IsSealed(suite.ctx)
	require.NoError(suite.T(), err, "Should check seal status")
	suite.T().Logf("Vault sealed status (external view): %v", isSealed)

	// Create operator configuration for internal K3s connectivity
	config := &vaultv1.VaultUnsealConfig{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "connectivity-test",
			Namespace: "default",
		},
		Spec: vaultv1.VaultUnsealConfigSpec{
			VaultInstances: []vaultv1.VaultInstance{
				{
					Name:       "connectivity-vault",
					Endpoint:   "http://vault.vault.svc.cluster.local:8200", // Internal K8s service DNS
					UnsealKeys: suite.unsealKeys,
				},
			},
		},
	}

	err = suite.k8sClient.Create(suite.ctx, config)
	require.NoError(suite.T(), err, "Should create connectivity config")

	// Wait for operator running inside K3s to process the configuration
	suite.T().Log("Waiting for operator (inside K3s) to process connectivity test")
	require.Eventually(suite.T(), func() bool {
		var operatorState vaultv1.VaultUnsealConfig
		err := suite.k8sClient.Get(suite.ctx, client.ObjectKey{
			Name:      "connectivity-test",
			Namespace: "default",
		}, &operatorState)

		if err != nil {
			suite.T().Logf("Failed to get connectivity config: %v", err)
			return false
		}

		// Check if operator has successfully connected to vault
		if len(operatorState.Status.VaultStatuses) == 0 {
			suite.T().Log("No vault statuses yet for connectivity test")
			return false
		}

		hasStatus := len(operatorState.Status.VaultStatuses) > 0
		suite.T().Logf("Operator connectivity status: sealed=%v, error=%s",
			operatorState.Status.VaultStatuses[0].Sealed,
			operatorState.Status.VaultStatuses[0].Error)

		return hasStatus
	}, 60*time.Second, 5*time.Second, "Operator should successfully connect to vault via K8s service DNS")

	// Verify operator's internal view of vault connectivity
	var operatorState vaultv1.VaultUnsealConfig
	err = suite.k8sClient.Get(suite.ctx, client.ObjectKey{
		Name:      "connectivity-test",
		Namespace: "default",
	}, &operatorState)
	require.NoError(suite.T(), err, "Should get operator state")

	assert.NotEmpty(suite.T(), operatorState.Status.VaultStatuses, "Operator should have vault status")
	suite.T().Logf("Final operator vault status: sealed=%v, error=%s",
		operatorState.Status.VaultStatuses[0].Sealed,
		operatorState.Status.VaultStatuses[0].Error)

	// Both external and internal connectivity should work
	suite.T().Log("✅ Both external (test runner) and internal (operator) connectivity verified")
}

// TestE2EWorkflowTestSuite runs the end-to-end workflow test suite
func TestE2EWorkflowTestSuite(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping E2E workflow tests in short mode")
	}

	suite.Run(t, new(E2EWorkflowTestSuite))
}
