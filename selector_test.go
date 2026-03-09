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

func TestSelectorResolver_ServiceToPod(t *testing.T) {
	g := New(WithResolver(NewSelectorResolver()))

	svc := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{
			"name": "web-svc", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"selector": map[string]interface{}{
				"app": "web",
			},
		},
	}}

	matchPod := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]interface{}{
			"name": "web-1", "namespace": "default",
			"labels": map[string]interface{}{"app": "web"},
		},
	}}

	noMatchPod := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]interface{}{
			"name": "db-1", "namespace": "default",
			"labels": map[string]interface{}{"app": "db"},
		},
	}}

	g.Load([]unstructured.Unstructured{svc, matchPod, noMatchPod})

	svcRef := ObjectRef{Kind: "Service", Namespace: "default", Name: "web-svc"}
	deps := g.DependenciesOf(svcRef)
	if len(deps) != 1 {
		t.Fatalf("expected 1 dep, got %d", len(deps))
	}
	if deps[0].To.Name != "web-1" {
		t.Fatalf("expected web-1, got %s", deps[0].To.Name)
	}
	if deps[0].Resolver != "selector" {
		t.Fatalf("expected resolver 'selector', got '%s'", deps[0].Resolver)
	}
}
