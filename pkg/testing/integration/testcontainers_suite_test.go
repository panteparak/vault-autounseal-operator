//go:build integration

package integration

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/panteparak/vault-autounseal-operator/pkg/unsealing/client"
)

const (
	vaultImage      = "hashicorp/vault:1.20.0"
	vaultPort       = "8200/tcp"
	defaultTimeout  = 30 * time.Second
	healthcheckPath = "/v1/sys/health"
)

// VaultTestSuite is the main test suite for Vault operations
type VaultTestSuite struct {
	suite.Suite
	ctx context.Context
}

// VaultContainer wraps a testcontainer with Vault-specific functionality
type VaultContainer struct {
	Container    testcontainers.Container
	Endpoint     string
	RootToken    string
	UnsealKeys   []string
	Port         int
	IsDevMode    bool
	InstanceName string
}

// VaultStatus represents Vault's seal status
type VaultStatus struct {
	Sealed      bool   `json:"sealed"`
	Initialized bool   `json:"initialized"`
	Progress    int    `json:"progress"`
	Threshold   int    `json:"threshold"`
	Version     string `json:"version"`
}

// VaultInitResponse represents Vault initialization response
type VaultInitResponse struct {
	Keys       []string `json:"keys"`
	KeysBase64 []string `json:"keys_base64"`
	RootToken  string   `json:"root_token"`
}

// SetupSuite runs once before all tests
func (suite *VaultTestSuite) SetupSuite() {
	suite.ctx = context.Background()
	suite.T().Log("🚀 Setting up Vault Integration Test Suite")
}

// TearDownSuite runs once after all tests
func (suite *VaultTestSuite) TearDownSuite() {
	suite.T().Log("🧹 Tearing down Vault Integration Test Suite")
}

// createDevVault creates a Vault container in development mode
func (suite *VaultTestSuite) createDevVault(name, rootToken string) *VaultContainer {
	t := suite.T()

	req := testcontainers.ContainerRequest{
		Image:        vaultImage,
		ExposedPorts: []string{vaultPort},
		Env: map[string]string{
			"VAULT_DEV_ROOT_TOKEN_ID":   rootToken,
			"VAULT_DEV_LISTEN_ADDRESS": "0.0.0.0:8200",
			"VAULT_LOG_LEVEL":          "warn",
		},
		Cmd: []string{"vault", "server", "-dev"},
		WaitingFor: wait.ForHTTP(healthcheckPath).
			WithPort(vaultPort).
			WithStartupTimeout(defaultTimeout).
			WithPollInterval(500 * time.Millisecond).
			WithStatusCodeMatcher(func(status int) bool {
				return status == http.StatusOK || status == http.StatusServiceUnavailable
			}),
		Name: fmt.Sprintf("vault-dev-%s-%d", name, time.Now().Unix()),
	}

	container, err := testcontainers.GenericContainer(suite.ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
		Reuse:           false, // Ensure fresh containers for each test
	})
	require.NoError(t, err, "Failed to start dev Vault container")

	host, err := container.Host(suite.ctx)
	require.NoError(t, err)

	port, err := container.MappedPort(suite.ctx, vaultPort)
	require.NoError(t, err)

	portInt, _ := strconv.Atoi(port.Port())
	endpoint := fmt.Sprintf("http://%s:%s", host, port.Port())

	vault := &VaultContainer{
		Container:    container,
		Endpoint:     endpoint,
		RootToken:    rootToken,
		Port:         portInt,
		IsDevMode:    true,
		InstanceName: name,
	}

	t.Logf("✅ Created dev Vault '%s' at %s", name, endpoint)
	return vault
}

