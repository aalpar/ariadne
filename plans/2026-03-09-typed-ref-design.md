# Typed-Reference Support Design

**Date:** 2026-03-09
**Status:** Approved

## Goal

Extend `RefRule` to auto-detect typed references — maps with kind/name/apiGroup sub-fields — so the target type can be discovered from the object data at runtime. Enables RBAC roleRef, HPA scaleTargetRef, CAPI infrastructureRef, and similar patterns.

## Core change

`resolveRef` auto-detects the value at `FieldPath`:
- **String** — bare name (existing behavior, unchanged)
- **Map** — typed reference: extract `kind`, `name`, group, and optionally `namespace` from sub-fields

No new fields on `RefRule`. `ToGroup`/`ToKind` become optional constraints — when set, they filter; when empty, the target type is discovered from the data.

## Extraction

Add `extractRawValues(obj, path) []interface{}` that returns whatever's at the leaf (strings, maps, etc.). Reuses `extractRecursive`'s path navigation and `[*]` wildcard handling.

`resolveRef` dispatches on type:

```go
values := extractRawValues(obj.Object, rule.FieldPath)
for _, val := range values {
    switch v := val.(type) {
    case string:
        // bare name — existing logic
    case map[string]interface{}:
        // typed ref — extract kind/name/group/namespace
    }
}
```

## Typed ref parsing

From a map, extract the ObjectRef:
1. `name` — required, always `"name"` key
2. `kind` — required, always `"kind"` key
3. Group — try `"apiGroup"` first (most K8s refs), then `"group"`, then parse from `"apiVersion"` (`"apps/v1"` → `"apps"`)
4. `namespace` — optional. If present, use it. If absent, "try both" (same-namespace then cluster-scoped)

If `ToGroup`/`ToKind` are set on the rule, verify the extracted type matches — skip on mismatch (constraint acts as filter).

## Reverse resolution guard

Current guard:
```go
if ref.Group != rule.ToGroup || ref.Kind != rule.ToKind { return nil }
```

Breaks when `ToGroup`/`ToKind` are empty (unconstrained). Fix:
```go
if rule.ToKind != "" && (ref.Group != rule.ToGroup || ref.Kind != rule.ToKind) {
    return nil
}
```

When unconstrained, every added object triggers a scan of `FromKind` sources. Bounded because `FromKind` is always specific.

## NamespaceFieldPath interaction

`NamespaceFieldPath` applies only to bare-name values. For typed refs, namespace comes from the map's `namespace` sub-field (or "try both" if absent).

## Changes

| File | Change |
|---|---|
| `rules.go` | Add `extractRawValues`. Refactor `resolveRef` to dispatch string vs map. Add typed-ref parsing helper. Update `resolveRefReverse` guard. |
| `rules_test.go` | Tests for: typed ref forward, typed ref reverse, unconstrained, partial constraint, type mismatch filtering. |

No changes to other files.

## Example usage

```go
// HPA — unconstrained, target type from data
RefRule{
    FromGroup: "autoscaling", FromKind: "HorizontalPodAutoscaler",
    FieldPath: "spec.scaleTargetRef",
}

// RBAC RoleBinding — partial constraint, must be rbac group
RefRule{
    FromGroup: "rbac.authorization.k8s.io", FromKind: "RoleBinding",
    ToGroup:   "rbac.authorization.k8s.io",
    FieldPath: "spec.roleRef",
}
```
