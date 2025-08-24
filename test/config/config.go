package config

import (
	"fmt"
	"time"
)

// Config holds test configuration for the vault-autounseal-operator tests
type Config struct {
	// K3s configuration
	K3sImage                string
	K3sVersion              string
	StartupTimeout          time.Duration
	ReadinessPollInterval   time.Duration

	// Vault configuration
	VaultImage              string
	VaultVersion            string

	// Test retry configuration
	RetryBackoff            time.Duration
	MaxBackoff              time.Duration
}

var globalConfig *Config

// GetGlobalConfig returns the global test configuration
func GetGlobalConfig() (*Config, error) {
	if globalConfig == nil {
		globalConfig = &Config{
			K3sVersion:              "v1.30.8-k3s1",
			StartupTimeout:          2 * time.Minute,
			ReadinessPollInterval:   2 * time.Second,
			VaultVersion:            "1.19.0",
			RetryBackoff:            500 * time.Millisecond,
			MaxBackoff:              30 * time.Second,
		}
	}
	return globalConfig, nil
}

// GetK3sImage returns the K3s container image name
func (c *Config) GetK3sImage() string {
	if c.K3sImage != "" {
		return c.K3sImage
	}
	return "rancher/k3s:" + c.K3sVersion
}

// GetK3sImageForVersion returns the K3s container image for a specific version
func (c *Config) GetK3sImageForVersion(version string) string {
	return "rancher/k3s:" + version
}

// GetVaultImage returns the Vault container image name
func (c *Config) GetVaultImage() string {
	if c.VaultImage != "" {
		return c.VaultImage
	}
	return "vault:" + c.VaultVersion
}

// GetVaultImageForVersion returns the Vault container image for a specific version
func (c *Config) GetVaultImageForVersion(version string) string {
	return "vault:" + version
}

// Validate validates the test configuration
func (c *Config) Validate() error {
	// Basic validation - all configurations are optional with defaults
	return nil
}

// String returns a string representation of the configuration
func (c *Config) String() string {
	return fmt.Sprintf("Config{K3sVersion:%s, VaultVersion:%s, StartupTimeout:%s, RetryBackoff:%s}",
		c.K3sVersion, c.VaultVersion, c.StartupTimeout, c.RetryBackoff)
}
