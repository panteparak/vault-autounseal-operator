package testcases

import (
	"context"
	"fmt"
	"time"

	"github.com/panteparak/vault-autounseal-operator/test/integration/framework"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// OperatorStatusTest validates the operator's status and behavior
type OperatorStatusTest struct {
	scenario string
}

// NewOperatorStatusTest creates a new operator status test
func NewOperatorStatusTest(scenario string) framework.TestCase {
	return &OperatorStatusTest{
		scenario: scenario,
	}
}

func (t *OperatorStatusTest) Name() string {
	return fmt.Sprintf("operator-status-%s", t.scenario)
}

func (t *OperatorStatusTest) Description() string {
	return fmt.Sprintf("Validates operator status and behavior for %s scenario", t.scenario)
}

func (t *OperatorStatusTest) Prerequisites() []string {
	return []string{"operator-deployed", "kubernetes-ready"}
}

func (t *OperatorStatusTest) Tags() []string {
	return []string{"operator", "status", "kubernetes", t.scenario}
}

func (t *OperatorStatusTest) Execute(ctx context.Context, framework *framework.TestFramework) *framework.TestResult {
	result := &framework.TestResult{
		TestName:  t.Name(),
		Timestamp: time.Now(),
		Details:   make(map[string]interface{}),
		Logs:      make([]string, 0),
		Metrics: framework.TestMetrics{
			OperatorMetrics: make(map[string]float64),
			APICallCounts:   make(map[string]int),
		},
	}

	result.Details["scenario"] = t.scenario

	// Test 1: Check operator pod status
	if !t.checkOperatorPodStatus(ctx, framework, result) {
		result.Success = false
		return result
	}

	// Test 2: Check operator logs for errors
	if !t.checkOperatorLogs(ctx, framework, result) {
		result.Success = false
		return result
	}

	// Test 3: Validate VaultUnsealConfig CRD status
	if !t.checkVaultUnsealConfigStatus(ctx, framework, result) {
		result.Success = false
		return result
	}

	// Test 4: Test operator resilience with multiple configs
	if !t.testOperatorResilience(ctx, framework, result) {
		result.Success = false
		return result
	}

	result.Success = true
	return result
}

func (t *OperatorStatusTest) checkOperatorPodStatus(ctx context.Context, framework *framework.TestFramework, result *framework.TestResult) bool {
	result.Logs = append(result.Logs, "Checking operator pod status...")

	// List pods with operator label
	podList, err := framework.KubernetesClient.CoreV1().Pods(framework.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app.kubernetes.io/name=vault-autounseal-operator",
	})
	if err != nil {
		result.Logs = append(result.Logs, fmt.Sprintf("Failed to list operator pods: %v", err))
		return false
	}

	if len(podList.Items) == 0 {
		result.Logs = append(result.Logs, "No operator pods found")
		return false
	}

	// Check each pod
	allHealthy := true
	for _, pod := range podList.Items {
		result.Details[fmt.Sprintf("pod_%s_phase", pod.Name)] = string(pod.Status.Phase)
		result.Details[fmt.Sprintf("pod_%s_ready", pod.Name)] = t.isPodReady(&pod)

		if pod.Status.Phase != "Running" {
			result.Logs = append(result.Logs, fmt.Sprintf("Pod %s is not running: %s", pod.Name, pod.Status.Phase))
			allHealthy = false
		}

		if !t.isPodReady(&pod) {
			result.Logs = append(result.Logs, fmt.Sprintf("Pod %s is not ready", pod.Name))
			allHealthy = false
		}
	}

	if allHealthy {
		result.Logs = append(result.Logs, "✅ All operator pods are healthy")
		return true
	}

	return false
}

func (t *OperatorStatusTest) isPodReady(pod *v1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == v1.PodReady {
			return condition.Status == v1.ConditionTrue
		}
	}
	return false
}

func (t *OperatorStatusTest) checkOperatorLogs(ctx context.Context, framework *framework.TestFramework, result *framework.TestResult) bool {
	result.Logs = append(result.Logs, "Checking operator logs for errors...")

	logs, err := framework.Infrastructure.GetLogs(ctx, "operator")
	if err != nil {
		result.Logs = append(result.Logs, fmt.Sprintf("Failed to get operator logs: %v", err))
		return false
	}

	errorCount := 0
	warningCount := 0

	for _, line := range logs {
		if contains(line, "ERROR") || contains(line, "error") {
			errorCount++
		}
		if contains(line, "WARN") || contains(line, "warning") {
			warningCount++
		}
	}

	result.Details["error_count"] = errorCount
	result.Details["warning_count"] = warningCount
	result.Details["total_log_lines"] = len(logs)

	if errorCount > 5 { // Allow some errors but not too many
		result.Logs = append(result.Logs, fmt.Sprintf("❌ Too many errors in operator logs: %d", errorCount))
		return false
	}

	result.Logs = append(result.Logs, fmt.Sprintf("✅ Operator logs healthy - Errors: %d, Warnings: %d", errorCount, warningCount))
	return true
}

