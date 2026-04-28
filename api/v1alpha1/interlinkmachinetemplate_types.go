package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InterlinkMachineTemplateSpec defines the desired state of InterlinkMachineTemplate.
type InterlinkMachineTemplateSpec struct {
	Template InterlinkMachineTemplateResource `json:"template"`
}

// InterlinkMachineTemplateResource describes the data needed to create an InterlinkMachine from a template.
type InterlinkMachineTemplateResource struct {
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              InterlinkMachineSpec `json:"spec"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=interlinkmachinetemplates,scope=Namespaced,categories=cluster-api
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// InterlinkMachineTemplate is the Schema for the interlinkmachinetemplates API.
type InterlinkMachineTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec InterlinkMachineTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// InterlinkMachineTemplateList contains a list of InterlinkMachineTemplate.
type InterlinkMachineTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []InterlinkMachineTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&InterlinkMachineTemplate{}, &InterlinkMachineTemplateList{})
}
