package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// InterlinkClusterSpec defines the desired state of InterlinkCluster.
type InterlinkClusterSpec struct {
	// ControlPlaneEndpoint represents the endpoint used to communicate with the control plane.
	// For interlink-backed clusters this is typically the address of the management Kubernetes API server.
	// +optional
	ControlPlaneEndpoint clusterv1.APIEndpoint `json:"controlPlaneEndpoint,omitempty"`
}

// InterlinkClusterStatus defines the observed state of InterlinkCluster.
type InterlinkClusterStatus struct {
	// Ready denotes that the cluster infrastructure is ready.
	// +optional
	Ready bool `json:"ready,omitempty"`

	// Conditions defines current service state of the InterlinkCluster.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=interlinkclusters,scope=Namespaced,categories=cluster-api
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels['cluster\\.x-k8s\\.io/cluster-name']"
// +kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// InterlinkCluster is the Schema for the interlinkclusters API.
// It represents the infrastructure side of a Cluster API cluster backed by interlink virtual nodes.
type InterlinkCluster struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InterlinkClusterSpec   `json:"spec,omitempty"`
	Status InterlinkClusterStatus `json:"status,omitempty"`
}

// GetConditions returns the list of conditions for an InterlinkCluster API object.
func (c *InterlinkCluster) GetConditions() clusterv1.Conditions {
	return c.Status.Conditions
}

// SetConditions will set the given conditions on an InterlinkCluster object.
func (c *InterlinkCluster) SetConditions(conditions clusterv1.Conditions) {
	c.Status.Conditions = conditions
}

// +kubebuilder:object:root=true

// InterlinkClusterList contains a list of InterlinkCluster.
type InterlinkClusterList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []InterlinkCluster `json:"items"`
}

func init() {
	SchemeBuilder.Register(&InterlinkCluster{}, &InterlinkClusterList{})
}
