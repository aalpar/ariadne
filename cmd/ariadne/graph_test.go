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

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestGraph_dotOutput(t *testing.T) {
	pod := newObj("", "v1", "Pod", "default", "web")
	unstructured.SetNestedField(pod.Object, "my-sa", "spec", "serviceAccountName")
	sa := newObj("", "v1", "ServiceAccount", "default", "my-sa")

	var buf bytes.Buffer
	err := graph([]unstructured.Unstructured{pod, sa}, "dot", false, &buf)
	if err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.HasPrefix(out, "digraph ariadne {") {
		t.Fatalf("expected DOT output, got:\n%s", out)
	}
	if !strings.Contains(out, "my-sa") {
		t.Fatalf("expected ServiceAccount node in output, got:\n%s", out)
	}
	if !strings.Contains(out, "->") {
		t.Fatalf("expected edge in output, got:\n%s", out)
	}
}

func TestGraph_jsonOutput(t *testing.T) {
	pod := newObj("", "v1", "Pod", "default", "web")
	unstructured.SetNestedField(pod.Object, "my-sa", "spec", "serviceAccountName")
	sa := newObj("", "v1", "ServiceAccount", "default", "my-sa")

	var buf bytes.Buffer
	err := graph([]unstructured.Unstructured{pod, sa}, "json", false, &buf)
	if err != nil {
		t.Fatal(err)
	}

	out := buf.String()
	if !strings.Contains(out, `"nodes"`) {
		t.Fatalf("expected JSON with nodes, got:\n%s", out)
	}
	if !strings.Contains(out, `"edges"`) {
		t.Fatalf("expected JSON with edges, got:\n%s", out)
	}
}

func TestGraph_invalidFormat(t *testing.T) {
	pod := newObj("", "v1", "Pod", "default", "web")

	var buf bytes.Buffer
	err := graph([]unstructured.Unstructured{pod}, "xml", false, &buf)
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
}
