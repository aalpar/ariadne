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

func TestParseIstioHost(t *testing.T) {
	tests := []struct {
		host      string
		sourceNS  string
		wantName  string
		wantNS    string
		wantOK    bool
	}{
		{"reviews", "default", "reviews", "default", true},
		{"reviews.prod", "default", "reviews", "prod", true},
		{"reviews.prod.svc", "default", "reviews", "prod", true},
		{"reviews.prod.svc.cluster.local", "default", "reviews", "prod", true},
		{"api.example.com", "default", "", "", false},
		{"deep.nested.example.com", "default", "", "", false},
		{"", "default", "", "", false},
		// 4-part non-FQDN: not a valid K8s DNS pattern
		{"a.b.c.d", "default", "", "", false},
		// Single segment with different source namespace
		{"frontend", "staging", "frontend", "staging", true},
	}

	for _, tt := range tests {
		name, ns, ok := parseIstioHost(tt.host, tt.sourceNS)
		if ok != tt.wantOK {
			t.Errorf("parseIstioHost(%q, %q): ok=%v, want %v", tt.host, tt.sourceNS, ok, tt.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if name != tt.wantName || ns != tt.wantNS {
			t.Errorf("parseIstioHost(%q, %q) = (%q, %q), want (%q, %q)",
				tt.host, tt.sourceNS, name, ns, tt.wantName, tt.wantNS)
		}
	}
}

func TestIstio_VirtualServiceForward(t *testing.T) {
	g := New(WithResolver(NewIstioResolver()))

	vs := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "networking.istio.io/v1", "kind": "VirtualService",
		"metadata": map[string]interface{}{
			"name": "reviews-route", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"http": []interface{}{
				map[string]interface{}{
					"route": []interface{}{
						map[string]interface{}{
							"destination": map[string]interface{}{
								"host": "reviews",
							},
						},
					},
				},
			},
		},
	}}

	svc := newCoreObj("Service", "default", "reviews")

	g.Load([]unstructured.Unstructured{vs, svc})

	vsRef := ObjectRef{Group: "networking.istio.io", Kind: "VirtualService", Namespace: "default", Name: "reviews-route"}
	deps := g.DependenciesOf(vsRef)
	if len(deps) != 1 {
		t.Fatalf("expected 1 dependency, got %d: %v", len(deps), deps)
	}
	if deps[0].To.Kind != "Service" || deps[0].To.Name != "reviews" {
		t.Errorf("expected edge to Service/reviews, got %v", deps[0].To)
	}
	if deps[0].To.Namespace != "default" {
		t.Errorf("expected same-namespace resolution, got ns=%q", deps[0].To.Namespace)
	}
	if deps[0].Type != EdgeRef {
		t.Errorf("expected EdgeRef, got %s", deps[0].Type)
	}
	if deps[0].Resolver != "istio" {
		t.Errorf("expected resolver 'istio', got %q", deps[0].Resolver)
	}
	if deps[0].Field != "spec.http[*].route[*].destination.host" {
		t.Errorf("unexpected field: %q", deps[0].Field)
	}
}

func TestIstio_VirtualServiceCrossNamespace(t *testing.T) {
	g := New(WithResolver(NewIstioResolver()))

	vs := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "networking.istio.io/v1", "kind": "VirtualService",
		"metadata": map[string]interface{}{
			"name": "cross-ns-route", "namespace": "frontend",
		},
		"spec": map[string]interface{}{
			"http": []interface{}{
				map[string]interface{}{
					"route": []interface{}{
						map[string]interface{}{
							"destination": map[string]interface{}{
								"host": "reviews.backend.svc.cluster.local",
							},
						},
					},
				},
			},
		},
	}}

	svc := newCoreObj("Service", "backend", "reviews")

	g.Load([]unstructured.Unstructured{vs, svc})

	vsRef := ObjectRef{Group: "networking.istio.io", Kind: "VirtualService", Namespace: "frontend", Name: "cross-ns-route"}
	deps := g.DependenciesOf(vsRef)
	if len(deps) != 1 {
		t.Fatalf("expected 1 dependency, got %d: %v", len(deps), deps)
	}
	if deps[0].To.Namespace != "backend" {
		t.Errorf("expected cross-namespace resolution to 'backend', got %q", deps[0].To.Namespace)
	}
}

