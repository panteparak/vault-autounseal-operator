package suites

import (
	"context"
	"fmt"

	"github.com/panteparak/vault-autounseal-operator/test/integration/framework"
	"github.com/panteparak/vault-autounseal-operator/test/integration/testcases"
)

// IntegrationTestSuite represents the main integration test suite
type IntegrationTestSuite struct {
	scenario   string
	testCases  []framework.TestCase
	vaultSetup framework.VaultScenarioSetup
}

// NewIntegrationTestSuite creates a new integration test suite for a specific scenario
func NewIntegrationTestSuite(scenario string, vaultSetup framework.VaultScenarioSetup) framework.TestSuite {
	// Create test cases for the scenario
	testCases := []framework.TestCase{
		testcases.NewVaultUnsealingTest(scenario),
		testcases.NewOperatorStatusTest(scenario),
		testcases.NewCRDValidationTest(scenario),
	}

	// Add scenario-specific test cases
	switch scenario {
	case "failover":
		testCases = append(testCases, testcases.NewFailoverTest())
	case "multi-vault":
		testCases = append(testCases, testcases.NewMultiVaultCoordinationTest())
	}

	return &IntegrationTestSuite{
		scenario:   scenario,
		testCases:  testCases,
		vaultSetup: vaultSetup,
	}
}

func (s *IntegrationTestSuite) Name() string {
	return fmt.Sprintf("integration-test-suite-%s", s.scenario)
}

func (s *IntegrationTestSuite) Description() string {
	return fmt.Sprintf("Integration test suite for %s scenario", s.scenario)
}

func (s *IntegrationTestSuite) TestCases() []framework.TestCase {
	return s.testCases
}

func (s *IntegrationTestSuite) Setup(ctx context.Context, fw *framework.TestFramework) error {
	// Setup infrastructure
	if err := fw.Infrastructure.Setup(ctx, fw.Config); err != nil {
		return fmt.Errorf("failed to setup infrastructure: %w", err)
	}

	// Create Vault instances according to the scenario
	for _, instanceSetup := range s.vaultSetup.Instances {
		_, err := fw.Infrastructure.CreateVaultInstance(ctx, instanceSetup)
		if err != nil {
			return fmt.Errorf("failed to create vault instance %s: %w", instanceSetup.Name, err)
		}
	}

	// Deploy the operator
	if err := fw.Infrastructure.DeployOperator(ctx, fw.Config.OperatorConfig); err != nil {
		return fmt.Errorf("failed to deploy operator: %w", err)
	}

	// Create VaultUnsealConfig resources
	if err := s.createVaultUnsealConfigs(ctx, fw); err != nil {
		return fmt.Errorf("failed to create VaultUnsealConfig resources: %w", err)
	}

	return nil
}

func (s *IntegrationTestSuite) Teardown(ctx context.Context, fw *framework.TestFramework) error {
	// Clean up VaultUnsealConfig resources
	if err := s.cleanupVaultUnsealConfigs(ctx, fw); err != nil {
		fmt.Printf("Warning: failed to cleanup VaultUnsealConfigs: %v\n", err)
	}

	// Clean up infrastructure
	return fw.Infrastructure.Cleanup(ctx)
}

func (s *IntegrationTestSuite) createVaultUnsealConfigs(ctx context.Context, fw *framework.TestFramework) error {
	configName := s.getConfigNameForScenario()

	// Create the appropriate VaultUnsealConfig for this scenario
	spec := s.buildVaultUnsealConfigSpec()

	return s.createVaultUnsealConfig(ctx, fw, configName, spec)
}

func (s *IntegrationTestSuite) getConfigNameForScenario() string {
	switch s.scenario {
	case "basic":
		return "test-basic-vault-config"
	case "failover":
		return "test-failover-vault-config"
	case "multi-vault":
		return "test-multi-vault-config"
	default:
		return "test-vault-config"
	}
}

