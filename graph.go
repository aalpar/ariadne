// Copyright 2026 The Ariadne Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

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

// NewDefault creates a Graph with all built-in resolvers registered.
func NewDefault(opts ...Option) *Graph {
	defaults := []Option{
		WithResolver(NewStructuralResolver()),
		WithResolver(NewSelectorResolver()),
		WithResolver(NewEventResolver()),
	}
	return New(append(defaults, opts...)...)
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

// removeEdgeFromSlice removes the first occurrence of e from the slice at edges[key].
// It cleans up the map entry if the slice becomes empty.
func removeEdgeFromSlice(edges map[ObjectRef][]Edge, key ObjectRef, e Edge) {
	s := edges[key]
	for i, existing := range s {
		if existing == e {
			edges[key] = append(s[:i], s[i+1:]...)
			if len(edges[key]) == 0 {
				delete(edges, key)
			}
			return
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

		// Snapshot outgoing edges before mutation, then clean up the other side.
		for _, e := range append([]Edge(nil), g.outEdges[ref]...) {
			removeEdgeFromSlice(g.inEdges, e.To, e)
			g.notify(GraphEvent{Type: EdgeRemoved, Edge: &e})
		}

		// Snapshot incoming edges before mutation, then clean up the other side.
		for _, e := range append([]Edge(nil), g.inEdges[ref]...) {
			removeEdgeFromSlice(g.outEdges, e.From, e)
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

// Upstream returns the transitive closure of DependenciesOf (all ancestors).
// Edges mean "From depends on To", so Upstream follows outEdges.
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
// Edges mean "From depends on To", so Downstream follows inEdges.
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
