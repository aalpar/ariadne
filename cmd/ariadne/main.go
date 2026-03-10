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
	"flag"
	"fmt"
	"os"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func main() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: ariadne <command> [args...]\n\nCommands:\n  graph   Output dependency graph (DOT or JSON)\n  lint    Check for dangling resource references\n")
	}
	flag.Parse()

	args := flag.Args()
	if len(args) == 0 {
		flag.Usage()
		os.Exit(2)
	}

	switch args[0] {
	case "graph":
		os.Exit(runGraph(args[1:]))
	case "lint":
		os.Exit(runLint(args[1:]))
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", args[0])
		flag.Usage()
		os.Exit(2)
	}
}

func runGraph(args []string) int {
	fs := flag.NewFlagSet("graph", flag.ExitOnError)
	format := fs.String("format", "dot", "Output format: dot, json")
	podTemplates := fs.Bool("pod-templates", false, "Extract PodTemplates from workloads")
	fs.Parse(args)

	var objs []unstructured.Unstructured
	var allErrs []error

	if fs.NArg() == 0 {
		o, errs := decodeYAML(os.Stdin)
		objs = append(objs, o...)
		allErrs = append(allErrs, errs...)
	} else {
		o, errs := readSources(fs.Args())
		objs = append(objs, o...)
		allErrs = append(allErrs, errs...)
	}

	for _, err := range allErrs {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
	}

	if len(objs) == 0 {
		fmt.Fprintln(os.Stderr, "no valid Kubernetes objects found")
		return 2
	}

	if err := graph(objs, *format, *podTemplates, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		return 2
	}
	return 0
}

func runLint(args []string) int {
	var objs []unstructured.Unstructured
	var allErrs []error

	if len(args) == 0 {
		// Read from stdin.
		o, errs := decodeYAML(os.Stdin)
		objs = append(objs, o...)
		allErrs = append(allErrs, errs...)
	} else {
		o, errs := readSources(args)
		objs = append(objs, o...)
		allErrs = append(allErrs, errs...)
	}

	for _, err := range allErrs {
		fmt.Fprintf(os.Stderr, "warning: %v\n", err)
	}

	if len(objs) == 0 {
		fmt.Fprintln(os.Stderr, "no valid Kubernetes objects found")
		return 2
	}

	count := lint(objs, os.Stdout)
	if count > 0 {
		return 1
	}
	return 0
}
