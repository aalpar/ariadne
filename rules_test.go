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

func TestRefRule_ExistingBehavior(t *testing.T) {
	r := NewRuleResolver("test", RefRule{
		FromGroup: "", FromKind: "PersistentVolumeClaim",
		ToGroup: "", ToKind: "PersistentVolume",
		FieldPath: "spec.volumeName",
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

func TestRefRule_SameNamespace(t *testing.T) {
	r := NewRuleResolver("test", RefRule{
		FromKind: "Pod", ToKind: "ConfigMap",
		FieldPath: "spec.volumes[*].configMap.name",
	})

	pod := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]interface{}{
			"name": "web", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"volumes": []interface{}{
				map[string]interface{}{
					"configMap": map[string]interface{}{"name": "app-config"},
				},
			},
		},
	}}

	cm := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "ConfigMap",
		"metadata": map[string]interface{}{
			"name": "app-config", "namespace": "default",
		},
	}}

	lookup := &stubLookup{
		objects: map[ObjectRef]*unstructured.Unstructured{
			{Kind: "ConfigMap", Namespace: "default", Name: "app-config"}: cm,
		},
	}

	edges := r.Resolve(pod, lookup)
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if edges[0].To.Kind != "ConfigMap" || edges[0].To.Name != "app-config" {
		t.Fatalf("unexpected target: %v", edges[0].To)
	}
	if edges[0].Type != EdgeRef {
		t.Fatalf("expected EdgeRef, got %v", edges[0].Type)
	}
}

func TestRefRule_ClusterScoped(t *testing.T) {
	r := NewRuleResolver("test", RefRule{
		FromKind: "PersistentVolumeClaim", ToKind: "PersistentVolume",
		FieldPath: "spec.volumeName",
	})

	pvc := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "PersistentVolumeClaim",
		"metadata": map[string]interface{}{
			"name": "my-pvc", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"volumeName": "my-pv",
		},
	}}

	pv := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "PersistentVolume",
		"metadata": map[string]interface{}{"name": "my-pv"},
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
	if edges[0].To.Namespace != "" {
		t.Fatalf("expected cluster-scoped target, got ns=%q", edges[0].To.Namespace)
	}
	if edges[0].Type != EdgeRef {
		t.Fatalf("expected EdgeRef for cluster-scoped, got %v", edges[0].Type)
	}
}

func TestRefRule_Reverse(t *testing.T) {
	rule := RefRule{
		FromKind: "Pod", ToKind: "ConfigMap",
		FieldPath: "spec.volumes[*].configMap.name",
	}
	r := NewRuleResolver("test", rule)

	// The ConfigMap is the object being added (target).
	cm := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "ConfigMap",
		"metadata": map[string]interface{}{
			"name": "app-config", "namespace": "default",
		},
	}}

	// A Pod that references it already exists in the graph.
	pod := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]interface{}{
			"name": "web", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"volumes": []interface{}{
				map[string]interface{}{
					"configMap": map[string]interface{}{"name": "app-config"},
				},
			},
		},
	}}

	lookup := &stubLookup{
		objects: map[ObjectRef]*unstructured.Unstructured{
			{Kind: "Pod", Namespace: "default", Name: "web"}: pod,
		},
	}

	edges := r.Resolve(cm, lookup)
	if len(edges) != 1 {
		t.Fatalf("expected 1 reverse edge, got %d", len(edges))
	}
	if edges[0].From.Kind != "Pod" || edges[0].From.Name != "web" {
		t.Fatalf("expected edge from Pod/web, got %v", edges[0].From)
	}
	if edges[0].To.Kind != "ConfigMap" || edges[0].To.Name != "app-config" {
		t.Fatalf("expected edge to ConfigMap/app-config, got %v", edges[0].To)
	}
	if edges[0].Type != EdgeRef {
		t.Fatalf("expected EdgeRef, got %v", edges[0].Type)
	}
}