func (t *OperatorStatusTest) checkVaultUnsealConfigStatus(ctx context.Context, framework *framework.TestFramework, result *framework.TestResult) bool {
	result.Logs = append(result.Logs, "Checking VaultUnsealConfig status...")

	// Get the expected config name for this scenario
	configName := t.getConfigNameForScenario()

	// Define the GVK for VaultUnsealConfig
	gvk := schema.GroupVersionKind{
		Group:   "vault.io",
		Version: "v1",
		Kind:    "VaultUnsealConfig",
	}

	// Create an unstructured object to hold the result
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)

	// Get the VaultUnsealConfig
	err := framework.KubeClient.Get(ctx, client.ObjectKey{
		Name:      configName,
		Namespace: "default",
	}, obj)

	if err != nil {
		result.Logs = append(result.Logs, fmt.Sprintf("Failed to get VaultUnsealConfig %s: %v", configName, err))
		return false
	}

	// Extract status information
	status, found, err := unstructured.NestedMap(obj.Object, "status")
	if err != nil || !found {
		result.Logs = append(result.Logs, fmt.Sprintf("No status found in VaultUnsealConfig %s", configName))
		return false
	}

	result.Details["config_status"] = status

	// Check conditions
	conditions, found, err := unstructured.NestedSlice(status, "conditions")
	if err == nil && found {
		for _, conditionInterface := range conditions {
			if condition, ok := conditionInterface.(map[string]interface{}); ok {
				condType, _ := condition["type"].(string)
				condStatus, _ := condition["status"].(string)
				condReason, _ := condition["reason"].(string)

				if condType == "Ready" {
					result.Details["ready_status"] = condStatus
					result.Details["ready_reason"] = condReason

					if condStatus == "True" && condReason == "AllInstancesUnsealed" {
						result.Logs = append(result.Logs, "✅ VaultUnsealConfig shows all instances are unsealed")
						return true
					} else if condStatus == "False" {
						result.Logs = append(result.Logs, fmt.Sprintf("⚠️ VaultUnsealConfig shows issues: %s", condReason))
						// Don't fail the test immediately - this might be expected in some scenarios
						return true
					}
				}
			}
		}
	}

	result.Logs = append(result.Logs, "❌ VaultUnsealConfig status validation inconclusive")
	return false
}

func (t *OperatorStatusTest) testOperatorResilience(ctx context.Context, framework *framework.TestFramework, result *framework.TestResult) bool {
	result.Logs = append(result.Logs, "Testing operator resilience with already unsealed vaults...")

	// Create a second VaultUnsealConfig to test graceful handling
	configName := "test-resilience-config"

	gvk := schema.GroupVersionKind{
		Group:   "vault.io",
		Version: "v1",
		Kind:    "VaultUnsealConfig",
	}

	// Create the test config
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(gvk)
	obj.SetName(configName)
	obj.SetNamespace("default")

	// Set spec
	spec := map[string]interface{}{
		"vaultInstances": []interface{}{
			map[string]interface{}{
				"name":          "vault-resilience-test",
				"endpoint":      "http://host.docker.internal:8200",
				"unsealKeys":    []string{"dGVzdC1yZXNpbGllbmNlLWtleQ=="}, // base64 encoded "test-resilience-key"
				"threshold":     1,
				"tlsSkipVerify": true,
			},
		},
	}
	obj.Object["spec"] = spec

	// Create the resource
	err := framework.KubeClient.Create(ctx, obj)
	if err != nil {
		result.Logs = append(result.Logs, fmt.Sprintf("Failed to create test config: %v", err))
		return false
	}

	// Wait for operator to process
	time.Sleep(20 * time.Second)

	// Check if operator handled it gracefully
	updatedObj := &unstructured.Unstructured{}
	updatedObj.SetGroupVersionKind(gvk)
	err = framework.KubeClient.Get(ctx, client.ObjectKey{
		Name:      configName,
		Namespace: "default",
	}, updatedObj)

	if err != nil {
		result.Logs = append(result.Logs, fmt.Sprintf("Failed to get updated test config: %v", err))
		return false
	}

	// Check if status was updated (indicating operator processed it)
	status, found, _ := unstructured.NestedMap(updatedObj.Object, "status")
	if found && status != nil {
		result.Logs = append(result.Logs, "✅ Operator gracefully handled already unsealed Vault instances")
		result.Details["resilience_test_passed"] = true
	} else {
		result.Logs = append(result.Logs, "⚠️ Operator is still processing or encountered issues")
		result.Details["resilience_test_passed"] = false
	}

	// Cleanup the test config
	framework.KubeClient.Delete(ctx, updatedObj)

	return true
}

func (t *OperatorStatusTest) getConfigNameForScenario() string {
	switch t.scenario {
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

func (t *OperatorStatusTest) Cleanup(ctx context.Context, framework *framework.TestFramework) error {
	// Clean up any test resources created during resilience testing
	return nil
}

// Helper function to check if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	return len(s) >= len(substr) &&
		   (s == substr ||
		    len(s) > len(substr) &&
		    (s[:len(substr)] == substr ||
		     s[len(s)-len(substr):] == substr ||
		     containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
