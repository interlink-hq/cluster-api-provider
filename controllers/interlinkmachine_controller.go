package controllers

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	infrav1 "github.com/interlink-hq/cluster-api-provider/api/v1alpha1"
	"github.com/interlink-hq/cluster-api-provider/internal/virtualnode"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
)

const (
	interlinkMachineFinalizer = "interlinkmachine.infrastructure.cluster.x-k8s.io"

	// defaultVirtualKubeletImage is used when the user does not specify a custom image.
	defaultVirtualKubeletImage = "ghcr.io/interTwin-eu/interlink/virtual-kubelet:latest"

	// providerIDPrefix is prepended to the node name to form the CAPI provider ID.
	providerIDPrefix = "interlink://"
)

// InterlinkMachineReconciler reconciles InterlinkMachine objects.
//
// Each InterlinkMachine corresponds to one interlink VirtualNode resource.  The
// reconciler creates and deletes VirtualNode objects in response to Machine
// lifecycle events, and reflects the readiness of the underlying Kubernetes node
// back into the InterlinkMachine status.
//
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=interlinkmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=interlinkmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=interlinkmachines/finalizers,verbs=update
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=virtualkubelet.io,resources=virtualnodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
type InterlinkMachineReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

// SetupWithManager sets up the InterlinkMachineReconciler with the controller manager.
func (r *InterlinkMachineReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.InterlinkMachine{}).
		// Also watch Nodes so that when a virtual node becomes Ready we can update the Machine status.
		Watches(
			&corev1.Node{},
			handler.EnqueueRequestsFromMapFunc(r.nodeToInterlinkMachine),
		).
		WithEventFilter(predicates.ResourceNotPaused(r.Log)).
		Complete(r)
}

// nodeToInterlinkMachine maps a Node event to the owning InterlinkMachine by matching
// the node name against the InterlinkMachine spec.nodeName.
func (r *InterlinkMachineReconciler) nodeToInterlinkMachine(ctx context.Context, obj client.Object) []ctrl.Request {
	node, ok := obj.(*corev1.Node)
	if !ok {
		return nil
	}

	machineList := &infrav1.InterlinkMachineList{}
	if err := r.List(ctx, machineList); err != nil {
		return nil
	}

	var requests []ctrl.Request
	for i := range machineList.Items {
		m := &machineList.Items[i]
		nodeName := m.Spec.NodeName
		if nodeName == "" {
			nodeName = m.Name
		}
		if nodeName == node.Name {
			requests = append(requests, ctrl.Request{
				NamespacedName: types.NamespacedName{
					Namespace: m.Namespace,
					Name:      m.Name,
				},
			})
		}
	}
	return requests
}

// Reconcile reconciles an InterlinkMachine.
func (r *InterlinkMachineReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	log := r.Log.WithValues("interlinkmachine", req.NamespacedName)

	// Fetch the InterlinkMachine object.
	interlinkMachine := &infrav1.InterlinkMachine{}
	if err := r.Get(ctx, req.NamespacedName, interlinkMachine); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Fetch the owning Machine.
	machine, err := util.GetOwnerMachine(ctx, r.Client, interlinkMachine.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if machine == nil {
		log.Info("Machine Controller has not yet set OwnerRef; re-queuing")
		return ctrl.Result{}, nil
	}

	// Fetch the Cluster the machine belongs to.
	cluster, err := util.GetClusterFromMetadata(ctx, r.Client, machine.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get cluster for machine %s: %w", machine.Name, err)
	}

	// Return early if paused.
	if annotations.IsPaused(cluster, interlinkMachine) {
		log.Info("InterlinkMachine or owning Cluster is paused; skipping reconciliation")
		return ctrl.Result{}, nil
	}

	// Initialize patch helper.
	patchHelper, err := patch.NewHelper(interlinkMachine, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Always patch status at the end.
	defer func() {
		if err := patchInterlinkMachine(ctx, patchHelper, interlinkMachine); err != nil {
			// The object may have been garbage-collected by the API server after the
			// last finalizer was removed; treat not-found as a no-op.
			if isNotFound(err) {
				return
			}
			log.Error(err, "failed to patch InterlinkMachine")
			if reterr == nil {
				reterr = err
			}
		}
	}()

	// Handle deletion.
	if !interlinkMachine.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, log, interlinkMachine)
	}

	return r.reconcileNormal(ctx, log, interlinkMachine)
}

