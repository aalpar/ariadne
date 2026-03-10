# Ariadne: Kubernetes Resource Dependency Tracker

**Date:** 2026-03-09
**Status:** Approved

## Overview

Ariadne is a Go library that builds and maintains a directed dependency graph
of Kubernetes resources. The caller provides K8s API objects (loaded via
client-go, informers, or any other mechanism); Ariadne resolves dependencies
between them and exposes a queryable, observable graph.

The name references Ariadne of Greek mythology, who gave Theseus the thread to
navigate the Labyrinth -- the same operation this library performs on the maze
of interconnected K8s resources.

## Goals

- Build a dependency graph from K8s API objects provided by the caller
- Support batch loading (initial sync) and incremental add/remove (watch updates)
- Resolve dependencies through multiple strategies: structural references,
  label/selector matching, K8s Events, and user-defined rules
- Provide graph query operations: single-hop, transitive closure, topological
  sort, cycle detection
- Notify listeners of graph changes (node/edge additions and removals)
- Export to DOT and JSON for debugging and visualization
- Minimize dependencies: only `k8s.io/apimachinery`; no client-go,
  controller-runtime, or third-party graph libraries

## Non-Goals

- Connecting to a K8s cluster (the caller does this)
- Watching resources (the caller feeds objects via Add/Remove)
- Visualizing the graph (DOT/JSON export is for debugging; rendering is external)
- Persisting the graph to disk

## Architecture

Single `Graph` struct with pluggable `Resolver` interface. Each resolver
implements one dependency-detection strategy. When objects are added or removed,
resolvers evaluate edges and listeners are notified.

```
Graph
 +-- nodes: map[ObjectRef]*node
 +-- outEdges: map[ObjectRef][]*Edge   (from -> edges)
 +-- inEdges: map[ObjectRef][]*Edge    (to -> edges)
 +-- resolvers: []Resolver
 +-- listeners: []ChangeListener
```

## Core Types

### ObjectRef

Uniquely identifies a K8s resource. Uses GroupKind (not GroupVersionKind)
because different API versions of the same kind refer to the same logical
object.

```go
type ObjectRef struct {
    Group     string  // "" for core API group
    Kind      string  // "Pod", "Service", etc.
    Namespace string  // "" for cluster-scoped resources
    Name      string
}
```

### Edge

A directed dependency between two resources.

```go
type Edge struct {
    From     ObjectRef
    To       ObjectRef
    Type     EdgeType   // how the dependency was discovered
    Resolver string     // which resolver produced this edge
    Field    string     // source field path, e.g. "spec.volumes[0].configMap.name"
}
```

### EdgeType

Classifies the mechanism of the dependency.

```go
type EdgeType int

const (
    EdgeNameRef       EdgeType = iota  // direct namespace+name reference
    EdgeLocalNameRef                    // name reference within same namespace
    EdgeLabelSelector                   // label/selector match
    EdgeEvent                           // inferred from K8s Event
    EdgeCustom                          // user-defined
)
```

### GraphEvent / ChangeListener

```go
type ChangeListener func(event GraphEvent)

type GraphEvent struct {
    Type EventType
    Ref  *ObjectRef  // for node events
    Edge *Edge       // for edge events
}

type EventType int

const (
    NodeAdded   EventType = iota
    NodeRemoved
    EdgeAdded
    EdgeRemoved
)
```

## Graph API

### Construction

```go
func New(opts ...Option) *Graph
func NewDefault() *Graph  // all built-in resolvers registered

func WithResolver(r Resolver) Option
func WithListener(fn ChangeListener) Option
```

### Mutation

```go
func (g *Graph) Add(objs ...unstructured.Unstructured)
func (g *Graph) Remove(refs ...ObjectRef)
func (g *Graph) Load(objs []unstructured.Unstructured)
```

