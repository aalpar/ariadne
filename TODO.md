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
- [x] **Kyverno resolver silently ignores non-core API groups** — parsed group-qualified kind strings (`"Kind"`, `"group/Kind"`, `"group/version/Kind"`) and used parsed group in both forward and reverse resolution.

## Performance

Benchmarks in `bench_test.go` (Apple M4 Max, realistic K8s object mix).

### GroupKind index (done)

Added `map[groupKind]map[namespace][]*node` index to `graphLookup`. `List` and `ListInNamespace` are now O(results) instead of O(all nodes). ownerRef reverse uses `ListByNamespace` for namespaced owners.

| Benchmark | Before | After | Speedup |
|---|---|---|---|
| Load/n=100 | 3.4ms | 2.6ms | 1.3x |
| Load/n=1000 | 138ms | 41ms | 3.4x |
| Load/n=10000 | 13.2s | 1.78s | **7.4x** |
| AddAll/n=10000 | 6.4s | 815ms | **7.8x** |
| AddSingle/graph=10000 | 1.16ms | 174µs | **6.6x** |

### Subjects constraint (done)

Constrained subjects rules to `ToKind: "ServiceAccount"` — User/Group are not API objects. The type guard in `resolveRefReverse` now skips ~95% of objects immediately.

| Benchmark | Before | After | Speedup |
|---|---|---|---|
| Load/n=10000 | 1.73s | 0.93s | **1.9x** |
| AddAll/n=10000 | 796ms | 401ms | **2.0x** |
| AddSingle/graph=10000 | 170µs | 85µs | **2.0x** |

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

### Extra Stuff

- [x] **PodTemplate extraction**: Extract synthetic `core/v1 PodTemplate` from workloads (Deployment, StatefulSet, DaemonSet, ReplicaSet, Job, CronJob) for static YAML analysis. Opt-in via `WithPodTemplates()`. Pod RefRules are mechanically mirrored to PodTemplate rules. Selector rules match against `template.metadata.labels`. (`podtemplate.go`)
- [ ] **Terminology clarification**: Is a K8s object a "resource"? Or is "resource" the registered API type (`kubectl api-resources`)?

## Missing core K8s resolvers

References in core/built-in K8s types not yet covered by `NewDefault()`.

### Fits existing primitives

- [ ] **Webhook → Service**: ValidatingWebhookConfiguration/MutatingWebhookConfiguration `webhooks[*].clientConfig.service` — typed-ref with name+namespace (cross-namespace). Cluster-scoped source. Hits every cluster with admission webhooks.
- [ ] **APIService → Service**: `spec.service` — typed-ref with name+namespace. Cluster-scoped source. Aggregated API servers.
- [ ] **ServiceAccount → Secret**: `spec.secrets[*].name` and `spec.imagePullSecrets[*].name` — bare name refs. Deprecated post-1.24 but still present in many clusters.
- [ ] **VolumeAttachment → PV**: `spec.source.persistentVolumeName` — bare name ref, cluster-scoped source.
- [ ] **VolumeAttachment → Node**: `spec.nodeName` — bare name ref, cluster-scoped source.
- [ ] **PV → CSIDriver**: `spec.csi.driver` — bare name ref matching CSIDriver object name. Both cluster-scoped.
- [ ] **StorageClass → CSIDriver**: `provisioner` — bare name ref matching CSIDriver object name. Both cluster-scoped.

### Beyond current primitives

- [ ] **EndpointSlice → Service**: `metadata.labels["kubernetes.io/service-name"]` — label-based convention, not a field path. Needs a custom resolver or a new rule type for label-key refs.
- [ ] **Namespace dependency**: Every namespaced object implicitly depends on its Namespace. Universal but extremely noisy — should be opt-in (`WithNamespaceDeps()`?). Needs custom resolver iterating all namespaced nodes.

## Missing CRD ecosystem resolvers

Opt-in resolvers for popular CRD ecosystems, same pattern as existing Gateway/Kyverno/etc.

### cert-manager

- [ ] **Certificate → Secret**: `spec.secretName` — bare name ref. The Secret where the cert is stored.
- [ ] **Certificate → Issuer/ClusterIssuer**: `spec.issuerRef` — typed-ref with name+kind+group. ClusterIssuer is cluster-scoped.
- [ ] **Ingress → Certificate**: annotation-based (`cert-manager.io/cluster-issuer`, `cert-manager.io/issuer`). Needs custom resolver reading annotations, not field paths.

### Istio

- [ ] **VirtualService → Service**: `spec.http[*].route[*].destination.host` — host is a DNS name (e.g., `reviews.default.svc.cluster.local` or short name `reviews`). Needs custom resolver to parse host format into namespace+name.
- [ ] **DestinationRule → Service**: `spec.host` — same DNS-name-as-ref pattern as VirtualService.
- [ ] **AuthorizationPolicy → workloads**: `spec.selector` — label-selector-based, fits `LabelSelectorRule`.

### Prometheus / monitoring

- [ ] **ServiceMonitor → Service**: `spec.selector` + `spec.namespaceSelector` — label-selector-based, fits `LabelSelectorRule`. `namespaceSelector` adds cross-namespace matching complexity.
- [ ] **PodMonitor → Pod**: `spec.selector` + `spec.namespaceSelector` — same pattern as ServiceMonitor.

### Flux CD

- [ ] **Kustomization → source**: `spec.sourceRef` — typed-ref with kind/name/namespace. Points to GitRepository, OCIRepository, Bucket, etc.
- [ ] **HelmRelease → HelmChart**: `spec.chartRef` or `spec.chart.spec.sourceRef` — typed-ref. Also HelmRelease → values Secrets/ConfigMaps via `spec.valuesFrom[*]`.
- [ ] **GitRepository → Secret**: `spec.secretRef.name` — bare name ref for auth credentials.

### Tekton

- [ ] **Pipeline → Task**: `spec.tasks[*].taskRef` — typed-ref with name+kind.
- [ ] **PipelineRun → Pipeline**: `spec.pipelineRef` — typed-ref with name+kind.
- [ ] **TaskRun → Task**: `spec.taskRef` — typed-ref with name+kind.

### Knative

- [ ] **Knative Service → Configuration**: ownerRef handles this, but `spec.template` is the source of truth for the Configuration spec.
- [ ] **Route → Knative Service**: `spec.traffic[*].revisionName` or `spec.traffic[*].configurationName` — bare name refs to Revisions or Configurations.
