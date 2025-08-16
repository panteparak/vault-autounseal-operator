package controller

import (
	"context"
	"testing"
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

func TestController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Controller Suite")
}

var _ = Describe("VaultUnsealConfigController", func() {
	var reconciler *VaultUnsealConfigReconciler
	var scheme *runtime.Scheme

	BeforeEach(func() {
		scheme = runtime.NewScheme()
		Expect(clientgoscheme.AddToScheme(scheme)).To(Succeed())
		Expect(vaultv1.AddToScheme(scheme)).To(Succeed())

		reconciler = &VaultUnsealConfigReconciler{
			Scheme: scheme,
			Log:    ctrl.Log.WithName("test"),
		}
	})

	Describe("Reconcile", func() {
		It("should handle non-existent resource gracefully", func() {
			client := fake.NewClientBuilder().WithScheme(scheme).Build()
			reconciler.Client = client

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      "non-existent",
					Namespace: "default",
				},
			}

			result, err := reconciler.Reconcile(context.Background(), req)
			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ctrl.Result{}))
		})

		It("should reconcile a valid VaultUnsealConfig", func() {
			vaultConfig := &vaultv1.VaultUnsealConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-config",
					Namespace: "default",
				},
				Spec: vaultv1.VaultUnsealConfigSpec{
					VaultInstances: []vaultv1.VaultInstance{
						{
							Name:          "vault-1",
							Endpoint:      "http://vault-1:8200",
							UnsealKeys:    []string{"dGVzdC1rZXk="}, // base64 encoded "test-key"
							TLSSkipVerify: true,
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
					Name:      "test-config",
					Namespace: "default",
				},
			}

			result, _ := reconciler.Reconcile(context.Background(), req)

			// We expect an error since we can't actually connect to vault in tests
			// but the reconciler should still update the status
			Expect(result.RequeueAfter).To(Equal(30 * time.Second))
		})
	})

	Describe("Input Validation and Edge Cases", func() {
		It("should handle empty vault instances list", func() {
			vaultConfig := &vaultv1.VaultUnsealConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "empty-config",
					Namespace: "default",
				},
				Spec: vaultv1.VaultUnsealConfigSpec{
					VaultInstances: []vaultv1.VaultInstance{},
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
					Name:      "empty-config",
					Namespace: "default",
				},
			}

			result, err := reconciler.Reconcile(context.Background(), req)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(30 * time.Second))
		})

		It("should handle vault instance with empty name", func() {
			vaultConfig := &vaultv1.VaultUnsealConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-name-config",
					Namespace: "default",
				},
				Spec: vaultv1.VaultUnsealConfigSpec{
					VaultInstances: []vaultv1.VaultInstance{
						{
							Name:       "", // Empty name
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
					Name:      "invalid-name-config",
					Namespace: "default",
				},
			}

			result, err := reconciler.Reconcile(context.Background(), req)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(30 * time.Second))
		})

		It("should handle vault instance with invalid endpoint", func() {
			vaultConfig := &vaultv1.VaultUnsealConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "invalid-endpoint-config",
					Namespace: "default",
				},
				Spec: vaultv1.VaultUnsealConfigSpec{
					VaultInstances: []vaultv1.VaultInstance{
						{
							Name:       "vault-1",
							Endpoint:   "not-a-url",
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
					Name:      "invalid-endpoint-config",
					Namespace: "default",
				},
			}

			result, err := reconciler.Reconcile(context.Background(), req)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(30 * time.Second))
		})

		It("should handle vault instance with no unseal keys", func() {
			vaultConfig := &vaultv1.VaultUnsealConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "no-keys-config",
					Namespace: "default",
				},
				Spec: vaultv1.VaultUnsealConfigSpec{
					VaultInstances: []vaultv1.VaultInstance{
						{
							Name:       "vault-1",
							Endpoint:   "http://vault:8200",
							UnsealKeys: []string{}, // No keys
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
					Name:      "no-keys-config",
					Namespace: "default",
				},
			}

			result, err := reconciler.Reconcile(context.Background(), req)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(30 * time.Second))
		})

		It("should handle vault instance with custom threshold", func() {
			threshold := 5
			vaultConfig := &vaultv1.VaultUnsealConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "custom-threshold-config",
					Namespace: "default",
				},
				Spec: vaultv1.VaultUnsealConfigSpec{
					VaultInstances: []vaultv1.VaultInstance{
						{
							Name:       "vault-1",
							Endpoint:   "http://vault:8200",
							UnsealKeys: []string{"a2V5MQ==", "a2V5Mg==", "a2V5Mw==", "a2V5NA==", "a2V5NQ==", "a2V5Ng=="},
							Threshold:  &threshold,
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
					Name:      "custom-threshold-config",
					Namespace: "default",
				},
			}

			result, err := reconciler.Reconcile(context.Background(), req)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(30 * time.Second))
		})

		It("should handle multiple vault instances with mixed configurations", func() {
			threshold2 := 2
			vaultConfig := &vaultv1.VaultUnsealConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "multi-vault-config",
					Namespace: "default",
				},
				Spec: vaultv1.VaultUnsealConfigSpec{
					VaultInstances: []vaultv1.VaultInstance{
						{
							Name:          "vault-1",
							Endpoint:      "http://vault-1:8200",
							UnsealKeys:    []string{"a2V5MQ==", "a2V5Mg==", "a2V5Mw=="},
							TLSSkipVerify: true,
						},
						{
							Name:       "vault-2",
							Endpoint:   "https://vault-2:8200",
							UnsealKeys: []string{"a2V5MQ==", "a2V5Mg=="},
							Threshold:  &threshold2,
						},
						{
							Name:       "vault-3",
							Endpoint:   "http://vault-3:8200",
							UnsealKeys: []string{"a2V5MQ=="},
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
					Name:      "multi-vault-config",
					Namespace: "default",
				},
			}

			result, err := reconciler.Reconcile(context.Background(), req)
			Expect(err).ToNot(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(30 * time.Second))
		})
	})

	Describe("Status and Condition Management", func() {
		It("should update status with correct conditions", func() {
			vaultConfig := &vaultv1.VaultUnsealConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "status-test-config",
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
					Name:      "status-test-config",
					Namespace: "default",
				},
			}

			_, _ = reconciler.Reconcile(context.Background(), req)

			// Verify the VaultUnsealConfig was updated
			var updatedConfig vaultv1.VaultUnsealConfig
			err := client.Get(context.Background(), req.NamespacedName, &updatedConfig)
			Expect(err).ToNot(HaveOccurred())

			// Should have at least one condition
			Expect(len(updatedConfig.Status.Conditions)).To(BeNumerically(">=", 1))

			// Should have Ready condition
			foundReady := false
			for _, condition := range updatedConfig.Status.Conditions {
				if condition.Type == "Ready" {
					foundReady = true
					break
				}
			}
			Expect(foundReady).To(BeTrue())
		})
	})
})
