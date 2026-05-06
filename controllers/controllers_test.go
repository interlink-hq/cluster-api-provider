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

func TestInterlinkMachineReconciler_PluginPodMode_WaitsForPod(t *testing.T) {
	g := NewWithT(t)
	scheme := buildScheme(t)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "default"},
	}
	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pilot-machine",
			Namespace: "default",
			Labels:    map[string]string{clusterv1.ClusterNameLabel: "test-cluster"},
		},
		Spec: clusterv1.MachineSpec{ClusterName: "test-cluster"},
	}
	interlinkMachine := &infrav1.InterlinkMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pilot-machine",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{APIVersion: clusterv1.GroupVersion.String(), Kind: "Machine", Name: machine.Name, UID: machine.UID, Controller: boolPtr(true)},
			},
		},
		Spec: infrav1.InterlinkMachineSpec{
			NodeName: "virtual-node-pilot",
			PluginSpec: &infrav1.PluginPodSpec{
				Image: "ghcr.io/interlink-hq/interlink/plugin-apptainer:latest",
				Port:  4000,
				NodeSelector: map[string]string{
					"interlink.eu/provider": "hpc-cluster",
				},
			},
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

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "pilot-machine"},
	})
	g.Expect(err).ToNot(HaveOccurred())
	// Plugin pod is not Running yet; should requeue.
	g.Expect(result.RequeueAfter).ToNot(BeZero())

	// The plugin Pod should have been created.
	pod := &corev1.Pod{}
	g.Expect(fakeClient.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "pilot-machine-plugin"}, pod)).To(Succeed())
	g.Expect(pod.Spec.Containers).To(HaveLen(1))
	g.Expect(pod.Spec.Containers[0].Image).To(Equal("ghcr.io/interlink-hq/interlink/plugin-apptainer:latest"))
	g.Expect(pod.Spec.Containers[0].Ports[0].ContainerPort).To(Equal(int32(4000)))
	g.Expect(pod.Spec.NodeSelector).To(HaveKeyWithValue("interlink.eu/provider", "hpc-cluster"))

	// The plugin Service should have been created.
	svc := &corev1.Service{}
	g.Expect(fakeClient.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "pilot-machine-plugin"}, svc)).To(Succeed())
	g.Expect(svc.Spec.Ports).To(HaveLen(1))
	g.Expect(svc.Spec.Ports[0].Port).To(Equal(int32(4000)))
	g.Expect(svc.Spec.Selector).To(HaveKeyWithValue("interlinkmachine.infrastructure.cluster.x-k8s.io/machine", "pilot-machine"))

	// The InterlinkMachine should NOT be ready yet.
	got := &infrav1.InterlinkMachine{}
	g.Expect(fakeClient.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "pilot-machine"}, got)).To(Succeed())
	g.Expect(got.Status.Ready).To(BeFalse())
}

func TestInterlinkMachineReconciler_PluginPodMode_MarksReadyWhenPodRunning(t *testing.T) {
	g := NewWithT(t)
	scheme := buildScheme(t)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "default"},
	}
	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pilot-machine",
			Namespace: "default",
			Labels:    map[string]string{clusterv1.ClusterNameLabel: "test-cluster"},
		},
		Spec: clusterv1.MachineSpec{ClusterName: "test-cluster"},
	}
	interlinkMachine := &infrav1.InterlinkMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pilot-machine",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{APIVersion: clusterv1.GroupVersion.String(), Kind: "Machine", Name: machine.Name, UID: machine.UID, Controller: boolPtr(true)},
			},
		},
		Spec: infrav1.InterlinkMachineSpec{
			NodeName: "virtual-node-pilot",
			PluginSpec: &infrav1.PluginPodSpec{
				Image: "ghcr.io/interlink-hq/interlink/plugin-apptainer:latest",
				Port:  4000,
			},
		},
	}

	// Pre-create the plugin Pod in Running phase.
	pluginPod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pilot-machine-plugin",
			Namespace: "default",
			Labels: map[string]string{
				"interlinkmachine.infrastructure.cluster.x-k8s.io/machine": "pilot-machine",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{{Name: "plugin", Image: "ghcr.io/interlink-hq/interlink/plugin-apptainer:latest"}},
		},
		Status: corev1.PodStatus{Phase: corev1.PodRunning},
	}

	// Pre-create the virtual Kubernetes node in Ready state.
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{Name: "virtual-node-pilot"},
		Status: corev1.NodeStatus{
			Conditions: []corev1.NodeCondition{
				{Type: corev1.NodeReady, Status: corev1.ConditionTrue},
			},
			Addresses: []corev1.NodeAddress{
				{Type: corev1.NodeInternalIP, Address: "10.0.0.5"},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster, machine, interlinkMachine, pluginPod, node).
		WithStatusSubresource(interlinkMachine, pluginPod).
		Build()

	reconciler := &controllers.InterlinkMachineReconciler{
		Client: fakeClient,
		Scheme: scheme,
		Log:    ctrl.Log.WithName("test"),
	}

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "pilot-machine"},
	})
	g.Expect(err).ToNot(HaveOccurred())

	// VirtualNode should have been created with the derived interLinkAddress.
	vn := &virtualnode.VirtualNode{}
	g.Expect(fakeClient.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "virtual-node-pilot"}, vn)).To(Succeed())
	g.Expect(vn.Spec.InterLinkAddress).To(Equal("http://pilot-machine-plugin.default.svc.cluster.local:4000"))

	// InterlinkMachine should be ready.
	got := &infrav1.InterlinkMachine{}
	g.Expect(fakeClient.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "pilot-machine"}, got)).To(Succeed())
	g.Expect(got.Status.Ready).To(BeTrue())
	g.Expect(got.Spec.ProviderID).ToNot(BeNil())
	g.Expect(*got.Spec.ProviderID).To(Equal("interlink://virtual-node-pilot"))
}

