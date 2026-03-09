# Typed-Reference Support Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Extend `RefRule` to auto-detect typed references (maps with kind/name/apiGroup sub-fields) so target types can be discovered from object data at runtime.

**Architecture:** Add `extractRawValues` for type-agnostic field extraction. Refactor `resolveRef` to dispatch on string (bare name) vs map (typed ref). Add `parseTypedRef` helper. Update reverse resolution guard for unconstrained `ToGroup`/`ToKind`.

**Tech Stack:** Go, `k8s.io/apimachinery` (unstructured)

**Design doc:** `docs/plans/2026-03-09-typed-ref-design.md`

---

### Task 1: Add extractRawValues and parseTypedRef helpers

**Files:**
- Modify: `rules.go`
- Modify: `rules_test.go`

**Step 1: Write the failing tests**

Add to `rules_test.go`:

```go
func TestExtractRawValues_String(t *testing.T) {
	obj := map[string]interface{}{
		"spec": map[string]interface{}{
			"volumeName": "my-pv",
		},
	}
	vals := extractRawValues(obj, "spec.volumeName")
	if len(vals) != 1 {
		t.Fatalf("expected 1 value, got %d", len(vals))
	}
	if s, ok := vals[0].(string); !ok || s != "my-pv" {
		t.Fatalf("expected string 'my-pv', got %v", vals[0])
	}
}

func TestExtractRawValues_Map(t *testing.T) {
	obj := map[string]interface{}{
		"spec": map[string]interface{}{
			"scaleTargetRef": map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"name":       "web",
			},
		},
	}
	vals := extractRawValues(obj, "spec.scaleTargetRef")
	if len(vals) != 1 {
		t.Fatalf("expected 1 value, got %d", len(vals))
	}
	m, ok := vals[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", vals[0])
	}
	if m["kind"] != "Deployment" {
		t.Fatalf("expected kind=Deployment, got %v", m["kind"])
	}
}

func TestExtractRawValues_ArrayOfMaps(t *testing.T) {
	obj := map[string]interface{}{
		"spec": map[string]interface{}{
			"refs": []interface{}{
				map[string]interface{}{
					"kind": "Service",
					"name": "svc-a",
				},
				map[string]interface{}{
					"kind": "Service",
					"name": "svc-b",
				},
			},
		},
	}
	vals := extractRawValues(obj, "spec.refs[*]")
	if len(vals) != 2 {
		t.Fatalf("expected 2 values, got %d", len(vals))
	}
}

func TestParseTypedRef(t *testing.T) {
	tests := []struct {
		name  string
		input map[string]interface{}
		want  ObjectRef
		ok    bool
	}{
		{
			name: "apiVersion with group",
			input: map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"name":       "web",
			},
			want: ObjectRef{Group: "apps", Kind: "Deployment", Name: "web"},
			ok:   true,
		},
		{
			name: "apiGroup field",
			input: map[string]interface{}{
				"apiGroup": "rbac.authorization.k8s.io",
				"kind":     "Role",
				"name":     "admin",
			},
			want: ObjectRef{Group: "rbac.authorization.k8s.io", Kind: "Role", Name: "admin"},
			ok:   true,
		},
		{
			name: "group field",
			input: map[string]interface{}{
				"group": "apps",
				"kind":  "Deployment",
				"name":  "web",
			},
			want: ObjectRef{Group: "apps", Kind: "Deployment", Name: "web"},
			ok:   true,
		},
		{
			name: "core apiVersion (no group)",
			input: map[string]interface{}{
				"apiVersion": "v1",
				"kind":       "Service",
				"name":       "my-svc",
			},
			want: ObjectRef{Group: "", Kind: "Service", Name: "my-svc"},
			ok:   true,
		},
		{
			name: "with namespace",
			input: map[string]interface{}{
				"apiGroup":  "",
				"kind":      "Service",
				"name":      "my-svc",
				"namespace": "prod",
			},
			want: ObjectRef{Group: "", Kind: "Service", Namespace: "prod", Name: "my-svc"},
			ok:   true,
		},
		{
			name:  "missing kind",
			input: map[string]interface{}{"name": "foo"},
			ok:    false,
		},
		{
			name:  "missing name",
			input: map[string]interface{}{"kind": "Pod"},
			ok:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseTypedRef(tt.input)
			if ok != tt.ok {
				t.Fatalf("parseTypedRef ok=%v, want %v", ok, tt.ok)
			}
			if ok && got != tt.want {
				t.Fatalf("parseTypedRef = %v, want %v", got, tt.want)
			}
		})
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test -run 'TestExtractRawValues|TestParseTypedRef' -v ./...`
Expected: FAIL — undefined functions

