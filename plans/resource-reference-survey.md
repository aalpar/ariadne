# K8s Resource Reference Survey

A comprehensive survey of every way Kubernetes resources reference other
things — K8s resources, external systems, and implicit relationships.
This informs the design of Ariadne's resolver system.

**Date:** 2026-03-09

---

## Table of Contents

1. [Reference Mechanism Taxonomy](#1-reference-mechanism-taxonomy)
2. [Current Coverage](#2-current-coverage)
3. [Core K8s — Missing References](#3-core-k8s--missing-references)
4. [Gateway API](#4-gateway-api)
5. [Helm](#5-helm)
6. [Cluster API](#6-cluster-api-capi)
7. [Prometheus Operator](#7-prometheus-operator)
8. [cert-manager](#8-cert-manager)
9. [Other CRD Ecosystems](#9-other-crd-ecosystems)
10. [External / Non-K8s References](#10-external--non-k8s-references)
11. [Architectural Gaps](#11-architectural-gaps)
12. [Priority Matrix](#12-priority-matrix)
13. [Design Decisions](#13-design-decisions)

---

## 1. Reference Mechanism Taxonomy

Every reference in K8s falls into one of these patterns:

### Mechanism 1: Local Name Reference (same namespace, name only)

A field contains a bare string — the name of a resource in the same
namespace. Go type `corev1.LocalObjectReference` or plain `string`.

Examples: `pod.spec.serviceAccountName`, `pod.spec.volumes[*].secret.secretName`

**Ariadne support:** `NameRefRule` with `SameNamespace: true`

### Mechanism 2: Cluster-Scoped Name Reference

Same as above, but the target is cluster-scoped (no namespace), so the
name alone is unambiguous globally.

Examples: `pod.spec.priorityClassName`, `pvc.spec.storageClassName`,
`pod.spec.nodeName`

**Ariadne support:** `NameRefRule` with `SameNamespace: false`

### Mechanism 3: Namespace + Name Reference (cross-namespace)

Explicit namespace and name fields, allowing cross-namespace references.
Go type `corev1.ObjectReference` or ad-hoc struct.

Examples: `roleBinding.subjects[*].{namespace, name}`,
`webhook.clientConfig.service.{namespace, name}`

**Ariadne support:** `NamespacedNameRefRule`

### Mechanism 4: Typed Reference (apiVersion + kind + name [+ namespace])

A "polymorphic pointer" — the target's type is specified alongside its
identity. Used when the reference can point to different kinds. Go types:
`corev1.ObjectReference`, `corev1.TypedLocalObjectReference`,
`corev1.TypedObjectReference`.

Examples: `ownerReferences[*].{apiVersion, kind, name, uid}`,
`hpa.spec.scaleTargetRef.{apiVersion, kind, name}`,
CAPI `spec.infrastructureRef`

**Ariadne support:** ownerReferences only (special-cased in `structural.go`).
No general-purpose typed ref rule.

### Mechanism 5: Label Selector

Selects a **set** of resources by label match. Can be `matchLabels`
(equality) or `matchExpressions` (set-based). Go type:
`metav1.LabelSelector`.

Examples: `service.spec.selector`, `deployment.spec.selector`,
`networkPolicy.spec.podSelector`

**Ariadne support:** `LabelSelectorRule` (matchLabels only — see
[architectural gaps](#matchexpressions-gap))

### Mechanism 6: Label Selector + Namespace Selector (compound)

Combines a label selector with a namespace selector to select resources
across namespaces. The namespace selector is itself a label selector
matching Namespace objects.

Examples: `networkPolicy.spec.ingress[*].from[*].{podSelector, namespaceSelector}`,
`prometheus.spec.serviceMonitorNamespaceSelector`

**Ariadne support:** Not supported.

### Mechanism 7: Annotation/Label-Based Ownership

Resources are grouped by shared annotation/label values. Not a
field-path reference — it's a convention.

Examples: Helm `meta.helm.sh/release-name`, `kubernetes.io/service-name`
on EndpointSlices, Strimzi `strimzi.io/cluster` label

**Ariadne support:** Not supported.

### Mechanism 8: String Reference to External System

A field contains a string that identifies something outside the K8s API —
a container image, a DNS name, a cloud resource ID, a URL.

Examples: `containers[*].image`, `service.spec.externalName`,
`storageClass.provisioner`, `webhook.clientConfig.url`

**Ariadne support:** Out of scope (not K8s-to-K8s).

### Mechanism 9: Implicit Containment / Same-Name Binding

Not a field reference. The relationship is structural: namespaced
resources belong to their namespace; Endpoints share a name with their
Service; CSINode has the same name as its Node.

**Ariadne support:** Not supported.

### Mechanism 10: Status-Field References

References in `.status` rather than `.spec`. System-set, representing
observed state.

Examples: `pod.status.nominatedNodeName`, `pv.spec.claimRef` (technically
spec, but system-managed binding), ArgoCD `status.resources`

**Ariadne support:** Not supported.

### Summary Table

| # | Mechanism                    | Prevalence | Ariadne Support              |
|---|------------------------------|------------|------------------------------|
| 1 | Local name (same NS)         | Very high  | `NameRefRule`                |
| 2 | Cluster-scoped name          | High       | `NameRefRule`                |
| 3 | Namespace + name             | High       | `NamespacedNameRefRule`      |
| 4 | Typed reference (GVK+name)   | High (CRDs)| ownerRefs only               |
| 5 | Label selector               | High       | `LabelSelectorRule` (partial)|
| 6 | Selector + NS selector       | Medium     | Not supported                |
| 7 | Annotation/label ownership   | Medium     | Not supported                |
| 8 | String ref to external       | Medium     | Out of scope                 |
| 9 | Implicit containment         | Low        | Not supported                |
| 10| Status references            | Low        | Not supported                |

---

## 2. Current Coverage

What the built-in resolvers handle today.

### Structural Resolver (`structural.go`)

| From | To | Field Path | Mechanism |
|------|----|------------|-----------|
| Pod | ServiceAccount | `spec.serviceAccountName` | Local name |
| Pod | ConfigMap | `spec.volumes[*].configMap.name` | Local name |
| Pod | ConfigMap | `spec.containers[*].envFrom[*].configMapRef.name` | Local name |
| Pod | Secret | `spec.volumes[*].secret.secretName` | Local name |
| Pod | Secret | `spec.containers[*].envFrom[*].secretRef.name` | Local name |
| Pod | PVC | `spec.volumes[*].persistentVolumeClaim.claimName` | Local name |
| PVC | PV | `spec.volumeName` | Cluster name |
| PVC | StorageClass | `spec.storageClassName` | Cluster name |
| Ingress | Service | `spec.rules[*].http.paths[*].backend.service.name` | Local name |
| Any | Any (owner) | `metadata.ownerReferences[*]` | Typed ref |

### Selector Resolver (`selector.go`)

| From | To | Selector Field Path | Mechanism |
|------|----|---------------------|-----------|
| Service | Pod | `spec.selector` | Label selector |
| NetworkPolicy | Pod | `spec.podSelector` | Label selector |
| PDB | Pod | `spec.selector.matchLabels` | Label selector |

### Event Resolver (`event.go`)

| From | To | Field | Mechanism |
|------|----|-------|-----------|
| Event | Any | `involvedObject` | Typed ref |

---

## 3. Core K8s — Missing References

### 3.1 Pod References

#### Environment Variable Key References

| To | Field Path | Notes |
|----|------------|-------|
| Secret | `spec.containers[*].env[*].valueFrom.secretKeyRef.name` | Per-key ref |
| ConfigMap | `spec.containers[*].env[*].valueFrom.configMapKeyRef.name` | Per-key ref |

Same patterns apply to `initContainers[*]` and `ephemeralContainers[*]`.

#### Image Pull Secrets

| To | Field Path |
|----|------------|
| Secret | `spec.imagePullSecrets[*].name` |

#### Projected Volumes

| To | Field Path |
|----|------------|
| ConfigMap | `spec.volumes[*].projected.sources[*].configMap.name` |
| Secret | `spec.volumes[*].projected.sources[*].secret.name` |

#### Volume Plugin Secret Refs

| To | Field Path | Volume Plugin |
|----|------------|---------------|
| Secret | `spec.volumes[*].csi.nodePublishSecretRef.name` | CSI |
| Secret | `spec.volumes[*].iscsi.secretRef.name` | iSCSI CHAP |
| Secret | `spec.volumes[*].rbd.secretRef.name` | Ceph RBD |
| Secret | `spec.volumes[*].cephfs.secretRef.name` | CephFS |
| Secret | `spec.volumes[*].azureFile.secretName` | Azure File |
| Secret | `spec.volumes[*].flexVolume.secretRef.name` | FlexVolume |
| Secret | `spec.volumes[*].storageos.secretRef.name` | StorageOS |
| Secret | `spec.volumes[*].scaleIO.secretRef.name` | ScaleIO |

#### Cluster-Scoped Name Refs

| To | Field Path | API Group |
|----|------------|-----------|
| Node | `spec.nodeName` | core |
| PriorityClass | `spec.priorityClassName` | scheduling.k8s.io |
| RuntimeClass | `spec.runtimeClassName` | node.k8s.io |

#### Dynamic Resource Allocation (resource.k8s.io)

| To | Field Path |
|----|------------|
| ResourceClaim | `spec.resourceClaims[*].resourceClaimName` |
| ResourceClaimTemplate | `spec.resourceClaims[*].resourceClaimTemplateName` |

#### Node Scheduling (label-based, not direct name refs)

| To | Field Path | Type |
|----|------------|------|
| Node | `spec.nodeSelector` | Label match (flat map) |
| Node | `spec.affinity.nodeAffinity.required...nodeSelectorTerms[*].matchExpressions` | Label expressions |
| Node | `spec.affinity.nodeAffinity.preferred...[*].preference.matchExpressions` | Soft preference |
| Pod | `spec.affinity.podAffinity.required...[*].labelSelector` | Co-location |
| Pod | `spec.affinity.podAntiAffinity.required...[*].labelSelector` | Separation |
| Node | `spec.topologySpreadConstraints[*].topologyKey` | Topology (label key, not a resource ref) |

#### Hand-Unrolled Pattern: Container Arrays

Every env, envFrom, and volumeMount rule that applies to
`spec.containers[*]` also applies identically to:
- `spec.initContainers[*]`
- `spec.ephemeralContainers[*]`

This is three copies of the same rules differing only in the array name.

### 3.2 Workload Controllers

#### Embedded Pod Templates

These controllers embed a `PodTemplateSpec`. Every Pod reference above
applies transitively through the template path prefix:

| Controller | Template Path Prefix |
|------------|---------------------|
| Deployment (apps) | `spec.template.spec.` |
| ReplicaSet (apps) | `spec.template.spec.` |
| StatefulSet (apps) | `spec.template.spec.` |
| DaemonSet (apps) | `spec.template.spec.` |
| Job (batch) | `spec.template.spec.` |
| CronJob (batch) | `spec.jobTemplate.spec.template.spec.` |

#### Controller-Specific References

| From | To | Field Path | Mechanism |
|------|----|------------|-----------|
| Deployment | Pod | `spec.selector` | Label selector |
| ReplicaSet | Pod | `spec.selector` | Label selector |
| StatefulSet | Pod | `spec.selector` | Label selector |
| DaemonSet | Pod | `spec.selector` | Label selector |
| Job | Pod | `spec.selector` | Label selector |
| StatefulSet | Service | `spec.serviceName` | Local name |
| StatefulSet | StorageClass | `spec.volumeClaimTemplates[*].spec.storageClassName` | Cluster name |

### 3.3 Ingress (missing)

| To | Field Path | Mechanism |
|----|------------|-----------|
| IngressClass | `spec.ingressClassName` | Cluster name |
| Secret | `spec.tls[*].secretName` | Local name |
| Service | `spec.defaultBackend.service.name` | Local name |
| Any | `spec.defaultBackend.resource.{apiGroup, kind, name}` | Typed ref |
| Any | `spec.rules[*].http.paths[*].backend.resource.{apiGroup, kind, name}` | Typed ref |

### 3.4 NetworkPolicy (missing)

Currently only `spec.podSelector` is modeled.

| To | Field Path | Mechanism |
|----|------------|-----------|
| Pod | `spec.ingress[*].from[*].podSelector` | Label selector |
| Namespace | `spec.ingress[*].from[*].namespaceSelector` | Label selector |
| Pod | `spec.egress[*].to[*].podSelector` | Label selector |
| Namespace | `spec.egress[*].to[*].namespaceSelector` | Label selector |
| (external) | `spec.ingress[*].from[*].ipBlock.cidr` | String ref |
| (external) | `spec.egress[*].to[*].ipBlock.cidr` | String ref |

Note: `podSelector` and `namespaceSelector` can be combined in the same
`from`/`to` element ("pods matching X in namespaces matching Y"). This
compound selector is Mechanism 6.

### 3.5 RBAC

| From | To | Field Path | Mechanism |
|------|----|------------|-----------|
| RoleBinding | Role/ClusterRole | `roleRef.{apiGroup, kind, name}` | Typed ref |
| RoleBinding | ServiceAccount/User/Group | `subjects[*].{kind, name, namespace, apiGroup}` | Typed ref |
| ClusterRoleBinding | ClusterRole | `roleRef.{apiGroup, kind, name}` | Typed ref |
| ClusterRoleBinding | ServiceAccount/User/Group | `subjects[*].{kind, name, namespace, apiGroup}` | Typed ref |
| ClusterRole | ClusterRole | `aggregationRule.clusterRoleSelectors[*]` | Label selector |

Both `roleRef` and `subjects[*]` are typed references — they carry
`{apiGroup, kind, name}`. This means `TypedRefRule` handles all of RBAC:

```go
NewRuleResolver("rbac",
    TypedRefRule{..., FromKind: "RoleBinding",        RefFieldPath: "roleRef"},
    TypedRefRule{..., FromKind: "RoleBinding",        RefFieldPath: "subjects[*]"},
    TypedRefRule{..., FromKind: "ClusterRoleBinding",  RefFieldPath: "roleRef"},
    TypedRefRule{..., FromKind: "ClusterRoleBinding",  RefFieldPath: "subjects[*]"},
)
```

`subjects[*].kind` can be `User` or `Group` — these are external
identities, not K8s resources. No special filtering is needed: the
`lookup.Get` call returns false for Users and Groups because they don't
exist as nodes in the graph. The graph is the filter.

`roleRef.kind` determines whether the target is Role (same namespace) or
ClusterRole (cluster-scoped). The namespace fallback in TypedRefRule
handles this: try same-namespace first, then cluster-scoped.

### 3.6 Admission Webhooks and APIService

| From | To | Field Path | Mechanism |
|------|----|------------|-----------|
| ValidatingWebhookConfig | Service | `webhooks[*].clientConfig.service.{name, namespace}` | NS+name |
| MutatingWebhookConfig | Service | `webhooks[*].clientConfig.service.{name, namespace}` | NS+name |
| APIService | Service | `spec.service.{name, namespace}` | NS+name |
| Both webhooks | (external) | `webhooks[*].clientConfig.url` | String ref |
| Both webhooks | Namespace | `webhooks[*].namespaceSelector` | Label selector |

### 3.7 HPA (HorizontalPodAutoscaler)

| To | Field Path | Mechanism |
|----|------------|-----------|
| Deployment/RS/SS/etc. | `spec.scaleTargetRef.{apiVersion, kind, name}` | Typed ref |
| Any (object metric) | `spec.metrics[*].object.describedObject.{apiVersion, kind, name}` | Typed ref |

### 3.8 PersistentVolume (missing)

| To | Field Path | Mechanism |
|----|------------|-----------|
| StorageClass | `spec.storageClassName` | Cluster name |
| PVC | `spec.claimRef.{name, namespace}` | NS+name (binding) |
| Node | `spec.nodeAffinity.required.nodeSelectorTerms[*]` | Label expressions |
| Secret | `spec.csi.nodePublishSecretRef.{name, namespace}` | NS+name |
| Secret | `spec.csi.nodeStageSecretRef.{name, namespace}` | NS+name |
| Secret | `spec.csi.controllerPublishSecretRef.{name, namespace}` | NS+name |
| Secret | `spec.csi.controllerExpandSecretRef.{name, namespace}` | NS+name |
| (cloud) | `spec.awsElasticBlockStore.volumeID` | External string |
| (cloud) | `spec.gcePersistentDisk.pdName` | External string |
| (cloud) | `spec.azureDisk.diskURI` | External string |
| (cloud) | `spec.nfs.server` | External string |

### 3.9 EndpointSlice / Endpoints

| From | To | Field Path | Mechanism |
|------|----|------------|-----------|
| Endpoints | Service | (same name in same namespace) | Implicit same-name |
| Endpoints | Pod | `subsets[*].addresses[*].targetRef.{name, namespace}` | Typed ref |
| Endpoints | Node | `subsets[*].addresses[*].nodeName` | Cluster name |
| EndpointSlice | Service | `metadata.labels["kubernetes.io/service-name"]` | Label ownership |
| EndpointSlice | Service | `metadata.ownerReferences` | Typed ref |
| EndpointSlice | Pod | `endpoints[*].targetRef.{name, namespace}` | NS+name |
| EndpointSlice | Node | `endpoints[*].nodeName` | Cluster name |

### 3.10 CSI / Storage

| From | To | Field Path | Mechanism |
|------|----|------------|-----------|
| CSINode | Node | `metadata.name` (same name) | Implicit same-name |
| CSINode | CSIDriver | `spec.drivers[*].name` | Cluster name |
| VolumeAttachment | Node | `spec.nodeName` | Cluster name |
| VolumeAttachment | PV | `spec.source.persistentVolumeName` | Cluster name |

### 3.11 ServiceAccount (pre-1.24)

| To | Field Path |
|----|------------|
| Secret | `secrets[*].name` |
| Secret | `imagePullSecrets[*].name` |

### 3.12 ResourceClaim (resource.k8s.io)

| From | To | Field Path | Mechanism |
|------|----|------------|-----------|
| ResourceClaim | DeviceClass | `spec.requests[*].deviceClassName` | Cluster name |
| ResourceClaimTemplate | DeviceClass | `spec.template.spec.requests[*].deviceClassName` | Cluster name |
| ResourceSlice | Node | `spec.nodeName` | Cluster name |

---

## 4. Gateway API

Gateway API uses typed references pervasively and introduces
`ReferenceGrant` for cross-namespace authorization.

| From | To | Field Path | Mechanism | Cross-NS |
|------|----|------------|-----------|----------|
| Gateway | GatewayClass | `spec.gatewayClassName` | Cluster name | N/A |
| Gateway | Secret | `spec.listeners[*].tls.certificateRefs[*].{group, kind, name, namespace}` | Typed ref | Yes (ReferenceGrant) |
| HTTPRoute | Gateway | `spec.parentRefs[*].{group, kind, name, namespace, sectionName}` | Typed ref | Yes |
| HTTPRoute | Service | `spec.rules[*].backendRefs[*].{group, kind, name, namespace}` | Typed ref | Yes |
| GRPCRoute | Gateway | `spec.parentRefs[*]` | Typed ref | Yes |
| GRPCRoute | Service | `spec.rules[*].backendRefs[*]` | Typed ref | Yes |
| TCPRoute | Gateway | `spec.parentRefs[*]` | Typed ref | Yes |
| TCPRoute | Service | `spec.rules[*].backendRefs[*]` | Typed ref | Yes |
| TLSRoute | Gateway | `spec.parentRefs[*]` | Typed ref | Yes |
| UDPRoute | Gateway | `spec.parentRefs[*]` | Typed ref | Yes |
| UDPRoute | Service | `spec.rules[*].backendRefs[*]` | Typed ref | Yes |
| ReferenceGrant | Namespace | `spec.from[*].namespace` | Cluster name | N/A |
| Gateway | Namespace | `spec.listeners[*].allowedRoutes.namespaces.selector` | Label selector | Yes |

All five route types (HTTP, GRPC, TCP, TLS, UDP) have identical
`parentRefs` and `backendRefs` structures. This is a hand-unrolled loop
— one `TypedRefRule` parameterized by kind covers all five.

`ReferenceGrant` introduces a gated cross-namespace reference pattern:
the edge exists only if both the reference AND the grant exist.

---

## 5. Helm

Helm doesn't use CRDs for its core model. It uses annotations and labels
on managed resources, plus Secret objects to store release state.

### Annotations on Managed Resources

- `meta.helm.sh/release-name: <release-name>`
- `meta.helm.sh/release-namespace: <release-namespace>`

### Labels on Managed Resources

- `app.kubernetes.io/managed-by: Helm`
- `helm.sh/chart: <chart-name>-<version>`
- `app.kubernetes.io/instance: <release-name>`

### Helm Hooks (annotations on resources)

- `helm.sh/hook: pre-install|post-install|pre-upgrade|post-upgrade|pre-delete|post-delete|pre-rollback|post-rollback|test`
- `helm.sh/hook-weight: "5"` (ordering within a phase)
- `helm.sh/hook-delete-policy: before-hook-creation|hook-succeeded|hook-failed`

### Helm Release Secrets

Helm stores release history as Secrets:

- **Type:** `helm.sh/release.v1`
- **Namespace:** release namespace
- **Name pattern:** `sh.helm.release.v1.<release-name>.v<revision>`
- **Labels:** `name: <release-name>`, `owner: helm`, `status: deployed|superseded`

### Modeling Strategy

Helm doesn't use ownerReferences (releases can span cluster-scoped
resources). Three approaches:

1. **Annotation grouping:** Resources with matching `meta.helm.sh/release-name`
   + `meta.helm.sh/release-namespace` belong together.
2. **Edge to release Secret:** Each managed resource → its
   `sh.helm.release.v1.<name>.v<N>` Secret.
3. **Virtual owner node:** Create a synthetic node representing the
   release, with edges from all managed resources.

This requires Mechanism 7 (annotation/label-based ownership) — a
pattern no current rule type can express.

---

## 6. Cluster API (CAPI)

CAPI is the canonical example of Mechanism 4 (typed references).

| From | To | Field Path | Mechanism |
|------|----|------------|-----------|
| Cluster | InfraProvider | `spec.infrastructureRef.{apiVersion, kind, name, namespace}` | Typed ref |
| Cluster | ControlPlane | `spec.controlPlaneRef.{apiVersion, kind, name, namespace}` | Typed ref |
| Machine | InfraProvider | `spec.infrastructureRef.{apiVersion, kind, name, namespace}` | Typed ref |
| Machine | BootstrapProvider | `spec.bootstrap.configRef.{apiVersion, kind, name, namespace}` | Typed ref |
| Machine | Cluster | `spec.clusterName` | Local name |
| MachineDeployment | Cluster | `spec.clusterName` | Local name |
| MachineDeployment | InfraTemplate | `spec.template.spec.infrastructureRef` | Typed ref |
| MachineDeployment | BootstrapTemplate | `spec.template.spec.bootstrap.configRef` | Typed ref |
| MachineSet | Cluster | `spec.clusterName` | Local name |
| MachinePool | Cluster | `spec.clusterName` | Local name |
| MachineHealthCheck | Cluster | `spec.clusterName` | Local name |
| MachineHealthCheck | RemediationTemplate | `spec.remediationTemplate.{apiVersion, kind, name, namespace}` | Typed ref |
| ClusterClass | InfraTemplate | `spec.infrastructure.ref` | Typed ref |
| ClusterClass | ControlPlane | `spec.controlPlane.ref` | Typed ref |
| ClusterClass | Worker templates | `spec.workers.machineDeployments[*].template.infrastructure.ref` | Typed ref |

All typed references follow the same `{apiVersion, kind, name, namespace?}`
structure. A single `TypedRefRule` type covers all of these.

---

## 7. Prometheus Operator

Prometheus Operator (monitoring.coreos.com) uses a two-level selector
pattern: the Prometheus CR selects ServiceMonitors by label, and each
ServiceMonitor selects Services by label.

### Cross-CRD Selector References

| From | To | Selector Field | NS Selector Field |
|------|----|----------------|-------------------|
| Prometheus | ServiceMonitor | `spec.serviceMonitorSelector` | `spec.serviceMonitorNamespaceSelector` |
| Prometheus | PodMonitor | `spec.podMonitorSelector` | `spec.podMonitorNamespaceSelector` |
| Prometheus | PrometheusRule | `spec.ruleSelector` | `spec.ruleNamespaceSelector` |
| Prometheus | ScrapeConfig | `spec.scrapeConfigSelector` | `spec.scrapeConfigNamespaceSelector` |
| Alertmanager | AlertmanagerConfig | `spec.alertmanagerConfigSelector` | `spec.alertmanagerConfigNamespaceSelector` |
| ServiceMonitor | Service | `spec.selector` | `spec.namespaceSelector` |
| PodMonitor | Pod | `spec.selector` | `spec.namespaceSelector` |
| Probe | Ingress | `spec.targets.ingress.selector` | — |

### Direct Name References

| From | To | Field Path |
|------|----|------------|
| Prometheus | ServiceAccount | `spec.serviceAccountName` |
| Prometheus | Secret | `spec.secrets[*]` |
| Prometheus | ConfigMap | `spec.configMaps[*]` |
| Prometheus | Secret | `spec.additionalScrapeConfigs.name` |
| Prometheus | Secret | `spec.remoteWrite[*].basicAuth.password.name` |
| Alertmanager | Secret | `spec.configSecret` |
| Alertmanager | ServiceAccount | `spec.serviceAccountName` |

The compound selector + namespace selector pattern (Mechanism 6)
appears throughout this ecosystem.

---

## 8. cert-manager

| From | To | Field Path | Mechanism |
|------|----|------------|-----------|
| Certificate | Issuer/ClusterIssuer | `spec.issuerRef.{name, kind, group}` | Typed ref |
| Certificate | Secret | `spec.secretName` | Local name |
| Certificate | Secret | `spec.keystores.jks.passwordSecretRef.name` | Local name |
| Certificate | Secret | `spec.keystores.pkcs12.passwordSecretRef.name` | Local name |
| CertificateRequest | Issuer/ClusterIssuer | `spec.issuerRef.{name, kind, group}` | Typed ref |
| Issuer | Secret | `spec.ca.secretName` | Local name |
| Issuer | Secret | `spec.acme.privateKeySecretRef.name` | Local name |
| Issuer | Secret | `spec.acme.solvers[*].dns01.*.apiTokenSecretRef.name` | Local name |
| Issuer | Secret | `spec.vault.auth.tokenSecretRef.name` | Local name |
| ClusterIssuer | Secret | (same paths as Issuer) | Local name |

`spec.issuerRef.kind` determines whether the target is namespace-scoped
(Issuer) or cluster-scoped (ClusterIssuer). This is the same
conditional-kind pattern as RBAC's `roleRef`.

---

## 9. Other CRD Ecosystems

### 9.1 Istio

| From | To | Field Path | Mechanism |
|------|----|------------|-----------|
| VirtualService | Gateway | `spec.gateways[*]` | `namespace/name` string |
| VirtualService | Service | `spec.http[*].route[*].destination.host` | DNS name |
| DestinationRule | Service | `spec.host` | DNS name |
| Gateway | Secret | `spec.servers[*].tls.credentialName` | Local name |
| AuthorizationPolicy | SA | `spec.rules[*].from[*].source.principals[*]` | SPIFFE string |
| PeerAuthentication | Pod | `spec.selector.matchLabels` | Label selector |
| Sidecar | Service | `spec.egress[*].hosts[*]` | `namespace/host` |

Istio's `host` field uses K8s short names (`reviews`) or FQDNs
(`reviews.prod.svc.cluster.local`). Resolving these requires DNS
convention parsing.

### 9.2 ArgoCD

| From | To | Field Path | Mechanism |
|------|----|------------|-----------|
| Application | AppProject | `spec.project` | Name ref |
| Application | (cluster) | `spec.destination.server` | External URL |
| Application | Namespace | `spec.destination.namespace` | Name ref |
| Application | (git) | `spec.source.repoURL` | External URL |
| Application | Managed resources | `status.resources[*].{group, version, kind, namespace, name}` | Status typed ref |

### 9.3 Crossplane

| From | To | Field Path | Mechanism |
|------|----|------------|-----------|
| Claim (XRC) | Composite (XR) | `spec.resourceRef.{apiVersion, kind, name}` | Typed ref |
| Composite | Managed Resource | `spec.resourceRefs[*].{apiVersion, kind, name}` | Typed ref |
| Composite | Composition | `spec.compositionRef.name` | Cluster name |
| Composition | XRD | `spec.compositeTypeRef.{apiVersion, kind}` | GVK ref (type, not instance) |
| ManagedResource | ProviderConfig | `spec.providerConfigRef.name` | Cluster name |
| ProviderConfig | Secret | `spec.credentials.secretRef.{name, namespace}` | NS+name |

### 9.4 Knative

| From | To | Field Path | Mechanism |
|------|----|------------|-----------|
| Route | Revision | `spec.traffic[*].revisionName` | Local name |
| Route | Configuration | `spec.traffic[*].configurationName` | Local name |
| DomainMapping | Knative Service | `spec.ref.{apiVersion, kind, name, namespace}` | Typed ref |
| Service→Config→Revision→Pod | ownerRef chain | `metadata.ownerReferences` | Typed ref |

### 9.5 Tekton

| From | To | Field Path | Mechanism |
|------|----|------------|-----------|
| Pipeline | Task | `spec.tasks[*].taskRef.{name, kind, apiVersion}` | Typed ref |
| PipelineRun | Pipeline | `spec.pipelineRef.{name, apiVersion}` | Typed ref |
| TaskRun | Task | `spec.taskRef.{name, kind, apiVersion}` | Typed ref |
| PipelineRun | Secret | `spec.workspaces[*].secret.secretName` | Local name |
| PipelineRun | ConfigMap | `spec.workspaces[*].configMap.name` | Local name |
| PipelineRun | PVC | `spec.workspaces[*].persistentVolumeClaim.claimName` | Local name |
| Pipeline | Task | `spec.finally[*].taskRef.{name, kind}` | Typed ref |

### 9.6 KEDA

| From | To | Field Path | Mechanism |
|------|----|------------|-----------|
| ScaledObject | Deployment/SS | `spec.scaleTargetRef.{apiVersion, kind, name}` | Typed ref |
| ScaledObject | TriggerAuth | `spec.triggers[*].authenticationRef.name` | Local name |
| ScaledJob | Job target | `spec.jobTargetRef.{apiVersion, kind, name}` | Typed ref |
| TriggerAuth | Secret | `spec.secretTargetRef[*].name` | Local name |
| TriggerAuth | ConfigMap | `spec.configMapTargetRef[*].name` | Local name |

### 9.7 Strimzi (Kafka)

| From | To | Field Path | Mechanism |
|------|----|------------|-----------|
| KafkaTopic | Kafka | `metadata.labels["strimzi.io/cluster"]` | Label name ref |
| KafkaUser | Kafka | `metadata.labels["strimzi.io/cluster"]` | Label name ref |
| KafkaConnect | Kafka | `spec.bootstrapServers` | Connection string |
| KafkaBridge | Kafka | `spec.bootstrapServers` | Connection string |

---

## 10. External / Non-K8s References

These are "leaf edges" — they point outside the K8s API. Listed for
completeness; generally out of scope for Ariadne.

| Source | Field Path | Points To |
|--------|------------|-----------|
| Pod | `spec.containers[*].image` | Container registry |
| Pod | `spec.initContainers[*].image` | Container registry |
| Service | `spec.externalName` | External DNS |
| Ingress | `spec.rules[*].host` | External DNS (declaration) |
| StorageClass | `provisioner` | CSI driver / cloud |
| IngressClass | `spec.controller` | Ingress controller |
| PV | `spec.csi.volumeHandle` | Cloud volume |
| PV | `spec.awsElasticBlockStore.volumeID` | AWS EBS |
| PV | `spec.gcePersistentDisk.pdName` | GCE PD |
| PV | `spec.azureDisk.diskURI` | Azure Disk |
| PV | `spec.nfs.server` | NFS server |
| Webhook | `webhooks[*].clientConfig.url` | External URL |
| Node | `spec.providerID` | Cloud instance |

**Recommendation:** Don't model as graph edges. These are data properties
of resources, not dependencies on other K8s resources in the graph.

---

## 11. Architectural Gaps

### matchExpressions Gap

The current `LabelSelectorRule` implementation uses
`extractMapValue` → `labels.SelectorFromSet`, which only handles
equality-based selectors (`matchLabels`). It cannot handle
`matchExpressions` (In, NotIn, Exists, DoesNotExist operators).

This affects:
- NetworkPolicy `spec.podSelector` (LabelSelector struct, not flat map)
- Deployment/RS/SS/DS `spec.selector` (LabelSelector with both matchLabels and matchExpressions)
- ClusterRole `aggregationRule.clusterRoleSelectors`
- Many CRD selectors

Fix: Parse the full `metav1.LabelSelector` structure using
`metav1.LabelSelectorAsSelector()` from `k8s.io/apimachinery` (already
a dependency). The `SelectorFieldPath` should point to the LabelSelector
object, and the resolver should handle both `matchLabels` and
`matchExpressions` sub-fields.

Note: `Service.spec.selector` is a plain `map[string]string]`, NOT a
LabelSelector. The current flat-map approach is correct for Services.
The fix needs to handle both formats or use separate rule types.

### Unified Reference Rule: `RefRule`

The survey reveals that `NameRefRule`, `NamespacedNameRefRule`, and the
proposed `TypedRefRule` are all points on a single spectrum. The
variable is how much of the target's identity is known at rule-definition
time vs. discovered at runtime from the object data.

This is analogous to Go's type system:

| Go type | Target constraint | What the rule knows | What the data provides |
|---------|-------------------|---------------------|----------------------|
| `any` | unconstrained | nothing | group, kind, namespace, name |
| `interface{ ... }` | partial | e.g., group only | kind, namespace, name |
| `*ServiceAccount` | fully constrained | group, kind | namespace, name |
| `*ObjectRef{...}` | fully qualified | group, kind, namespace, name | (verification only) |

The base assumption is that **the target is resolved at runtime**.
Specifying the target type at definition time is a constraint — an
optional narrowing, not a different mechanism.

#### Design

```go
// RefRule is the unified reference rule.
//
// RefFieldPath points to the reference data in the source object.
// The data at that path may be:
//   - A typed ref (map with kind/name/group sub-fields)
//   - A bare name (string)
//   - An array of either (via [*] in the path)
//
// ToGroup/ToKind optionally constrain the target type:
//   - Empty (any):           target type discovered from object data
//   - Partial (group only):  target must be in this group, kind from data
//   - Full (group + kind):   target must be this exact type
//
// When the data is a bare name string, ToGroup/ToKind are required
// because the data has no type information to discover.
//
// SameNamespace controls namespace defaulting when the reference data
// doesn't include an explicit namespace.
type RefRule struct {
    FromGroup, FromKind string
    RefFieldPath        string

    ToGroup, ToKind     string // optional type constraint
    SameNamespace       bool   // default namespace to source's namespace
}
```

#### How it works

The resolver navigates to `RefFieldPath` and inspects the value:

**If the value is a map** (typed reference):
1. Read `kind`, `name` from sub-fields (always these names)
2. Read group from `apiVersion` (parse), `apiGroup`, or `group` sub-field
3. Read `namespace` if present; else default per `SameNamespace`
4. If `ToGroup`/`ToKind` are set, verify the extracted type matches — skip
   if it doesn't (type constraint acts as a filter)
5. Construct `ObjectRef`, call `lookup.Get`

**If the value is a string** (bare name):
1. `ToGroup`/`ToKind` must be set (data has no type information)
2. Construct `ObjectRef` from the string name + rule's `ToGroup`/`ToKind`
3. Namespace from source object (if `SameNamespace`) or empty
4. Call `lookup.Get`

**Namespace fallback** when the data has no explicit namespace: try
same-namespace first, then cluster-scoped (`""`). At most one succeeds
because K8s doesn't allow the same GroupKind to be both namespaced and
cluster-scoped. This handles conditional-kind cases (Role vs
ClusterRole, Issuer vs ClusterIssuer) naturally.

**Non-K8s targets** (RBAC User/Group, external URLs) are automatically
excluded: `lookup.Get` returns false because they aren't nodes in the
graph. The graph is the filter.

**Reverse resolution:** List all `FromKind` objects, read their refs,
check for match. When `ToGroup`/`ToKind` are set, short-circuit: skip
if the newly-added object doesn't match the constraint. When
unconstrained, scan all `FromKind` objects (bounded, small for
CRD-specific rules).

#### The spectrum in practice

```go
// Fully unconstrained (any) — target type from object data
// CAPI: spec.infrastructureRef could point to AWSCluster, AzureCluster, etc.
RefRule{
    FromGroup: "cluster.x-k8s.io", FromKind: "Cluster",
    RefFieldPath: "spec.infrastructureRef",
}

// Partially constrained (interface) — must be in rbac group, kind from data
// RBAC roleRef: kind is Role or ClusterRole, determined at runtime
RefRule{
    FromGroup: "rbac.authorization.k8s.io", FromKind: "RoleBinding",
    RefFieldPath: "roleRef",
    ToGroup: "rbac.authorization.k8s.io",
}

// Fully constrained (concrete type) — target type known, name from data
// Pod → ServiceAccount: field is just a name string
RefRule{
    FromGroup: "", FromKind: "Pod",
    RefFieldPath: "spec.serviceAccountName",
    ToGroup: "", ToKind: "ServiceAccount",
    SameNamespace: true,
}

// Fully qualified (literal) — everything known, data is verification
// Webhook → specific Service: namespace and name in the object data,
// but we know the target must be a Service
RefRule{
    FromGroup: "admissionregistration.k8s.io",
    FromKind: "ValidatingWebhookConfiguration",
    RefFieldPath: "webhooks[*].clientConfig.service",
    ToGroup: "", ToKind: "Service",
}
```

#### Unified examples

```go
// RBAC — all reference patterns, one rule type
NewRuleResolver("rbac",
    // roleRef: partially constrained (must be rbac group)
    RefRule{
        FromGroup: "rbac.authorization.k8s.io", FromKind: "RoleBinding",
        RefFieldPath: "roleRef",
        ToGroup: "rbac.authorization.k8s.io",
    },
    // subjects: unconstrained (ServiceAccount, User, Group — graph filters)
    RefRule{
        FromGroup: "rbac.authorization.k8s.io", FromKind: "RoleBinding",
        RefFieldPath: "subjects[*]",
    },
    RefRule{
        FromGroup: "rbac.authorization.k8s.io", FromKind: "ClusterRoleBinding",
        RefFieldPath: "roleRef",
        ToGroup: "rbac.authorization.k8s.io",
    },
    RefRule{
        FromGroup: "rbac.authorization.k8s.io", FromKind: "ClusterRoleBinding",
        RefFieldPath: "subjects[*]",
    },
)

// HPA — unconstrained (Deployment, ReplicaSet, StatefulSet, etc.)
NewRuleResolver("hpa",
    RefRule{
        FromGroup: "autoscaling", FromKind: "HorizontalPodAutoscaler",
        RefFieldPath: "spec.scaleTargetRef",
    },
)

// Pod → ServiceAccount — fully constrained (bare name string)
RefRule{
    FromGroup: "", FromKind: "Pod",
    RefFieldPath: "spec.serviceAccountName",
    ToGroup: "", ToKind: "ServiceAccount",
    SameNamespace: true,
}

// Pod → ConfigMap — fully constrained (bare name string)
RefRule{
    FromGroup: "", FromKind: "Pod",
    RefFieldPath: "spec.volumes[*].configMap.name",
    ToGroup: "", ToKind: "ConfigMap",
    SameNamespace: true,
}

// Gateway API — unconstrained typed refs
NewRuleResolver("gateway-api",
    RefRule{
        FromGroup: "gateway.networking.k8s.io", FromKind: "HTTPRoute",
        RefFieldPath: "spec.parentRefs[*]",
    },
    RefRule{
        FromGroup: "gateway.networking.k8s.io", FromKind: "HTTPRoute",
        RefFieldPath: "spec.rules[*].backendRefs[*]",
    },
    // ... GRPCRoute, TCPRoute, TLSRoute, UDPRoute: same field paths
)

// CAPI — unconstrained typed refs + constrained name refs
NewRuleResolver("capi",
    RefRule{
        FromGroup: "cluster.x-k8s.io", FromKind: "Cluster",
        RefFieldPath: "spec.infrastructureRef",
    },
    RefRule{
        FromGroup: "cluster.x-k8s.io", FromKind: "Cluster",
        RefFieldPath: "spec.controlPlaneRef",
    },
    RefRule{
        FromGroup: "cluster.x-k8s.io", FromKind: "Machine",
        RefFieldPath: "spec.clusterName",
        ToGroup: "cluster.x-k8s.io", ToKind: "Cluster",
        SameNamespace: true,
    },
)
```

#### What RefRule replaces

`RefRule` subsumes the existing rule types:

| Current type | Equivalent RefRule |
|---|---|
| `NameRefRule{ToKind: "SA", FieldPath: "spec.serviceAccountName", SameNamespace: true}` | `RefRule{RefFieldPath: "spec.serviceAccountName", ToKind: "SA", SameNamespace: true}` |
| `NamespacedNameRefRule{ToKind: "Svc", NameFieldPath: "...name", NamespaceFieldPath: "...namespace"}` | `RefRule{RefFieldPath: "webhooks[*].clientConfig.service", ToKind: "Svc"}` |
| `TypedRefRule{RefFieldPath: "spec.infrastructureRef"}` | `RefRule{RefFieldPath: "spec.infrastructureRef"}` |

Whether to keep the old types as aliases/sugar or replace them outright
is an implementation choice. The conceptual model is one rule type with
a type constraint that ranges from `any` to fully qualified.

#### Implementation

The resolver detects the data shape at `RefFieldPath` automatically:

- Value is `map[string]interface{}` → typed ref → read sub-fields
- Value is `string` → bare name → requires `ToGroup`/`ToKind`

No changes to `Lookup`, `Graph`, `ObjectRef`, `Edge`, or the `Resolver`
interface. The new `RefRule` is a `Rule` processed by `ruleResolver`,
same as the existing types. Old rule types can remain as sugar that
constructs a `RefRule` internally.

### Other Missing Rule Types

| Rule Type | What It Covers | Priority |
|-----------|----------------|----------|
| **PodTemplateRule** | Apply all Pod rules to embedded PodTemplateSpec at a path prefix | Medium |
| **AnnotationGroupRule** | Group resources by shared annotation values | Medium |
| **ImplicitNameRule** | Same-name bindings (Endpoints↔Service, CSINode↔Node) | Low |

#### PodTemplateRule

A meta-rule that applies all registered Pod rules to an embedded
PodTemplateSpec at a given path prefix. Avoids duplicating every Pod
rule for Deployment, RS, SS, DS, Job, CronJob.

Two strategies exist:

**Strategy A (template-aware):** When a Deployment is loaded, apply all
Pod rules with the path prefix `spec.template.spec.`. This produces
`Deployment → ConfigMap` edges directly.

**Strategy B (ownerRef-only):** Rely on the runtime ownerRef chain
(Deployment → ReplicaSet → Pod). Pod rules fire on the actual Pod.
Deployment only transitively depends on ConfigMap through the Pod.

Strategy A is better for static analysis (no Pods in the graph yet).
Strategy B is better for runtime graphs (actual Pods exist).

---

## 12. Priority Matrix

### Tier 1 — Core K8s, High Frequency

All expressible as `RefRule` with full type constraint (bare name fields):

| From → To | Field Path |
|-----------|------------|
| Pod → Secret | `spec.imagePullSecrets[*].name` |
| Pod → Secret | `spec.containers[*].env[*].valueFrom.secretKeyRef.name` |
| Pod → ConfigMap | `spec.containers[*].env[*].valueFrom.configMapKeyRef.name` |
| Pod → ConfigMap | `spec.volumes[*].projected.sources[*].configMap.name` |
| Pod → Secret | `spec.volumes[*].projected.sources[*].secret.name` |
| Ingress → Secret | `spec.tls[*].secretName` |
| Ingress → IngressClass | `spec.ingressClassName` |
| Ingress → Service | `spec.defaultBackend.service.name` |
| StatefulSet → Service | `spec.serviceName` |
| PV → StorageClass | `spec.storageClassName` |
| Pod → Node | `spec.nodeName` |
| Pod → PriorityClass | `spec.priorityClassName` |
| Pod → RuntimeClass | `spec.runtimeClassName` |

Plus: initContainers and ephemeralContainers mirrors of existing
container rules.

### Tier 2 — Core K8s, Uses Unconstrained/Partial RefRule

All expressible as `RefRule` with unconstrained or partial type constraint:

| Item | RefRule constraint |
|------|----------|
| RBAC roleRef → Role/ClusterRole | Partial (`ToGroup` only) |
| RBAC subjects → ServiceAccount/User/Group | Unconstrained (graph filters) |
| HPA → scaleTargetRef | Unconstrained |
| Webhook configs → Service | Fully constrained (`ToKind: "Service"`) |
| APIService → Service | Fully constrained (`ToKind: "Service"`) |
| PV → PVC (claimRef) | Fully constrained (`ToKind: "PVC"`) |
| PV → Secret (CSI refs) | Fully constrained (`ToKind: "Secret"`) |
| NetworkPolicy namespace selectors | Compound selector support (separate) |
| Fix LabelSelectorRule for matchExpressions | Implementation fix (separate) |

### Tier 3 — CRD Ecosystems (user-space or contrib)

Gateway API, cert-manager, Prometheus Operator, CAPI, Knative, Tekton,
KEDA, ArgoCD, Crossplane, Strimzi, Istio, Helm grouping.

These are best expressed as user-provided resolvers using the rule
primitives (including the new TypedRefRule). Some could ship as optional
built-in resolvers.

---

## 13. Design Decisions

### Resolved: Unified RefRule

`NameRefRule`, `NamespacedNameRefRule`, and the proposed `TypedRefRule`
are one concept: `RefRule`. See [Unified Reference Rule](#unified-reference-rule-refrule)
in section 11 for the full design.

The core model: **references are resolved at runtime**. The target
type is discovered from the object data. Specifying the target type at
definition time is a constraint — an optional narrowing that ranges
from `any` (unconstrained) to a fully qualified type, analogous to Go's
type system.

Two key properties make this work:

1. **The graph is the filter.** `lookup.Get` returns false for things
   that aren't nodes (RBAC Users/Groups, external URLs). No explicit
   filtering needed.
2. **Namespace fallback resolves conditional kinds.** Try same-namespace,
   then cluster-scoped. Role vs ClusterRole, Issuer vs ClusterIssuer —
   all handled by the same logic.

### Open: Embedded Pod Templates

Should a Deployment that embeds a PodTemplateSpec referencing a ConfigMap
produce a `Deployment → ConfigMap` edge?

- **Yes (Strategy A):** Better for static analysis, Helm chart inspection,
  "what does this Deployment depend on?" queries. But creates edges that
  don't exist in the K8s API — the Deployment doesn't reference the
  ConfigMap, its Pods do.
- **No (Strategy B):** Rely on the runtime ownerRef chain
  (Deployment → ReplicaSet → Pod). Pod rules fire on actual Pods.
  Cleaner model but requires Pods in the graph.

### Open: matchExpressions Fix

The current `LabelSelectorRule` only handles `matchLabels` (flat map
via `labels.SelectorFromSet`). It cannot handle `matchExpressions`.
Fix is straightforward at v0.x (no consumers to break):

- `LabelSelectorRule.SelectorFieldPath` should accept both flat maps
  (Service `spec.selector`) and full LabelSelector structs (NetworkPolicy
  `spec.podSelector`).
- Detect which format is present at runtime: if the value at the path
  has a `matchLabels` or `matchExpressions` sub-key, parse as
  LabelSelector using `metav1.LabelSelectorAsSelector()`; otherwise
  treat as flat map via `labels.SelectorFromSet`.
