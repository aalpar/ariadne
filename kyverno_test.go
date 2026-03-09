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

func TestKyverno_ClusterPolicyForward(t *testing.T) {
	g := New(WithResolver(NewKyvernoResolver()))

	policy := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "kyverno.io/v1", "kind": "ClusterPolicy",
		"metadata": map[string]interface{}{
			"name": "require-labels",
		},
		"spec": map[string]interface{}{
			"rules": []interface{}{
				map[string]interface{}{
					"match": map[string]interface{}{
						"resources": map[string]interface{}{
							"kinds": []interface{}{"Pod"},
						},
					},
				},
			},
		},
	}}

	pod1 := newCoreObj("Pod", "default", "web")
	pod2 := newCoreObj("Pod", "kube-system", "dns")

	g.Load([]unstructured.Unstructured{policy, pod1, pod2})

	policyRef := ObjectRef{Group: "kyverno.io", Kind: "ClusterPolicy", Name: "require-labels"}
	deps := g.DependenciesOf(policyRef)
	if len(deps) != 2 {
		t.Fatalf("expected 2 edges from ClusterPolicy, got %d", len(deps))
	}
	for _, e := range deps {
		if e.Type != EdgeCustom {
			t.Errorf("expected EdgeCustom, got %s", e.Type)
		}
		if e.Resolver != "kyverno" {
			t.Errorf("expected resolver 'kyverno', got '%s'", e.Resolver)
		}
		if e.To.Kind != "Pod" {
			t.Errorf("expected target kind Pod, got %s", e.To.Kind)
		}
	}
}

func TestKyverno_PolicyNamespaceScoped(t *testing.T) {
	g := New(WithResolver(NewKyvernoResolver()))

	policy := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "kyverno.io/v1", "kind": "Policy",
		"metadata": map[string]interface{}{
			"name": "require-labels", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"rules": []interface{}{
				map[string]interface{}{
					"match": map[string]interface{}{
						"resources": map[string]interface{}{
							"kinds": []interface{}{"Pod"},
						},
					},
				},
			},
		},
	}}

	podSameNS := newCoreObj("Pod", "default", "web")
	podOtherNS := newCoreObj("Pod", "other", "api")

	g.Load([]unstructured.Unstructured{policy, podSameNS, podOtherNS})

	policyRef := ObjectRef{Group: "kyverno.io", Kind: "Policy", Namespace: "default", Name: "require-labels"}
	deps := g.DependenciesOf(policyRef)
	if len(deps) != 1 {
		t.Fatalf("expected 1 edge from namespaced Policy, got %d", len(deps))
	}
	if deps[0].To.Namespace != "default" {
		t.Errorf("expected target in 'default' namespace, got '%s'", deps[0].To.Namespace)
	}
}

func TestKyverno_ReverseAdd(t *testing.T) {
	g := New(WithResolver(NewKyvernoResolver()))

	policy := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "kyverno.io/v1", "kind": "ClusterPolicy",
		"metadata": map[string]interface{}{
			"name": "require-labels",
		},
		"spec": map[string]interface{}{
			"rules": []interface{}{
				map[string]interface{}{
					"match": map[string]interface{}{
						"resources": map[string]interface{}{
							"kinds": []interface{}{"Pod"},
						},
					},
				},
			},
		},
	}}

	pod := newCoreObj("Pod", "default", "web")

	// Add policy first, then pod — reverse resolution should create the edge.
	g.Add(policy)
	g.Add(pod)

	podRef := ObjectRef{Kind: "Pod", Namespace: "default", Name: "web"}
	incoming := g.DependentsOf(podRef)
	if len(incoming) != 1 {
		t.Fatalf("expected 1 incoming edge to Pod from reverse resolution, got %d", len(incoming))
	}
	if incoming[0].From.Kind != "ClusterPolicy" {
		t.Errorf("expected edge from ClusterPolicy, got from %s", incoming[0].From.Kind)
	}
	if incoming[0].Type != EdgeCustom {
		t.Errorf("expected EdgeCustom, got %s", incoming[0].Type)
	}
}