func TestRefRule_ReverseWithNamespace(t *testing.T) {
	rule := RefRule{
		FromGroup: "example.com", FromKind: "MyResource",
		ToKind:             "Service",
		FieldPath:          "spec.backendRef.name",
		NamespaceFieldPath: "spec.backendRef.namespace",
	}
	r := NewRuleResolver("test", rule)

	// The Service is the object being added.
	svc := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{
			"name": "backend", "namespace": "prod",
		},
	}}

	// A MyResource in a different namespace references it via explicit namespace.
	myRes := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "example.com/v1", "kind": "MyResource",
		"metadata": map[string]interface{}{
			"name": "my-res", "namespace": "staging",
		},
		"spec": map[string]interface{}{
			"backendRef": map[string]interface{}{
				"name":      "backend",
				"namespace": "prod",
			},
		},
	}}

	lookup := &stubLookup{
		objects: map[ObjectRef]*unstructured.Unstructured{
			{Group: "example.com", Kind: "MyResource", Namespace: "staging", Name: "my-res"}: myRes,
		},
	}

	edges := r.Resolve(svc, lookup)
	if len(edges) != 1 {
		t.Fatalf("expected 1 reverse edge, got %d", len(edges))
	}
	if edges[0].From.Name != "my-res" || edges[0].To.Name != "backend" {
		t.Fatalf("unexpected edge: %v -> %v", edges[0].From, edges[0].To)
	}
}

func TestRefRule_TypedRef(t *testing.T) {
	r := NewRuleResolver("test", RefRule{
		FromGroup: "autoscaling", FromKind: "HorizontalPodAutoscaler",
		FieldPath: "spec.scaleTargetRef",
	})

	hpa := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "autoscaling/v2", "kind": "HorizontalPodAutoscaler",
		"metadata": map[string]interface{}{
			"name": "web-hpa", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"scaleTargetRef": map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"name":       "web",
			},
		},
	}}

	deploy := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]interface{}{
			"name": "web", "namespace": "default",
		},
	}}

	lookup := &stubLookup{
		objects: map[ObjectRef]*unstructured.Unstructured{
			{Group: "apps", Kind: "Deployment", Namespace: "default", Name: "web"}: deploy,
		},
	}

	edges := r.Resolve(hpa, lookup)
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if edges[0].To.Group != "apps" || edges[0].To.Kind != "Deployment" || edges[0].To.Name != "web" {
		t.Fatalf("unexpected target: %v", edges[0].To)
	}
	if edges[0].Type != EdgeRef {
		t.Fatalf("expected EdgeRef, got %v", edges[0].Type)
	}
}

func TestRefRule_TypedRefWithConstraint(t *testing.T) {
	r := NewRuleResolver("test", RefRule{
		FromGroup: "rbac.authorization.k8s.io", FromKind: "RoleBinding",
		ToGroup:   "rbac.authorization.k8s.io",
		FieldPath: "spec.roleRef",
	})

	rb := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "RoleBinding",
		"metadata": map[string]interface{}{
			"name": "admin-binding", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"roleRef": map[string]interface{}{
				"apiGroup": "rbac.authorization.k8s.io",
				"kind":     "Role",
				"name":     "admin",
			},
		},
	}}

	role := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "Role",
		"metadata": map[string]interface{}{
			"name": "admin", "namespace": "default",
		},
	}}

	lookup := &stubLookup{
		objects: map[ObjectRef]*unstructured.Unstructured{
			{Group: "rbac.authorization.k8s.io", Kind: "Role", Namespace: "default", Name: "admin"}: role,
		},
	}

	edges := r.Resolve(rb, lookup)
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if edges[0].To.Kind != "Role" {
		t.Fatalf("expected Role, got %v", edges[0].To.Kind)
	}
}

func TestRefRule_TypedRefReverse(t *testing.T) {
	r := NewRuleResolver("test", RefRule{
		FromGroup: "autoscaling", FromKind: "HorizontalPodAutoscaler",
		FieldPath: "spec.scaleTargetRef",
	})

	// A Deployment is being added; an HPA already exists that references it.
	deploy := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]interface{}{
			"name": "web", "namespace": "default",
		},
	}}

	hpa := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "autoscaling/v2", "kind": "HorizontalPodAutoscaler",
		"metadata": map[string]interface{}{
			"name": "web-hpa", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"scaleTargetRef": map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"name":       "web",
			},
		},
	}}

	lookup := &stubLookup{
		objects: map[ObjectRef]*unstructured.Unstructured{
			{Group: "autoscaling", Kind: "HorizontalPodAutoscaler", Namespace: "default", Name: "web-hpa"}: hpa,
		},
	}

	edges := r.Resolve(deploy, lookup)
	if len(edges) != 1 {
		t.Fatalf("expected 1 reverse edge, got %d: %v", len(edges), edges)
	}
	if edges[0].From.Kind != "HorizontalPodAutoscaler" {
		t.Fatalf("expected edge from HPA, got %v", edges[0].From)
	}
	if edges[0].To.Kind != "Deployment" || edges[0].To.Name != "web" {
		t.Fatalf("expected edge to Deployment/web, got %v", edges[0].To)
	}
}

