package controller

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/go-logr/logr"
	vaultv1 "github.com/panteparak/vault-autounseal-operator/pkg/api/v1"
	"github.com/panteparak/vault-autounseal-operator/pkg/vault"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

const (
	// DefaultRequeueAfterSeconds is the default requeue time in seconds.
	DefaultRequeueAfterSeconds = 30
	// DefaultTimeoutSeconds is the default timeout in seconds.
	DefaultTimeoutSeconds = 30
	// DefaultThreshold is the default threshold for unsealing.
	DefaultThreshold = 3
)

// VaultClientRepository manages vault client instances.
type VaultClientRepository interface {
	GetClient(ctx context.Context, key string, instance *vaultv1.VaultInstance) (vault.VaultClient, error)
	Close() error
}

// ReconcilerOptions holds configuration for the reconciler.
type ReconcilerOptions struct {
	RequeueAfter time.Duration
	Timeout      time.Duration
}

// DefaultReconcilerOptions returns default reconciler options.
func DefaultReconcilerOptions() *ReconcilerOptions {
	return &ReconcilerOptions{
		RequeueAfter: DefaultRequeueAfterSeconds * time.Second,
		Timeout:      DefaultTimeoutSeconds * time.Second,
	}
}

// VaultUnsealConfigReconciler reconciles a VaultUnsealConfig object
type VaultUnsealConfigReconciler struct {
	client.Client
	Log              logr.Logger
	Scheme           *runtime.Scheme
	ClientRepository VaultClientRepository
	Options          *ReconcilerOptions
}

// NewVaultUnsealConfigReconciler creates a new reconciler with dependencies.
func NewVaultUnsealConfigReconciler(
	client client.Client,
	logger logr.Logger,
	scheme *runtime.Scheme,
	repository VaultClientRepository,
	options *ReconcilerOptions,
) *VaultUnsealConfigReconciler {
	if options == nil {
		options = DefaultReconcilerOptions()
	}

	return &VaultUnsealConfigReconciler{
		Client:           client,
		Log:              logger,
		Scheme:           scheme,
		ClientRepository: repository,
		Options:          options,
	}
}

// DefaultVaultClientRepository implements VaultClientRepository.
type DefaultVaultClientRepository struct {
	clients   map[string]*vault.Client
	clientsMu sync.RWMutex
	factory   vault.ClientFactory
}

// NewDefaultVaultClientRepository creates a new vault client repository.
func NewDefaultVaultClientRepository(factory vault.ClientFactory) *DefaultVaultClientRepository {
	if factory == nil {
		factory = &vault.DefaultClientFactory{}
	}

	return &DefaultVaultClientRepository{
		clients: make(map[string]*vault.Client),
		factory: factory,
	}
}

// +kubebuilder:rbac:groups=vault.io,resources=vaultunsealconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=vault.io,resources=vaultunsealconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=vault.io,resources=vaultunsealconfigs/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=events,verbs=create;patch

// GetClient retrieves or creates a vault client for the given instance.
func (r *DefaultVaultClientRepository) GetClient(
	_ context.Context,
	key string,
	instance *vaultv1.VaultInstance,
) (vault.VaultClient, error) {
	r.clientsMu.RLock()
	if client, exists := r.clients[key]; exists {
		r.clientsMu.RUnlock()

		return client, nil
	}
	r.clientsMu.RUnlock()

	r.clientsMu.Lock()
	defer r.clientsMu.Unlock()

	// Double-check after acquiring write lock
	if client, exists := r.clients[key]; exists {
		return client, nil
	}

	timeout := DefaultTimeoutSeconds * time.Second
	vaultClient, err := r.factory.NewClient(instance.Endpoint, instance.TLSSkipVerify, timeout)
	if err != nil {
		return nil, fmt.Errorf("failed to create vault client for %s: %w", key, err)
	}

	if concreteClient, ok := vaultClient.(*vault.Client); ok {
		r.clients[key] = concreteClient
	}

	return vaultClient, nil
}

// Close closes all vault clients in the repository.
func (r *DefaultVaultClientRepository) Close() error {
	r.clientsMu.Lock()
	defer r.clientsMu.Unlock()

	var lastErr error
	for key, client := range r.clients {
		if err := client.Close(); err != nil {
			lastErr = fmt.Errorf("failed to close client %s: %w", key, err)
		}
	}

	r.clients = make(map[string]*vault.Client)

	return lastErr
}

