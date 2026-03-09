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
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestExportDOT(t *testing.T) {
	g := New(WithResolver(&stubResolver{}))
	g.Add(
		newCoreObj("ConfigMap", "default", "config"),
		newCoreObj("Pod", "default", "web"),
	)

	var buf bytes.Buffer
	if err := g.ExportDOT(&buf); err != nil {
		t.Fatal(err)
	}

	dot := buf.String()
	if !strings.Contains(dot, "digraph ariadne") {
		t.Fatal("missing digraph header")
	}
	if !strings.Contains(dot, "core/Pod/default/web") {
		t.Fatal("missing pod node")
	}
	if !strings.Contains(dot, "->") {
		t.Fatal("missing edge arrow")
	}
}

func TestExportJSON(t *testing.T) {
	g := New(WithResolver(&stubResolver{}))
	g.Add(
		newCoreObj("ConfigMap", "default", "config"),
		newCoreObj("Pod", "default", "web"),
	)

	var buf bytes.Buffer
	if err := g.ExportJSON(&buf); err != nil {
		t.Fatal(err)
	}

	var result struct {
		Nodes []ObjectRef `json:"nodes"`
		Edges []struct {
			From     ObjectRef `json:"from"`
			To       ObjectRef `json:"to"`
			Type     string    `json:"type"`
			Resolver string    `json:"resolver"`
			Field    string    `json:"field"`
		} `json:"edges"`
	}
	if err := json.Unmarshal(buf.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if len(result.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(result.Nodes))
	}
	if len(result.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(result.Edges))
	}
}