func TestRefRule_TypedRefReverseNoMatch(t *testing.T) {
	// HPA targets Deployment, but we add a StatefulSet — no edge.
	r := NewRuleResolver("test", RefRule{
		FromGroup: "autoscaling", FromKind: "HorizontalPodAutoscaler",
		FieldPath: "spec.scaleTargetRef",
	})

	ss := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1", "kind": "StatefulSet",
		"metadata": map[string]interface{}{
			"name": "db", "namespace": "default",
		},
	}}

	hpa := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "autoscaling/v2", "kind": "HorizontalPodAutoscaler",
		"metadata": map[string]interface{}{
			"name": "web-hpa", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"scaleTargetRef": map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"name":       "web",
			},
		},
	}}

	lookup := &stubLookup{
		objects: map[ObjectRef]*unstructured.Unstructured{
			{Group: "autoscaling", Kind: "HorizontalPodAutoscaler", Namespace: "default", Name: "web-hpa"}: hpa,
		},
	}

	edges := r.Resolve(ss, lookup)
	if len(edges) != 0 {
		t.Fatalf("expected 0 edges (HPA targets Deployment, not StatefulSet), got %d: %v", len(edges), edges)
	}
}

func TestRefRule_TypedRefConstraintMismatch(t *testing.T) {
	// Rule constrains ToGroup to "apps" but the ref points to "batch"
	r := NewRuleResolver("test", RefRule{
		FromKind:  "MyController",
		ToGroup:   "apps",
		FieldPath: "spec.targetRef",
	})

	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "MyController",
		"metadata": map[string]interface{}{
			"name": "ctl", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"targetRef": map[string]interface{}{
				"apiGroup": "batch",
				"kind":     "Job",
				"name":     "my-job",
			},
		},
	}}

	job := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "batch/v1", "kind": "Job",
		"metadata": map[string]interface{}{
			"name": "my-job", "namespace": "default",
		},
	}}

	lookup := &stubLookup{
		objects: map[ObjectRef]*unstructured.Unstructured{
			{Group: "batch", Kind: "Job", Namespace: "default", Name: "my-job"}: job,
		},
	}

	edges := r.Resolve(obj, lookup)
	if len(edges) != 0 {
		t.Fatalf("expected 0 edges (constraint mismatch), got %d: %v", len(edges), edges)
	}
}

func TestExtractRawValues_String(t *testing.T) {
	obj := map[string]interface{}{
		"spec": map[string]interface{}{
			"volumeName": "my-pv",
		},
	}
	vals := extractRawValues(obj, "spec.volumeName")
	if len(vals) != 1 {
		t.Fatalf("expected 1 value, got %d", len(vals))
	}
	if s, ok := vals[0].(string); !ok || s != "my-pv" {
		t.Fatalf("expected string 'my-pv', got %v", vals[0])
	}
}

func TestExtractRawValues_Map(t *testing.T) {
	obj := map[string]interface{}{
		"spec": map[string]interface{}{
			"scaleTargetRef": map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"name":       "web",
			},
		},
	}
	vals := extractRawValues(obj, "spec.scaleTargetRef")
	if len(vals) != 1 {
		t.Fatalf("expected 1 value, got %d", len(vals))
	}
	m, ok := vals[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", vals[0])
	}
	if m["kind"] != "Deployment" {
		t.Fatalf("expected kind=Deployment, got %v", m["kind"])
	}
}

func TestExtractRawValues_ArrayOfMaps(t *testing.T) {
	obj := map[string]interface{}{
		"spec": map[string]interface{}{
			"refs": []interface{}{
				map[string]interface{}{
					"kind": "Service",
					"name": "svc-a",
				},
				map[string]interface{}{
					"kind": "Service",
					"name": "svc-b",
				},
			},
		},
	}
	vals := extractRawValues(obj, "spec.refs[*]")
	if len(vals) != 2 {
		t.Fatalf("expected 2 values, got %d", len(vals))
	}
}