// createSealedVault creates a Vault container in production mode (sealed)
func (suite *VaultTestSuite) createSealedVault(name string) *VaultContainer {
	t := suite.T()

	vaultConfig := `{
		"backend": {"file": {"path": "/vault/file"}},
		"listener": {"tcp": {"address": "0.0.0.0:8200", "tls_disable": true}},
		"disable_mlock": true,
		"log_level": "warn"
	}`

	req := testcontainers.ContainerRequest{
		Image:        vaultImage,
		ExposedPorts: []string{vaultPort},
		Env: map[string]string{
			"VAULT_LOCAL_CONFIG": vaultConfig,
		},
		Cmd: []string{"vault", "server", "-config=/vault/config"},
		WaitingFor: wait.ForListeningPort(vaultPort).
			WithStartupTimeout(defaultTimeout).
			WithPollInterval(500 * time.Millisecond),
		Name: fmt.Sprintf("vault-sealed-%s-%d", name, time.Now().Unix()),
	}

	container, err := testcontainers.GenericContainer(suite.ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
		Reuse:           false, // Ensure fresh containers for each test
	})
	require.NoError(t, err, "Failed to start sealed Vault container")

	host, err := container.Host(suite.ctx)
	require.NoError(t, err)

	port, err := container.MappedPort(suite.ctx, vaultPort)
	require.NoError(t, err)

	portInt, _ := strconv.Atoi(port.Port())
	endpoint := fmt.Sprintf("http://%s:%s", host, port.Port())

	vault := &VaultContainer{
		Container:    container,
		Endpoint:     endpoint,
		Port:         portInt,
		IsDevMode:    false,
		InstanceName: name,
	}

	// Wait for Vault to be ready
	time.Sleep(3 * time.Second)

	// Initialize the sealed vault
	suite.initializeVault(vault)

	t.Logf("✅ Created sealed Vault '%s' at %s", name, endpoint)
	return vault
}

// initializeVault initializes a sealed Vault and extracts keys
func (suite *VaultTestSuite) initializeVault(vault *VaultContainer) {
	t := suite.T()

	initPayload := strings.NewReader(`{"secret_shares": 3, "secret_threshold": 3}`)

	httpClient := &http.Client{Timeout: 10 * time.Second}
	resp, err := httpClient.Post(
		fmt.Sprintf("%s/v1/sys/init", vault.Endpoint),
		"application/json",
		initPayload,
	)

	if err != nil {
		t.Logf("❌ Failed to initialize vault: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Logf("❌ Vault init failed with status %d: %s", resp.StatusCode, string(body))
		return
	}

	var initResp VaultInitResponse
	if err := json.NewDecoder(resp.Body).Decode(&initResp); err != nil {
		t.Logf("❌ Failed to decode init response: %v", err)
		return
	}

	vault.UnsealKeys = initResp.KeysBase64
	vault.RootToken = initResp.RootToken

	t.Logf("🔑 Vault '%s' initialized with %d unseal keys", vault.InstanceName, len(vault.UnsealKeys))
}

// getVaultStatus retrieves the current Vault status
func (suite *VaultTestSuite) getVaultStatus(vault *VaultContainer) (*VaultStatus, error) {
	httpClient := &http.Client{Timeout: 5 * time.Second}
	resp, err := httpClient.Get(fmt.Sprintf("%s/v1/sys/seal-status", vault.Endpoint))
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var status VaultStatus
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return nil, err
	}

	return &status, nil
}

