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

package main

import (
	"bytes"
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func newObj(group, version, kind, ns, name string) unstructured.Unstructured {
	obj := unstructured.Unstructured{}
	if group == "" {
		obj.SetAPIVersion(version)
	} else {
		obj.SetAPIVersion(group + "/" + version)
	}
	obj.SetKind(kind)
	obj.SetNamespace(ns)
	obj.SetName(name)
	return obj
}

func TestLint_detectsDanglingRef(t *testing.T) {
	pod := newObj("", "v1", "Pod", "default", "web")
	unstructured.SetNestedField(pod.Object, "missing-sa", "spec", "serviceAccountName")

	var buf bytes.Buffer
	count := lint([]unstructured.Unstructured{pod}, &buf)

	if count == 0 {
		t.Fatal("expected dangling reference, got count=0")
	}
	if buf.Len() == 0 {
		t.Fatal("expected non-empty output")
	}
	t.Logf("output:\n%s", buf.String())
}

func TestLint_noDanglingWhenComplete(t *testing.T) {
	pod := newObj("", "v1", "Pod", "default", "web")
	unstructured.SetNestedField(pod.Object, "my-sa", "spec", "serviceAccountName")

	sa := newObj("", "v1", "ServiceAccount", "default", "my-sa")

	var buf bytes.Buffer
	count := lint([]unstructured.Unstructured{pod, sa}, &buf)

	if count != 0 {
		t.Fatalf("expected 0 dangling references, got %d:\n%s", count, buf.String())
	}
}

func TestLint_filtersOwnerRefs(t *testing.T) {
	pod := newObj("", "v1", "Pod", "default", "web-xyz")
	pod.SetOwnerReferences([]metav1.OwnerReference{{
		APIVersion: "apps/v1",
		Kind:       "ReplicaSet",
		Name:       "web-abc123",
	}})

	var buf bytes.Buffer
	count := lint([]unstructured.Unstructured{pod}, &buf)

	if count != 0 {
		t.Fatalf("expected ownerRef edges to be filtered, got %d:\n%s", count, buf.String())
	}
}

func TestLint_filtersEvents(t *testing.T) {
	event := newObj("", "v1", "Event", "default", "web.12345")
	unstructured.SetNestedField(event.Object, "v1", "involvedObject", "apiVersion")
	unstructured.SetNestedField(event.Object, "Pod", "involvedObject", "kind")
	unstructured.SetNestedField(event.Object, "default", "involvedObject", "namespace")
	unstructured.SetNestedField(event.Object, "web", "involvedObject", "name")

	var buf bytes.Buffer
	count := lint([]unstructured.Unstructured{event}, &buf)

	if count != 0 {
		t.Fatalf("expected event edges to be filtered, got %d:\n%s", count, buf.String())
	}
}

func TestLint_integration(t *testing.T) {
	// Pod with two volumes: one referencing a ConfigMap, one referencing a Secret.
	pod := newObj("", "v1", "Pod", "default", "web")
	unstructured.SetNestedSlice(pod.Object, []interface{}{
		map[string]interface{}{
			"name": "config-vol",
			"configMap": map[string]interface{}{
				"name": "app-config",
			},
		},
		map[string]interface{}{
			"name": "secret-vol",
			"secret": map[string]interface{}{
				"secretName": "app-secret",
			},
		},
	}, "spec", "volumes")

	// ConfigMap that satisfies the first volume reference.
	cm := newObj("", "v1", "ConfigMap", "default", "app-config")

	// Secret "app-secret" is deliberately absent — lint should catch this.

	// Service with a selector that matches zero pods. Selector non-matches
	// are NOT dangling references; they are valid (a selector can match nothing).
	svc := newObj("", "v1", "Service", "default", "web-svc")
	unstructured.SetNestedStringMap(svc.Object, map[string]string{"app": "web"}, "spec", "selector")

	var buf bytes.Buffer
	count := lint([]unstructured.Unstructured{pod, cm, svc}, &buf)
	output := buf.String()
	t.Logf("output:\n%s", output)

	if count < 1 {
		t.Fatal("expected at least one finding, got count=0")
	}
	if !strings.Contains(output, "app-secret") {
		t.Fatalf("expected output to mention missing Secret 'app-secret', got:\n%s", output)
	}
	if strings.Contains(output, "web-svc") {
		t.Fatalf("Service selector non-match should not be reported as dangling, got:\n%s", output)
	}
}
