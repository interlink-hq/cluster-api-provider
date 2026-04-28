# Architecture

This document describes the architectural choices and design rationale for the
Cluster API infrastructure provider for [interLink](https://github.com/interTwin-eu/interLink)
virtual nodes.

## Overview

[interLink](https://github.com/interTwin-eu/interLink) creates virtual Kubernetes
nodes that transparently offload pod execution to remote or HPC resources.
This provider integrates those virtual nodes into the
[Cluster API (CAPI)](https://cluster-api.sigs.k8s.io) framework, so that the
standard CAPI lifecycle, autoscaling, and topology machinery applies equally to
virtual nodes backed by remote compute.

## Key Components

```
┌─────────────────────────────────────────────────────────────────┐
│  Cluster API core (Machine, Cluster, MachineDeployment, …)      │
│                          │ owns / watches                        │
└──────────────────────────┼──────────────────────────────────────┘
                           │
          ┌────────────────┴─────────────────┐
          │                                  │
  ┌───────▼──────────┐            ┌──────────▼────────────┐
  │ InterlinkCluster │            │  InterlinkMachine      │
  │ (infrastructure  │            │  (infrastructure side  │
  │  side of Cluster)│            │   of Machine)          │
  └───────┬──────────┘            └──────────┬─────────────┘
          │ sets ready=true                   │
          │ exposes control-plane             │ creates / deletes
          │ endpoint                          │
                                    ┌─────────┴─────────────────┐
                                    │                           │
                            ┌───────▼──────┐           ┌───────▼──────┐
                            │  VirtualNode │           │  Plugin Pod  │
                            │  (interlink  │           │  + Service   │
                            │   CRD)       │           │  (pilot mode)│
                            └───────┬──────┘           └──────────────┘
                                    │ virtual-kubelet registers
                                    ▼
                            Kubernetes Node
                            (virtual, appears real)
```

## Architectural Choices

### 1. Standard Cluster API Infrastructure Provider Contract

The provider follows the official CAPI infrastructure provider contract,
implementing two CRD pairs:

| Kind | Paired with |
|------|-------------|
| `InterlinkCluster` | CAPI `Cluster` |
| `InterlinkMachine` | CAPI `Machine` |

Each resource has a corresponding `*Template` variant
(`InterlinkClusterTemplate`, `InterlinkMachineTemplate`) used by
`ClusterClass` and `MachineDeployment` respectively.

**Rationale:** Strict conformance to the CAPI contract means that all CAPI
tooling — the Cluster Autoscaler CAPI integration, `clusterctl`, topology
controllers, and MachineDeployment rolling updates — works with this provider
without modification.

### 2. Thin Cluster Infrastructure Layer

`InterlinkCluster` is deliberately minimal: it immediately marks
`status.ready = true` and exposes the management cluster's own API server as
the control-plane endpoint.  There is no separate control-plane to provision.

**Rationale:** interLink virtual nodes join an *existing* management cluster
rather than forming a new one. The `InterlinkCluster` object satisfies the
CAPI contract (a `Cluster` object cannot become ready without an infrastructure
counterpart) while avoiding the overhead of a dedicated control plane.

### 3. On-Demand Plugin Pods — "Pilot" Mode

When `InterlinkMachine.spec.pluginSpec` is set, the reconciler creates:

1. A **Pod** (named `<machine>-plugin`) that runs the interLink plugin binary.
   This Pod is scheduled on an *existing* virtual node that already has access
   to the target remote resource provider (e.g. an HPC cluster).
2. A **ClusterIP Service** (named `<machine>-plugin`) that exposes the Pod's
   plugin port inside the cluster network.
3. The `interLinkAddress` for the `VirtualNode` is derived automatically from
   the Service DNS name:
   `http://<machine>-plugin.<namespace>.svc.cluster.local:<port>`.

Once the plugin Pod reaches `Running` phase, the reconciler creates (or
updates) the `VirtualNode` resource with the derived address.

**Rationale:**

- **No pre-deployed interLink instance required.** Instances are created and
  destroyed alongside their machines, so the number of running plugin processes
  exactly matches the number of active virtual nodes. This avoids idle resource
  consumption and simplifies cluster bootstrap.
- **Leverage existing virtual nodes as a substrate.** Plugin Pods are placed on
  already-registered virtual nodes via `nodeSelector` and `tolerations`,
  inheriting the connectivity those nodes have to remote systems (e.g. HPC
  login nodes). No extra network tunnels are needed.
- **Uniform lifecycle.** Deleting an `InterlinkMachine` cascades — via owner
  references — to the `VirtualNode`, the plugin Pod, and the Service, so
  there is a single control point for cleanup.

### 4. Static Address Mode

When `InterlinkMachine.spec.interLinkAddress` is set and `pluginSpec` is
omitted, the controller creates a `VirtualNode` pointing directly at the
provided address. This suits environments where an interLink API server is
already running independently (e.g. a long-lived sidecar or external service).

**Rationale:** Supports existing interLink deployments without requiring
migration to the pilot-Pod model. The two modes are mutually exclusive but
follow the same reconcile path after the address is resolved.

### 5. VirtualNode as the Source of Truth for Readiness

The `InterlinkMachineReconciler` watches `corev1.Node` objects in addition to
`InterlinkMachine` objects. When the virtual-kubelet registers the Node and its
`Ready` condition becomes `True`, the controller marks the corresponding
`InterlinkMachine` as ready and copies the node addresses into
`InterlinkMachine.status.addresses`.

**Rationale:** The Kubernetes `Node` object is the authoritative signal that
the virtual node is functional. Watching it directly (rather than polling the
`VirtualNode` CRD) means the machine becomes ready as quickly as possible,
and the controller reacts to node health changes without separate timers.

### 6. Finalizer-Based Lifecycle

All cleanup is gated behind a Kubernetes finalizer
(`interlinkmachine.infrastructure.cluster.x-k8s.io`). The finalizer is added
on the first successful reconcile and removed only after the `VirtualNode` (and
plugin Pod / Service, if applicable) have been deleted.

**Rationale:** Prevents orphaned `VirtualNode` resources when a machine is
deleted before the controller has had a chance to run. Kubernetes defers the
object's deletion until all finalizers are removed, so the controller always
gets at least one reconcile for cleanup.

### 7. CAPI Conditions for Status Propagation

The provider uses the CAPI conditions API
(`sigs.k8s.io/cluster-api/util/conditions`) to surface granular status:

| Condition | What it tracks |
|-----------|----------------|
| `VirtualNodeReady` | The `VirtualNode` resource exists and the corresponding Node is Ready. |
| `PluginPodReady` | The plugin Pod is `Running` and the Service is available (pilot mode only). |
| `Ready` (summary) | Aggregated summary of the above conditions. |

**Rationale:** CAPI tooling (e.g. `clusterctl describe cluster`) reads these
standard conditions to give operators a unified health view across all provider
types. Using the CAPI conditions library ensures correct semantics
(severity, reason codes, observed generation) with minimal boilerplate.

### 8. controller-runtime Reconciler Pattern

Controllers are implemented using
[controller-runtime](https://github.com/kubernetes-sigs/controller-runtime)
and follow the standard reconcile loop:

- **Idempotency:** `controllerutil.CreateOrUpdate` is used for all child
  objects (Service, Pod, VirtualNode). Running the reconciler multiple times
  produces the same end state.
- **Requeue on transient conditions:** when a resource is not yet ready (e.g.
  plugin Pod not Running, Node not Ready) the reconciler returns
  `ctrl.Result{RequeueAfter: 30s}` rather than blocking.
- **Patch helper:** status and spec updates use CAPI's `patch.Helper` to
  produce minimal patch payloads and avoid lost-update races.

## Benefits Summary

| Benefit | How it is achieved |
|---------|--------------------|
| Drop-in CAPI integration | Strict conformance to the infrastructure provider contract |
| Autoscaling support | Compatible with Cluster Autoscaler's CAPI provider out of the box |
| No idle interLink processes | Plugin Pods are created/destroyed with each machine (pilot mode) |
| Single control point | Owner references cascade deletion through all child objects |
| HPC / remote resource access | Plugin Pods inherit virtual-node connectivity to the target system |
| MachineDeployment support | Template CRDs enable rolling updates and replica management |
| Observability | Standard CAPI conditions surfaced via `clusterctl` and the API |
| Existing deployments supported | Static `interLinkAddress` mode requires no changes to running infrastructure |

## Sequence Diagrams

### Machine Provisioning (Pilot Mode)

```
User / Autoscaler          CAPI core             InterlinkMachineReconciler
      │                       │                          │
      │  create MachineDeployment                        │
      │──────────────────────►│                          │
      │                       │  create Machine          │
      │                       │──────────────────────────►
      │                       │  create InterlinkMachine │
      │                       │──────────────────────────►
      │                                                  │
      │                              reconcileNormal     │
      │                              ──────────────────► │
      │                              create Pod + Service│
      │                              ──────────────────► │
      │                              (wait for Pod Ready)│
      │                              ──────────────────► │
      │                              create VirtualNode  │
      │                              ──────────────────► │
      │                              (wait for Node Ready)
      │                              ──────────────────► │
      │                              set ready=true      │
      │                              ──────────────────► │
      │                       │ Machine.status.ready=true│
      │                       │◄─────────────────────────│
```

### Machine Deletion

```
User / Autoscaler          CAPI core             InterlinkMachineReconciler
      │                       │                          │
      │  delete Machine        │                         │
      │──────────────────────►│                          │
      │                       │  delete InterlinkMachine │
      │                       │──────────────────────────►
      │                                                  │
      │                              reconcileDelete     │
      │                              ──────────────────► │
      │                              delete VirtualNode  │
      │                              ──────────────────► │
      │                              remove finalizer    │
      │                              ──────────────────► │
      │                       │  (Pod/Service GC'd via   │
      │                       │   owner references)      │
```
