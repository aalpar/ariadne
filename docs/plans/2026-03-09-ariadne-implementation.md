# Ariadne Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Implement the Ariadne library — a Go package that builds and queries a directed dependency graph of Kubernetes resources.

**Architecture:** Single `Graph` struct with pluggable `Resolver` interface. Rule-based primitives for user-extensible dependency definitions. Adjacency list graph with concurrent-safe reads and exclusive writes.

**Tech Stack:** Go 1.26, `k8s.io/apimachinery` (unstructured, schema, labels)

**Design doc:** `docs/plans/2026-03-09-ariadne-design.md`

---

### Task 1: Module init + core types + interfaces

**Files:**
- Create: `go.mod`
- Create: `types.go`
- Create: `resolver.go`

**Step 1: Initialize Go module**

Run:
```bash
cd /Users/aalpar/projects/ariadne
go mod init github.com/aalpar/ariadne
go get k8s.io/apimachinery@latest
```

**Step 2: Create types.go**

```go
package ariadne

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ObjectRef uniquely identifies a K8s resource.
// Uses GroupKind (not GVK) because different API versions
// of the same kind refer to the same logical object.
type ObjectRef struct {
	Group     string
	Kind      string
	Namespace string
	Name      string
}

func (r ObjectRef) String() string {
	g := r.Group
	if g == "" {
		g = "core"
	}
	if r.Namespace == "" {
		return fmt.Sprintf("%s/%s/%s", g, r.Kind, r.Name)
	}
	return fmt.Sprintf("%s/%s/%s/%s", g, r.Kind, r.Namespace, r.Name)
}

// RefFromUnstructured extracts an ObjectRef from an unstructured K8s object.
func RefFromUnstructured(obj *unstructured.Unstructured) ObjectRef {
	gvk := obj.GroupVersionKind()
	return ObjectRef{
		Group:     gvk.Group,
		Kind:      gvk.Kind,
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
}

// EdgeType classifies how a dependency was discovered.
type EdgeType int

const (
	EdgeNameRef       EdgeType = iota // direct namespace+name reference
	EdgeLocalNameRef                  // name reference within same namespace
	EdgeLabelSelector                 // label/selector match
	EdgeEvent                         // inferred from K8s Event
	EdgeCustom                        // user-defined
)

func (t EdgeType) String() string {
	switch t {
	case EdgeNameRef:
		return "name_ref"
	case EdgeLocalNameRef:
		return "local_name_ref"
	case EdgeLabelSelector:
		return "label_selector"
	case EdgeEvent:
		return "event"
	case EdgeCustom:
		return "custom"
	default:
		return fmt.Sprintf("unknown(%d)", int(t))
	}
}

// Edge is a directed dependency between two resources.
type Edge struct {
	From     ObjectRef
	To       ObjectRef
	Type     EdgeType
	Resolver string
	Field    string
}

// EventType classifies a graph change event.
type EventType int

const (
	NodeAdded EventType = iota
	NodeRemoved
	EdgeAdded
	EdgeRemoved
)

// GraphEvent represents a change to the graph.
type GraphEvent struct {
	Type EventType
	Ref  *ObjectRef
	Edge *Edge
}

// ChangeListener receives graph change notifications.
type ChangeListener func(event GraphEvent)
```

**Step 3: Create resolver.go**

```go
package ariadne

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

// Resolver discovers dependency edges involving a given object.
// Resolve returns edges in both directions: "obj depends on X"
// and "existing Y depends on obj".
type Resolver interface {
	Name() string
	Resolve(obj *unstructured.Unstructured, lookup Lookup) []Edge
}

// Lookup provides read-only access to objects in the graph.
// Resolvers use this to find potential dependency targets.
type Lookup interface {
	Get(ref ObjectRef) (*unstructured.Unstructured, bool)
	List(group, kind string) []*unstructured.Unstructured
	ListInNamespace(group, kind, namespace string) []*unstructured.Unstructured
}
```

**Step 4: Verify it compiles**

Run: `cd /Users/aalpar/projects/ariadne && go build ./...`
Expected: clean compile, no errors

**Step 5: Commit**

```bash
git add go.mod go.sum types.go resolver.go
git commit -m "Add core types and resolver interface

ObjectRef, Edge, EdgeType, GraphEvent, ChangeListener, Resolver, Lookup."
```

---

### Task 2: Graph — nodes, mutations, basic queries

**Files:**
- Create: `graph.go`
- Create: `graph_test.go`

**Step 1: Write the failing test**

Create `graph_test.go` with a test helper for building unstructured objects,
then test Add/Remove/Has/Nodes.

```go
package ariadne

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func newObj(group, version, kind, namespace, name string) unstructured.Unstructured {
	obj := unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{Group: group, Version: version, Kind: kind})
	obj.SetNamespace(namespace)
	obj.SetName(name)
	if obj.Object == nil {
		obj.Object = make(map[string]interface{})
	}
	return obj
}

func newCoreObj(kind, namespace, name string) unstructured.Unstructured {
	return newObj("", "v1", kind, namespace, name)
}

func TestAddAndHas(t *testing.T) {
	g := New()

	pod := newCoreObj("Pod", "default", "nginx")
	g.Add(pod)

	ref := ObjectRef{Kind: "Pod", Namespace: "default", Name: "nginx"}
	if !g.Has(ref) {
		t.Fatal("expected graph to contain the pod")
	}

	unknown := ObjectRef{Kind: "Pod", Namespace: "default", Name: "unknown"}
	if g.Has(unknown) {
		t.Fatal("expected graph to not contain unknown pod")
	}
}

func TestNodes(t *testing.T) {
	g := New()
	g.Add(
		newCoreObj("Pod", "default", "a"),
		newCoreObj("Pod", "default", "b"),
		newCoreObj("ConfigMap", "default", "c"),
	)

	nodes := g.Nodes()
	if len(nodes) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(nodes))
	}
}

func TestRemove(t *testing.T) {
	g := New()
	pod := newCoreObj("Pod", "default", "nginx")
	g.Add(pod)

	ref := ObjectRef{Kind: "Pod", Namespace: "default", Name: "nginx"}
	g.Remove(ref)

	if g.Has(ref) {
		t.Fatal("expected pod to be removed")
	}
	if len(g.Nodes()) != 0 {
		t.Fatal("expected empty graph after remove")
	}
}

func TestRemoveNonexistent(t *testing.T) {
	g := New()
	// Should not panic
	g.Remove(ObjectRef{Kind: "Pod", Namespace: "default", Name: "nope"})
}
```

**Step 2: Run test to verify it fails**

Run: `cd /Users/aalpar/projects/ariadne && go test -v -run 'TestAdd|TestNodes|TestRemove' ./...`
Expected: FAIL — `New` not defined

**Step 3: Write graph.go**

