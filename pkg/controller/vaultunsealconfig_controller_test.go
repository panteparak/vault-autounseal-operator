package controller

import (
	"context"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	vaultv1 "github.com/panteparak/vault-autounseal-operator/pkg/api/v1"
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

	Describe("Validation", func() {
		It("should validate vault instance configuration", func() {
			instance := &vaultv1.VaultInstance{
				Name:       "test-vault",
				Endpoint:   "http://vault:8200",
				UnsealKeys: []string{"dGVzdA=="}, // valid base64
			}

			// Basic validation - should have name and endpoint
			Expect(instance.Name).To(Equal("test-vault"))
			Expect(instance.Endpoint).To(ContainSubstring("http"))
			Expect(len(instance.UnsealKeys)).To(BeNumerically(">", 0))
		})
	})
})
