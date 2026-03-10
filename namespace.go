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

// NewNamespaceResolver returns a resolver that creates edges from every
// namespaced object to its Namespace. Cluster-scoped objects and Namespace
// objects themselves are skipped.
func NewNamespaceResolver() Resolver {
	return &namespaceResolver{}
}

// WithNamespaceDeps enables namespace dependency edges. Every namespaced
// object will have an edge to its Namespace object. This is opt-in because
// it is noisy — every namespaced object gains an extra edge.
func WithNamespaceDeps() Option {
	return WithResolver(NewNamespaceResolver())
}

type namespaceResolver struct{}

func (r *namespaceResolver) Name() string { return "namespace" }

func (r *namespaceResolver) Extract(obj *unstructured.Unstructured) []Edge {
	ref := RefFromUnstructured(obj)
	if ref.Namespace == "" || ref.Kind == "Namespace" {
		return nil
	}
	return []Edge{{
		From:     ref,
		To:       ObjectRef{Kind: "Namespace", Name: ref.Namespace},
		Type:     EdgeRef,
		Resolver: "namespace",
		Field:    "metadata.namespace",
	}}
}

func (r *namespaceResolver) Resolve(obj *unstructured.Unstructured, lookup Lookup) []Edge {
	ref := RefFromUnstructured(obj)
	if ref.Kind == "Namespace" {
		return r.resolveNamespace(ref, lookup)
	}
	return r.resolveNamespaced(ref, lookup)
}

// resolveNamespaced handles a namespaced object being added:
// emit an edge to its Namespace if the Namespace exists in the graph.
func (r *namespaceResolver) resolveNamespaced(ref ObjectRef, lookup Lookup) []Edge {
	if ref.Namespace == "" {
		return nil
	}
	nsRef := ObjectRef{Kind: "Namespace", Name: ref.Namespace}
	if _, ok := lookup.Get(nsRef); !ok {
		return nil
	}
	return []Edge{{
		From:     ref,
		To:       nsRef,
		Type:     EdgeRef,
		Resolver: "namespace",
		Field:    "metadata.namespace",
	}}
}

// resolveNamespace handles a Namespace being added:
// find all objects in that namespace and emit edges from each to this Namespace.
func (r *namespaceResolver) resolveNamespace(ref ObjectRef, lookup Lookup) []Edge {
	objs := lookup.ListByNamespace(ref.Name)
	var edges []Edge
	for _, obj := range objs {
		objRef := RefFromUnstructured(obj)
		if objRef.Kind == "Namespace" {
			continue
		}
		edges = append(edges, Edge{
			From:     objRef,
			To:       ref,
			Type:     EdgeRef,
			Resolver: "namespace",
			Field:    "metadata.namespace",
		})
	}
	return edges
}