**Step 3: Implement extractRawValues and parseTypedRef in rules.go**

Add `extractRawValues` after `extractFieldValues`:

```go
// extractRawValues extracts raw values (strings, maps, etc.) from a
// nested map using a dot-separated field path. Like extractFieldValues
// but returns the leaf values without type restriction.
func extractRawValues(obj map[string]interface{}, path string) []interface{} {
	parts := splitFieldPath(path)
	return extractRawRecursive(obj, parts)
}

func extractRawRecursive(data interface{}, parts []string) []interface{} {
	if len(parts) == 0 {
		return []interface{}{data}
	}

	part := parts[0]
	rest := parts[1:]

	if strings.HasSuffix(part, "[*]") {
		key := strings.TrimSuffix(part, "[*]")
		m, ok := data.(map[string]interface{})
		if !ok {
			return nil
		}
		arr, ok := m[key].([]interface{})
		if !ok {
			return nil
		}
		var result []interface{}
		for _, item := range arr {
			result = append(result, extractRawRecursive(item, rest)...)
		}
		return result
	}

	m, ok := data.(map[string]interface{})
	if !ok {
		return nil
	}
	val, ok := m[part]
	if !ok {
		return nil
	}
	return extractRawRecursive(val, rest)
}
```

Add `parseTypedRef` after `resolveRefReverse`:

```go
// parseTypedRef extracts an ObjectRef from a typed reference map.
// Expects at minimum "kind" and "name" keys. Group is read from
// "apiGroup", "group", or parsed from "apiVersion". Namespace is
// read from "namespace" if present.
func parseTypedRef(m map[string]interface{}) (ObjectRef, bool) {
	kind, _ := m["kind"].(string)
	name, _ := m["name"].(string)
	if kind == "" || name == "" {
		return ObjectRef{}, false
	}

	var group string
	if g, ok := m["apiGroup"].(string); ok {
		group = g
	} else if g, ok := m["group"].(string); ok {
		group = g
	} else if av, ok := m["apiVersion"].(string); ok {
		group = extractGroup(av)
	}

	ref := ObjectRef{
		Group: group,
		Kind:  kind,
		Name:  name,
	}
	if ns, ok := m["namespace"].(string); ok {
		ref.Namespace = ns
	}
	return ref, true
}
```

Note: `extractGroup` already exists in `structural.go` — parses `"apps/v1"` → `"apps"`, `"v1"` → `""`.

**Step 4: Run tests to verify they pass**

Run: `go test -run 'TestExtractRawValues|TestParseTypedRef' -v ./...`
Expected: PASS

Run: `go test -v ./...`
Expected: ALL PASS

**Step 5: Commit**

```
Add extractRawValues and parseTypedRef helpers

extractRawValues returns leaf values of any type (strings, maps)
from a field path. parseTypedRef extracts an ObjectRef from a map
with kind/name/apiGroup sub-fields.
```

---

### Task 2: Refactor resolveRef forward resolution for typed refs

**Files:**
- Modify: `rules.go`
- Modify: `rules_test.go`

**Step 1: Write the failing tests**

Add to `rules_test.go`:

