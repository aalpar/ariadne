# CRD Example Resolvers — Design

Date: 2026-03-09

## Goal

Example resolvers showing how users extend Ariadne for their own CRDs. Each is an exported constructor (`NewXxxResolver()`) not registered by `NewDefault()`. Validated with tests.

## File layout

One file per ecosystem: `<name>.go` + `<name>_test.go`.

## Ecosystems

### Gateway API (`gateway.go`)

Pure RefRule. Three rules:

| From | Field path | To | Type |
|---|---|---|---|
| HTTPRoute (gateway.networking.k8s.io) | `spec.rules[*].backendRefs[*]` | (typed-ref) | typed-ref |
| HTTPRoute | `spec.parentRefs[*]` | (typed-ref, usually Gateway) | typed-ref |
| Gateway (gateway.networking.k8s.io) | `spec.gatewayClassName` | GatewayClass | bare name |

Tests: HTTPRoute → Service via backendRef, HTTPRoute → Gateway via parentRef (forward + reverse), Gateway → GatewayClass.

### Cluster API (`clusterapi.go`)

Pure RefRule. Six rules, all unconstrained typed-refs (ToGroup/ToKind empty):

| From | Field path |
|---|---|
| Machine (cluster.x-k8s.io) | `spec.infrastructureRef` |
| Machine | `spec.bootstrap.configRef` |
| Cluster (cluster.x-k8s.io) | `spec.infrastructureRef` |
| Cluster | `spec.controlPlaneRef` |
| MachineDeployment (cluster.x-k8s.io) | `spec.template.spec.infrastructureRef` |
| MachineDeployment | `spec.template.spec.bootstrap.configRef` |

Tests: Machine → DockerMachine, Cluster → DockerCluster + KubeadmControlPlane, forward + reverse.

### Crossplane (`crossplane.go`)

Two patterns:

**a) providerConfigRef** — RefRule for a concrete managed resource type. Example uses `database.aws.crossplane.io/RDSInstance`. Users replicate the pattern for their own managed resource types.

**b) compositeTypeRef** — Custom Resolver. Composition `spec.compositeTypeRef` has group+kind but no name. Resolver matches all objects of that GroupKind in the graph. Edge type: `EdgeCustom`.

Tests: RDSInstance → ProviderConfig via providerConfigRef. Composition → all XMyDatabase instances via compositeTypeRef.

### Kyverno (`kyverno.go`)

Custom Resolver. ClusterPolicy/Policy `spec.rules[*].match.resources.kinds` lists kind strings. Resolver matches all instances of those kinds.

Forward: Policy added → find all objects of matching kinds.
Reverse: Object added → check all Policies for matching kind lists.

Edge type: `EdgeCustom`.

Tests: ClusterPolicy listing "Pod" → matches all Pods. Reverse: Pod added after Policy → edge created.

### Argo CD (`argocd.go`)

Pure RefRule. Two rules:

| From | Field path | To |
|---|---|---|
| Application (argoproj.io) | `spec.destination.namespace` | Namespace (core) |
| Application | `spec.project` | AppProject (argoproj.io) |

Tests: Application → Namespace, Application → AppProject, forward + reverse.
