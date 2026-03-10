# TODO

- [x] Add typed-reference support to RefRule — parse maps with kind/name/apiVersion sub-fields to discover target type at runtime (enables RBAC roleRef, HPA scaleTargetRef, CAPI infrastructureRef, etc.).
- [x] Wire typed-reference rules into structural resolver (HPA scaleTargetRef, RoleBinding/ClusterRoleBinding roleRef + subjects, PV claimRef).
- [x] Integration tests for typed-reference resolution (HPA→Deployment, RoleBinding→Role+SA, ClusterRoleBinding→ClusterRole+SA, PV→PVC via claimRef).
- [x] Fix EdgeType mismatch for cluster-scoped resources in resolveBareName/resolveTypedRef (forward used EdgeLocalNameRef, reverse used EdgeNameRef when both are cluster-scoped).

## API surface review

- [x] **Edge.Resolver hardcoded `"rule"` in NewRuleResolver** — threaded `resolverName` through all helper functions so edges are correct at construction. Eliminated overwrite loops in structural/selector/crossplane wrappers.
- [x] **README says `NameRefRule` but code uses `RefRule`** — README.md:33 references the old type name.
- [x] **No `Graph.Get()`** — added `Get(ref) (*unstructured.Unstructured, bool)`.
- [x] **Event edge direction contradicts dependency convention** — reversed to `From: Event → To: Pod` ("Event depends on Pod"). `Downstream(pod)` now returns Events; `Upstream(event)` returns the involved object.
- [x] **`EdgeNameRef` vs `EdgeLocalNameRef` naming** — collapsed both into `EdgeRef`. The local-vs-qualified distinction is derivable from edge endpoint namespaces.
- [x] **No `Update` operation documented** — `Add` now handles re-adds: removes stale edges, updates the stored object, re-resolves. No `Remove` + `Add` dance needed.
- [x] **Dedup logic duplicated between `addEdge` and `Load`** — extracted `insertEdge` helper used by both.
- [x] **Three wrapper types doing the same `call inner → overwrite Resolver` pattern** — `namedResolver` deleted; overwrite loops removed from `structuralResolver` and `crossplaneResolver` (both still exist for ownerRef/compositeTypeRef logic).
- [x] **`extractRecursive` / `extractRawRecursive` duplication** — `extractFieldValues` now delegates to `extractRawValues` + string type filter. Single traversal implementation.
- [x] **Export sort logic duplication** — extracted `sortedNodes` and `sortedEdges` helpers shared by `ExportDOT` and `ExportJSON`.
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
