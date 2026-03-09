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
		// Pod (references SA, ConfigMap, Secret via volumes; owned by ReplicaSet)
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
		t.Fatalf("expected 1 svc dep, got %d: %v", len(svcDeps), svcDeps)
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

	// Total edges: Pod->SA, Pod->CM, Pod->Secret, Pod->RS(owner), Svc->Pod = 5
	edges := g.Edges()
	if len(edges) != 5 {
		t.Fatalf("expected 5 edges, got %d", len(edges))
	}

	// Service's Upstream should include Pod and all Pod's deps (SA, CM, Secret, RS)
	upstream := g.Upstream(svcRef)
	if len(upstream) < 4 {
		t.Fatalf("expected at least 4 upstream of svc, got %d: %v", len(upstream), upstream)
	}
}