func TestParseTypedRef(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]interface{}
		want  ObjectRef
		ok    bool
	}{
		{
			name: "apiVersion with group",
			input: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"name":       "web",
			},
			want: ObjectRef{Group: "apps", Kind: "Deployment", Name: "web"},
			ok:   true,
		},
		{
			name: "apiGroup field",
			input: map[string]interface{}{
				"apiGroup": "rbac.authorization.k8s.io",
				"kind":     "Role",
				"name":     "admin",
			},
			want: ObjectRef{Group: "rbac.authorization.k8s.io", Kind: "Role", Name: "admin"},
			ok:   true,
		},
		{
			name: "group field",
			input: map[string]interface{}{
				"group": "apps",
				"kind":  "Deployment",
				"name":  "web",
			},
			want: ObjectRef{Group: "apps", Kind: "Deployment", Name: "web"},
			ok:   true,
		},
		{
			name: "core apiVersion (no group)",
			input: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Service",
				"name":       "my-svc",
			},
			want: ObjectRef{Group: "", Kind: "Service", Name: "my-svc"},
			ok:   true,
		},
		{
			name: "with namespace",
			input: map[string]interface{}{
				"apiGroup":  "",
				"kind":      "Service",
				"name":      "my-svc",
				"namespace": "prod",
			},
			want: ObjectRef{Group: "", Kind: "Service", Namespace: "prod", Name: "my-svc"},
			ok:   true,
		},
		{
			name:  "missing kind",
			input: map[string]interface{}{"name": "foo"},
			ok:    false,
		},
		{
			name:  "missing name",
			input: map[string]interface{}{"kind": "Pod"},
			ok:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseTypedRef(tt.input)
			if ok != tt.ok {
				t.Fatalf("parseTypedRef ok=%v, want %v", ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Fatalf("parseTypedRef = %v, want %v", got, tt.want)
			}
		})
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

func (s *stubLookup) ListByNamespace(namespace string) []*unstructured.Unstructured {
	var result []*unstructured.Unstructured
	for ref, obj := range s.objects {
		if ref.Namespace == namespace {
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

func TestRefRule_Extract_SameNamespace(t *testing.T) {
	r := NewRuleResolver("test", RefRule{
		FromKind: "Pod", ToKind: "ConfigMap",
		FieldPath: "spec.volumes[*].configMap.name",
	})

	pod := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]interface{}{
			"name": "web", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"volumes": []interface{}{
				map[string]interface{}{
					"configMap": map[string]interface{}{"name": "app-config"},
				},
			},
		},
	}}

	edges := r.Extract(pod)
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d: %v", len(edges), edges)
	}
	if edges[0].To.Kind != "ConfigMap" || edges[0].To.Name != "app-config" {
		t.Fatalf("unexpected target: %v", edges[0].To)
	}
	if edges[0].To.Namespace != "default" {
		t.Fatalf("expected To.Namespace=default, got %q", edges[0].To.Namespace)
	}
	if edges[0].Type != EdgeRef {
		t.Fatalf("expected EdgeRef, got %v", edges[0].Type)
	}
}

func TestRefRule_Extract_ClusterScoped(t *testing.T) {
	r := NewRuleResolver("test", RefRule{
		FromKind:      "Pod",
		ToKind:        "Node",
		FieldPath:     "spec.nodeName",
		ClusterScoped: true,
	})

	pod := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]interface{}{
			"name": "web", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"nodeName": "node-1",
		},
	}}

	edges := r.Extract(pod)
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d: %v", len(edges), edges)
	}
	if edges[0].To.Kind != "Node" || edges[0].To.Name != "node-1" {
		t.Fatalf("unexpected target: %v", edges[0].To)
	}
	if edges[0].To.Namespace != "" {
		t.Fatalf("expected empty namespace for cluster-scoped target, got %q", edges[0].To.Namespace)
	}
}

func TestRefRule_Extract_WithNamespaceField(t *testing.T) {
	r := NewRuleResolver("test", RefRule{
		FromGroup: "example.com", FromKind: "MyResource",
		ToKind:             "Service",
		FieldPath:          "spec.backendRef.name",
		NamespaceFieldPath: "spec.backendRef.namespace",
	})

	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "example.com/v1", "kind": "MyResource",
		"metadata": map[string]interface{}{
			"name": "my-res", "namespace": "staging",
		},
		"spec": map[string]interface{}{
			"backendRef": map[string]interface{}{
				"name":      "backend",
				"namespace": "prod",
			},
		},
	}}

	edges := r.Extract(obj)
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d: %v", len(edges), edges)
	}
	if edges[0].To.Namespace != "prod" {
		t.Fatalf("expected To.Namespace=prod, got %q", edges[0].To.Namespace)
	}
	if edges[0].To.Name != "backend" {
		t.Fatalf("expected To.Name=backend, got %q", edges[0].To.Name)
	}
}