```go
package ariadne

import (
	"sync"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type node struct {
	ref ObjectRef
	obj *unstructured.Unstructured
}

// Graph is a directed dependency graph of Kubernetes resources.
type Graph struct {
	mu        sync.RWMutex
	nodes     map[ObjectRef]*node
	outEdges  map[ObjectRef][]Edge
	inEdges   map[ObjectRef][]Edge
	resolvers []Resolver
	listeners []ChangeListener
}

// Option configures a Graph during construction.
type Option func(*Graph)

// WithResolver adds a resolver to the graph.
func WithResolver(r Resolver) Option {
	return func(g *Graph) {
		g.resolvers = append(g.resolvers, r)
	}
}

// WithListener adds a change listener to the graph.
func WithListener(fn ChangeListener) Option {
	return func(g *Graph) {
		g.listeners = append(g.listeners, fn)
	}
}

// New creates a new empty graph.
func New(opts ...Option) *Graph {
	g := &Graph{
		nodes:    make(map[ObjectRef]*node),
		outEdges: make(map[ObjectRef][]Edge),
		inEdges:  make(map[ObjectRef][]Edge),
	}
	for _, opt := range opts {
		opt(g)
	}
	return g
}

func (g *Graph) notify(event GraphEvent) {
	for _, fn := range g.listeners {
		fn(event)
	}
}

func (g *Graph) addEdge(e Edge) {
	for _, existing := range g.outEdges[e.From] {
		if existing == e {
			return
		}
	}
	g.outEdges[e.From] = append(g.outEdges[e.From], e)
	g.inEdges[e.To] = append(g.inEdges[e.To], e)
	g.notify(GraphEvent{Type: EdgeAdded, Edge: &e})
}

func (g *Graph) removeEdge(e Edge) {
	out := g.outEdges[e.From]
	for i, existing := range out {
		if existing == e {
			g.outEdges[e.From] = append(out[:i], out[i+1:]...)
			break
		}
	}
	in := g.inEdges[e.To]
	for i, existing := range in {
		if existing == e {
			g.inEdges[e.To] = append(in[:i], in[i+1:]...)
			break
		}
	}
}

// Add adds objects to the graph, resolves their dependencies, and notifies listeners.
func (g *Graph) Add(objs ...unstructured.Unstructured) {
	g.mu.Lock()
	defer g.mu.Unlock()

	lookup := &graphLookup{nodes: g.nodes}

	for i := range objs {
		objCopy := objs[i]
		ref := RefFromUnstructured(&objCopy)

		g.nodes[ref] = &node{ref: ref, obj: &objCopy}
		g.notify(GraphEvent{Type: NodeAdded, Ref: &ref})

		for _, r := range g.resolvers {
			for _, e := range r.Resolve(&objCopy, lookup) {
				g.addEdge(e)
			}
		}
	}
}

// Remove removes objects and all their edges from the graph.
func (g *Graph) Remove(refs ...ObjectRef) {
	g.mu.Lock()
	defer g.mu.Unlock()

	for _, ref := range refs {
		if _, ok := g.nodes[ref]; !ok {
			continue
		}

		// Remove outgoing edges
		for _, e := range g.outEdges[ref] {
			g.removeEdge(e)
			g.notify(GraphEvent{Type: EdgeRemoved, Edge: &e})
		}

		// Remove incoming edges
		for _, e := range g.inEdges[ref] {
			g.removeEdge(e)
			g.notify(GraphEvent{Type: EdgeRemoved, Edge: &e})
		}

		delete(g.outEdges, ref)
		delete(g.inEdges, ref)
		delete(g.nodes, ref)
		g.notify(GraphEvent{Type: NodeRemoved, Ref: &ref})
	}
}

// Has returns whether the graph contains the given resource.
func (g *Graph) Has(ref ObjectRef) bool {
	g.mu.RLock()
	defer g.mu.RUnlock()
	_, ok := g.nodes[ref]
	return ok
}

// Nodes returns all object refs in the graph.
func (g *Graph) Nodes() []ObjectRef {
	g.mu.RLock()
	defer g.mu.RUnlock()
	refs := make([]ObjectRef, 0, len(g.nodes))
	for ref := range g.nodes {
		refs = append(refs, ref)
	}
	return refs
}

// Edges returns all edges in the graph.
func (g *Graph) Edges() []Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	var edges []Edge
	for _, ee := range g.outEdges {
		edges = append(edges, ee...)
	}
	return edges
}

// DependenciesOf returns outgoing edges from the given resource (what it depends on).
func (g *Graph) DependenciesOf(ref ObjectRef) []Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	edges := g.outEdges[ref]
	result := make([]Edge, len(edges))
	copy(result, edges)
	return result
}

// DependentsOf returns incoming edges to the given resource (what depends on it).
func (g *Graph) DependentsOf(ref ObjectRef) []Edge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	edges := g.inEdges[ref]
	result := make([]Edge, len(edges))
	copy(result, edges)
	return result
}

// graphLookup implements Lookup backed by the graph's node map.
type graphLookup struct {
	nodes map[ObjectRef]*node
}

func (l *graphLookup) Get(ref ObjectRef) (*unstructured.Unstructured, bool) {
	n, ok := l.nodes[ref]
	if !ok {
		return nil, false
	}
	return n.obj, true
}

func (l *graphLookup) List(group, kind string) []*unstructured.Unstructured {
	var result []*unstructured.Unstructured
	for ref, n := range l.nodes {
		if ref.Group == group && ref.Kind == kind {
			result = append(result, n.obj)
		}
	}
	return result
}

func (l *graphLookup) ListInNamespace(group, kind, namespace string) []*unstructured.Unstructured {
	var result []*unstructured.Unstructured
	for ref, n := range l.nodes {
		if ref.Group == group && ref.Kind == kind && ref.Namespace == namespace {
			result = append(result, n.obj)
		}
	}
	return result
}
```

**Step 4: Run tests to verify they pass**

Run: `cd /Users/aalpar/projects/ariadne && go test -v -run 'TestAdd|TestNodes|TestRemove' ./...`
Expected: PASS (4 tests)

**Step 5: Commit**

```bash
git add graph.go graph_test.go
git commit -m "Add Graph with Add/Remove/Has/Nodes/Edges/DependenciesOf/DependentsOf"
```

---

### Task 3: Edge resolution and dependency queries

**Files:**
- Modify: `graph_test.go`

Tests that use a stub resolver to verify edge creation on Add and cleanup on Remove.

**Step 1: Write the failing tests**

Append to `graph_test.go`:

```go
// stubResolver creates an edge from any Pod to any ConfigMap with the same namespace.
type stubResolver struct{}

func (s *stubResolver) Name() string { return "stub" }

func (s *stubResolver) Resolve(obj *unstructured.Unstructured, lookup Lookup) []Edge {
	ref := RefFromUnstructured(obj)
	var edges []Edge

	if ref.Kind == "Pod" {
		// Pod -> ConfigMap: check if any ConfigMaps exist
		for _, cm := range lookup.ListInNamespace("", "ConfigMap", ref.Namespace) {
			cmRef := RefFromUnstructured(cm)
			edges = append(edges, Edge{
				From:     ref,
				To:       cmRef,
				Type:     EdgeLocalNameRef,
				Resolver: "stub",
				Field:    "test",
			})
		}
	}

	if ref.Kind == "ConfigMap" {
		// Reverse: check if any existing Pods should depend on this ConfigMap
		for _, pod := range lookup.ListInNamespace("", "Pod", ref.Namespace) {
			podRef := RefFromUnstructured(pod)
			edges = append(edges, Edge{
				From:     podRef,
				To:       ref,
				Type:     EdgeLocalNameRef,
				Resolver: "stub",
				Field:    "test",
			})
		}
	}

	return edges
}

func TestEdgeResolution(t *testing.T) {
	g := New(WithResolver(&stubResolver{}))

	cm := newCoreObj("ConfigMap", "default", "config")
	pod := newCoreObj("Pod", "default", "nginx")
	g.Add(cm, pod)

	podRef := ObjectRef{Kind: "Pod", Namespace: "default", Name: "nginx"}
	cmRef := ObjectRef{Kind: "ConfigMap", Namespace: "default", Name: "config"}

	deps := g.DependenciesOf(podRef)
	if len(deps) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(deps))
	}
	if deps[0].To != cmRef {
		t.Fatalf("expected dependency to ConfigMap, got %v", deps[0].To)
	}

	dependents := g.DependentsOf(cmRef)
	if len(dependents) != 1 {
		t.Fatalf("expected 1 dependent, got %d", len(dependents))
	}
	if dependents[0].From != podRef {
		t.Fatalf("expected dependent from Pod, got %v", dependents[0].From)
	}
}

func TestRemoveCleansEdges(t *testing.T) {
	g := New(WithResolver(&stubResolver{}))

	cm := newCoreObj("ConfigMap", "default", "config")
	pod := newCoreObj("Pod", "default", "nginx")
	g.Add(cm, pod)

	podRef := ObjectRef{Kind: "Pod", Namespace: "default", Name: "nginx"}
	g.Remove(podRef)

	if len(g.Edges()) != 0 {
		t.Fatalf("expected 0 edges after removing pod, got %d", len(g.Edges()))
	}
}
```

**Step 2: Run tests**

Run: `cd /Users/aalpar/projects/ariadne && go test -v -run 'TestEdge|TestRemoveCleans' ./...`
Expected: PASS

**Step 3: Commit**

```bash
git add graph_test.go
git commit -m "Add edge resolution and dependency query tests"
```

---

### Task 4: Load (batch) and change notifications

**Files:**
- Modify: `graph.go` (add Load method)
- Modify: `graph_test.go`

**Step 1: Write the failing tests**

Append to `graph_test.go`:

```go
func TestChangeNotifications(t *testing.T) {
	var events []GraphEvent
	g := New(
		WithResolver(&stubResolver{}),
		WithListener(func(e GraphEvent) {
			events = append(events, e)
		}),
	)

	pod := newCoreObj("Pod", "default", "nginx")
	cm := newCoreObj("ConfigMap", "default", "config")
	g.Add(cm, pod)

	// Expect: NodeAdded(cm), NodeAdded(pod), EdgeAdded(pod->cm)
	nodeAdded := 0
	edgeAdded := 0
	for _, e := range events {
		switch e.Type {
		case NodeAdded:
			nodeAdded++
		case EdgeAdded:
			edgeAdded++
		}
	}
	if nodeAdded != 2 {
		t.Fatalf("expected 2 NodeAdded events, got %d", nodeAdded)
	}
	if edgeAdded != 1 {
		t.Fatalf("expected 1 EdgeAdded event, got %d", edgeAdded)
	}
}

func TestLoadBatchesNotifications(t *testing.T) {
	var events []GraphEvent
	graphReady := false

	g := New(
		WithResolver(&stubResolver{}),
		WithListener(func(e GraphEvent) {
			// During Load, all notifications should arrive after
			// all objects are in the graph. Verify by checking that
			// the graph has all nodes when the first event fires.
			if !graphReady && e.Type == NodeAdded {
				graphReady = true
			}
			events = append(events, e)
		}),
	)

	objs := []unstructured.Unstructured{
		newCoreObj("ConfigMap", "default", "config"),
		newCoreObj("Pod", "default", "nginx"),
	}
	g.Load(objs)

	if len(events) == 0 {
		t.Fatal("expected events from Load")
	}

	// Verify same result as Add: 2 nodes, 1 edge
	if len(g.Nodes()) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(g.Nodes()))
	}
	if len(g.Edges()) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(g.Edges()))
	}
}
```

