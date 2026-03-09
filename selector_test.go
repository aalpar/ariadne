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

func TestSelectorResolver_MatchExpressions(t *testing.T) {
	r := NewRuleResolver("test", LabelSelectorRule{
		FromGroup: "policy", FromKind: "PodDisruptionBudget",
		ToKind:            "Pod",
		SelectorFieldPath: "spec.selector",
	})

	pdb := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "policy/v1", "kind": "PodDisruptionBudget",
		"metadata": map[string]interface{}{
			"name": "web-pdb", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"selector": map[string]interface{}{
				"matchLabels": map[string]interface{}{
					"app": "web",
				},
				"matchExpressions": []interface{}{
					map[string]interface{}{
						"key":      "tier",
						"operator": "In",
						"values":   []interface{}{"frontend", "backend"},
					},
				},
			},
		},
	}}

	frontendPod := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]interface{}{
			"name": "web-fe", "namespace": "default",
			"labels": map[string]interface{}{
				"app":  "web",
				"tier": "frontend",
			},
		},
	}}

	// Matches matchLabels but not matchExpressions
	noTierPod := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]interface{}{
			"name": "web-notier", "namespace": "default",
			"labels": map[string]interface{}{
				"app": "web",
			},
		},
	}}

	// Wrong app entirely
	dbPod := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]interface{}{
			"name": "db-1", "namespace": "default",
			"labels": map[string]interface{}{
				"app":  "db",
				"tier": "frontend",
			},
		},
	}}

	lookup := &stubLookup{
		objects: map[ObjectRef]*unstructured.Unstructured{
			{Kind: "Pod", Namespace: "default", Name: "web-fe"}:     frontendPod,
			{Kind: "Pod", Namespace: "default", Name: "web-notier"}: noTierPod,
			{Kind: "Pod", Namespace: "default", Name: "db-1"}:       dbPod,
		},
	}

	edges := r.Resolve(pdb, lookup)
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge (only frontend pod matches both matchLabels and matchExpressions), got %d: %v", len(edges), edges)
	}
	if edges[0].To.Name != "web-fe" {
		t.Fatalf("expected edge to web-fe, got %s", edges[0].To.Name)
	}
}

func TestSelectorResolver_MatchExpressionsReverse(t *testing.T) {
	r := NewRuleResolver("test", LabelSelectorRule{
		FromGroup: "policy", FromKind: "PodDisruptionBudget",
		ToKind:            "Pod",
		SelectorFieldPath: "spec.selector",
	})

	// A matching pod is being added; the PDB already exists in the graph.
	pod := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]interface{}{
			"name": "web-be", "namespace": "default",
			"labels": map[string]interface{}{
				"app":  "web",
				"tier": "backend",
			},
		},
	}}

	pdb := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "policy/v1", "kind": "PodDisruptionBudget",
		"metadata": map[string]interface{}{
			"name": "web-pdb", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"selector": map[string]interface{}{
				"matchExpressions": []interface{}{
					map[string]interface{}{
						"key":      "tier",
						"operator": "In",
						"values":   []interface{}{"frontend", "backend"},
					},
				},
				"matchLabels": map[string]interface{}{
					"app": "web",
				},
			},
		},
	}}

	lookup := &stubLookup{
		objects: map[ObjectRef]*unstructured.Unstructured{
			{Group: "policy", Kind: "PodDisruptionBudget", Namespace: "default", Name: "web-pdb"}: pdb,
		},
	}

	edges := r.Resolve(pod, lookup)
	if len(edges) != 1 {
		t.Fatalf("expected 1 reverse edge, got %d: %v", len(edges), edges)
	}
	if edges[0].From.Kind != "PodDisruptionBudget" {
		t.Fatalf("expected edge from PDB, got %v", edges[0].From)
	}
}