func TestRefRule_Extract_TypedRef(t *testing.T) {
	r := NewRuleResolver("test", RefRule{
		FromGroup: "autoscaling", FromKind: "HorizontalPodAutoscaler",
		FieldPath: "spec.scaleTargetRef",
	})

	hpa := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "autoscaling/v2", "kind": "HorizontalPodAutoscaler",
		"metadata": map[string]interface{}{
			"name": "web-hpa", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"scaleTargetRef": map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"name":       "web",
			},
		},
	}}

	edges := r.Extract(hpa)
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d: %v", len(edges), edges)
	}
	e := edges[0]
	if e.To.Group != "apps" || e.To.Kind != "Deployment" || e.To.Namespace != "default" || e.To.Name != "web" {
		t.Fatalf("expected apps/Deployment/default/web, got %v", e.To)
	}
}

func TestRefRule_Extract_SkipsNonMatchingKind(t *testing.T) {
	r := NewRuleResolver("test", RefRule{
		FromKind: "Pod", ToKind: "ConfigMap",
		FieldPath: "spec.volumes[*].configMap.name",
	})

	cm := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "ConfigMap",
		"metadata": map[string]interface{}{
			"name": "app-config", "namespace": "default",
		},
	}}

	edges := r.Extract(cm)
	if len(edges) != 0 {
		t.Fatalf("expected 0 edges for non-matching kind, got %d: %v", len(edges), edges)
	}
}

func TestLabelRefRule_Forward(t *testing.T) {
	r := NewRuleResolver("test", LabelRefRule{
		FromGroup: "discovery.k8s.io", FromKind: "EndpointSlice",
		ToKind:   "Service",
		LabelKey: "kubernetes.io/service-name",
	})

	eps := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "discovery.k8s.io/v1", "kind": "EndpointSlice",
		"metadata": map[string]interface{}{
			"name": "my-svc-abc", "namespace": "default",
			"labels": map[string]interface{}{
				"kubernetes.io/service-name": "my-svc",
			},
		},
	}}

	svc := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{
			"name": "my-svc", "namespace": "default",
		},
	}}

	lookup := &stubLookup{
		objects: map[ObjectRef]*unstructured.Unstructured{
			{Kind: "Service", Namespace: "default", Name: "my-svc"}: svc,
		},
	}

	edges := r.Resolve(eps, lookup)
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d: %v", len(edges), edges)
	}
	if edges[0].From.Kind != "EndpointSlice" || edges[0].To.Kind != "Service" {
		t.Fatalf("unexpected edge: %v -> %v", edges[0].From, edges[0].To)
	}
	if edges[0].To.Name != "my-svc" {
		t.Fatalf("expected target name my-svc, got %q", edges[0].To.Name)
	}
	if edges[0].Type != EdgeRef {
		t.Fatalf("expected EdgeRef, got %v", edges[0].Type)
	}
	if edges[0].Field != "metadata.labels[kubernetes.io/service-name]" {
		t.Fatalf("unexpected field: %q", edges[0].Field)
	}
}

func TestLabelRefRule_Reverse(t *testing.T) {
	r := NewRuleResolver("test", LabelRefRule{
		FromGroup: "discovery.k8s.io", FromKind: "EndpointSlice",
		ToKind:   "Service",
		LabelKey: "kubernetes.io/service-name",
	})

	// Service is being added; EndpointSlice already exists.
	svc := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{
			"name": "my-svc", "namespace": "default",
		},
	}}

	eps := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "discovery.k8s.io/v1", "kind": "EndpointSlice",
		"metadata": map[string]interface{}{
			"name": "my-svc-abc", "namespace": "default",
			"labels": map[string]interface{}{
				"kubernetes.io/service-name": "my-svc",
			},
		},
	}}

	lookup := &stubLookup{
		objects: map[ObjectRef]*unstructured.Unstructured{
			{Group: "discovery.k8s.io", Kind: "EndpointSlice", Namespace: "default", Name: "my-svc-abc"}: eps,
		},
	}

	edges := r.Resolve(svc, lookup)
	if len(edges) != 1 {
		t.Fatalf("expected 1 reverse edge, got %d: %v", len(edges), edges)
	}
	if edges[0].From.Kind != "EndpointSlice" || edges[0].From.Name != "my-svc-abc" {
		t.Fatalf("expected edge from EndpointSlice/my-svc-abc, got %v", edges[0].From)
	}
	if edges[0].To.Kind != "Service" || edges[0].To.Name != "my-svc" {
		t.Fatalf("expected edge to Service/my-svc, got %v", edges[0].To)
	}
}

