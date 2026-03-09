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

import "errors"

// ErrCycle is returned by TopologicalSort when the graph contains a cycle.
var ErrCycle = errors.New("graph contains a cycle")

// TopologicalSort returns nodes in dependency order (dependencies first).
// Returns ErrCycle if the graph contains a cycle.
func (g *Graph) TopologicalSort() ([]ObjectRef, error) {
	g.mu.RLock()
	defer g.mu.RUnlock()

	// Kahn's algorithm
	// inDegree here means "number of outgoing dependency edges" — nodes with
	// no dependencies come first.
	inDegree := make(map[ObjectRef]int, len(g.nodes))
	for ref := range g.nodes {
		inDegree[ref] = len(g.outEdges[ref])
	}

	var queue []ObjectRef
	for ref, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, ref)
		}
	}

	var sorted []ObjectRef
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		sorted = append(sorted, current)

		for _, e := range g.inEdges[current] {
			inDegree[e.From]--
			if inDegree[e.From] == 0 {
				queue = append(queue, e.From)
			}
		}
	}

	if len(sorted) != len(g.nodes) {
		return nil, ErrCycle
	}
	return sorted, nil
}

// Cycles finds all elementary cycles using DFS-based cycle detection.
func (g *Graph) Cycles() [][]ObjectRef {
	g.mu.RLock()
	defer g.mu.RUnlock()

	var cycles [][]ObjectRef
	visited := make(map[ObjectRef]bool)
	onStack := make(map[ObjectRef]bool)
	path := []ObjectRef{}

	var dfs func(ref ObjectRef)
	dfs = func(ref ObjectRef) {
		visited[ref] = true
		onStack[ref] = true
		path = append(path, ref)

		for _, e := range g.outEdges[ref] {
			if !visited[e.To] {
				dfs(e.To)
			} else if onStack[e.To] {
				start := -1
				for i, r := range path {
					if r == e.To {
						start = i
						break
					}
				}
				if start >= 0 {
					cycle := make([]ObjectRef, len(path[start:]))
					copy(cycle, path[start:])
					cycles = append(cycles, cycle)
				}
			}
		}

		path = path[:len(path)-1]
		onStack[ref] = false
	}

	for ref := range g.nodes {
		if !visited[ref] {
			dfs(ref)
		}
	}

	return cycles
}
