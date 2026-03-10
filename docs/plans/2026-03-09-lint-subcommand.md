# `ariadne lint` Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build a CLI tool that reads K8s YAML manifests and reports dangling references (edges pointing to objects not in the input set).

**Architecture:** Three files in `cmd/ariadne/`. `yaml.go` reads and decodes YAML from stdin/files/directories. `lint.go` builds the graph, walks edges, filters, and formats output. `main.go` dispatches to `lint`. Uses `k8s.io/apimachinery` for YAML decoding — zero new dependencies.

**Tech Stack:** Go, `k8s.io/apimachinery/pkg/util/yaml` (YAMLOrJSONDecoder), `k8s.io/apimachinery/pkg/apis/meta/v1/unstructured`

---

### Task 1: YAML decoding — read a single YAML stream into unstructured objects

**Files:**
- Create: `cmd/ariadne/yaml.go`
- Test: `cmd/ariadne/yaml_test.go`

**Step 1: Write the failing test**

In `cmd/ariadne/yaml_test.go`:

```go
package main

import (
	"strings"
	"testing"
)

func TestDecodeYAML_singleDoc(t *testing.T) {
	input := `apiVersion: v1
kind: ConfigMap
metadata:
  name: test
  namespace: default
`
	objs, errs := decodeYAML(strings.NewReader(input))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(objs) != 1 {
		t.Fatalf("expected 1 object, got %d", len(objs))
	}
	if objs[0].GetName() != "test" {
		t.Errorf("expected name 'test', got %q", objs[0].GetName())
	}
}

func TestDecodeYAML_multiDoc(t *testing.T) {
	input := `apiVersion: v1
kind: ConfigMap
metadata:
  name: cm1
  namespace: default
---
apiVersion: v1
kind: Secret
metadata:
  name: s1
  namespace: default
`
	objs, errs := decodeYAML(strings.NewReader(input))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(objs) != 2 {
		t.Fatalf("expected 2 objects, got %d", len(objs))
	}
}

func TestDecodeYAML_skipsEmpty(t *testing.T) {
	input := `---
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: test
  namespace: default
---
`
	objs, errs := decodeYAML(strings.NewReader(input))
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(objs) != 1 {
		t.Fatalf("expected 1 object, got %d", len(objs))
	}
}

func TestDecodeYAML_invalidDoc(t *testing.T) {
	input := `apiVersion: v1
kind: ConfigMap
metadata:
  name: good
  namespace: default
---
not: valid: yaml: {{{}}}
---
apiVersion: v1
kind: Secret
metadata:
  name: also-good
  namespace: default
`
	objs, errs := decodeYAML(strings.NewReader(input))
	if len(errs) == 0 {
		t.Fatal("expected at least one error for invalid doc")
	}
	if len(objs) != 2 {
		t.Fatalf("expected 2 valid objects, got %d", len(objs))
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/ariadne/ -run TestDecodeYAML -v`
Expected: FAIL — `decodeYAML` not defined.

**Step 3: Write minimal implementation**

In `cmd/ariadne/yaml.go`:

```go
package main

import (
	"io"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/util/yaml"
)

// decodeYAML reads a multi-document YAML stream and returns all valid
// unstructured objects. Invalid documents are collected as errors
// rather than aborting the entire stream.
func decodeYAML(r io.Reader) ([]unstructured.Unstructured, []error) {
	var objs []unstructured.Unstructured
	var errs []error

	decoder := yaml.NewYAMLOrJSONDecoder(r, 4096)
	for {
		var obj unstructured.Unstructured
		if err := decoder.Decode(&obj); err != nil {
			if err == io.EOF {
				break
			}
			errs = append(errs, err)
			continue
		}
		// Skip empty documents (--- separators with no content).
		if obj.Object == nil {
			continue
		}
		objs = append(objs, obj)
	}
	return objs, errs
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./cmd/ariadne/ -run TestDecodeYAML -v`
Expected: PASS (all four tests).

**Step 5: Commit**

```bash
git add cmd/ariadne/yaml.go cmd/ariadne/yaml_test.go
git commit -m "Add YAML decoding for lint subcommand"
```

---

### Task 2: File/directory reading — collect YAML from filesystem args

**Files:**
- Modify: `cmd/ariadne/yaml.go`
- Modify: `cmd/ariadne/yaml_test.go`

**Step 1: Write the failing test**

Append to `cmd/ariadne/yaml_test.go`:

```go
func TestReadSources_file(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.yaml")
	os.WriteFile(path, []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: from-file
  namespace: default
`), 0644)

	objs, errs := readSources([]string{path})
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(objs) != 1 {
		t.Fatalf("expected 1 object, got %d", len(objs))
	}
	if objs[0].GetName() != "from-file" {
		t.Errorf("expected name 'from-file', got %q", objs[0].GetName())
	}
}

