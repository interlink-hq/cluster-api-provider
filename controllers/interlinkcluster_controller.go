// Package controllers contains reconcilers for interlink Cluster API resources.
package controllers

import (
	"context"

	"github.com/go-logr/logr"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	infrav1 "github.com/interlink-hq/cluster-api-provider/api/v1alpha1"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"
	"sigs.k8s.io/cluster-api/util"
	"sigs.k8s.io/cluster-api/util/annotations"
	"sigs.k8s.io/cluster-api/util/conditions"
	"sigs.k8s.io/cluster-api/util/patch"
	"sigs.k8s.io/cluster-api/util/predicates"
)

const (
	interlinkClusterFinalizer = "interlinkcluster.infrastructure.cluster.x-k8s.io"
)

// InterlinkClusterReconciler reconciles InterlinkCluster objects.
//
// The InterlinkCluster is a lightweight infrastructure cluster object. It does not
// provision any actual infrastructure; its main responsibility is to:
//  1. Report the control-plane endpoint (taken from the owning CAPI Cluster or supplied explicitly).
//  2. Set status.ready = true so that CAPI proceeds with bootstrapping worker machines.
//
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=interlinkclusters,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=interlinkclusters/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=infrastructure.cluster.x-k8s.io,resources=interlinkclusters/finalizers,verbs=update
// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters;clusters/status,verbs=get;list;watch
type InterlinkClusterReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

// SetupWithManager sets up the InterlinkClusterReconciler with the controller manager.
func (r *InterlinkClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&infrav1.InterlinkCluster{}).
		WithEventFilter(predicates.ResourceNotPaused(r.Log)).
		Complete(r)
}

// Reconcile reconciles the InterlinkCluster.
func (r *InterlinkClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (_ ctrl.Result, reterr error) {
	log := r.Log.WithValues("interlinkcluster", req.NamespacedName)

	// Fetch the InterlinkCluster object.
	interlinkCluster := &infrav1.InterlinkCluster{}
	if err := r.Get(ctx, req.NamespacedName, interlinkCluster); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Fetch the owning Cluster.
	cluster, err := util.GetOwnerCluster(ctx, r.Client, interlinkCluster.ObjectMeta)
	if err != nil {
		return ctrl.Result{}, err
	}
	if cluster == nil {
		log.Info("Cluster Controller has not yet set OwnerRef; re-queuing")
		return ctrl.Result{}, nil
	}

	// Return early if paused.
	if annotations.IsPaused(cluster, interlinkCluster) {
		log.Info("InterlinkCluster or owning Cluster is paused; skipping reconciliation")
		return ctrl.Result{}, nil
	}

	// Initialize a patch helper.
	patchHelper, err := patch.NewHelper(interlinkCluster, r.Client)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Always patch status at the end of the reconcile loop.
	defer func() {
		if err := patchInterlinkCluster(ctx, patchHelper, interlinkCluster); err != nil {
			if isNotFound(err) {
				return
			}
			log.Error(err, "failed to patch InterlinkCluster")
			if reterr == nil {
				reterr = err
			}
		}
	}()

	// Handle deletion.
	if !interlinkCluster.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, log, interlinkCluster)
	}

	return r.reconcileNormal(ctx, log, cluster, interlinkCluster)
}

func (r *InterlinkClusterReconciler) reconcileNormal(
	ctx context.Context,
	log logr.Logger,
	cluster *clusterv1.Cluster,
	interlinkCluster *infrav1.InterlinkCluster,
) (ctrl.Result, error) {
	// Add finalizer.
	if !controllerutil.ContainsFinalizer(interlinkCluster, interlinkClusterFinalizer) {
		controllerutil.AddFinalizer(interlinkCluster, interlinkClusterFinalizer)
	}

	// If no explicit control-plane endpoint was set, inherit it from the owning Cluster.
	if interlinkCluster.Spec.ControlPlaneEndpoint.Host == "" && cluster.Spec.ControlPlaneEndpoint.Host != "" {
		interlinkCluster.Spec.ControlPlaneEndpoint = infrav1.InterlinkClusterSpec{
			ControlPlaneEndpoint: cluster.Spec.ControlPlaneEndpoint,
		}.ControlPlaneEndpoint
		log.Info("Inherited control-plane endpoint from Cluster", "host", interlinkCluster.Spec.ControlPlaneEndpoint.Host)
	}

	if interlinkCluster.Spec.ControlPlaneEndpoint.Host == "" {
		log.Info("ControlPlaneEndpoint not yet available; waiting")
		return ctrl.Result{}, nil
	}

	// Mark as Ready.
	interlinkCluster.Status.Ready = true
	conditions.MarkTrue(interlinkCluster, clusterv1.ReadyCondition)

	log.Info("InterlinkCluster reconciled successfully", "ready", true)
	return ctrl.Result{}, nil
}

func (r *InterlinkClusterReconciler) reconcileDelete(
	ctx context.Context,
	log logr.Logger,
	interlinkCluster *infrav1.InterlinkCluster,
) (ctrl.Result, error) {
	log.Info("Reconciling InterlinkCluster deletion")

	controllerutil.RemoveFinalizer(interlinkCluster, interlinkClusterFinalizer)
	return ctrl.Result{}, nil
}

func patchInterlinkCluster(ctx context.Context, patchHelper *patch.Helper, interlinkCluster *infrav1.InterlinkCluster) error {
	conditions.SetSummary(interlinkCluster,
		conditions.WithConditions(
			infrav1.VirtualNodeReadyCondition,
		),
	)

	return patchHelper.Patch(
		ctx,
		interlinkCluster,
		patch.WithOwnedConditions{Conditions: []clusterv1.ConditionType{
			clusterv1.ReadyCondition,
			infrav1.VirtualNodeReadyCondition,
		}},
		patch.WithStatusObservedGeneration{},
	)
}
