package v1alpha1

import clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

const (
	// VirtualNodeReadyCondition reports the state of the interlink VirtualNode resource.
	VirtualNodeReadyCondition clusterv1.ConditionType = "VirtualNodeReady"

	// VirtualNodeProvisioningReason is used when the VirtualNode is being created.
	VirtualNodeProvisioningReason = "VirtualNodeProvisioning"

	// VirtualNodeNotReadyReason is used when the virtual Kubernetes node has not yet reported Ready.
	VirtualNodeNotReadyReason = "VirtualNodeNotReady"

	// VirtualNodeDeletingReason is used when the VirtualNode is being deleted.
	VirtualNodeDeletingReason = "VirtualNodeDeleting"

	// PluginPodReadyCondition reports whether the pilot plugin Pod (created when
	// PluginSpec is set) is Running and ready to serve interLink requests.
	PluginPodReadyCondition clusterv1.ConditionType = "PluginPodReady"

	// PluginPodProvisioningReason is used when the plugin Pod or its Service is
	// being created or is not yet Running.
	PluginPodProvisioningReason = "PluginPodProvisioning"

	// PluginPodNotReadyReason is used when the plugin Pod exists but has not yet
	// reached the Running phase.
	PluginPodNotReadyReason = "PluginPodNotReady"
)
