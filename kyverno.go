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

// NewKyvernoResolver returns a resolver for Kyverno policy-to-resource
// relationships. When a ClusterPolicy or Policy lists resource kinds in
// spec.rules[*].match.resources.kinds, this resolver creates edges from
// the policy to every instance of those kinds in the graph.
//
// Parses kind strings in Kyverno's standard formats:
//   - "Kind"              → core API group (e.g., "Pod")
//   - "group/Kind"        → explicit group (e.g., "apps/Deployment")
//   - "group/version/Kind" → explicit group, version ignored (e.g., "apps/v1/Deployment")
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
	for _, kr := range kinds {
		var targets []*unstructured.Unstructured
		if ref.Namespace != "" {
			// Namespaced Policy: only match resources in the same namespace.
			targets = lookup.ListInNamespace(kr.Group, kr.Kind, ref.Namespace)
		} else {
			// ClusterPolicy: match all instances across namespaces.
			targets = lookup.List(kr.Group, kr.Kind)
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
		for _, kr := range kinds {
			if kr.Kind == ref.Kind && kr.Group == ref.Group {
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
		for _, kr := range kinds {
			if kr.Kind == ref.Kind && kr.Group == ref.Group {
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

// policyKindRef holds a parsed kind reference from a Kyverno policy.
type policyKindRef struct {
	Group string
	Kind  string
}

// parseKyvernoKind parses a Kyverno kind string into group and kind.
// Formats: "Kind", "group/Kind", "group/version/Kind".
func parseKyvernoKind(s string) policyKindRef {
	parts := strings.Split(s, "/")
	switch len(parts) {
	case 3:
		return policyKindRef{Group: parts[0], Kind: parts[2]}
	case 2:
		return policyKindRef{Group: parts[0], Kind: parts[1]}
	default:
		return policyKindRef{Kind: s}
	}
}

// extractPolicyKinds extracts and parses the kind strings from
// spec.rules[*].match.resources.kinds.
func extractPolicyKinds(obj *unstructured.Unstructured) []policyKindRef {
	raw := extractFieldValues(obj.Object, "spec.rules[*].match.resources.kinds[*]")
	refs := make([]policyKindRef, len(raw))
	for i, s := range raw {
		refs[i] = parseKyvernoKind(s)
	}
	return refs
}
