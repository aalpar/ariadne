# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What is Ariadne

Ariadne is a Go library that builds a directed dependency graph of Kubernetes resources. The caller provides K8s API objects (via client-go, informers, etc.); Ariadne resolves dependencies between them and exposes a queryable, observable graph. It does **not** connect to clusters, watch resources, or persist state — those are the caller's responsibility.

Only dependency: `k8s.io/apimachinery` (for `unstructured.Unstructured`, `schema.GroupVersionKind`, `labels.Selector`). No client-go, controller-runtime, or graph libraries.

## Commands

```bash
go test ./...           # run all tests
go test -run TestName   # run a single test
go test -v ./...        # verbose output
go test -race ./...     # with race detector
go vet ./...            # static analysis
```

No Makefile, no build step — this is a library package.

## Architecture

Single-package library (`package ariadne`). All source files are at the repo root.

### Core flow

1. Caller creates a `Graph` via `New(opts...)` or `NewDefault()` (registers all built-in resolvers)
2. Caller feeds K8s objects via `Add()` (incremental) or `Load()` (batch)
3. For each object, registered `Resolver`s produce `Edge`s by inspecting the object and querying existing nodes via `Lookup`
4. `ChangeListener`s are notified of graph mutations (node/edge add/remove)
5. Caller queries the graph: single-hop (`DependenciesOf`/`DependentsOf`), transitive (`Upstream`/`Downstream`), topological sort, cycle detection, export

### Key design decisions

- **ObjectRef uses GroupKind, not GroupVersionKind** — different API versions of the same kind are the same logical object
- **Edge direction**: `From` depends on `To` (edges point from dependent to dependency)
- **Resolvers are bidirectional** — when object X is added, resolvers emit both "X depends on existing Y" and "existing Z depends on X" edges. This is why resolvers receive a `Lookup` interface.
- **`Load` vs `Add`**: `Load` inserts all nodes first, then resolves edges, then notifies. `Add` resolves per-object as it goes. Use `Load` for initial sync so resolvers can see the full set.
- **Listeners fire under the write lock** — expensive listener work must be dispatched to a separate goroutine

### File layout

| File | Purpose |
|---|---|
| `types.go` | `ObjectRef`, `Edge`, `EdgeType`, `GraphEvent`, `ChangeListener` |
| `resolver.go` | `Resolver` and `Lookup` interfaces |
| `graph.go` | `Graph` struct, `New`/`NewDefault`, `Add`/`Remove`/`Load`, query methods, `graphLookup` |
| `rules.go` | Declarative rule types (`RefRule`, `LabelSelectorRule`), `NewRuleResolver`, field path extraction |
| `structural.go` | Built-in resolver for known K8s references (Pod→SA, Pod→ConfigMap, ownerRefs, etc.) |
| `selector.go` | Built-in resolver for label/selector matching (Service→Pod, NetworkPolicy→Pod) |
| `event.go` | Built-in resolver for K8s Event→involvedObject edges |
| `topo.go` | `TopologicalSort` (Kahn's algorithm), `Cycles` (DFS) |
| `export.go` | `ExportDOT`, `ExportJSON` |

### Resolver hierarchy

Built-in resolvers are composed from the same `Rule` primitives available to users:

```
NewStructuralResolver()
  └─ NewRuleResolver("structural", ...RefRules)  + ownerRef logic
NewSelectorResolver()
  └─ NewRuleResolver("selector", ...LabelSelectorRules)
NewEventResolver()
  └─ custom (parses involvedObject directly)
```

Users extend the graph by calling `NewRuleResolver("my-crd", ...rules)` with their own `RefRule`/`LabelSelectorRule` definitions, or by implementing the `Resolver` interface directly.

### Field path syntax

`rules.go` uses dot-separated paths with `[*]` for slice wildcards: `spec.volumes[*].configMap.name`. This is parsed by `extractFieldValues` / `extractRecursive`, not a full JSONPath implementation.

## Testing patterns

- Tests are `_test.go` files in `package ariadne` (white-box)
- Helper constructors: `newObj(group, version, kind, ns, name)` and `newCoreObj(kind, ns, name)` in `graph_test.go`
- Stub resolvers (`stubResolver`, `chainResolver`) are defined inline in test files for testing graph mechanics independently of built-in resolvers
- `integration_test.go` uses `NewDefault()` with realistic K8s objects to validate the full resolver stack

## Concurrency

`Graph` uses `sync.RWMutex`. Reads take RLock, writes take Lock. Always use `go test -race` when modifying graph internals.

## Versioning

v0.x/v1.x with zero consumers. Break freely — no stability guarantees yet.

## Commits

- No "Co-Authored-By" lines
- Never commit directly to master/main — always branch + PR
- Prefer standard library over new dependencies
