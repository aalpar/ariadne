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

	"github.com/aalpar/ariadne"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// graph builds a dependency graph from the given objects and writes it
// to w in the specified format ("dot" or "json").
func graph(objs []unstructured.Unstructured, format string, w io.Writer) error {
	g := ariadne.NewDefault(
		ariadne.WithResolver(ariadne.NewArgoCDResolver()),
		ariadne.WithResolver(ariadne.NewKyvernoResolver()),
		ariadne.WithResolver(ariadne.NewCrossplaneResolver()),
		ariadne.WithResolver(ariadne.NewGatewayAPIResolver()),
		ariadne.WithResolver(ariadne.NewClusterAPIResolver()),
	)
	g.Load(objs)

	switch format {
	case "dot":
		return g.ExportDOT(w)
	case "json":
		return g.ExportJSON(w)
	default:
		return fmt.Errorf("unknown format: %q (valid: dot, json)", format)
	}
}
