# cluster-api-provider

A [Cluster API](https://cluster-api.sigs.k8s.io) infrastructure provider for
[interlink](https://github.com/interTwin-eu/interLink) virtual nodes.

## Overview

[interlink](https://github.com/interTwin-eu/interLink) creates virtual Kubernetes
nodes that transparently offload pod execution to remote or HPC resources.  This
provider makes those virtual nodes first-class Cluster API citizens, enabling:

- **Autoscaling** – the
  [Cluster Autoscaler](https://github.com/kubernetes/autoscaler/tree/master/cluster-autoscaler)
  can scale the pool of virtual nodes in/out by creating/deleting `InterlinkMachine`
  objects, just as it would for any other CAPI infrastructure provider.
- **Lifecycle management** – virtual nodes are created and deleted through the
  normal CAPI Machine lifecycle, including finalizers and status propagation.
- **MachineDeployments** – rolling updates, replica management, and topology
  support all work out of the box.

## Architecture

```
Cluster API core
    │  watches/manages
    ▼
InterlinkCluster (infrastructure.cluster.x-k8s.io/v1alpha1)
    └─ marks cluster ready, inherits controlPlaneEndpoint

InterlinkMachine  (infrastructure.cluster.x-k8s.io/v1alpha1)
    └─ creates / deletes  ──►  VirtualNode (virtualkubelet.io/v1alpha1)
                                    │
                                    ▼
                          interlink virtual-kubelet pod
                                    │
                                    ▼
                          Kubernetes Node (virtual, appears as real)
```

## Custom Resource Definitions

| Kind | Purpose |
|------|---------|
| `InterlinkCluster` | Infrastructure side of a CAPI Cluster. Marks `status.ready=true` and exposes the control-plane endpoint. |
| `InterlinkClusterTemplate` | Template for `InterlinkCluster` used by `ClusterClass`. |
| `InterlinkMachine` | Infrastructure side of a CAPI Machine. Creates/deletes an interlink `VirtualNode` and reflects its readiness. |
| `InterlinkMachineTemplate` | Template for `InterlinkMachine` used by `MachineDeployment`. |

### InterlinkMachine spec

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: InterlinkMachine
metadata:
  name: my-virtual-node
spec:
  # Required: URL of the interLink API server
  interLinkAddress: "http://interlink-api.interlink-system.svc:3000"

  # Optional: name of the Kubernetes node (defaults to metadata.name)
  nodeName: "my-virtual-node"

  # Optional: capacity to advertise on the virtual node
  resources:
    cpu: "10"
    memory: "64Gi"
    pods: "110"

  # Optional: extra labels applied to the virtual node
  labels:
    node.kubernetes.io/type: virtual

  # Optional: taints applied to the virtual node
  taints:
  - key: virtual-node.interlink.eu
    effect: NoSchedule
```

## Quick start

### Prerequisites

- Kubernetes cluster with [Cluster API](https://cluster-api.sigs.k8s.io/user/quick-start) installed
- [interlink](https://github.com/interTwin-eu/interLink) running and its
  `VirtualNode` CRD present in the cluster

### Install the provider

```bash
# Apply CRDs
kubectl apply -f config/crd/bases/

# Apply RBAC and manager
kubectl apply -f config/default/
```

Or build from source:

```bash
docker build -t ghcr.io/interlink-hq/cluster-api-provider:dev .
docker push ghcr.io/interlink-hq/cluster-api-provider:dev
# update config/manager/manager.yaml image reference, then:
kubectl apply -f config/default/
```

### Create a cluster with virtual nodes

```bash
# 1. Apply the templates
kubectl apply -f examples/templates.yaml

# 2. Apply the cluster and MachineDeployment
kubectl apply -f examples/cluster.yaml

# 3. Watch machines come up
kubectl get interlinkmachines -w
```

## Development

```bash
# Run tests
go test ./...

# Build
go build ./...

# Run controller locally against the current kubeconfig
go run main.go --leader-elect=false
```

## Autoscaling with Cluster Autoscaler

The provider is compatible with the Cluster Autoscaler's CAPI integration.
Configure the autoscaler with:

```yaml
- --cloud-provider=clusterapi
- --node-group-auto-discovery=clusterapi:namespace=default,clusterName=my-interlink-cluster
```

The autoscaler will scale `MachineDeployments` that target
`InterlinkMachineTemplate` up and down, causing the provider to create or
delete `VirtualNode` resources accordingly.

## License

Apache 2.0