func (r *VaultUnsealConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("reconciler", "VaultUnsealConfig")

	// Create a timeout context for this reconciliation
	ctx, cancel := context.WithTimeout(ctx, r.Options.Timeout)
	defer cancel()

	// Fetch the VaultUnsealConfig instance
	var vaultConfig vaultv1.VaultUnsealConfig
	if err := r.Get(ctx, req.NamespacedName, &vaultConfig); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	logger.Info("Reconciling VaultUnsealConfig - Event-driven controller",
		"name", vaultConfig.Name,
		"namespace", vaultConfig.Namespace,
		"generation", vaultConfig.Generation,
		"instances", len(vaultConfig.Spec.VaultInstances),
		"note", "Triggered by VaultUnsealConfig or Pod events",
	)

	// Process each vault instance
	vaultStatuses, allReady := r.processVaultInstances(ctx, logger, &vaultConfig)

	// Update status
	r.updateVaultConfigStatus(&vaultConfig, vaultStatuses, allReady)

	// Update the status
	if err := r.Status().Update(ctx, &vaultConfig); err != nil {
		logger.Error(err, "unable to update VaultUnsealConfig status")

		return ctrl.Result{}, fmt.Errorf("failed to update status: %w", err)
	}

	logger.V(1).Info("Reconciliation completed", "allReady", allReady, "statuses", len(vaultStatuses))

	// Requeue for periodic reconciliation
	return ctrl.Result{RequeueAfter: r.Options.RequeueAfter}, nil
}

func (r *VaultUnsealConfigReconciler) processVaultInstances(
	ctx context.Context,
	logger logr.Logger,
	vaultConfig *vaultv1.VaultUnsealConfig,
) ([]vaultv1.VaultInstanceStatus, bool) {
	vaultStatuses := make([]vaultv1.VaultInstanceStatus, 0, len(vaultConfig.Spec.VaultInstances))
	allReady := true

	for i := range vaultConfig.Spec.VaultInstances {
		instance := &vaultConfig.Spec.VaultInstances[i]
		instanceLogger := logger.WithValues("instance", instance.Name, "endpoint", instance.Endpoint)

		status, err := r.processVaultInstance(ctx, instanceLogger, instance, vaultConfig.Namespace)
		if err != nil {
			instanceLogger.Error(err, "failed to process vault instance")
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

	return vaultStatuses, allReady
}

func (r *VaultUnsealConfigReconciler) updateVaultConfigStatus(
	vaultConfig *vaultv1.VaultUnsealConfig,
	vaultStatuses []vaultv1.VaultInstanceStatus,
	allReady bool,
) {
	vaultConfig.Status.VaultStatuses = vaultStatuses

	// Count sealed instances for better messaging
	sealedCount := 0
	for _, status := range vaultStatuses {
		if status.Sealed {
			sealedCount++
		}
	}

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
		condition.Message = fmt.Sprintf("%d of %d vault instances are sealed",
			sealedCount, len(vaultConfig.Spec.VaultInstances))
	}

	// Update or append condition
	r.updateCondition(vaultConfig, &condition)
}

func (r *VaultUnsealConfigReconciler) updateCondition(
	vaultConfig *vaultv1.VaultUnsealConfig,
	condition *metav1.Condition,
) {
	updated := false
	for i, existingCondition := range vaultConfig.Status.Conditions {
		if existingCondition.Type == condition.Type {
			vaultConfig.Status.Conditions[i] = *condition
			updated = true
			break
		}
	}
	if !updated {
		vaultConfig.Status.Conditions = append(vaultConfig.Status.Conditions, *condition)
	}
}

func (r *VaultUnsealConfigReconciler) processVaultInstance(
	ctx context.Context,
	logger logr.Logger,
	instance *vaultv1.VaultInstance,
	namespace string,
) (vaultv1.VaultInstanceStatus, error) {
	clientKey := fmt.Sprintf("%s/%s", namespace, instance.Name)

	// Get or create vault client using the repository
	vaultClient, err := r.ClientRepository.GetClient(ctx, clientKey, instance)
	if err != nil {
		return vaultv1.VaultInstanceStatus{}, fmt.Errorf("failed to get vault client: %w", err)
	}

	// Check if vault is sealed
	isSealed, err := vaultClient.IsSealed(ctx)
	if err != nil {
		return vaultv1.VaultInstanceStatus{}, fmt.Errorf("failed to check seal status: %w", err)
	}

	logger.V(1).Info("Vault seal status checked", "sealed", isSealed)

	status := vaultv1.VaultInstanceStatus{
		Name:   instance.Name,
		Sealed: isSealed,
	}

	// If sealed, attempt to unseal
	if isSealed {
		threshold := getThreshold(instance)
		logger.Info("Attempting to unseal vault", "threshold", threshold, "keyCount", len(instance.UnsealKeys))

		sealStatus, err := vaultClient.Unseal(ctx, instance.UnsealKeys, threshold)
		if err != nil {
			return vaultv1.VaultInstanceStatus{}, fmt.Errorf("failed to unseal vault: %w", err)
		}

		status.Sealed = sealStatus.Sealed
		if !sealStatus.Sealed {
			now := metav1.NewTime(time.Now())
			status.LastUnsealed = &now
			logger.Info("Vault successfully unsealed")
		} else {
			logger.Info("Vault remains sealed after unseal attempt",
				"progress", sealStatus.Progress, "required", sealStatus.T)
		}
	} else {
		// Already unsealed - update last unsealed time
		now := metav1.NewTime(time.Now())
		status.LastUnsealed = &now
		logger.V(1).Info("Vault is already unsealed")
	}

	return status, nil
}

// getThreshold returns the threshold value, defaulting to 3 if not set.
func getThreshold(instance *vaultv1.VaultInstance) int {
	if instance.Threshold != nil {
		return *instance.Threshold
	}

	return DefaultThreshold
}

// SetupWithManager sets up the controller with the Manager.
func (r *VaultUnsealConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&vaultv1.VaultUnsealConfig{}).
		Watches(
			&corev1.Pod{},
			handler.EnqueueRequestsFromMapFunc(r.findVaultConfigsForPod),
		).
		Complete(r)
}

