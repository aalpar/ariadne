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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// workloadTemplatePaths maps workload GroupKinds to the field path
// containing their embedded PodTemplateSpec.
var workloadTemplatePaths = map[groupKind]string{
	{Group: "apps", Kind: "Deployment"}:  "spec.template",
	{Group: "apps", Kind: "StatefulSet"}: "spec.template",
	{Group: "apps", Kind: "DaemonSet"}:   "spec.template",
	{Group: "apps", Kind: "ReplicaSet"}:  "spec.template",
	{Group: "batch", Kind: "Job"}:        "spec.template",
	{Group: "batch", Kind: "CronJob"}:    "spec.jobTemplate.spec.template",
}

// ExtractPodTemplates extracts synthetic PodTemplate objects from workloads.
// For each Deployment, StatefulSet, DaemonSet, ReplicaSet, Job, or CronJob,
// it creates a core/v1 PodTemplate with the embedded pod template spec.
// The synthetic PodTemplate has the same name/namespace as the workload
// and an ownerReference pointing back to it.
func ExtractPodTemplates(objs []unstructured.Unstructured) []unstructured.Unstructured {
	var templates []unstructured.Unstructured
	for i := range objs {
		obj := &objs[i]
		gvk := obj.GroupVersionKind()
		path, ok := workloadTemplatePaths[groupKind{Group: gvk.Group, Kind: gvk.Kind}]
		if !ok {
			continue
		}
		if pt := buildPodTemplate(obj, path); pt != nil {
			templates = append(templates, *pt)
		}
	}
	return templates
}

func buildPodTemplate(obj *unstructured.Unstructured, templatePath string) *unstructured.Unstructured {
	raw := extractRawValues(obj.Object, templatePath)
	if len(raw) == 0 {
		return nil
	}
	tmplMap, ok := raw[0].(map[string]interface{})
	if !ok {
		return nil
	}

	pt := &unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "PodTemplate",
		"metadata": map[string]interface{}{
			"name":      obj.GetName(),
			"namespace": obj.GetNamespace(),
		},
		"template": tmplMap,
	}}
	pt.SetOwnerReferences([]metav1.OwnerReference{{
		APIVersion: obj.GetAPIVersion(),
		Kind:       obj.GetKind(),
		Name:       obj.GetName(),
	}})

	return pt
}

// podTemplateRules generates PodTemplate RefRules from Pod RefRules.
// For each Pod rule with FieldPath "spec.X", it creates a rule with
// FromKind "PodTemplate" and FieldPath "template.spec.X".
func podTemplateRules(podRules []RefRule) []RefRule {
	rules := make([]RefRule, len(podRules))
	for i, r := range podRules {
		rules[i] = RefRule{
			FromKind:      "PodTemplate",
			ToGroup:       r.ToGroup,
			ToKind:        r.ToKind,
			FieldPath:     "template." + r.FieldPath,
			ClusterScoped: r.ClusterScoped,
		}
		if r.NamespaceFieldPath != "" {
			rules[i].NamespaceFieldPath = "template." + r.NamespaceFieldPath
		}
	}
	return rules
}
