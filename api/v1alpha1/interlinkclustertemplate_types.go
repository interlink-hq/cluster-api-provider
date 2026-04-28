package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// InterlinkClusterTemplateSpec defines the desired state of InterlinkClusterTemplate.
type InterlinkClusterTemplateSpec struct {
	Template InterlinkClusterTemplateResource `json:"template"`
}

// InterlinkClusterTemplateResource describes the data needed to create an InterlinkCluster from a template.
type InterlinkClusterTemplateResource struct {
	// +optional
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              InterlinkClusterSpec `json:"spec"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:path=interlinkclustertemplates,scope=Namespaced,categories=cluster-api
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// InterlinkClusterTemplate is the Schema for the interlinkclustertemplates API.
type InterlinkClusterTemplate struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec InterlinkClusterTemplateSpec `json:"spec,omitempty"`
}

// +kubebuilder:object:root=true

// InterlinkClusterTemplateList contains a list of InterlinkClusterTemplate.
type InterlinkClusterTemplateList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []InterlinkClusterTemplate `json:"items"`
}

func init() {
	SchemeBuilder.Register(&InterlinkClusterTemplate{}, &InterlinkClusterTemplateList{})
}
