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

func TestArgoCD_ApplicationRefs(t *testing.T) {
	g := New(WithResolver(NewArgoCDResolver()))

	app := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "argoproj.io/v1alpha1", "kind": "Application",
		"metadata": map[string]interface{}{
			"name": "web-app", "namespace": "argocd",
		},
		"spec": map[string]interface{}{
			"project": "default",
			"destination": map[string]interface{}{
				"namespace": "production",
			},
		},
	}}
	ns := newCoreObj("Namespace", "", "production")
	proj := newObj("argoproj.io", "v1alpha1", "AppProject", "argocd", "default")

	g.Load([]unstructured.Unstructured{app, ns, proj})

	appRef := ObjectRef{Group: "argoproj.io", Kind: "Application", Namespace: "argocd", Name: "web-app"}
	nsRef := ObjectRef{Kind: "Namespace", Name: "production"}
	projRef := ObjectRef{Group: "argoproj.io", Kind: "AppProject", Namespace: "argocd", Name: "default"}

	deps := g.DependenciesOf(appRef)
	if len(deps) != 2 {
		t.Fatalf("expected 2 dependencies, got %d: %v", len(deps), deps)
	}

	foundNS, foundProj := false, false
	for _, e := range deps {
		switch {
		case e.To == nsRef:
			foundNS = true
			if e.Type != EdgeNameRef {
				t.Errorf("namespace edge: expected EdgeNameRef, got %v", e.Type)
			}
			if e.Field != "spec.destination.namespace" {
				t.Errorf("namespace edge: expected field spec.destination.namespace, got %s", e.Field)
			}
		case e.To == projRef:
			foundProj = true
			if e.Type != EdgeLocalNameRef {
				t.Errorf("project edge: expected EdgeLocalNameRef, got %v", e.Type)
			}
			if e.Field != "spec.project" {
				t.Errorf("project edge: expected field spec.project, got %s", e.Field)
			}
		default:
			t.Errorf("unexpected edge: %+v", e)
		}
	}

	if !foundNS {
		t.Error("missing Application -> Namespace edge")
	}
	if !foundProj {
		t.Error("missing Application -> AppProject edge")
	}

	// Verify reverse lookups.
	nsDeps := g.DependentsOf(nsRef)
	if len(nsDeps) != 1 || nsDeps[0].From != appRef {
		t.Errorf("expected Namespace to have 1 dependent (Application), got %v", nsDeps)
	}
	projDeps := g.DependentsOf(projRef)
	if len(projDeps) != 1 || projDeps[0].From != appRef {
		t.Errorf("expected AppProject to have 1 dependent (Application), got %v", projDeps)
	}
}

func TestArgoCD_ReverseAdd(t *testing.T) {
	g := New(WithResolver(NewArgoCDResolver()))

	// Add Application first — no targets exist yet, so no edges.
	app := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "argoproj.io/v1alpha1", "kind": "Application",
		"metadata": map[string]interface{}{
			"name": "web-app", "namespace": "argocd",
		},
		"spec": map[string]interface{}{
			"project": "default",
			"destination": map[string]interface{}{
				"namespace": "production",
			},
		},
	}}
	g.Add(app)

	appRef := ObjectRef{Group: "argoproj.io", Kind: "Application", Namespace: "argocd", Name: "web-app"}
	if deps := g.DependenciesOf(appRef); len(deps) != 0 {
		t.Fatalf("expected 0 dependencies before targets exist, got %d", len(deps))
	}

	// Add Namespace — reverse resolution should create the edge.
	ns := newCoreObj("Namespace", "", "production")
	g.Add(ns)

	nsRef := ObjectRef{Kind: "Namespace", Name: "production"}
	deps := g.DependenciesOf(appRef)
	if len(deps) != 1 {
		t.Fatalf("expected 1 dependency after adding Namespace, got %d: %v", len(deps), deps)
	}
	if deps[0].To != nsRef {
		t.Errorf("expected edge to Namespace, got %+v", deps[0])
	}
	if deps[0].Type != EdgeNameRef {
		t.Errorf("expected EdgeNameRef, got %v", deps[0].Type)
	}
}
