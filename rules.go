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
	"k8s.io/apimachinery/pkg/labels"
)

// Rule is a declarative dependency rule.
type Rule interface {
	rule() // marker method
}

// RefRule matches a field that contains the name of a target resource.
// When NamespaceFieldPath is set, the namespace is read from the object.
// When empty, resolution tries the source's namespace first, then
// cluster-scoped (""). At most one matches because K8s does not allow
// the same GroupKind to be both namespaced and cluster-scoped.
type RefRule struct {
	FromGroup, FromKind string
	ToGroup, ToKind     string
	FieldPath           string // path to name value(s)
	NamespaceFieldPath  string // optional: path to namespace value(s)
}

func (RefRule) rule() {}

// NameRefRule matches a field that contains the name of a target resource.
type NameRefRule struct {
	FromGroup, FromKind string
	ToGroup, ToKind     string
	FieldPath           string
	SameNamespace       bool
}

func (NameRefRule) rule() {}

// NamespacedNameRefRule matches explicit namespace+name field pairs.
type NamespacedNameRefRule struct {
	FromGroup, FromKind string
	ToGroup, ToKind     string
	NameFieldPath       string
	NamespaceFieldPath  string // "" means same namespace as source
}

func (NamespacedNameRefRule) rule() {}

// LabelSelectorRule matches target resources by label selector.
type LabelSelectorRule struct {
	FromGroup, FromKind string
	ToGroup, ToKind     string
	SelectorFieldPath   string
	TargetNamespace     string // "" = same namespace; "*" = all namespaces
}

func (LabelSelectorRule) rule() {}

// NewRuleResolver creates a Resolver from declarative rules.
func NewRuleResolver(name string, rules ...Rule) Resolver {
	return &ruleResolver{name: name, rules: rules}
}

type ruleResolver struct {
	name  string
	rules []Rule
}

func (r *ruleResolver) Name() string { return r.name }

func (r *ruleResolver) Resolve(obj *unstructured.Unstructured, lookup Lookup) []Edge {
	ref := RefFromUnstructured(obj)
	var edges []Edge

	for _, rule := range r.rules {
		switch rule := rule.(type) {
		case NameRefRule:
			edges = append(edges, resolveNameRef(ref, obj, rule, lookup)...)
		case NamespacedNameRefRule:
			edges = append(edges, resolveNamespacedNameRef(ref, obj, rule, lookup)...)
		case RefRule:
			edges = append(edges, resolveRef(ref, obj, rule, lookup)...)
		case LabelSelectorRule:
			edges = append(edges, resolveLabelSelector(ref, obj, rule, lookup)...)
		}
	}

	return edges
}

func resolveNameRef(ref ObjectRef, obj *unstructured.Unstructured, rule NameRefRule, lookup Lookup) []Edge {
	if ref.Group != rule.FromGroup || ref.Kind != rule.FromKind {
		return resolveNameRefReverse(ref, obj, rule, lookup)
	}

	var edges []Edge
	names := extractFieldValues(obj.Object, rule.FieldPath)
	for _, name := range names {
		toRef := ObjectRef{
			Group: rule.ToGroup,
			Kind:  rule.ToKind,
			Name:  name,
		}
		if rule.SameNamespace {
			toRef.Namespace = ref.Namespace
		}
		if _, ok := lookup.Get(toRef); ok {
			edgeType := EdgeNameRef
			if rule.SameNamespace {
				edgeType = EdgeLocalNameRef
			}
			edges = append(edges, Edge{
				From:     ref,
				To:       toRef,
				Type:     edgeType,
				Resolver: "rule",
				Field:    rule.FieldPath,
			})
		}
	}
	return edges
}

