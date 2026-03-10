# TODO

- [x] Add typed-reference support to RefRule — parse maps with kind/name/apiVersion sub-fields to discover target type at runtime (enables RBAC roleRef, HPA scaleTargetRef, CAPI infrastructureRef, etc.).
- [x] Wire typed-reference rules into structural resolver (HPA scaleTargetRef, RoleBinding/ClusterRoleBinding roleRef + subjects, PV claimRef).
- [x] Integration tests for typed-reference resolution (HPA→Deployment, RoleBinding→Role+SA, ClusterRoleBinding→ClusterRole+SA, PV→PVC via claimRef).
- [x] Fix EdgeType mismatch for cluster-scoped resources in resolveBareName/resolveTypedRef (forward used EdgeLocalNameRef, reverse used EdgeNameRef when both are cluster-scoped).

## API surface review

- [x] **Edge.Resolver hardcoded `"rule"` in NewRuleResolver** — threaded `resolverName` through all helper functions so edges are correct at construction. Eliminated overwrite loops in structural/selector/crossplane wrappers.
- [ ] **README says `NameRefRule` but code uses `RefRule`** — README.md:33 references the old type name.
- [ ] **No `Graph.Get()`** — the graph stores `*unstructured.Unstructured` objects but doesn't expose them. Callers and listeners can't retrieve stored objects. Decide: expose via `Graph.Get(ref) (*unstructured.Unstructured, bool)`, or document that callers maintain their own object store.
- [ ] **Event edge direction contradicts dependency convention** — convention is "From depends on To" but event.go:62-68 creates `From: Pod → To: Event`. A Pod doesn't depend on its Event. This means `TopologicalSort` places Events before the Pods they describe. Decide: reverse direction (`Event → Pod`), or document events as a special case of "relatedness" rather than strict dependency.
- [ ] **`EdgeNameRef` vs `EdgeLocalNameRef` naming** — `EdgeNameRef` sounds like the generic case but actually means "fully qualified" (explicit namespace or cluster-scoped). `EdgeLocalNameRef` is the common case (bare name, namespace inferred). Possible renames: `EdgeQualifiedRef`/`EdgeLocalRef`, or collapse both into `EdgeRef` if no consumer needs the distinction.
- [ ] **No `Update` operation documented** — if an object's spec changes, callers must `Remove` + `Add`. This works but isn't documented anywhere. Add doc comment on `Add` or `Graph` mentioning the pattern. **Note:** `Add` for an existing ref overwrites the node (`graph.go:121`) but does not remove stale edges from the previous version — so re-adding a Pod that changed from ConfigMap "A" to "B" leaves edges to both. Either make `Add` handle re-adds correctly (remove old edges first) or document that `Remove` + `Add` is required.
- [ ] **Dedup logic duplicated between `addEdge` and `Load`** — `addEdge` (`graph.go:85-88`) checks for duplicates by linear scan then inserts into both edge maps. `Load` (`graph.go:288-302`) inlines the same logic with a `dup` flag because it batches notifications. A change to the dedup criterion must be applied in both places. Fix: extract a shared insert helper that both call, with a flag controlling inline vs. batched notification.
- [x] **Three wrapper types doing the same `call inner → overwrite Resolver` pattern** — `namedResolver` deleted; overwrite loops removed from `structuralResolver` and `crossplaneResolver` (both still exist for ownerRef/compositeTypeRef logic).
- [ ] **`extractRecursive` / `extractRawRecursive` duplication** — both functions (`rules.go:461-495` and `rules.go:504-541`) implement identical recursive field-path traversal with `[*]` wildcard expansion. The only difference is the leaf type: `[]interface{}` vs `[]string`. A bug or enhancement in path parsing must be applied in two places. Fix: implement `extractFieldValues` as `extractRawValues` + type filter.
- [ ] **Export sort logic duplication** — `ExportDOT` (`export.go:34-58`) and `ExportJSON` (`export.go:93-110`) contain identical node-sorting and edge-sorting code. Fix: extract `sortedNodes` and `sortedEdges` helpers.
- [ ] **Kyverno resolver silently ignores non-core API groups** — `kyverno.go:88,109` checks `ref.Group == ""`, so policies targeting Deployments (`apps` group) produce no edges. Comment at line 27 documents "plain kind names only" but this is easy to miss. Fix: either document more prominently or extend `extractPolicyKinds` to parse group-qualified kinds.

## Performance

- [ ] Unconstrained reverse resolution for subjects rules: when any object is added, the reverse resolver scans all RoleBindings/ClusterRoleBindings because ToKind/ToGroup are empty. For large graphs this is O(objects × bindings). Options: constrain to known subject kinds (ServiceAccount), add a kind-based index to Lookup, or move subjects handling to custom resolver logic. Defer until performance data exists.
- [ ] **ownerRef reverse resolution scans entire graph** — `resolveOwnerRefs` (`structural.go:261`) calls `lookup.ListAll()` for every added object, iterating every node to find objects whose `ownerReferences` point to the new object. This is O(N) per `Add`, making bulk insertion O(N²). At 10K objects this is 100M iterations. Same mitigation options as subjects (namespace-scoped filter, kind index). Defer until benchmarks exist.

## CRD-level typed references

Example resolvers showing how users extend Ariadne for their own CRDs. Not registered by `NewDefault()` — tested as proof that the primitives work.

### Fits existing primitives (RefRule / LabelSelectorRule)

- [x] **Gateway API**: HTTPRoute `spec.rules[*].backendRefs[*]` — typed-ref with kind/name/group/namespace. Exercises cross-namespace typed refs. Also: HTTPRoute `spec.parentRefs[*]` → Gateway. (`gateway.go`)
- [x] **Cluster API**: Machine `spec.infrastructureRef`, Cluster `spec.infrastructureRef`, MachineDeployment `spec.template.spec.infrastructureRef` — all standard typed-refs. Also: Cluster `spec.controlPlaneRef`. (`clusterapi.go`)
- [x] **Crossplane providerConfigRef**: Managed resource `spec.providerConfigRef.name` — bare name ref to ProviderConfig. Callers register their managed resource types via `ManagedResource`. (`crossplane.go`)

### Beyond current primitives

These patterns don't fit RefRule/LabelSelectorRule cleanly. They reveal what a custom `Resolver` implementation looks like vs. declarative rules.

- [x] **Kyverno**: ClusterPolicy/Policy `spec.rules[*].match.resources.kinds` — kind-level matching via custom Resolver. ClusterPolicy matches all namespaces; Policy matches same namespace only. (`kyverno.go`)
- [x] **Argo CD**: Application `spec.destination.namespace` → Namespace + `spec.project` → AppProject — decomposed into two bare name RefRules. (`argocd.go`)
- [x] **Crossplane compositeTypeRef**: Composition `spec.compositeTypeRef` (group+kind, no name) — custom Resolver matching all instances of the referenced GroupKind. (`crossplane.go`)
