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
	sa := newCoreObj("ServiceAccount", "default", "my-sa")

	edges := ResolveAll([]unstructured.Unstructured{pod, sa}, NewStructuralResolver())

	// The edge Pod->SA should appear exactly once, even though both
	// objects are resolved (forward from Pod, reverse from SA).
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

	svc := newCoreObj("Service", "default", "web-svc")
	unstructured.SetNestedStringMap(svc.Object, map[string]string{
		"app": "web",
	}, "spec", "selector")

	// Pod has label matching Service selector.
	pod.SetLabels(map[string]string{"app": "web"})

	edges := ResolveAll(
		[]unstructured.Unstructured{pod, svc},
		NewStructuralResolver(),
		NewSelectorResolver(),
	)

	var hasRef, hasSelector bool
	for _, e := range edges {
		if e.Type == EdgeRef && e.To.Kind == "ServiceAccount" {
			hasRef = true
		}
		if e.Type == EdgeLabelSelector && e.From.Kind == "Service" {
			hasSelector = true
		}
	}
	if !hasRef {
		t.Error("expected ref edge from structural resolver")
	}
	if !hasSelector {
		t.Error("expected selector edge from selector resolver")
	}
}