func resolveNameRefReverse(ref ObjectRef, obj *unstructured.Unstructured, rule NameRefRule, lookup Lookup) []Edge {
	if ref.Group != rule.ToGroup || ref.Kind != rule.ToKind {
		return nil
	}

	var edges []Edge
	var sources []*unstructured.Unstructured
	if rule.SameNamespace {
		sources = lookup.ListInNamespace(rule.FromGroup, rule.FromKind, ref.Namespace)
	} else {
		sources = lookup.List(rule.FromGroup, rule.FromKind)
	}

	for _, src := range sources {
		srcRef := RefFromUnstructured(src)
		names := extractFieldValues(src.Object, rule.FieldPath)
		for _, name := range names {
			if name == ref.Name {
				edgeType := EdgeNameRef
				if rule.SameNamespace {
					edgeType = EdgeLocalNameRef
				}
				edges = append(edges, Edge{
					From:     srcRef,
					To:       ref,
					Type:     edgeType,
					Resolver: "rule",
					Field:    rule.FieldPath,
				})
			}
		}
	}
	return edges
}

func resolveRef(ref ObjectRef, obj *unstructured.Unstructured, rule RefRule, lookup Lookup) []Edge {
	if ref.Group != rule.FromGroup || ref.Kind != rule.FromKind {
		return resolveRefReverse(ref, obj, rule, lookup)
	}

	names := extractFieldValues(obj.Object, rule.FieldPath)
	if len(names) == 0 {
		return nil
	}

	var edges []Edge

	if rule.NamespaceFieldPath != "" {
		namespaces := extractFieldValues(obj.Object, rule.NamespaceFieldPath)
		for i, name := range names {
			ns := ref.Namespace
			if i < len(namespaces) {
				ns = namespaces[i]
			}
			toRef := ObjectRef{
				Group:     rule.ToGroup,
				Kind:      rule.ToKind,
				Namespace: ns,
				Name:      name,
			}
			if _, ok := lookup.Get(toRef); ok {
				edges = append(edges, Edge{
					From:     ref,
					To:       toRef,
					Type:     EdgeNameRef,
					Resolver: "rule",
					Field:    rule.FieldPath,
				})
			}
		}
		return edges
	}

	// No NamespaceFieldPath: try same-namespace, then cluster-scoped.
	for _, name := range names {
		sameNS := ObjectRef{
			Group:     rule.ToGroup,
			Kind:      rule.ToKind,
			Namespace: ref.Namespace,
			Name:      name,
		}
		if _, ok := lookup.Get(sameNS); ok {
			edges = append(edges, Edge{
				From:     ref,
				To:       sameNS,
				Type:     EdgeLocalNameRef,
				Resolver: "rule",
				Field:    rule.FieldPath,
			})
			continue
		}
		clusterScoped := ObjectRef{
			Group: rule.ToGroup,
			Kind:  rule.ToKind,
			Name:  name,
		}
		if _, ok := lookup.Get(clusterScoped); ok {
			edges = append(edges, Edge{
				From:     ref,
				To:       clusterScoped,
				Type:     EdgeNameRef,
				Resolver: "rule",
				Field:    rule.FieldPath,
			})
		}
	}
	return edges
}

func resolveRefReverse(ref ObjectRef, obj *unstructured.Unstructured, rule RefRule, lookup Lookup) []Edge {
	return nil // implemented in Task 2
}

func resolveNamespacedNameRef(ref ObjectRef, obj *unstructured.Unstructured, rule NamespacedNameRefRule, lookup Lookup) []Edge {
	if ref.Group != rule.FromGroup || ref.Kind != rule.FromKind {
		return nil
	}

	var edges []Edge
	names := extractFieldValues(obj.Object, rule.NameFieldPath)

	var namespaces []string
	if rule.NamespaceFieldPath == "" {
		for range names {
			namespaces = append(namespaces, ref.Namespace)
		}
	} else {
		namespaces = extractFieldValues(obj.Object, rule.NamespaceFieldPath)
	}

	for i, name := range names {
		ns := ref.Namespace
		if i < len(namespaces) {
			ns = namespaces[i]
		}
		toRef := ObjectRef{
			Group:     rule.ToGroup,
			Kind:      rule.ToKind,
			Namespace: ns,
			Name:      name,
		}
		if _, ok := lookup.Get(toRef); ok {
			edges = append(edges, Edge{
				From:     ref,
				To:       toRef,
				Type:     EdgeNameRef,
				Resolver: "rule",
				Field:    rule.NameFieldPath,
			})
		}
	}
	return edges
}

