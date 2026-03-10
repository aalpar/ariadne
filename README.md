# Ariadne

Ariadne builds a directed dependency graph of Kubernetes resources. You feed it
K8s API objects; it resolves the relationships between them and gives you a
queryable, observable graph.

Named after Ariadne of Greek mythology, who gave Theseus the thread to navigate
the Labyrinth.

## Goals

Many tools visualize Kubernetes resource relationships — kubectl-tree,
kube-lineage, KubeView, kubectl-graph, Lens Resource Map — but they all connect
to a live cluster and own the full pipeline from API discovery to rendering.
The relationship knowledge (how a Pod references a ConfigMap, how a Service
selects Pods) is hard-coded inside each tool, reimplemented independently, and
not reusable.

Ariadne is a **library, not a tool**. It aims to be the shared foundation that
tools embed:

- **Embeddable anywhere.** No cluster connection, no informers, no HTTP
  servers. You provide `unstructured.Unstructured` objects from any source —
  client-go, YAML on disk, test fixtures — and get a graph back. This makes it
  usable in operators, CI pipelines, static analysis, GitOps tools, and custom
  dashboards.

- **Minimal dependency surface.** Only `k8s.io/apimachinery`. No client-go, no
  controller-runtime, no graph libraries. Consumers don't inherit a dependency
  tree that conflicts with their own.

- **Declarative, shareable rules.** Most resource relationships are expressed as
  data (`RefRule`, `LabelSelectorRule`), not code. A rule like "Certificate
  references a Secret via `spec.secretName`" is a struct literal, not a
  function. Rules can be packaged, published, and composed — the same primitives
  the built-in resolvers use are available to users for CRDs and custom
  resources. Some relationships (like `ownerReferences`) are generic across all
  kinds and handled by dedicated code rather than rules.

- **Incremental and reactive.** `Add`/`Remove` with change listeners, not just
  one-shot graph construction. This supports live use in controllers and
  informer-based systems, not just static analysis.

The long-term bet is that community-contributed rule sets for popular CRDs
(cert-manager, Istio, ArgoCD, Crossplane, Prometheus Operator, etc.) become the
shared, tool-agnostic registry of how Kubernetes resources relate to each other.

## Install

```
go get github.com/aalpar/ariadne
```

## Quick start

```go
package main

import (
	"fmt"
	"os"

	"github.com/aalpar/ariadne"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func main() {
	// NewDefault registers the built-in structural, selector, and event resolvers.
	g := ariadne.NewDefault()

	// Feed objects from any source (client-go, informers, YAML, etc.)
	g.Load([]unstructured.Unstructured{
		makeConfigMap("default", "app-config"),
		makePod("default", "web", "app-config"),
		makeService("default", "web-svc", map[string]interface{}{"app": "web"}),
	})

	// Query: what does the pod depend on?
	podRef := ariadne.ObjectRef{Kind: "Pod", Namespace: "default", Name: "web"}
	for _, edge := range g.DependenciesOf(podRef) {
		fmt.Printf("%s -> %s  (%s via %s)\n", edge.From, edge.To, edge.Type, edge.Field)
	}

	// Transitive: everything upstream of the service
	svcRef := ariadne.ObjectRef{Kind: "Service", Namespace: "default", Name: "web-svc"}
	for _, ref := range g.Upstream(svcRef) {
		fmt.Println("  upstream:", ref)
	}

	// Export for visualization
	g.ExportDOT(os.Stdout)
}
```

## How it works

When you add objects to the graph, registered **resolvers** inspect each object
and emit edges. Resolvers are bidirectional: adding a Pod discovers that the Pod
depends on a ConfigMap, *and* adding a ConfigMap discovers that existing Pods
depend on it.

```
Add(obj) → for each Resolver → Resolve(obj, lookup) → []Edge → notify listeners
```

Edges point from dependent to dependency: `From` depends on `To`.

### `Load` vs `Add`

- **`Load`** inserts all nodes first, then resolves edges, then notifies
  listeners. Use this for initial sync so resolvers can see the full object set.
- **`Add`** resolves per-object as it goes. Use this for incremental watch
  updates.

## Built-in resolvers

`NewDefault()` registers all three:

### Structural

Direct field references between known K8s resource types:

| From | To | Field |
|---|---|---|
| Pod | ServiceAccount | `spec.serviceAccountName` |
| Pod | ConfigMap | `spec.volumes[*].configMap.name` |
| Pod | ConfigMap | `spec.volumes[*].projected.sources[*].configMap.name` |
| Pod | ConfigMap | `spec.{,init,ephemeral}Containers[*].envFrom[*].configMapRef.name` |
| Pod | ConfigMap | `spec.{,init,ephemeral}Containers[*].env[*].valueFrom.configMapKeyRef.name` |
| Pod | Secret | `spec.imagePullSecrets[*].name` |
| Pod | Secret | `spec.volumes[*].secret.secretName` |
| Pod | Secret | `spec.volumes[*].projected.sources[*].secret.name` |
| Pod | Secret | `spec.{,init,ephemeral}Containers[*].envFrom[*].secretRef.name` |
| Pod | Secret | `spec.{,init,ephemeral}Containers[*].env[*].valueFrom.secretKeyRef.name` |
| Pod | PersistentVolumeClaim | `spec.volumes[*].persistentVolumeClaim.claimName` |
| Pod | Node | `spec.nodeName` |
| Pod | PriorityClass | `spec.priorityClassName` |
| Pod | RuntimeClass | `spec.runtimeClassName` |
| PVC | PersistentVolume | `spec.volumeName` |
| PVC | StorageClass | `spec.storageClassName` |
| PV | StorageClass | `spec.storageClassName` |
| Ingress | Service | `spec.rules[*].http.paths[*].backend.service.name` |
| Ingress | Service | `spec.defaultBackend.service.name` |
| Ingress | Secret | `spec.tls[*].secretName` |
| Ingress | IngressClass | `spec.ingressClassName` |
| StatefulSet | Service | `spec.serviceName` |
| *any* | *owner* | `metadata.ownerReferences` |

### Selector

Label/selector matching:

| From | To | Selector Field |
|---|---|---|
| Service | Pod | `spec.selector` |
| NetworkPolicy | Pod | `spec.podSelector` |
| PodDisruptionBudget | Pod | `spec.selector.matchLabels` |

### Event

Creates edges from K8s Event `involvedObject` references to the events
themselves.

## Custom resolvers

### Declarative rules

For CRDs and custom resources, compose rules from the same primitives the
built-in resolvers use:

```go
g := ariadne.New(
	ariadne.WithResolver(ariadne.NewRuleResolver("my-app",
		ariadne.RefRule{
			FromGroup: "db.example.com", FromKind: "DatabaseCluster",
			ToKind:    "Secret",
			FieldPath: "spec.credentialsSecretName",
		},
		ariadne.RefRule{
			FromGroup: "gateway.example.com", FromKind: "HTTPRoute",
			ToKind:             "Service",
			FieldPath:          "spec.backendRefs[*].name",
			NamespaceFieldPath: "spec.backendRefs[*].namespace",
		},
		ariadne.LabelSelectorRule{
			FromGroup: "app.example.com", FromKind: "Canary",
			ToKind:            "Pod",
			SelectorFieldPath: "spec.selector.matchLabels",
		},
	)),
)
```

**Rule types:**

- `RefRule` — a field contains the name of a target resource, with optional
  `NamespaceFieldPath` for cross-namespace references. When no namespace field
  is specified, resolution tries the source's namespace first, then
  cluster-scoped — rules don't need to know whether the target is namespaced.
- `LabelSelectorRule` — match targets by label selector

### Resolver interface

For logic beyond declarative rules, implement the interface directly:

```go
type Resolver interface {
	Name() string
	Resolve(obj *unstructured.Unstructured, lookup Lookup) []Edge
}
```

Resolvers receive a read-only `Lookup` to query existing graph nodes. They
cannot mutate the graph or access edges.

## Graph API

### Query

```go
g.DependenciesOf(ref)  // direct outgoing edges (what ref depends on)
g.DependentsOf(ref)    // direct incoming edges (what depends on ref)
g.Upstream(ref)        // transitive closure of DependenciesOf
g.Downstream(ref)      // transitive closure of DependentsOf
g.Nodes()              // all ObjectRefs
g.Edges()              // all Edges
g.Has(ref)             // membership check
```

### Topological operations

```go
sorted, err := g.TopologicalSort()  // dependency order; returns ErrCycle if cycles exist
cycles := g.Cycles()                // find all elementary cycles
```

### Change notifications

```go
g := ariadne.NewDefault(
	ariadne.WithListener(func(event ariadne.GraphEvent) {
		// event.Type: NodeAdded, NodeRemoved, EdgeAdded, EdgeRemoved
		// event.Ref:  set for node events
		// event.Edge: set for edge events
	}),
)
```

Listeners fire synchronously under the write lock. Dispatch expensive work to a
goroutine.

### Export

```go
g.ExportDOT(w)   // Graphviz DOT format
g.ExportJSON(w)   // JSON with nodes and edges arrays
```

## Concurrency

`Graph` is safe for concurrent use. Reads (`DependenciesOf`, `Upstream`, etc.)
run concurrently; writes (`Add`, `Remove`, `Load`) are exclusive.

## Dependencies

- `k8s.io/apimachinery` — unstructured types, schema, label selectors
- Go standard library

Does **not** depend on client-go, controller-runtime, or any graph library.