func TestLabelRefRule_Extract(t *testing.T) {
	r := NewRuleResolver("test", LabelRefRule{
		FromGroup: "discovery.k8s.io", FromKind: "EndpointSlice",
		ToKind:   "Service",
		LabelKey: "kubernetes.io/service-name",
	})

	eps := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "discovery.k8s.io/v1", "kind": "EndpointSlice",
		"metadata": map[string]interface{}{
			"name": "my-svc-abc", "namespace": "default",
			"labels": map[string]interface{}{
				"kubernetes.io/service-name": "my-svc",
			},
		},
	}}

	edges := r.Extract(eps)
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d: %v", len(edges), edges)
	}
	if edges[0].To.Kind != "Service" || edges[0].To.Name != "my-svc" {
		t.Fatalf("unexpected target: %v", edges[0].To)
	}
	if edges[0].To.Namespace != "default" {
		t.Fatalf("expected same-namespace target, got ns=%q", edges[0].To.Namespace)
	}
}

func TestLabelRefRule_Extract_ClusterScoped(t *testing.T) {
	r := NewRuleResolver("test", LabelRefRule{
		FromKind:      "MyResource",
		ToKind:        "MyClusterKind",
		LabelKey:      "example.com/target",
		ClusterScoped: true,
	})

	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "MyResource",
		"metadata": map[string]interface{}{
			"name": "res-1", "namespace": "default",
			"labels": map[string]interface{}{
				"example.com/target": "global-thing",
			},
		},
	}}

	edges := r.Extract(obj)
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if edges[0].To.Namespace != "" {
		t.Fatalf("expected cluster-scoped target (empty ns), got %q", edges[0].To.Namespace)
	}
}

func TestLabelRefRule_NoLabel(t *testing.T) {
	r := NewRuleResolver("test", LabelRefRule{
		FromGroup: "discovery.k8s.io", FromKind: "EndpointSlice",
		ToKind:   "Service",
		LabelKey: "kubernetes.io/service-name",
	})

	// EndpointSlice without the label.
	eps := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "discovery.k8s.io/v1", "kind": "EndpointSlice",
		"metadata": map[string]interface{}{
			"name": "orphan", "namespace": "default",
		},
	}}

	svc := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{
			"name": "my-svc", "namespace": "default",
		},
	}}

	lookup := &stubLookup{
		objects: map[ObjectRef]*unstructured.Unstructured{
			{Kind: "Service", Namespace: "default", Name: "my-svc"}: svc,
		},
	}

	edges := r.Resolve(eps, lookup)
	if len(edges) != 0 {
		t.Fatalf("expected 0 edges (no label), got %d: %v", len(edges), edges)
	}

	extractEdges := r.Extract(eps)
	if len(extractEdges) != 0 {
		t.Fatalf("expected 0 extract edges (no label), got %d", len(extractEdges))
	}
}

func TestLabelRefRule_WrongKind(t *testing.T) {
	r := NewRuleResolver("test", LabelRefRule{
		FromGroup: "discovery.k8s.io", FromKind: "EndpointSlice",
		ToKind:   "Service",
		LabelKey: "kubernetes.io/service-name",
	})

	// A Pod with the label — should not match.
	pod := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]interface{}{
			"name": "web", "namespace": "default",
			"labels": map[string]interface{}{
				"kubernetes.io/service-name": "my-svc",
			},
		},
	}}

	edges := r.Extract(pod)
	if len(edges) != 0 {
		t.Fatalf("expected 0 edges (wrong From kind), got %d", len(edges))
	}
}

