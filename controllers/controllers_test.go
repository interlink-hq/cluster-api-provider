package controllers_test

import (
	"context"
	"testing"

	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	clusterv1 "sigs.k8s.io/cluster-api/api/v1beta1"

	infrav1 "github.com/interlink-hq/cluster-api-provider/api/v1alpha1"
	"github.com/interlink-hq/cluster-api-provider/controllers"
	"github.com/interlink-hq/cluster-api-provider/internal/virtualnode"
)

func buildScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	s := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(s)
	_ = corev1.AddToScheme(s)
	_ = clusterv1.AddToScheme(s)
	_ = infrav1.AddToScheme(s)
	_ = virtualnode.AddToScheme(s)
	return s
}

// -------------------------------------------------------------------
// InterlinkCluster tests
// -------------------------------------------------------------------

func TestInterlinkClusterReconciler_MarksReady(t *testing.T) {
	g := NewWithT(t)
	scheme := buildScheme(t)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		Spec: clusterv1.ClusterSpec{
			ControlPlaneEndpoint: clusterv1.APIEndpoint{
				Host: "10.0.0.1",
				Port: 6443,
			},
		},
	}

	interlinkCluster := &infrav1.InterlinkCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: clusterv1.GroupVersion.String(),
					Kind:       "Cluster",
					Name:       cluster.Name,
					UID:        cluster.UID,
					Controller: boolPtr(true),
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster, interlinkCluster).
		WithStatusSubresource(interlinkCluster).
		Build()

	reconciler := &controllers.InterlinkClusterReconciler{
		Client: fakeClient,
		Scheme: scheme,
		Log:    ctrl.Log.WithName("test"),
	}

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: "default",
			Name:      "test-cluster",
		},
	})
	g.Expect(err).ToNot(HaveOccurred())

	got := &infrav1.InterlinkCluster{}
	g.Expect(fakeClient.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "test-cluster"}, got)).To(Succeed())
	g.Expect(got.Status.Ready).To(BeTrue())
	g.Expect(got.Spec.ControlPlaneEndpoint.Host).To(Equal("10.0.0.1"))
}

func TestInterlinkClusterReconciler_WaitsForEndpoint(t *testing.T) {
	g := NewWithT(t)
	scheme := buildScheme(t)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
		// No control-plane endpoint set.
	}

	interlinkCluster := &infrav1.InterlinkCluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: clusterv1.GroupVersion.String(),
					Kind:       "Cluster",
					Name:       cluster.Name,
					UID:        cluster.UID,
					Controller: boolPtr(true),
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster, interlinkCluster).
		WithStatusSubresource(interlinkCluster).
		Build()

	reconciler := &controllers.InterlinkClusterReconciler{
		Client: fakeClient,
		Scheme: scheme,
		Log:    ctrl.Log.WithName("test"),
	}

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "test-cluster"},
	})
	g.Expect(err).ToNot(HaveOccurred())

	got := &infrav1.InterlinkCluster{}
	g.Expect(fakeClient.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "test-cluster"}, got)).To(Succeed())
	// Should NOT be ready yet because no endpoint.
	g.Expect(got.Status.Ready).To(BeFalse())
}

// -------------------------------------------------------------------
// InterlinkMachine tests
// -------------------------------------------------------------------

func TestInterlinkMachineReconciler_CreatesVirtualNode(t *testing.T) {
	g := NewWithT(t)
	scheme := buildScheme(t)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "default",
		},
	}

	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-machine",
			Namespace: "default",
			Labels: map[string]string{
				clusterv1.ClusterNameLabel: "test-cluster",
			},
		},
		Spec: clusterv1.MachineSpec{
			ClusterName: "test-cluster",
		},
	}

	interlinkMachine := &infrav1.InterlinkMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-machine",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion: clusterv1.GroupVersion.String(),
					Kind:       "Machine",
					Name:       machine.Name,
					UID:        machine.UID,
					Controller: boolPtr(true),
				},
			},
		},
		Spec: infrav1.InterlinkMachineSpec{
			NodeName:         "virtual-node-1",
			InterLinkAddress: "http://interlink:3000",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster, machine, interlinkMachine).
		WithStatusSubresource(interlinkMachine).
		Build()

	reconciler := &controllers.InterlinkMachineReconciler{
		Client: fakeClient,
		Scheme: scheme,
		Log:    ctrl.Log.WithName("test"),
	}

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "test-machine"},
	})
	g.Expect(err).ToNot(HaveOccurred())

	// VirtualNode should have been created.
	vn := &virtualnode.VirtualNode{}
	g.Expect(fakeClient.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "virtual-node-1"}, vn)).To(Succeed())
	g.Expect(vn.Spec.NodeName).To(Equal("virtual-node-1"))
	g.Expect(vn.Spec.InterLinkAddress).To(Equal("http://interlink:3000"))
}

