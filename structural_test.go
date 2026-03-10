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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func TestStructuralResolver_Tier1References(t *testing.T) {
	g := New(WithResolver(NewStructuralResolver()))

	objs := []unstructured.Unstructured{
		// Targets
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "Secret",
			"metadata": map[string]interface{}{
				"name": "pull-secret", "namespace": "default",
			},
		}},
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "Secret",
			"metadata": map[string]interface{}{
				"name": "db-creds", "namespace": "default",
			},
		}},
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "ConfigMap",
			"metadata": map[string]interface{}{
				"name": "env-config", "namespace": "default",
			},
		}},
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "Secret",
			"metadata": map[string]interface{}{
				"name": "tls-cert", "namespace": "default",
			},
		}},
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "Service",
			"metadata": map[string]interface{}{
				"name": "headless-svc", "namespace": "default",
			},
		}},
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "Node",
			"metadata": map[string]interface{}{"name": "node-1"},
		}},
		{Object: map[string]interface{}{
			"apiVersion": "storage.k8s.io/v1", "kind": "StorageClass",
			"metadata": map[string]interface{}{"name": "fast-ssd"},
		}},
		// Pod with imagePullSecrets, env valueFrom, projected volumes, nodeName
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "Pod",
			"metadata": map[string]interface{}{
				"name": "app", "namespace": "default",
			},
			"spec": map[string]interface{}{
				"nodeName": "node-1",
				"imagePullSecrets": []interface{}{
					map[string]interface{}{"name": "pull-secret"},
				},
				"containers": []interface{}{
					map[string]interface{}{
						"name": "main",
						"env": []interface{}{
							map[string]interface{}{
								"name": "DB_PASS",
								"valueFrom": map[string]interface{}{
									"secretKeyRef": map[string]interface{}{
										"name": "db-creds",
										"key":  "password",
									},
								},
							},
							map[string]interface{}{
								"name": "APP_ENV",
								"valueFrom": map[string]interface{}{
									"configMapKeyRef": map[string]interface{}{
										"name": "env-config",
										"key":  "environment",
									},
								},
							},
						},
					},
				},
			},
		}},
		// Ingress with TLS
		{Object: map[string]interface{}{
			"apiVersion": "networking.k8s.io/v1", "kind": "Ingress",
			"metadata": map[string]interface{}{
				"name": "web-ingress", "namespace": "default",
			},
			"spec": map[string]interface{}{
				"tls": []interface{}{
					map[string]interface{}{
						"secretName": "tls-cert",
					},
				},
			},
		}},
		// StatefulSet with serviceName
		{Object: map[string]interface{}{
			"apiVersion": "apps/v1", "kind": "StatefulSet",
			"metadata": map[string]interface{}{
				"name": "db", "namespace": "default",
			},
			"spec": map[string]interface{}{
				"serviceName": "headless-svc",
			},
		}},
		// PV with storageClassName
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "PersistentVolume",
			"metadata": map[string]interface{}{"name": "pv-1"},
			"spec": map[string]interface{}{
				"storageClassName": "fast-ssd",
			},
		}},
	}

	g.Load(objs)

	tests := []struct {
		name     string
		from     ObjectRef
		toKind   string
		toName   string
	}{
		{"Pod->Secret (imagePullSecrets)", ObjectRef{Kind: "Pod", Namespace: "default", Name: "app"}, "Secret", "pull-secret"},
		{"Pod->Secret (env secretKeyRef)", ObjectRef{Kind: "Pod", Namespace: "default", Name: "app"}, "Secret", "db-creds"},
		{"Pod->ConfigMap (env configMapKeyRef)", ObjectRef{Kind: "Pod", Namespace: "default", Name: "app"}, "ConfigMap", "env-config"},
		{"Pod->Node", ObjectRef{Kind: "Pod", Namespace: "default", Name: "app"}, "Node", "node-1"},
		{"Ingress->Secret (TLS)", ObjectRef{Group: "networking.k8s.io", Kind: "Ingress", Namespace: "default", Name: "web-ingress"}, "Secret", "tls-cert"},
		{"StatefulSet->Service", ObjectRef{Group: "apps", Kind: "StatefulSet", Namespace: "default", Name: "db"}, "Service", "headless-svc"},
		{"PV->StorageClass", ObjectRef{Kind: "PersistentVolume", Name: "pv-1"}, "StorageClass", "fast-ssd"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := g.DependenciesOf(tt.from)
			found := false
			for _, dep := range deps {
				if dep.To.Kind == tt.toKind && dep.To.Name == tt.toName {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("expected %s -> %s/%s, got deps: %v", tt.from, tt.toKind, tt.toName, deps)
			}
		})
	}
}

