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
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDecodeYAML_singleDoc(t *testing.T) {
	const doc = `apiVersion: v1
kind: ConfigMap
metadata:
  name: my-config
  namespace: default
`
	objs, errs := decodeYAML(strings.NewReader(doc))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(objs) != 1 {
		t.Fatalf("expected 1 object, got %d", len(objs))
	}
	if got := objs[0].GetName(); got != "my-config" {
		t.Errorf("expected name %q, got %q", "my-config", got)
	}
}

func TestDecodeYAML_multiDoc(t *testing.T) {
	const doc = `apiVersion: v1
kind: ConfigMap
metadata:
  name: first
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: second
`
	objs, errs := decodeYAML(strings.NewReader(doc))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(objs) != 2 {
		t.Fatalf("expected 2 objects, got %d", len(objs))
	}
	if got := objs[0].GetName(); got != "first" {
		t.Errorf("expected first object name %q, got %q", "first", got)
	}
	if got := objs[1].GetName(); got != "second" {
		t.Errorf("expected second object name %q, got %q", "second", got)
	}
}

func TestDecodeYAML_skipsEmpty(t *testing.T) {
	const doc = `apiVersion: v1
kind: ConfigMap
metadata:
  name: before
---
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: after
`
	objs, errs := decodeYAML(strings.NewReader(doc))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(objs) != 2 {
		t.Fatalf("expected 2 objects (empty doc skipped), got %d", len(objs))
	}
	if got := objs[0].GetName(); got != "before" {
		t.Errorf("expected first object name %q, got %q", "before", got)
	}
	if got := objs[1].GetName(); got != "after" {
		t.Errorf("expected second object name %q, got %q", "after", got)
	}
}

func TestDecodeYAML_invalidDoc(t *testing.T) {
	const doc = `apiVersion: v1
kind: ConfigMap
metadata:
  name: good-before
---
not: valid: yaml: [
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: good-after
`
	objs, errs := decodeYAML(strings.NewReader(doc))
	if len(errs) != 1 {
		t.Fatalf("expected 1 error, got %d: %v", len(errs), errs)
	}
	if len(objs) != 2 {
		t.Fatalf("expected 2 valid objects, got %d", len(objs))
	}
	if got := objs[0].GetName(); got != "good-before" {
		t.Errorf("expected first object name %q, got %q", "good-before", got)
	}
	if got := objs[1].GetName(); got != "good-after" {
		t.Errorf("expected second object name %q, got %q", "good-after", got)
	}
}

const testYAML = `apiVersion: v1
kind: ConfigMap
metadata:
  name: %s
`

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadSources_file(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "obj.yaml")
	writeFile(t, path, fmt.Sprintf(testYAML, "from-file"))

	objs, errs := readSources([]string{path})
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(objs) != 1 {
		t.Fatalf("expected 1 object, got %d", len(objs))
	}
	if got := objs[0].GetName(); got != "from-file" {
		t.Errorf("expected name %q, got %q", "from-file", got)
	}
}

func TestReadSources_directory(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "a.yaml"), fmt.Sprintf(testYAML, "alpha"))
	writeFile(t, filepath.Join(tmp, "b.yml"), fmt.Sprintf(testYAML, "bravo"))
	writeFile(t, filepath.Join(tmp, "c.txt"), fmt.Sprintf(testYAML, "charlie"))

	objs, errs := readSources([]string{tmp})
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(objs) != 2 {
		t.Fatalf("expected 2 objects (txt ignored), got %d", len(objs))
	}
}

func TestReadSources_recursiveDir(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "top.yaml"), fmt.Sprintf(testYAML, "top-level"))

	sub := filepath.Join(tmp, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(sub, "nested.yaml"), fmt.Sprintf(testYAML, "nested"))

	objs, errs := readSources([]string{tmp})
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(objs) != 2 {
		t.Fatalf("expected 2 objects (recursive), got %d", len(objs))
	}
}
