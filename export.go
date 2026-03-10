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
	"encoding/json"
	"fmt"
	"io"
	"sort"
)

func sortedNodes(nodes map[ObjectRef]*node) []ObjectRef {
	sorted := make([]ObjectRef, 0, len(nodes))
	for ref := range nodes {
		sorted = append(sorted, ref)
	}
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].String() < sorted[j].String()
	})
	return sorted
}

func sortedEdges(outEdges map[ObjectRef][]Edge) []Edge {
	var sorted []Edge
	for _, ee := range outEdges {
		sorted = append(sorted, ee...)
	}
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].From.String() != sorted[j].From.String() {
			return sorted[i].From.String() < sorted[j].From.String()
		}
		return sorted[i].To.String() < sorted[j].To.String()
	})
	return sorted
}

// ExportDOT writes the graph in Graphviz DOT format.
func (g *Graph) ExportDOT(w io.Writer) error {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if _, err := fmt.Fprintln(w, "digraph ariadne {"); err != nil {
		return err
	}

	for _, ref := range sortedNodes(g.nodes) {
		if _, err := fmt.Fprintf(w, "    %q;\n", ref.String()); err != nil {
			return err
		}
	}

	for _, e := range sortedEdges(g.outEdges) {
		label := e.Resolver
		if e.Field != "" {
			label += ":" + e.Field
		}
		if _, err := fmt.Fprintf(w, "    %q -> %q [label=%q];\n",
			e.From.String(), e.To.String(), label); err != nil {
			return err
		}
	}

	_, err := fmt.Fprintln(w, "}")
	return err
}

type jsonGraph struct {
	Nodes []ObjectRef `json:"nodes"`
	Edges []jsonEdge  `json:"edges"`
}

type jsonEdge struct {
	From     ObjectRef `json:"from"`
	To       ObjectRef `json:"to"`
	Type     string    `json:"type"`
	Resolver string    `json:"resolver"`
	Field    string    `json:"field"`
}

// ExportJSON writes the graph as JSON.
func (g *Graph) ExportJSON(w io.Writer) error {
	g.mu.RLock()
	defer g.mu.RUnlock()

	nodes := sortedNodes(g.nodes)
	edges := sortedEdges(g.outEdges)

	jEdges := make([]jsonEdge, len(edges))
	for i, e := range edges {
		jEdges[i] = jsonEdge{
			From:     e.From,
			To:       e.To,
			Type:     e.Type.String(),
			Resolver: e.Resolver,
			Field:    e.Field,
		}
	}

	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(jsonGraph{Nodes: nodes, Edges: jEdges})
}