func TestLabelSelectorRule_NamespaceSelectorAny(t *testing.T) {
	r := NewRuleResolver("test", LabelSelectorRule{
		FromGroup:                  "monitoring.coreos.com",
		FromKind:                   "ServiceMonitor",
		ToKind:                     "Service",
		SelectorFieldPath:          "spec.selector",
		NamespaceSelectorFieldPath: "spec.namespaceSelector",
	})

	sm := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "monitoring.coreos.com/v1", "kind": "ServiceMonitor",
		"metadata": map[string]interface{}{
			"name": "mon", "namespace": "monitoring",
		},
		"spec": map[string]interface{}{
			"namespaceSelector": map[string]interface{}{"any": true},
			"selector": map[string]interface{}{
				"matchLabels": map[string]interface{}{"app": "web"},
			},
		},
	}}

	svcDefault := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{
			"name": "web", "namespace": "default",
			"labels": map[string]interface{}{"app": "web"},
		},
	}}

	svcProd := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{
			"name": "web", "namespace": "prod",
			"labels": map[string]interface{}{"app": "web"},
		},
	}}

	lookup := &stubLookup{objects: map[ObjectRef]*unstructured.Unstructured{
		{Kind: "Service", Namespace: "default", Name: "web"}: svcDefault,
		{Kind: "Service", Namespace: "prod", Name: "web"}:    svcProd,
	}}

	edges := r.Resolve(sm, lookup)
	if len(edges) != 2 {
		t.Fatalf("any:true should match services in all namespaces, got %d edges: %v", len(edges), edges)
	}
}

func TestLabelSelectorRule_NamespaceSelectorMatchNames(t *testing.T) {
	r := NewRuleResolver("test", LabelSelectorRule{
		FromGroup:                  "monitoring.coreos.com",
		FromKind:                   "ServiceMonitor",
		ToKind:                     "Service",
		SelectorFieldPath:          "spec.selector",
		NamespaceSelectorFieldPath: "spec.namespaceSelector",
	})

	sm := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "monitoring.coreos.com/v1", "kind": "ServiceMonitor",
		"metadata": map[string]interface{}{
			"name": "mon", "namespace": "monitoring",
		},
		"spec": map[string]interface{}{
			"namespaceSelector": map[string]interface{}{
				"matchNames": []interface{}{"prod"},
			},
			"selector": map[string]interface{}{
				"matchLabels": map[string]interface{}{"app": "web"},
			},
		},
	}}

	svcProd := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{
			"name": "web", "namespace": "prod",
			"labels": map[string]interface{}{"app": "web"},
		},
	}}

	svcStaging := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{
			"name": "web", "namespace": "staging",
			"labels": map[string]interface{}{"app": "web"},
		},
	}}

	lookup := &stubLookup{objects: map[ObjectRef]*unstructured.Unstructured{
		{Kind: "Service", Namespace: "prod", Name: "web"}:    svcProd,
		{Kind: "Service", Namespace: "staging", Name: "web"}: svcStaging,
	}}

	edges := r.Resolve(sm, lookup)
	if len(edges) != 1 {
		t.Fatalf("matchNames:[prod] should match only prod, got %d edges: %v", len(edges), edges)
	}
	if edges[0].To.Namespace != "prod" {
		t.Fatalf("expected edge to prod namespace, got %s", edges[0].To.Namespace)
	}
}

func TestLabelSelectorRule_NamespaceSelectorAbsent(t *testing.T) {
	r := NewRuleResolver("test", LabelSelectorRule{
		FromGroup:                  "monitoring.coreos.com",
		FromKind:                   "ServiceMonitor",
		ToKind:                     "Service",
		SelectorFieldPath:          "spec.selector",
		NamespaceSelectorFieldPath: "spec.namespaceSelector",
	})

	sm := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "monitoring.coreos.com/v1", "kind": "ServiceMonitor",
		"metadata": map[string]interface{}{
			"name": "mon", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"selector": map[string]interface{}{
				"matchLabels": map[string]interface{}{"app": "web"},
			},
		},
	}}

	svcSameNS := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{
			"name": "web", "namespace": "default",
			"labels": map[string]interface{}{"app": "web"},
		},
	}}

	svcOtherNS := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{
			"name": "web", "namespace": "prod",
			"labels": map[string]interface{}{"app": "web"},
		},
	}}

	lookup := &stubLookup{objects: map[ObjectRef]*unstructured.Unstructured{
		{Kind: "Service", Namespace: "default", Name: "web"}: svcSameNS,
		{Kind: "Service", Namespace: "prod", Name: "web"}:    svcOtherNS,
	}}

	edges := r.Resolve(sm, lookup)
	if len(edges) != 1 {
		t.Fatalf("absent namespaceSelector should match same namespace only, got %d edges: %v", len(edges), edges)
	}
	if edges[0].To.Namespace != "default" {
		t.Fatalf("expected edge to default namespace, got %s", edges[0].To.Namespace)
	}
}