func TestReadSources_directory(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.yaml"), []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: a
  namespace: default
`), 0644)
	os.WriteFile(filepath.Join(dir, "b.yml"), []byte(`apiVersion: v1
kind: Secret
metadata:
  name: b
  namespace: default
`), 0644)
	os.WriteFile(filepath.Join(dir, "c.txt"), []byte(`should be ignored`), 0644)

	objs, errs := readSources([]string{dir})
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(objs) != 2 {
		t.Fatalf("expected 2 objects, got %d", len(objs))
	}
}

func TestReadSources_recursiveDir(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	os.MkdirAll(sub, 0755)
	os.WriteFile(filepath.Join(dir, "top.yaml"), []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: top
  namespace: default
`), 0644)
	os.WriteFile(filepath.Join(sub, "nested.yaml"), []byte(`apiVersion: v1
kind: ConfigMap
metadata:
  name: nested
  namespace: default
`), 0644)

	objs, errs := readSources([]string{dir})
	if len(errs) != 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(objs) != 2 {
		t.Fatalf("expected 2 objects, got %d", len(objs))
	}
}
```

Add imports: `"os"`, `"path/filepath"`.

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/ariadne/ -run TestReadSources -v`
Expected: FAIL — `readSources` not defined.

**Step 3: Write minimal implementation**

Append to `cmd/ariadne/yaml.go`:

```go
// readSources reads YAML from file and directory paths.
// Directories are walked recursively for *.yaml and *.yml files.
func readSources(paths []string) ([]unstructured.Unstructured, []error) {
	var allObjs []unstructured.Unstructured
	var allErrs []error

	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			allErrs = append(allErrs, fmt.Errorf("%s: %w", p, err))
			continue
		}
		if info.IsDir() {
			objs, errs := readDir(p)
			allObjs = append(allObjs, objs...)
			allErrs = append(allErrs, errs...)
		} else {
			objs, errs := readFile(p)
			allObjs = append(allObjs, objs...)
			allErrs = append(allErrs, errs...)
		}
	}
	return allObjs, allErrs
}

func readFile(path string) ([]unstructured.Unstructured, []error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, []error{fmt.Errorf("%s: %w", path, err)}
	}
	defer f.Close()
	objs, errs := decodeYAML(f)
	for i := range errs {
		errs[i] = fmt.Errorf("%s: %w", path, errs[i])
	}
	return objs, errs
}

func readDir(dir string) ([]unstructured.Unstructured, []error) {
	var allObjs []unstructured.Unstructured
	var allErrs []error

	filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			allErrs = append(allErrs, fmt.Errorf("%s: %w", path, err))
			return nil
		}
		if d.IsDir() {
			return nil
		}
		ext := filepath.Ext(path)
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}
		objs, errs := readFile(path)
		allObjs = append(allObjs, objs...)
		allErrs = append(allErrs, errs...)
		return nil
	})
	return allObjs, allErrs
}
```

Add imports: `"fmt"`, `"io/fs"`, `"os"`, `"path/filepath"`.

**Step 4: Run test to verify it passes**

Run: `go test ./cmd/ariadne/ -run TestReadSources -v`
Expected: PASS (all three tests).

**Step 5: Commit**

```bash
git add cmd/ariadne/yaml.go cmd/ariadne/yaml_test.go
git commit -m "Add file and directory reading for lint subcommand"
```

---

### Task 3: Lint logic — detect dangling references with filtering

**Files:**
- Create: `cmd/ariadne/lint.go`
- Test: `cmd/ariadne/lint_test.go`

**Step 1: Write the failing test**

In `cmd/ariadne/lint_test.go`:

```go
package main

import (
	"bytes"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func newObj(group, version, kind, ns, name string) unstructured.Unstructured {
	obj := unstructured.Unstructured{}
	obj.SetAPIVersion(group + "/" + version)
	if group == "" {
		obj.SetAPIVersion(version)
	}
	obj.SetKind(kind)
	obj.SetNamespace(ns)
	obj.SetName(name)
	return obj
}

func TestLint_detectsDanglingRef(t *testing.T) {
	// Pod references a ConfigMap that doesn't exist.
	pod := newObj("", "v1", "Pod", "default", "web")
	unstructured.SetNestedField(pod.Object, "my-config",
		"spec", "serviceAccountName")

	objs := []unstructured.Unstructured{pod}
	var buf bytes.Buffer
	count := lint(objs, &buf)

	if count == 0 {
		t.Fatal("expected at least one dangling reference")
	}
	if buf.Len() == 0 {
		t.Fatal("expected output")
	}
}

func TestLint_noDanglingWhenComplete(t *testing.T) {
	pod := newObj("", "v1", "Pod", "default", "web")
	unstructured.SetNestedField(pod.Object, "my-sa",
		"spec", "serviceAccountName")
	sa := newObj("", "v1", "ServiceAccount", "default", "my-sa")

	objs := []unstructured.Unstructured{pod, sa}
	var buf bytes.Buffer
	count := lint(objs, &buf)

	if count != 0 {
		t.Fatalf("expected 0 dangling refs, got %d; output:\n%s", count, buf.String())
	}
}

func TestLint_filtersOwnerRefs(t *testing.T) {
	// Pod with an ownerRef to a ReplicaSet not in the set.
	// This should NOT be reported as a dangling reference.
	pod := newObj("", "v1", "Pod", "default", "web")
	pod.SetOwnerReferences([]metav1.OwnerReference{{
		APIVersion: "apps/v1",
		Kind:       "ReplicaSet",
		Name:       "web-abc123",
	}})

	objs := []unstructured.Unstructured{pod}
	var buf bytes.Buffer
	count := lint(objs, &buf)

	if count != 0 {
		t.Fatalf("expected 0 (ownerRef filtered), got %d; output:\n%s", count, buf.String())
	}
}

func TestLint_filtersEvents(t *testing.T) {
	// An Event whose involvedObject isn't in the set.
	// Should NOT be reported.
	event := newObj("", "v1", "Event", "default", "web.12345")
	unstructured.SetNestedField(event.Object, "v1", "involvedObject", "apiVersion")
	unstructured.SetNestedField(event.Object, "Pod", "involvedObject", "kind")
	unstructured.SetNestedField(event.Object, "default", "involvedObject", "namespace")
	unstructured.SetNestedField(event.Object, "web", "involvedObject", "name")

	objs := []unstructured.Unstructured{event}
	var buf bytes.Buffer
	count := lint(objs, &buf)

	if count != 0 {
		t.Fatalf("expected 0 (event filtered), got %d; output:\n%s", count, buf.String())
	}
}
```

Add import: `metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"`.

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/ariadne/ -run TestLint -v`
Expected: FAIL — `lint` not defined.

**Step 3: Write minimal implementation**

In `cmd/ariadne/lint.go`:

```go
package main

import (
	"fmt"
	"io"
	"sort"

	"github.com/aalpar/ariadne"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// lint builds a graph from objs, finds dangling references, writes them
// to w, and returns the count. Filters out ownerRef and event edges.
func lint(objs []unstructured.Unstructured, w io.Writer) int {
	g := ariadne.New(
		ariadne.WithResolver(ariadne.NewStructuralResolver()),
		ariadne.WithResolver(ariadne.NewSelectorResolver()),
		ariadne.WithResolver(ariadne.NewArgoCDResolver()),
		ariadne.WithResolver(ariadne.NewKyvernoResolver()),
		ariadne.WithResolver(ariadne.NewCrossplaneResolver()),
		ariadne.WithResolver(ariadne.NewGatewayAPIResolver()),
		ariadne.WithResolver(ariadne.NewClusterAPIResolver()),
	)
	g.Load(objs)

	type finding struct {
		from  ariadne.ObjectRef
		to    ariadne.ObjectRef
		field string
	}

	var findings []finding
	for _, e := range g.Edges() {
		// Skip ownerRef edges — set by controllers, not manifest authors.
		if e.Field == "metadata.ownerReferences" {
			continue
		}
		// Skip event edges — runtime objects.
		if e.Resolver == "event" {
			continue
		}
		// If the target doesn't exist in the graph, it's dangling.
		if !g.Has(e.To) {
			findings = append(findings, finding{
				from:  e.From,
				to:    e.To,
				field: e.Field,
			})
		}
	}

	sort.Slice(findings, func(i, j int) bool {
		if findings[i].from.String() != findings[j].from.String() {
			return findings[i].from.String() < findings[j].from.String()
		}
		return findings[i].to.String() < findings[j].to.String()
	})

	for _, f := range findings {
		fmt.Fprintf(w, "%s -> %s (%s): not found\n", f.from, f.to, f.field)
	}
	return len(findings)
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./cmd/ariadne/ -run TestLint -v`
Expected: PASS (all four tests).

Note: `NewCrossplaneResolver()` takes variadic `ManagedResource` args — calling it with zero args is valid and means it only resolves Composition→Composite edges, not managed→ProviderConfig. This is the right default for the linter since we don't know which managed resource types exist.

**Step 5: Commit**

```bash
git add cmd/ariadne/lint.go cmd/ariadne/lint_test.go
git commit -m "Add lint logic with dangling reference detection and filtering"
```

---

### Task 4: main.go — wire up CLI entry point

**Files:**
- Create: `cmd/ariadne/main.go`

**Step 1: Write the implementation**

In `cmd/ariadne/main.go`:

```go
package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: ariadne <command> [args...]\n\nCommands:\n  lint    Check for dangling resource references\n")
	}
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
		os.Exit(2)
	}

	switch args[0] {
	case "lint":
		os.Exit(runLint(args[1:]))
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[0])
		flag.Usage()
		os.Exit(2)
	}
}

