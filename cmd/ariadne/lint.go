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

package main

import (
	"fmt"
	"io"
	"sort"

	"github.com/aalpar/ariadne"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

type finding struct {
	from  ariadne.ObjectRef
	to    ariadne.ObjectRef
	field string
}

// lint resolves all potential edges in the object set, filters out runtime
// edges (ownerRefs, events), and reports references to targets not present
// in the input. Returns the number of findings printed to w.
func lint(objs []unstructured.Unstructured, w io.Writer) int {
	edges := ariadne.ResolveAll(objs,
		ariadne.NewStructuralResolver(),
		ariadne.NewSelectorResolver(),
		ariadne.NewArgoCDResolver(),
		ariadne.NewKyvernoResolver(),
		ariadne.NewCrossplaneResolver(),
		ariadne.NewGatewayAPIResolver(),
		ariadne.NewClusterAPIResolver(),
	)

	nodes := make(map[ariadne.ObjectRef]struct{}, len(objs))
	for i := range objs {
		nodes[ariadne.RefFromUnstructured(&objs[i])] = struct{}{}
	}

	var findings []finding
	for _, e := range edges {
		if e.Field == "metadata.ownerReferences" {
			continue
		}
		if e.Resolver == "event" {
			continue
		}
		if _, exists := nodes[e.To]; !exists {
			findings = append(findings, finding{
				from:  e.From,
				to:    e.To,
				field: e.Field,
			})
		}
	}

	sort.Slice(findings, func(i, j int) bool {
		if fi, fj := findings[i].from.String(), findings[j].from.String(); fi != fj {
			return fi < fj
		}
		return findings[i].to.String() < findings[j].to.String()
	})

	for _, f := range findings {
		fmt.Fprintf(w, "%s -> %s (%s): not found\n", f.from, f.to, f.field)
	}

	return len(findings)
}
