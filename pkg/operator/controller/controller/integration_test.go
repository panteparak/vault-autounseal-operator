package controller

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	vaultv1 "github.com/panteparak/vault-autounseal-operator/pkg/api/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("VaultUnsealConfig Controller Integration", func() {
	var reconciler *VaultUnsealConfigReconciler
	var scheme *runtime.Scheme
	var ctx context.Context

	BeforeEach(func() {
		ctx = context.Background()
		scheme = runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
		Expect(vaultv1.AddToScheme(scheme)).To(Succeed())

		reconciler = &VaultUnsealConfigReconciler{
			Scheme: scheme,
			Log:    ctrl.Log.WithName("integration-test"),
		}
	})

	Describe("End-to-End Reconciliation Flow", func() {
		It("should handle complete reconciliation cycle", func() {
			vaultConfig := &vaultv1.VaultUnsealConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "e2e-test-config",
					Namespace:  "vault-system",
					Generation: 1,
				},
				Spec: vaultv1.VaultUnsealConfigSpec{
					VaultInstances: []vaultv1.VaultInstance{
						{
							Name:          "vault-primary",
							Endpoint:      "http://vault-0.vault-internal:8200",
							UnsealKeys:    []string{"dGVzdC1rZXktMQ==", "dGVzdC1rZXktMg==", "dGVzdC1rZXktMw=="},
							TLSSkipVerify: false,
						},
						{
							Name:          "vault-secondary",
							Endpoint:      "https://vault-1.vault-internal:8200",
							UnsealKeys:    []string{"dGVzdC1rZXktMQ==", "dGVzdC1rZXktMg==", "dGVzdC1rZXktMw=="},
							TLSSkipVerify: true,
							Threshold:     intPtr(2),
						},
					},
				},
			}

			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(vaultConfig).
				WithStatusSubresource(vaultConfig).
				Build()
			reconciler.Client = client

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "e2e-test-config",
					Namespace: "vault-system",
				},
			}

			// First reconciliation
			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(30 * time.Second))

			// Verify status was updated
			var updatedConfig vaultv1.VaultUnsealConfig
			err = client.Get(ctx, req.NamespacedName, &updatedConfig)
			Expect(err).ToNot(HaveOccurred())

			// Should have status for both instances
			Expect(len(updatedConfig.Status.VaultStatuses)).To(Equal(2))

			// Should have Ready condition
			Expect(len(updatedConfig.Status.Conditions)).To(BeNumerically(">=", 1))
			readyCondition := findCondition(updatedConfig.Status.Conditions, "Ready")
			Expect(readyCondition).ToNot(BeNil())
			Expect(readyCondition.ObservedGeneration).To(Equal(int64(1)))

			// Second reconciliation should be consistent
			result2, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(result2).To(Equal(result))
		})

		It("should handle configuration updates", func() {
			initialConfig := &vaultv1.VaultUnsealConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "update-test-config",
					Namespace:  "default",
					Generation: 1,
				},
				Spec: vaultv1.VaultUnsealConfigSpec{
					VaultInstances: []vaultv1.VaultInstance{
						{
							Name:       "vault-1",
							Endpoint:   "http://vault:8200",
							UnsealKeys: []string{"dGVzdA=="},
						},
					},
				},
			}

			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(initialConfig).
				WithStatusSubresource(initialConfig).
				Build()
			reconciler.Client = client

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "update-test-config",
					Namespace: "default",
				},
			}

			// Initial reconciliation
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			// Update configuration (simulate spec change)
			var configToUpdate vaultv1.VaultUnsealConfig
			err = client.Get(ctx, req.NamespacedName, &configToUpdate)
			Expect(err).ToNot(HaveOccurred())

			// Simulate a spec update (adding another vault instance)
			configToUpdate.Generation = 2
			configToUpdate.Spec.VaultInstances = append(configToUpdate.Spec.VaultInstances, vaultv1.VaultInstance{
				Name:       "vault-2",
				Endpoint:   "http://vault-2:8200",
				UnsealKeys: []string{"dGVzdA=="},
			})

			err = client.Update(ctx, &configToUpdate)
			Expect(err).ToNot(HaveOccurred())

			// Reconcile after update
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			// Verify updated status
			var finalConfig vaultv1.VaultUnsealConfig
			err = client.Get(ctx, req.NamespacedName, &finalConfig)
			Expect(err).ToNot(HaveOccurred())

			// Should now have 2 vault instances in status
			Expect(len(finalConfig.Status.VaultStatuses)).To(Equal(2))

			// Condition should reflect new generation
			readyCondition := findCondition(finalConfig.Status.Conditions, "Ready")
			Expect(readyCondition).ToNot(BeNil())
			Expect(readyCondition.ObservedGeneration).To(Equal(int64(2)))
		})
	})

	Describe("Concurrent Reconciliation", func() {
		It("should handle multiple concurrent reconcile requests", func() {
			configs := make([]*vaultv1.VaultUnsealConfig, 5)
			for i := 0; i < 5; i++ {
				configs[i] = &vaultv1.VaultUnsealConfig{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "concurrent-config-" + string(rune('0'+i)),
						Namespace: "default",
					},
					Spec: vaultv1.VaultUnsealConfigSpec{
						VaultInstances: []vaultv1.VaultInstance{
							{
								Name:       "vault-" + string(rune('0'+i)),
								Endpoint:   "http://vault-" + string(rune('0'+i)) + ":8200",
								UnsealKeys: []string{"dGVzdA=="},
							},
						},
					},
				}
			}

			clientBuilder := fake.NewClientBuilder().WithScheme(scheme)
			for _, config := range configs {
				clientBuilder = clientBuilder.WithObjects(config).WithStatusSubresource(config)
			}
			client := clientBuilder.Build()
			reconciler.Client = client

			// Reconcile all configs concurrently
			done := make(chan bool, 5)
			for i := 0; i < 5; i++ {
				go func(index int) {
					defer func() { done <- true }()

					req := reconcile.Request{
						NamespacedName: types.NamespacedName{
							Name:      "concurrent-config-" + string(rune('0'+index)),
							Namespace: "default",
						},
					}

					result, err := reconciler.Reconcile(ctx, req)
					Expect(err).ToNot(HaveOccurred())
					Expect(result.RequeueAfter).To(Equal(30 * time.Second))
				}(i)
			}

			// Wait for all reconciliations to complete
			for i := 0; i < 5; i++ {
				<-done
			}

			// Verify all configs have been processed
			for i := 0; i < 5; i++ {
				var config vaultv1.VaultUnsealConfig
				err := client.Get(ctx, types.NamespacedName{
					Name:      "concurrent-config-" + string(rune('0'+i)),
					Namespace: "default",
				}, &config)
				Expect(err).ToNot(HaveOccurred())
				Expect(len(config.Status.VaultStatuses)).To(Equal(1))
			}
		})
	})

	Describe("Resource Cleanup", func() {
		It("should handle resource deletion gracefully", func() {
			vaultConfig := &vaultv1.VaultUnsealConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "cleanup-test-config",
					Namespace: "default",
				},
				Spec: vaultv1.VaultUnsealConfigSpec{
					VaultInstances: []vaultv1.VaultInstance{
						{
							Name:       "vault-1",
							Endpoint:   "http://vault:8200",
							UnsealKeys: []string{"dGVzdA=="},
						},
					},
				},
			}

			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(vaultConfig).
				WithStatusSubresource(vaultConfig).
				Build()
			reconciler.Client = client

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "cleanup-test-config",
					Namespace: "default",
				},
			}

			// Initial reconciliation
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			// Delete the resource
			err = client.Delete(ctx, vaultConfig)
			Expect(err).ToNot(HaveOccurred())

			// Reconcile after deletion should not error
			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{})) // Should not requeue
		})
	})

	Describe("Status Condition Management", func() {
		It("should manage multiple condition transitions", func() {
			vaultConfig := &vaultv1.VaultUnsealConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:       "condition-test-config",
					Namespace:  "default",
					Generation: 1,
				},
				Spec: vaultv1.VaultUnsealConfigSpec{
					VaultInstances: []vaultv1.VaultInstance{
						{
							Name:       "vault-1",
							Endpoint:   "http://vault:8200",
							UnsealKeys: []string{"dGVzdA=="},
						},
					},
				},
			}

			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(vaultConfig).
				WithStatusSubresource(vaultConfig).
				Build()
			reconciler.Client = client

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "condition-test-config",
					Namespace: "default",
				},
			}

			// First reconciliation
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			// Check initial condition
			var config1 vaultv1.VaultUnsealConfig
			err = client.Get(ctx, req.NamespacedName, &config1)
			Expect(err).ToNot(HaveOccurred())

			initialCondition := findCondition(config1.Status.Conditions, "Ready")
			Expect(initialCondition).ToNot(BeNil())
			Expect(initialCondition.ObservedGeneration).To(Equal(int64(1)))
			initialTransitionTime := initialCondition.LastTransitionTime

			// Wait a moment to ensure time difference
			time.Sleep(10 * time.Millisecond)

			// Second reconciliation (should update condition)
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			var config2 vaultv1.VaultUnsealConfig
			err = client.Get(ctx, req.NamespacedName, &config2)
			Expect(err).ToNot(HaveOccurred())

			secondCondition := findCondition(config2.Status.Conditions, "Ready")
			Expect(secondCondition).ToNot(BeNil())

			// Condition should be updated with new transition time
			Expect(secondCondition.LastTransitionTime.After(initialTransitionTime.Time)).To(BeTrue())
		})
	})

	Describe("Error Recovery", func() {
		It("should continue processing other instances when one fails", func() {
			vaultConfig := &vaultv1.VaultUnsealConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "error-recovery-config",
					Namespace: "default",
				},
				Spec: vaultv1.VaultUnsealConfigSpec{
					VaultInstances: []vaultv1.VaultInstance{
						{
							Name:       "vault-good",
							Endpoint:   "http://vault-good:8200",
							UnsealKeys: []string{"dGVzdA=="},
						},
						{
							Name:       "vault-bad",
							Endpoint:   "invalid-url-format",
							UnsealKeys: []string{"dGVzdA=="},
						},
						{
							Name:       "vault-another-good",
							Endpoint:   "http://vault-another:8200",
							UnsealKeys: []string{"dGVzdA=="},
						},
					},
				},
			}

			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(vaultConfig).
				WithStatusSubresource(vaultConfig).
				Build()
			reconciler.Client = client

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "error-recovery-config",
					Namespace: "default",
				},
			}

			// Reconcile should handle partial failures
			result, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(30 * time.Second))

			// Verify status includes all instances (including failed ones)
			var config vaultv1.VaultUnsealConfig
			err = client.Get(ctx, req.NamespacedName, &config)
			Expect(err).ToNot(HaveOccurred())

			Expect(len(config.Status.VaultStatuses)).To(Equal(3))

			// Find the status for the bad vault
			var badVaultStatus *vaultv1.VaultInstanceStatus
			for _, status := range config.Status.VaultStatuses {
				if status.Name == "vault-bad" {
					badVaultStatus = &status
					break
				}
			}

			Expect(badVaultStatus).ToNot(BeNil())
			Expect(badVaultStatus.Error).ToNot(BeEmpty())
			Expect(badVaultStatus.Sealed).To(BeTrue())

			// Ready condition should be false due to failures
			readyCondition := findCondition(config.Status.Conditions, "Ready")
			Expect(readyCondition).ToNot(BeNil())
			Expect(readyCondition.Status).To(Equal(metav1.ConditionFalse))
			Expect(readyCondition.Reason).To(Equal("SomeInstancesSealed"))
		})
	})

	Describe("Vault Client Management", func() {
		It("should reuse vault clients across reconciliations", func() {
			vaultConfig := &vaultv1.VaultUnsealConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "client-reuse-config",
					Namespace: "test-namespace",
				},
				Spec: vaultv1.VaultUnsealConfigSpec{
					VaultInstances: []vaultv1.VaultInstance{
						{
							Name:       "vault-1",
							Endpoint:   "http://vault:8200",
							UnsealKeys: []string{"dGVzdA=="},
						},
					},
				},
			}

			client := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(vaultConfig).
				WithStatusSubresource(vaultConfig).
				Build()
			reconciler.Client = client

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "client-reuse-config",
					Namespace: "test-namespace",
				},
			}

			// First reconciliation - should create client
			_, err := reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			// Verify client was created
			Expect(reconciler.vaultClients).ToNot(BeNil())
			Expect(len(reconciler.vaultClients)).To(Equal(1))

			clientKey := "test-namespace/vault-1"
			Expect(reconciler.vaultClients).To(HaveKey(clientKey))
			firstClient := reconciler.vaultClients[clientKey]

			// Second reconciliation - should reuse client
			_, err = reconciler.Reconcile(ctx, req)
			Expect(err).ToNot(HaveOccurred())

			// Client should be the same instance
			Expect(reconciler.vaultClients[clientKey]).To(BeIdenticalTo(firstClient))
		})
	})
})

// Helper functions
func findCondition(conditions []metav1.Condition, conditionType string) *metav1.Condition {
	for _, condition := range conditions {
		if condition.Type == conditionType {
			return &condition
		}
	}
	return nil
}

func intPtr(i int) *int {
	return &i
}