func resolveLabelSelector(ref ObjectRef, obj *unstructured.Unstructured, rule LabelSelectorRule, lookup Lookup) []Edge {
	if ref.Group != rule.FromGroup || ref.Kind != rule.FromKind {
		return resolveLabelSelectorReverse(ref, obj, rule, lookup)
	}

	selectorMap := extractMapValue(obj.Object, rule.SelectorFieldPath)
	if selectorMap == nil {
		return nil
	}

	sel := labels.SelectorFromSet(labels.Set(selectorMap))

	ns := ref.Namespace
	if rule.TargetNamespace != "" && rule.TargetNamespace != "*" {
		ns = rule.TargetNamespace
	}

	var targets []*unstructured.Unstructured
	if rule.TargetNamespace == "*" {
		targets = lookup.List(rule.ToGroup, rule.ToKind)
	} else {
		targets = lookup.ListInNamespace(rule.ToGroup, rule.ToKind, ns)
	}

	var edges []Edge
	for _, target := range targets {
		targetLabels := target.GetLabels()
		if sel.Matches(labels.Set(targetLabels)) {
			edges = append(edges, Edge{
				From:     ref,
				To:       RefFromUnstructured(target),
				Type:     EdgeLabelSelector,
				Resolver: "rule",
				Field:    rule.SelectorFieldPath,
			})
		}
	}
	return edges
}

func resolveLabelSelectorReverse(ref ObjectRef, obj *unstructured.Unstructured, rule LabelSelectorRule, lookup Lookup) []Edge {
	if ref.Group != rule.ToGroup || ref.Kind != rule.ToKind {
		return nil
	}

	targetLabels := obj.GetLabels()
	if len(targetLabels) == 0 {
		return nil
	}

	var sources []*unstructured.Unstructured
	if rule.TargetNamespace == "*" {
		sources = lookup.List(rule.FromGroup, rule.FromKind)
	} else {
		sources = lookup.ListInNamespace(rule.FromGroup, rule.FromKind, ref.Namespace)
	}

	var edges []Edge
	for _, src := range sources {
		selectorMap := extractMapValue(src.Object, rule.SelectorFieldPath)
		if selectorMap == nil {
			continue
		}
		sel := labels.SelectorFromSet(labels.Set(selectorMap))
		if sel.Matches(labels.Set(targetLabels)) {
			edges = append(edges, Edge{
				From:     RefFromUnstructured(src),
				To:       ref,
				Type:     EdgeLabelSelector,
				Resolver: "rule",
				Field:    rule.SelectorFieldPath,
			})
		}
	}
	return edges
}

// extractFieldValues extracts string values from a nested map using a
// dot-separated field path. Supports [*] wildcard for slices.
func extractFieldValues(obj map[string]interface{}, path string) []string {
	parts := splitFieldPath(path)
	return extractRecursive(obj, parts)
}

func extractRecursive(data interface{}, parts []string) []string {
	if len(parts) == 0 {
		if s, ok := data.(string); ok {
			return []string{s}
		}
		return nil
	}

	part := parts[0]
	rest := parts[1:]

	if strings.HasSuffix(part, "[*]") {
		key := strings.TrimSuffix(part, "[*]")
		m, ok := data.(map[string]interface{})
		if !ok {
			return nil
		}
		arr, ok := m[key].([]interface{})
		if !ok {
			return nil
		}
		var result []string
		for _, item := range arr {
			result = append(result, extractRecursive(item, rest)...)
		}
		return result
	}

	m, ok := data.(map[string]interface{})
	if !ok {
		return nil
	}
	val, ok := m[part]
	if !ok {
		return nil
	}
	return extractRecursive(val, rest)
}

func extractMapValue(obj map[string]interface{}, path string) map[string]string {
	parts := splitFieldPath(path)
	var current interface{} = obj
	for _, part := range parts {
		m, ok := current.(map[string]interface{})
		if !ok {
			return nil
		}
		current, ok = m[part]
		if !ok {
			return nil
		}
	}

	m, ok := current.(map[string]interface{})
	if !ok {
		return nil
	}
	result := make(map[string]string, len(m))
	for k, v := range m {
		if s, ok := v.(string); ok {
			result[k] = s
		}
	}
	return result
}

func splitFieldPath(path string) []string {
	return strings.Split(path, ".")
}
