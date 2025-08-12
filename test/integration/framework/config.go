package framework

import (
	"fmt"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// LoadTestConfig loads test configuration from a YAML file
func LoadTestConfig(configPath string) (*TestConfig, error) {
	// Use default config if no path provided
	if configPath == "" {
		return getDefaultTestConfig(), nil
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file %s: %w", configPath, err)
	}

	var config TestConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config file %s: %w", configPath, err)
	}

	// Apply defaults to any missing values
	applyDefaults(&config)

	// Validate configuration
	if err := validateConfig(&config); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	return &config, nil
}

// getDefaultTestConfig returns a default test configuration
func getDefaultTestConfig() *TestConfig {
	return &TestConfig{
		VaultVersion:  "1.20.0",
		TestScenarios: []string{"basic", "failover", "multi-vault"},
		Timeouts: TimeoutConfig{
			VaultStartup:   60 * time.Second,
			OperatorReady:  120 * time.Second,
			VaultUnseal:    30 * time.Second,
			StatusUpdate:   45 * time.Second,
			TestExecution:  300 * time.Second,
			CleanupTimeout: 60 * time.Second,
		},
		VaultConfig: VaultTestConfig{
			DevMode:          true,
			InitializeVaults: true,
			UnsealThreshold:  3,
			SecretShares:     3,
			Endpoints: map[string]string{
				"basic":      "http://localhost:8200",
				"primary":    "http://localhost:8200",
				"standby":    "http://localhost:8201",
				"finance":    "http://localhost:8200",
				"engineering": "http://localhost:8201",
				"operations": "http://localhost:8202",
			},
			TLSConfig: TLSConfig{
				SkipVerify: true,
			},
		},
		OperatorConfig: OperatorConfig{
			Image:    "vault-autounseal-operator",
			Tag:      "test",
			LogLevel: "debug",
			Resources: ResourceLimits{
				CPU:    "500m",
				Memory: "256Mi",
			},
			HelmValues: map[string]string{
				"image.pullPolicy": "Never",
				"operator.logLevel": "debug",
			},
		},
		TestSettings: TestSettings{
			Parallel:            false,
			MaxConcurrency:      3,
			FailFast:            false,
			VerboseLogging:      true,
			CollectLogs:         true,
			GenerateReports:     true,
			KeepResourcesOnFail: false,
		},
		Environment: map[string]string{
			"GO_VERSION": "1.21",
		},
	}
}

// applyDefaults fills in any missing configuration values with defaults
func applyDefaults(config *TestConfig) {
	defaults := getDefaultTestConfig()

	if config.VaultVersion == "" {
		config.VaultVersion = defaults.VaultVersion
	}

	if len(config.TestScenarios) == 0 {
		config.TestScenarios = defaults.TestScenarios
	}

	// Apply timeout defaults
	if config.Timeouts.VaultStartup == 0 {
		config.Timeouts.VaultStartup = defaults.Timeouts.VaultStartup
	}
	if config.Timeouts.OperatorReady == 0 {
		config.Timeouts.OperatorReady = defaults.Timeouts.OperatorReady
	}
	if config.Timeouts.VaultUnseal == 0 {
		config.Timeouts.VaultUnseal = defaults.Timeouts.VaultUnseal
	}
	if config.Timeouts.StatusUpdate == 0 {
		config.Timeouts.StatusUpdate = defaults.Timeouts.StatusUpdate
	}
	if config.Timeouts.TestExecution == 0 {
		config.Timeouts.TestExecution = defaults.Timeouts.TestExecution
	}
	if config.Timeouts.CleanupTimeout == 0 {
		config.Timeouts.CleanupTimeout = defaults.Timeouts.CleanupTimeout
	}

	// Apply vault config defaults
	if config.VaultConfig.UnsealThreshold == 0 {
		config.VaultConfig.UnsealThreshold = defaults.VaultConfig.UnsealThreshold
	}
	if config.VaultConfig.SecretShares == 0 {
		config.VaultConfig.SecretShares = defaults.VaultConfig.SecretShares
	}
	if config.VaultConfig.Endpoints == nil {
		config.VaultConfig.Endpoints = defaults.VaultConfig.Endpoints
	}

	// Apply operator config defaults
	if config.OperatorConfig.Image == "" {
		config.OperatorConfig.Image = defaults.OperatorConfig.Image
	}
	if config.OperatorConfig.Tag == "" {
		config.OperatorConfig.Tag = defaults.OperatorConfig.Tag
	}
	if config.OperatorConfig.LogLevel == "" {
		config.OperatorConfig.LogLevel = defaults.OperatorConfig.LogLevel
	}
	if config.OperatorConfig.Resources.CPU == "" {
		config.OperatorConfig.Resources.CPU = defaults.OperatorConfig.Resources.CPU
	}
	if config.OperatorConfig.Resources.Memory == "" {
		config.OperatorConfig.Resources.Memory = defaults.OperatorConfig.Resources.Memory
	}

	// Apply test settings defaults
	if config.TestSettings.MaxConcurrency == 0 {
		config.TestSettings.MaxConcurrency = defaults.TestSettings.MaxConcurrency
	}

	// Ensure environment map exists
	if config.Environment == nil {
		config.Environment = make(map[string]string)
	}
}

