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
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// ObjectRef uniquely identifies a K8s resource.
// Uses GroupKind (not GVK) because different API versions
// of the same kind refer to the same logical object.
type ObjectRef struct {
	Group     string
	Kind      string
	Namespace string
	Name      string
}

func (r ObjectRef) String() string {
	g := r.Group
	if g == "" {
		g = "core"
	}
	if r.Namespace == "" {
		return fmt.Sprintf("%s/%s/%s", g, r.Kind, r.Name)
	}
	return fmt.Sprintf("%s/%s/%s/%s", g, r.Kind, r.Namespace, r.Name)
}

// RefFromUnstructured extracts an ObjectRef from an unstructured K8s object.
func RefFromUnstructured(obj *unstructured.Unstructured) ObjectRef {
	gvk := obj.GroupVersionKind()
	return ObjectRef{
		Group:     gvk.Group,
		Kind:      gvk.Kind,
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}
}

// EdgeType classifies how a dependency was discovered.
type EdgeType int

const (
	EdgeNameRef       EdgeType = iota // direct namespace+name reference
	EdgeLocalNameRef                  // name reference within same namespace
	EdgeLabelSelector                 // label/selector match
	EdgeEvent                         // inferred from K8s Event
	EdgeCustom                        // user-defined
)

func (t EdgeType) String() string {
	switch t {
	case EdgeNameRef:
		return "name_ref"
	case EdgeLocalNameRef:
		return "local_name_ref"
	case EdgeLabelSelector:
		return "label_selector"
	case EdgeEvent:
		return "event"
	case EdgeCustom:
		return "custom"
	default:
		return fmt.Sprintf("unknown(%d)", int(t))
	}
}

// Edge is a directed dependency between two resources.
type Edge struct {
	From     ObjectRef
	To       ObjectRef
	Type     EdgeType
	Resolver string
	Field    string
}

// EventType classifies a graph change event.
type EventType int

const (
	NodeAdded EventType = iota
	NodeRemoved
	EdgeAdded
	EdgeRemoved
)

// GraphEvent represents a change to the graph.
type GraphEvent struct {
	Type EventType
	Ref  *ObjectRef
	Edge *Edge
}

// ChangeListener receives graph change notifications.
type ChangeListener func(event GraphEvent)
