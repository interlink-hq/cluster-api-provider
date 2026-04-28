# cluster-api-provider

> [!WARNING]
> **Work In Progress — not functioning or tested in production yet.**
> This repository is an early draft. APIs, configuration, and behaviour are
> subject to breaking changes without notice.  Do **not** use in production.

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
- **On-demand plugin Pods** – the controller spawns the interLink plugin binary
  as a Pod on an existing virtual node, removing the need for a pre-deployed
  interLink instance.

## Architecture

The controller provisions the interLink plugin on-demand as a regular Pod placed
on an existing virtual node.  No separate interLink deployment is required:

```
Cluster API core
    │  watches/manages
    ▼
InterlinkMachine
    ├─ creates  ──►  Pod  "<machine>-plugin"  (runs interLink plugin binary)
    │                 │  scheduled on an existing virtual node
    ├─ creates  ──►  Service  "<machine>-plugin"  (ClusterIP)
    │                 │  exposes Pod on the plugin port
    └─ derives interLinkAddress from the Service, then creates VirtualNode
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

The controller creates a Pod and ClusterIP Service named `<machine-name>-plugin`,
then derives the address as `http://<machine>-plugin.<namespace>.svc.cluster.local:<port>`.

```yaml
apiVersion: infrastructure.cluster.x-k8s.io/v1alpha1
kind: InterlinkMachine
metadata:
  name: my-virtual-node
spec:
  # Optional: override the derived interLinkAddress
  # interLinkAddress: "http://custom-endpoint:3000"

  nodeName: "my-virtual-node"
  resources:
    cpu: "8"
    memory: "32Gi"
    pods: "110"
  labels:
    node.kubernetes.io/type: virtual
    interlink.eu/provider: hpc-apptainer
  taints:
  - key: virtual-node.interlink.eu
    effect: NoSchedule

  pluginSpec:
    # Container image running the interLink plugin binary.
    image: "ghcr.io/interlink-hq/interlink/plugin-apptainer:latest"

    # TCP port the plugin listens on (default: 4000).
    port: 4000

    # Schedule the plugin Pod on an existing virtual node that has access to
    # the remote resource provider.
    nodeSelector:
      interlink.eu/provider: hpc-cluster

    # Tolerate the taint that virtual nodes typically carry.
    tolerations:
    - key: virtual-node.interlink.eu
      operator: Exists
      effect: NoSchedule

    # Plugin-specific environment variables.
    env:
    - name: INTERLINK_PORT
      value: "4000"

    # Compute resources for the plugin container.
    resources:
      requests:
        cpu: "500m"
        memory: "256Mi"
```

### InterlinkMachine status conditions

| Condition | Meaning |
|-----------|---------|
| `VirtualNodeReady` | The interlink `VirtualNode` resource exists and the corresponding Kubernetes Node is Ready. |
| `PluginPodReady` | The plugin Pod is in the `Running` phase and the Service is available. |

## Quick start

### Prerequisites

- Kubernetes cluster with [Cluster API](https://cluster-api.sigs.k8s.io/user/quick-start) installed
- [interlink](https://github.com/interTwin-eu/interLink) `VirtualNode` CRD present in the cluster
- An existing interLink virtual node to host the plugin Pods

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

### Create a cluster

```bash
# Apply the cluster, MachineDeployment, and InterlinkMachineTemplate
kubectl apply -f examples/pilot.yaml

# Watch machines come up — the controller creates a plugin Pod per machine
kubectl get interlinkmachines -w
kubectl get pods -l interlinkmachine.infrastructure.cluster.x-k8s.io/machine
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
`InterlinkMachineTemplate` up and down.  Each new Machine automatically spins
up a dedicated plugin Pod on an existing virtual node, making interLink
instances fully on-demand.

## License

Apache 2.0