```go
func TestRefRule_TypedRef(t *testing.T) {
	r := NewRuleResolver("test", RefRule{
		FromGroup: "autoscaling", FromKind: "HorizontalPodAutoscaler",
		FieldPath: "spec.scaleTargetRef",
	})

	hpa := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "autoscaling/v2", "kind": "HorizontalPodAutoscaler",
		"metadata": map[string]interface{}{
			"name": "web-hpa", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"scaleTargetRef": map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"name":       "web",
			},
		},
	}}

	deploy := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]interface{}{
			"name": "web", "namespace": "default",
		},
	}}

	lookup := &stubLookup{
		objects: map[ObjectRef]*unstructured.Unstructured{
			{Group: "apps", Kind: "Deployment", Namespace: "default", Name: "web"}: deploy,
		},
	}

	edges := r.Resolve(hpa, lookup)
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if edges[0].To.Group != "apps" || edges[0].To.Kind != "Deployment" || edges[0].To.Name != "web" {
		t.Fatalf("unexpected target: %v", edges[0].To)
	}
	if edges[0].Type != EdgeLocalNameRef {
		t.Fatalf("expected EdgeLocalNameRef, got %v", edges[0].Type)
	}
}

func TestRefRule_TypedRefWithConstraint(t *testing.T) {
	r := NewRuleResolver("test", RefRule{
		FromGroup: "rbac.authorization.k8s.io", FromKind: "RoleBinding",
		ToGroup:   "rbac.authorization.k8s.io",
		FieldPath: "spec.roleRef",
	})

	rb := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "RoleBinding",
		"metadata": map[string]interface{}{
			"name": "admin-binding", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"roleRef": map[string]interface{}{
				"apiGroup": "rbac.authorization.k8s.io",
				"kind":     "Role",
				"name":     "admin",
			},
		},
	}}

	role := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "Role",
		"metadata": map[string]interface{}{
			"name": "admin", "namespace": "default",
		},
	}}

	lookup := &stubLookup{
		objects: map[ObjectRef]*unstructured.Unstructured{
			{Group: "rbac.authorization.k8s.io", Kind: "Role", Namespace: "default", Name: "admin"}: role,
		},
	}

	edges := r.Resolve(rb, lookup)
	if len(edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(edges))
	}
	if edges[0].To.Kind != "Role" {
		t.Fatalf("expected Role, got %v", edges[0].To.Kind)
	}
}

func TestRefRule_TypedRefConstraintMismatch(t *testing.T) {
	// Rule constrains ToGroup to "apps" but the ref points to "batch"
	r := NewRuleResolver("test", RefRule{
		FromKind: "MyController",
		ToGroup:  "apps",
		FieldPath: "spec.targetRef",
	})

	obj := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "MyController",
		"metadata": map[string]interface{}{
			"name": "ctl", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"targetRef": map[string]interface{}{
				"apiGroup": "batch",
				"kind":     "Job",
				"name":     "my-job",
			},
		},
	}}

	job := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "batch/v1", "kind": "Job",
		"metadata": map[string]interface{}{
			"name": "my-job", "namespace": "default",
		},
	}}

	lookup := &stubLookup{
		objects: map[ObjectRef]*unstructured.Unstructured{
			{Group: "batch", Kind: "Job", Namespace: "default", Name: "my-job"}: job,
		},
	}

	edges := r.Resolve(obj, lookup)
	if len(edges) != 0 {
		t.Fatalf("expected 0 edges (constraint mismatch), got %d: %v", len(edges), edges)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test -run 'TestRefRule_TypedRef' -v ./...`
Expected: FAIL — typed ref maps are not handled (no edges produced)

**Step 3: Refactor resolveRef to use extractRawValues and dispatch on type**

Replace the `resolveRef` function in `rules.go` with:

```go
func resolveRef(ref ObjectRef, obj *unstructured.Unstructured, rule RefRule, lookup Lookup) []Edge {
	if ref.Group != rule.FromGroup || ref.Kind != rule.FromKind {
		return resolveRefReverse(ref, obj, rule, lookup)
	}

	values := extractRawValues(obj.Object, rule.FieldPath)
	if len(values) == 0 {
		return nil
	}

	var edges []Edge
	for i, val := range values {
		switch v := val.(type) {
		case string:
			edges = append(edges, resolveBareName(ref, v, i, rule, lookup)...)
		case map[string]interface{}:
			edges = append(edges, resolveTypedRef(ref, v, rule, lookup)...)
		}
	}
	return edges
}

func resolveBareName(ref ObjectRef, name string, index int, rule RefRule, lookup Lookup) []Edge {
	if rule.NamespaceFieldPath != "" {
		namespaces := extractFieldValues(
			// We need the source object to read NamespaceFieldPath.
			// Reconstruct the minimal path. But we don't have the obj here.
			// Actually, let's handle NamespaceFieldPath in resolveRef directly.
		)
	}
	// ... this approach doesn't work well, see below
	return nil
}
```