**Step 2: Run tests to verify Load fails**

Run: `cd /Users/aalpar/projects/ariadne && go test -v -run 'TestChange|TestLoad' ./...`
Expected: FAIL — `Load` not defined

**Step 3: Add Load method to graph.go**

```go
// Load adds all objects to the graph before resolving edges.
// Notifications are emitted after the full batch is processed,
// so listeners see a consistent graph.
func (g *Graph) Load(objs []unstructured.Unstructured) {
	g.mu.Lock()
	defer g.mu.Unlock()

	// Phase 1: insert all nodes
	refs := make([]ObjectRef, len(objs))
	for i := range objs {
		objCopy := objs[i]
		ref := RefFromUnstructured(&objCopy)
		refs[i] = ref
		g.nodes[ref] = &node{ref: ref, obj: &objCopy}
	}

	lookup := &graphLookup{nodes: g.nodes}
	var events []GraphEvent

	for i := range refs {
		r := refs[i]
		events = append(events, GraphEvent{Type: NodeAdded, Ref: &r})
	}

	// Phase 2: resolve all edges
	for i := range objs {
		for _, r := range g.resolvers {
			for _, e := range r.Resolve(g.nodes[refs[i]].obj, lookup) {
				// Deduplicate
				dup := false
				for _, existing := range g.outEdges[e.From] {
					if existing == e {
						dup = true
						break
					}
				}
				if !dup {
					g.outEdges[e.From] = append(g.outEdges[e.From], e)
					g.inEdges[e.To] = append(g.inEdges[e.To], e)
					eCopy := e
					events = append(events, GraphEvent{Type: EdgeAdded, Edge: &eCopy})
				}
			}
		}
	}

	// Phase 3: notify
	for _, event := range events {
		g.notify(event)
	}
}
```

**Step 4: Run tests**

Run: `cd /Users/aalpar/projects/ariadne && go test -v -run 'TestChange|TestLoad' ./...`
Expected: PASS

**Step 5: Commit**

```bash
git add graph.go graph_test.go
git commit -m "Add Load (batch add) with deferred notifications"
```

---

### Task 5: Transitive queries — Upstream and Downstream

**Files:**
- Modify: `graph.go` (add Upstream, Downstream)
- Modify: `graph_test.go`

**Step 1: Write the failing test**

Append to `graph_test.go`:

```go
// chainResolver creates edges: A -> B -> C (by name convention "a"->"b"->"c")
type chainResolver struct{}

func (c *chainResolver) Name() string { return "chain" }

func (c *chainResolver) Resolve(obj *unstructured.Unstructured, lookup Lookup) []Edge {
	ref := RefFromUnstructured(obj)
	var edges []Edge

	chain := map[string]string{"a": "b", "b": "c"}
	if target, ok := chain[ref.Name]; ok {
		toRef := ObjectRef{Kind: ref.Kind, Namespace: ref.Namespace, Name: target}
		if _, exists := lookup.Get(toRef); exists {
			edges = append(edges, Edge{
				From: ref, To: toRef,
				Type: EdgeLocalNameRef, Resolver: "chain", Field: "test",
			})
		}
	}

	reverse := map[string]string{"b": "a", "c": "b"}
	if source, ok := reverse[ref.Name]; ok {
		fromRef := ObjectRef{Kind: ref.Kind, Namespace: ref.Namespace, Name: source}
		if _, exists := lookup.Get(fromRef); exists {
			edges = append(edges, Edge{
				From: fromRef, To: ref,
				Type: EdgeLocalNameRef, Resolver: "chain", Field: "test",
			})
		}
	}

	return edges
}

func TestUpstream(t *testing.T) {
	g := New(WithResolver(&chainResolver{}))
	g.Load([]unstructured.Unstructured{
		newCoreObj("Pod", "default", "a"),
		newCoreObj("Pod", "default", "b"),
		newCoreObj("Pod", "default", "c"),
	})

	// c's upstream: b, a (transitive)
	upstream := g.Upstream(ObjectRef{Kind: "Pod", Namespace: "default", Name: "c"})
	if len(upstream) != 2 {
		t.Fatalf("expected 2 upstream nodes, got %d: %v", len(upstream), upstream)
	}
}

func TestDownstream(t *testing.T) {
	g := New(WithResolver(&chainResolver{}))
	g.Load([]unstructured.Unstructured{
		newCoreObj("Pod", "default", "a"),
		newCoreObj("Pod", "default", "b"),
		newCoreObj("Pod", "default", "c"),
	})

	// a's downstream: b, c (transitive)
	downstream := g.Downstream(ObjectRef{Kind: "Pod", Namespace: "default", Name: "a"})
	if len(downstream) != 2 {
		t.Fatalf("expected 2 downstream nodes, got %d: %v", len(downstream), downstream)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /Users/aalpar/projects/ariadne && go test -v -run 'TestUpstream|TestDownstream' ./...`
Expected: FAIL — `Upstream` not defined

**Step 3: Add Upstream and Downstream to graph.go**

```go
// Upstream returns the transitive closure of DependenciesOf (all ancestors).
func (g *Graph) Upstream(ref ObjectRef) []ObjectRef {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.walkTransitive(ref, func(r ObjectRef) []Edge {
		return g.outEdges[r]
	}, func(e Edge) ObjectRef {
		return e.To
	})
}

// Downstream returns the transitive closure of DependentsOf (all descendants).
func (g *Graph) Downstream(ref ObjectRef) []ObjectRef {
	g.mu.RLock()
	defer g.mu.RUnlock()
	return g.walkTransitive(ref, func(r ObjectRef) []Edge {
		return g.inEdges[r]
	}, func(e Edge) ObjectRef {
		return e.From
	})
}

// walkTransitive does BFS from ref, following edges via getEdges/getNeighbor.
func (g *Graph) walkTransitive(
	ref ObjectRef,
	getEdges func(ObjectRef) []Edge,
	getNeighbor func(Edge) ObjectRef,
) []ObjectRef {
	visited := map[ObjectRef]bool{ref: true}
	queue := []ObjectRef{ref}
	var result []ObjectRef

	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, e := range getEdges(current) {
			neighbor := getNeighbor(e)
			if !visited[neighbor] {
				visited[neighbor] = true
				result = append(result, neighbor)
				queue = append(queue, neighbor)
			}
		}
	}
	return result
}
```

**Step 4: Run tests**

Run: `cd /Users/aalpar/projects/ariadne && go test -v -run 'TestUpstream|TestDownstream' ./...`
Expected: PASS

**Step 5: Commit**

```bash
git add graph.go graph_test.go
git commit -m "Add Upstream and Downstream transitive queries"
```

---

### Task 6: Topological sort and cycle detection

**Files:**
- Create: `topo.go`
- Create: `topo_test.go`

**Step 1: Write the failing tests**

