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

// NewArgoCDResolver returns a resolver for Argo CD Application references.
// Handles destination namespace and project references.
//
// Not registered by NewDefault() — opt in with WithResolver(NewArgoCDResolver()).
func NewArgoCDResolver() Resolver {
	return NewRuleResolver("argocd",
		// Application -> target Namespace
		RefRule{
			FromGroup: "argoproj.io", FromKind: "Application",
			ToKind:    "Namespace",
			FieldPath: "spec.destination.namespace",
		},
		// Application -> AppProject
		RefRule{
			FromGroup: "argoproj.io", FromKind: "Application",
			ToGroup: "argoproj.io", ToKind: "AppProject",
			FieldPath: "spec.project",
		},
	)
}