func (r *InterlinkMachineReconciler) reconcileNormal(
	ctx context.Context,
	log logr.Logger,
	interlinkMachine *infrav1.InterlinkMachine,
) (ctrl.Result, error) {
	// Add finalizer.
	if !controllerutil.ContainsFinalizer(interlinkMachine, interlinkMachineFinalizer) {
		controllerutil.AddFinalizer(interlinkMachine, interlinkMachineFinalizer)
	}

	// Determine the virtual node name.
	nodeName := interlinkMachine.Spec.NodeName
	if nodeName == "" {
		nodeName = interlinkMachine.Name
	}

	// Create or update the VirtualNode resource.
	if err := r.reconcileVirtualNode(ctx, log, interlinkMachine, nodeName); err != nil {
		conditions.MarkFalse(
			interlinkMachine,
			infrav1.VirtualNodeReadyCondition,
			infrav1.VirtualNodeProvisioningReason,
			clusterv1.ConditionSeverityWarning,
			"%s", err.Error(),
		)
		return ctrl.Result{}, err
	}

	// Check if the corresponding Kubernetes Node is Ready.
	ready, addresses, err := r.isNodeReady(ctx, nodeName)
	if err != nil {
		return ctrl.Result{}, err
	}

	if !ready {
		log.Info("Virtual node not yet ready; waiting", "nodeName", nodeName)
		conditions.MarkFalse(
			interlinkMachine,
			infrav1.VirtualNodeReadyCondition,
			infrav1.VirtualNodeNotReadyReason,
			clusterv1.ConditionSeverityInfo,
			"%s", "waiting for virtual node to become Ready",
		)
		return ctrl.Result{RequeueAfter: requeueInterval}, nil
	}

	// Set provider ID and mark as ready.
	providerID := providerIDPrefix + nodeName
	interlinkMachine.Spec.ProviderID = &providerID
	interlinkMachine.Status.Addresses = addresses
	interlinkMachine.Status.Ready = true
	conditions.MarkTrue(interlinkMachine, infrav1.VirtualNodeReadyCondition)

	log.Info("InterlinkMachine reconciled successfully", "nodeName", nodeName, "ready", true)
	return ctrl.Result{}, nil
}

func (r *InterlinkMachineReconciler) reconcileDelete(
	ctx context.Context,
	log logr.Logger,
	interlinkMachine *infrav1.InterlinkMachine,
) (ctrl.Result, error) {
	log.Info("Reconciling InterlinkMachine deletion")

	nodeName := interlinkMachine.Spec.NodeName
	if nodeName == "" {
		nodeName = interlinkMachine.Name
	}

	// Delete the VirtualNode resource if it exists.
	vn := &virtualnode.VirtualNode{}
	err := r.Get(ctx, types.NamespacedName{
		Namespace: interlinkMachine.Namespace,
		Name:      nodeName,
	}, vn)
	if err != nil && !apierrors.IsNotFound(err) {
		return ctrl.Result{}, fmt.Errorf("failed to get VirtualNode %s: %w", nodeName, err)
	}

	if err == nil {
		// VirtualNode still exists; delete it.
		if err := r.Delete(ctx, vn); err != nil && !apierrors.IsNotFound(err) {
			return ctrl.Result{}, fmt.Errorf("failed to delete VirtualNode %s: %w", nodeName, err)
		}
		log.Info("Deleted VirtualNode", "name", nodeName)
		conditions.MarkFalse(
			interlinkMachine,
			infrav1.VirtualNodeReadyCondition,
			infrav1.VirtualNodeDeletingReason,
			clusterv1.ConditionSeverityInfo,
			"%s", "VirtualNode is being deleted",
		)
	}

	controllerutil.RemoveFinalizer(interlinkMachine, interlinkMachineFinalizer)
	return ctrl.Result{}, nil
}

