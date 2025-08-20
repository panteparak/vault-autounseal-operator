package common

import (
	"context"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/k3s"
	"github.com/testcontainers/testcontainers-go/modules/vault"
	"github.com/testcontainers/testcontainers-go/wait"
)

// ContainerManager manages TestContainers lifecycle
type ContainerManager struct {
	ctx            context.Context
	k3sContainer   *k3s.K3sContainer
	vaultContainer *vault.VaultContainer
	vaultAddr      string
	cleanupFuncs   []func()
}

// NewContainerManager creates a new container manager
func NewContainerManager(ctx context.Context) *ContainerManager {
	return &ContainerManager{
		ctx:          ctx,
		cleanupFuncs: make([]func(), 0),
	}
}

// StartK3s starts a K3s container with default configuration
func (cm *ContainerManager) StartK3s() (*k3s.K3sContainer, error) {
	k3sContainer, err := k3s.Run(cm.ctx,
		"rancher/k3s:v1.32.1-k3s1",
		testcontainers.WithWaitStrategy(
			wait.ForLog("k3s is up and running").
				WithStartupTimeout(180*time.Second).
				WithPollInterval(5*time.Second),
		),
	)
	if err != nil {
		return nil, err
	}

	cm.k3sContainer = k3sContainer
	cm.addCleanup(func() {
		k3sContainer.Terminate(context.Background())
	})

	return k3sContainer, nil
}

// StartVault starts a Vault container with default configuration
func (cm *ContainerManager) StartVault(token string) (*vault.VaultContainer, string, error) {
	vaultContainer, err := vault.Run(cm.ctx,
		"hashicorp/vault:1.19.0",
		vault.WithToken(token),
		testcontainers.WithWaitStrategy(
			wait.ForHTTP("/v1/sys/health").
				WithStatusCodeMatcher(func(status int) bool {
					return status == 200 || status == 429
				}).
				WithStartupTimeout(90*time.Second),
		),
	)
	if err != nil {
		return nil, "", err
	}

	vaultAddr, err := vaultContainer.HttpHostAddress(cm.ctx)
	if err != nil {
		vaultContainer.Terminate(cm.ctx)
		return nil, "", err
	}

	cm.vaultContainer = vaultContainer
	cm.vaultAddr = vaultAddr
	cm.addCleanup(func() {
		vaultContainer.Terminate(context.Background())
	})

	return vaultContainer, vaultAddr, nil
}

// StartVaultWithConfig starts Vault with custom configuration
func (cm *ContainerManager) StartVaultWithConfig(token string, opts ...testcontainers.ContainerCustomizer) (*vault.VaultContainer, string, error) {
	options := []testcontainers.ContainerCustomizer{
		vault.WithToken(token),
		testcontainers.WithWaitStrategy(
			wait.ForHTTP("/v1/sys/health").
				WithStatusCodeMatcher(func(status int) bool {
					return status == 200 || status == 429
				}).
				WithStartupTimeout(120*time.Second),
		),
	}
	options = append(options, opts...)

	vaultContainer, err := vault.Run(cm.ctx, "hashicorp/vault:1.19.0", options...)
	if err != nil {
		return nil, "", err
	}

	vaultAddr, err := vaultContainer.HttpHostAddress(cm.ctx)
	if err != nil {
		vaultContainer.Terminate(cm.ctx)
		return nil, "", err
	}

	cm.addCleanup(func() {
		vaultContainer.Terminate(context.Background())
	})

	return vaultContainer, vaultAddr, nil
}

// GetK3s returns the managed K3s container
func (cm *ContainerManager) GetK3s() *k3s.K3sContainer {
	return cm.k3sContainer
}

// GetVault returns the managed Vault container and address
func (cm *ContainerManager) GetVault() (*vault.VaultContainer, string) {
	return cm.vaultContainer, cm.vaultAddr
}

// addCleanup adds a cleanup function
func (cm *ContainerManager) addCleanup(cleanup func()) {
	cm.cleanupFuncs = append(cm.cleanupFuncs, cleanup)
}

// Cleanup performs cleanup of all managed containers
func (cm *ContainerManager) Cleanup() {
	for i := len(cm.cleanupFuncs) - 1; i >= 0; i-- {
		cm.cleanupFuncs[i]()
	}
	cm.cleanupFuncs = nil
}