```go
package ariadne

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestTopologicalSort(t *testing.T) {
	g := New(WithResolver(&chainResolver{}))
	g.Load([]unstructured.Unstructured{
		newCoreObj("Pod", "default", "a"),
		newCoreObj("Pod", "default", "b"),
		newCoreObj("Pod", "default", "c"),
	})

	sorted, err := g.TopologicalSort()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(sorted) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(sorted))
	}

	// "a" must come before "b", "b" before "c"
	pos := map[ObjectRef]int{}
	for i, ref := range sorted {
		pos[ref] = i
	}
	a := ObjectRef{Kind: "Pod", Namespace: "default", Name: "a"}
	b := ObjectRef{Kind: "Pod", Namespace: "default", Name: "b"}
	c := ObjectRef{Kind: "Pod", Namespace: "default", Name: "c"}

	if pos[a] > pos[b] || pos[b] > pos[c] {
		t.Fatalf("wrong order: a=%d b=%d c=%d", pos[a], pos[b], pos[c])
	}
}

func TestTopologicalSortCycleError(t *testing.T) {
	// cycleResolver: a -> b -> a
	g := New(WithResolver(&cycleResolver{}))
	g.Load([]unstructured.Unstructured{
		newCoreObj("Pod", "default", "a"),
		newCoreObj("Pod", "default", "b"),
	})

	_, err := g.TopologicalSort()
	if err == nil {
		t.Fatal("expected cycle error")
	}
}

func TestCycles(t *testing.T) {
	g := New(WithResolver(&cycleResolver{}))
	g.Load([]unstructured.Unstructured{
		newCoreObj("Pod", "default", "a"),
		newCoreObj("Pod", "default", "b"),
	})

	cycles := g.Cycles()
	if len(cycles) == 0 {
		t.Fatal("expected at least 1 cycle")
	}
}

func TestCyclesEmpty(t *testing.T) {
	g := New(WithResolver(&chainResolver{}))
	g.Load([]unstructured.Unstructured{
		newCoreObj("Pod", "default", "a"),
		newCoreObj("Pod", "default", "b"),
		newCoreObj("Pod", "default", "c"),
	})

	cycles := g.Cycles()
	if len(cycles) != 0 {
		t.Fatalf("expected 0 cycles, got %d", len(cycles))
	}
}

// cycleResolver creates a -> b -> a
type cycleResolver struct{}

func (c *cycleResolver) Name() string { return "cycle" }

func (c *cycleResolver) Resolve(obj *unstructured.Unstructured, lookup Lookup) []Edge {
	ref := RefFromUnstructured(obj)
	var edges []Edge

	cycle := map[string]string{"a": "b", "b": "a"}
	if target, ok := cycle[ref.Name]; ok {
		toRef := ObjectRef{Kind: ref.Kind, Namespace: ref.Namespace, Name: target}
		if _, exists := lookup.Get(toRef); exists {
			edges = append(edges, Edge{
				From: ref, To: toRef,
				Type: EdgeLocalNameRef, Resolver: "cycle", Field: "test",
			})
		}
	}
	return edges
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /Users/aalpar/projects/ariadne && go test -v -run 'TestTopological|TestCycle' ./...`
Expected: FAIL — `TopologicalSort` not defined

**Step 3: Create topo.go**

```go
package ariadne

import "errors"

// ErrCycle is returned by TopologicalSort when the graph contains a cycle.
var ErrCycle = errors.New("graph contains a cycle")

// TopologicalSort returns nodes in dependency order (dependencies first).
// Returns ErrCycle if the graph contains a cycle.
func (g *Graph) TopologicalSort() ([]ObjectRef, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Kahn's algorithm
	inDegree := make(map[ObjectRef]int, len(g.nodes))
	for ref := range g.nodes {
		inDegree[ref] = len(g.outEdges[ref])
	}

	var queue []ObjectRef
	for ref, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, ref)
		}
	}

	var sorted []ObjectRef
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		sorted = append(sorted, current)

		for _, e := range g.inEdges[current] {
			inDegree[e.From]--
			if inDegree[e.From] == 0 {
				queue = append(queue, e.From)
			}
		}
	}

	if len(sorted) != len(g.nodes) {
		return nil, ErrCycle
	}
	return sorted, nil
}

// Cycles finds all elementary cycles in the graph using Johnson's algorithm
// simplified to DFS-based cycle detection.
func (g *Graph) Cycles() [][]ObjectRef {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var cycles [][]ObjectRef
	visited := make(map[ObjectRef]bool)
	onStack := make(map[ObjectRef]bool)
	path := []ObjectRef{}

	var dfs func(ref ObjectRef)
	dfs = func(ref ObjectRef) {
		visited[ref] = true
		onStack[ref] = true
		path = append(path, ref)

		for _, e := range g.outEdges[ref] {
			if !visited[e.To] {
				dfs(e.To)
			} else if onStack[e.To] {
				// Found a cycle: extract it from path
				start := -1
				for i, r := range path {
					if r == e.To {
						start = i
						break
					}
				}
				if start >= 0 {
					cycle := make([]ObjectRef, len(path[start:]))
					copy(cycle, path[start:])
					cycles = append(cycles, cycle)
				}
			}
		}

		path = path[:len(path)-1]
		onStack[ref] = false
	}

	for ref := range g.nodes {
		if !visited[ref] {
			dfs(ref)
		}
	}

	return cycles
}
```

**Step 4: Run tests**

Run: `cd /Users/aalpar/projects/ariadne && go test -v -run 'TestTopological|TestCycle' ./...`
Expected: PASS

**Step 5: Commit**

```bash
git add topo.go topo_test.go
git commit -m "Add TopologicalSort and Cycles detection"
```

---

### Task 7: Field path extraction + rule types + NewRuleResolver

**Files:**
- Create: `rules.go`
- Create: `rules_test.go`

This is the engine that makes built-in and user-defined resolvers work.

**Step 1: Write the failing tests**

```go
package ariadne

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

func TestExtractFieldValues_Simple(t *testing.T) {
	obj := map[string]interface{}{
		"spec": map[string]interface{}{
			"volumeName": "my-pv",
		},
	}
	vals := extractFieldValues(obj, "spec.volumeName")
	if len(vals) != 1 || vals[0] != "my-pv" {
		t.Fatalf("expected [my-pv], got %v", vals)
	}
}

func TestExtractFieldValues_Wildcard(t *testing.T) {
	obj := map[string]interface{}{
		"spec": map[string]interface{}{
			"volumes": []interface{}{
				map[string]interface{}{
					"configMap": map[string]interface{}{
						"name": "cm-a",
					},
				},
				map[string]interface{}{
					"secret": map[string]interface{}{
						"secretName": "sec-b",
					},
				},
				map[string]interface{}{
					"configMap": map[string]interface{}{
						"name": "cm-c",
					},
				},
			},
		},
	}

	vals := extractFieldValues(obj, "spec.volumes[*].configMap.name")
	if len(vals) != 2 {
		t.Fatalf("expected 2 values, got %d: %v", len(vals), vals)
	}
}

func TestExtractFieldValues_NestedWildcard(t *testing.T) {
	obj := map[string]interface{}{
		"spec": map[string]interface{}{
			"containers": []interface{}{
				map[string]interface{}{
					"envFrom": []interface{}{
						map[string]interface{}{
							"configMapRef": map[string]interface{}{
								"name": "env-cm",
							},
						},
					},
				},
			},
		},
	}

	vals := extractFieldValues(obj, "spec.containers[*].envFrom[*].configMapRef.name")
	if len(vals) != 1 || vals[0] != "env-cm" {
		t.Fatalf("expected [env-cm], got %v", vals)
	}
}

func TestExtractFieldValues_Missing(t *testing.T) {
	obj := map[string]interface{}{
		"spec": map[string]interface{}{},
	}
	vals := extractFieldValues(obj, "spec.volumeName")
	if len(vals) != 0 {
		t.Fatalf("expected empty, got %v", vals)
	}
}

func TestNameRefRule(t *testing.T) {
	r := NewRuleResolver("test", NameRefRule{
		FromGroup: "", FromKind: "PersistentVolumeClaim",
		ToGroup: "", ToKind: "PersistentVolume",
		FieldPath:     "spec.volumeName",
		SameNamespace: false,
	})

	pvc := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "PersistentVolumeClaim",
		"metadata": map[string]interface{}{
			"name":      "my-pvc",
			"namespace": "default",
		},
		"spec": map[string]interface{}{
			"volumeName": "my-pv",
		},
	}}

	pv := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "PersistentVolume",
		"metadata": map[string]interface{}{
			"name": "my-pv",
		},
	}}

	lookup := &stubLookup{
		objects: map[ObjectRef]*unstructured.Unstructured{
			{Kind: "PersistentVolume", Name: "my-pv"}: pv,
		},
	}

	edges := r.Resolve(pvc, lookup)
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if edges[0].To.Kind != "PersistentVolume" || edges[0].To.Name != "my-pv" {
		t.Fatalf("unexpected edge target: %v", edges[0].To)
	}
}

func TestLabelSelectorRule(t *testing.T) {
	r := NewRuleResolver("test", LabelSelectorRule{
		FromGroup: "", FromKind: "Service",
		ToGroup: "", ToKind: "Pod",
		SelectorFieldPath: "spec.selector",
	})

	svc := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata": map[string]interface{}{
			"name":      "my-svc",
			"namespace": "default",
		},
		"spec": map[string]interface{}{
			"selector": map[string]interface{}{
				"app": "nginx",
			},
		},
	}}

	matchingPod := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":      "nginx-abc",
			"namespace": "default",
			"labels": map[string]interface{}{
				"app": "nginx",
			},
		},
	}}

	nonMatchingPod := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":      "redis-xyz",
			"namespace": "default",
			"labels": map[string]interface{}{
				"app": "redis",
			},
		},
	}}

	lookup := &stubLookup{
		objects: map[ObjectRef]*unstructured.Unstructured{
			{Kind: "Pod", Namespace: "default", Name: "nginx-abc"}: matchingPod,
			{Kind: "Pod", Namespace: "default", Name: "redis-xyz"}: nonMatchingPod,
		},
	}

	edges := r.Resolve(svc, lookup)
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge (to matching pod), got %d", len(edges))
	}
	if edges[0].To.Name != "nginx-abc" {
		t.Fatalf("expected edge to nginx-abc, got %v", edges[0].To.Name)
	}
}

// stubLookup is a simple Lookup implementation for unit tests.
type stubLookup struct {
	objects map[ObjectRef]*unstructured.Unstructured
}

func (s *stubLookup) Get(ref ObjectRef) (*unstructured.Unstructured, bool) {
	obj, ok := s.objects[ref]
	return obj, ok
}

func (s *stubLookup) List(group, kind string) []*unstructured.Unstructured {
	var result []*unstructured.Unstructured
	for ref, obj := range s.objects {
		if ref.Group == group && ref.Kind == kind {
			result = append(result, obj)
		}
	}
	return result
}

func (s *stubLookup) ListInNamespace(group, kind, namespace string) []*unstructured.Unstructured {
	var result []*unstructured.Unstructured
	for ref, obj := range s.objects {
		if ref.Group == group && ref.Kind == kind && ref.Namespace == namespace {
			result = append(result, obj)
		}
	}
	return result
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /Users/aalpar/projects/ariadne && go test -v -run 'TestExtract|TestNameRef|TestLabelSelector' ./...`
Expected: FAIL — `extractFieldValues` not defined

