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

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		case RefRule:
			edges = append(edges, resolveRef(ref, obj, rule, lookup)...)
		case LabelSelectorRule:
			edges = append(edges, resolveLabelSelector(ref, obj, rule, lookup)...)
		}
	}

	return edges
}

func resolveRef(ref ObjectRef, obj *unstructured.Unstructured, rule RefRule, lookup Lookup) []Edge {
	if ref.Group != rule.FromGroup || ref.Kind != rule.FromKind {
		return resolveRefReverse(ref, obj, rule, lookup)
	}

	values := extractRawValues(obj.Object, rule.FieldPath)
	if len(values) == 0 {
		return nil
	}

	var namespaces []string
	if rule.NamespaceFieldPath != "" {
		namespaces = extractFieldValues(obj.Object, rule.NamespaceFieldPath)
	}

	var edges []Edge
	for i, val := range values {
		switch v := val.(type) {
		case string:
			edges = append(edges, resolveBareName(ref, v, i, namespaces, rule, lookup)...)
		case map[string]interface{}:
			edges = append(edges, resolveTypedRef(ref, v, rule, lookup)...)
		}
	}
	return edges
}

// resolveBareName handles a bare string name value.
func resolveBareName(ref ObjectRef, name string, index int, namespaces []string, rule RefRule, lookup Lookup) []Edge {
	if len(namespaces) > 0 {
		ns := ref.Namespace
		if index < len(namespaces) {
			ns = namespaces[index]
		}
		toRef := ObjectRef{
			Group:     rule.ToGroup,
			Kind:      rule.ToKind,
			Namespace: ns,
			Name:      name,
		}
		if _, ok := lookup.Get(toRef); ok {
			return []Edge{{
				From:     ref,
				To:       toRef,
				Type:     EdgeNameRef,
				Resolver: "rule",
				Field:    rule.FieldPath,
			}}
		}
		return nil
	}

	// No NamespaceFieldPath: try same-namespace, then cluster-scoped.
	sameNS := ObjectRef{
		Group:     rule.ToGroup,
		Kind:      rule.ToKind,
		Namespace: ref.Namespace,
		Name:      name,
	}
	if _, ok := lookup.Get(sameNS); ok {
		return []Edge{{
			From:     ref,
			To:       sameNS,
			Type:     EdgeLocalNameRef,
			Resolver: "rule",
			Field:    rule.FieldPath,
		}}
	}
	clusterScoped := ObjectRef{
		Group: rule.ToGroup,
		Kind:  rule.ToKind,
		Name:  name,
	}
	if _, ok := lookup.Get(clusterScoped); ok {
		return []Edge{{
			From:     ref,
			To:       clusterScoped,
			Type:     EdgeNameRef,
			Resolver: "rule",
			Field:    rule.FieldPath,
		}}
	}
	return nil
}

// resolveTypedRef handles a typed reference map (kind/name/apiGroup).
func resolveTypedRef(ref ObjectRef, m map[string]interface{}, rule RefRule, lookup Lookup) []Edge {
	toRef, ok := parseTypedRef(m)
	if !ok {
		return nil
	}

	// Apply type constraint if set.
	if rule.ToGroup != "" && toRef.Group != rule.ToGroup {
		return nil
	}
	if rule.ToKind != "" && toRef.Kind != rule.ToKind {
		return nil
	}

	// If typed ref has explicit namespace, use it directly.
	if toRef.Namespace != "" {
		if _, ok := lookup.Get(toRef); ok {
			return []Edge{{
				From:     ref,
				To:       toRef,
				Type:     EdgeNameRef,
				Resolver: "rule",
				Field:    rule.FieldPath,
			}}
		}
		return nil
	}

	// No namespace in ref: try same-namespace, then cluster-scoped.
	sameNS := toRef
	sameNS.Namespace = ref.Namespace
	if _, ok := lookup.Get(sameNS); ok {
		return []Edge{{
			From:     ref,
			To:       sameNS,
			Type:     EdgeLocalNameRef,
			Resolver: "rule",
			Field:    rule.FieldPath,
		}}
	}
	if _, ok := lookup.Get(toRef); ok {
		return []Edge{{
			From:     ref,
			To:       toRef,
			Type:     EdgeNameRef,
			Resolver: "rule",
			Field:    rule.FieldPath,
		}}
	}
	return nil
}

