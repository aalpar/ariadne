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

// NewEventResolver returns a resolver that creates edges from K8s Events
// to their involvedObject.
func NewEventResolver() Resolver {
	return &eventResolver{}
}

type eventResolver struct{}

func (e *eventResolver) Name() string { return "event" }

func (e *eventResolver) Extract(obj *unstructured.Unstructured) []Edge {
	ref := RefFromUnstructured(obj)
	if ref.Kind != "Event" {
		return nil
	}

	involved, ok := obj.Object["involvedObject"].(map[string]interface{})
	if !ok {
		return nil
	}

	name, _ := involved["name"].(string)
	ns, _ := involved["namespace"].(string)
	kind, _ := involved["kind"].(string)
	apiVersion, _ := involved["apiVersion"].(string)

	if name == "" || kind == "" {
		return nil
	}

	return []Edge{{
		From: ref,
		To: ObjectRef{
			Group:     extractGroup(apiVersion),
			Kind:      kind,
			Namespace: ns,
			Name:      name,
		},
		Type:     EdgeEvent,
		Resolver: "event",
		Field:    "involvedObject",
	}}
}

func (e *eventResolver) Resolve(obj *unstructured.Unstructured, lookup Lookup) []Edge {
	ref := RefFromUnstructured(obj)
	if ref.Kind == "Event" {
		return e.resolveEvent(ref, obj, lookup)
	}
	return e.resolveReverseEvent(ref, obj, lookup)
}

func (e *eventResolver) resolveEvent(ref ObjectRef, obj *unstructured.Unstructured, lookup Lookup) []Edge {
	involved, ok := obj.Object["involvedObject"].(map[string]interface{})
	if !ok {
		return nil
	}

	name, _ := involved["name"].(string)
	ns, _ := involved["namespace"].(string)
	kind, _ := involved["kind"].(string)
	apiVersion, _ := involved["apiVersion"].(string)

	if name == "" || kind == "" {
		return nil
	}

	involvedRef := ObjectRef{
		Group:     extractGroup(apiVersion),
		Kind:      kind,
		Namespace: ns,
		Name:      name,
	}

	if _, exists := lookup.Get(involvedRef); !exists {
		return nil
	}

	return []Edge{{
		From:     ref,
		To:       involvedRef,
		Type:     EdgeEvent,
		Resolver: "event",
		Field:    "involvedObject",
	}}
}

func (e *eventResolver) resolveReverseEvent(ref ObjectRef, obj *unstructured.Unstructured, lookup Lookup) []Edge {
	events := lookup.List("", "Event")
	var edges []Edge

	for _, evt := range events {
		involved, ok := evt.Object["involvedObject"].(map[string]interface{})
		if !ok {
			continue
		}

		name, _ := involved["name"].(string)
		ns, _ := involved["namespace"].(string)
		kind, _ := involved["kind"].(string)
		apiVersion, _ := involved["apiVersion"].(string)

		involvedRef := ObjectRef{
			Group:     extractGroup(apiVersion),
			Kind:      kind,
			Namespace: ns,
			Name:      name,
		}

		if involvedRef == ref {
			evtRef := RefFromUnstructured(evt)
			edges = append(edges, Edge{
				From:     evtRef,
				To:       ref,
				Type:     EdgeEvent,
				Resolver: "event",
				Field:    "involvedObject",
			})
		}
	}
	return edges
}