**Step 3: Create rules.go**

```go
package ariadne

import (
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
)

// Rule is a declarative dependency rule.
type Rule interface {
	rule() // marker method
}

// NameRefRule matches a field that contains the name of a target resource.
type NameRefRule struct {
	FromGroup, FromKind string
	ToGroup, ToKind     string
	FieldPath           string
	SameNamespace       bool
}

func (NameRefRule) rule() {}

// NamespacedNameRefRule matches explicit namespace+name field pairs.
type NamespacedNameRefRule struct {
	FromGroup, FromKind string
	ToGroup, ToKind     string
	NameFieldPath       string
	NamespaceFieldPath  string // "" means same namespace as source
}

func (NamespacedNameRefRule) rule() {}

// LabelSelectorRule matches target resources by label selector.
type LabelSelectorRule struct {
	FromGroup, FromKind string
	ToGroup, ToKind     string
	SelectorFieldPath   string
	TargetNamespace     string // "" = same namespace; "*" = all namespaces
}

func (LabelSelectorRule) rule() {}

// NewRuleResolver creates a Resolver from declarative rules.
func NewRuleResolver(name string, rules ...Rule) Resolver {
	return &ruleResolver{name: name, rules: rules}
}

type ruleResolver struct {
	name  string
	rules []Rule
}

func (r *ruleResolver) Name() string { return r.name }

func (r *ruleResolver) Resolve(obj *unstructured.Unstructured, lookup Lookup) []Edge {
	ref := RefFromUnstructured(obj)
	var edges []Edge

	for _, rule := range r.rules {
		switch rule := rule.(type) {
		case NameRefRule:
			edges = append(edges, resolveNameRef(ref, obj, rule, lookup)...)
		case NamespacedNameRefRule:
			edges = append(edges, resolveNamespacedNameRef(ref, obj, rule, lookup)...)
		case LabelSelectorRule:
			edges = append(edges, resolveLabelSelector(ref, obj, rule, lookup)...)
		}
	}

	return edges
}

func resolveNameRef(ref ObjectRef, obj *unstructured.Unstructured, rule NameRefRule, lookup Lookup) []Edge {
	if ref.Group != rule.FromGroup || ref.Kind != rule.FromKind {
		// Check reverse: is this object a potential target?
		return resolveNameRefReverse(ref, obj, rule, lookup)
	}

	var edges []Edge
	names := extractFieldValues(obj.Object, rule.FieldPath)
	for _, name := range names {
		toRef := ObjectRef{
			Group: rule.ToGroup,
			Kind:  rule.ToKind,
			Name:  name,
		}
		if rule.SameNamespace {
			toRef.Namespace = ref.Namespace
		}
		if _, ok := lookup.Get(toRef); ok {
			edgeType := EdgeNameRef
			if rule.SameNamespace {
				edgeType = EdgeLocalNameRef
			}
			edges = append(edges, Edge{
				From:     ref,
				To:       toRef,
				Type:     edgeType,
				Resolver: "rule",
				Field:    rule.FieldPath,
			})
		}
	}
	return edges
}

func resolveNameRefReverse(ref ObjectRef, obj *unstructured.Unstructured, rule NameRefRule, lookup Lookup) []Edge {
	if ref.Group != rule.ToGroup || ref.Kind != rule.ToKind {
		return nil
	}

	var edges []Edge
	var sources []*unstructured.Unstructured
	if rule.SameNamespace {
		sources = lookup.ListInNamespace(rule.FromGroup, rule.FromKind, ref.Namespace)
	} else {
		sources = lookup.List(rule.FromGroup, rule.FromKind)
	}

	for _, src := range sources {
		srcRef := RefFromUnstructured(src)
		names := extractFieldValues(src.Object, rule.FieldPath)
		for _, name := range names {
			if name == ref.Name {
				edgeType := EdgeNameRef
				if rule.SameNamespace {
					edgeType = EdgeLocalNameRef
				}
				edges = append(edges, Edge{
					From:     srcRef,
					To:       ref,
					Type:     edgeType,
					Resolver: "rule",
					Field:    rule.FieldPath,
				})
			}
		}
	}
	return edges
}

func resolveNamespacedNameRef(ref ObjectRef, obj *unstructured.Unstructured, rule NamespacedNameRefRule, lookup Lookup) []Edge {
	if ref.Group != rule.FromGroup || ref.Kind != rule.FromKind {
		return nil
	}

	var edges []Edge
	names := extractFieldValues(obj.Object, rule.NameFieldPath)

	var namespaces []string
	if rule.NamespaceFieldPath == "" {
		for range names {
			namespaces = append(namespaces, ref.Namespace)
		}
	} else {
		namespaces = extractFieldValues(obj.Object, rule.NamespaceFieldPath)
	}

	for i, name := range names {
		ns := ref.Namespace
		if i < len(namespaces) {
			ns = namespaces[i]
		}
		toRef := ObjectRef{
			Group:     rule.ToGroup,
			Kind:      rule.ToKind,
			Namespace: ns,
			Name:      name,
		}
		if _, ok := lookup.Get(toRef); ok {
			edges = append(edges, Edge{
				From:     ref,
				To:       toRef,
				Type:     EdgeNameRef,
				Resolver: "rule",
				Field:    rule.NameFieldPath,
			})
		}
	}
	return edges
}

func resolveLabelSelector(ref ObjectRef, obj *unstructured.Unstructured, rule LabelSelectorRule, lookup Lookup) []Edge {
	if ref.Group != rule.FromGroup || ref.Kind != rule.FromKind {
		// Reverse: is this object a potential target?
		return resolveLabelSelectorReverse(ref, obj, rule, lookup)
	}

	selectorMap := extractMapValue(obj.Object, rule.SelectorFieldPath)
	if selectorMap == nil {
		return nil
	}

	sel := labels.SelectorFromSet(labels.Set(selectorMap))

	ns := ref.Namespace
	if rule.TargetNamespace != "" && rule.TargetNamespace != "*" {
		ns = rule.TargetNamespace
	}

	var targets []*unstructured.Unstructured
	if rule.TargetNamespace == "*" {
		targets = lookup.List(rule.ToGroup, rule.ToKind)
	} else {
		targets = lookup.ListInNamespace(rule.ToGroup, rule.ToKind, ns)
	}

	var edges []Edge
	for _, target := range targets {
		targetLabels := target.GetLabels()
		if sel.Matches(labels.Set(targetLabels)) {
			edges = append(edges, Edge{
				From:     ref,
				To:       RefFromUnstructured(target),
				Type:     EdgeLabelSelector,
				Resolver: "rule",
				Field:    rule.SelectorFieldPath,
			})
		}
	}
	return edges
}

func resolveLabelSelectorReverse(ref ObjectRef, obj *unstructured.Unstructured, rule LabelSelectorRule, lookup Lookup) []Edge {
	if ref.Group != rule.ToGroup || ref.Kind != rule.ToKind {
		return nil
	}

	targetLabels := obj.GetLabels()
	if len(targetLabels) == 0 {
		return nil
	}

	var sources []*unstructured.Unstructured
	if rule.TargetNamespace == "*" {
		sources = lookup.List(rule.FromGroup, rule.FromKind)
	} else {
		sources = lookup.ListInNamespace(rule.FromGroup, rule.FromKind, ref.Namespace)
	}

	var edges []Edge
	for _, src := range sources {
		selectorMap := extractMapValue(src.Object, rule.SelectorFieldPath)
		if selectorMap == nil {
			continue
		}
		sel := labels.SelectorFromSet(labels.Set(selectorMap))
		if sel.Matches(labels.Set(targetLabels)) {
			edges = append(edges, Edge{
				From:     RefFromUnstructured(src),
				To:       ref,
				Type:     EdgeLabelSelector,
				Resolver: "rule",
				Field:    rule.SelectorFieldPath,
			})
		}
	}
	return edges
}

// extractFieldValues extracts string values from a nested map using a
// dot-separated field path. Supports [*] wildcard for slices.
// Example: "spec.volumes[*].configMap.name"
func extractFieldValues(obj map[string]interface{}, path string) []string {
	parts := splitFieldPath(path)
	return extractRecursive(obj, parts)
}

func extractRecursive(data interface{}, parts []string) []string {
	if len(parts) == 0 {
		if s, ok := data.(string); ok {
			return []string{s}
		}
		return nil
	}

	part := parts[0]
	rest := parts[1:]

	if strings.HasSuffix(part, "[*]") {
		key := strings.TrimSuffix(part, "[*]")
		m, ok := data.(map[string]interface{})
		if !ok {
			return nil
		}
		arr, ok := m[key].([]interface{})
		if !ok {
			return nil
		}
		var result []string
		for _, item := range arr {
			result = append(result, extractRecursive(item, rest)...)
		}
		return result
	}

	m, ok := data.(map[string]interface{})
	if !ok {
		return nil
	}
	val, ok := m[part]
	if !ok {
		return nil
	}
	return extractRecursive(val, rest)
}

// extractMapValue extracts a map[string]string from a nested path.
// Used for label selectors.
func extractMapValue(obj map[string]interface{}, path string) map[string]string {
	parts := splitFieldPath(path)
	var current interface{} = obj
	for _, part := range parts {
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil
		}
		current, ok = m[part]
		if !ok {
			return nil
		}
	}

	m, ok := current.(map[string]interface{})
	if !ok {
		return nil
	}
	result := make(map[string]string, len(m))
	for k, v := range m {
		if s, ok := v.(string); ok {
			result[k] = s
		}
	}
	return result
}

func splitFieldPath(path string) []string {
	return strings.Split(path, ".")
}
```