func resolveRefReverse(ref ObjectRef, obj *unstructured.Unstructured, rule RefRule, lookup Lookup) []Edge {
	// Type constraint guard: skip if the added object can't be a target.
	if rule.ToKind != "" && (ref.Group != rule.ToGroup || ref.Kind != rule.ToKind) {
		return nil
	}

	// For unconstrained rules (ToKind empty), scan all sources.
	// For constrained rules, scope by namespace when possible.
	var sources []*unstructured.Unstructured
	if rule.ToKind == "" || rule.NamespaceFieldPath != "" {
		sources = lookup.List(rule.FromGroup, rule.FromKind)
	} else if ref.Namespace != "" {
		sources = lookup.ListInNamespace(rule.FromGroup, rule.FromKind, ref.Namespace)
	} else {
		sources = lookup.List(rule.FromGroup, rule.FromKind)
	}

	var edges []Edge
	for _, src := range sources {
		srcRef := RefFromUnstructured(src)
		values := extractRawValues(src.Object, rule.FieldPath)

		for i, val := range values {
			switch v := val.(type) {
			case string:
				edge := reverseMatchBareName(srcRef, ref, v, i, src, rule)
				if edge != nil {
					edges = append(edges, *edge)
				}
			case map[string]interface{}:
				edge := reverseMatchTypedRef(srcRef, ref, v, rule)
				if edge != nil {
					edges = append(edges, *edge)
				}
			}
		}
	}
	return edges
}

func reverseMatchBareName(srcRef, targetRef ObjectRef, name string, index int, src *unstructured.Unstructured, rule RefRule) *Edge {
	if name != targetRef.Name {
		return nil
	}

	if rule.NamespaceFieldPath != "" {
		namespaces := extractFieldValues(src.Object, rule.NamespaceFieldPath)
		ns := srcRef.Namespace
		if index < len(namespaces) {
			ns = namespaces[index]
		}
		if ns != targetRef.Namespace {
			return nil
		}
		return &Edge{
			From:     srcRef,
			To:       targetRef,
			Type:     EdgeNameRef,
			Resolver: "rule",
			Field:    rule.FieldPath,
		}
	}

	edgeType := EdgeLocalNameRef
	if targetRef.Namespace == "" {
		edgeType = EdgeNameRef
	}
	return &Edge{
		From:     srcRef,
		To:       targetRef,
		Type:     edgeType,
		Resolver: "rule",
		Field:    rule.FieldPath,
	}
}

func reverseMatchTypedRef(srcRef, targetRef ObjectRef, m map[string]interface{}, rule RefRule) *Edge {
	parsed, ok := parseTypedRef(m)
	if !ok {
		return nil
	}

	// Apply type constraint if set.
	if rule.ToGroup != "" && parsed.Group != rule.ToGroup {
		return nil
	}
	if rule.ToKind != "" && parsed.Kind != rule.ToKind {
		return nil
	}

	// Check if the parsed ref matches the target.
	if parsed.Group != targetRef.Group || parsed.Kind != targetRef.Kind || parsed.Name != targetRef.Name {
		return nil
	}

	// Namespace matching.
	if parsed.Namespace != "" {
		if parsed.Namespace != targetRef.Namespace {
			return nil
		}
	} else {
		// No namespace in ref: matches same-namespace or cluster-scoped.
		if targetRef.Namespace != "" && targetRef.Namespace != srcRef.Namespace {
			return nil
		}
	}

	edgeType := EdgeLocalNameRef
	if parsed.Namespace != "" || targetRef.Namespace == "" {
		edgeType = EdgeNameRef
	}
	return &Edge{
		From:     srcRef,
		To:       targetRef,
		Type:     edgeType,
		Resolver: "rule",
		Field:    rule.FieldPath,
	}
}