func TestInterlinkMachineReconciler_MarksReadyWhenNodeReady(t *testing.T) {
	g := NewWithT(t)
	scheme := buildScheme(t)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "default"},
	}
	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-machine",
			Namespace: "default",
			Labels:    map[string]string{clusterv1.ClusterNameLabel: "test-cluster"},
		},
		Spec: clusterv1.MachineSpec{ClusterName: "test-cluster"},
	}
	interlinkMachine := &infrav1.InterlinkMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-machine",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{APIVersion: clusterv1.GroupVersion.String(), Kind: "Machine", Name: machine.Name, UID: machine.UID, Controller: boolPtr(true)},
			},
		},
		Spec: infrav1.InterlinkMachineSpec{
			NodeName:         "virtual-node-1",
			InterLinkAddress: "http://interlink:3000",
		},
	}

	// Pre-create a Ready node.
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "virtual-node-1"},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
			Addresses: []corev1.NodeAddress{
				{Type: corev1.NodeInternalIP, Address: "192.168.1.1"},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster, machine, interlinkMachine, node).
		WithStatusSubresource(interlinkMachine).
		Build()

	reconciler := &controllers.InterlinkMachineReconciler{
		Client: fakeClient,
		Scheme: scheme,
		Log:    ctrl.Log.WithName("test"),
	}

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "test-machine"},
	})
	g.Expect(err).ToNot(HaveOccurred())

	got := &infrav1.InterlinkMachine{}
	g.Expect(fakeClient.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "test-machine"}, got)).To(Succeed())
	g.Expect(got.Status.Ready).To(BeTrue())
	g.Expect(got.Spec.ProviderID).ToNot(BeNil())
	g.Expect(*got.Spec.ProviderID).To(Equal("interlink://virtual-node-1"))
	g.Expect(got.Status.Addresses).To(HaveLen(1))
}

func TestInterlinkMachineReconciler_DeletesVirtualNode(t *testing.T) {
	g := NewWithT(t)
	scheme := buildScheme(t)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "default"},
	}
	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-machine",
			Namespace: "default",
			Labels:    map[string]string{clusterv1.ClusterNameLabel: "test-cluster"},
		},
		Spec: clusterv1.MachineSpec{ClusterName: "test-cluster"},
	}

	now := metav1.Now()
	interlinkMachine := &infrav1.InterlinkMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-machine",
			Namespace: "default",
			Finalizers: []string{"interlinkmachine.infrastructure.cluster.x-k8s.io"},
			DeletionTimestamp: &now,
			OwnerReferences: []metav1.OwnerReference{
				{APIVersion: clusterv1.GroupVersion.String(), Kind: "Machine", Name: machine.Name, UID: machine.UID, Controller: boolPtr(true)},
			},
		},
		Spec: infrav1.InterlinkMachineSpec{
			NodeName:         "virtual-node-1",
			InterLinkAddress: "http://interlink:3000",
		},
	}

	// Pre-create the VirtualNode to be deleted.
	vn := &virtualnode.VirtualNode{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "virtual-node-1",
			Namespace: "default",
		},
		Spec: virtualnode.VirtualNodeSpec{
			NodeName:         "virtual-node-1",
			InterLinkAddress: "http://interlink:3000",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster, machine, interlinkMachine, vn).
		WithStatusSubresource(interlinkMachine).
		Build()

	reconciler := &controllers.InterlinkMachineReconciler{
		Client: fakeClient,
		Scheme: scheme,
		Log:    ctrl.Log.WithName("test"),
	}

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "test-machine"},
	})
	g.Expect(err).ToNot(HaveOccurred())

	// VirtualNode should be gone.
	deletedVN := &virtualnode.VirtualNode{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "virtual-node-1"}, deletedVN)
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue())

	// After the last finalizer is removed from an object with a DeletionTimestamp,
	// the API server deletes the object.  The InterlinkMachine should therefore be gone.
	gone := &infrav1.InterlinkMachine{}
	err = fakeClient.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "test-machine"}, gone)
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue())
}

func boolPtr(b bool) *bool { return &b }
