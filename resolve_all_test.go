package ariadne

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestResolveAll_detectsMissingTarget(t *testing.T) {
	pod := newCoreObj("Pod", "default", "web")
	unstructured.SetNestedField(pod.Object, "missing-sa", "spec", "serviceAccountName")

	edges := ResolveAll([]unstructured.Unstructured{pod}, NewStructuralResolver())

	var found bool
	for _, e := range edges {
		if e.To.Kind == "ServiceAccount" && e.To.Name == "missing-sa" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected edge to missing ServiceAccount, got edges: %v", edges)
	}
}

func TestResolveAll_includesExistingTarget(t *testing.T) {
	pod := newCoreObj("Pod", "default", "web")
	unstructured.SetNestedField(pod.Object, "my-sa", "spec", "serviceAccountName")
	sa := newCoreObj("ServiceAccount", "default", "my-sa")

	edges := ResolveAll([]unstructured.Unstructured{pod, sa}, NewStructuralResolver())

	var found bool
	for _, e := range edges {
		if e.To.Kind == "ServiceAccount" && e.To.Name == "my-sa" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected edge to existing ServiceAccount, got edges: %v", edges)
	}
}

func TestResolveAll_includesOwnerRefs(t *testing.T) {
	// ResolveAll returns raw edges — ownerRef filtering is the caller's job.
	pod := newCoreObj("Pod", "default", "web")
	pod.SetOwnerReferences([]metav1.OwnerReference{{
		APIVersion: "apps/v1",
		Kind:       "ReplicaSet",
		Name:       "web-abc",
	}})

	edges := ResolveAll([]unstructured.Unstructured{pod}, NewStructuralResolver())

	var found bool
	for _, e := range edges {
		if e.Field == "metadata.ownerReferences" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected ownerRef edge in raw output (caller filters)")
	}
}

func TestResolveAll_deduplicates(t *testing.T) {
	pod := newCoreObj("Pod", "default", "web")
	unstructured.SetNestedField(pod.Object, "my-sa", "spec", "serviceAccountName")

	edges := ResolveAll([]unstructured.Unstructured{pod}, NewStructuralResolver())

	count := 0
	for _, e := range edges {
		if e.From.Kind == "Pod" && e.To.Kind == "ServiceAccount" && e.To.Name == "my-sa" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("expected exactly 1 Pod->SA edge, got %d; all edges: %v", count, edges)
	}
}

func TestResolveAll_multipleResolvers(t *testing.T) {
	pod := newCoreObj("Pod", "default", "web")
	unstructured.SetNestedField(pod.Object, "my-sa", "spec", "serviceAccountName")

	event := newCoreObj("Event", "default", "web.12345")
	unstructured.SetNestedField(event.Object, "v1", "involvedObject", "apiVersion")
	unstructured.SetNestedField(event.Object, "Pod", "involvedObject", "kind")
	unstructured.SetNestedField(event.Object, "default", "involvedObject", "namespace")
	unstructured.SetNestedField(event.Object, "web", "involvedObject", "name")

	edges := ResolveAll(
		[]unstructured.Unstructured{pod, event},
		NewStructuralResolver(),
		NewEventResolver(),
	)

	var hasRef, hasEvent bool
	for _, e := range edges {
		if e.Type == EdgeRef && e.To.Kind == "ServiceAccount" {
			hasRef = true
		}
		if e.Type == EdgeEvent && e.To.Kind == "Pod" {
			hasEvent = true
		}
	}
	if !hasRef {
		t.Error("expected ref edge from structural resolver")
	}
	if !hasEvent {
		t.Error("expected event edge from event resolver")
	}
}

func TestResolveAll_clusterScopedNamespace(t *testing.T) {
	pod := newCoreObj("Pod", "default", "web")
	unstructured.SetNestedField(pod.Object, "node-1", "spec", "nodeName")

	edges := ResolveAll([]unstructured.Unstructured{pod}, NewStructuralResolver())

	for _, e := range edges {
		if e.To.Kind == "Node" {
			if e.To.Namespace != "" {
				t.Fatalf("Node is cluster-scoped; expected empty namespace, got %q", e.To.Namespace)
			}
			return
		}
	}
	t.Error("expected edge to Node")
}
