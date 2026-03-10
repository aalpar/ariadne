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

func TestEventResolver(t *testing.T) {
	g := New(WithResolver(NewEventResolver()))

	pod := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]interface{}{
			"name": "web", "namespace": "default",
		},
	}}

	event := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Event",
		"metadata": map[string]interface{}{
			"name": "web.abc123", "namespace": "default",
		},
		"involvedObject": map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "Pod",
			"name":       "web",
			"namespace":  "default",
		},
	}}

	g.Load([]unstructured.Unstructured{pod, event})

	podRef := ObjectRef{Kind: "Pod", Namespace: "default", Name: "web"}
	deps := g.DependentsOf(podRef)
	if len(deps) != 1 {
		t.Fatalf("expected 1 event dependent, got %d", len(deps))
	}
	if deps[0].Resolver != "event" {
		t.Fatalf("expected resolver 'event', got '%s'", deps[0].Resolver)
	}

	// Event depends on Pod, not the other way around.
	eventRef := ObjectRef{Kind: "Event", Namespace: "default", Name: "web.abc123"}
	eventDeps := g.DependenciesOf(eventRef)
	if len(eventDeps) != 1 {
		t.Fatalf("expected event to depend on pod, got %d deps", len(eventDeps))
	}
	if eventDeps[0].To != podRef {
		t.Fatalf("expected event dep target to be pod, got %v", eventDeps[0].To)
	}
}

func TestEventResolver_Extract(t *testing.T) {
	r := NewEventResolver()

	event := newCoreObj("Event", "default", "web.12345")
	unstructured.SetNestedField(event.Object, "v1", "involvedObject", "apiVersion")
	unstructured.SetNestedField(event.Object, "Pod", "involvedObject", "kind")
	unstructured.SetNestedField(event.Object, "default", "involvedObject", "namespace")
	unstructured.SetNestedField(event.Object, "web", "involvedObject", "name")

	edges := r.Extract(&event)
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if edges[0].To.Kind != "Pod" || edges[0].To.Name != "web" {
		t.Fatalf("unexpected target: %v", edges[0].To)
	}
	if edges[0].Type != EdgeEvent {
		t.Fatalf("expected EdgeEvent, got %v", edges[0].Type)
	}
}

func TestEventResolver_Extract_NonEvent(t *testing.T) {
	r := NewEventResolver()

	pod := newCoreObj("Pod", "default", "web")
	edges := r.Extract(&pod)
	if len(edges) != 0 {
		t.Fatalf("expected 0 edges for non-Event, got %d", len(edges))
	}
}