func runLint(args []string) int {
	var objs []unstructured.Unstructured
	var allErrs []error

	if len(args) == 0 {
		// Read from stdin.
		o, errs := decodeYAML(os.Stdin)
		objs = append(objs, o...)
		allErrs = append(allErrs, errs...)
	} else {
		o, errs := readSources(args)
		objs = append(objs, o...)
		allErrs = append(allErrs, errs...)
	}

	for _, err := range allErrs {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
	}

	if len(objs) == 0 {
		fmt.Fprintln(os.Stderr, "no valid Kubernetes objects found")
		return 2
	}

	count := lint(objs, os.Stdout)
	if count > 0 {
		return 1
	}
	return 0
}
```

Add import: `"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"`.

**Step 2: Verify it compiles and runs**

Run: `go build ./cmd/ariadne/ && echo "build OK"`
Expected: build OK

Run: `echo 'apiVersion: v1
kind: Pod
metadata:
  name: test
  namespace: default
spec:
  serviceAccountName: missing-sa' | go run ./cmd/ariadne lint`

Expected: output showing the dangling reference to `missing-sa`, exit code 1.

Run: `echo 'apiVersion: v1
kind: ConfigMap
metadata:
  name: test
  namespace: default' | go run ./cmd/ariadne lint`

Expected: no output, exit code 0.

**Step 3: Commit**

```bash
git add cmd/ariadne/main.go
git commit -m "Add CLI entry point for ariadne lint"
```

---

### Task 5: Integration test — end-to-end with realistic manifests

**Files:**
- Modify: `cmd/ariadne/lint_test.go`

**Step 1: Write the integration test**

Append to `cmd/ariadne/lint_test.go`:

```go
func TestLint_integration(t *testing.T) {
	// Realistic scenario: Deployment with Pod template referencing
	// a ConfigMap and Secret. ConfigMap exists, Secret doesn't.
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

	cm := newObj("", "v1", "ConfigMap", "default", "app-config")
	// Note: app-secret is deliberately missing.

	svc := newObj("", "v1", "Service", "default", "web-svc")
	unstructured.SetNestedStringMap(svc.Object, map[string]string{
		"app": "web",
	}, "spec", "selector")
	// No matching Pod labels — Service selector won't match,
	// but that's not a "dangling ref" since it's label-based.

	objs := []unstructured.Unstructured{pod, cm, svc}
	var buf bytes.Buffer
	count := lint(objs, &buf)

	output := buf.String()
	// Should find the missing Secret.
	if count < 1 {
		t.Fatalf("expected at least 1 finding, got %d", count)
	}
	if !strings.Contains(output, "app-secret") {
		t.Errorf("expected finding for app-secret, got:\n%s", output)
	}
	// Should NOT report the service selector as dangling.
	if strings.Contains(output, "web-svc") {
		t.Errorf("service selector should not appear as dangling:\n%s", output)
	}
}
```

Add import: `"strings"`.

**Step 2: Run test**

Run: `go test ./cmd/ariadne/ -run TestLint_integration -v`
Expected: PASS.

**Step 3: Commit**

```bash
git add cmd/ariadne/lint_test.go
git commit -m "Add integration test for lint with realistic manifests"
```

---

### Task 6: Run full test suite, smoke test, clean up

**Step 1: Run all tests**

Run: `go test ./... -v -race`
Expected: All PASS, no race conditions.

**Step 2: Run vet**

Run: `go vet ./...`
Expected: No issues.

**Step 3: Manual smoke test**

Run against a real manifest set if available, or against a synthetic one:

```bash
cat <<'EOF' | go run ./cmd/ariadne lint
apiVersion: v1
kind: Pod
metadata:
  name: nginx
  namespace: default
spec:
  serviceAccountName: nginx-sa
  containers:
  - name: nginx
    image: nginx
    envFrom:
    - configMapRef:
        name: nginx-env
  volumes:
  - name: config
    configMap:
      name: nginx-config
  - name: certs
    secret:
      secretName: nginx-tls
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: nginx-config
  namespace: default
---
apiVersion: v1
kind: ServiceAccount
metadata:
  name: nginx-sa
  namespace: default
EOF
```

Expected: Reports dangling references for `nginx-env` (ConfigMap) and `nginx-tls` (Secret). Does NOT report `nginx-config` or `nginx-sa` (both present).

**Step 4: Commit any fixes**

If smoke test reveals issues, fix and commit.

**Step 5: Final commit**

```bash
git add -A
git commit -m "Finalize ariadne lint subcommand"
```
