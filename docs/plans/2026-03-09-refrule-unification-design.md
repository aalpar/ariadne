# RefRule Unification Design

**Date:** 2026-03-09
**Status:** Approved

## Goal

Replace `NameRefRule` and `NamespacedNameRefRule` with a single `RefRule` type. Same expressive power, fewer types. Fix missing reverse resolution for namespace-qualified references.

## The type

```go
type RefRule struct {
    FromGroup, FromKind string
    ToGroup, ToKind     string
    FieldPath           string // path to name value(s)
    NamespaceFieldPath  string // optional: path to namespace value(s)
}
```

## Resolution logic

### Forward (added object matches FromGroup/FromKind)

1. Extract names via `extractFieldValues(obj, FieldPath)`
2. Determine namespace for each name:
   - `NamespaceFieldPath` set: extract namespaces, pair positionally with names
   - `NamespaceFieldPath` empty: try source's namespace first, then `""` (cluster-scoped). First `lookup.Get` hit wins.
3. Emit edge for each match

### Reverse (added object matches ToGroup/ToKind)

1. Find candidate sources:
   - `NamespaceFieldPath` empty + target namespaced: `ListInNamespace(FromGroup, FromKind, target.Namespace)`
   - `NamespaceFieldPath` empty + target cluster-scoped: `List(FromGroup, FromKind)`
   - `NamespaceFieldPath` set: `List(FromGroup, FromKind)`
2. For each source, extract names (and namespaces if `NamespaceFieldPath` set)
3. Check if extracted name/namespace matches the added target
4. Emit edge for each match

### Edge type derivation

Derived from the resolution result, not the rule definition:

| Condition | EdgeType |
|---|---|
| `NamespaceFieldPath` set | `EdgeNameRef` |
| `NamespaceFieldPath` empty, matched same-namespace | `EdgeLocalNameRef` |
| `NamespaceFieldPath` empty, matched cluster-scoped | `EdgeNameRef` |

### Namespace "try both" rationale

K8s does not allow the same GroupKind to be both namespaced and cluster-scoped. When `NamespaceFieldPath` is empty, trying same-namespace then cluster-scoped produces at most one hit. This eliminates the need for a `SameNamespace` boolean — rules don't need to know whether targets are namespaced or cluster-scoped at definition time.

## Changes

| File | Change |
|---|---|
| `rules.go` | Replace `NameRefRule` + `NamespacedNameRefRule` with `RefRule`. Replace resolve functions with `resolveRef` + `resolveRefReverse`. Update type switch. |
| `structural.go` | Change `NameRefRule{...}` literals to `RefRule{...}`, drop `SameNamespace`. |
| `rules_test.go` | Update `TestNameRefRule` to use `RefRule`. Add test for `RefRule` with `NamespaceFieldPath`. |

No changes to: `types.go`, `resolver.go`, `selector.go`, `event.go`, `structural_test.go`, `integration_test.go`, `graph.go`, `topo.go`, `export.go`.

## Migration

```go
// Before
NameRefRule{
    FromKind: "Pod", ToKind: "ConfigMap",
    FieldPath: "spec.volumes[*].configMap.name", SameNamespace: true,
}

// After
RefRule{
    FromKind: "Pod", ToKind: "ConfigMap",
    FieldPath: "spec.volumes[*].configMap.name",
}
```

## Future

Typed-reference support (parsing maps with kind/name/apiVersion sub-fields) will extend `RefRule` in a follow-up. See TODO.md.
