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
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestCertManager_CertificateRefs(t *testing.T) {
	g := New(WithResolver(NewCertManagerResolver()))

	cert := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "cert-manager.io/v1", "kind": "Certificate",
		"metadata": map[string]interface{}{
			"name": "web-cert", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"secretName": "web-tls",
			"issuerRef": map[string]interface{}{
				"name":  "letsencrypt-prod",
				"kind":  "ClusterIssuer",
				"group": "cert-manager.io",
			},
		},
	}}

	secret := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Secret",
		"metadata": map[string]interface{}{
			"name": "web-tls", "namespace": "default",
		},
	}}

	issuer := newObj("cert-manager.io", "v1", "ClusterIssuer", "", "letsencrypt-prod")

	g.Load([]unstructured.Unstructured{cert, secret, issuer})

	certRef := ObjectRef{Group: "cert-manager.io", Kind: "Certificate", Namespace: "default", Name: "web-cert"}
	deps := g.DependenciesOf(certRef)
	if len(deps) != 2 {
		t.Fatalf("expected 2 dependencies, got %d: %v", len(deps), deps)
	}

	foundSecret, foundIssuer := false, false
	for _, e := range deps {
		switch {
		case e.To.Kind == "Secret" && e.To.Name == "web-tls":
			foundSecret = true
			if e.Field != "spec.secretName" {
				t.Errorf("secret edge: expected field spec.secretName, got %s", e.Field)
			}
		case e.To.Kind == "ClusterIssuer" && e.To.Name == "letsencrypt-prod":
			foundIssuer = true
			if e.To.Namespace != "" {
				t.Errorf("ClusterIssuer should be cluster-scoped, got ns=%q", e.To.Namespace)
			}
		default:
			t.Errorf("unexpected edge: %+v", e)
		}
	}

	if !foundSecret {
		t.Error("missing Certificate -> Secret edge")
	}
	if !foundIssuer {
		t.Error("missing Certificate -> ClusterIssuer edge")
	}
}

func TestCertManager_CertificateToNamespacedIssuer(t *testing.T) {
	g := New(WithResolver(NewCertManagerResolver()))

	cert := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "cert-manager.io/v1", "kind": "Certificate",
		"metadata": map[string]interface{}{
			"name": "internal-cert", "namespace": "apps",
		},
		"spec": map[string]interface{}{
			"secretName": "internal-tls",
			"issuerRef": map[string]interface{}{
				"name":  "ca-issuer",
				"kind":  "Issuer",
				"group": "cert-manager.io",
			},
		},
	}}

	secret := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Secret",
		"metadata": map[string]interface{}{
			"name": "internal-tls", "namespace": "apps",
		},
	}}

	issuer := newObj("cert-manager.io", "v1", "Issuer", "apps", "ca-issuer")

	g.Load([]unstructured.Unstructured{cert, secret, issuer})

	certRef := ObjectRef{Group: "cert-manager.io", Kind: "Certificate", Namespace: "apps", Name: "internal-cert"}
	deps := g.DependenciesOf(certRef)

	var foundIssuer bool
	for _, e := range deps {
		if e.To.Kind == "Issuer" && e.To.Name == "ca-issuer" {
			foundIssuer = true
			if e.To.Namespace != "apps" {
				t.Errorf("Issuer should be same-namespace, got ns=%q", e.To.Namespace)
			}
		}
	}
	if !foundIssuer {
		t.Errorf("missing Certificate -> Issuer edge, got deps: %v", deps)
	}
}

func TestCertManager_IngressAnnotations(t *testing.T) {
	g := New(WithResolver(NewCertManagerResolver()))

	ingress := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "networking.k8s.io/v1", "kind": "Ingress",
		"metadata": map[string]interface{}{
			"name": "web", "namespace": "default",
			"annotations": map[string]interface{}{
				"cert-manager.io/cluster-issuer": "letsencrypt-prod",
			},
		},
	}}

	issuer := newObj("cert-manager.io", "v1", "ClusterIssuer", "", "letsencrypt-prod")

	g.Load([]unstructured.Unstructured{ingress, issuer})

	ingressRef := ObjectRef{Group: "networking.k8s.io", Kind: "Ingress", Namespace: "default", Name: "web"}
	deps := g.DependenciesOf(ingressRef)
	if len(deps) != 1 {
		t.Fatalf("expected 1 dependency, got %d: %v", len(deps), deps)
	}
	if deps[0].To.Kind != "ClusterIssuer" || deps[0].To.Name != "letsencrypt-prod" {
		t.Fatalf("unexpected target: %v", deps[0].To)
	}
	if deps[0].To.Namespace != "" {
		t.Errorf("ClusterIssuer should be cluster-scoped, got ns=%q", deps[0].To.Namespace)
	}
	if deps[0].Field != "metadata.annotations[cert-manager.io/cluster-issuer]" {
		t.Errorf("unexpected field: %q", deps[0].Field)
	}
}

