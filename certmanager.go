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

// NewCertManagerResolver returns a resolver for cert-manager references.
// Handles Certificate→Secret, Certificate→Issuer/ClusterIssuer, and
// Ingress→Issuer/ClusterIssuer (via annotations).
//
// Not registered by NewDefault() — opt in with WithResolver(NewCertManagerResolver()).
func NewCertManagerResolver() Resolver {
	return NewRuleResolver("cert-manager",
		// Certificate -> Secret (where the cert is stored)
		RefRule{
			FromGroup: "cert-manager.io", FromKind: "Certificate",
			ToKind:    "Secret",
			FieldPath: "spec.secretName",
		},
		// Certificate -> Issuer
		RefRule{
			FromGroup: "cert-manager.io", FromKind: "Certificate",
			ToGroup: "cert-manager.io", ToKind: "Issuer",
			FieldPath: "spec.issuerRef",
		},
		// Certificate -> ClusterIssuer
		RefRule{
			FromGroup: "cert-manager.io", FromKind: "Certificate",
			ToGroup: "cert-manager.io", ToKind: "ClusterIssuer",
			FieldPath:     "spec.issuerRef",
			ClusterScoped: true,
		},
		// Ingress -> Issuer (via annotation)
		AnnotationRefRule{
			FromGroup: "networking.k8s.io", FromKind: "Ingress",
			ToGroup: "cert-manager.io", ToKind: "Issuer",
			AnnotationKey: "cert-manager.io/issuer",
		},
		// Ingress -> ClusterIssuer (via annotation)
		AnnotationRefRule{
			FromGroup: "networking.k8s.io", FromKind: "Ingress",
			ToGroup: "cert-manager.io", ToKind: "ClusterIssuer",
			AnnotationKey: "cert-manager.io/cluster-issuer",
			ClusterScoped: true,
		},
	)
}
