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

// NewGatewayAPIResolver returns a resolver for Gateway API resource references.
// Handles HTTPRoute backendRefs and parentRefs (typed-refs) and
// Gateway gatewayClassName (bare name ref).
//
// Not registered by NewDefault() — opt in with WithResolver(NewGatewayAPIResolver()).
func NewGatewayAPIResolver() Resolver {
	return NewRuleResolver("gateway-api",
		// HTTPRoute -> backend services (typed-ref: kind/name/group/namespace)
		RefRule{
			FromGroup: "gateway.networking.k8s.io", FromKind: "HTTPRoute",
			FieldPath: "spec.rules[*].backendRefs[*]",
		},
		// HTTPRoute -> parent Gateway (typed-ref)
		RefRule{
			FromGroup: "gateway.networking.k8s.io", FromKind: "HTTPRoute",
			FieldPath: "spec.parentRefs[*]",
		},
		// Gateway -> GatewayClass (bare name, cluster-scoped)
		RefRule{
			FromGroup: "gateway.networking.k8s.io", FromKind: "Gateway",
			ToGroup: "gateway.networking.k8s.io", ToKind: "GatewayClass",
			FieldPath: "spec.gatewayClassName",
		},
	)
}
