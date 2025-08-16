package v1

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestTypes(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Types Suite")
}

var _ = Describe("VaultUnsealConfig API Types", func() {
	Describe("VaultInstance Validation", func() {
		It("should create a valid VaultInstance with all fields", func() {
			threshold := 3
			instance := VaultInstance{
				Name:          "test-vault",
				Endpoint:      "https://vault.example.com:8200",
				UnsealKeys:    []string{"key1", "key2", "key3", "key4", "key5"},
				Threshold:     &threshold,
				TLSSkipVerify: true,
			}

			Expect(instance.Name).To(Equal("test-vault"))
			Expect(instance.Endpoint).To(Equal("https://vault.example.com:8200"))
			Expect(len(instance.UnsealKeys)).To(Equal(5))
			Expect(*instance.Threshold).To(Equal(3))
			Expect(instance.TLSSkipVerify).To(BeTrue())
		})

		It("should create a valid VaultInstance with minimal fields", func() {
			instance := VaultInstance{
				Name:       "minimal-vault",
				Endpoint:   "http://vault:8200",
				UnsealKeys: []string{"key1"},
			}

			Expect(instance.Name).To(Equal("minimal-vault"))
			Expect(instance.Endpoint).To(Equal("http://vault:8200"))
			Expect(len(instance.UnsealKeys)).To(Equal(1))
			Expect(instance.Threshold).To(BeNil())       // Should use default
			Expect(instance.TLSSkipVerify).To(BeFalse()) // Default value
		})

		It("should handle various endpoint formats", func() {
			endpoints := []string{
				"http://vault:8200",
				"https://vault.example.com:8200",
				"http://192.168.1.100:8200",
				"https://vault-cluster.namespace.svc.cluster.local:8200",
				"http://localhost:8200",
			}

			for _, endpoint := range endpoints {
				instance := VaultInstance{
					Name:       "test-vault",
					Endpoint:   endpoint,
					UnsealKeys: []string{"key1"},
				}
				Expect(instance.Endpoint).To(Equal(endpoint))
			}
		})

		It("should handle different threshold values", func() {
			testCases := []struct {
				threshold *int
				expected  *int
			}{
				{nil, nil},               // Default threshold
				{intPtr(1), intPtr(1)},   // Single key threshold
				{intPtr(3), intPtr(3)},   // Standard threshold
				{intPtr(5), intPtr(5)},   // High threshold
				{intPtr(10), intPtr(10)}, // Very high threshold
			}

			for _, tc := range testCases {
				instance := VaultInstance{
					Name:       "test-vault",
					Endpoint:   "http://vault:8200",
					UnsealKeys: []string{"key1", "key2", "key3", "key4", "key5", "key6", "key7", "key8", "key9", "key10"},
					Threshold:  tc.threshold,
				}

				if tc.expected == nil {
					Expect(instance.Threshold).To(BeNil())
				} else {
					Expect(*instance.Threshold).To(Equal(*tc.expected))
				}
			}
		})

		It("should handle various unseal key configurations", func() {
			testCases := []struct {
				description string
				keys        []string
				expectedLen int
			}{
				{"single key", []string{"key1"}, 1},
				{"standard shamir keys", []string{"key1", "key2", "key3", "key4", "key5"}, 5},
				{"many keys", make([]string, 10), 10},
				{"empty keys", []string{}, 0},
			}

			for _, tc := range testCases {
				// Fill test keys for the "many keys" case
				if len(tc.keys) == 10 && tc.keys[0] == "" {
					for i := 0; i < 10; i++ {
						tc.keys[i] = "key" + string(rune('0'+i))
					}
				}

				instance := VaultInstance{
					Name:       "test-vault",
					Endpoint:   "http://vault:8200",
					UnsealKeys: tc.keys,
				}

				Expect(len(instance.UnsealKeys)).To(Equal(tc.expectedLen), "Failed for case: %s", tc.description)
			}
		})
	})

	Describe("VaultUnsealConfig Validation", func() {
		It("should create a valid VaultUnsealConfig", func() {
			config := VaultUnsealConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-config",
					Namespace: "vault-system",
				},
				Spec: VaultUnsealConfigSpec{
					VaultInstances: []VaultInstance{
						{
							Name:       "vault-1",
							Endpoint:   "http://vault-1:8200",
							UnsealKeys: []string{"key1", "key2", "key3"},
						},
						{
							Name:       "vault-2",
							Endpoint:   "http://vault-2:8200",
							UnsealKeys: []string{"key1", "key2", "key3"},
						},
					},
				},
			}

			Expect(config.Name).To(Equal("test-config"))
			Expect(config.Namespace).To(Equal("vault-system"))
			Expect(len(config.Spec.VaultInstances)).To(Equal(2))
		})

		It("should handle empty vault instances", func() {
			config := VaultUnsealConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-config",
					Namespace: "default",
				},
				Spec: VaultUnsealConfigSpec{
					VaultInstances: []VaultInstance{},
				},
			}

			Expect(config.Name).To(Equal("empty-config"))
			Expect(len(config.Spec.VaultInstances)).To(Equal(0))
		})

		It("should handle large number of vault instances", func() {
			instances := make([]VaultInstance, 50)
			for i := 0; i < 50; i++ {
				instances[i] = VaultInstance{
					Name:       "vault-" + string(rune('0'+i%10)),
					Endpoint:   "http://vault-" + string(rune('0'+i%10)) + ":8200",
					UnsealKeys: []string{"key1", "key2", "key3"},
				}
			}

			config := VaultUnsealConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "large-config",
					Namespace: "vault-system",
				},
				Spec: VaultUnsealConfigSpec{
					VaultInstances: instances,
				},
			}

			Expect(len(config.Spec.VaultInstances)).To(Equal(50))
		})
	})

	Describe("VaultInstanceStatus Validation", func() {
		It("should create valid status objects", func() {
			now := metav1.Now()
			status := VaultInstanceStatus{
				Name:         "vault-1",
				Sealed:       false,
				Error:        "",
				LastUnsealed: &now,
			}

			Expect(status.Name).To(Equal("vault-1"))
			Expect(status.Sealed).To(BeFalse())
			Expect(status.Error).To(BeEmpty())
			Expect(status.LastUnsealed).ToNot(BeNil())
		})

		It("should handle error states", func() {
			status := VaultInstanceStatus{
				Name:   "vault-1",
				Sealed: true,
				Error:  "Connection refused",
			}

			Expect(status.Name).To(Equal("vault-1"))
			Expect(status.Sealed).To(BeTrue())
			Expect(status.Error).To(Equal("Connection refused"))
			Expect(status.LastUnsealed).To(BeNil())
		})

		It("should handle various error messages", func() {
			errorMessages := []string{
				"Connection refused",
				"Timeout connecting to vault",
				"Invalid unseal key",
				"Vault is not initialized",
				"TLS verification failed",
				"Authentication failed",
				"",
			}

			for _, errorMsg := range errorMessages {
				status := VaultInstanceStatus{
					Name:  "vault-1",
					Error: errorMsg,
				}
				Expect(status.Error).To(Equal(errorMsg))
			}
		})
	})

	Describe("VaultUnsealConfigStatus Validation", func() {
		It("should handle status with multiple vault instances", func() {
			now := metav1.Now()
			status := VaultUnsealConfigStatus{
				VaultStatuses: []VaultInstanceStatus{
					{
						Name:         "vault-1",
						Sealed:       false,
						LastUnsealed: &now,
					},
					{
						Name:   "vault-2",
						Sealed: true,
						Error:  "Connection failed",
					},
				},
				Conditions: []metav1.Condition{
					{
						Type:    "Ready",
						Status:  metav1.ConditionFalse,
						Reason:  "SomeInstancesSealed",
						Message: "1 of 2 vault instances need attention",
					},
				},
			}

			Expect(len(status.VaultStatuses)).To(Equal(2))
			Expect(len(status.Conditions)).To(Equal(1))
			Expect(status.Conditions[0].Type).To(Equal("Ready"))
			Expect(status.Conditions[0].Status).To(Equal(metav1.ConditionFalse))
		})

		It("should handle various condition states", func() {
			conditions := []metav1.Condition{
				{
					Type:    "Ready",
					Status:  metav1.ConditionTrue,
					Reason:  "AllInstancesUnsealed",
					Message: "All 3 vault instances are unsealed",
				},
				{
					Type:    "Ready",
					Status:  metav1.ConditionFalse,
					Reason:  "SomeInstancesSealed",
					Message: "2 of 3 vault instances need attention",
				},
				{
					Type:    "Ready",
					Status:  metav1.ConditionUnknown,
					Reason:  "CheckInProgress",
					Message: "Checking vault instance status",
				},
			}

			for _, condition := range conditions {
				status := VaultUnsealConfigStatus{
					Conditions: []metav1.Condition{condition},
				}
				Expect(len(status.Conditions)).To(Equal(1))
				Expect(status.Conditions[0].Type).To(Equal(condition.Type))
				Expect(status.Conditions[0].Status).To(Equal(condition.Status))
			}
		})

		It("should handle multiple conditions", func() {
			status := VaultUnsealConfigStatus{
				Conditions: []metav1.Condition{
					{
						Type:   "Ready",
						Status: metav1.ConditionTrue,
						Reason: "AllInstancesUnsealed",
					},
					{
						Type:   "Degraded",
						Status: metav1.ConditionFalse,
						Reason: "AllInstancesHealthy",
					},
					{
						Type:   "Progressing",
						Status: metav1.ConditionFalse,
						Reason: "NoOperationInProgress",
					},
				},
			}

			Expect(len(status.Conditions)).To(Equal(3))

			// Find and verify each condition type
			conditionTypes := make(map[string]metav1.ConditionStatus)
			for _, condition := range status.Conditions {
				conditionTypes[condition.Type] = condition.Status
			}

			Expect(conditionTypes["Ready"]).To(Equal(metav1.ConditionTrue))
			Expect(conditionTypes["Degraded"]).To(Equal(metav1.ConditionFalse))
			Expect(conditionTypes["Progressing"]).To(Equal(metav1.ConditionFalse))
		})
	})

	Describe("Edge Cases and Boundary Conditions", func() {
		It("should handle very long names", func() {
			longName := "very-long-vault-instance-name-that-exceeds-normal-length-boundaries-and-tests-system-limits"
			instance := VaultInstance{
				Name:       longName,
				Endpoint:   "http://vault:8200",
				UnsealKeys: []string{"key1"},
			}
			Expect(instance.Name).To(Equal(longName))
		})

		It("should handle special characters in names", func() {
			specialNames := []string{
				"vault-with-dashes",
				"vault_with_underscores",
				"vault.with.dots",
				"vault123numbers",
				"VaultWithCamelCase",
			}

			for _, name := range specialNames {
				instance := VaultInstance{
					Name:       name,
					Endpoint:   "http://vault:8200",
					UnsealKeys: []string{"key1"},
				}
				Expect(instance.Name).To(Equal(name))
			}
		})

		It("should handle extreme threshold values within valid range", func() {
			extremeThresholds := []*int{
				intPtr(1),   // Minimum
				intPtr(100), // Very high but valid
				intPtr(255), // Edge case high value
			}

			for _, threshold := range extremeThresholds {
				instance := VaultInstance{
					Name:       "test-vault",
					Endpoint:   "http://vault:8200",
					UnsealKeys: make([]string, *threshold), // Create enough keys
					Threshold:  threshold,
				}

				// Fill the keys slice
				for i := 0; i < *threshold; i++ {
					instance.UnsealKeys[i] = "key" + string(rune('A'+i%26))
				}

				Expect(*instance.Threshold).To(Equal(*threshold))
				Expect(len(instance.UnsealKeys)).To(Equal(*threshold))
			}
		})
	})
})

// Helper function to create int pointers
func intPtr(i int) *int {
	return &i
}