- `Add`: adds objects, runs resolvers, notifies listeners per-object.
- `Remove`: removes objects and all associated edges, notifies listeners.
- `Load`: batch add. Resolves all edges after all objects are added. Emits
  notifications after the full batch is resolved (listeners see a consistent
  graph, not a half-built one).

### Query -- Single Hop

```go
func (g *Graph) DependenciesOf(ref ObjectRef) []Edge  // outgoing
func (g *Graph) DependentsOf(ref ObjectRef) []Edge    // incoming
```

### Query -- Transitive

```go
func (g *Graph) Upstream(ref ObjectRef) []ObjectRef    // transitive DependenciesOf
func (g *Graph) Downstream(ref ObjectRef) []ObjectRef  // transitive DependentsOf
```

### Topological Operations

```go
func (g *Graph) TopologicalSort() ([]ObjectRef, error)  // error if cycles
func (g *Graph) Cycles() [][]ObjectRef
```

### Introspection

```go
func (g *Graph) Nodes() []ObjectRef
func (g *Graph) Edges() []Edge
func (g *Graph) Has(ref ObjectRef) bool
```

### Export

```go
func (g *Graph) ExportDOT(w io.Writer) error
func (g *Graph) ExportJSON(w io.Writer) error
```

## Resolver Interface

```go
type Resolver interface {
    Name() string
    Resolve(obj *unstructured.Unstructured, lookup Lookup) []Edge
}

type Lookup interface {
    Get(ref ObjectRef) (*unstructured.Unstructured, bool)
    List(group, kind string) []*unstructured.Unstructured
    ListInNamespace(group, kind, namespace string) []*unstructured.Unstructured
}
```

When an object is added:

1. Object is inserted into the node map.
2. Each resolver receives `Resolve(obj, lookup)`.
3. Resolver returns edges in both directions -- "obj depends on X" and
   "existing Y depends on obj".
4. Edges are added; listeners notified.

When an object is removed:

1. All edges where `From == ref || To == ref` are removed.
2. Node is removed.
3. Listeners notified (edge removals first, then node removal).

Resolvers only read the graph via `Lookup`. They cannot mutate the graph or
access edges. This keeps resolvers testable and prevents circular
dependencies.

## Built-in Primitives (Rule Types)

Reusable building blocks for constructing resolvers. Users compose rules
without implementing the `Resolver` interface directly.

```go
type Rule interface{ rule() }

// NameRefRule: field contains the name of a target resource.
type NameRefRule struct {
    FromGroup, FromKind string
    ToGroup, ToKind     string
    FieldPath           string  // JSONPath-like
    SameNamespace       bool    // target assumed same namespace as source
}

// NamespacedNameRefRule: explicit namespace + name fields.
type NamespacedNameRefRule struct {
    FromGroup, FromKind string
    ToGroup, ToKind     string
    NameFieldPath       string
    NamespaceFieldPath  string  // "" means same namespace as source
}

// LabelSelectorRule: match target resources by label selector.
type LabelSelectorRule struct {
    FromGroup, FromKind string
    ToGroup, ToKind     string
    SelectorFieldPath   string
    TargetNamespace     string  // "" = same namespace; "*" = all namespaces
}

func NewRuleResolver(name string, rules ...Rule) Resolver
```

Example -- user-defined CRD dependency:

```go
ariadne.NameRefRule{
    FromGroup: "db.example.com", FromKind: "DatabaseCluster",
    ToGroup:   "",               ToKind:   "Secret",
    FieldPath: "spec.credentialsSecretName",
    SameNamespace: true,
}
```

## Built-in Resolvers

### StructuralResolver

Hardcoded rules for known K8s core resource references. Implemented as
`NewRuleResolver("structural", ...rules)` -- same primitive system users get.

