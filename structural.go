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

// podRefRules defines RefRules for Pod dependencies.
// Also used as the basis for PodTemplate rules via podTemplateRules().
var podRefRules = []RefRule{
	// Pod -> ServiceAccount
	{FromKind: "Pod", ToKind: "ServiceAccount", FieldPath: "spec.serviceAccountName"},
	// Pod -> ConfigMap (volumes)
	{FromKind: "Pod", ToKind: "ConfigMap", FieldPath: "spec.volumes[*].configMap.name"},
	// Pod -> ConfigMap (envFrom)
	{FromKind: "Pod", ToKind: "ConfigMap", FieldPath: "spec.containers[*].envFrom[*].configMapRef.name"},
	// Pod -> Secret (volumes)
	{FromKind: "Pod", ToKind: "Secret", FieldPath: "spec.volumes[*].secret.secretName"},
	// Pod -> Secret (envFrom)
	{FromKind: "Pod", ToKind: "Secret", FieldPath: "spec.containers[*].envFrom[*].secretRef.name"},
	// Pod -> PVC
	{FromKind: "Pod", ToKind: "PersistentVolumeClaim", FieldPath: "spec.volumes[*].persistentVolumeClaim.claimName"},
	// Pod -> Secret (imagePullSecrets)
	{FromKind: "Pod", ToKind: "Secret", FieldPath: "spec.imagePullSecrets[*].name"},
	// Pod -> ConfigMap (env valueFrom)
	{FromKind: "Pod", ToKind: "ConfigMap", FieldPath: "spec.containers[*].env[*].valueFrom.configMapKeyRef.name"},
	// Pod -> Secret (env valueFrom)
	{FromKind: "Pod", ToKind: "Secret", FieldPath: "spec.containers[*].env[*].valueFrom.secretKeyRef.name"},
	// Pod -> ConfigMap (projected volumes)
	{FromKind: "Pod", ToKind: "ConfigMap", FieldPath: "spec.volumes[*].projected.sources[*].configMap.name"},
	// Pod -> Secret (projected volumes)
	{FromKind: "Pod", ToKind: "Secret", FieldPath: "spec.volumes[*].projected.sources[*].secret.name"},
	// Pod -> Node
	{FromKind: "Pod", ToKind: "Node", FieldPath: "spec.nodeName", ClusterScoped: true},
	// Pod -> PriorityClass
	{FromKind: "Pod", ToGroup: "scheduling.k8s.io", ToKind: "PriorityClass", FieldPath: "spec.priorityClassName", ClusterScoped: true},
	// Pod -> RuntimeClass
	{FromKind: "Pod", ToGroup: "node.k8s.io", ToKind: "RuntimeClass", FieldPath: "spec.runtimeClassName", ClusterScoped: true},
	// initContainers mirrors
	{FromKind: "Pod", ToKind: "ConfigMap", FieldPath: "spec.initContainers[*].envFrom[*].configMapRef.name"},
	{FromKind: "Pod", ToKind: "Secret", FieldPath: "spec.initContainers[*].envFrom[*].secretRef.name"},
	{FromKind: "Pod", ToKind: "ConfigMap", FieldPath: "spec.initContainers[*].env[*].valueFrom.configMapKeyRef.name"},
	{FromKind: "Pod", ToKind: "Secret", FieldPath: "spec.initContainers[*].env[*].valueFrom.secretKeyRef.name"},
	// ephemeralContainers mirrors
	{FromKind: "Pod", ToKind: "ConfigMap", FieldPath: "spec.ephemeralContainers[*].envFrom[*].configMapRef.name"},
	{FromKind: "Pod", ToKind: "Secret", FieldPath: "spec.ephemeralContainers[*].envFrom[*].secretRef.name"},
	{FromKind: "Pod", ToKind: "ConfigMap", FieldPath: "spec.ephemeralContainers[*].env[*].valueFrom.configMapKeyRef.name"},
	{FromKind: "Pod", ToKind: "Secret", FieldPath: "spec.ephemeralContainers[*].env[*].valueFrom.secretKeyRef.name"},
}

