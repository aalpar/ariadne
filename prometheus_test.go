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

func TestPrometheus_ServiceMonitorSameNamespace(t *testing.T) {
	g := New(WithResolver(NewPrometheusResolver()))

	sm := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "monitoring.coreos.com/v1", "kind": "ServiceMonitor",
		"metadata": map[string]interface{}{
			"name": "web-monitor", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"selector": map[string]interface{}{
				"matchLabels": map[string]interface{}{"app": "web"},
			},
		},
	}}

	svc := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{
			"name": "web-svc", "namespace": "default",
			"labels": map[string]interface{}{"app": "web"},
		},
	}}

	svcOther := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{
			"name": "web-svc", "namespace": "prod",
			"labels": map[string]interface{}{"app": "web"},
		},
	}}

	g.Load([]unstructured.Unstructured{sm, svc, svcOther})

	smRef := ObjectRef{Group: "monitoring.coreos.com", Kind: "ServiceMonitor", Namespace: "default", Name: "web-monitor"}
	deps := g.DependenciesOf(smRef)
	if len(deps) != 1 {
		t.Fatalf("same-namespace: expected 1 dep, got %d: %v", len(deps), deps)
	}
	if deps[0].To.Namespace != "default" {
		t.Errorf("expected default namespace, got %s", deps[0].To.Namespace)
	}
	if deps[0].Resolver != "prometheus" {
		t.Errorf("expected resolver 'prometheus', got '%s'", deps[0].Resolver)
	}
}

func TestPrometheus_ServiceMonitorAllNamespaces(t *testing.T) {
	g := New(WithResolver(NewPrometheusResolver()))

	sm := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "monitoring.coreos.com/v1", "kind": "ServiceMonitor",
		"metadata": map[string]interface{}{
			"name": "global-monitor", "namespace": "monitoring",
		},
		"spec": map[string]interface{}{
			"namespaceSelector": map[string]interface{}{"any": true},
			"selector": map[string]interface{}{
				"matchLabels": map[string]interface{}{"monitored": "true"},
			},
		},
	}}

	svc1 := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{
			"name": "api", "namespace": "default",
			"labels": map[string]interface{}{"monitored": "true"},
		},
	}}

	svc2 := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{
			"name": "api", "namespace": "prod",
			"labels": map[string]interface{}{"monitored": "true"},
		},
	}}

	svcNoLabel := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{
			"name": "internal", "namespace": "prod",
			"labels": map[string]interface{}{"monitored": "false"},
		},
	}}

	g.Load([]unstructured.Unstructured{sm, svc1, svc2, svcNoLabel})

	smRef := ObjectRef{Group: "monitoring.coreos.com", Kind: "ServiceMonitor", Namespace: "monitoring", Name: "global-monitor"}
	deps := g.DependenciesOf(smRef)
	if len(deps) != 2 {
		t.Fatalf("any:true should match 2 services, got %d: %v", len(deps), deps)
	}
}

func TestPrometheus_ServiceMonitorMatchNames(t *testing.T) {
	g := New(WithResolver(NewPrometheusResolver()))

	sm := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "monitoring.coreos.com/v1", "kind": "ServiceMonitor",
		"metadata": map[string]interface{}{
			"name": "targeted-monitor", "namespace": "monitoring",
		},
		"spec": map[string]interface{}{
			"namespaceSelector": map[string]interface{}{
				"matchNames": []interface{}{"prod", "staging"},
			},
			"selector": map[string]interface{}{
				"matchLabels": map[string]interface{}{"app": "web"},
			},
		},
	}}

	svcProd := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{
			"name": "web", "namespace": "prod",
			"labels": map[string]interface{}{"app": "web"},
		},
	}}

	svcStaging := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{
			"name": "web", "namespace": "staging",
			"labels": map[string]interface{}{"app": "web"},
		},
	}}

	svcDev := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{
			"name": "web", "namespace": "dev",
			"labels": map[string]interface{}{"app": "web"},
		},
	}}

	g.Load([]unstructured.Unstructured{sm, svcProd, svcStaging, svcDev})

	smRef := ObjectRef{Group: "monitoring.coreos.com", Kind: "ServiceMonitor", Namespace: "monitoring", Name: "targeted-monitor"}
	deps := g.DependenciesOf(smRef)
	if len(deps) != 2 {
		t.Fatalf("matchNames:[prod,staging] should match 2, got %d: %v", len(deps), deps)
	}

	namespaces := map[string]bool{}
	for _, e := range deps {
		namespaces[e.To.Namespace] = true
	}
	if !namespaces["prod"] || !namespaces["staging"] {
		t.Errorf("expected prod and staging, got %v", namespaces)
	}
	if namespaces["dev"] {
		t.Error("dev should not be matched")
	}
}

