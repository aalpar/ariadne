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

func makeDeployment(ns, name, saName string, labels map[string]interface{}) unstructured.Unstructured {
	deploy := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]interface{}{
			"name": name, "namespace": ns,
		},
		"spec": map[string]interface{}{
			"template": map[string]interface{}{
				"metadata": map[string]interface{}{
					"labels": labels,
				},
				"spec": map[string]interface{}{
					"serviceAccountName": saName,
				},
			},
		},
	}}
	return deploy
}

func TestExtractPodTemplates_Deployment(t *testing.T) {
	deploy := makeDeployment("default", "web", "web-sa", map[string]interface{}{"app": "web"})

	templates := ExtractPodTemplates([]unstructured.Unstructured{deploy})
	if len(templates) != 1 {
		t.Fatalf("expected 1 PodTemplate, got %d", len(templates))
	}

	pt := templates[0]
	if pt.GetKind() != "PodTemplate" {
		t.Fatalf("expected kind PodTemplate, got %s", pt.GetKind())
	}
	if pt.GetName() != "web" {
		t.Fatalf("expected name 'web', got %q", pt.GetName())
	}
	if pt.GetNamespace() != "default" {
		t.Fatalf("expected namespace 'default', got %q", pt.GetNamespace())
	}

	owners := pt.GetOwnerReferences()
	if len(owners) != 1 {
		t.Fatalf("expected 1 ownerReference, got %d", len(owners))
	}
	if owners[0].Kind != "Deployment" || owners[0].Name != "web" {
		t.Fatalf("unexpected ownerReference: %v", owners[0])
	}
}

func TestExtractPodTemplates_AllWorkloadKinds(t *testing.T) {
	workloads := []unstructured.Unstructured{
		newObj("apps", "v1", "Deployment", "ns", "d"),
		newObj("apps", "v1", "StatefulSet", "ns", "ss"),
		newObj("apps", "v1", "DaemonSet", "ns", "ds"),
		newObj("apps", "v1", "ReplicaSet", "ns", "rs"),
		newObj("batch", "v1", "Job", "ns", "j"),
	}
	// Set spec.template on each.
	for i := range workloads {
		unstructured.SetNestedMap(workloads[i].Object, map[string]interface{}{
			"metadata": map[string]interface{}{"labels": map[string]interface{}{"app": "test"}},
			"spec":     map[string]interface{}{"serviceAccountName": "sa"},
		}, "spec", "template")
	}

	templates := ExtractPodTemplates(workloads)
	if len(templates) != 5 {
		t.Fatalf("expected 5 PodTemplates, got %d", len(templates))
	}
	for i, pt := range templates {
		if pt.GetKind() != "PodTemplate" {
			t.Errorf("template[%d]: expected kind PodTemplate, got %s", i, pt.GetKind())
		}
	}
}

func TestExtractPodTemplates_CronJob(t *testing.T) {
	cj := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "batch/v1", "kind": "CronJob",
		"metadata": map[string]interface{}{
			"name": "nightly", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"jobTemplate": map[string]interface{}{
				"spec": map[string]interface{}{
					"template": map[string]interface{}{
						"metadata": map[string]interface{}{
							"labels": map[string]interface{}{"app": "nightly"},
						},
						"spec": map[string]interface{}{
							"serviceAccountName": "cron-sa",
						},
					},
				},
			},
		},
	}}

	templates := ExtractPodTemplates([]unstructured.Unstructured{cj})
	if len(templates) != 1 {
		t.Fatalf("expected 1 PodTemplate, got %d", len(templates))
	}
	if templates[0].GetName() != "nightly" {
		t.Fatalf("expected name 'nightly', got %q", templates[0].GetName())
	}
}

func TestExtractPodTemplates_SkipsNonWorkloads(t *testing.T) {
	objs := []unstructured.Unstructured{
		newCoreObj("Service", "default", "svc"),
		newCoreObj("ConfigMap", "default", "cm"),
		newCoreObj("Pod", "default", "pod"),
	}

	templates := ExtractPodTemplates(objs)
	if len(templates) != 0 {
		t.Fatalf("expected 0 PodTemplates from non-workloads, got %d", len(templates))
	}
}

