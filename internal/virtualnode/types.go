// Package virtualnode provides types and helpers for interacting with the
// interlink VirtualNode custom resource.
package virtualnode

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	// GroupVersion is the group/version for the interlink VirtualNode resource.
	GroupVersion = schema.GroupVersion{Group: "virtualkubelet.io", Version: "v1alpha1"}

	// SchemeBuilder registers the VirtualNode types.
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)

	// AddToScheme adds VirtualNode types to a scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion,
		&VirtualNode{},
		&VirtualNodeList{},
	)
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}

// VirtualNodeSpec defines the desired state of VirtualNode.
type VirtualNodeSpec struct {
	// NodeName is the name of the Kubernetes virtual node.
	NodeName string `json:"nodeName"`

	// InterLinkAddress is the HTTP(S) address of the interLink API server.
	InterLinkAddress string `json:"interLinkAddress"`

	// CPU is the total CPU capacity to advertise on the virtual node.
	// +optional
	CPU *resource.Quantity `json:"cpu,omitempty"`

	// Memory is the total memory capacity to advertise on the virtual node.
	// +optional
	Memory *resource.Quantity `json:"memory,omitempty"`

	// EphemeralStorage is the total ephemeral storage capacity.
	// +optional
	EphemeralStorage *resource.Quantity `json:"ephemeralStorage,omitempty"`

	// Pods is the maximum number of pods the virtual node can host.
	// +optional
	Pods *resource.Quantity `json:"pods,omitempty"`

	// Labels is a set of extra labels to apply to the virtual Kubernetes node.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Taints is a list of taints to apply to the virtual Kubernetes node.
	// +optional
	Taints []corev1.Taint `json:"taints,omitempty"`
}

// VirtualNodeStatus defines the observed state of VirtualNode.
type VirtualNodeStatus struct {
	// Conditions holds conditions for the VirtualNode.
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// VirtualNode is the interlink custom resource that configures a virtual Kubernetes node.
type VirtualNode struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VirtualNodeSpec   `json:"spec,omitempty"`
	Status VirtualNodeStatus `json:"status,omitempty"`
}

// DeepCopyObject implements runtime.Object.
func (v *VirtualNode) DeepCopyObject() runtime.Object {
	if v == nil {
		return nil
	}
	out := v.DeepCopy()
	return out
}

// DeepCopy creates a deep copy of VirtualNode.
func (v *VirtualNode) DeepCopy() *VirtualNode {
	if v == nil {
		return nil
	}
	out := new(VirtualNode)
	v.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies VirtualNode into out.
func (v *VirtualNode) DeepCopyInto(out *VirtualNode) {
	*out = *v
	out.TypeMeta = v.TypeMeta
	v.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	v.Spec.DeepCopyInto(&out.Spec)
	v.Status.DeepCopyInto(&out.Status)
}

// DeepCopyInto copies VirtualNodeSpec into out.
func (s *VirtualNodeSpec) DeepCopyInto(out *VirtualNodeSpec) {
	*out = *s
	if s.CPU != nil {
		x := s.CPU.DeepCopy()
		out.CPU = &x
	}
	if s.Memory != nil {
		x := s.Memory.DeepCopy()
		out.Memory = &x
	}
	if s.EphemeralStorage != nil {
		x := s.EphemeralStorage.DeepCopy()
		out.EphemeralStorage = &x
	}
	if s.Pods != nil {
		x := s.Pods.DeepCopy()
		out.Pods = &x
	}
	if s.Labels != nil {
		out.Labels = make(map[string]string, len(s.Labels))
		for k, v := range s.Labels {
			out.Labels[k] = v
		}
	}
	if s.Taints != nil {
		out.Taints = make([]corev1.Taint, len(s.Taints))
		for i := range s.Taints {
			s.Taints[i].DeepCopyInto(&out.Taints[i])
		}
	}
}

// DeepCopyInto copies VirtualNodeStatus into out.
func (s *VirtualNodeStatus) DeepCopyInto(out *VirtualNodeStatus) {
	*out = *s
	if s.Conditions != nil {
		out.Conditions = make([]metav1.Condition, len(s.Conditions))
		for i := range s.Conditions {
			s.Conditions[i].DeepCopyInto(&out.Conditions[i])
		}
	}
}

// VirtualNodeList contains a list of VirtualNode.
type VirtualNodeList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []VirtualNode `json:"items"`
}

// DeepCopyObject implements runtime.Object.
func (vl *VirtualNodeList) DeepCopyObject() runtime.Object {
	if vl == nil {
		return nil
	}
	out := new(VirtualNodeList)
	vl.DeepCopyInto(out)
	return out
}

// DeepCopyInto copies VirtualNodeList into out.
func (vl *VirtualNodeList) DeepCopyInto(out *VirtualNodeList) {
	*out = *vl
	out.TypeMeta = vl.TypeMeta
	vl.ListMeta.DeepCopyInto(&out.ListMeta)
	if vl.Items != nil {
		out.Items = make([]VirtualNode, len(vl.Items))
		for i := range vl.Items {
			vl.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}
