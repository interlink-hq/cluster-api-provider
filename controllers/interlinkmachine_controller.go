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
	"k8s.io/apimachinery/pkg/util/intstr"
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
// When PluginSpec is set on the InterlinkMachine, the reconciler also creates a
// Pod (running the interLink plugin binary) and a ClusterIP Service on an existing
// virtual node. The Service address is used as the interLink endpoint ("pilot" mode).
//
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=interlinkmachines,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=interlinkmachines/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=interlinkmachines/finalizers,verbs=update
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=machines;machines/status,verbs=get;list;watch
// +kubebuilder:rbac:groups=virtualkubelet.io,resources=virtualnodes,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=nodes,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services,verbs=get;list;watch;create;update;patch;delete
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

	// Determine the interLink address, potentially from a plugin Pod/Service.
	interLinkAddr := interlinkMachine.Spec.InterLinkAddress

	if interlinkMachine.Spec.PluginSpec != nil {
		addr, err := r.reconcilePluginPodAndService(ctx, log, interlinkMachine)
		if err != nil {
			conditions.MarkFalse(
				interlinkMachine,
				infrav1.PluginPodReadyCondition,
				infrav1.PluginPodProvisioningReason,
				clusterv1.ConditionSeverityWarning,
				"%s", err.Error(),
			)
			return ctrl.Result{}, err
		}
		if addr == "" {
			// Plugin Pod is not yet Running; requeue and wait.
			return ctrl.Result{RequeueAfter: requeueInterval}, nil
		}
		// Use the derived address only when no explicit address was provided.
		if interLinkAddr == "" {
			interLinkAddr = addr
		}
	}

	if interLinkAddr == "" {
		return ctrl.Result{}, fmt.Errorf("interLinkAddress must be set when pluginSpec is not configured")
	}

	// Create or update the VirtualNode resource.
	if err := r.reconcileVirtualNode(ctx, log, interlinkMachine, nodeName, interLinkAddr); err != nil {
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
	interLinkAddr string,
) error {
	vn := &virtualnode.VirtualNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      nodeName,
			Namespace: interlinkMachine.Namespace,
		},
	}

	result, err := controllerutil.CreateOrUpdate(ctx, r.Client, vn, func() error {
		vn.Spec = buildVirtualNodeSpec(interlinkMachine, nodeName, interLinkAddr)
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
func buildVirtualNodeSpec(m *infrav1.InterlinkMachine, nodeName string, interLinkAddr string) virtualnode.VirtualNodeSpec {
	spec := virtualnode.VirtualNodeSpec{
		NodeName:         nodeName,
		InterLinkAddress: interLinkAddr,
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

// reconcilePluginPodAndService ensures the plugin Pod and its ClusterIP Service exist
// for the given InterlinkMachine (pilot mode). It returns the derived interLink address
// (http://<svc>.<ns>.svc.cluster.local:<port>) once the Pod is Running, or an empty
// string when the Pod has not yet reached Running phase.
func (r *InterlinkMachineReconciler) reconcilePluginPodAndService(
	ctx context.Context,
	log logr.Logger,
	m *infrav1.InterlinkMachine,
) (string, error) {
	pluginName := m.Name + "-plugin"

	port := m.Spec.PluginSpec.Port
	if port == 0 {
		port = defaultPluginPort
	}

	podLabels := map[string]string{
		pluginPodLabelKey: m.Name,
	}

	// --- Service ---
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pluginName,
			Namespace: m.Namespace,
		},
	}
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, svc, func() error {
		svc.Spec.Selector = podLabels
		svc.Spec.Type = corev1.ServiceTypeClusterIP
		svc.Spec.Ports = []corev1.ServicePort{
			{
				Name:       "plugin",
				Port:       port,
				TargetPort: intstr.FromInt(int(port)),
				Protocol:   corev1.ProtocolTCP,
			},
		}
		return controllerutil.SetControllerReference(m, svc, r.Scheme)
	})
	if err != nil {
		return "", fmt.Errorf("failed to reconcile plugin Service %s: %w", pluginName, err)
	}

	// --- Pod ---
	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pluginName,
			Namespace: m.Namespace,
		},
	}
	_, err = controllerutil.CreateOrUpdate(ctx, r.Client, pod, func() error {
		// Pod spec is largely immutable after creation; only set it on first create.
		if pod.CreationTimestamp.IsZero() {
			pod.Labels = podLabels
			pod.Spec = buildPluginPodSpec(m.Spec.PluginSpec, port)
		}
		return controllerutil.SetControllerReference(m, pod, r.Scheme)
	})
	if err != nil {
		return "", fmt.Errorf("failed to reconcile plugin Pod %s: %w", pluginName, err)
	}

	// Re-fetch to get current status.
	if err := r.Get(ctx, types.NamespacedName{Name: pluginName, Namespace: m.Namespace}, pod); err != nil {
		return "", fmt.Errorf("failed to get plugin Pod %s: %w", pluginName, err)
	}

	if pod.Status.Phase != corev1.PodRunning {
		log.Info("Plugin pod not yet running; waiting", "pod", pluginName, "phase", pod.Status.Phase)
		conditions.MarkFalse(
			m,
			infrav1.PluginPodReadyCondition,
			infrav1.PluginPodNotReadyReason,
			clusterv1.ConditionSeverityInfo,
			"plugin pod %s is in phase %s", pluginName, pod.Status.Phase,
		)
		return "", nil
	}

	conditions.MarkTrue(m, infrav1.PluginPodReadyCondition)
	addr := fmt.Sprintf("http://%s.%s.svc.cluster.local:%d", pluginName, m.Namespace, port)
	log.Info("Plugin pod is running", "pod", pluginName, "interLinkAddress", addr)
	return addr, nil
}

// buildPluginPodSpec constructs the PodSpec for the interLink plugin container.
func buildPluginPodSpec(spec *infrav1.PluginPodSpec, port int32) corev1.PodSpec {
	return corev1.PodSpec{
		NodeSelector:  spec.NodeSelector,
		Tolerations:   spec.Tolerations,
		RestartPolicy: corev1.RestartPolicyAlways,
		Containers: []corev1.Container{
			{
				Name:  "plugin",
				Image: spec.Image,
				Ports: []corev1.ContainerPort{
					{
						Name:          "plugin",
						ContainerPort: port,
						Protocol:      corev1.ProtocolTCP,
					},
				},
				Env:       spec.Env,
				Resources: spec.Resources,
			},
		},
	}
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
	summaryConditions := []clusterv1.ConditionType{infrav1.VirtualNodeReadyCondition}
	ownedConditions := []clusterv1.ConditionType{
		clusterv1.ReadyCondition,
		infrav1.VirtualNodeReadyCondition,
	}
	if interlinkMachine.Spec.PluginSpec != nil {
		summaryConditions = append(summaryConditions, infrav1.PluginPodReadyCondition)
		ownedConditions = append(ownedConditions, infrav1.PluginPodReadyCondition)
	}

	conditions.SetSummary(interlinkMachine,
		conditions.WithConditions(summaryConditions...),
	)

	return patchHelper.Patch(
		ctx,
		interlinkMachine,
		patch.WithOwnedConditions{Conditions: ownedConditions},
		patch.WithStatusObservedGeneration{},
	)
}
