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

// Resolver discovers dependency edges involving a given object.
// Resolve returns edges in both directions: "obj depends on X"
// and "existing Y depends on obj".
type Resolver interface {
	Name() string
	Resolve(obj *unstructured.Unstructured, lookup Lookup) []Edge
}

// Lookup provides read-only access to objects in the graph.
// Resolvers use this to find potential dependency targets.
type Lookup interface {
	Get(ref ObjectRef) (*unstructured.Unstructured, bool)
	List(group, kind string) []*unstructured.Unstructured
	ListInNamespace(group, kind, namespace string) []*unstructured.Unstructured
}
