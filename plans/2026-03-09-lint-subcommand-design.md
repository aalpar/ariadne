# Design: `ariadne lint`

A CLI tool that reads Kubernetes YAML manifests and reports dangling references — edges that point to objects not present in the input set.

## Motivation

Ariadne has zero consumers. Before building more library, build a first consumer that validates the API surface against real usage. A dangling-reference linter has clear standalone value (run it in CI against Helm charts or kustomize output) and exercises the resolver system deeply.

No existing tool in the K8s ecosystem does offline reference validation. kubectl-tree, kube-lineage, and kubectl-graph all require a live cluster.

## Behavior

**Input:** When no args are provided, reads multi-document YAML from stdin. When args are provided, reads each arg as a file or walks directories recursively for `*.yaml` and `*.yml` files. Both can be combined.

**Processing:**
1. Decode all YAML documents into `unstructured.Unstructured` objects
2. Call `graph.Load(objects)` with `NewDefault()` + all ecosystem resolvers
3. Walk `graph.Edges()` — if `graph.Has(edge.To)` is false, the edge is a dangling reference
4. Filter out ownerRef edges (runtime-set, not manifest-authored) and event edges (runtime objects)
5. Sort findings deterministically, print, set exit code

**Output:** Human-readable, one line per finding:
```
Pod default/web-server -> ConfigMap default/app-config (spec.volumes[0].configMap.name): not found
```

**Exit codes:** 0 = clean, 1 = dangling references found.

## Filtering

Not every dangling edge is a bug:

- **OwnerReferences** — set by controllers at runtime, not by manifest authors. Skip.
- **Events** — runtime objects. Skip.
- **Implicit namespace references** — not modeled as edges by Ariadne. Not an issue.

Report only edges from structural, selector, and ecosystem resolvers. The `Edge.Resolver` field carries the resolver name for filtering.

## File layout

```
cmd/ariadne/
  main.go    — entry point, flag parsing, stdin vs file/dir dispatch
  lint.go    — lint subcommand: build graph, detect dangling edges, format output
  yaml.go    — YAML reading: stdin, files, directory walking, multi-doc splitting
```

## Dependencies

Zero new dependencies. YAML-to-JSON conversion and Unstructured decoding use `k8s.io/apimachinery` (already in `go.mod`):
- `k8s.io/apimachinery/pkg/util/yaml` for stream splitting
- `k8s.io/apimachinery/pkg/apis/meta/v1/unstructured` + runtime scheme for decoding

Standard library for flag parsing, file I/O, directory walking.

## Design decisions

- **`Load()` not `Add()`** — all objects must be visible before edge resolution. This is exactly the batch use case `Load` was designed for.
- **All resolvers always enabled** — ecosystem resolvers on non-matching resources cost nothing (zero edges produced). No flags needed.
- **No `cobra`/`pflag`** — standard `flag` package is sufficient. Keeps dependency surface at zero.
- **No JSON output yet** — YAGNI. Add `--json` when someone asks for it.

## Testing

1. **YAML reading** — multi-doc splitting, directory walking, invalid YAML (skip with warning, don't crash)
2. **Finding detection** — known object set with deliberate gaps, verify correct dangling edges reported
3. **Filtering** — verify ownerRef and event edges excluded
4. **Output formatting** — snapshot test of human-readable format
5. **Exit code** — 0 when clean, 1 when findings
6. **Integration** — lint logic against real resolver stack using `unstructured.Unstructured` objects
7. **Smoke test** — manual: `helm template <chart> | go run ./cmd/ariadne lint`