// validateConfig validates the test configuration
func validateConfig(config *TestConfig) error {
	if config.VaultVersion == "" {
		return fmt.Errorf("vault version cannot be empty")
	}

	if len(config.TestScenarios) == 0 {
		return fmt.Errorf("at least one test scenario must be specified")
	}

	// Validate timeouts are positive
	if config.Timeouts.VaultStartup <= 0 {
		return fmt.Errorf("vault startup timeout must be positive")
	}
	if config.Timeouts.OperatorReady <= 0 {
		return fmt.Errorf("operator ready timeout must be positive")
	}
	if config.Timeouts.VaultUnseal <= 0 {
		return fmt.Errorf("vault unseal timeout must be positive")
	}

	// Validate vault config
	if config.VaultConfig.UnsealThreshold <= 0 {
		return fmt.Errorf("unseal threshold must be positive")
	}
	if config.VaultConfig.SecretShares < config.VaultConfig.UnsealThreshold {
		return fmt.Errorf("secret shares must be >= unseal threshold")
	}

	// Validate operator config
	if config.OperatorConfig.Image == "" {
		return fmt.Errorf("operator image cannot be empty")
	}

	// Validate test settings
	if config.TestSettings.MaxConcurrency <= 0 {
		return fmt.Errorf("max concurrency must be positive")
	}

	return nil
}

// SaveTestConfig saves test configuration to a YAML file
func SaveTestConfig(config *TestConfig, path string) error {
	data, err := yaml.Marshal(config)
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file %s: %w", path, err)
	}

	return nil
}

// LoadTestScenarios loads test scenarios from a configuration file
func LoadTestScenarios(scenarioPath string) ([]TestScenario, error) {
	if scenarioPath == "" {
		return getDefaultTestScenarios(), nil
	}

	data, err := os.ReadFile(scenarioPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read scenario file %s: %w", scenarioPath, err)
	}

	var scenarios []TestScenario
	if err := yaml.Unmarshal(data, &scenarios); err != nil {
		return nil, fmt.Errorf("failed to parse scenario file %s: %w", scenarioPath, err)
	}

	return scenarios, nil
}

// getDefaultTestScenarios returns default test scenarios
func getDefaultTestScenarios() []TestScenario {
	return []TestScenario{
		{
			Name:        "basic",
			Description: "Single Vault instance in dev mode",
			VaultSetup: VaultScenarioSetup{
				Instances: []VaultInstanceSetup{
					{
						Name:         "vault-basic",
						Port:         8200,
						DevMode:      true,
						InitialState: "unsealed",
						RootToken:    "root",
						Environment: map[string]string{
							"VAULT_DEV_ROOT_TOKEN_ID":     "root",
							"VAULT_DEV_LISTEN_ADDRESS":    "0.0.0.0:8200",
						},
						HealthChecks: HealthCheckConfig{
							Enabled:      true,
							Endpoint:     "/v1/sys/health",
							Interval:     5 * time.Second,
							Timeout:      2 * time.Second,
							Retries:      5,
							StartupDelay: 10 * time.Second,
						},
					},
				},
			},
			TestCases: []string{
				"vault-unsealing",
				"operator-status",
				"crd-validation",
			},
			Prerequisites: []string{"kubernetes-ready"},
			Cleanup:       true,
		},
		{
			Name:        "failover",
			Description: "Primary + Standby Vault instances for failover testing",
			VaultSetup: VaultScenarioSetup{
				Instances: []VaultInstanceSetup{
					{
						Name:         "vault-primary",
						Port:         8200,
						DevMode:      true,
						InitialState: "unsealed",
						RootToken:    "root",
						Environment: map[string]string{
							"VAULT_DEV_ROOT_TOKEN_ID":     "root",
							"VAULT_DEV_LISTEN_ADDRESS":    "0.0.0.0:8200",
						},
					},
					{
						Name:         "vault-standby",
						Port:         8201,
						DevMode:      true,
						InitialState: "unsealed",
						RootToken:    "standby-token",
						Environment: map[string]string{
							"VAULT_DEV_ROOT_TOKEN_ID":     "standby-token",
							"VAULT_DEV_LISTEN_ADDRESS":    "0.0.0.0:8200",
						},
					},
				},
			},
			TestCases: []string{
				"vault-unsealing",
				"failover-testing",
				"operator-status",
			},
			Prerequisites: []string{"kubernetes-ready"},
			Cleanup:       true,
		},
		{
			Name:        "multi-vault",
			Description: "Multiple independent Vault clusters",
			VaultSetup: VaultScenarioSetup{
				Instances: []VaultInstanceSetup{
					{
						Name:         "vault-finance",
						Port:         8200,
						DevMode:      true,
						InitialState: "unsealed",
						RootToken:    "finance-root",
						Environment: map[string]string{
							"VAULT_DEV_ROOT_TOKEN_ID":     "finance-root",
							"VAULT_DEV_LISTEN_ADDRESS":    "0.0.0.0:8200",
						},
					},
					{
						Name:         "vault-engineering",
						Port:         8201,
						DevMode:      true,
						InitialState: "unsealed",
						RootToken:    "eng-root",
						Environment: map[string]string{
							"VAULT_DEV_ROOT_TOKEN_ID":     "eng-root",
							"VAULT_DEV_LISTEN_ADDRESS":    "0.0.0.0:8200",
						},
					},
					{
						Name:         "vault-operations",
						Port:         8202,
						DevMode:      true,
						InitialState: "unsealed",
						RootToken:    "ops-root",
						Environment: map[string]string{
							"VAULT_DEV_ROOT_TOKEN_ID":     "ops-root",
							"VAULT_DEV_LISTEN_ADDRESS":    "0.0.0.0:8200",
						},
					},
				},
			},
			TestCases: []string{
				"vault-unsealing",
				"multi-vault-coordination",
				"operator-status",
			},
			Prerequisites: []string{"kubernetes-ready"},
			Cleanup:       true,
		},
	}
}
