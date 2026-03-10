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
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ManagedResource identifies a Crossplane managed resource type for
// providerConfigRef resolution.
type ManagedResource struct {
	Group string
	Kind  string
}

// NewCrossplaneResolver returns a resolver for Crossplane resource references.
// Combines two patterns:
//   - providerConfigRef: RefRule for specific managed resource types
//     (callers pass their managed resource group/kind via ManagedResource)
//   - compositeTypeRef: custom resolver matching Compositions to all
//     instances of the referenced composite resource GroupKind
//
// Not registered by NewDefault() — opt in with WithResolver(NewCrossplaneResolver(...)).
func NewCrossplaneResolver(managedResources ...ManagedResource) Resolver {
	var rules []Rule
	for _, mr := range managedResources {
		rules = append(rules, RefRule{
			FromGroup: mr.Group, FromKind: mr.Kind,
			ToGroup: "pkg.crossplane.io", ToKind: "ProviderConfig",
			FieldPath: "spec.providerConfigRef.name",
		})
	}
	var ruleResolver Resolver
	if len(rules) > 0 {
		ruleResolver = NewRuleResolver("crossplane", rules...)
	}
	return &crossplaneResolver{ruleResolver: ruleResolver}
}

type crossplaneResolver struct {
	ruleResolver Resolver
}

func (r *crossplaneResolver) Name() string { return "crossplane" }

func (r *crossplaneResolver) Extract(obj *unstructured.Unstructured) []Edge {
	if r.ruleResolver != nil {
		return r.ruleResolver.Extract(obj)
	}
	return nil
}

func (r *crossplaneResolver) Resolve(obj *unstructured.Unstructured, lookup Lookup) []Edge {
	var edges []Edge

	// providerConfigRef via declarative rules
	if r.ruleResolver != nil {
		edges = append(edges, r.ruleResolver.Resolve(obj, lookup)...)
	}

	// compositeTypeRef via custom logic
	edges = append(edges, r.resolveCompositeTypeRef(obj, lookup)...)

	return edges
}

func (r *crossplaneResolver) resolveCompositeTypeRef(obj *unstructured.Unstructured, lookup Lookup) []Edge {
	ref := RefFromUnstructured(obj)
	gvk := obj.GroupVersionKind()

	if gvk.Group == "apiextensions.crossplane.io" && gvk.Kind == "Composition" {
		return r.compositeForward(ref, obj, lookup)
	}
	return r.compositeReverse(ref, obj, lookup)
}

func (r *crossplaneResolver) compositeForward(ref ObjectRef, obj *unstructured.Unstructured, lookup Lookup) []Edge {
	group, kind := extractCompositeTypeRef(obj)
	if kind == "" {
		return nil
	}

	var edges []Edge
	for _, target := range lookup.List(group, kind) {
		edges = append(edges, Edge{
			From:     ref,
			To:       RefFromUnstructured(target),
			Type:     EdgeCustom,
			Resolver: "crossplane",
			Field:    "spec.compositeTypeRef",
		})
	}
	return edges
}

func (r *crossplaneResolver) compositeReverse(ref ObjectRef, obj *unstructured.Unstructured, lookup Lookup) []Edge {
	var edges []Edge
	for _, comp := range lookup.List("apiextensions.crossplane.io", "Composition") {
		group, kind := extractCompositeTypeRef(comp)
		if kind == "" {
			continue
		}
		if ref.Group == group && ref.Kind == kind {
			edges = append(edges, Edge{
				From:     RefFromUnstructured(comp),
				To:       ref,
				Type:     EdgeCustom,
				Resolver: "crossplane",
				Field:    "spec.compositeTypeRef",
			})
		}
	}
	return edges
}

func extractCompositeTypeRef(obj *unstructured.Unstructured) (group, kind string) {
	spec, _ := obj.Object["spec"].(map[string]interface{})
	if spec == nil {
		return "", ""
	}
	ref, _ := spec["compositeTypeRef"].(map[string]interface{})
	if ref == nil {
		return "", ""
	}
	kind, _ = ref["kind"].(string)
	group, _ = ref["apiGroup"].(string)
	return group, kind
}
