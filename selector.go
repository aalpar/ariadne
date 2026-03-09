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

import "k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

// NewSelectorResolver returns a resolver for label/selector-based dependencies.
func NewSelectorResolver() Resolver {
	r := NewRuleResolver("selector",
		LabelSelectorRule{
			FromKind: "Service", ToKind: "Pod",
			SelectorFieldPath: "spec.selector",
		},
		LabelSelectorRule{
			FromGroup: "networking.k8s.io", FromKind: "NetworkPolicy",
			ToKind:            "Pod",
			SelectorFieldPath: "spec.podSelector",
		},
		LabelSelectorRule{
			FromGroup: "policy", FromKind: "PodDisruptionBudget",
			ToKind:            "Pod",
			SelectorFieldPath: "spec.selector",
		},
	)
	return &namedResolver{name: "selector", inner: r}
}

type namedResolver struct {
	name  string
	inner Resolver
}

func (n *namedResolver) Name() string { return n.name }

func (n *namedResolver) Resolve(obj *unstructured.Unstructured, lookup Lookup) []Edge {
	edges := n.inner.Resolve(obj, lookup)
	for i := range edges {
		edges[i].Resolver = n.name
	}
	return edges
}