func (s *IntegrationTestSuite) buildVaultUnsealConfigSpec() map[string]interface{} {
	var vaultInstances []interface{}

	switch s.scenario {
	case "basic":
		vaultInstances = []interface{}{
			map[string]interface{}{
				"name":          "vault-basic",
				"endpoint":      "http://host.docker.internal:8200",
				"unsealKeys":    []string{"YmFzaWMta2V5LTE=", "YmFzaWMta2V5LTI=", "YmFzaWMta2V5LTM="}, // base64 encoded keys
				"threshold":     3,
				"tlsSkipVerify": true,
			},
		}
	case "failover":
		vaultInstances = []interface{}{
			map[string]interface{}{
				"name":          "vault-primary",
				"endpoint":      "http://host.docker.internal:8200",
				"unsealKeys":    []string{"ZmFpbG92ZXItcHJpbWFyeS1rZXktMQ==", "ZmFpbG92ZXItcHJpbWFyeS1rZXktMg==", "ZmFpbG92ZXItcHJpbWFyeS1rZXktMw=="},
				"threshold":     3,
				"tlsSkipVerify": true,
			},
			map[string]interface{}{
				"name":          "vault-standby",
				"endpoint":      "http://host.docker.internal:8201",
				"unsealKeys":    []string{"ZmFpbG92ZXItc3RhbmRieS1rZXktMQ==", "ZmFpbG92ZXItc3RhbmRieS1rZXktMg==", "ZmFpbG92ZXItc3RhbmRieS1rZXktMw=="},
				"threshold":     3,
				"tlsSkipVerify": true,
			},
		}
	case "multi-vault":
		vaultInstances = []interface{}{
			map[string]interface{}{
				"name":          "vault-finance",
				"endpoint":      "http://host.docker.internal:8200",
				"unsealKeys":    []string{"ZmluYW5jZS1rZXktMQ==", "ZmluYW5jZS1rZXktMg==", "ZmluYW5jZS1rZXktMw=="},
				"threshold":     3,
				"tlsSkipVerify": true,
			},
			map[string]interface{}{
				"name":          "vault-engineering",
				"endpoint":      "http://host.docker.internal:8201",
				"unsealKeys":    []string{"ZW5naW5lZXJpbmcta2V5LTE=", "ZW5naW5lZXJpbmcta2V5LTI=", "ZW5naW5lZXJpbmcta2V5LTM="},
				"threshold":     3,
				"tlsSkipVerify": true,
			},
			map[string]interface{}{
				"name":          "vault-operations",
				"endpoint":      "http://host.docker.internal:8202",
				"unsealKeys":    []string{"b3BlcmF0aW9ucy1rZXktMQ==", "b3BlcmF0aW9ucy1rZXktMg==", "b3BlcmF0aW9ucy1rZXktMw=="},
				"threshold":     3,
				"tlsSkipVerify": true,
			},
		}
	}

	return map[string]interface{}{
		"vaultInstances": vaultInstances,
	}
}

func (s *IntegrationTestSuite) createVaultUnsealConfig(ctx context.Context, fw *framework.TestFramework, name string, spec map[string]interface{}) error {
	// This would create the actual VaultUnsealConfig CRD resource
	// For now, we'll use a placeholder implementation

	fmt.Printf("Creating VaultUnsealConfig: %s with spec: %+v\n", name, spec)

	// In a real implementation, this would:
	// 1. Create an unstructured.Unstructured object
	// 2. Set the GVK for VaultUnsealConfig
	// 3. Set the name, namespace, and spec
	// 4. Create it using fw.KubeClient.Create()

	return nil
}

func (s *IntegrationTestSuite) cleanupVaultUnsealConfigs(ctx context.Context, fw *framework.TestFramework) error {
	// This would clean up the VaultUnsealConfig resources
	// For now, we'll use a placeholder implementation

	fmt.Printf("Cleaning up VaultUnsealConfig resources for scenario: %s\n", s.scenario)

	return nil
}
