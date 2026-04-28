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
)
