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

func TestNamespaceResolver_Extract(t *testing.T) {
	r := NewNamespaceResolver()

	pod := newCoreObj("Pod", "default", "web")
	edges := r.Extract(&pod)
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	e := edges[0]
	if e.From.Kind != "Pod" || e.From.Name != "web" {
		t.Fatalf("unexpected From: %v", e.From)
	}
	if e.To != (ObjectRef{Kind: "Namespace", Name: "default"}) {
		t.Fatalf("unexpected To: %v", e.To)
	}
	if e.Type != EdgeRef {
		t.Fatalf("expected EdgeRef, got %v", e.Type)
	}
	if e.Resolver != "namespace" {
		t.Fatalf("expected resolver 'namespace', got %q", e.Resolver)
	}
	if e.Field != "metadata.namespace" {
		t.Fatalf("expected field 'metadata.namespace', got %q", e.Field)
	}
}

func TestNamespaceResolver_Extract_ClusterScoped(t *testing.T) {
	r := NewNamespaceResolver()

	node := newCoreObj("Node", "", "worker-1")
	edges := r.Extract(&node)
	if len(edges) != 0 {
		t.Fatalf("expected 0 edges for cluster-scoped object, got %d", len(edges))
	}
}

func TestNamespaceResolver_Extract_SkipsNamespace(t *testing.T) {
	r := NewNamespaceResolver()

	ns := newCoreObj("Namespace", "", "default")
	edges := r.Extract(&ns)
	if len(edges) != 0 {
		t.Fatalf("expected 0 edges for Namespace object, got %d", len(edges))
	}
}

func TestNamespaceResolver_Resolve_Forward(t *testing.T) {
	g := New(WithResolver(NewNamespaceResolver()))

	ns := newCoreObj("Namespace", "", "default")
	pod := newCoreObj("Pod", "default", "web")
	g.Load([]unstructured.Unstructured{ns, pod})

	podRef := ObjectRef{Kind: "Pod", Namespace: "default", Name: "web"}
	deps := g.DependenciesOf(podRef)
	if len(deps) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(deps))
	}
	if deps[0].To != (ObjectRef{Kind: "Namespace", Name: "default"}) {
		t.Fatalf("unexpected dependency target: %v", deps[0].To)
	}
}

func TestNamespaceResolver_Resolve_Reverse(t *testing.T) {
	// When a Namespace is added, existing namespaced objects should get edges.
	g := New(WithResolver(NewNamespaceResolver()))

	pod := newCoreObj("Pod", "default", "web")
	svc := newCoreObj("Service", "default", "api")
	g.Add(pod, svc)

	// Neither should have namespace edges yet (no Namespace object in graph).
	podRef := ObjectRef{Kind: "Pod", Namespace: "default", Name: "web"}
	svcRef := ObjectRef{Kind: "Service", Namespace: "default", Name: "api"}
	if len(g.DependenciesOf(podRef)) != 0 {
		t.Fatalf("expected no deps before namespace added")
	}

	// Now add the Namespace — reverse resolution should create edges.
	ns := newCoreObj("Namespace", "", "default")
	g.Add(ns)

	nsRef := ObjectRef{Kind: "Namespace", Name: "default"}
	dependents := g.DependentsOf(nsRef)
	if len(dependents) != 2 {
		t.Fatalf("expected 2 dependents of namespace, got %d", len(dependents))
	}

	// Both pod and svc should now depend on the namespace.
	refs := map[ObjectRef]bool{}
	for _, e := range dependents {
		refs[e.From] = true
	}
	if !refs[podRef] || !refs[svcRef] {
		t.Fatalf("expected pod and svc as dependents, got %v", refs)
	}
}

func TestNamespaceResolver_Resolve_NoNamespaceInGraph(t *testing.T) {
	// If the Namespace object isn't in the graph, no forward edge.
	g := New(WithResolver(NewNamespaceResolver()))

	pod := newCoreObj("Pod", "default", "web")
	g.Add(pod)

	podRef := ObjectRef{Kind: "Pod", Namespace: "default", Name: "web"}
	deps := g.DependenciesOf(podRef)
	if len(deps) != 0 {
		t.Fatalf("expected 0 deps when namespace not in graph, got %d", len(deps))
	}
}

func TestNamespaceResolver_Resolve_CrossNamespace(t *testing.T) {
	// Objects in different namespaces point to different Namespace objects.
	g := New(WithResolver(NewNamespaceResolver()))

	ns1 := newCoreObj("Namespace", "", "alpha")
	ns2 := newCoreObj("Namespace", "", "beta")
	podA := newCoreObj("Pod", "alpha", "a")
	podB := newCoreObj("Pod", "beta", "b")
	g.Load([]unstructured.Unstructured{ns1, ns2, podA, podB})

	alphaRef := ObjectRef{Kind: "Namespace", Name: "alpha"}
	betaRef := ObjectRef{Kind: "Namespace", Name: "beta"}

	alphaDeps := g.DependentsOf(alphaRef)
	if len(alphaDeps) != 1 || alphaDeps[0].From.Name != "a" {
		t.Fatalf("expected pod 'a' as dependent of alpha, got %v", alphaDeps)
	}
	betaDeps := g.DependentsOf(betaRef)
	if len(betaDeps) != 1 || betaDeps[0].From.Name != "b" {
		t.Fatalf("expected pod 'b' as dependent of beta, got %v", betaDeps)
	}
}

func TestWithNamespaceDeps(t *testing.T) {
	// WithNamespaceDeps is sugar for WithResolver(NewNamespaceResolver()).
	g := NewDefault(WithNamespaceDeps())

	ns := newCoreObj("Namespace", "", "default")
	pod := newCoreObj("Pod", "default", "web")
	g.Load([]unstructured.Unstructured{ns, pod})

	podRef := ObjectRef{Kind: "Pod", Namespace: "default", Name: "web"}
	deps := g.DependenciesOf(podRef)

	hasNsDep := false
	for _, e := range deps {
		if e.Resolver == "namespace" {
			hasNsDep = true
			break
		}
	}
	if !hasNsDep {
		t.Fatalf("expected namespace dependency edge, deps: %v", deps)
	}
}
