# TODO

- [x] Add typed-reference support to RefRule — parse maps with kind/name/apiVersion sub-fields to discover target type at runtime (enables RBAC roleRef, HPA scaleTargetRef, CAPI infrastructureRef, etc.).
- [x] Wire typed-reference rules into structural resolver (HPA scaleTargetRef, RoleBinding/ClusterRoleBinding roleRef + subjects, PV claimRef).
- [x] Integration tests for typed-reference resolution (HPA→Deployment, RoleBinding→Role+SA, ClusterRoleBinding→ClusterRole+SA, PV→PVC via claimRef).
- [x] Fix EdgeType mismatch for cluster-scoped resources in resolveBareName/resolveTypedRef (forward used EdgeLocalNameRef, reverse used EdgeNameRef when both are cluster-scoped).

## Performance

- [ ] Unconstrained reverse resolution for subjects rules: when any object is added, the reverse resolver scans all RoleBindings/ClusterRoleBindings because ToKind/ToGroup are empty. For large graphs this is O(objects × bindings). Options: constrain to known subject kinds (ServiceAccount), add a kind-based index to Lookup, or move subjects handling to custom resolver logic. Defer until performance data exists.

## CRD-level typed references

Patterns for users extending Ariadne with their own resolvers. Not built-in — document and test as examples.

- [ ] Gateway API: HTTPRoute `backendRefs[*]` (kind/name/group/namespace), ReferenceGrant cross-namespace trust.
- [ ] Cluster API: Machine `spec.infrastructureRef`, Cluster `spec.infrastructureRef`, MachineDeployment `spec.template.spec.infrastructureRef`.
- [ ] Kyverno/OPA: ClusterPolicy `spec.rules[*].match.resources` (not a direct ref, but a selector-like pattern over kinds).
- [ ] Argo CD: Application `spec.source` → repo + `spec.destination` → cluster/namespace (composite ref, not a single typed-ref).
- [ ] Crossplane: Composition `spec.compositeTypeRef` (group/kind), managed resource `spec.providerConfigRef` (kind/name).
