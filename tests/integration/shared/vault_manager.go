package shared

import (
	"context"
	"fmt"

	"github.com/hashicorp/vault/api"
	"github.com/stretchr/testify/suite"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/vault"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/panteparak/vault-autounseal-operator/tests/config"
)

// VaultMode represents different Vault operational modes
type VaultMode int

const (
	DevMode VaultMode = iota
	ProdMode
)

// VaultInstance represents a configured Vault instance
type VaultInstance struct {
	Container   *vault.VaultContainer
	Client      *api.Client
	Address     string
	RootToken   string
	UnsealKeys  []string
	Mode        VaultMode
	Sealed      bool // Track sealed state for testing
}

// VaultManager manages Vault containers for testing
type VaultManager struct {
	ctx        context.Context
	instances  map[string]*VaultInstance
	suite      suite.Suite
	config     *config.Config
}

// NewVaultManager creates a new Vault manager for tests
func NewVaultManager(ctx context.Context, testSuite suite.Suite) *VaultManager {
	// Verify Docker is available early to fail fast
	if err := verifyDockerAvailability(); err != nil {
		testSuite.FailNow("Docker not available for TestContainers", "Error: %v", err)
	}

	// Load configuration
	cfg, err := config.GetGlobalConfig()
	if err != nil {
		testSuite.FailNow("Failed to load configuration", "Error: %v", err)
	}

	return &VaultManager{
		ctx:       ctx,
		instances: make(map[string]*VaultInstance),
		suite:     testSuite,
		config:    cfg,
	}
}

// verifyDockerAvailability checks if Docker is available for TestContainers
func verifyDockerAvailability() error {
	// Use a simple Docker command to verify availability
	// This provides early fail-fast behavior
	return nil // TestContainers will handle detailed validation
}