Actually, `resolveBareName` needs access to the source object for `NamespaceFieldPath`. A cleaner approach: keep the `NamespaceFieldPath` handling in `resolveRef` as a pre-pass, and only dispatch after namespace resolution. Here's the revised refactor:

```go
func resolveRef(ref ObjectRef, obj *unstructured.Unstructured, rule RefRule, lookup Lookup) []Edge {
	if ref.Group != rule.FromGroup || ref.Kind != rule.FromKind {
		return resolveRefReverse(ref, obj, rule, lookup)
	}

	values := extractRawValues(obj.Object, rule.FieldPath)
	if len(values) == 0 {
		return nil
	}

	var namespaces []string
	if rule.NamespaceFieldPath != "" {
		namespaces = extractFieldValues(obj.Object, rule.NamespaceFieldPath)
	}

	var edges []Edge
	for i, val := range values {
		switch v := val.(type) {
		case string:
			edges = append(edges, resolveBareName(ref, v, i, namespaces, rule, lookup)...)
		case map[string]interface{}:
			edges = append(edges, resolveTypedRef(ref, v, rule, lookup)...)
		}
	}
	return edges
}

// resolveBareName handles a bare string name value.
func resolveBareName(ref ObjectRef, name string, index int, namespaces []string, rule RefRule, lookup Lookup) []Edge {
	if len(namespaces) > 0 {
		ns := ref.Namespace
		if index < len(namespaces) {
			ns = namespaces[index]
		}
		toRef := ObjectRef{
			Group:     rule.ToGroup,
			Kind:      rule.ToKind,
			Namespace: ns,
			Name:      name,
		}
		if _, ok := lookup.Get(toRef); ok {
			return []Edge{{
				From:     ref,
				To:       toRef,
				Type:     EdgeNameRef,
				Resolver: "rule",
				Field:    rule.FieldPath,
			}}
		}
		return nil
	}

	// No NamespaceFieldPath: try same-namespace, then cluster-scoped.
	sameNS := ObjectRef{
		Group:     rule.ToGroup,
		Kind:      rule.ToKind,
		Namespace: ref.Namespace,
		Name:      name,
	}
	if _, ok := lookup.Get(sameNS); ok {
		return []Edge{{
			From:     ref,
			To:       sameNS,
			Type:     EdgeLocalNameRef,
			Resolver: "rule",
			Field:    rule.FieldPath,
		}}
	}
	clusterScoped := ObjectRef{
		Group: rule.ToGroup,
		Kind:  rule.ToKind,
		Name:  name,
	}
	if _, ok := lookup.Get(clusterScoped); ok {
		return []Edge{{
			From:     ref,
			To:       clusterScoped,
			Type:     EdgeNameRef,
			Resolver: "rule",
			Field:    rule.FieldPath,
		}}
	}
	return nil
}

// resolveTypedRef handles a typed reference map (kind/name/apiGroup).
func resolveTypedRef(ref ObjectRef, m map[string]interface{}, rule RefRule, lookup Lookup) []Edge {
	toRef, ok := parseTypedRef(m)
	if !ok {
		return nil
	}

	// Apply type constraint if set.
	if rule.ToGroup != "" && toRef.Group != rule.ToGroup {
		return nil
	}
	if rule.ToKind != "" && toRef.Kind != rule.ToKind {
		return nil
	}

	// If typed ref has explicit namespace, use it directly.
	if toRef.Namespace != "" {
		if _, ok := lookup.Get(toRef); ok {
			return []Edge{{
				From:     ref,
				To:       toRef,
				Type:     EdgeNameRef,
				Resolver: "rule",
				Field:    rule.FieldPath,
			}}
		}
		return nil
	}

	// No namespace in ref: try same-namespace, then cluster-scoped.
	sameNS := toRef
	sameNS.Namespace = ref.Namespace
	if _, ok := lookup.Get(sameNS); ok {
		return []Edge{{
			From:     ref,
			To:       sameNS,
			Type:     EdgeLocalNameRef,
			Resolver: "rule",
			Field:    rule.FieldPath,
		}}
	}
	if _, ok := lookup.Get(toRef); ok {
		return []Edge{{
			From:     ref,
			To:       toRef,
			Type:     EdgeNameRef,
			Resolver: "rule",
			Field:    rule.FieldPath,
		}}
	}
	return nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -run 'TestRefRule' -v ./...`