func TestPrometheus_PodMonitor(t *testing.T) {
	g := New(WithResolver(NewPrometheusResolver()))

	pm := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "monitoring.coreos.com/v1", "kind": "PodMonitor",
		"metadata": map[string]interface{}{
			"name": "pod-mon", "namespace": "monitoring",
		},
		"spec": map[string]interface{}{
			"namespaceSelector": map[string]interface{}{"any": true},
			"selector": map[string]interface{}{
				"matchLabels": map[string]interface{}{"app": "worker"},
			},
		},
	}}

	pod := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]interface{}{
			"name": "worker-1", "namespace": "jobs",
			"labels": map[string]interface{}{"app": "worker"},
		},
	}}

	g.Load([]unstructured.Unstructured{pm, pod})

	pmRef := ObjectRef{Group: "monitoring.coreos.com", Kind: "PodMonitor", Namespace: "monitoring", Name: "pod-mon"}
	deps := g.DependenciesOf(pmRef)
	if len(deps) != 1 {
		t.Fatalf("PodMonitor should match 1 pod, got %d: %v", len(deps), deps)
	}
	if deps[0].To.Kind != "Pod" || deps[0].To.Name != "worker-1" {
		t.Errorf("unexpected target: %v", deps[0].To)
	}
}

func TestPrometheus_ReverseAdd(t *testing.T) {
	g := New(WithResolver(NewPrometheusResolver()))

	// Add ServiceMonitor first — no Services exist yet.
	sm := unstructured.Unstructured{Object: map[string]interface{}{
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
	g.Add(sm)

	smRef := ObjectRef{Group: "monitoring.coreos.com", Kind: "ServiceMonitor", Namespace: "monitoring", Name: "mon"}
	if deps := g.DependenciesOf(smRef); len(deps) != 0 {
		t.Fatalf("expected 0 deps before Service exists, got %d", len(deps))
	}

	// Add matching Service in prod — reverse resolution creates the edge.
	svc := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{
			"name": "web", "namespace": "prod",
			"labels": map[string]interface{}{"app": "web"},
		},
	}}
	g.Add(svc)

	deps := g.DependenciesOf(smRef)
	if len(deps) != 1 {
		t.Fatalf("expected 1 dep after adding Service, got %d: %v", len(deps), deps)
	}
	if deps[0].To.Kind != "Service" || deps[0].To.Namespace != "prod" {
		t.Errorf("unexpected target: %v", deps[0].To)
	}

	// Add non-matching Service in staging — should NOT create an edge.
	svcStaging := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{
			"name": "web", "namespace": "staging",
			"labels": map[string]interface{}{"app": "web"},
		},
	}}
	g.Add(svcStaging)

	deps = g.DependenciesOf(smRef)
	if len(deps) != 1 {
		t.Fatalf("staging Service should not match matchNames:[prod], got %d deps: %v", len(deps), deps)
	}
}

func TestPrometheus_SelectorMismatch(t *testing.T) {
	g := New(WithResolver(NewPrometheusResolver()))

	sm := unstructured.Unstructured{Object: map[string]interface{}{
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

	svc := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{
			"name": "db-svc", "namespace": "default",
			"labels": map[string]interface{}{"app": "db"},
		},
	}}

	g.Load([]unstructured.Unstructured{sm, svc})

	smRef := ObjectRef{Group: "monitoring.coreos.com", Kind: "ServiceMonitor", Namespace: "default", Name: "mon"}
	if deps := g.DependenciesOf(smRef); len(deps) != 0 {
		t.Fatalf("mismatched selector should produce 0 edges, got %d: %v", len(deps), deps)
	}
}