| From | To | Field |
|---|---|---|
| Pod | ServiceAccount | `spec.serviceAccountName` |
| Pod | ConfigMap | `spec.volumes[*].configMap.name` |
| Pod | ConfigMap | `spec.containers[*].envFrom[*].configMapRef.name` |
| Pod | Secret | `spec.volumes[*].secret.secretName` |
| Pod | Secret | `spec.containers[*].envFrom[*].secretRef.name` |
| Pod | PVC | `spec.volumes[*].persistentVolumeClaim.claimName` |
| PVC | PV | `spec.volumeName` |
| PVC | StorageClass | `spec.storageClassName` |
| Ingress | Service | `spec.rules[*].http.paths[*].backend.service.name` |
| any | any (owner) | `metadata.ownerReferences[*]` |

The ownerReferences rule is generic: it applies to any resource and creates
an edge from the owned resource to its owner. This handles Deployment ->
ReplicaSet -> Pod chains automatically.

### SelectorResolver

Label/selector matching for resources that select others by labels.

| From | To | Selector Field |
|---|---|---|
| Service | Pod | `spec.selector` |
| NetworkPolicy | Pod | `spec.podSelector` |
| PodDisruptionBudget | Pod | `spec.selector` |

### EventResolver

Treats K8s Event objects as dependency signals. When an Event is added,
creates an edge from the Event's `involvedObject` to the Event itself.
Lower priority than structural/selector edges -- supplementary signal for
cases where the other resolvers have no rule.

### Default Configuration

```go
func NewDefault() *Graph
```

Returns a `Graph` with StructuralResolver, SelectorResolver, and
EventResolver registered. Users can add more resolvers via
`WithResolver` or by calling `NewRuleResolver` with custom rules.

## Concurrency

- `Graph` uses `sync.RWMutex`.
- Read operations (`DependenciesOf`, `DependentsOf`, `Upstream`, etc.)
  take a read lock -- concurrent reads are safe.
- Write operations (`Add`, `Remove`, `Load`) take a write lock.
- Listeners are called synchronously under the write lock. If a listener
  needs to do expensive work, it should dispatch to its own goroutine.

## Serialization

### DOT

```
digraph ariadne {
    "apps/Deployment/default/nginx" -> "core/ConfigMap/default/nginx-config"
        [label="structural:spec.volumes[0].configMap.name"];
    "core/Service/default/nginx" -> "core/Pod/default/nginx-abc123"
        [label="selector:spec.selector"];
}
```

Node labels: `group/Kind/namespace/name` (group shown as "core" for core API).
Edge labels: `resolver:field`.

### JSON

```json
{
  "nodes": [
    {"group": "", "kind": "Pod", "namespace": "default", "name": "nginx-abc123"}
  ],
  "edges": [
    {
      "from": {"group": "apps", "kind": "Deployment", "namespace": "default", "name": "nginx"},
      "to": {"group": "", "kind": "ConfigMap", "namespace": "default", "name": "nginx-config"},
      "type": "name_ref",
      "resolver": "structural",
      "field": "spec.volumes[0].configMap.name"
    }
  ]
}
```

## Module Structure

```
github.com/<user>/ariadne/
    go.mod              module github.com/<user>/ariadne
    graph.go            Graph struct, Add/Remove/Load, query methods
    types.go            ObjectRef, Edge, EdgeType, GraphEvent, etc.
    resolver.go         Resolver interface, Lookup interface
    rules.go            Rule types (NameRefRule, etc.), NewRuleResolver
    structural.go       built-in structural resolver rules
    selector.go         built-in selector resolver
    event.go            built-in event resolver
    export.go           ExportDOT, ExportJSON
    topo.go             TopologicalSort, Cycles
    *_test.go           tests per file
```

## Dependencies

- `k8s.io/apimachinery` -- for `unstructured.Unstructured`,
  `schema.GroupVersionKind`, `labels.Selector`. Minimal K8s types-only
  package.
- Go standard library only beyond that.

Does NOT depend on:

- `client-go` -- the caller loads objects however they want
- `controller-runtime` -- no framework coupling
- Any graph library -- adjacency list is ~100 lines; avoids transitive
  dependency tax

## Open Questions

None -- design approved, ready for implementation planning.
