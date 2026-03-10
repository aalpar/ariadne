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

// stubResolver creates an edge from any Pod to any ConfigMap with the same namespace.
type stubResolver struct{}

func (s *stubResolver) Name() string { return "stub" }

func (s *stubResolver) Extract(_ *unstructured.Unstructured) []Edge { return nil }

func (s *stubResolver) Resolve(obj *unstructured.Unstructured, lookup Lookup) []Edge {
	ref := RefFromUnstructured(obj)
	var edges []Edge

	if ref.Kind == "Pod" {
		for _, cm := range lookup.ListInNamespace("", "ConfigMap", ref.Namespace) {
			cmRef := RefFromUnstructured(cm)
			edges = append(edges, Edge{
				From:     ref,
				To:       cmRef,
				Type:     EdgeRef,
				Resolver: "stub",
				Field:    "test",
			})
		}
	}

	if ref.Kind == "ConfigMap" {
		for _, pod := range lookup.ListInNamespace("", "Pod", ref.Namespace) {
			podRef := RefFromUnstructured(pod)
			edges = append(edges, Edge{
				From:     podRef,
				To:       ref,
				Type:     EdgeRef,
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

	g := New(
		WithResolver(&stubResolver{}),
		WithListener(func(e GraphEvent) {
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

	if len(g.Nodes()) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(g.Nodes()))
	}
	if len(g.Edges()) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(g.Edges()))
	}
}

func TestGet(t *testing.T) {
	g := New()
	pod := newCoreObj("Pod", "default", "nginx")
	g.Add(pod)

	ref := ObjectRef{Kind: "Pod", Namespace: "default", Name: "nginx"}
	obj, ok := g.Get(ref)
	if !ok {
		t.Fatal("expected Get to find the pod")
	}
	if obj.GetName() != "nginx" {
		t.Fatalf("expected name 'nginx', got '%s'", obj.GetName())
	}

	_, ok = g.Get(ObjectRef{Kind: "Pod", Namespace: "default", Name: "missing"})
	if ok {
		t.Fatal("expected Get to return false for missing ref")
	}
}

func makePodWithVolume(ns, name, configMapName string) unstructured.Unstructured {
	return unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]interface{}{
			"name": name, "namespace": ns,
		},
		"spec": map[string]interface{}{
			"volumes": []interface{}{
				map[string]interface{}{
					"name": "vol",
					"configMap": map[string]interface{}{
						"name": configMapName,
					},
				},
			},
		},
	}}
}

func TestAddReAdd(t *testing.T) {
	g := New(WithResolver(NewStructuralResolver()))

	cmA := newCoreObj("ConfigMap", "default", "config-a")
	cmB := newCoreObj("ConfigMap", "default", "config-b")
	pod := makePodWithVolume("default", "web", "config-a")
	g.Add(cmA, cmB, pod)

	podRef := ObjectRef{Kind: "Pod", Namespace: "default", Name: "web"}
	cmARef := ObjectRef{Kind: "ConfigMap", Namespace: "default", Name: "config-a"}
	cmBRef := ObjectRef{Kind: "ConfigMap", Namespace: "default", Name: "config-b"}

	// Pod should depend on config-a, not config-b.
	deps := g.DependenciesOf(podRef)
	if len(deps) != 1 {
		t.Fatalf("expected 1 dependency, got %d: %v", len(deps), deps)
	}
	if deps[0].To != cmARef {
		t.Fatalf("expected dep to config-a, got %v", deps[0].To)
	}

	// Re-add pod referencing config-b instead.
	pod2 := makePodWithVolume("default", "web", "config-b")
	g.Add(pod2)

	deps = g.DependenciesOf(podRef)
	if len(deps) != 1 {
		t.Fatalf("after re-add: expected 1 dependency, got %d: %v", len(deps), deps)
	}
	if deps[0].To != cmBRef {
		t.Fatalf("after re-add: expected dep to config-b, got %v", deps[0].To)
	}

	// config-a should have no dependents.
	if len(g.DependentsOf(cmARef)) != 0 {
		t.Fatal("after re-add: config-a should have no dependents")
	}
}

