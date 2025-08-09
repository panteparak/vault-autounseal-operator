package controller

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	vaultv1 "github.com/panteparak/vault-autounseal-operator/pkg/api/v1"
	"github.com/panteparak/vault-autounseal-operator/pkg/vault"
)

// VaultUnsealConfigReconciler reconciles a VaultUnsealConfig object
type VaultUnsealConfigReconciler struct {
	client.Client
	Log    logr.Logger
	Scheme *runtime.Scheme

	vaultClients map[string]*vault.Client
}

// +kubebuilder:rbac:groups=vault.io,resources=vaultunsealconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vault.io,resources=vaultunsealconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vault.io,resources=vaultunsealconfigs/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch

func (r *VaultUnsealConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Fetch the VaultUnsealConfig instance
	var vaultConfig vaultv1.VaultUnsealConfig
	if err := r.Get(ctx, req.NamespacedName, &vaultConfig); err != nil {
		logger.Error(err, "unable to fetch VaultUnsealConfig")
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Initialize vault clients map if needed
	if r.vaultClients == nil {
		r.vaultClients = make(map[string]*vault.Client)
	}

	logger.Info("Reconciling VaultUnsealConfig", "name", vaultConfig.Name, "namespace", vaultConfig.Namespace)

	// Process each vault instance
	var vaultStatuses []vaultv1.VaultInstanceStatus
	allReady := true

	for _, instance := range vaultConfig.Spec.VaultInstances {
		status, err := r.processVaultInstance(ctx, &instance, vaultConfig.Namespace)
		if err != nil {
			logger.Error(err, "failed to process vault instance", "instance", instance.Name)
			status = vaultv1.VaultInstanceStatus{
				Name:   instance.Name,
				Sealed: true,
				Error:  err.Error(),
			}
			allReady = false
		}
		if status.Sealed {
			allReady = false
		}
		vaultStatuses = append(vaultStatuses, status)
	}

	// Update status
	vaultConfig.Status.VaultStatuses = vaultStatuses

	// Update conditions
	condition := metav1.Condition{
		Type:               "Ready",
		LastTransitionTime: metav1.NewTime(time.Now()),
		ObservedGeneration: vaultConfig.Generation,
	}

	if allReady {
		condition.Status = metav1.ConditionTrue
		condition.Reason = "AllInstancesUnsealed"
		condition.Message = fmt.Sprintf("All %d vault instances are unsealed", len(vaultConfig.Spec.VaultInstances))
	} else {
		condition.Status = metav1.ConditionFalse
		condition.Reason = "SomeInstancesSealed"
		condition.Message = fmt.Sprintf("%d of %d vault instances need attention", len(vaultStatuses), len(vaultConfig.Spec.VaultInstances))
	}

	// Update or append condition
	updated := false
	for i, existingCondition := range vaultConfig.Status.Conditions {
		if existingCondition.Type == condition.Type {
			vaultConfig.Status.Conditions[i] = condition
			updated = true
			break
		}
	}
	if !updated {
		vaultConfig.Status.Conditions = append(vaultConfig.Status.Conditions, condition)
	}

	// Update the status
	if err := r.Status().Update(ctx, &vaultConfig); err != nil {
		logger.Error(err, "unable to update VaultUnsealConfig status")
		return ctrl.Result{}, err
	}

	// Requeue for periodic reconciliation
	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *VaultUnsealConfigReconciler) processVaultInstance(ctx context.Context, instance *vaultv1.VaultInstance, namespace string) (vaultv1.VaultInstanceStatus, error) {
	clientKey := fmt.Sprintf("%s/%s", namespace, instance.Name)

	// Get or create vault client
	vaultClient, exists := r.vaultClients[clientKey]
	if !exists {
		timeout := 30 * time.Second
		var err error
		vaultClient, err = vault.NewClient(instance.Endpoint, instance.TLSSkipVerify, timeout)
		if err != nil {
			return vaultv1.VaultInstanceStatus{}, fmt.Errorf("failed to create vault client: %w", err)
		}
		r.vaultClients[clientKey] = vaultClient
	}

	// Check if vault is sealed
	isSealed, err := vaultClient.IsSealed(ctx)
	if err != nil {
		return vaultv1.VaultInstanceStatus{}, fmt.Errorf("failed to check seal status: %w", err)
	}

	status := vaultv1.VaultInstanceStatus{
		Name:   instance.Name,
		Sealed: isSealed,
	}

	// If sealed, attempt to unseal
	if isSealed {
		threshold := 3
		if instance.Threshold != nil {
			threshold = *instance.Threshold
		}

		sealStatus, err := vaultClient.Unseal(ctx, instance.UnsealKeys, threshold)
		if err != nil {
			return vaultv1.VaultInstanceStatus{}, fmt.Errorf("failed to unseal vault: %w", err)
		}

		status.Sealed = sealStatus.Sealed
		if !sealStatus.Sealed {
			now := metav1.NewTime(time.Now())
			status.LastUnsealed = &now
		}
	} else {
		// Already unsealed
		now := metav1.NewTime(time.Now())
		status.LastUnsealed = &now
	}

	return status, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *VaultUnsealConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vaultv1.VaultUnsealConfig{}).
		Complete(r)
}
