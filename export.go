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

// ExportDOT writes the graph in Graphviz DOT format.
func (g *Graph) ExportDOT(w io.Writer) error {
	g.mu.RLock()
	defer g.mu.RUnlock()

	if _, err := fmt.Fprintln(w, "digraph ariadne {"); err != nil {
		return err
	}

	// Sort nodes for deterministic output
	nodes := make([]ObjectRef, 0, len(g.nodes))
	for ref := range g.nodes {
		nodes = append(nodes, ref)
	}
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].String() < nodes[j].String()
	})

	for _, ref := range nodes {
		if _, err := fmt.Fprintf(w, "    %q;\n", ref.String()); err != nil {
			return err
		}
	}

	// Sort edges for deterministic output
	var edges []Edge
	for _, ee := range g.outEdges {
		edges = append(edges, ee...)
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From.String() != edges[j].From.String() {
			return edges[i].From.String() < edges[j].From.String()
		}
		return edges[i].To.String() < edges[j].To.String()
	})

	for _, e := range edges {
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

	nodes := make([]ObjectRef, 0, len(g.nodes))
	for ref := range g.nodes {
		nodes = append(nodes, ref)
	}
	sort.Slice(nodes, func(i, j int) bool {
		return nodes[i].String() < nodes[j].String()
	})

	var edges []Edge
	for _, ee := range g.outEdges {
		edges = append(edges, ee...)
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].From.String() != edges[j].From.String() {
			return edges[i].From.String() < edges[j].From.String()
		}
		return edges[i].To.String() < edges[j].To.String()
	})

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
