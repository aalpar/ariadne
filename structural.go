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
		NameRefRule{
			FromKind: "Pod", ToKind: "ServiceAccount",
			FieldPath: "spec.serviceAccountName", SameNamespace: true,
		},
		// Pod -> ConfigMap (volumes)
		NameRefRule{
			FromKind: "Pod", ToKind: "ConfigMap",
			FieldPath: "spec.volumes[*].configMap.name", SameNamespace: true,
		},
		// Pod -> ConfigMap (envFrom)
		NameRefRule{
			FromKind: "Pod", ToKind: "ConfigMap",
			FieldPath: "spec.containers[*].envFrom[*].configMapRef.name", SameNamespace: true,
		},
		// Pod -> Secret (volumes)
		NameRefRule{
			FromKind: "Pod", ToKind: "Secret",
			FieldPath: "spec.volumes[*].secret.secretName", SameNamespace: true,
		},
		// Pod -> Secret (envFrom)
		NameRefRule{
			FromKind: "Pod", ToKind: "Secret",
			FieldPath: "spec.containers[*].envFrom[*].secretRef.name", SameNamespace: true,
		},
		// Pod -> PVC
		NameRefRule{
			FromKind: "Pod", ToKind: "PersistentVolumeClaim",
			FieldPath: "spec.volumes[*].persistentVolumeClaim.claimName", SameNamespace: true,
		},
		// PVC -> PV
		NameRefRule{
			FromKind: "PersistentVolumeClaim", ToKind: "PersistentVolume",
			FieldPath: "spec.volumeName", SameNamespace: false,
		},
		// PVC -> StorageClass
		NameRefRule{
			FromGroup: "", FromKind: "PersistentVolumeClaim",
			ToGroup: "storage.k8s.io", ToKind: "StorageClass",
			FieldPath: "spec.storageClassName", SameNamespace: false,
		},
		// Ingress -> Service
		NameRefRule{
			FromGroup: "networking.k8s.io", FromKind: "Ingress",
			ToKind:    "Service",
			FieldPath: "spec.rules[*].http.paths[*].backend.service.name", SameNamespace: true,
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
