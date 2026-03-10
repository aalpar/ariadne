# RefRule Unification Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Replace `NameRefRule` and `NamespacedNameRefRule` with a single `RefRule` type that handles both bare-name and explicit-namespace references, with "try both" namespace resolution.

**Architecture:** Single `RefRule` struct with optional `NamespaceFieldPath`. Forward resolution extracts names (and optionally namespaces) from the source object and looks up targets. Reverse resolution finds existing sources that reference a newly-added target. Edge type is derived from the resolution result, not the rule definition.

**Tech Stack:** Go, `k8s.io/apimachinery` (unstructured)

**Design doc:** `docs/plans/2026-03-09-refrule-unification-design.md`

---

### Task 1: Add RefRule type and forward resolution

**Files:**
- Modify: `rules.go`

**Step 1: Write the failing test**

Add to `rules_test.go`:

```go
func TestRefRule_SameNamespace(t *testing.T) {
	r := NewRuleResolver("test", RefRule{
		FromKind: "Pod", ToKind: "ConfigMap",
		FieldPath: "spec.volumes[*].configMap.name",
	})

	pod := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]interface{}{
			"name": "web", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"volumes": []interface{}{
				map[string]interface{}{
					"configMap": map[string]interface{}{"name": "app-config"},
				},
			},
		},
	}}

	cm := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "ConfigMap",
		"metadata": map[string]interface{}{
			"name": "app-config", "namespace": "default",
		},
	}}

	lookup := &stubLookup{
		objects: map[ObjectRef]*unstructured.Unstructured{
			{Kind: "ConfigMap", Namespace: "default", Name: "app-config"}: cm,
		},
	}

	edges := r.Resolve(pod, lookup)
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if edges[0].To.Kind != "ConfigMap" || edges[0].To.Name != "app-config" {
		t.Fatalf("unexpected target: %v", edges[0].To)
	}
	if edges[0].Type != EdgeLocalNameRef {
		t.Fatalf("expected EdgeLocalNameRef, got %v", edges[0].Type)
	}
}

func TestRefRule_ClusterScoped(t *testing.T) {
	r := NewRuleResolver("test", RefRule{
		FromKind: "PersistentVolumeClaim", ToKind: "PersistentVolume",
		FieldPath: "spec.volumeName",
	})

	pvc := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "PersistentVolumeClaim",
		"metadata": map[string]interface{}{
			"name": "my-pvc", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"volumeName": "my-pv",
		},
	}}

	pv := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "PersistentVolume",
		"metadata": map[string]interface{}{"name": "my-pv"},
	}}

	lookup := &stubLookup{
		objects: map[ObjectRef]*unstructured.Unstructured{
			{Kind: "PersistentVolume", Name: "my-pv"}: pv,
		},
	}

	edges := r.Resolve(pvc, lookup)
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if edges[0].To.Namespace != "" {
		t.Fatalf("expected cluster-scoped target, got ns=%q", edges[0].To.Namespace)
	}
	if edges[0].Type != EdgeNameRef {
		t.Fatalf("expected EdgeNameRef for cluster-scoped, got %v", edges[0].Type)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test -run 'TestRefRule_' -v ./...`
Expected: FAIL — `RefRule` undefined

**Step 3: Add RefRule type and forward resolution to rules.go**

Add the `RefRule` struct after the `Rule` interface (before `NameRefRule`):

```go
// RefRule matches a field that contains the name of a target resource.
// When NamespaceFieldPath is set, the namespace is read from the object.
// When empty, resolution tries the source's namespace first, then
// cluster-scoped (""). At most one matches because K8s does not allow
// the same GroupKind to be both namespaced and cluster-scoped.
type RefRule struct {
	FromGroup, FromKind string
	ToGroup, ToKind     string
	FieldPath           string // path to name value(s)
	NamespaceFieldPath  string // optional: path to namespace value(s)
}

func (RefRule) rule() {}
```

Add the `resolveRef` function:

```go
func resolveRef(ref ObjectRef, obj *unstructured.Unstructured, rule RefRule, lookup Lookup) []Edge {
	if ref.Group != rule.FromGroup || ref.Kind != rule.FromKind {
		return resolveRefReverse(ref, obj, rule, lookup)
	}

	names := extractFieldValues(obj.Object, rule.FieldPath)
	if len(names) == 0 {
		return nil
	}

	var edges []Edge

	if rule.NamespaceFieldPath != "" {
		namespaces := extractFieldValues(obj.Object, rule.NamespaceFieldPath)
		for i, name := range names {
			ns := ref.Namespace
			if i < len(namespaces) {
				ns = namespaces[i]
			}
			toRef := ObjectRef{
				Group:     rule.ToGroup,
				Kind:      rule.ToKind,
				Namespace: ns,
				Name:      name,
			}
			if _, ok := lookup.Get(toRef); ok {
				edges = append(edges, Edge{
					From:     ref,
					To:       toRef,
					Type:     EdgeNameRef,
					Resolver: "rule",
					Field:    rule.FieldPath,
				})
			}
		}
		return edges
	}

	// No NamespaceFieldPath: try same-namespace, then cluster-scoped.
	for _, name := range names {
		sameNS := ObjectRef{
			Group:     rule.ToGroup,
			Kind:      rule.ToKind,
			Namespace: ref.Namespace,
			Name:      name,
		}
		if _, ok := lookup.Get(sameNS); ok {
			edges = append(edges, Edge{
				From:     ref,
				To:       sameNS,
				Type:     EdgeLocalNameRef,
				Resolver: "rule",
				Field:    rule.FieldPath,
			})
			continue
		}
		clusterScoped := ObjectRef{
			Group: rule.ToGroup,
			Kind:  rule.ToKind,
			Name:  name,
		}
		if _, ok := lookup.Get(clusterScoped); ok {
			edges = append(edges, Edge{
				From:     ref,
				To:       clusterScoped,
				Type:     EdgeNameRef,
				Resolver: "rule",
				Field:    rule.FieldPath,
			})
		}
	}
	return edges
}
```