func TestStructuralResolver_Extract_OwnerRef(t *testing.T) {
	r := NewStructuralResolver()

	pod := newCoreObj("Pod", "default", "web-xyz")
	pod.SetOwnerReferences([]metav1.OwnerReference{{
		APIVersion: "apps/v1",
		Kind:       "ReplicaSet",
		Name:       "web-abc",
	}})

	edges := r.Extract(&pod)

	var found bool
	for _, e := range edges {
		if e.Field == "metadata.ownerReferences" && e.To.Kind == "ReplicaSet" && e.To.Name == "web-abc" {
			found = true
			if e.To.Namespace != "default" {
				t.Fatalf("ownerRef target should inherit source namespace, got %q", e.To.Namespace)
			}
		}
	}
	if !found {
		t.Errorf("expected ownerRef edge, got: %v", edges)
	}
}

func TestStructuralResolver_Extract_RefRule(t *testing.T) {
	r := NewStructuralResolver()

	pod := newCoreObj("Pod", "default", "web")
	unstructured.SetNestedField(pod.Object, "my-sa", "spec", "serviceAccountName")

	edges := r.Extract(&pod)

	var found bool
	for _, e := range edges {
		if e.To.Kind == "ServiceAccount" && e.To.Name == "my-sa" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected ServiceAccount ref edge from Extract, got: %v", edges)
	}
}

func TestStructuralResolver_StorageAndSARules(t *testing.T) {
	g := New(WithResolver(NewStructuralResolver()))

	objs := []unstructured.Unstructured{
		// Targets
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "Secret",
			"metadata": map[string]interface{}{
				"name": "sa-token", "namespace": "default",
			},
		}},
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "Secret",
			"metadata": map[string]interface{}{
				"name": "registry-creds", "namespace": "default",
			},
		}},
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "PersistentVolume",
			"metadata": map[string]interface{}{"name": "pv-data"},
		}},
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "Node",
			"metadata": map[string]interface{}{"name": "worker-1"},
		}},
		{Object: map[string]interface{}{
			"apiVersion": "storage.k8s.io/v1", "kind": "CSIDriver",
			"metadata": map[string]interface{}{"name": "ebs.csi.aws.com"},
		}},
		// Sources
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "ServiceAccount",
			"metadata": map[string]interface{}{
				"name": "my-sa", "namespace": "default",
			},
			"secrets": []interface{}{
				map[string]interface{}{"name": "sa-token"},
			},
			"imagePullSecrets": []interface{}{
				map[string]interface{}{"name": "registry-creds"},
			},
		}},
		{Object: map[string]interface{}{
			"apiVersion": "storage.k8s.io/v1", "kind": "VolumeAttachment",
			"metadata": map[string]interface{}{"name": "va-1"},
			"spec": map[string]interface{}{
				"nodeName": "worker-1",
				"source": map[string]interface{}{
					"persistentVolumeName": "pv-data",
				},
			},
		}},
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "PersistentVolume",
			"metadata": map[string]interface{}{"name": "pv-csi"},
			"spec": map[string]interface{}{
				"csi": map[string]interface{}{
					"driver": "ebs.csi.aws.com",
				},
			},
		}},
		{Object: map[string]interface{}{
			"apiVersion": "storage.k8s.io/v1", "kind": "StorageClass",
			"metadata":   map[string]interface{}{"name": "gp3"},
			"provisioner": "ebs.csi.aws.com",
		}},
	}

	g.Load(objs)

	tests := []struct {
		name   string
		from   ObjectRef
		toKind string
		toName string
	}{
		{"SA->Secret (secrets)", ObjectRef{Kind: "ServiceAccount", Namespace: "default", Name: "my-sa"}, "Secret", "sa-token"},
		{"SA->Secret (imagePullSecrets)", ObjectRef{Kind: "ServiceAccount", Namespace: "default", Name: "my-sa"}, "Secret", "registry-creds"},
		{"VolumeAttachment->PV", ObjectRef{Group: "storage.k8s.io", Kind: "VolumeAttachment", Name: "va-1"}, "PersistentVolume", "pv-data"},
		{"VolumeAttachment->Node", ObjectRef{Group: "storage.k8s.io", Kind: "VolumeAttachment", Name: "va-1"}, "Node", "worker-1"},
		{"PV->CSIDriver", ObjectRef{Kind: "PersistentVolume", Name: "pv-csi"}, "CSIDriver", "ebs.csi.aws.com"},
		{"StorageClass->CSIDriver", ObjectRef{Group: "storage.k8s.io", Kind: "StorageClass", Name: "gp3"}, "CSIDriver", "ebs.csi.aws.com"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			deps := g.DependenciesOf(tt.from)
			found := false
			for _, dep := range deps {
				if dep.To.Kind == tt.toKind && dep.To.Name == tt.toName {
					found = true
					break
				}
			}
			if !found {
				t.Fatalf("expected %s -> %s/%s, got deps: %v", tt.from, tt.toKind, tt.toName, deps)
			}
		})
	}
}