func TestPodTemplateRules_Generation(t *testing.T) {
	input := []RefRule{
		{FromKind: "Pod", ToKind: "ServiceAccount", FieldPath: "spec.serviceAccountName"},
		{FromKind: "Pod", ToKind: "ConfigMap", FieldPath: "spec.volumes[*].configMap.name"},
		{FromKind: "Pod", ToKind: "Node", FieldPath: "spec.nodeName", ClusterScoped: true},
	}

	got := podTemplateRules(input)
	if len(got) != len(input) {
		t.Fatalf("expected %d rules, got %d", len(input), len(got))
	}
	for i, r := range got {
		if r.FromKind != "PodTemplate" {
			t.Errorf("rule[%d]: expected FromKind PodTemplate, got %q", i, r.FromKind)
		}
		if r.ToKind != input[i].ToKind {
			t.Errorf("rule[%d]: expected ToKind %q, got %q", i, input[i].ToKind, r.ToKind)
		}
		want := "template." + input[i].FieldPath
		if r.FieldPath != want {
			t.Errorf("rule[%d]: expected FieldPath %q, got %q", i, want, r.FieldPath)
		}
		if r.ClusterScoped != input[i].ClusterScoped {
			t.Errorf("rule[%d]: ClusterScoped mismatch", i)
		}
	}
}

// TestPodTemplate_RefResolution verifies that PodTemplate → ServiceAccount/ConfigMap/Secret
// edges are resolved when PodTemplates are in the graph.
func TestPodTemplate_RefResolution(t *testing.T) {
	sa := newCoreObj("ServiceAccount", "default", "web-sa")
	cm := newCoreObj("ConfigMap", "default", "app-config")
	secret := newCoreObj("Secret", "default", "app-secret")

	pt := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "PodTemplate",
		"metadata": map[string]interface{}{
			"name": "web", "namespace": "default",
		},
		"template": map[string]interface{}{
			"spec": map[string]interface{}{
				"serviceAccountName": "web-sa",
				"volumes": []interface{}{
					map[string]interface{}{
						"configMap": map[string]interface{}{"name": "app-config"},
					},
					map[string]interface{}{
						"secret": map[string]interface{}{"secretName": "app-secret"},
					},
				},
			},
		},
	}}

	g := New(WithResolver(NewStructuralResolver()))
	g.Load([]unstructured.Unstructured{sa, cm, secret, pt})

	ptRef := ObjectRef{Kind: "PodTemplate", Namespace: "default", Name: "web"}
	deps := g.DependenciesOf(ptRef)
	if len(deps) != 3 {
		t.Fatalf("expected 3 dependencies, got %d: %v", len(deps), deps)
	}

	kinds := map[string]bool{}
	for _, e := range deps {
		kinds[e.To.Kind] = true
	}
	for _, want := range []string{"ServiceAccount", "ConfigMap", "Secret"} {
		if !kinds[want] {
			t.Errorf("expected dependency on %s, not found in %v", want, deps)
		}
	}
}

// TestPodTemplate_SelectorMatching verifies that Service → PodTemplate edges
// are resolved via label selector matching against template.metadata.labels.
func TestPodTemplate_SelectorMatching(t *testing.T) {
	pt := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "PodTemplate",
		"metadata": map[string]interface{}{
			"name": "web", "namespace": "default",
			"labels": map[string]interface{}{"managed-by": "ariadne"}, // NOT the pod labels
		},
		"template": map[string]interface{}{
			"metadata": map[string]interface{}{
				"labels": map[string]interface{}{"app": "web", "version": "v1"},
			},
			"spec": map[string]interface{}{},
		},
	}}

	svc := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{
			"name": "web-svc", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"selector": map[string]interface{}{"app": "web"},
		},
	}}

	// A Service that should NOT match (selects different labels).
	svcOther := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{
			"name": "other-svc", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"selector": map[string]interface{}{"app": "other"},
		},
	}}

	g := New(WithResolver(NewSelectorResolver()))
	g.Load([]unstructured.Unstructured{pt, svc, svcOther})

	svcRef := ObjectRef{Kind: "Service", Namespace: "default", Name: "web-svc"}
	deps := g.DependenciesOf(svcRef)
	if len(deps) != 1 {
		t.Fatalf("expected 1 dependency from web-svc, got %d: %v", len(deps), deps)
	}
	if deps[0].To.Kind != "PodTemplate" || deps[0].To.Name != "web" {
		t.Fatalf("expected Service → PodTemplate/web, got %v", deps[0].To)
	}

	// other-svc should NOT have edges.
	otherRef := ObjectRef{Kind: "Service", Namespace: "default", Name: "other-svc"}
	if len(g.DependenciesOf(otherRef)) != 0 {
		t.Fatal("expected no dependencies from other-svc")
	}

	// Verify matching uses template labels, not metadata labels.
	// A Service selecting "managed-by: ariadne" should NOT match.
	svcMeta := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{
			"name": "meta-svc", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"selector": map[string]interface{}{"managed-by": "ariadne"},
		},
	}}
	g.Add(svcMeta)
	metaRef := ObjectRef{Kind: "Service", Namespace: "default", Name: "meta-svc"}
	if len(g.DependenciesOf(metaRef)) != 0 {
		t.Fatal("Service selecting metadata.labels should NOT match PodTemplate's template.metadata.labels")
	}
}