// unsealVault unseals a Vault using the provided base64 key
func (suite *VaultTestSuite) unsealVault(vault *VaultContainer, base64Key string) error {
	payload := strings.NewReader(fmt.Sprintf(`{"key": "%s"}`, base64Key))

	httpClient := &http.Client{Timeout: 5 * time.Second}
	resp, err := httpClient.Post(
		fmt.Sprintf("%s/v1/sys/unseal", vault.Endpoint),
		"application/json",
		payload,
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("unseal failed with status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

// cleanupVault terminates a Vault container with proper cleanup
func (suite *VaultTestSuite) cleanupVault(vault *VaultContainer) {
	if vault != nil && vault.Container != nil {
		timeout := 10 * time.Second

		// Graceful stop first
		if err := vault.Container.Stop(suite.ctx, &timeout); err != nil {
			suite.T().Logf("⚠️ Warning: Failed to stop Vault '%s': %v", vault.InstanceName, err)
		}

		// Force terminate if needed
		if err := vault.Container.Terminate(suite.ctx); err != nil {
			suite.T().Logf("⚠️ Warning: Failed to terminate Vault '%s': %v", vault.InstanceName, err)
		} else {
			suite.T().Logf("🧹 Cleaned up Vault '%s'", vault.InstanceName)
		}
	}
}

// TestBasicVaultOperations tests basic Vault functionality
func (suite *VaultTestSuite) TestBasicVaultOperations() {
	suite.T().Log("🧪 Testing basic Vault operations")

	vault := suite.createDevVault("basic", "test-root-token")
	defer suite.cleanupVault(vault)

	// Test health check
	status, err := suite.getVaultStatus(vault)
	require.NoError(suite.T(), err, "Should get Vault status")
	assert.False(suite.T(), status.Sealed, "Dev vault should not be sealed")
	assert.True(suite.T(), status.Initialized, "Dev vault should be initialized")

	// Test client creation and connectivity
	vaultClient, err := client.NewClient(vault.Endpoint, true, 5*time.Second)
	require.NoError(suite.T(), err, "Should create Vault client")
	assert.NotNil(suite.T(), vaultClient, "Client should not be nil")

	// Test basic connectivity
	healthResp, err := vaultClient.HealthCheck(suite.ctx)
	require.NoError(suite.T(), err, "Should check health")
	assert.NotNil(suite.T(), healthResp, "Health response should not be nil")
}

// TestSealedVaultOperations tests sealed Vault operations
func (suite *VaultTestSuite) TestSealedVaultOperations() {
	suite.T().Log("🔒 Testing sealed Vault operations")

	vault := suite.createSealedVault("sealed")
	defer suite.cleanupVault(vault)

	// Verify initial sealed state
	status, err := suite.getVaultStatus(vault)
	require.NoError(suite.T(), err, "Should get Vault status")
	assert.True(suite.T(), status.Sealed, "Vault should be sealed initially")
	assert.True(suite.T(), status.Initialized, "Vault should be initialized")

	// Test manual unsealing (we know we have 3 keys and threshold 3)
	for i, key := range vault.UnsealKeys {
		err := suite.unsealVault(vault, key)
		require.NoError(suite.T(), err, "Unseal operation %d should succeed", i+1)

		// Check current status after each key
		currentStatus, err := suite.getVaultStatus(vault)
		require.NoError(suite.T(), err, "Should get status after key %d", i+1)

		suite.T().Logf("After key %d: sealed=%v, progress=%d", i+1, currentStatus.Sealed, currentStatus.Progress)

		// Break if unsealed
		if !currentStatus.Sealed {
			suite.T().Logf("✅ Vault unsealed after %d keys", i+1)
			break
		}

		// Avoid infinite loop - we expect to be unsealed after 3 keys
		if i+1 >= 3 {
			break
		}
	}

	// Verify vault is unsealed
	finalStatus, err := suite.getVaultStatus(vault)
	require.NoError(suite.T(), err, "Should get final status")
	assert.False(suite.T(), finalStatus.Sealed, "Vault should be unsealed after providing threshold keys")
}

// TestClientUnsealOperations tests unsealing through our client
func (suite *VaultTestSuite) TestClientUnsealOperations() {
	suite.T().Log("🔧 Testing client unseal operations")

	vault := suite.createSealedVault("client-unseal")
	defer suite.cleanupVault(vault)

	vaultClient, err := client.NewClient(vault.Endpoint, true, 5*time.Second)
	require.NoError(suite.T(), err, "Should create client")

	// Use the base64 keys directly as strings
	unsealKeys := vault.UnsealKeys

	// Test unsealing through client (use threshold of 3)
	_, err = vaultClient.Unseal(suite.ctx, unsealKeys, 3)
	require.NoError(suite.T(), err, "Client should unseal Vault successfully")

	// Verify vault is unsealed and healthy
	healthResp, err := vaultClient.HealthCheck(suite.ctx)
	require.NoError(suite.T(), err, "Should check health after unseal")
	assert.NotNil(suite.T(), healthResp, "Health response should not be nil after unsealing")
}

// TestPerformanceUnderLoad tests Vault operations under load
func (suite *VaultTestSuite) TestPerformanceUnderLoad() {
	if testing.Short() {
		suite.T().Skip("Skipping performance test in short mode")
	}

	suite.T().Log("⚡ Testing performance under load")

	vault := suite.createDevVault("performance", "perf-token")
	defer suite.cleanupVault(vault)

	vaultClient, err := client.NewClient(vault.Endpoint, true, 2*time.Second)
	require.NoError(suite.T(), err, "Should create client")

	// Test concurrent health checks
	const goroutines = 10
	const checksPerGoroutine = 5

	results := make(chan bool, goroutines*checksPerGoroutine)

	for i := 0; i < goroutines; i++ {
		go func() {
			for j := 0; j < checksPerGoroutine; j++ {
				healthResp, err := vaultClient.HealthCheck(suite.ctx)
				results <- (err == nil && healthResp != nil)
			}
		}()
	}

	successCount := 0
	for i := 0; i < goroutines*checksPerGoroutine; i++ {
		if <-results {
			successCount++
		}
	}

	successRate := float64(successCount) / float64(goroutines*checksPerGoroutine)
	assert.GreaterOrEqual(suite.T(), successRate, 0.95, "Success rate should be at least 95%%")

	suite.T().Logf("📊 Performance test: %d/%d successful (%.1f%%)",
		successCount, goroutines*checksPerGoroutine, successRate*100)
}

// TestVaultUnsealConfigScenario tests a scenario similar to VaultUnsealConfig CRD
func (suite *VaultTestSuite) TestVaultUnsealConfigScenario() {
	suite.T().Log("📝 Testing VaultUnsealConfig-like scenario")

	vault := suite.createSealedVault("unseal-config")
	defer suite.cleanupVault(vault)

	// Simulate VaultUnsealConfig spec with base64 encoded keys
	var base64Keys []string
	for _, key := range vault.UnsealKeys {
		// Re-encode to simulate base64 keys from CRD
		decodedKey, err := base64.StdEncoding.DecodeString(key)
		require.NoError(suite.T(), err, "Should decode key")

		reEncodedKey := base64.StdEncoding.EncodeToString(decodedKey)
		base64Keys = append(base64Keys, reEncodedKey)
	}

	vaultClient, err := client.NewClient(vault.Endpoint, true, 5*time.Second)
	require.NoError(suite.T(), err, "Should create client")

	// Use the base64 keys directly as strings (as the client expects)
	unsealKeys := base64Keys

	// Unseal the vault (as operator would do)
	_, err = vaultClient.Unseal(suite.ctx, unsealKeys, 3)
	require.NoError(suite.T(), err, "Should unseal Vault with CRD-style keys")

	// Verify unsealed
	status, err := suite.getVaultStatus(vault)
	require.NoError(suite.T(), err, "Should get status after unseal")
	assert.False(suite.T(), status.Sealed, "Vault should be unsealed")

	// Verify health through client
	healthResp, err := vaultClient.HealthCheck(suite.ctx)
	require.NoError(suite.T(), err, "Should check health")
	assert.NotNil(suite.T(), healthResp, "Health response should not be nil")
}

// TestInSuite runs the test suite
func TestVaultIntegrationSuite(t *testing.T) {
	suite.Run(t, new(VaultTestSuite))
}