func TestAddReAddNotifications(t *testing.T) {
	var events []GraphEvent
	g := New(
		WithResolver(NewStructuralResolver()),
		WithListener(func(e GraphEvent) {
			events = append(events, e)
		}),
	)

	cm := newCoreObj("ConfigMap", "default", "config-a")
	pod := makePodWithVolume("default", "web", "config-a")
	g.Add(cm, pod)

	// Clear events from initial add.
	events = nil

	// Re-add the same pod (same spec, same edges).
	pod2 := makePodWithVolume("default", "web", "config-a")
	g.Add(pod2)

	// Should see EdgeRemoved + EdgeAdded (old edges cleared, same edges re-resolved).
	// Should NOT see NodeAdded (node already existed).
	for _, e := range events {
		if e.Type == NodeAdded {
			t.Fatal("re-add should not fire NodeAdded")
		}
	}

	edgeRemoved := 0
	edgeAdded := 0
	for _, e := range events {
		switch e.Type {
		case EdgeRemoved:
			edgeRemoved++
		case EdgeAdded:
			edgeAdded++
		}
	}
	if edgeRemoved == 0 {
		t.Fatal("re-add should fire EdgeRemoved for old edges")
	}
	if edgeAdded == 0 {
		t.Fatal("re-add should fire EdgeAdded for re-resolved edges")
	}
}

// chainResolver creates edges: A -> B -> C (by name convention "a"->"b"->"c")
type chainResolver struct{}

func (c *chainResolver) Name() string { return "chain" }

func (c *chainResolver) Extract(_ *unstructured.Unstructured) []Edge { return nil }

func (c *chainResolver) Resolve(obj *unstructured.Unstructured, lookup Lookup) []Edge {
	ref := RefFromUnstructured(obj)
	var edges []Edge

	chain := map[string]string{"a": "b", "b": "c"}
	if target, ok := chain[ref.Name]; ok {
		toRef := ObjectRef{Kind: ref.Kind, Namespace: ref.Namespace, Name: target}
		if _, exists := lookup.Get(toRef); exists {
			edges = append(edges, Edge{
				From: ref, To: toRef,
				Type: EdgeRef, Resolver: "chain", Field: "test",
			})
		}
	}

	reverse := map[string]string{"b": "a", "c": "b"}
	if source, ok := reverse[ref.Name]; ok {
		fromRef := ObjectRef{Kind: ref.Kind, Namespace: ref.Namespace, Name: source}
		if _, exists := lookup.Get(fromRef); exists {
			edges = append(edges, Edge{
				From: fromRef, To: ref,
				Type: EdgeRef, Resolver: "chain", Field: "test",
			})
		}
	}

	return edges
}

func TestUpstream(t *testing.T) {
	// Chain: a depends on b, b depends on c (edges: a->b, b->c)
	// Upstream(a) = transitive dependencies of a = {b, c}
	g := New(WithResolver(&chainResolver{}))
	g.Load([]unstructured.Unstructured{
		newCoreObj("Pod", "default", "a"),
		newCoreObj("Pod", "default", "b"),
		newCoreObj("Pod", "default", "c"),
	})

	upstream := g.Upstream(ObjectRef{Kind: "Pod", Namespace: "default", Name: "a"})
	if len(upstream) != 2 {
		t.Fatalf("expected 2 upstream nodes, got %d: %v", len(upstream), upstream)
	}
}

func TestDownstream(t *testing.T) {
	// Chain: a depends on b, b depends on c (edges: a->b, b->c)
	// Downstream(c) = transitive dependents of c = {b, a}
	g := New(WithResolver(&chainResolver{}))
	g.Load([]unstructured.Unstructured{
		newCoreObj("Pod", "default", "a"),
		newCoreObj("Pod", "default", "b"),
		newCoreObj("Pod", "default", "c"),
	})

	downstream := g.Downstream(ObjectRef{Kind: "Pod", Namespace: "default", Name: "c"})
	if len(downstream) != 2 {
		t.Fatalf("expected 2 downstream nodes, got %d: %v", len(downstream), downstream)
	}
}
