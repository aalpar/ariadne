// Copyright 2026 The Ariadne Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package ariadne

import (
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// NewStructuralResolver returns a resolver for known K8s resource references.
func NewStructuralResolver() Resolver {
	rules := NewRuleResolver("structural",
		// Pod -> ServiceAccount
		RefRule{
			FromKind: "Pod", ToKind: "ServiceAccount",
			FieldPath: "spec.serviceAccountName",
		},
		// Pod -> ConfigMap (volumes)
		RefRule{
			FromKind: "Pod", ToKind: "ConfigMap",
			FieldPath: "spec.volumes[*].configMap.name",
		},
		// Pod -> ConfigMap (envFrom)
		RefRule{
			FromKind: "Pod", ToKind: "ConfigMap",
			FieldPath: "spec.containers[*].envFrom[*].configMapRef.name",
		},
		// Pod -> Secret (volumes)
		RefRule{
			FromKind: "Pod", ToKind: "Secret",
			FieldPath: "spec.volumes[*].secret.secretName",
		},
		// Pod -> Secret (envFrom)
		RefRule{
			FromKind: "Pod", ToKind: "Secret",
			FieldPath: "spec.containers[*].envFrom[*].secretRef.name",
		},
		// Pod -> PVC
		RefRule{
			FromKind: "Pod", ToKind: "PersistentVolumeClaim",
			FieldPath: "spec.volumes[*].persistentVolumeClaim.claimName",
		},
		// PVC -> PV
		RefRule{
			FromKind: "PersistentVolumeClaim", ToKind: "PersistentVolume",
			FieldPath: "spec.volumeName",
		},
		// PVC -> StorageClass
		RefRule{
			FromGroup: "", FromKind: "PersistentVolumeClaim",
			ToGroup: "storage.k8s.io", ToKind: "StorageClass",
			FieldPath: "spec.storageClassName",
		},
		// Ingress -> Service
		RefRule{
			FromGroup: "networking.k8s.io", FromKind: "Ingress",
			ToKind:    "Service",
			FieldPath: "spec.rules[*].http.paths[*].backend.service.name",
		},
		// Ingress -> Service (default backend)
		RefRule{
			FromGroup: "networking.k8s.io", FromKind: "Ingress",
			ToKind:    "Service",
			FieldPath: "spec.defaultBackend.service.name",
		},
		// Ingress -> Secret (TLS)
		RefRule{
			FromGroup: "networking.k8s.io", FromKind: "Ingress",
			ToKind:    "Secret",
			FieldPath: "spec.tls[*].secretName",
		},
		// Ingress -> IngressClass
		RefRule{
			FromGroup: "networking.k8s.io", FromKind: "Ingress",
			ToGroup: "networking.k8s.io", ToKind: "IngressClass",
			FieldPath: "spec.ingressClassName",
		},
		// Pod -> Secret (imagePullSecrets)
		RefRule{
			FromKind: "Pod", ToKind: "Secret",
			FieldPath: "spec.imagePullSecrets[*].name",
		},
		// Pod -> ConfigMap (env valueFrom)
		RefRule{
			FromKind: "Pod", ToKind: "ConfigMap",
			FieldPath: "spec.containers[*].env[*].valueFrom.configMapKeyRef.name",
		},
		// Pod -> Secret (env valueFrom)
		RefRule{
			FromKind: "Pod", ToKind: "Secret",
			FieldPath: "spec.containers[*].env[*].valueFrom.secretKeyRef.name",
		},
		// Pod -> ConfigMap (projected volumes)
		RefRule{
			FromKind: "Pod", ToKind: "ConfigMap",
			FieldPath: "spec.volumes[*].projected.sources[*].configMap.name",
		},
		// Pod -> Secret (projected volumes)
		RefRule{
			FromKind: "Pod", ToKind: "Secret",
			FieldPath: "spec.volumes[*].projected.sources[*].secret.name",
		},
		// Pod -> Node
		RefRule{
			FromKind: "Pod", ToKind: "Node",
			FieldPath: "spec.nodeName",
		},
		// Pod -> PriorityClass
		RefRule{
			FromKind: "Pod",
			ToGroup: "scheduling.k8s.io", ToKind: "PriorityClass",
			FieldPath: "spec.priorityClassName",
		},
		// Pod -> RuntimeClass
		RefRule{
			FromKind: "Pod",
			ToGroup: "node.k8s.io", ToKind: "RuntimeClass",
			FieldPath: "spec.runtimeClassName",
		},
		// StatefulSet -> Service (headless)
		RefRule{
			FromGroup: "apps", FromKind: "StatefulSet",
			ToKind:    "Service",
			FieldPath: "spec.serviceName",
		},
		// PV -> StorageClass
		RefRule{
			FromKind: "PersistentVolume",
			ToGroup: "storage.k8s.io", ToKind: "StorageClass",
			FieldPath: "spec.storageClassName",
		},
		// initContainers mirrors
		RefRule{
			FromKind: "Pod", ToKind: "ConfigMap",
			FieldPath: "spec.initContainers[*].envFrom[*].configMapRef.name",
		},
		RefRule{
			FromKind: "Pod", ToKind: "Secret",
			FieldPath: "spec.initContainers[*].envFrom[*].secretRef.name",
		},
		RefRule{
			FromKind: "Pod", ToKind: "ConfigMap",
			FieldPath: "spec.initContainers[*].env[*].valueFrom.configMapKeyRef.name",
		},
		RefRule{
			FromKind: "Pod", ToKind: "Secret",
			FieldPath: "spec.initContainers[*].env[*].valueFrom.secretKeyRef.name",
		},
		// ephemeralContainers mirrors
		RefRule{
			FromKind: "Pod", ToKind: "ConfigMap",
			FieldPath: "spec.ephemeralContainers[*].envFrom[*].configMapRef.name",
		},
		RefRule{
			FromKind: "Pod", ToKind: "Secret",
			FieldPath: "spec.ephemeralContainers[*].envFrom[*].secretRef.name",
		},
		RefRule{
			FromKind: "Pod", ToKind: "ConfigMap",
			FieldPath: "spec.ephemeralContainers[*].env[*].valueFrom.configMapKeyRef.name",
		},
		RefRule{
			FromKind: "Pod", ToKind: "Secret",
			FieldPath: "spec.ephemeralContainers[*].env[*].valueFrom.secretKeyRef.name",
		},

		// Typed references

		// HPA -> scaleTargetRef (unconstrained: any kind)
		RefRule{
			FromGroup: "autoscaling", FromKind: "HorizontalPodAutoscaler",
			FieldPath: "spec.scaleTargetRef",
		},
		// RoleBinding -> roleRef (constrained to rbac.authorization.k8s.io)
		RefRule{
			FromGroup: "rbac.authorization.k8s.io", FromKind: "RoleBinding",
			ToGroup:   "rbac.authorization.k8s.io",
			FieldPath: "roleRef",
		},
		// RoleBinding -> subjects (ServiceAccount, etc.)
		RefRule{
			FromGroup: "rbac.authorization.k8s.io", FromKind: "RoleBinding",
			FieldPath: "subjects[*]",
		},
		// ClusterRoleBinding -> roleRef
		RefRule{
			FromGroup: "rbac.authorization.k8s.io", FromKind: "ClusterRoleBinding",
			ToGroup:   "rbac.authorization.k8s.io",
			FieldPath: "roleRef",
		},
		// ClusterRoleBinding -> subjects (ServiceAccount, etc.)
		RefRule{
			FromGroup: "rbac.authorization.k8s.io", FromKind: "ClusterRoleBinding",
			FieldPath: "subjects[*]",
		},
		// PV -> PVC (claimRef with explicit namespace)
		RefRule{
			FromKind:  "PersistentVolume",
			ToKind:    "PersistentVolumeClaim",
			FieldPath: "spec.claimRef",
		},
	)

	return &structuralResolver{rules: rules}
}

type structuralResolver struct {
	rules Resolver
}

func (s *structuralResolver) Name() string { return "structural" }

func (s *structuralResolver) Resolve(obj *unstructured.Unstructured, lookup Lookup) []Edge {
	edges := s.rules.Resolve(obj, lookup)
	for i := range edges {
		edges[i].Resolver = "structural"
	}
	edges = append(edges, resolveOwnerRefs(obj, lookup)...)
	return edges
}

func resolveOwnerRefs(obj *unstructured.Unstructured, lookup Lookup) []Edge {
	ref := RefFromUnstructured(obj)
	var edges []Edge

	// Forward: obj has ownerReferences pointing to existing objects.
	for _, owner := range obj.GetOwnerReferences() {
		ownerRef := ObjectRef{
			Group:     extractGroup(owner.APIVersion),
			Kind:      owner.Kind,
			Namespace: ref.Namespace,
			Name:      owner.Name,
		}
		if _, ok := lookup.Get(ownerRef); ok {
			edges = append(edges, Edge{
				From:     ref,
				To:       ownerRef,
				Type:     EdgeNameRef,
				Resolver: "structural",
				Field:    "metadata.ownerReferences",
			})
		}
	}

	// Reverse: existing objects may have ownerReferences pointing to obj.
	// Namespaced owners can only own resources in the same namespace.
	// Cluster-scoped owners (Namespace == "") can own resources in any namespace.
	for _, existing := range lookup.ListAll() {
		if ref.Namespace != "" && existing.GetNamespace() != ref.Namespace {
			continue
		}
		for _, owner := range existing.GetOwnerReferences() {
			if extractGroup(owner.APIVersion) == ref.Group &&
				owner.Kind == ref.Kind &&
				owner.Name == ref.Name {
				edges = append(edges, Edge{
					From:     RefFromUnstructured(existing),
					To:       ref,
					Type:     EdgeNameRef,
					Resolver: "structural",
					Field:    "metadata.ownerReferences",
				})
			}
		}
	}

	return edges
}

// extractGroup extracts the API group from an apiVersion string.
// "apps/v1" -> "apps", "v1" -> ""
func extractGroup(apiVersion string) string {
	group, _, ok := strings.Cut(apiVersion, "/")
	if !ok {
		return ""
	}
	return group
}