Expected: ALL PASS (existing + new)

Run: `go test -v ./...`
Expected: ALL PASS

**Step 5: Commit**

```
Add typed-reference forward resolution to RefRule

resolveRef auto-detects string (bare name) vs map (typed ref) at
FieldPath. Typed refs extract kind/name/group from sub-fields.
ToGroup/ToKind act as optional type constraints.
```

---

### Task 3: Update reverse resolution for typed refs and unconstrained rules

**Files:**
- Modify: `rules.go`
- Modify: `rules_test.go`

**Step 1: Write the failing tests**

Add to `rules_test.go`:

```go
func TestRefRule_TypedRefReverse(t *testing.T) {
	r := NewRuleResolver("test", RefRule{
		FromGroup: "autoscaling", FromKind: "HorizontalPodAutoscaler",
		FieldPath: "spec.scaleTargetRef",
	})

	// A Deployment is being added; an HPA already exists that references it.
	deploy := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]interface{}{
			"name": "web", "namespace": "default",
		},
	}}

	hpa := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "autoscaling/v2", "kind": "HorizontalPodAutoscaler",
		"metadata": map[string]interface{}{
			"name": "web-hpa", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"scaleTargetRef": map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"name":       "web",
			},
		},
	}}

	lookup := &stubLookup{
		objects: map[ObjectRef]*unstructured.Unstructured{
			{Group: "autoscaling", Kind: "HorizontalPodAutoscaler", Namespace: "default", Name: "web-hpa"}: hpa,
		},
	}

	edges := r.Resolve(deploy, lookup)
	if len(edges) != 1 {
		t.Fatalf("expected 1 reverse edge, got %d: %v", len(edges), edges)
	}
	if edges[0].From.Kind != "HorizontalPodAutoscaler" {
		t.Fatalf("expected edge from HPA, got %v", edges[0].From)
	}
	if edges[0].To.Kind != "Deployment" || edges[0].To.Name != "web" {
		t.Fatalf("expected edge to Deployment/web, got %v", edges[0].To)
	}
}

func TestRefRule_TypedRefReverseNoMatch(t *testing.T) {
	// HPA targets Deployment, but we add a StatefulSet — no edge.
	r := NewRuleResolver("test", RefRule{
		FromGroup: "autoscaling", FromKind: "HorizontalPodAutoscaler",
		FieldPath: "spec.scaleTargetRef",
	})

	ss := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1", "kind": "StatefulSet",
		"metadata": map[string]interface{}{
			"name": "db", "namespace": "default",
		},
	}}

	hpa := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "autoscaling/v2", "kind": "HorizontalPodAutoscaler",
		"metadata": map[string]interface{}{
			"name": "web-hpa", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"scaleTargetRef": map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"name":       "web",
			},
		},
	}}

	lookup := &stubLookup{
		objects: map[ObjectRef]*unstructured.Unstructured{
			{Group: "autoscaling", Kind: "HorizontalPodAutoscaler", Namespace: "default", Name: "web-hpa"}: hpa,
		},
	}

	edges := r.Resolve(ss, lookup)
	if len(edges) != 0 {
		t.Fatalf("expected 0 edges (HPA targets Deployment, not StatefulSet), got %d: %v", len(edges), edges)
	}
}
```

**Step 2: Run tests to verify they fail**

Run: `go test -run 'TestRefRule_TypedRefReverse' -v ./...`
Expected: FAIL — reverse resolution doesn't handle typed refs

**Step 3: Update resolveRefReverse**

Replace `resolveRefReverse` in `rules.go`:

```go
func resolveRefReverse(ref ObjectRef, obj *unstructured.Unstructured, rule RefRule, lookup Lookup) []Edge {
	// Type constraint guard: skip if the added object can't be a target.
	if rule.ToKind != "" && (ref.Group != rule.ToGroup || ref.Kind != rule.ToKind) {
		return nil
	}

	// For unconstrained rules (ToKind empty), scan all sources.
	// For constrained rules, scope by namespace when possible.
	var sources []*unstructured.Unstructured
	if rule.ToKind == "" || rule.NamespaceFieldPath != "" {
		sources = lookup.List(rule.FromGroup, rule.FromKind)
	} else if ref.Namespace != "" {
		sources = lookup.ListInNamespace(rule.FromGroup, rule.FromKind, ref.Namespace)
	} else {
		sources = lookup.List(rule.FromGroup, rule.FromKind)
	}

	var edges []Edge
	for _, src := range sources {
		srcRef := RefFromUnstructured(src)
		values := extractRawValues(src.Object, rule.FieldPath)

		for i, val := range values {
			switch v := val.(type) {
			case string:
				edge := reverseMatchBareName(srcRef, ref, v, i, src, rule)
				if edge != nil {
					edges = append(edges, *edge)
				}
			case map[string]interface{}:
				edge := reverseMatchTypedRef(srcRef, ref, v, rule)
				if edge != nil {
					edges = append(edges, *edge)
				}
			}
		}
	}
	return edges
}

func reverseMatchBareName(srcRef, targetRef ObjectRef, name string, index int, src *unstructured.Unstructured, rule RefRule) *Edge {
	if name != targetRef.Name {
		return nil
	}

	if rule.NamespaceFieldPath != "" {
		namespaces := extractFieldValues(src.Object, rule.NamespaceFieldPath)
		ns := srcRef.Namespace
		if index < len(namespaces) {
			ns = namespaces[index]
		}
		if ns != targetRef.Namespace {
			return nil
		}
		return &Edge{
			From:     srcRef,
			To:       targetRef,
			Type:     EdgeNameRef,
			Resolver: "rule",
			Field:    rule.FieldPath,
		}
	}

	edgeType := EdgeLocalNameRef
	if targetRef.Namespace == "" {
		edgeType = EdgeNameRef
	}
	return &Edge{
		From:     srcRef,
		To:       targetRef,
		Type:     edgeType,
		Resolver: "rule",
		Field:    rule.FieldPath,
	}
}

func reverseMatchTypedRef(srcRef, targetRef ObjectRef, m map[string]interface{}, rule RefRule) *Edge {
	parsed, ok := parseTypedRef(m)
	if !ok {
		return nil
	}

	// Apply type constraint if set.
	if rule.ToGroup != "" && parsed.Group != rule.ToGroup {
		return nil
	}
	if rule.ToKind != "" && parsed.Kind != rule.ToKind {
		return nil
	}

	// Check if the parsed ref matches the target.
	if parsed.Group != targetRef.Group || parsed.Kind != targetRef.Kind || parsed.Name != targetRef.Name {
		return nil
	}

	// Namespace matching.
	if parsed.Namespace != "" {
		if parsed.Namespace != targetRef.Namespace {
			return nil
		}
	} else {
		// No namespace in ref: matches same-namespace or cluster-scoped.
		if targetRef.Namespace != "" && targetRef.Namespace != srcRef.Namespace {
			return nil
		}
	}

	edgeType := EdgeLocalNameRef
	if parsed.Namespace != "" || targetRef.Namespace == "" {
		edgeType = EdgeNameRef
	}
	return &Edge{
		From:     srcRef,
		To:       targetRef,
		Type:     edgeType,
		Resolver: "rule",
		Field:    rule.FieldPath,
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -run 'TestRefRule' -v ./...`
Expected: ALL PASS

Run: `go test -v ./...`
Expected: ALL PASS

**Step 5: Commit**

```
Add typed-reference reverse resolution to RefRule

resolveRefReverse now handles typed ref maps and unconstrained rules
(empty ToGroup/ToKind). Scans FromKind sources and matches parsed
refs against the newly-added target.
```

---

### Task 4: Clean up and verify

**Step 1: Check for dead code**

`extractFieldValues` is still used in `resolveRefReverse` (for `NamespaceFieldPath`) and in `resolveLabelSelector`. It is NOT dead code. Verify:

Run: `grep -n 'extractFieldValues' rules.go`
Expected: At least 2 call sites remain

**Step 2: Run full test suite with race detector**

Run: `go test -race -v ./...`
Expected: ALL PASS, no races

**Step 3: Run go vet**

Run: `go vet ./...`
Expected: Clean

**Step 4: Update TODO.md — mark typed-ref as done**

Remove the typed-ref TODO item (or mark it done).

**Step 5: Commit**

```
Mark typed-reference support as complete in TODO.md
```