func TestInterlinkMachineReconciler_PluginPodMode_DefaultPort(t *testing.T) {
	g := NewWithT(t)
	scheme := buildScheme(t)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "default"},
	}
	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pilot-default-port",
			Namespace: "default",
			Labels:    map[string]string{clusterv1.ClusterNameLabel: "test-cluster"},
		},
		Spec: clusterv1.MachineSpec{ClusterName: "test-cluster"},
	}
	// PluginSpec with no Port set → should default to 4000.
	interlinkMachine := &infrav1.InterlinkMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pilot-default-port",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{APIVersion: clusterv1.GroupVersion.String(), Kind: "Machine", Name: machine.Name, UID: machine.UID, Controller: boolPtr(true)},
			},
		},
		Spec: infrav1.InterlinkMachineSpec{
			PluginSpec: &infrav1.PluginPodSpec{
				Image: "ghcr.io/interlink-hq/interlink/plugin-apptainer:latest",
				// Port intentionally left as zero to exercise the default.
			},
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
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "pilot-default-port"},
	})
	g.Expect(err).ToNot(HaveOccurred())

	svc := &corev1.Service{}
	g.Expect(fakeClient.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "pilot-default-port-plugin"}, svc)).To(Succeed())
	g.Expect(svc.Spec.Ports[0].Port).To(Equal(int32(4000)))

	pod := &corev1.Pod{}
	g.Expect(fakeClient.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "pilot-default-port-plugin"}, pod)).To(Succeed())
	g.Expect(pod.Spec.Containers[0].Ports[0].ContainerPort).To(Equal(int32(4000)))
}

func boolPtr(b bool) *bool { return &b }

func TestInterlinkMachineReconciler_PluginPodMode_VolumesAndMounts(t *testing.T) {
	g := NewWithT(t)
	scheme := buildScheme(t)

	cluster := &clusterv1.Cluster{
		ObjectMeta: metav1.ObjectMeta{Name: "test-cluster", Namespace: "default"},
	}
	machine := &clusterv1.Machine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "apptainer-machine",
			Namespace: "default",
			Labels:    map[string]string{clusterv1.ClusterNameLabel: "test-cluster"},
		},
		Spec: clusterv1.MachineSpec{ClusterName: "test-cluster"},
	}
	interlinkMachine := &infrav1.InterlinkMachine{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "apptainer-machine",
			Namespace: "default",
			OwnerReferences: []metav1.OwnerReference{
				{APIVersion: clusterv1.GroupVersion.String(), Kind: "Machine", Name: machine.Name, UID: machine.UID, Controller: boolPtr(true)},
			},
		},
		Spec: infrav1.InterlinkMachineSpec{
			NodeName: "virtual-node-apptainer",
			PluginSpec: &infrav1.PluginPodSpec{
				Image: "ghcr.io/interlink-hq/interlink-apptainer-plugin:latest",
				Port:  4000,
				Env: []corev1.EnvVar{
					{Name: "APPTAINERCONFIGPATH", Value: "/etc/interlink/ApptainerConfig.yaml"},
				},
				Volumes: []corev1.Volume{
					{
						Name: "plugin-config",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{Name: "apptainer-plugin-config"},
							},
						},
					},
				},
				VolumeMounts: []corev1.VolumeMount{
					{Name: "plugin-config", MountPath: "/etc/interlink", ReadOnly: true},
				},
			},
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

	result, err := reconciler.Reconcile(context.Background(), ctrl.Request{
		NamespacedName: types.NamespacedName{Namespace: "default", Name: "apptainer-machine"},
	})
	g.Expect(err).ToNot(HaveOccurred())
	// Plugin pod is not Running yet; should requeue.
	g.Expect(result.RequeueAfter).ToNot(BeZero())

	// Verify the plugin Pod has the expected volumes and volumeMounts.
	pod := &corev1.Pod{}
	g.Expect(fakeClient.Get(context.Background(), types.NamespacedName{Namespace: "default", Name: "apptainer-machine-plugin"}, pod)).To(Succeed())
	g.Expect(pod.Spec.Containers[0].Image).To(Equal("ghcr.io/interlink-hq/interlink-apptainer-plugin:latest"))
	g.Expect(pod.Spec.Volumes).To(HaveLen(1))
	g.Expect(pod.Spec.Volumes[0].Name).To(Equal("plugin-config"))
	g.Expect(pod.Spec.Volumes[0].ConfigMap).ToNot(BeNil())
	g.Expect(pod.Spec.Volumes[0].ConfigMap.Name).To(Equal("apptainer-plugin-config"))
	g.Expect(pod.Spec.Containers[0].VolumeMounts).To(HaveLen(1))
	g.Expect(pod.Spec.Containers[0].VolumeMounts[0].Name).To(Equal("plugin-config"))
	g.Expect(pod.Spec.Containers[0].VolumeMounts[0].MountPath).To(Equal("/etc/interlink"))
	g.Expect(pod.Spec.Containers[0].VolumeMounts[0].ReadOnly).To(BeTrue())
	g.Expect(pod.Spec.Containers[0].Env).To(ContainElement(
		corev1.EnvVar{Name: "APPTAINERCONFIGPATH", Value: "/etc/interlink/ApptainerConfig.yaml"},
	))
}