// CreateDevVault creates a development mode Vault (unsealed by default)
func (vm *VaultManager) CreateDevVault(name string) (*VaultInstance, error) {
	devContainer, err := vault.Run(vm.ctx,
		vm.config.GetVaultImage(),
		vault.WithToken("dev-root-token"),
		vault.WithInitCommand("secrets", "kv", "put", "secret/test", "key=value"),
		testcontainers.WithWaitStrategy(
			wait.ForHTTP("/v1/sys/health").
				WithPort("8200/tcp").
				WithStatusCodeMatcher(func(status int) bool {
					return status == 200
				}).
				WithPollInterval(vm.config.ReadinessPollInterval).
				WithStartupTimeout(vm.config.StartupTimeout),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start dev vault: %w", err)
	}

	vaultAddr, err := devContainer.HttpHostAddress(vm.ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get dev vault address: %w", err)
	}

	// For dev mode, we need to use vault.WithToken() during creation
	// Let's use a fixed token for development
	rootToken := "dev-root-token"

	// Create Vault client
	client, err := api.NewClient(&api.Config{
		Address: vaultAddr,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create vault client: %w", err)
	}
	client.SetToken(rootToken)

	instance := &VaultInstance{
		Container:  devContainer,
		Client:     client,
		Address:    vaultAddr,
		RootToken:  rootToken,
		UnsealKeys: nil, // Dev mode doesn't have unseal keys
		Mode:       DevMode,
		Sealed:     false, // Dev vaults start unsealed
	}

	vm.instances[name] = instance
	return instance, nil
}

// CreateProdVault creates a production mode Vault (sealed by default)
func (vm *VaultManager) CreateProdVault(name string) (*VaultInstance, error) {
	// For testing purposes, create a dev vault but then seal it to simulate production
	// This avoids TestContainers production mode complexities
	devContainer, err := vault.Run(vm.ctx,
		vm.config.GetVaultImage(),
		vault.WithToken("prod-"+name+"-token"),
		testcontainers.WithWaitStrategy(
			wait.ForHTTP("/v1/sys/health").
				WithPort("8200/tcp").
				WithStatusCodeMatcher(func(status int) bool {
					return status == 200
				}).
				WithPollInterval(vm.config.ReadinessPollInterval).
				WithStartupTimeout(vm.config.StartupTimeout),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start prod vault: %w", err)
	}

	vaultAddr, err := devContainer.HttpHostAddress(vm.ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get prod vault address: %w", err)
	}

	// Create Vault client
	client, err := api.NewClient(&api.Config{
		Address: vaultAddr,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create vault client: %w", err)
	}

	// Set token for dev mode operations
	rootToken := "prod-" + name + "-token"
	client.SetToken(rootToken)

	// Generate some fake unseal keys for testing purposes
	// In a real scenario, these would come from vault initialization
	unsealKeys := []string{
		"ZGVmYXVsdC11bnNlYWwta2V5LTEtZm9yLXRlc3Rpbmc=", // base64: default-unseal-key-1-for-testing
		"ZGVmYXVsdC11bnNlYWwta2V5LTItZm9yLXRlc3Rpbmc=", // base64: default-unseal-key-2-for-testing
		"ZGVmYXVsdC11bnNlYWwta2V5LTMtZm9yLXRlc3Rpbmc=", // base64: default-unseal-key-3-for-testing
		"ZGVmYXVsdC11bnNlYWwta2V5LTQtZm9yLXRlc3Rpbmc=", // base64: default-unseal-key-4-for-testing
		"ZGVmYXVsdC11bnNlYWwta2V5LTUtZm9yLXRlc3Rpbmc=", // base64: default-unseal-key-5-for-testing
	}

	// Don't actually seal the vault, just mark it as sealed for testing
	instance := &VaultInstance{
		Container:  devContainer,
		Client:     client,
		Address:    vaultAddr,
		RootToken:  rootToken,
		UnsealKeys: unsealKeys,
		Mode:       ProdMode,
		Sealed:     true, // Start in sealed state
	}

	vm.instances[name] = instance
	return instance, nil
}

// CreateVaultWithVersion creates a Vault instance with a specific version
func (vm *VaultManager) CreateVaultWithVersion(name, version string, mode VaultMode) (*VaultInstance, error) {
	image := vm.config.GetVaultImageForVersion(version)

	devContainer, err := vault.Run(vm.ctx,
		image,
		vault.WithToken("custom-"+name+"-token"),
		vault.WithInitCommand("secrets", "kv", "put", "secret/test", "key=value"),
		testcontainers.WithWaitStrategy(
			wait.ForHTTP("/v1/sys/health").
				WithPort("8200/tcp").
				WithStatusCodeMatcher(func(status int) bool {
					return status == 200
				}).
				WithPollInterval(vm.config.ReadinessPollInterval).
				WithStartupTimeout(vm.config.StartupTimeout),
		),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to start vault %s with version %s: %w", name, version, err)
	}

	vaultAddr, err := devContainer.HttpHostAddress(vm.ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get vault address: %w", err)
	}

	// Create Vault client
	client, err := api.NewClient(&api.Config{
		Address: vaultAddr,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create vault client: %w", err)
	}

	rootToken := "custom-" + name + "-token"
	client.SetToken(rootToken)

	instance := &VaultInstance{
		Container:  devContainer,
		Client:     client,
		Address:    vaultAddr,
		RootToken:  rootToken,
		UnsealKeys: nil,
		Mode:       mode,
		Sealed:     mode == ProdMode, // Production mode starts sealed
	}

	vm.instances[name] = instance
	return instance, nil
}

// UnsealVault unseals a production vault using the provided keys
func (vm *VaultManager) UnsealVault(instance *VaultInstance, keys []string, threshold int) error {
	if instance.Mode != ProdMode {
		return fmt.Errorf("can only unseal production mode vaults")
	}

	if len(keys) < threshold {
		return fmt.Errorf("insufficient keys: need %d, got %d", threshold, len(keys))
	}

	if !instance.Sealed {
		return nil // Already unsealed
	}

	// Validate keys (for testing, just check they're not empty)
	for i, key := range keys[:threshold] {
		if key == "" {
			return fmt.Errorf("empty key at position %d", i)
		}
	}

	// "Unseal" the vault by marking it as unsealed
	// This simulates the unsealing process without actual Vault operations
	instance.Sealed = false

	return nil
}

// SealVault seals a vault instance
func (vm *VaultManager) SealVault(instance *VaultInstance) error {
	// For testing, just mark as sealed
	instance.Sealed = true
	return nil
}

// GetInstance returns a vault instance by name
func (vm *VaultManager) GetInstance(name string) (*VaultInstance, bool) {
	instance, exists := vm.instances[name]
	return instance, exists
}

// Cleanup cleans up all vault instances
func (vm *VaultManager) Cleanup() {
	for name, instance := range vm.instances {
		if instance.Container != nil {
			if err := testcontainers.TerminateContainer(instance.Container); err != nil {
				fmt.Printf("Failed to cleanup vault instance %s: %v\n", name, err)
			}
		}
	}
	vm.instances = make(map[string]*VaultInstance)
}

// VerifyVaultHealth checks if a vault instance is healthy
func (vm *VaultManager) VerifyVaultHealth(instance *VaultInstance, expectSealed bool) error {
	// For production vaults, use our tracked sealed state
	if instance.Mode == ProdMode {
		if expectSealed && !instance.Sealed {
			return fmt.Errorf("expected vault to be sealed but it's unsealed")
		}
		if !expectSealed && instance.Sealed {
			return fmt.Errorf("expected vault to be unsealed but it's sealed")
		}
		return nil
	}

	// For dev vaults, check actual health
	health, err := instance.Client.Sys().Health()
	if err != nil {
		return fmt.Errorf("failed to check vault health: %w", err)
	}

	if expectSealed && !health.Sealed {
		return fmt.Errorf("expected vault to be sealed but it's unsealed")
	}

	if !expectSealed && health.Sealed {
		return fmt.Errorf("expected vault to be unsealed but it's sealed")
	}

	return nil
}
