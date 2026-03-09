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

func TestExtractFieldValues_Simple(t *testing.T) {
	obj := map[string]interface{}{
		"spec": map[string]interface{}{
			"volumeName": "my-pv",
		},
	}
	vals := extractFieldValues(obj, "spec.volumeName")
	if len(vals) != 1 || vals[0] != "my-pv" {
		t.Fatalf("expected [my-pv], got %v", vals)
	}
}

func TestExtractFieldValues_Wildcard(t *testing.T) {
	obj := map[string]interface{}{
		"spec": map[string]interface{}{
			"volumes": []interface{}{
				map[string]interface{}{
					"configMap": map[string]interface{}{
						"name": "cm-a",
					},
				},
				map[string]interface{}{
					"secret": map[string]interface{}{
						"secretName": "sec-b",
					},
				},
				map[string]interface{}{
					"configMap": map[string]interface{}{
						"name": "cm-c",
					},
				},
			},
		},
	}

	vals := extractFieldValues(obj, "spec.volumes[*].configMap.name")
	if len(vals) != 2 {
		t.Fatalf("expected 2 values, got %d: %v", len(vals), vals)
	}
}

func TestExtractFieldValues_NestedWildcard(t *testing.T) {
	obj := map[string]interface{}{
		"spec": map[string]interface{}{
			"containers": []interface{}{
				map[string]interface{}{
					"envFrom": []interface{}{
						map[string]interface{}{
							"configMapRef": map[string]interface{}{
								"name": "env-cm",
							},
						},
					},
				},
			},
		},
	}

	vals := extractFieldValues(obj, "spec.containers[*].envFrom[*].configMapRef.name")
	if len(vals) != 1 || vals[0] != "env-cm" {
		t.Fatalf("expected [env-cm], got %v", vals)
	}
}

func TestExtractFieldValues_Missing(t *testing.T) {
	obj := map[string]interface{}{
		"spec": map[string]interface{}{},
	}
	vals := extractFieldValues(obj, "spec.volumeName")
	if len(vals) != 0 {
		t.Fatalf("expected empty, got %v", vals)
	}
}

func TestNameRefRule(t *testing.T) {
	r := NewRuleResolver("test", NameRefRule{
		FromGroup: "", FromKind: "PersistentVolumeClaim",
		ToGroup: "", ToKind: "PersistentVolume",
		FieldPath:     "spec.volumeName",
		SameNamespace: false,
	})

	pvc := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "PersistentVolumeClaim",
		"metadata": map[string]interface{}{
			"name":      "my-pvc",
			"namespace": "default",
		},
		"spec": map[string]interface{}{
			"volumeName": "my-pv",
		},
	}}

	pv := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "PersistentVolume",
		"metadata": map[string]interface{}{
			"name": "my-pv",
		},
	}}

	lookup := &stubLookup{
		objects: map[ObjectRef]*unstructured.Unstructured{
			{Kind: "PersistentVolume", Name: "my-pv"}: pv,
		},
	}

	edges := r.Resolve(pvc, lookup)
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if edges[0].To.Kind != "PersistentVolume" || edges[0].To.Name != "my-pv" {
		t.Fatalf("unexpected edge target: %v", edges[0].To)
	}
}

func TestLabelSelectorRule(t *testing.T) {
	r := NewRuleResolver("test", LabelSelectorRule{
		FromGroup: "", FromKind: "Service",
		ToGroup: "", ToKind: "Pod",
		SelectorFieldPath: "spec.selector",
	})

	svc := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata": map[string]interface{}{
			"name":      "my-svc",
			"namespace": "default",
		},
		"spec": map[string]interface{}{
			"selector": map[string]interface{}{
				"app": "nginx",
			},
		},
	}}

	matchingPod := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":      "nginx-abc",
			"namespace": "default",
			"labels": map[string]interface{}{
				"app": "nginx",
			},
		},
	}}

	nonMatchingPod := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":      "redis-xyz",
			"namespace": "default",
			"labels": map[string]interface{}{
				"app": "redis",
			},
		},
	}}

	lookup := &stubLookup{
		objects: map[ObjectRef]*unstructured.Unstructured{
			{Kind: "Pod", Namespace: "default", Name: "nginx-abc"}: matchingPod,
			{Kind: "Pod", Namespace: "default", Name: "redis-xyz"}: nonMatchingPod,
		},
	}

	edges := r.Resolve(svc, lookup)
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge (to matching pod), got %d", len(edges))
	}
	if edges[0].To.Name != "nginx-abc" {
		t.Fatalf("expected edge to nginx-abc, got %v", edges[0].To.Name)
	}
}

// stubLookup is a simple Lookup implementation for unit tests.
type stubLookup struct {
	objects map[ObjectRef]*unstructured.Unstructured
}

func (s *stubLookup) Get(ref ObjectRef) (*unstructured.Unstructured, bool) {
	obj, ok := s.objects[ref]
	return obj, ok
}

func (s *stubLookup) List(group, kind string) []*unstructured.Unstructured {
	var result []*unstructured.Unstructured
	for ref, obj := range s.objects {
		if ref.Group == group && ref.Kind == kind {
			result = append(result, obj)
		}
	}
	return result
}

func (s *stubLookup) ListInNamespace(group, kind, namespace string) []*unstructured.Unstructured {
	var result []*unstructured.Unstructured
	for ref, obj := range s.objects {
		if ref.Group == group && ref.Kind == kind && ref.Namespace == namespace {
			result = append(result, obj)
		}
	}
	return result
}

func (s *stubLookup) ListAll() []*unstructured.Unstructured {
	result := make([]*unstructured.Unstructured, 0, len(s.objects))
	for _, obj := range s.objects {
		result = append(result, obj)
	}
	return result
}