func TestIstio_VirtualServiceMultipleRoutes(t *testing.T) {
	g := New(WithResolver(NewIstioResolver()))

	vs := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "networking.istio.io/v1", "kind": "VirtualService",
		"metadata": map[string]interface{}{
			"name": "multi-route", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"http": []interface{}{
				map[string]interface{}{
					"route": []interface{}{
						map[string]interface{}{
							"destination": map[string]interface{}{"host": "reviews"},
						},
						map[string]interface{}{
							"destination": map[string]interface{}{"host": "ratings"},
						},
					},
				},
			},
			"tcp": []interface{}{
				map[string]interface{}{
					"route": []interface{}{
						map[string]interface{}{
							"destination": map[string]interface{}{"host": "db"},
						},
					},
				},
			},
		},
	}}

	reviews := newCoreObj("Service", "default", "reviews")
	ratings := newCoreObj("Service", "default", "ratings")
	db := newCoreObj("Service", "default", "db")

	g.Load([]unstructured.Unstructured{vs, reviews, ratings, db})

	vsRef := ObjectRef{Group: "networking.istio.io", Kind: "VirtualService", Namespace: "default", Name: "multi-route"}
	deps := g.DependenciesOf(vsRef)
	if len(deps) != 3 {
		t.Fatalf("expected 3 dependencies, got %d: %v", len(deps), deps)
	}

	names := map[string]bool{}
	for _, e := range deps {
		names[e.To.Name] = true
	}
	for _, want := range []string{"reviews", "ratings", "db"} {
		if !names[want] {
			t.Errorf("missing edge to Service/%s", want)
		}
	}
}

func TestIstio_DestinationRuleForward(t *testing.T) {
	g := New(WithResolver(NewIstioResolver()))

	dr := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "networking.istio.io/v1", "kind": "DestinationRule",
		"metadata": map[string]interface{}{
			"name": "reviews-dr", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"host": "reviews.prod.svc",
		},
	}}

	svc := newCoreObj("Service", "prod", "reviews")

	g.Load([]unstructured.Unstructured{dr, svc})

	drRef := ObjectRef{Group: "networking.istio.io", Kind: "DestinationRule", Namespace: "default", Name: "reviews-dr"}
	deps := g.DependenciesOf(drRef)
	if len(deps) != 1 {
		t.Fatalf("expected 1 dependency, got %d: %v", len(deps), deps)
	}
	if deps[0].To.Name != "reviews" || deps[0].To.Namespace != "prod" {
		t.Errorf("expected reviews in prod, got %v", deps[0].To)
	}
	if deps[0].Field != "spec.host" {
		t.Errorf("unexpected field: %q", deps[0].Field)
	}
}

func TestIstio_ExternalHostIgnored(t *testing.T) {
	g := New(WithResolver(NewIstioResolver()))

	vs := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "networking.istio.io/v1", "kind": "VirtualService",
		"metadata": map[string]interface{}{
			"name": "external-route", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"http": []interface{}{
				map[string]interface{}{
					"route": []interface{}{
						map[string]interface{}{
							"destination": map[string]interface{}{
								"host": "api.example.com",
							},
						},
					},
				},
			},
		},
	}}

	g.Load([]unstructured.Unstructured{vs})

	vsRef := ObjectRef{Group: "networking.istio.io", Kind: "VirtualService", Namespace: "default", Name: "external-route"}
	deps := g.DependenciesOf(vsRef)
	if len(deps) != 0 {
		t.Fatalf("expected 0 dependencies for external host, got %d: %v", len(deps), deps)
	}
}