**Step 4: Run tests**

Run: `cd /Users/aalpar/projects/ariadne && go test -v -run 'TestExtract|TestNameRef|TestLabelSelector' ./...`
Expected: PASS

**Step 5: Commit**

```bash
git add rules.go rules_test.go
git commit -m "Add field path extraction, rule types, and NewRuleResolver"
```

---

### Task 8: Built-in structural, selector, and event resolvers

**Files:**
- Create: `structural.go`
- Create: `selector.go`
- Create: `event.go`
- Create: `structural_test.go`
- Create: `selector_test.go`
- Create: `event_test.go`

**Step 1: Create structural.go**

```go
package ariadne

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

// NewStructuralResolver returns a resolver for known K8s resource references.
func NewStructuralResolver() Resolver {
	rules := NewRuleResolver("structural",
		// Pod -> ServiceAccount
		NameRefRule{
			FromKind: "Pod", ToKind: "ServiceAccount",
			FieldPath: "spec.serviceAccountName", SameNamespace: true,
		},
		// Pod -> ConfigMap (volumes)
		NameRefRule{
			FromKind: "Pod", ToKind: "ConfigMap",
			FieldPath: "spec.volumes[*].configMap.name", SameNamespace: true,
		},
		// Pod -> ConfigMap (envFrom)
		NameRefRule{
			FromKind: "Pod", ToKind: "ConfigMap",
			FieldPath: "spec.containers[*].envFrom[*].configMapRef.name", SameNamespace: true,
		},
		// Pod -> Secret (volumes)
		NameRefRule{
			FromKind: "Pod", ToKind: "Secret",
			FieldPath: "spec.volumes[*].secret.secretName", SameNamespace: true,
		},
		// Pod -> Secret (envFrom)
		NameRefRule{
			FromKind: "Pod", ToKind: "Secret",
			FieldPath: "spec.containers[*].envFrom[*].secretRef.name", SameNamespace: true,
		},
		// Pod -> PVC
		NameRefRule{
			FromKind: "Pod", ToKind: "PersistentVolumeClaim",
			FieldPath: "spec.volumes[*].persistentVolumeClaim.claimName", SameNamespace: true,
		},
		// PVC -> PV
		NameRefRule{
			FromKind: "PersistentVolumeClaim", ToKind: "PersistentVolume",
			FieldPath: "spec.volumeName", SameNamespace: false,
		},
		// PVC -> StorageClass (storageClassName is cluster-scoped)
		NameRefRule{
			FromGroup: "", FromKind: "PersistentVolumeClaim",
			ToGroup: "storage.k8s.io", ToKind: "StorageClass",
			FieldPath: "spec.storageClassName", SameNamespace: false,
		},
		// Ingress -> Service
		NameRefRule{
			FromGroup: "networking.k8s.io", FromKind: "Ingress",
			ToKind:    "Service",
			FieldPath: "spec.rules[*].http.paths[*].backend.service.name", SameNamespace: true,
		},
	)

	return &structuralResolver{rules: rules}
}

// structuralResolver combines rule-based resolution with ownerReference handling.
type structuralResolver struct {
	rules Resolver
}

func (s *structuralResolver) Name() string { return "structural" }

func (s *structuralResolver) Resolve(obj *unstructured.Unstructured, lookup Lookup) []Edge {
	edges := s.rules.Resolve(obj, lookup)

	// Fix resolver name (the inner ruleResolver uses "structural" but
	// we want consistency)
	for i := range edges {
		edges[i].Resolver = "structural"
	}

	// ownerReferences: generic rule for any resource
	edges = append(edges, resolveOwnerRefs(obj, lookup)...)

	return edges
}

func resolveOwnerRefs(obj *unstructured.Unstructured, lookup Lookup) []Edge {
	ref := RefFromUnstructured(obj)
	owners := obj.GetOwnerReferences()
	var edges []Edge

	for _, owner := range owners {
		ownerRef := ObjectRef{
			Group:     extractGroup(owner.APIVersion),
			Kind:      owner.Kind,
			Namespace: ref.Namespace, // owners are in the same namespace
			Name:      owner.Name,
		}
		if _, ok := lookup.Get(ownerRef); ok {
			edges = append(edges, Edge{
				From:     ref,
				To:       ownerRef,
				Type:     EdgeNameRef,
				Resolver: "structural",
				Field:    "metadata.ownerReferences",
			})
		}
	}
	return edges
}

// extractGroup extracts the API group from an apiVersion string.
// "v1" -> "", "apps/v1" -> "apps"
func extractGroup(apiVersion string) string {
	parts := splitFieldPath(apiVersion)
	if len(parts) == 1 {
		return "" // core group
	}
	return parts[0]
}
```

**Step 2: Write structural_test.go**