// TestWithPodTemplates_EndToEnd tests the full opt-in extraction pipeline:
// Deployment + ServiceAccount + Service → extracts PodTemplate →
// Deployment ← PodTemplate → ServiceAccount, Service → PodTemplate.
func TestWithPodTemplates_EndToEnd(t *testing.T) {
	deploy := makeDeployment("default", "web", "web-sa",
		map[string]interface{}{"app": "web"})
	sa := newCoreObj("ServiceAccount", "default", "web-sa")
	svc := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{
			"name": "web-svc", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"selector": map[string]interface{}{"app": "web"},
		},
	}}

	g := NewDefault(WithPodTemplates())
	g.Load([]unstructured.Unstructured{deploy, sa, svc})

	// PodTemplate should exist.
	ptRef := ObjectRef{Kind: "PodTemplate", Namespace: "default", Name: "web"}
	if !g.Has(ptRef) {
		t.Fatal("expected PodTemplate/web to be in the graph")
	}

	// PodTemplate → Deployment (ownerRef).
	ptDeps := g.DependenciesOf(ptRef)
	foundDeploy := false
	foundSA := false
	for _, e := range ptDeps {
		if e.To.Kind == "Deployment" && e.To.Name == "web" {
			foundDeploy = true
		}
		if e.To.Kind == "ServiceAccount" && e.To.Name == "web-sa" {
			foundSA = true
		}
	}
	if !foundDeploy {
		t.Error("expected PodTemplate → Deployment edge (ownerRef)")
	}
	if !foundSA {
		t.Error("expected PodTemplate → ServiceAccount edge")
	}

	// Service → PodTemplate (selector).
	svcRef := ObjectRef{Kind: "Service", Namespace: "default", Name: "web-svc"}
	svcDeps := g.DependenciesOf(svcRef)
	foundPT := false
	for _, e := range svcDeps {
		if e.To.Kind == "PodTemplate" && e.To.Name == "web" {
			foundPT = true
		}
	}
	if !foundPT {
		t.Error("expected Service → PodTemplate edge (selector)")
	}

	// Transitive: Service.Upstream should reach Deployment and ServiceAccount.
	upstream := g.Upstream(svcRef)
	upstreamKinds := map[string]bool{}
	for _, ref := range upstream {
		upstreamKinds[ref.Kind] = true
	}
	for _, want := range []string{"PodTemplate", "Deployment", "ServiceAccount"} {
		if !upstreamKinds[want] {
			t.Errorf("expected %s in Service upstream, got %v", want, upstream)
		}
	}
}

// TestWithPodTemplates_DisabledByDefault ensures PodTemplates are not
// extracted when WithPodTemplates is not set.
func TestWithPodTemplates_DisabledByDefault(t *testing.T) {
	deploy := makeDeployment("default", "web", "web-sa",
		map[string]interface{}{"app": "web"})

	g := NewDefault()
	g.Load([]unstructured.Unstructured{deploy})

	ptRef := ObjectRef{Kind: "PodTemplate", Namespace: "default", Name: "web"}
	if g.Has(ptRef) {
		t.Fatal("PodTemplate should not exist when WithPodTemplates is not set")
	}
}
