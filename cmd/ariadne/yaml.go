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
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	yamlutil "k8s.io/apimachinery/pkg/util/yaml"
)

// decodeYAML reads a YAML (or JSON) stream from r, returning all successfully
// decoded Kubernetes objects and any non-EOF errors encountered along the way.
// Empty documents (those producing a nil Object map) are silently skipped.
func decodeYAML(r io.Reader) ([]unstructured.Unstructured, []error) {
	decoder := yamlutil.NewYAMLOrJSONDecoder(r, 4096)

	var objects []unstructured.Unstructured
	var errs []error

	for {
		var obj unstructured.Unstructured
		if err := decoder.Decode(&obj); err != nil {
			if err == io.EOF {
				break
			}
			errs = append(errs, err)
			continue
		}
		if obj.Object == nil {
			continue
		}
		objects = append(objects, obj)
	}

	return objects, errs
}

// readSources reads Kubernetes objects from the given filesystem paths.
// Each path may be a regular file or a directory; directories are walked
// recursively. Only files ending in .yaml or .yml are read.
func readSources(paths []string) ([]unstructured.Unstructured, []error) {
	var objects []unstructured.Unstructured
	var errs []error

	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", p, err))
			continue
		}
		var objs []unstructured.Unstructured
		var ee []error
		if info.IsDir() {
			objs, ee = readDir(p)
		} else {
			objs, ee = readFile(p)
		}
		objects = append(objects, objs...)
		errs = append(errs, ee...)
	}

	return objects, errs
}

// readFile reads a single file and decodes all Kubernetes objects from it.
func readFile(path string) ([]unstructured.Unstructured, []error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, []error{fmt.Errorf("%s: %w", path, err)}
	}
	defer f.Close()

	objs, errs := decodeYAML(f)
	for i, e := range errs {
		errs[i] = fmt.Errorf("%s: %w", path, e)
	}
	return objs, errs
}

// readDir recursively walks dir and reads all .yaml and .yml files.
func readDir(dir string) ([]unstructured.Unstructured, []error) {
	var objects []unstructured.Unstructured
	var errs []error

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", path, err))
			return nil
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".yaml" && ext != ".yml" {
			return nil
		}
		objs, ee := readFile(path)
		objects = append(objects, objs...)
		errs = append(errs, ee...)
		return nil
	})
	if err != nil {
		errs = append(errs, fmt.Errorf("%s: %w", dir, err))
	}

	return objects, errs
}