func TestCertManager_IngressNamespacedIssuer(t *testing.T) {
	g := New(WithResolver(NewCertManagerResolver()))

	ingress := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "networking.k8s.io/v1", "kind": "Ingress",
		"metadata": map[string]interface{}{
			"name": "api", "namespace": "staging",
			"annotations": map[string]interface{}{
				"cert-manager.io/issuer": "self-signed",
			},
		},
	}}

	issuer := newObj("cert-manager.io", "v1", "Issuer", "staging", "self-signed")

	g.Load([]unstructured.Unstructured{ingress, issuer})

	ingressRef := ObjectRef{Group: "networking.k8s.io", Kind: "Ingress", Namespace: "staging", Name: "api"}
	deps := g.DependenciesOf(ingressRef)
	if len(deps) != 1 {
		t.Fatalf("expected 1 dependency, got %d: %v", len(deps), deps)
	}
	if deps[0].To.Kind != "Issuer" || deps[0].To.Name != "self-signed" {
		t.Fatalf("unexpected target: %v", deps[0].To)
	}
	if deps[0].To.Namespace != "staging" {
		t.Errorf("Issuer should be same-namespace, got ns=%q", deps[0].To.Namespace)
	}
}

func TestCertManager_ReverseAdd(t *testing.T) {
	g := New(WithResolver(NewCertManagerResolver()))

	// Add Certificate first — Secret doesn't exist yet.
	cert := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "cert-manager.io/v1", "kind": "Certificate",
		"metadata": map[string]interface{}{
			"name": "web-cert", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"secretName": "web-tls",
			"issuerRef": map[string]interface{}{
				"name":  "letsencrypt-prod",
				"kind":  "ClusterIssuer",
				"group": "cert-manager.io",
			},
		},
	}}
	g.Add(cert)

	certRef := ObjectRef{Group: "cert-manager.io", Kind: "Certificate", Namespace: "default", Name: "web-cert"}
	if deps := g.DependenciesOf(certRef); len(deps) != 0 {
		t.Fatalf("expected 0 deps before targets, got %d", len(deps))
	}

	// Add Secret — reverse resolution creates the edge.
	secret := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Secret",
		"metadata": map[string]interface{}{
			"name": "web-tls", "namespace": "default",
		},
	}}
	g.Add(secret)

	deps := g.DependenciesOf(certRef)
	if len(deps) != 1 {
		t.Fatalf("expected 1 dependency after adding Secret, got %d: %v", len(deps), deps)
	}
	if deps[0].To.Kind != "Secret" || deps[0].To.Name != "web-tls" {
		t.Errorf("expected edge to Secret/web-tls, got %v", deps[0].To)
	}
}

func TestCertManager_Extract(t *testing.T) {
	r := NewCertManagerResolver()

	cert := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "cert-manager.io/v1", "kind": "Certificate",
		"metadata": map[string]interface{}{
			"name": "web-cert", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"secretName": "web-tls",
			"issuerRef": map[string]interface{}{
				"name":  "letsencrypt-prod",
				"kind":  "ClusterIssuer",
				"group": "cert-manager.io",
			},
		},
	}}

	edges := r.Extract(cert)
	if len(edges) != 2 {
		t.Fatalf("expected 2 extract edges (secret + issuer), got %d: %v", len(edges), edges)
	}

	foundSecret, foundIssuer := false, false
	for _, e := range edges {
		switch {
		case e.To.Kind == "Secret":
			foundSecret = true
			if e.To.Namespace != "default" {
				t.Errorf("Secret should be same-namespace, got ns=%q", e.To.Namespace)
			}
		case e.To.Kind == "ClusterIssuer":
			foundIssuer = true
			if e.To.Namespace != "" {
				t.Errorf("ClusterIssuer should be cluster-scoped, got ns=%q", e.To.Namespace)
			}
		}
	}
	if !foundSecret {
		t.Error("missing Certificate -> Secret extract edge")
	}
	if !foundIssuer {
		t.Error("missing Certificate -> ClusterIssuer extract edge")
	}

	// Ingress annotation extraction
	ingress := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "networking.k8s.io/v1", "kind": "Ingress",
		"metadata": map[string]interface{}{
			"name": "web", "namespace": "default",
			"annotations": map[string]interface{}{
				"cert-manager.io/cluster-issuer": "letsencrypt-prod",
			},
		},
	}}

	ingressEdges := r.Extract(ingress)
	if len(ingressEdges) != 1 {
		t.Fatalf("expected 1 ingress extract edge, got %d: %v", len(ingressEdges), ingressEdges)
	}
	if ingressEdges[0].To.Kind != "ClusterIssuer" || ingressEdges[0].To.Namespace != "" {
		t.Errorf("unexpected ingress extract edge: %v", ingressEdges[0])
	}
}
