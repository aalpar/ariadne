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

// NewPrometheusResolver returns a resolver for Prometheus Operator resources.
// Handles ServiceMonitor→Service and PodMonitor→Pod via label selector
// matching, with support for cross-namespace targeting via namespaceSelector.
//
// namespaceSelector semantics:
//   - absent/empty: match targets in the same namespace as the monitor
//   - {any: true}: match targets in all namespaces
//   - {matchNames: ["ns1", "ns2"]}: match targets in listed namespaces only
//
// Not registered by NewDefault() — opt in with WithResolver(NewPrometheusResolver()).
func NewPrometheusResolver() Resolver {
	return NewRuleResolver("prometheus",
		LabelSelectorRule{
			FromGroup:                  "monitoring.coreos.com",
			FromKind:                   "ServiceMonitor",
			ToKind:                     "Service",
			SelectorFieldPath:          "spec.selector",
			NamespaceSelectorFieldPath: "spec.namespaceSelector",
		},
		LabelSelectorRule{
			FromGroup:                  "monitoring.coreos.com",
			FromKind:                   "PodMonitor",
			ToKind:                     "Pod",
			SelectorFieldPath:          "spec.selector",
			NamespaceSelectorFieldPath: "spec.namespaceSelector",
		},
	)
}