Add the type switch case in `ruleResolver.Resolve`:

```go
		case RefRule:
			edges = append(edges, resolveRef(ref, obj, rule, lookup)...)
```

Add a stub for `resolveRefReverse` so it compiles:

```go
func resolveRefReverse(ref ObjectRef, obj *unstructured.Unstructured, rule RefRule, lookup Lookup) []Edge {
	return nil // implemented in Task 2
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -run 'TestRefRule_' -v ./...`
Expected: PASS

**Step 5: Commit**

```
Add RefRule type with forward resolution

New unified rule type that replaces NameRefRule and NamespacedNameRefRule.
Uses "try both" namespace strategy: same-namespace first, then
cluster-scoped. Reverse resolution stubbed for next task.
```

---

### Task 2: Add RefRule reverse resolution

**Files:**
- Modify: `rules.go`
- Modify: `rules_test.go`

**Step 1: Write the failing test**

Add to `rules_test.go`:

```go
func TestRefRule_Reverse(t *testing.T) {
	rule := RefRule{
		FromKind: "Pod", ToKind: "ConfigMap",
		FieldPath: "spec.volumes[*].configMap.name",
	}
	r := NewRuleResolver("test", rule)

	// The ConfigMap is the object being added (target).
	cm := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "ConfigMap",
		"metadata": map[string]interface{}{
			"name": "app-config", "namespace": "default",
		},
	}}

	// A Pod that references it already exists in the graph.
	pod := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Pod",
		"metadata": map[string]interface{}{
			"name": "web", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"volumes": []interface{}{
				map[string]interface{}{
					"configMap": map[string]interface{}{"name": "app-config"},
				},
			},
		},
	}}

	lookup := &stubLookup{
		objects: map[ObjectRef]*unstructured.Unstructured{
			{Kind: "Pod", Namespace: "default", Name: "web"}: pod,
		},
	}

	edges := r.Resolve(cm, lookup)
	if len(edges) != 1 {
		t.Fatalf("expected 1 reverse edge, got %d", len(edges))
	}
	if edges[0].From.Kind != "Pod" || edges[0].From.Name != "web" {
		t.Fatalf("expected edge from Pod/web, got %v", edges[0].From)
	}
	if edges[0].To.Kind != "ConfigMap" || edges[0].To.Name != "app-config" {
		t.Fatalf("expected edge to ConfigMap/app-config, got %v", edges[0].To)
	}
	if edges[0].Type != EdgeLocalNameRef {
		t.Fatalf("expected EdgeLocalNameRef, got %v", edges[0].Type)
	}
}

func TestRefRule_ReverseWithNamespace(t *testing.T) {
	rule := RefRule{
		FromGroup: "example.com", FromKind: "MyResource",
		ToKind:             "Service",
		FieldPath:          "spec.backendRef.name",
		NamespaceFieldPath: "spec.backendRef.namespace",
	}
	r := NewRuleResolver("test", rule)

	// The Service is the object being added.
	svc := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{
			"name": "backend", "namespace": "prod",
		},
	}}

	// A MyResource in a different namespace references it via explicit namespace.
	myRes := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "example.com/v1", "kind": "MyResource",
		"metadata": map[string]interface{}{
			"name": "my-res", "namespace": "staging",
		},
		"spec": map[string]interface{}{
			"backendRef": map[string]interface{}{
				"name":      "backend",
				"namespace": "prod",
			},
		},
	}}

	lookup := &stubLookup{
		objects: map[ObjectRef]*unstructured.Unstructured{
			{Group: "example.com", Kind: "MyResource", Namespace: "staging", Name: "my-res"}: myRes,
		},
	}

	edges := r.Resolve(svc, lookup)
	if len(edges) != 1 {
		t.Fatalf("expected 1 reverse edge, got %d", len(edges))
	}
	if edges[0].From.Name != "my-res" || edges[0].To.Name != "backend" {
		t.Fatalf("unexpected edge: %v -> %v", edges[0].From, edges[0].To)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test -run 'TestRefRule_Reverse' -v ./...`
