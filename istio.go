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

// VirtualService host field paths for all protocol blocks.
var vsHostPaths = []string{
	"spec.http[*].route[*].destination.host",
	"spec.tcp[*].route[*].destination.host",
	"spec.tls[*].route[*].destination.host",
}

// NewIstioResolver returns a resolver for Istio resource references.
// Handles VirtualService→Service and DestinationRule→Service via DNS host
// name parsing, and AuthorizationPolicy→Pod via label selector matching.
//
// DNS host formats:
//   - "reviews"                            → Service in same namespace
//   - "reviews.prod"                       → Service in prod namespace
//   - "reviews.prod.svc"                   → same, partial FQDN
//   - "reviews.prod.svc.cluster.local"     → same, full FQDN
//   - "api.example.com"                    → external, no edge produced
//
// Not registered by NewDefault() — opt in with WithResolver(NewIstioResolver()).
func NewIstioResolver() Resolver {
	return &istioResolver{
		ruleResolver: NewRuleResolver("istio",
			LabelSelectorRule{
				FromGroup:         "security.istio.io",
				FromKind:          "AuthorizationPolicy",
				ToKind:            "Pod",
				SelectorFieldPath: "spec.selector",
			},
		),
	}
}

type istioResolver struct {
	ruleResolver Resolver
}

func (r *istioResolver) Name() string { return "istio" }

func (r *istioResolver) Extract(obj *unstructured.Unstructured) []Edge {
	var edges []Edge
	edges = append(edges, r.ruleResolver.Extract(obj)...)
	edges = append(edges, r.extractHosts(obj)...)
	return edges
}

func (r *istioResolver) Resolve(obj *unstructured.Unstructured, lookup Lookup) []Edge {
	var edges []Edge
	edges = append(edges, r.ruleResolver.Resolve(obj, lookup)...)
	edges = append(edges, r.resolveHosts(obj, lookup)...)
	return edges
}

// extractHosts returns forward-only edges from DNS host fields.
func (r *istioResolver) extractHosts(obj *unstructured.Unstructured) []Edge {
	ref := RefFromUnstructured(obj)
	gvk := obj.GroupVersionKind()
	if !isIstioNetworking(gvk.Group, gvk.Kind) {
		return nil
	}

	hosts := extractIstioHosts(obj)
	var edges []Edge
	for _, h := range hosts {
		name, ns, ok := parseIstioHost(h.host, ref.Namespace)
		if !ok {
			continue
		}
		edges = append(edges, Edge{
			From:     ref,
			To:       ObjectRef{Kind: "Service", Namespace: ns, Name: name},
			Type:     EdgeRef,
			Resolver: "istio",
			Field:    h.field,
		})
	}
	return edges
}

// resolveHosts handles bidirectional resolution for DNS host fields.
func (r *istioResolver) resolveHosts(obj *unstructured.Unstructured, lookup Lookup) []Edge {
	gvk := obj.GroupVersionKind()
	if isIstioNetworking(gvk.Group, gvk.Kind) {
		return r.resolveHostsForward(obj, lookup)
	}
	return r.resolveHostsReverse(obj, lookup)
}

func (r *istioResolver) resolveHostsForward(obj *unstructured.Unstructured, lookup Lookup) []Edge {
	ref := RefFromUnstructured(obj)
	hosts := extractIstioHosts(obj)
	var edges []Edge
	for _, h := range hosts {
		name, ns, ok := parseIstioHost(h.host, ref.Namespace)
		if !ok {
			continue
		}
		svcRef := ObjectRef{Kind: "Service", Namespace: ns, Name: name}
		if _, ok := lookup.Get(svcRef); ok {
			edges = append(edges, Edge{
				From:     ref,
				To:       svcRef,
				Type:     EdgeRef,
				Resolver: "istio",
				Field:    h.field,
			})
		}
	}
	return edges
}

func (r *istioResolver) resolveHostsReverse(obj *unstructured.Unstructured, lookup Lookup) []Edge {
	ref := RefFromUnstructured(obj)
	if ref.Kind != "Service" || ref.Group != "" {
		return nil
	}

	var edges []Edge
	for _, kind := range []string{"VirtualService", "DestinationRule"} {
		for _, src := range lookup.List("networking.istio.io", kind) {
			srcRef := RefFromUnstructured(src)
			for _, h := range extractIstioHosts(src) {
				name, ns, ok := parseIstioHost(h.host, srcRef.Namespace)
				if !ok {
					continue
				}
				if name == ref.Name && ns == ref.Namespace {
					edges = append(edges, Edge{
						From:     srcRef,
						To:       ref,
						Type:     EdgeRef,
						Resolver: "istio",
						Field:    h.field,
					})
				}
			}
		}
	}
	return edges
}

func isIstioNetworking(group, kind string) bool {
	return group == "networking.istio.io" && (kind == "VirtualService" || kind == "DestinationRule")
}

// istioHostRef pairs a host string with the field path it was extracted from.
type istioHostRef struct {
	host  string
	field string
}

func extractIstioHosts(obj *unstructured.Unstructured) []istioHostRef {
	gvk := obj.GroupVersionKind()
	var refs []istioHostRef

	switch gvk.Kind {
	case "VirtualService":
		for _, path := range vsHostPaths {
			for _, host := range extractFieldValues(obj.Object, path) {
				refs = append(refs, istioHostRef{host: host, field: path})
			}
		}
	case "DestinationRule":
		hosts := extractFieldValues(obj.Object, "spec.host")
		if len(hosts) > 0 {
			refs = append(refs, istioHostRef{host: hosts[0], field: "spec.host"})
		}
	}

	return refs
}

// parseIstioHost parses an Istio DNS host string into a Service name and
// namespace. Returns ok=false for external hostnames.
//
// Formats:
//
//	"reviews"                         → (reviews, sourceNS)
//	"reviews.prod"                    → (reviews, prod)
//	"reviews.prod.svc"                → (reviews, prod)
//	"reviews.prod.svc.cluster.local"  → (reviews, prod)
//	"api.example.com"                 → ("", "", false)
func parseIstioHost(host, sourceNamespace string) (name, namespace string, ok bool) {
	if host == "" {
		return "", "", false
	}

	parts := strings.Split(host, ".")

	switch {
	case len(parts) == 1:
		return parts[0], sourceNamespace, true
	case len(parts) == 2:
		return parts[0], parts[1], true
	case len(parts) == 3 && parts[2] == "svc":
		return parts[0], parts[1], true
	case len(parts) == 5 && parts[2] == "svc" && parts[3] == "cluster" && parts[4] == "local":
		return parts[0], parts[1], true
	default:
		return "", "", false
	}
}
