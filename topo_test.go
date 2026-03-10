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
)

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
				Type: EdgeRef, Resolver: "cycle", Field: "test",
			})
		}
	}
	return edges
}

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

	// Edges: a->b, b->c (a depends on b, b depends on c)
	// Dependency order (deps first): c, b, a
	pos := map[ObjectRef]int{}
	for i, ref := range sorted {
		pos[ref] = i
	}
	a := ObjectRef{Kind: "Pod", Namespace: "default", Name: "a"}
	b := ObjectRef{Kind: "Pod", Namespace: "default", Name: "b"}
	c := ObjectRef{Kind: "Pod", Namespace: "default", Name: "c"}

	if pos[c] > pos[b] || pos[b] > pos[a] {
		t.Fatalf("wrong order: c=%d b=%d a=%d", pos[c], pos[b], pos[a])
	}
}

func TestTopologicalSortCycleError(t *testing.T) {
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
