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

func TestStructuralResolver_OwnerRefReverse(t *testing.T) {
	g := New(WithResolver(NewStructuralResolver()))

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

	rs := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1", "kind": "ReplicaSet",
		"metadata": map[string]interface{}{
			"name": "web-rs", "namespace": "default",
		},
	}}

	// Add child first, then owner — the reverse path must discover the edge.
	g.Add(pod)
	g.Add(rs)

	podRef := ObjectRef{Kind: "Pod", Namespace: "default", Name: "web-pod"}
	deps := g.DependenciesOf(podRef)
	if len(deps) != 1 {
		t.Fatalf("expected 1 owner dep, got %d", len(deps))
	}
	if deps[0].To.Kind != "ReplicaSet" || deps[0].To.Name != "web-rs" {
		t.Fatalf("unexpected owner: %v", deps[0].To)
	}

	// Verify from the owner's perspective too.
	rsRef := ObjectRef{Group: "apps", Kind: "ReplicaSet", Namespace: "default", Name: "web-rs"}
	dependents := g.DependentsOf(rsRef)
	if len(dependents) != 1 {
		t.Fatalf("expected 1 dependent, got %d", len(dependents))
	}
	if dependents[0].From.Kind != "Pod" || dependents[0].From.Name != "web-pod" {
		t.Fatalf("unexpected dependent: %v", dependents[0].From)
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