// parseTypedRef extracts an ObjectRef from a typed reference map.
// Expects at minimum "kind" and "name" keys. Group is read from
// "apiGroup", "group", or parsed from "apiVersion". Namespace is
// read from "namespace" if present.
func parseTypedRef(m map[string]interface{}) (ObjectRef, bool) {
	kind, _ := m["kind"].(string)
	name, _ := m["name"].(string)
	if kind == "" || name == "" {
		return ObjectRef{}, false
	}

	var group string
	if g, ok := m["apiGroup"].(string); ok {
		group = g
	} else if g, ok := m["group"].(string); ok {
		group = g
	} else if av, ok := m["apiVersion"].(string); ok {
		group = extractGroup(av)
	}

	ref := ObjectRef{
		Group: group,
		Kind:  kind,
		Name:  name,
	}
	if ns, ok := m["namespace"].(string); ok {
		ref.Namespace = ns
	}
	return ref, true
}

func resolveLabelSelector(ref ObjectRef, obj *unstructured.Unstructured, rule LabelSelectorRule, lookup Lookup) []Edge {
	if ref.Group != rule.FromGroup || ref.Kind != rule.FromKind {
		return resolveLabelSelectorReverse(ref, obj, rule, lookup)
	}

	sel := extractSelector(obj.Object, rule.SelectorFieldPath)
	if sel == nil {
		return nil
	}

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
		sel := extractSelector(src.Object, rule.SelectorFieldPath)
		if sel == nil {
			continue
		}
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

// extractRawValues extracts raw values (strings, maps, etc.) from a
// nested map using a dot-separated field path. Like extractFieldValues
// but returns the leaf values without type restriction.
func extractRawValues(obj map[string]interface{}, path string) []interface{} {
	parts := splitFieldPath(path)
	return extractRawRecursive(obj, parts)
}

func extractRawRecursive(data interface{}, parts []string) []interface{} {
	if len(parts) == 0 {
		return []interface{}{data}
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
		var result []interface{}
		for _, item := range arr {
			result = append(result, extractRawRecursive(item, rest)...)
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
	return extractRawRecursive(val, rest)
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

// extractSelector navigates to a field path and parses the value as a
// labels.Selector. Handles both formats:
//   - Full LabelSelector: {matchLabels: {...}, matchExpressions: [...]}
//   - Flat map: {key: value, ...} (e.g., Service.spec.selector)
func extractSelector(obj map[string]interface{}, path string) labels.Selector {
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

	_, hasMatchLabels := m["matchLabels"]
	_, hasMatchExpressions := m["matchExpressions"]

	if hasMatchLabels || hasMatchExpressions {
		ls := &metav1.LabelSelector{}
		if ml, ok := m["matchLabels"].(map[string]interface{}); ok {
			ls.MatchLabels = make(map[string]string, len(ml))
			for k, v := range ml {
				if s, ok := v.(string); ok {
					ls.MatchLabels[k] = s
				}
			}
		}
		if me, ok := m["matchExpressions"].([]interface{}); ok {
			for _, item := range me {
				expr, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				req := metav1.LabelSelectorRequirement{}
				if key, ok := expr["key"].(string); ok {
					req.Key = key
				}
				if op, ok := expr["operator"].(string); ok {
					req.Operator = metav1.LabelSelectorOperator(op)
				}
				if vals, ok := expr["values"].([]interface{}); ok {
					for _, v := range vals {
						if s, ok := v.(string); ok {
							req.Values = append(req.Values, s)
						}
					}
				}
				ls.MatchExpressions = append(ls.MatchExpressions, req)
			}
		}
		sel, err := metav1.LabelSelectorAsSelector(ls)
		if err != nil {
			return nil
		}
		return sel
	}

	// Flat map (e.g., Service.spec.selector)
	flat := make(map[string]string, len(m))
	for k, v := range m {
		if s, ok := v.(string); ok {
			flat[k] = s
		}
	}
	return labels.SelectorFromSet(labels.Set(flat))
}

func splitFieldPath(path string) []string {
	return strings.Split(path, ".")
}