```go
package ariadne

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestStructuralResolver_PodConfigMap(t *testing.T) {
	g := New(WithResolver(NewStructuralResolver()))

	cm := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "ConfigMap",
		"metadata": map[string]interface{}{
			"name": "app-config", "namespace": "default",
		},
	}}

	pod := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]interface{}{
			"name": "web", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"volumes": []interface{}{
				map[string]interface{}{
					"configMap": map[string]interface{}{
						"name": "app-config",
					},
				},
			},
		},
	}}

	g.Load([]unstructured.Unstructured{cm, pod})

	podRef := ObjectRef{Kind: "Pod", Namespace: "default", Name: "web"}
	deps := g.DependenciesOf(podRef)
	if len(deps) != 1 {
		t.Fatalf("expected 1 dependency, got %d: %v", len(deps), deps)
	}
	if deps[0].To.Kind != "ConfigMap" || deps[0].To.Name != "app-config" {
		t.Fatalf("unexpected target: %v", deps[0].To)
	}
	if deps[0].Resolver != "structural" {
		t.Fatalf("expected resolver 'structural', got '%s'", deps[0].Resolver)
	}
}

func TestStructuralResolver_OwnerRef(t *testing.T) {
	g := New(WithResolver(NewStructuralResolver()))

	rs := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1", "kind": "ReplicaSet",
		"metadata": map[string]interface{}{
			"name": "web-rs", "namespace": "default",
		},
	}}

	pod := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]interface{}{
			"name": "web-pod", "namespace": "default",
			"ownerReferences": []interface{}{
				map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "ReplicaSet",
					"name":       "web-rs",
					"uid":        "fake-uid",
				},
			},
		},
	}}

	g.Load([]unstructured.Unstructured{rs, pod})

	podRef := ObjectRef{Kind: "Pod", Namespace: "default", Name: "web-pod"}
	deps := g.DependenciesOf(podRef)
	if len(deps) != 1 {
		t.Fatalf("expected 1 owner dep, got %d", len(deps))
	}
	if deps[0].To.Kind != "ReplicaSet" || deps[0].To.Name != "web-rs" {
		t.Fatalf("unexpected owner: %v", deps[0].To)
	}
}

func TestStructuralResolver_PVCToPV(t *testing.T) {
	g := New(WithResolver(NewStructuralResolver()))

	pv := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "PersistentVolume",
		"metadata": map[string]interface{}{"name": "my-pv"},
	}}

	pvc := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "PersistentVolumeClaim",
		"metadata": map[string]interface{}{
			"name": "my-pvc", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"volumeName": "my-pv",
		},
	}}

	g.Load([]unstructured.Unstructured{pv, pvc})

	pvcRef := ObjectRef{Kind: "PersistentVolumeClaim", Namespace: "default", Name: "my-pvc"}
	deps := g.DependenciesOf(pvcRef)
	if len(deps) != 1 {
		t.Fatalf("expected 1 dep, got %d", len(deps))
	}
	if deps[0].To.Kind != "PersistentVolume" {
		t.Fatalf("expected PV target, got %v", deps[0].To)
	}
}
```

**Step 3: Create selector.go**

```go
package ariadne

// NewSelectorResolver returns a resolver for label/selector-based dependencies.
func NewSelectorResolver() Resolver {
	r := NewRuleResolver("selector",
		// Service -> Pod
		LabelSelectorRule{
			FromKind: "Service", ToKind: "Pod",
			SelectorFieldPath: "spec.selector",
		},
		// NetworkPolicy -> Pod
		LabelSelectorRule{
			FromGroup: "networking.k8s.io", FromKind: "NetworkPolicy",
			ToKind:            "Pod",
			SelectorFieldPath: "spec.podSelector",
		},
		// PodDisruptionBudget -> Pod
		LabelSelectorRule{
			FromGroup: "policy", FromKind: "PodDisruptionBudget",
			ToKind:            "Pod",
			SelectorFieldPath: "spec.selector.matchLabels",
		},
	)
	return &namedResolver{name: "selector", inner: r}
}

// namedResolver wraps a resolver with a different name and sets
// the Resolver field on all edges.
type namedResolver struct {
	name  string
	inner Resolver
}

func (n *namedResolver) Name() string { return n.name }

func (n *namedResolver) Resolve(obj *unstructured.Unstructured, lookup Lookup) []Edge {
	edges := n.inner.Resolve(obj, lookup)
	for i := range edges {
		edges[i].Resolver = n.name
	}
	return edges
}
```

Note: needs the unstructured import. Add to selector.go:

```go
import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
```

**Step 4: Write selector_test.go**

```go
package ariadne

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestSelectorResolver_ServiceToPod(t *testing.T) {
	g := New(WithResolver(NewSelectorResolver()))

	svc := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{
			"name": "web-svc", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"selector": map[string]interface{}{
				"app": "web",
			},
		},
	}}

	matchPod := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]interface{}{
			"name": "web-1", "namespace": "default",
			"labels": map[string]interface{}{"app": "web"},
		},
	}}

	noMatchPod := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]interface{}{
			"name": "db-1", "namespace": "default",
			"labels": map[string]interface{}{"app": "db"},
		},
	}}

	g.Load([]unstructured.Unstructured{svc, matchPod, noMatchPod})

	svcRef := ObjectRef{Kind: "Service", Namespace: "default", Name: "web-svc"}
	deps := g.DependenciesOf(svcRef)
	if len(deps) != 1 {
		t.Fatalf("expected 1 dep, got %d", len(deps))
	}
	if deps[0].To.Name != "web-1" {
		t.Fatalf("expected web-1, got %s", deps[0].To.Name)
	}
	if deps[0].Resolver != "selector" {
		t.Fatalf("expected resolver 'selector', got '%s'", deps[0].Resolver)
	}
}
```

**Step 5: Create event.go**

```go
package ariadne

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

// NewEventResolver returns a resolver that creates edges from K8s Events
// to their involvedObject.
func NewEventResolver() Resolver {
	return &eventResolver{}
}

type eventResolver struct{}

func (e *eventResolver) Name() string { return "event" }

func (e *eventResolver) Resolve(obj *unstructured.Unstructured, lookup Lookup) []Edge {
	ref := RefFromUnstructured(obj)

	// Only handle Event objects
	if ref.Kind == "Event" {
		return e.resolveEvent(ref, obj, lookup)
	}

	// Reverse: when a non-Event is added, check existing Events
	return e.resolveReverseEvent(ref, obj, lookup)
}

func (e *eventResolver) resolveEvent(ref ObjectRef, obj *unstructured.Unstructured, lookup Lookup) []Edge {
	involved, ok := obj.Object["involvedObject"].(map[string]interface{})
	if !ok {
		return nil
	}

	name, _ := involved["name"].(string)
	ns, _ := involved["namespace"].(string)
	kind, _ := involved["kind"].(string)
	apiVersion, _ := involved["apiVersion"].(string)

	if name == "" || kind == "" {
		return nil
	}

	involvedRef := ObjectRef{
		Group:     extractGroup(apiVersion),
		Kind:      kind,
		Namespace: ns,
		Name:      name,
	}

	if _, exists := lookup.Get(involvedRef); !exists {
		return nil
	}

	return []Edge{{
		From:     involvedRef,
		To:       ref,
		Type:     EdgeEvent,
		Resolver: "event",
		Field:    "involvedObject",
	}}
}

func (e *eventResolver) resolveReverseEvent(ref ObjectRef, obj *unstructured.Unstructured, lookup Lookup) []Edge {
	events := lookup.List("", "Event")
	var edges []Edge

	for _, evt := range events {
		involved, ok := evt.Object["involvedObject"].(map[string]interface{})
		if !ok {
			continue
		}

		name, _ := involved["name"].(string)
		ns, _ := involved["namespace"].(string)
		kind, _ := involved["kind"].(string)
		apiVersion, _ := involved["apiVersion"].(string)

		involvedRef := ObjectRef{
			Group:     extractGroup(apiVersion),
			Kind:      kind,
			Namespace: ns,
			Name:      name,
		}

		if involvedRef == ref {
			evtRef := RefFromUnstructured(evt)
			edges = append(edges, Edge{
				From:     ref,
				To:       evtRef,
				Type:     EdgeEvent,
				Resolver: "event",
				Field:    "involvedObject",
			})
		}
	}
	return edges
}
```

**Step 6: Write event_test.go**

```go
package ariadne

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestEventResolver(t *testing.T) {
	g := New(WithResolver(NewEventResolver()))

	pod := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]interface{}{
			"name": "web", "namespace": "default",
		},
	}}

	event := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Event",
		"metadata": map[string]interface{}{
			"name": "web.abc123", "namespace": "default",
		},
		"involvedObject": map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"name":       "web",
			"namespace":  "default",
		},
	}}

	g.Load([]unstructured.Unstructured{pod, event})

	podRef := ObjectRef{Kind: "Pod", Namespace: "default", Name: "web"}
	deps := g.DependenciesOf(podRef)
	if len(deps) != 1 {
		t.Fatalf("expected 1 event dep, got %d", len(deps))
	}
	if deps[0].Resolver != "event" {
		t.Fatalf("expected resolver 'event', got '%s'", deps[0].Resolver)
	}
}
```

**Step 7: Run all tests**

Run: `cd /Users/aalpar/projects/ariadne && go test -v ./...`
Expected: ALL PASS

**Step 8: Commit**

```bash
git add structural.go structural_test.go selector.go selector_test.go event.go event_test.go
git commit -m "Add built-in structural, selector, and event resolvers"
```

---

### Task 9: NewDefault + integration test

**Files:**
- Modify: `graph.go` (add NewDefault)
- Create: `integration_test.go`

**Step 1: Add NewDefault to graph.go**

```go
// NewDefault creates a Graph with all built-in resolvers registered.
func NewDefault(opts ...Option) *Graph {
	defaults := []Option{
		WithResolver(NewStructuralResolver()),
		WithResolver(NewSelectorResolver()),
		WithResolver(NewEventResolver()),
	}
	return New(append(defaults, opts...)...)
}
```

**Step 2: Write integration_test.go**