func TestStructuralResolver_EndpointSliceToService(t *testing.T) {
	g := New(WithResolver(NewStructuralResolver()))

	svc := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{
			"name": "my-svc", "namespace": "default",
		},
	}}

	eps := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "discovery.k8s.io/v1", "kind": "EndpointSlice",
		"metadata": map[string]interface{}{
			"name": "my-svc-abc", "namespace": "default",
			"labels": map[string]interface{}{
				"kubernetes.io/service-name": "my-svc",
			},
		},
	}}

	g.Load([]unstructured.Unstructured{svc, eps})

	epsRef := ObjectRef{Group: "discovery.k8s.io", Kind: "EndpointSlice", Namespace: "default", Name: "my-svc-abc"}
	deps := g.DependenciesOf(epsRef)
	if len(deps) != 1 {
		t.Fatalf("expected 1 dependency, got %d: %v", len(deps), deps)
	}
	if deps[0].To.Kind != "Service" || deps[0].To.Name != "my-svc" {
		t.Fatalf("unexpected target: %v", deps[0].To)
	}
	if deps[0].Resolver != "structural" {
		t.Fatalf("expected resolver 'structural', got %q", deps[0].Resolver)
	}

	// Verify reverse: Service has EndpointSlice as dependent.
	svcRef := ObjectRef{Kind: "Service", Namespace: "default", Name: "my-svc"}
	dependents := g.DependentsOf(svcRef)
	if len(dependents) != 1 {
		t.Fatalf("expected 1 dependent, got %d: %v", len(dependents), dependents)
	}
	if dependents[0].From.Kind != "EndpointSlice" {
		t.Fatalf("expected dependent to be EndpointSlice, got %v", dependents[0].From)
	}
}

func TestStructuralResolver_EndpointSliceReverse(t *testing.T) {
	g := New(WithResolver(NewStructuralResolver()))

	eps := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "discovery.k8s.io/v1", "kind": "EndpointSlice",
		"metadata": map[string]interface{}{
			"name": "my-svc-abc", "namespace": "default",
			"labels": map[string]interface{}{
				"kubernetes.io/service-name": "my-svc",
			},
		},
	}}

	svc := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{
			"name": "my-svc", "namespace": "default",
		},
	}}

	// Add EndpointSlice first, then Service — reverse resolution must discover the edge.
	g.Add(eps)
	g.Add(svc)

	epsRef := ObjectRef{Group: "discovery.k8s.io", Kind: "EndpointSlice", Namespace: "default", Name: "my-svc-abc"}
	deps := g.DependenciesOf(epsRef)
	if len(deps) != 1 {
		t.Fatalf("expected 1 dependency via reverse resolution, got %d: %v", len(deps), deps)
	}
	if deps[0].To.Kind != "Service" || deps[0].To.Name != "my-svc" {
		t.Fatalf("unexpected target: %v", deps[0].To)
	}
}

func TestStructuralResolver_Extract_ClusterScopedTarget(t *testing.T) {
	r := NewStructuralResolver()

	pod := newCoreObj("Pod", "default", "web")
	unstructured.SetNestedField(pod.Object, "node-1", "spec", "nodeName")

	edges := r.Extract(&pod)

	var found bool
	for _, e := range edges {
		if e.To.Kind == "Node" && e.To.Name == "node-1" {
			found = true
			if e.To.Namespace != "" {
				t.Fatalf("Node is cluster-scoped, expected empty namespace, got %q", e.To.Namespace)
			}
		}
	}
	if !found {
		t.Errorf("expected Node ref edge from Extract, got: %v", edges)
	}
}