// reconcileVirtualNode creates or updates the VirtualNode resource for this machine.
func (r *InterlinkMachineReconciler) reconcileVirtualNode(
	ctx context.Context,
	log logr.Logger,
	interlinkMachine *infrav1.InterlinkMachine,
	nodeName string,
) error {
	vn := &virtualnode.VirtualNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nodeName,
			Namespace: interlinkMachine.Namespace,
		},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, vn, func() error {
		vn.Spec = buildVirtualNodeSpec(interlinkMachine, nodeName)
		// Set the InterlinkMachine as owner so the VirtualNode is garbage-collected.
		return controllerutil.SetControllerReference(interlinkMachine, vn, r.Scheme)
	})
	if err != nil {
		return fmt.Errorf("failed to create/update VirtualNode %s: %w", nodeName, err)
	}

	log.Info("Reconciled VirtualNode", "name", nodeName, "result", result)
	return nil
}

// buildVirtualNodeSpec converts the InterlinkMachine spec into a VirtualNode spec.
func buildVirtualNodeSpec(m *infrav1.InterlinkMachine, nodeName string) virtualnode.VirtualNodeSpec {
	spec := virtualnode.VirtualNodeSpec{
		NodeName:         nodeName,
		InterLinkAddress: m.Spec.InterLinkAddress,
		Labels:           m.Spec.Labels,
		Taints:           m.Spec.Taints,
	}

	if res := m.Spec.Resources; res != (infrav1.VirtualNodeResources{}) {
		if res.CPU != nil {
			q := res.CPU.DeepCopy()
			spec.CPU = &q
		}
		if res.Memory != nil {
			q := res.Memory.DeepCopy()
			spec.Memory = &q
		}
		if res.EphemeralStorage != nil {
			q := res.EphemeralStorage.DeepCopy()
			spec.EphemeralStorage = &q
		}
		if res.Pods != nil {
			q := res.Pods.DeepCopy()
			spec.Pods = &q
		}
	}

	return spec
}

// isNodeReady checks if the Kubernetes Node named nodeName exists and is in Ready condition.
func (r *InterlinkMachineReconciler) isNodeReady(ctx context.Context, nodeName string) (bool, []clusterv1.MachineAddress, error) {
	node := &corev1.Node{}
	if err := r.Get(ctx, types.NamespacedName{Name: nodeName}, node); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil, nil
		}
		return false, nil, err
	}

	for _, cond := range node.Status.Conditions {
		if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
			addrs := nodeAddresses(node)
			return true, addrs, nil
		}
	}
	return false, nil, nil
}

// nodeAddresses converts corev1.NodeAddress entries into CAPI MachineAddresses.
func nodeAddresses(node *corev1.Node) []clusterv1.MachineAddress {
	addrs := make([]clusterv1.MachineAddress, 0, len(node.Status.Addresses))
	for _, a := range node.Status.Addresses {
		addrs = append(addrs, clusterv1.MachineAddress{
			Type:    clusterv1.MachineAddressType(a.Type),
			Address: a.Address,
		})
	}
	return addrs
}

func patchInterlinkMachine(ctx context.Context, patchHelper *patch.Helper, interlinkMachine *infrav1.InterlinkMachine) error {
	conditions.SetSummary(interlinkMachine,
		conditions.WithConditions(
			infrav1.VirtualNodeReadyCondition,
		),
	)

	return patchHelper.Patch(
		ctx,
		interlinkMachine,
		patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyCondition,
			infrav1.VirtualNodeReadyCondition,
		}},
		patch.WithStatusObservedGeneration{},
	)
}
