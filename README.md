# Ariadne

Ariadne builds a directed dependency graph of Kubernetes resources. You feed it
K8s API objects; it resolves the relationships between them and gives you a
queryable, observable graph.

Named after Ariadne of Greek mythology, who gave Theseus the thread to navigate
the Labyrinth.

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
| Pod | ConfigMap | `spec.containers[*].envFrom[*].configMapRef.name` |
| Pod | Secret | `spec.volumes[*].secret.secretName` |
| Pod | Secret | `spec.containers[*].envFrom[*].secretRef.name` |
| Pod | PersistentVolumeClaim | `spec.volumes[*].persistentVolumeClaim.claimName` |
| PVC | PersistentVolume | `spec.volumeName` |
| PVC | StorageClass | `spec.storageClassName` |
| Ingress | Service | `spec.rules[*].http.paths[*].backend.service.name` |
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
		ariadne.NameRefRule{
			FromGroup: "db.example.com", FromKind: "DatabaseCluster",
			ToKind:    "Secret",
			FieldPath: "spec.credentialsSecretName",
			SameNamespace: true,
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

- `NameRefRule` — a field contains the name of a target resource
- `NamespacedNameRefRule` — explicit namespace + name field pair
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