Expected: FAIL — `resolveRefReverse` returns nil

**Step 3: Implement resolveRefReverse**

Replace the stub in `rules.go`:

```go
func resolveRefReverse(ref ObjectRef, obj *unstructured.Unstructured, rule RefRule, lookup Lookup) []Edge {
	if ref.Group != rule.ToGroup || ref.Kind != rule.ToKind {
		return nil
	}

	var sources []*unstructured.Unstructured
	if rule.NamespaceFieldPath != "" {
		// Explicit namespace field: any namespace can reference this target.
		sources = lookup.List(rule.FromGroup, rule.FromKind)
	} else if ref.Namespace != "" {
		// Same-namespace defaulting: only sources in target's namespace.
		sources = lookup.ListInNamespace(rule.FromGroup, rule.FromKind, ref.Namespace)
	} else {
		// Cluster-scoped target: any namespace's source could reference it.
		sources = lookup.List(rule.FromGroup, rule.FromKind)
	}

	var edges []Edge
	for _, src := range sources {
		srcRef := RefFromUnstructured(src)
		names := extractFieldValues(src.Object, rule.FieldPath)

		if rule.NamespaceFieldPath != "" {
			namespaces := extractFieldValues(src.Object, rule.NamespaceFieldPath)
			for i, name := range names {
				ns := srcRef.Namespace
				if i < len(namespaces) {
					ns = namespaces[i]
				}
				if name == ref.Name && ns == ref.Namespace {
					edges = append(edges, Edge{
						From:     srcRef,
						To:       ref,
						Type:     EdgeNameRef,
						Resolver: "rule",
						Field:    rule.FieldPath,
					})
				}
			}
			continue
		}

		for _, name := range names {
			if name == ref.Name {
				edgeType := EdgeLocalNameRef
				if ref.Namespace == "" {
					edgeType = EdgeNameRef
				}
				edges = append(edges, Edge{
					From:     srcRef,
					To:       ref,
					Type:     edgeType,
					Resolver: "rule",
					Field:    rule.FieldPath,
				})
			}
		}
	}
	return edges
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -run 'TestRefRule' -v ./...`
Expected: PASS (all 4 RefRule tests)

**Step 5: Commit**

```
Add RefRule reverse resolution

Handles same-namespace, cluster-scoped, and explicit-namespace
references bidirectionally. Fixes the gap where NamespacedNameRefRule
had no reverse resolution.
```

---

### Task 3: Migrate structural.go and remove old types

**Files:**
- Modify: `structural.go`
- Modify: `rules.go`
- Modify: `rules_test.go`

**Step 1: Migrate structural.go from NameRefRule to RefRule**

Replace every `NameRefRule{...}` with `RefRule{...}` in `structural.go`. Drop `SameNamespace` field. The `FieldPath` field name is the same. Example:

```go
// Before
NameRefRule{
	FromKind: "Pod", ToKind: "ServiceAccount",
	FieldPath: "spec.serviceAccountName", SameNamespace: true,
},

// After
RefRule{
	FromKind: "Pod", ToKind: "ServiceAccount",
	FieldPath: "spec.serviceAccountName",
},
```

Apply this to all 9 rules in `NewStructuralResolver()`.

**Step 2: Update TestNameRefRule in rules_test.go**

Rename `TestNameRefRule` to `TestRefRule_ExistingBehavior` and change the rule from `NameRefRule{...}` to `RefRule{...}`:

```go
func TestRefRule_ExistingBehavior(t *testing.T) {
	r := NewRuleResolver("test", RefRule{
		FromGroup: "", FromKind: "PersistentVolumeClaim",
		ToGroup: "", ToKind: "PersistentVolume",
		FieldPath: "spec.volumeName",
	})

	// ... rest of test unchanged ...
}
```

**Step 3: Remove NameRefRule, NamespacedNameRefRule, and their resolve functions from rules.go**

Delete:
- `NameRefRule` struct and `rule()` method (lines 29-37)
- `NamespacedNameRefRule` struct and `rule()` method (lines 39-47)
- `case NameRefRule:` and `case NamespacedNameRefRule:` from the type switch (lines 77-80)
- `resolveNameRef` function (lines 89-120)
- `resolveNameRefReverse` function (lines 122-155)
- `resolveNamespacedNameRef` function (lines 157-196)

**Step 4: Run all tests**

Run: `go test -v ./...`
Expected: ALL PASS (31 tests, now using RefRule throughout)

Run: `go vet ./...`
Expected: Clean

**Step 5: Commit**

```
Replace NameRefRule and NamespacedNameRefRule with RefRule

All structural resolver rules now use the unified RefRule type.
Removed the old types and their dedicated resolve functions.
```

---

### Task 4: Verify with race detector and clean up

**Step 1: Run tests with race detector**

Run: `go test -race -v ./...`
Expected: ALL PASS, no race conditions

**Step 2: Run go vet**

Run: `go vet ./...`
Expected: Clean

**Step 3: Final commit (if any cleanup needed)**

Only if steps 1-2 revealed issues.