func TestLabelSelectorRule_NamespaceSelectorReverse_Any(t *testing.T) {
	r := NewRuleResolver("test", LabelSelectorRule{
		FromGroup:                  "monitoring.coreos.com",
		FromKind:                   "ServiceMonitor",
		ToKind:                     "Service",
		SelectorFieldPath:          "spec.selector",
		NamespaceSelectorFieldPath: "spec.namespaceSelector",
	})

	// Service being added; ServiceMonitor already exists in another namespace.
	svc := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{
			"name": "web", "namespace": "prod",
			"labels": map[string]interface{}{"app": "web"},
		},
	}}

	sm := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "monitoring.coreos.com/v1", "kind": "ServiceMonitor",
		"metadata": map[string]interface{}{
			"name": "mon", "namespace": "monitoring",
		},
		"spec": map[string]interface{}{
			"namespaceSelector": map[string]interface{}{"any": true},
			"selector": map[string]interface{}{
				"matchLabels": map[string]interface{}{"app": "web"},
			},
		},
	}}

	lookup := &stubLookup{objects: map[ObjectRef]*unstructured.Unstructured{
		{Group: "monitoring.coreos.com", Kind: "ServiceMonitor", Namespace: "monitoring", Name: "mon"}: sm,
	}}

	edges := r.Resolve(svc, lookup)
	if len(edges) != 1 {
		t.Fatalf("reverse with any:true should find cross-namespace ServiceMonitor, got %d edges: %v", len(edges), edges)
	}
	if edges[0].From.Kind != "ServiceMonitor" || edges[0].To.Name != "web" {
		t.Fatalf("unexpected edge: %+v", edges[0])
	}
}

func TestLabelSelectorRule_NamespaceSelectorReverse_MatchNames(t *testing.T) {
	r := NewRuleResolver("test", LabelSelectorRule{
		FromGroup:                  "monitoring.coreos.com",
		FromKind:                   "ServiceMonitor",
		ToKind:                     "Service",
		SelectorFieldPath:          "spec.selector",
		NamespaceSelectorFieldPath: "spec.namespaceSelector",
	})

	// Service in prod — should be matched.
	svcProd := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{
			"name": "web", "namespace": "prod",
			"labels": map[string]interface{}{"app": "web"},
		},
	}}

	// Service in staging — should NOT be matched.
	svcStaging := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{
			"name": "web", "namespace": "staging",
			"labels": map[string]interface{}{"app": "web"},
		},
	}}

	sm := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "monitoring.coreos.com/v1", "kind": "ServiceMonitor",
		"metadata": map[string]interface{}{
			"name": "mon", "namespace": "monitoring",
		},
		"spec": map[string]interface{}{
			"namespaceSelector": map[string]interface{}{
				"matchNames": []interface{}{"prod"},
			},
			"selector": map[string]interface{}{
				"matchLabels": map[string]interface{}{"app": "web"},
			},
		},
	}}

	lookup := &stubLookup{objects: map[ObjectRef]*unstructured.Unstructured{
		{Group: "monitoring.coreos.com", Kind: "ServiceMonitor", Namespace: "monitoring", Name: "mon"}: sm,
	}}

	edgesProd := r.Resolve(svcProd, lookup)
	if len(edgesProd) != 1 {
		t.Fatalf("reverse matchNames:[prod] should match prod Service, got %d edges", len(edgesProd))
	}

	edgesStaging := r.Resolve(svcStaging, lookup)
	if len(edgesStaging) != 0 {
		t.Fatalf("reverse matchNames:[prod] should NOT match staging Service, got %d edges", len(edgesStaging))
	}
}

func TestLabelSelectorRule_Extract_ReturnsNil(t *testing.T) {
	r := NewRuleResolver("test", LabelSelectorRule{
		FromGroup: "", FromKind: "Service",
		ToGroup: "", ToKind: "Pod",
		SelectorFieldPath: "spec.selector",
	})

	svc := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{
			"name": "my-svc", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"selector": map[string]interface{}{
				"app": "nginx",
			},
		},
	}}

	edges := r.Extract(svc)
	if len(edges) != 0 {
		t.Fatalf("expected 0 edges (selector needs Lookup), got %d: %v", len(edges), edges)
	}
}
