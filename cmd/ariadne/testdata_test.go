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
	"bytes"
	"strings"
	"testing"
)

// testdataCase defines expected minimums for a testdata YAML file.
type testdataCase struct {
	file string

	// Without PodTemplates.
	minNodes int
	minEdges int

	// With PodTemplates enabled.
	minNodesPT int
	minEdgesPT int

	// Edges that must appear in the DOT output (with PodTemplates).
	wantEdges []string
}

var testdataCases = []testdataCase{
	{
		file:       "../../testdata/online-boutique.yaml",
		minNodes:   35, minEdges: 0,
		minNodesPT: 47, minEdgesPT: 35,
		wantEdges: []string{
			// PodTemplate → Deployment (ownerRef)
			`"core/PodTemplate/frontend" -> "apps/Deployment/frontend"`,
			// PodTemplate → ServiceAccount (template.spec.serviceAccountName)
			`"core/PodTemplate/frontend" -> "core/ServiceAccount/frontend"`,
		},
	},
	{
		file:       "../../testdata/ingress-nginx.yaml",
		minNodes:   19, minEdges: 8,
		minNodesPT: 20, minEdgesPT: 14,
		wantEdges: []string{
			// RBAC chain: ClusterRoleBinding → ClusterRole
			`"rbac.authorization.k8s.io/ClusterRoleBinding/ingress-nginx" -> "rbac.authorization.k8s.io/ClusterRole/ingress-nginx"`,
			// RBAC chain: ClusterRoleBinding → ServiceAccount
			`"rbac.authorization.k8s.io/ClusterRoleBinding/ingress-nginx" -> "core/ServiceAccount/ingress-nginx/ingress-nginx"`,
			// RoleBinding → Role
			`"rbac.authorization.k8s.io/RoleBinding/ingress-nginx/ingress-nginx" -> "rbac.authorization.k8s.io/Role/ingress-nginx/ingress-nginx"`,
		},
	},
	{
		file:       "../../testdata/cert-manager.yaml",
		minNodes:   49, minEdges: 28,
		minNodesPT: 52, minEdgesPT: 37,
		wantEdges: []string{
			// One of the many ClusterRoleBinding → ServiceAccount edges
			`"rbac.authorization.k8s.io/ClusterRoleBinding/cert-manager-cainjector" -> "core/ServiceAccount/cert-manager/cert-manager-cainjector"`,
			// ClusterRoleBinding → ClusterRole
			`"rbac.authorization.k8s.io/ClusterRoleBinding/cert-manager-cainjector" -> "rbac.authorization.k8s.io/ClusterRole/cert-manager-cainjector"`,
		},
	},
	{
		file:       "../../testdata/kube-prometheus.yaml",
		minNodes:   95, minEdges: 16,
		minNodesPT: 101, minEdgesPT: 46,
		wantEdges: []string{
			// PodTemplate → ServiceAccount
			`"core/PodTemplate/monitoring/grafana" -> "core/ServiceAccount/monitoring/grafana"`,
			// PodTemplate → ConfigMap (volume)
			`"core/PodTemplate/monitoring/grafana" -> "core/ConfigMap/monitoring/grafana-dashboards"`,
			// PodTemplate → Secret (volume)
			`"core/PodTemplate/monitoring/grafana" -> "core/Secret/monitoring/grafana-config"`,
			// Service → PodTemplate (selector)
			`"core/Service/monitoring/grafana" -> "core/PodTemplate/monitoring/grafana"`,
			// NetworkPolicy → PodTemplate (podSelector)
			`"networking.k8s.io/NetworkPolicy/monitoring/grafana" -> "core/PodTemplate/monitoring/grafana"`,
			// RBAC
			`"rbac.authorization.k8s.io/ClusterRoleBinding/prometheus-k8s" -> "core/ServiceAccount/monitoring/prometheus-k8s"`,
		},
	},
}

func TestTestdata_Graph(t *testing.T) {
	for _, tc := range testdataCases {
		name := tc.file[strings.LastIndex(tc.file, "/")+1 : strings.LastIndex(tc.file, ".")]
		t.Run(name, func(t *testing.T) {
			objs, errs := readFile(tc.file)
			if len(objs) == 0 {
				t.Fatalf("no objects loaded (errors: %v)", errs)
			}

			// Without PodTemplates.
			var buf bytes.Buffer
			if err := graph(objs, "dot", false, &buf); err != nil {
				t.Fatal(err)
			}
			dot := buf.String()
			nodes := countNodes(dot)
			edges := countEdges(dot)
			if nodes < tc.minNodes {
				t.Errorf("nodes = %d, want >= %d", nodes, tc.minNodes)
			}
			if edges < tc.minEdges {
				t.Errorf("edges = %d, want >= %d", edges, tc.minEdges)
			}

			// With PodTemplates.
			buf.Reset()
			if err := graph(objs, "dot", true, &buf); err != nil {
				t.Fatal(err)
			}
			dotPT := buf.String()
			nodesPT := countNodes(dotPT)
			edgesPT := countEdges(dotPT)
			if nodesPT < tc.minNodesPT {
				t.Errorf("nodes (pod-templates) = %d, want >= %d", nodesPT, tc.minNodesPT)
			}
			if edgesPT < tc.minEdgesPT {
				t.Errorf("edges (pod-templates) = %d, want >= %d", edgesPT, tc.minEdgesPT)
			}

			// Check specific edges.
			for _, want := range tc.wantEdges {
				if !strings.Contains(dotPT, want) {
					t.Errorf("missing edge: %s", want)
				}
			}
		})
	}
}

func TestTestdata_Lint(t *testing.T) {
	// Lint should succeed (no panic, no errors) on all testdata files.
	// We don't assert specific finding counts since dangling refs are
	// expected in isolated manifests (e.g. references to cluster objects).
	files := []string{
		"../../testdata/online-boutique.yaml",
		"../../testdata/ingress-nginx.yaml",
		"../../testdata/cert-manager.yaml",
		"../../testdata/kube-prometheus.yaml",
	}
	for _, f := range files {
		name := f[strings.LastIndex(f, "/")+1 : strings.LastIndex(f, ".")]
		t.Run(name, func(t *testing.T) {
			objs, errs := readFile(f)
			if len(objs) == 0 {
				t.Fatalf("no objects loaded (errors: %v)", errs)
			}
			var buf bytes.Buffer
			count := lint(objs, &buf)
			t.Logf("%s: %d objects, %d findings", name, len(objs), count)
		})
	}
}

func countNodes(dot string) int {
	n := 0
	for _, line := range strings.Split(dot, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, `"`) && strings.HasSuffix(line, `";`) && !strings.Contains(line, "->") {
			n++
		}
	}
	return n
}

func countEdges(dot string) int {
	n := 0
	for _, line := range strings.Split(dot, "\n") {
		if strings.Contains(line, "->") {
			n++
		}
	}
	return n
}