// NewStructuralResolver returns a resolver for known K8s resource references.
func NewStructuralResolver() Resolver {
	// Start with Pod rules and their PodTemplate mirrors.
	var allRules []Rule
	for _, r := range podRefRules {
		allRules = append(allRules, r)
	}
	for _, r := range podTemplateRules(podRefRules) {
		allRules = append(allRules, r)
	}

	// Non-Pod rules (these don't have PodTemplate equivalents).
	allRules = append(allRules,
		// PVC -> PV
		RefRule{
			FromKind: "PersistentVolumeClaim", ToKind: "PersistentVolume",
			FieldPath:     "spec.volumeName",
			ClusterScoped: true,
		},
		// PVC -> StorageClass
		RefRule{
			FromGroup: "", FromKind: "PersistentVolumeClaim",
			ToGroup: "storage.k8s.io", ToKind: "StorageClass",
			FieldPath:     "spec.storageClassName",
			ClusterScoped: true,
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
			FieldPath:     "spec.ingressClassName",
			ClusterScoped: true,
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
			FieldPath:     "spec.storageClassName",
			ClusterScoped: true,
		},
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
		// RoleBinding -> subjects (ServiceAccount only)
		RefRule{
			FromGroup: "rbac.authorization.k8s.io", FromKind: "RoleBinding",
			ToKind:    "ServiceAccount",
			FieldPath: "subjects[*]",
		},
		// ClusterRoleBinding -> roleRef
		RefRule{
			FromGroup: "rbac.authorization.k8s.io", FromKind: "ClusterRoleBinding",
			ToGroup:   "rbac.authorization.k8s.io",
			FieldPath: "roleRef",
		},
		// ClusterRoleBinding -> subjects (ServiceAccount only)
		RefRule{
			FromGroup: "rbac.authorization.k8s.io", FromKind: "ClusterRoleBinding",
			ToKind:    "ServiceAccount",
			FieldPath: "subjects[*]",
		},
		// PV -> PVC (claimRef with explicit namespace)
		RefRule{
			FromKind:  "PersistentVolume",
			ToKind:    "PersistentVolumeClaim",
			FieldPath: "spec.claimRef",
		},
		// ValidatingWebhookConfiguration -> Service (cross-namespace)
		RefRule{
			FromGroup: "admissionregistration.k8s.io", FromKind: "ValidatingWebhookConfiguration",
			ToKind:             "Service",
			FieldPath:          "webhooks[*].clientConfig.service.name",
			NamespaceFieldPath: "webhooks[*].clientConfig.service.namespace",
		},
		// MutatingWebhookConfiguration -> Service (cross-namespace)
		RefRule{
			FromGroup: "admissionregistration.k8s.io", FromKind: "MutatingWebhookConfiguration",
			ToKind:             "Service",
			FieldPath:          "webhooks[*].clientConfig.service.name",
			NamespaceFieldPath: "webhooks[*].clientConfig.service.namespace",
		},
		// APIService -> Service (cross-namespace)
		RefRule{
			FromGroup: "apiregistration.k8s.io", FromKind: "APIService",
			ToKind:             "Service",
			FieldPath:          "spec.service.name",
			NamespaceFieldPath: "spec.service.namespace",
		},
		// ServiceAccount -> Secret (legacy token secrets, deprecated post-1.24)
		RefRule{
			FromKind: "ServiceAccount", ToKind: "Secret",
			FieldPath: "secrets[*].name",
		},
		// ServiceAccount -> Secret (image pull secrets)
		RefRule{
			FromKind: "ServiceAccount", ToKind: "Secret",
			FieldPath: "imagePullSecrets[*].name",
		},
		// VolumeAttachment -> PV
		RefRule{
			FromGroup: "storage.k8s.io", FromKind: "VolumeAttachment",
			ToKind:        "PersistentVolume",
			FieldPath:     "spec.source.persistentVolumeName",
			ClusterScoped: true,
		},
		// VolumeAttachment -> Node
		RefRule{
			FromGroup: "storage.k8s.io", FromKind: "VolumeAttachment",
			ToKind:        "Node",
			FieldPath:     "spec.nodeName",
			ClusterScoped: true,
		},
		// PV -> CSIDriver
		RefRule{
			FromKind: "PersistentVolume",
			ToGroup: "storage.k8s.io", ToKind: "CSIDriver",
			FieldPath:     "spec.csi.driver",
			ClusterScoped: true,
		},
		// StorageClass -> CSIDriver
		RefRule{
			FromGroup: "storage.k8s.io", FromKind: "StorageClass",
			ToGroup: "storage.k8s.io", ToKind: "CSIDriver",
			FieldPath:     "provisioner",
			ClusterScoped: true,
		},
	)

	rules := NewRuleResolver("structural", allRules...)
	return &structuralResolver{rules: rules}
}

type structuralResolver struct {
	rules Resolver
}

func (s *structuralResolver) Name() string { return "structural" }

func (s *structuralResolver) Extract(obj *unstructured.Unstructured) []Edge {
	edges := s.rules.Extract(obj)
	edges = append(edges, extractOwnerRefsForward(obj)...)
	return edges
}

func extractOwnerRefsForward(obj *unstructured.Unstructured) []Edge {
	ref := RefFromUnstructured(obj)
	var edges []Edge
	for _, owner := range obj.GetOwnerReferences() {
		edges = append(edges, Edge{
			From: ref,
			To: ObjectRef{
				Group:     extractGroup(owner.APIVersion),
				Kind:      owner.Kind,
				Namespace: ref.Namespace,
				Name:      owner.Name,
			},
			Type:     EdgeRef,
			Resolver: "structural",
			Field:    "metadata.ownerReferences",
		})
	}
	return edges
}

func (s *structuralResolver) Resolve(obj *unstructured.Unstructured, lookup Lookup) []Edge {
	edges := s.rules.Resolve(obj, lookup)
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
				Type:     EdgeRef,
				Resolver: "structural",
				Field:    "metadata.ownerReferences",
			})
		}
	}

	// Reverse: existing objects may have ownerReferences pointing to obj.
	// Namespaced owners can only own resources in the same namespace.
	// Cluster-scoped owners (Namespace == "") can own resources in any namespace.
	var candidates []*unstructured.Unstructured
	if ref.Namespace != "" {
		candidates = lookup.ListByNamespace(ref.Namespace)
	} else {
		candidates = lookup.ListAll()
	}
	for _, existing := range candidates {
		for _, owner := range existing.GetOwnerReferences() {
			if extractGroup(owner.APIVersion) == ref.Group &&
				owner.Kind == ref.Kind &&
				owner.Name == ref.Name {
				edges = append(edges, Edge{
					From:     RefFromUnstructured(existing),
					To:       ref,
					Type:     EdgeRef,
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
