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
	ListAll() []*unstructured.Unstructured
}

// ResolveAll runs resolvers against all objects and returns every potential
// edge, including edges to targets not in the input set. This is useful for
// detecting dangling references — edges whose target is absent.
//
// Unlike Graph.Load, which only emits edges between objects that both exist,
// ResolveAll uses a permissive lookup that causes resolvers to emit forward
// edges unconditionally. Reverse resolution and label-selector matching use
// real data from the input set.
//
// The returned edges are deduplicated.
func ResolveAll(objs []unstructured.Unstructured, resolvers ...Resolver) []Edge {
	ptrs := make([]*unstructured.Unstructured, len(objs))
	for i := range objs {
		ptrs[i] = &objs[i]
	}
	lookup := &permissiveLookup{ptrs: ptrs}

	seen := make(map[Edge]struct{})
	var edges []Edge
	for i := range objs {
		for _, r := range resolvers {
			for _, e := range r.Resolve(&objs[i], lookup) {
				if _, dup := seen[e]; dup {
					continue
				}
				seen[e] = struct{}{}
				edges = append(edges, e)
			}
		}
	}
	return edges
}

// permissiveLookup implements Lookup with a Get that always returns true.
// This causes forward-resolving rules to emit edges even when the target
// doesn't exist. List methods return real data so reverse resolution and
// label-selector matching work correctly.
type permissiveLookup struct {
	ptrs []*unstructured.Unstructured
}

func (l *permissiveLookup) Get(ref ObjectRef) (*unstructured.Unstructured, bool) {
	return nil, true
}

func (l *permissiveLookup) List(group, kind string) []*unstructured.Unstructured {
	var result []*unstructured.Unstructured
	for _, obj := range l.ptrs {
		ref := RefFromUnstructured(obj)
		if ref.Group == group && ref.Kind == kind {
			result = append(result, obj)
		}
	}
	return result
}

func (l *permissiveLookup) ListInNamespace(group, kind, namespace string) []*unstructured.Unstructured {
	var result []*unstructured.Unstructured
	for _, obj := range l.ptrs {
		ref := RefFromUnstructured(obj)
		if ref.Group == group && ref.Kind == kind && ref.Namespace == namespace {
			result = append(result, obj)
		}
	}
	return result
}

func (l *permissiveLookup) ListAll() []*unstructured.Unstructured {
	return l.ptrs
}
