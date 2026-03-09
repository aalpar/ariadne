# TODO

- [x] Add typed-reference support to RefRule — parse maps with kind/name/apiVersion sub-fields to discover target type at runtime (enables RBAC roleRef, HPA scaleTargetRef, CAPI infrastructureRef, etc.).
- [x] Wire typed-reference rules into structural resolver (HPA scaleTargetRef, RoleBinding/ClusterRoleBinding roleRef + subjects, PV claimRef).
- [x] Integration tests for typed-reference resolution (HPA→Deployment, RoleBinding→Role+SA, ClusterRoleBinding→ClusterRole+SA, PV→PVC via claimRef).
- [x] Fix EdgeType mismatch for cluster-scoped resources in resolveBareName/resolveTypedRef (forward used EdgeLocalNameRef, reverse used EdgeNameRef when both are cluster-scoped).

## Performance

- [ ] Unconstrained reverse resolution for subjects rules: when any object is added, the reverse resolver scans all RoleBindings/ClusterRoleBindings because ToKind/ToGroup are empty. For large graphs this is O(objects × bindings). Options: constrain to known subject kinds (ServiceAccount), add a kind-based index to Lookup, or move subjects handling to custom resolver logic. Defer until performance data exists.

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
