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

func TestCrossplane_ProviderConfigRef(t *testing.T) {
	g := New(WithResolver(NewCrossplaneResolver(
		ManagedResource{Group: "database.aws.crossplane.io", Kind: "RDSInstance"},
	)))

	rds := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "database.aws.crossplane.io/v1alpha1", "kind": "RDSInstance",
		"metadata": map[string]interface{}{
			"name": "my-db", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"providerConfigRef": map[string]interface{}{
				"name": "aws-provider",
			},
		},
	}}
	pc := newObj("pkg.crossplane.io", "v1", "ProviderConfig", "", "aws-provider")

	g.Load([]unstructured.Unstructured{rds, pc})

	rdsRef := ObjectRef{
		Group: "database.aws.crossplane.io", Kind: "RDSInstance",
		Namespace: "default", Name: "my-db",
	}
	edges := g.DependenciesOf(rdsRef)
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}

	e := edges[0]
	if e.To.Group != "pkg.crossplane.io" || e.To.Kind != "ProviderConfig" || e.To.Name != "aws-provider" {
		t.Fatalf("unexpected target: %v", e.To)
	}
	if e.Resolver != "crossplane" {
		t.Fatalf("expected resolver 'crossplane', got %q", e.Resolver)
	}
	if e.Field != "spec.providerConfigRef.name" {
		t.Fatalf("expected field 'spec.providerConfigRef.name', got %q", e.Field)
	}
}

func TestCrossplane_CompositeTypeRef(t *testing.T) {
	g := New(WithResolver(NewCrossplaneResolver()))

	composition := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apiextensions.crossplane.io/v1", "kind": "Composition",
		"metadata": map[string]interface{}{
			"name": "mydatabase-composition",
		},
		"spec": map[string]interface{}{
			"compositeTypeRef": map[string]interface{}{
				"apiGroup": "myapp.example.org",
				"kind":     "XMyDatabase",
			},
		},
	}}
	db1 := newObj("myapp.example.org", "v1", "XMyDatabase", "", "db-instance-1")
	db2 := newObj("myapp.example.org", "v1", "XMyDatabase", "", "db-instance-2")

	g.Load([]unstructured.Unstructured{composition, db1, db2})

	compRef := ObjectRef{
		Group: "apiextensions.crossplane.io", Kind: "Composition",
		Name: "mydatabase-composition",
	}
	edges := g.DependenciesOf(compRef)
	if len(edges) != 2 {
		t.Fatalf("expected 2 edges, got %d", len(edges))
	}

	targets := map[string]bool{}
	for _, e := range edges {
		if e.Type != EdgeCustom {
			t.Fatalf("expected EdgeCustom, got %v", e.Type)
		}
		if e.Resolver != "crossplane" {
			t.Fatalf("expected resolver 'crossplane', got %q", e.Resolver)
		}
		if e.Field != "spec.compositeTypeRef" {
			t.Fatalf("expected field 'spec.compositeTypeRef', got %q", e.Field)
		}
		targets[e.To.Name] = true
	}
	if !targets["db-instance-1"] || !targets["db-instance-2"] {
		t.Fatalf("expected edges to both db instances, got %v", targets)
	}
}

func TestCrossplane_CompositeReverseAdd(t *testing.T) {
	g := New(WithResolver(NewCrossplaneResolver()))

	composition := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apiextensions.crossplane.io/v1", "kind": "Composition",
		"metadata": map[string]interface{}{
			"name": "mydatabase-composition",
		},
		"spec": map[string]interface{}{
			"compositeTypeRef": map[string]interface{}{
				"apiGroup": "myapp.example.org",
				"kind":     "XMyDatabase",
			},
		},
	}}
	g.Add(composition)

	db := newObj("myapp.example.org", "v1", "XMyDatabase", "", "db-instance-1")
	g.Add(db)

	compRef := ObjectRef{
		Group: "apiextensions.crossplane.io", Kind: "Composition",
		Name: "mydatabase-composition",
	}
	edges := g.DependenciesOf(compRef)
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}

	e := edges[0]
	if e.From != compRef {
		t.Fatalf("expected edge from composition, got %v", e.From)
	}
	if e.To.Group != "myapp.example.org" || e.To.Kind != "XMyDatabase" || e.To.Name != "db-instance-1" {
		t.Fatalf("unexpected target: %v", e.To)
	}
	if e.Type != EdgeCustom {
		t.Fatalf("expected EdgeCustom, got %v", e.Type)
	}
	if e.Resolver != "crossplane" {
		t.Fatalf("expected resolver 'crossplane', got %q", e.Resolver)
	}
}