func TestIstio_ReverseAdd(t *testing.T) {
	g := New(WithResolver(NewIstioResolver()))

	vs := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "networking.istio.io/v1", "kind": "VirtualService",
		"metadata": map[string]interface{}{
			"name": "reviews-route", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"http": []interface{}{
				map[string]interface{}{
					"route": []interface{}{
						map[string]interface{}{
							"destination": map[string]interface{}{
								"host": "reviews",
							},
						},
					},
				},
			},
		},
	}}

	// Add VirtualService first — no Service exists yet.
	g.Add(vs)

	vsRef := ObjectRef{Group: "networking.istio.io", Kind: "VirtualService", Namespace: "default", Name: "reviews-route"}
	if deps := g.DependenciesOf(vsRef); len(deps) != 0 {
		t.Fatalf("expected 0 deps before Service, got %d", len(deps))
	}

	// Add Service — reverse resolution creates the edge.
	svc := newCoreObj("Service", "default", "reviews")
	g.Add(svc)

	deps := g.DependenciesOf(vsRef)
	if len(deps) != 1 {
		t.Fatalf("expected 1 dependency after adding Service, got %d: %v", len(deps), deps)
	}
	if deps[0].To.Kind != "Service" || deps[0].To.Name != "reviews" {
		t.Errorf("expected edge to Service/reviews, got %v", deps[0].To)
	}
}

func TestIstio_Extract(t *testing.T) {
	r := NewIstioResolver()

	vs := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "networking.istio.io/v1", "kind": "VirtualService",
		"metadata": map[string]interface{}{
			"name": "reviews-route", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"http": []interface{}{
				map[string]interface{}{
					"route": []interface{}{
						map[string]interface{}{
							"destination": map[string]interface{}{
								"host": "reviews",
							},
						},
						map[string]interface{}{
							"destination": map[string]interface{}{
								"host": "ratings.prod.svc.cluster.local",
							},
						},
					},
				},
			},
		},
	}}

	edges := r.Extract(vs)
	if len(edges) != 2 {
		t.Fatalf("expected 2 extract edges, got %d: %v", len(edges), edges)
	}

	found := map[string]string{}
	for _, e := range edges {
		found[e.To.Name] = e.To.Namespace
	}
	if ns, ok := found["reviews"]; !ok || ns != "default" {
		t.Errorf("missing or wrong reviews extract edge: ns=%q, ok=%v", ns, ok)
	}
	if ns, ok := found["ratings"]; !ok || ns != "prod" {
		t.Errorf("missing or wrong ratings extract edge: ns=%q, ok=%v", ns, ok)
	}
}

func TestIstio_AuthorizationPolicy(t *testing.T) {
	g := New(WithResolver(NewIstioResolver()))

	policy := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "security.istio.io/v1", "kind": "AuthorizationPolicy",
		"metadata": map[string]interface{}{
			"name": "deny-all", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"selector": map[string]interface{}{
				"matchLabels": map[string]interface{}{
					"app": "reviews",
				},
			},
		},
	}}

	pod := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]interface{}{
			"name": "reviews-abc", "namespace": "default",
			"labels": map[string]interface{}{
				"app": "reviews",
			},
		},
	}}

	nonMatchingPod := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]interface{}{
			"name": "ratings-xyz", "namespace": "default",
			"labels": map[string]interface{}{
				"app": "ratings",
			},
		},
	}}

	g.Load([]unstructured.Unstructured{policy, pod, nonMatchingPod})

	policyRef := ObjectRef{Group: "security.istio.io", Kind: "AuthorizationPolicy", Namespace: "default", Name: "deny-all"}
	deps := g.DependenciesOf(policyRef)
	if len(deps) != 1 {
		t.Fatalf("expected 1 dependency, got %d: %v", len(deps), deps)
	}
	if deps[0].To.Name != "reviews-abc" {
		t.Errorf("expected edge to reviews-abc, got %v", deps[0].To)
	}
	if deps[0].Type != EdgeLabelSelector {
		t.Errorf("expected EdgeLabelSelector, got %s", deps[0].Type)
	}
}
