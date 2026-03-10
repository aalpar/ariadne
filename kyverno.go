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

// NewKyvernoResolver returns a resolver for Kyverno policy-to-resource
// relationships. When a ClusterPolicy or Policy lists resource kinds in
// spec.rules[*].match.resources.kinds, this resolver creates edges from
// the policy to every instance of those kinds in the graph.
//
// Handles plain kind names only (e.g., "Pod", "Service"). Does not parse
// group-qualified kinds like "apps/v1/Deployment".
//
// Not registered by NewDefault() — opt in with WithResolver(NewKyvernoResolver()).
func NewKyvernoResolver() Resolver {
	return &kyvernoResolver{}
}

type kyvernoResolver struct{}

func (r *kyvernoResolver) Name() string { return "kyverno" }

func (r *kyvernoResolver) Extract(_ *unstructured.Unstructured) []Edge { return nil }

func (r *kyvernoResolver) Resolve(obj *unstructured.Unstructured, lookup Lookup) []Edge {
	ref := RefFromUnstructured(obj)
	gvk := obj.GroupVersionKind()

	if isKyvernoPolicy(gvk.Group, gvk.Kind) {
		return r.resolveForward(ref, obj, lookup)
	}
	return r.resolveReverse(ref, obj, lookup)
}

func isKyvernoPolicy(group, kind string) bool {
	return group == "kyverno.io" && (kind == "ClusterPolicy" || kind == "Policy")
}

func (r *kyvernoResolver) resolveForward(ref ObjectRef, obj *unstructured.Unstructured, lookup Lookup) []Edge {
	kinds := extractPolicyKinds(obj)
	if len(kinds) == 0 {
		return nil
	}

	var edges []Edge
	for _, kind := range kinds {
		var targets []*unstructured.Unstructured
		if ref.Namespace != "" {
			// Namespaced Policy: only match resources in the same namespace.
			targets = lookup.ListInNamespace("", kind, ref.Namespace)
		} else {
			// ClusterPolicy: match all instances across namespaces.
			targets = lookup.List("", kind)
		}
		for _, target := range targets {
			edges = append(edges, Edge{
				From:     ref,
				To:       RefFromUnstructured(target),
				Type:     EdgeCustom,
				Resolver: "kyverno",
				Field:    "spec.rules[*].match.resources.kinds",
			})
		}
	}
	return edges
}

func (r *kyvernoResolver) resolveReverse(ref ObjectRef, obj *unstructured.Unstructured, lookup Lookup) []Edge {
	var edges []Edge

	// Check ClusterPolicy (cluster-scoped)
	for _, policy := range lookup.List("kyverno.io", "ClusterPolicy") {
		kinds := extractPolicyKinds(policy)
		for _, kind := range kinds {
			if kind == ref.Kind && ref.Group == "" {
				edges = append(edges, Edge{
					From:     RefFromUnstructured(policy),
					To:       ref,
					Type:     EdgeCustom,
					Resolver: "kyverno",
					Field:    "spec.rules[*].match.resources.kinds",
				})
				break
			}
		}
	}

	// Check Policy (namespaced — only matches same namespace)
	for _, policy := range lookup.List("kyverno.io", "Policy") {
		policyRef := RefFromUnstructured(policy)
		if policyRef.Namespace != ref.Namespace {
			continue
		}
		kinds := extractPolicyKinds(policy)
		for _, kind := range kinds {
			if kind == ref.Kind && ref.Group == "" {
				edges = append(edges, Edge{
					From:     policyRef,
					To:       ref,
					Type:     EdgeCustom,
					Resolver: "kyverno",
					Field:    "spec.rules[*].match.resources.kinds",
				})
				break
			}
		}
	}

	return edges
}

// extractPolicyKinds extracts the kind strings from
// spec.rules[*].match.resources.kinds.
func extractPolicyKinds(obj *unstructured.Unstructured) []string {
	return extractFieldValues(obj.Object, "spec.rules[*].match.resources.kinds[*]")
}
