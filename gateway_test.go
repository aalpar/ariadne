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

func TestGatewayAPI_HTTPRouteBackendRef(t *testing.T) {
	g := New(WithResolver(NewGatewayAPIResolver()))

	objs := []unstructured.Unstructured{
		// Service referenced by backendRef
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "Service",
			"metadata": map[string]interface{}{
				"name": "web-svc", "namespace": "default",
			},
		}},
		// HTTPRoute with backendRef pointing to the Service
		{Object: map[string]interface{}{
			"apiVersion": "gateway.networking.k8s.io/v1", "kind": "HTTPRoute",
			"metadata": map[string]interface{}{
				"name": "web-route", "namespace": "default",
			},
			"spec": map[string]interface{}{
				"rules": []interface{}{
					map[string]interface{}{
						"backendRefs": []interface{}{
							map[string]interface{}{
								"group": "",
								"kind":  "Service",
								"name":  "web-svc",
							},
						},
					},
				},
			},
		}},
	}

	g.Load(objs)

	routeRef := ObjectRef{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute", Namespace: "default", Name: "web-route"}
	deps := g.DependenciesOf(routeRef)
	if len(deps) != 1 {
		t.Fatalf("expected 1 HTTPRoute dep, got %d: %v", len(deps), deps)
	}

	svcRef := ObjectRef{Kind: "Service", Namespace: "default", Name: "web-svc"}
	if deps[0].To != svcRef {
		t.Fatalf("expected HTTPRoute -> Service, got %v", deps[0].To)
	}

	// Service should have HTTPRoute as a dependent.
	dependents := g.DependentsOf(svcRef)
	if len(dependents) != 1 {
		t.Fatalf("expected 1 dependent of Service, got %d: %v", len(dependents), dependents)
	}
	if dependents[0].From != routeRef {
		t.Fatalf("expected HTTPRoute as dependent, got %v", dependents[0].From)
	}
}

func TestGatewayAPI_HTTPRouteParentRef(t *testing.T) {
	g := New(WithResolver(NewGatewayAPIResolver()))

	objs := []unstructured.Unstructured{
		// Gateway
		{Object: map[string]interface{}{
			"apiVersion": "gateway.networking.k8s.io/v1", "kind": "Gateway",
			"metadata": map[string]interface{}{
				"name": "main-gw", "namespace": "default",
			},
		}},
		// HTTPRoute with parentRef pointing to the Gateway
		{Object: map[string]interface{}{
			"apiVersion": "gateway.networking.k8s.io/v1", "kind": "HTTPRoute",
			"metadata": map[string]interface{}{
				"name": "web-route", "namespace": "default",
			},
			"spec": map[string]interface{}{
				"parentRefs": []interface{}{
					map[string]interface{}{
						"group": "gateway.networking.k8s.io",
						"kind":  "Gateway",
						"name":  "main-gw",
					},
				},
			},
		}},
	}

	g.Load(objs)

	routeRef := ObjectRef{Group: "gateway.networking.k8s.io", Kind: "HTTPRoute", Namespace: "default", Name: "web-route"}
	gwRef := ObjectRef{Group: "gateway.networking.k8s.io", Kind: "Gateway", Namespace: "default", Name: "main-gw"}

	deps := g.DependenciesOf(routeRef)
	if len(deps) != 1 {
		t.Fatalf("expected 1 HTTPRoute dep, got %d: %v", len(deps), deps)
	}
	if deps[0].To != gwRef {
		t.Fatalf("expected HTTPRoute -> Gateway, got %v", deps[0].To)
	}

	// Test reverse: add Gateway first via Add, then HTTPRoute.
	g2 := New(WithResolver(NewGatewayAPIResolver()))

	gw := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "gateway.networking.k8s.io/v1", "kind": "Gateway",
		"metadata": map[string]interface{}{
			"name": "main-gw", "namespace": "default",
		},
	}}
	g2.Add(gw)

	// No edge yet — HTTPRoute isn't in the graph.
	if len(g2.DependentsOf(gwRef)) != 0 {
		t.Fatal("expected 0 dependents of Gateway before HTTPRoute is added")
	}

	route := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "gateway.networking.k8s.io/v1", "kind": "HTTPRoute",
		"metadata": map[string]interface{}{
			"name": "web-route", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"parentRefs": []interface{}{
				map[string]interface{}{
					"group": "gateway.networking.k8s.io",
					"kind":  "Gateway",
					"name":  "main-gw",
				},
			},
		},
	}}
	g2.Add(route)

	deps2 := g2.DependenciesOf(routeRef)
	if len(deps2) != 1 {
		t.Fatalf("expected 1 HTTPRoute dep after Add, got %d: %v", len(deps2), deps2)
	}
	if deps2[0].To != gwRef {
		t.Fatalf("expected HTTPRoute -> Gateway after Add, got %v", deps2[0].To)
	}
}

func TestGatewayAPI_GatewayClassName(t *testing.T) {
	g := New(WithResolver(NewGatewayAPIResolver()))

	objs := []unstructured.Unstructured{
		// GatewayClass (cluster-scoped, no namespace)
		{Object: map[string]interface{}{
			"apiVersion": "gateway.networking.k8s.io/v1", "kind": "GatewayClass",
			"metadata": map[string]interface{}{
				"name": "istio",
			},
		}},
		// Gateway referencing the GatewayClass
		{Object: map[string]interface{}{
			"apiVersion": "gateway.networking.k8s.io/v1", "kind": "Gateway",
			"metadata": map[string]interface{}{
				"name": "main-gw", "namespace": "default",
			},
			"spec": map[string]interface{}{
				"gatewayClassName": "istio",
			},
		}},
	}

	g.Load(objs)

	gwRef := ObjectRef{Group: "gateway.networking.k8s.io", Kind: "Gateway", Namespace: "default", Name: "main-gw"}
	deps := g.DependenciesOf(gwRef)
	if len(deps) != 1 {
		t.Fatalf("expected 1 Gateway dep, got %d: %v", len(deps), deps)
	}

	gcRef := ObjectRef{Group: "gateway.networking.k8s.io", Kind: "GatewayClass", Name: "istio"}
	if deps[0].To != gcRef {
		t.Fatalf("expected Gateway -> GatewayClass, got %v", deps[0].To)
	}

	// GatewayClass should have Gateway as a dependent.
	dependents := g.DependentsOf(gcRef)
	if len(dependents) != 1 {
		t.Fatalf("expected 1 dependent of GatewayClass, got %d: %v", len(dependents), dependents)
	}
	if dependents[0].From != gwRef {
		t.Fatalf("expected Gateway as dependent, got %v", dependents[0].From)
	}
}
