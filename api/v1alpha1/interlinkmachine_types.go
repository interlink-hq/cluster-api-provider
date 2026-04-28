package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
)

// InterlinkMachineSpec defines the desired state of InterlinkMachine.
type InterlinkMachineSpec struct {
	// ProviderID is the identifier for the InterlinkMachine in the provider namespace.
	// It will be set to interlink://<nodeName> once the VirtualNode is registered.
	// +optional
	ProviderID *string `json:"providerID,omitempty"`

	// NodeName is the name of the Kubernetes (virtual) node that will be created
	// by interlink for this machine. If not set, it defaults to the Machine name.
	// +optional
	NodeName string `json:"nodeName,omitempty"`

	// InterLinkAddress is the HTTP(S) address of the interLink API server that manages
	// the remote resource(s) behind this virtual node.
	// Required when PluginSpec is not set. When PluginSpec is provided and this field
	// is empty the address is derived automatically from the plugin Service.
	// +optional
	InterLinkAddress string `json:"interLinkAddress,omitempty"`

	// PluginSpec, when set, instructs the controller to spawn a Pod (and a matching
	// ClusterIP Service) that runs the interLink plugin binary on an existing virtual
	// node. The Service address is then used as the interLinkAddress for the VirtualNode,
	// enabling on-demand provisioning of interLink instances ("pilot" mode).
	// Either PluginSpec or InterLinkAddress must be provided.
	// +optional
	PluginSpec *PluginPodSpec `json:"pluginSpec,omitempty"`

	// VirtualKubeletImage is the container image to use for the virtual-kubelet pod
	// that implements this virtual node. Defaults to the project's official image.
	// +optional
	VirtualKubeletImage string `json:"virtualKubeletImage,omitempty"`

	// Resources describes the total capacity that will be advertised on the virtual node.
	// +optional
	Resources VirtualNodeResources `json:"resources,omitempty"`

	// Labels is a set of extra labels that will be applied to the virtual Kubernetes node.
	// +optional
	Labels map[string]string `json:"labels,omitempty"`

	// Taints is a list of taints to apply to the virtual Kubernetes node.
	// +optional
	Taints []corev1.Taint `json:"taints,omitempty"`
}

// PluginPodSpec describes a Pod that runs the interLink plugin binary on an existing
// virtual node. The controller creates the Pod and a ClusterIP Service, then uses the
// Service's address as the interLinkAddress for the VirtualNode.
type PluginPodSpec struct {
	// Image is the container image for the interLink plugin binary
	// (e.g. "ghcr.io/interlink-hq/interlink/plugin-apptainer:latest").
	// +kubebuilder:validation:Required
	Image string `json:"image"`

	// Port is the TCP port on which the plugin listens inside the container.
	// Defaults to 4000.
	// +optional
	// +kubebuilder:default=4000
	Port int32 `json:"port,omitempty"`

	// NodeSelector constrains the plugin Pod to nodes whose labels match the
	// given key/value pairs. Use this to target an existing interLink virtual
	// node that has access to the remote resource provider (e.g. an HPC cluster).
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Tolerations are applied to the plugin Pod so it can be scheduled on nodes
	// that carry matching taints (e.g. virtual-node.interlink.eu/NoSchedule).
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// Env is a list of environment variables to set in the plugin container.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`

	// Resources specifies the compute resources required by the plugin container.
	// +optional
	Resources corev1.ResourceRequirements `json:"resources,omitempty"`
}

// VirtualNodeResources describes the capacity of a virtual node.
type VirtualNodeResources struct {
	// CPU is the total CPU capacity of the virtual node (e.g. "10", "100m").
	// +optional
	CPU *resource.Quantity `json:"cpu,omitempty"`

	// Memory is the total memory capacity of the virtual node (e.g. "256Gi").
	// +optional
	Memory *resource.Quantity `json:"memory,omitempty"`

	// EphemeralStorage is the total ephemeral storage capacity of the virtual node.
	// +optional
	EphemeralStorage *resource.Quantity `json:"ephemeralStorage,omitempty"`

	// Pods is the maximum number of pods the virtual node can host.
	// +optional
	Pods *resource.Quantity `json:"pods,omitempty"`
}

// InterlinkMachineStatus defines the observed state of InterlinkMachine.
type InterlinkMachineStatus struct {
	// Ready is true when the provider resource is ready.
	// +optional
	Ready bool `json:"ready,omitempty"`

	// Addresses contains the associated addresses for the virtual node.
	// +optional
	Addresses []clusterv1.MachineAddress `json:"addresses,omitempty"`

	// Conditions defines current service state of the InterlinkMachine.
	// +optional
	Conditions clusterv1.Conditions `json:"conditions,omitempty"`

	// FailureReason will be set in the event that there is a terminal problem
	// reconciling the InterlinkMachine and will contain a succinct value suitable
	// for machine interpretation.
	// +optional
	FailureReason *string `json:"failureReason,omitempty"`

	// FailureMessage will be set in the event that there is a terminal problem
	// reconciling the InterlinkMachine and will contain a more verbose string suitable
	// for logging and human consumption.
	// +optional
	FailureMessage *string `json:"failureMessage,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=interlinkmachines,scope=Namespaced,categories=cluster-api
// +kubebuilder:printcolumn:name="Cluster",type="string",JSONPath=".metadata.labels['cluster\\.x-k8s\\.io/cluster-name']"
// +kubebuilder:printcolumn:name="Ready",type="boolean",JSONPath=".status.ready"
// +kubebuilder:printcolumn:name="ProviderID",type="string",JSONPath=".spec.providerID"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// InterlinkMachine is the Schema for the interlinkmachines API.
// Each InterlinkMachine corresponds to one interlink virtual node, allowing
// Cluster API (and its autoscaler integration) to manage the pool of virtual nodes.
type InterlinkMachine struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   InterlinkMachineSpec   `json:"spec,omitempty"`
	Status InterlinkMachineStatus `json:"status,omitempty"`
}

// GetConditions returns the list of conditions for an InterlinkMachine API object.
func (m *InterlinkMachine) GetConditions() clusterv1.Conditions {
	return m.Status.Conditions
}

// SetConditions will set the given conditions on an InterlinkMachine object.
func (m *InterlinkMachine) SetConditions(conditions clusterv1.Conditions) {
	m.Status.Conditions = conditions
}

// +kubebuilder:object:root=true

// InterlinkMachineList contains a list of InterlinkMachine.
type InterlinkMachineList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []InterlinkMachine `json:"items"`
}

func init() {
	SchemeBuilder.Register(&InterlinkMachine{}, &InterlinkMachineList{})
}