// findVaultConfigsForPod finds VaultUnsealConfigs that should be reconciled when a pod changes
func (r *VaultUnsealConfigReconciler) findVaultConfigsForPod(ctx context.Context, obj client.Object) []reconcile.Request {
	pod, ok := obj.(*corev1.Pod)
	if !ok {
		return []reconcile.Request{}
	}

	// Check if this is a Vault pod by looking at labels
	if !r.isVaultPod(pod) {
		return []reconcile.Request{}
	}

	logger := r.Log.WithValues("pod", pod.Name, "namespace", pod.Namespace)

	// List all VaultUnsealConfigs
	var configs vaultv1.VaultUnsealConfigList
	if err := r.List(ctx, &configs); err != nil {
		logger.Error(err, "failed to list VaultUnsealConfigs")
		return []reconcile.Request{}
	}

	var requests []reconcile.Request
	for _, config := range configs.Items {
		if r.configMatchesPod(&config, pod) {
			logger.V(1).Info("Pod event triggers VaultUnsealConfig reconciliation",
				"config", config.Name,
				"podPhase", pod.Status.Phase)

			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      config.Name,
					Namespace: config.Namespace,
				},
			})
		}
	}

	return requests
}

// isVaultPod checks if a pod is a Vault pod based on labels
func (r *VaultUnsealConfigReconciler) isVaultPod(pod *corev1.Pod) bool {
	if pod.Labels == nil {
		return false
	}

	// Check for common Vault pod labels
	appName := pod.Labels["app.kubernetes.io/name"]
	component := pod.Labels["app.kubernetes.io/component"]

	// Match various Vault deployment patterns
	return appName == "vault" ||
		component == "server" ||
		strings.Contains(pod.Name, "vault")
}

// configMatchesPod checks if a VaultUnsealConfig should be reconciled for this pod
func (r *VaultUnsealConfigReconciler) configMatchesPod(config *vaultv1.VaultUnsealConfig, pod *corev1.Pod) bool {
	for _, instance := range config.Spec.VaultInstances {
		// Check if pod matches the instance configuration
		if r.podMatchesInstance(pod, &instance) {
			return true
		}
	}
	return false
}

// podMatchesInstance checks if a pod matches a specific vault instance configuration
func (r *VaultUnsealConfigReconciler) podMatchesInstance(pod *corev1.Pod, instance *vaultv1.VaultInstance) bool {
	// If instance specifies a namespace and it doesn't match, skip
	if instance.Namespace != "" && instance.Namespace != pod.Namespace {
		return false
	}

	// If instance has pod selector, use it
	if len(instance.PodSelector) > 0 {
		return r.podMatchesSelector(pod, instance.PodSelector)
	}

	// Default matching: check if pod looks like a vault pod
	return r.isVaultPod(pod)
}

// podMatchesSelector checks if a pod matches the given label selector
func (r *VaultUnsealConfigReconciler) podMatchesSelector(pod *corev1.Pod, selector map[string]string) bool {
	if pod.Labels == nil {
		return false
	}

	for key, value := range selector {
		if podValue, exists := pod.Labels[key]; !exists || podValue != value {
			return false
		}
	}
	return true
}