```go
package ariadne

import (
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestIntegration_FullStack(t *testing.T) {
	g := NewDefault()

	objs := []unstructured.Unstructured{
		// ServiceAccount
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "ServiceAccount",
			"metadata": map[string]interface{}{
				"name": "web-sa", "namespace": "default",
			},
		}},
		// ConfigMap
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "ConfigMap",
			"metadata": map[string]interface{}{
				"name": "web-config", "namespace": "default",
			},
		}},
		// Secret
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "Secret",
			"metadata": map[string]interface{}{
				"name": "web-secret", "namespace": "default",
			},
		}},
		// Pod (references SA, ConfigMap, Secret)
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "Pod",
			"metadata": map[string]interface{}{
				"name": "web", "namespace": "default",
				"labels": map[string]interface{}{"app": "web"},
				"ownerReferences": []interface{}{
					map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "ReplicaSet",
						"name":       "web-rs",
						"uid":        "fake",
					},
				},
			},
			"spec": map[string]interface{}{
				"serviceAccountName": "web-sa",
				"volumes": []interface{}{
					map[string]interface{}{
						"configMap": map[string]interface{}{"name": "web-config"},
					},
					map[string]interface{}{
						"secret": map[string]interface{}{"secretName": "web-secret"},
					},
				},
			},
		}},
		// ReplicaSet (owner of Pod)
		{Object: map[string]interface{}{
			"apiVersion": "apps/v1", "kind": "ReplicaSet",
			"metadata": map[string]interface{}{
				"name": "web-rs", "namespace": "default",
			},
		}},
		// Service (selects Pod by label)
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "Service",
			"metadata": map[string]interface{}{
				"name": "web-svc", "namespace": "default",
			},
			"spec": map[string]interface{}{
				"selector": map[string]interface{}{"app": "web"},
			},
		}},
	}

	g.Load(objs)

	// Pod should depend on: SA, ConfigMap, Secret, ReplicaSet (owner)
	podRef := ObjectRef{Kind: "Pod", Namespace: "default", Name: "web"}
	podDeps := g.DependenciesOf(podRef)
	if len(podDeps) != 4 {
		t.Fatalf("expected 4 pod deps, got %d: %v", len(podDeps), podDeps)
	}

	// Service should depend on Pod (selector)
	svcRef := ObjectRef{Kind: "Service", Namespace: "default", Name: "web-svc"}
	svcDeps := g.DependenciesOf(svcRef)
	if len(svcDeps) != 1 {
		t.Fatalf("expected 1 svc dep, got %d", len(svcDeps))
	}
	if svcDeps[0].To != podRef {
		t.Fatalf("expected svc -> pod, got %v", svcDeps[0].To)
	}

	// Topological sort should succeed (no cycles)
	sorted, err := g.TopologicalSort()
	if err != nil {
		t.Fatalf("unexpected cycle: %v", err)
	}
	if len(sorted) != 6 {
		t.Fatalf("expected 6 sorted nodes, got %d", len(sorted))
	}

	// Total edges
	edges := g.Edges()
	if len(edges) != 5 {
		t.Fatalf("expected 5 edges, got %d", len(edges))
	}

	// Service's upstream should include Pod and all Pod's deps
	upstream := g.Upstream(svcRef)
	if len(upstream) < 4 {
		t.Fatalf("expected at least 4 upstream of svc, got %d: %v", len(upstream), upstream)
	}
}
```

**Step 3: Run tests**

Run: `cd /Users/aalpar/projects/ariadne && go test -v -run TestIntegration ./...`
Expected: PASS

**Step 4: Run full test suite**

Run: `cd /Users/aalpar/projects/ariadne && go test -v ./...`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add graph.go integration_test.go
git commit -m "Add NewDefault and integration test with full resource stack"
```

---

### Task 10: Export — DOT and JSON

**Files:**
- Create: `export.go`
- Create: `export_test.go`

**Step 1: Write failing tests**

```go
package ariadne

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestExportDOT(t *testing.T) {
	g := New(WithResolver(&stubResolver{}))
	g.Add(
		newCoreObj("ConfigMap", "default", "config"),
		newCoreObj("Pod", "default", "web"),
	)

	var buf bytes.Buffer
	if err := g.ExportDOT(&buf); err != nil {
		t.Fatal(err)
	}

	dot := buf.String()
	if !strings.Contains(dot, "digraph ariadne") {
		t.Fatal("missing digraph header")
	}
	if !strings.Contains(dot, "core/Pod/default/web") {
		t.Fatal("missing pod node")
	}
	if !strings.Contains(dot, "->") {
		t.Fatal("missing edge arrow")
	}
}

func TestExportJSON(t *testing.T) {
	g := New(WithResolver(&stubResolver{}))
	g.Add(
		newCoreObj("ConfigMap", "default", "config"),
		newCoreObj("Pod", "default", "web"),
	)

	var buf bytes.Buffer
	if err := g.ExportJSON(&buf); err != nil {
		t.Fatal(err)
	}

	var result struct {
		Nodes []ObjectRef `json:"nodes"`
		Edges []struct {
			From     ObjectRef `json:"from"`
			To       ObjectRef `json:"to"`
			Type     string    `json:"type"`
			Resolver string    `json:"resolver"`
			Field    string    `json:"field"`
		} `json:"edges"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(result.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(result.Nodes))
	}
	if len(result.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(result.Edges))
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /Users/aalpar/projects/ariadne && go test -v -run 'TestExport' ./...`
Expected: FAIL — `ExportDOT` not defined

**Step 3: Create export.go**

```go
package ariadne

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

// ExportDOT writes the graph in Graphviz DOT format.
func (g *Graph) ExportDOT(w io.Writer) error {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if _, err := fmt.Fprintln(w, "digraph ariadne {"); err != nil {
		return err
	}

	// Sort nodes for deterministic output
	nodes := make([]ObjectRef, 0, len(g.nodes))
	for ref := range g.nodes {
		nodes = append(nodes, ref)
	}
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].String() < nodes[j].String()
	})

	for _, ref := range nodes {
		if _, err := fmt.Fprintf(w, "    %q;\n", ref.String()); err != nil {
			return err
		}
	}

	// Sort edges for deterministic output
	var edges []Edge
	for _, ee := range g.outEdges {
		edges = append(edges, ee...)
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From.String() != edges[j].From.String() {
			return edges[i].From.String() < edges[j].From.String()
		}
		return edges[i].To.String() < edges[j].To.String()
	})

	for _, e := range edges {
		label := e.Resolver
		if e.Field != "" {
			label += ":" + e.Field
		}
		if _, err := fmt.Fprintf(w, "    %q -> %q [label=%q];\n",
			e.From.String(), e.To.String(), label); err != nil {
			return err
		}
	}

	_, err := fmt.Fprintln(w, "}")
	return err
}

type jsonGraph struct {
	Nodes []ObjectRef `json:"nodes"`
	Edges []jsonEdge  `json:"edges"`
}

type jsonEdge struct {
	From     ObjectRef `json:"from"`
	To       ObjectRef `json:"to"`
	Type     string    `json:"type"`
	Resolver string    `json:"resolver"`
	Field    string    `json:"field"`
}

// ExportJSON writes the graph as JSON.
func (g *Graph) ExportJSON(w io.Writer) error {
	g.mu.RLock()
	defer g.mu.RUnlock()

	nodes := make([]ObjectRef, 0, len(g.nodes))
	for ref := range g.nodes {
		nodes = append(nodes, ref)
	}
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].String() < nodes[j].String()
	})

	var edges []Edge
	for _, ee := range g.outEdges {
		edges = append(edges, ee...)
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From.String() != edges[j].From.String() {
			return edges[i].From.String() < edges[j].From.String()
		}
		return edges[i].To.String() < edges[j].To.String()
	})

	jEdges := make([]jsonEdge, len(edges))
	for i, e := range edges {
		jEdges[i] = jsonEdge{
			From:     e.From,
			To:       e.To,
			Type:     e.Type.String(),
			Resolver: e.Resolver,
			Field:    e.Field,
		}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(jsonGraph{Nodes: nodes, Edges: jEdges})
}
```

**Step 4: Run tests**

Run: `cd /Users/aalpar/projects/ariadne && go test -v -run 'TestExport' ./...`
Expected: PASS

**Step 5: Run full test suite**

Run: `cd /Users/aalpar/projects/ariadne && go test -v ./...`
Expected: ALL PASS

**Step 6: Commit**

```bash
git add export.go export_test.go
git commit -m "Add DOT and JSON graph export"
```

---

## Post-Implementation Checklist

After all tasks are complete:

1. Run full test suite: `go test -v ./...`
2. Run with race detector: `go test -race ./...`
3. Run vet: `go vet ./...`
4. Verify `go mod tidy` produces no changes
5. Review exported API surface: `go doc ./...`
