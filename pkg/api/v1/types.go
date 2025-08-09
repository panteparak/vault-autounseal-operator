package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// +kubebuilder:object:root=true
// +kubebuilder:object:generate=true
// +kubebuilder:subresource:status

// VaultUnsealConfig is the Schema for the vaultunsealconfigs API
type VaultUnsealConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VaultUnsealConfigSpec   `json:"spec,omitempty"`
	Status VaultUnsealConfigStatus `json:"status,omitempty"`
}

// DeepCopyObject returns a deep copy of the object
func (v *VaultUnsealConfig) DeepCopyObject() runtime.Object {
	if c := v.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopy returns a deep copy of VaultUnsealConfig
func (v *VaultUnsealConfig) DeepCopy() *VaultUnsealConfig {
	if v == nil {
		return nil
	}
	out := new(VaultUnsealConfig)
	v.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies all fields from this object into another
func (v *VaultUnsealConfig) DeepCopyInto(out *VaultUnsealConfig) {
	*out = *v
	out.TypeMeta = v.TypeMeta
	v.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	v.Spec.DeepCopyInto(&out.Spec)
	v.Status.DeepCopyInto(&out.Status)
}

// VaultUnsealConfigSpec defines the desired state of VaultUnsealConfig
type VaultUnsealConfigSpec struct {
	// VaultInstances is a list of vault instances to manage
	VaultInstances []VaultInstance `json:"vaultInstances"`
}

// VaultInstance represents a single Vault instance configuration
type VaultInstance struct {
	// Name is the unique identifier for this vault instance
	Name string `json:"name"`

	// Endpoint is the URL of the vault instance
	Endpoint string `json:"endpoint"`

	// UnsealKeys is a list of unseal keys for this instance
	UnsealKeys []string `json:"unsealKeys"`

	// Threshold is the number of unseal keys required (default: 3)
	// +optional
	Threshold *int `json:"threshold,omitempty"`

	// TLSSkipVerify disables TLS certificate verification (default: false)
	// +optional
	TLSSkipVerify bool `json:"tlsSkipVerify,omitempty"`

	// HAEnabled indicates if this is a HA setup (default: false)
	// +optional
	HAEnabled bool `json:"haEnabled,omitempty"`

	// PodSelector selects pods to monitor for HA setups
	// +optional
	PodSelector map[string]string `json:"podSelector,omitempty"`

	// Namespace is the target namespace for pod monitoring
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// VaultUnsealConfigStatus defines the observed state of VaultUnsealConfig
type VaultUnsealConfigStatus struct {
	// Conditions represent the latest available observations
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// VaultStatuses shows the status of each vault instance
	// +optional
	VaultStatuses []VaultInstanceStatus `json:"vaultStatuses,omitempty"`
}

// VaultInstanceStatus represents the status of a single vault instance
type VaultInstanceStatus struct {
	// Name of the vault instance
	Name string `json:"name"`

	// Sealed indicates if the vault is sealed
	Sealed bool `json:"sealed"`

	// LastUnsealed is the timestamp of the last successful unseal operation
	// +optional
	LastUnsealed *metav1.Time `json:"lastUnsealed,omitempty"`

	// Error contains any error message from the last operation
	// +optional
	Error string `json:"error,omitempty"`
}

// +kubebuilder:object:root=true

// VaultUnsealConfigList contains a list of VaultUnsealConfig
type VaultUnsealConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VaultUnsealConfig `json:"items"`
}

// DeepCopyObject returns a deep copy of the object
func (v *VaultUnsealConfigList) DeepCopyObject() runtime.Object {
	if c := v.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopy returns a deep copy of VaultUnsealConfigList
func (v *VaultUnsealConfigList) DeepCopy() *VaultUnsealConfigList {
	if v == nil {
		return nil
	}
	out := new(VaultUnsealConfigList)
	v.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies all fields from this object into another
func (v *VaultUnsealConfigList) DeepCopyInto(out *VaultUnsealConfigList) {
	*out = *v
	out.TypeMeta = v.TypeMeta
	v.ListMeta.DeepCopyInto(&out.ListMeta)
	if v.Items != nil {
		in, out := &v.Items, &out.Items
		*out = make([]VaultUnsealConfig, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopyInto copies all fields from this object into another
func (v *VaultUnsealConfigSpec) DeepCopyInto(out *VaultUnsealConfigSpec) {
	*out = *v
	if v.VaultInstances != nil {
		in, out := &v.VaultInstances, &out.VaultInstances
		*out = make([]VaultInstance, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy returns a deep copy of VaultUnsealConfigSpec
func (v *VaultUnsealConfigSpec) DeepCopy() *VaultUnsealConfigSpec {
	if v == nil {
		return nil
	}
	out := new(VaultUnsealConfigSpec)
	v.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies all fields from this object into another
func (v *VaultInstance) DeepCopyInto(out *VaultInstance) {
	*out = *v
	if v.Threshold != nil {
		in, out := &v.Threshold, &out.Threshold
		*out = new(int)
		**out = **in
	}
	if v.UnsealKeys != nil {
		in, out := &v.UnsealKeys, &out.UnsealKeys
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if v.PodSelector != nil {
		in, out := &v.PodSelector, &out.PodSelector
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy returns a deep copy of VaultInstance
func (v *VaultInstance) DeepCopy() *VaultInstance {
	if v == nil {
		return nil
	}
	out := new(VaultInstance)
	v.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies all fields from this object into another
func (v *VaultUnsealConfigStatus) DeepCopyInto(out *VaultUnsealConfigStatus) {
	*out = *v
	if v.Conditions != nil {
		in, out := &v.Conditions, &out.Conditions
		*out = make([]metav1.Condition, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if v.VaultStatuses != nil {
		in, out := &v.VaultStatuses, &out.VaultStatuses
		*out = make([]VaultInstanceStatus, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy returns a deep copy of VaultUnsealConfigStatus
func (v *VaultUnsealConfigStatus) DeepCopy() *VaultUnsealConfigStatus {
	if v == nil {
		return nil
	}
	out := new(VaultUnsealConfigStatus)
	v.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies all fields from this object into another
func (v *VaultInstanceStatus) DeepCopyInto(out *VaultInstanceStatus) {
	*out = *v
	if v.LastUnsealed != nil {
		in, out := &v.LastUnsealed, &out.LastUnsealed
		*out = (*in).DeepCopy()
	}
}

// DeepCopy returns a deep copy of VaultInstanceStatus
func (v *VaultInstanceStatus) DeepCopy() *VaultInstanceStatus {
	if v == nil {
		return nil
	}
	out := new(VaultInstanceStatus)
	v.DeepCopyInto(out)
	return out
}
