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
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestIntegration_FullStack(t *testing.T) {
	g := NewDefault()

	objs := []unstructured.Unstructured{
		// ServiceAccount
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "ServiceAccount",
			"metadata": map[string]interface{}{
				"name": "web-sa", "namespace": "default",
			},
		}},
		// ConfigMap
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "ConfigMap",
			"metadata": map[string]interface{}{
				"name": "web-config", "namespace": "default",
			},
		}},
		// Secret
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "Secret",
			"metadata": map[string]interface{}{
				"name": "web-secret", "namespace": "default",
			},
		}},
		// Pod (references SA, ConfigMap, Secret via volumes; owned by ReplicaSet)
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "Pod",
			"metadata": map[string]interface{}{
				"name": "web", "namespace": "default",
				"labels": map[string]interface{}{"app": "web"},
				"ownerReferences": []interface{}{
					map[string]interface{}{
						"apiVersion": "apps/v1",
						"kind":       "ReplicaSet",
						"name":       "web-rs",
						"uid":        "fake",
					},
				},
			},
			"spec": map[string]interface{}{
				"serviceAccountName": "web-sa",
				"volumes": []interface{}{
					map[string]interface{}{
						"configMap": map[string]interface{}{"name": "web-config"},
					},
					map[string]interface{}{
						"secret": map[string]interface{}{"secretName": "web-secret"},
					},
				},
			},
		}},
		// ReplicaSet (owner of Pod)
		{Object: map[string]interface{}{
			"apiVersion": "apps/v1", "kind": "ReplicaSet",
			"metadata": map[string]interface{}{
				"name": "web-rs", "namespace": "default",
			},
		}},
		// Service (selects Pod by label)
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "Service",
			"metadata": map[string]interface{}{
				"name": "web-svc", "namespace": "default",
			},
			"spec": map[string]interface{}{
				"selector": map[string]interface{}{"app": "web"},
			},
		}},
	}

	g.Load(objs)

	// Pod should depend on: SA, ConfigMap, Secret, ReplicaSet (owner)
	podRef := ObjectRef{Kind: "Pod", Namespace: "default", Name: "web"}
	podDeps := g.DependenciesOf(podRef)
	if len(podDeps) != 4 {
		t.Fatalf("expected 4 pod deps, got %d: %v", len(podDeps), podDeps)
	}

	// Service should depend on Pod (selector)
	svcRef := ObjectRef{Kind: "Service", Namespace: "default", Name: "web-svc"}
	svcDeps := g.DependenciesOf(svcRef)
	if len(svcDeps) != 1 {
		t.Fatalf("expected 1 svc dep, got %d: %v", len(svcDeps), svcDeps)
	}
	if svcDeps[0].To != podRef {
		t.Fatalf("expected svc -> pod, got %v", svcDeps[0].To)
	}

	// Topological sort should succeed (no cycles)
	sorted, err := g.TopologicalSort()
	if err != nil {
		t.Fatalf("unexpected cycle: %v", err)
	}
	if len(sorted) != 6 {
		t.Fatalf("expected 6 sorted nodes, got %d", len(sorted))
	}

	// Total edges: Pod->SA, Pod->CM, Pod->Secret, Pod->RS(owner), Svc->Pod = 5
	edges := g.Edges()
	if len(edges) != 5 {
		t.Fatalf("expected 5 edges, got %d", len(edges))
	}

	// Service's Upstream should include Pod and all Pod's deps (SA, CM, Secret, RS)
	upstream := g.Upstream(svcRef)
	if len(upstream) < 4 {
		t.Fatalf("expected at least 4 upstream of svc, got %d: %v", len(upstream), upstream)
	}
}

func TestIntegration_HPAScaleTargetRef(t *testing.T) {
	g := NewDefault()

	objs := []unstructured.Unstructured{
		// Deployment
		{Object: map[string]interface{}{
			"apiVersion": "apps/v1", "kind": "Deployment",
			"metadata": map[string]interface{}{
				"name": "web", "namespace": "default",
			},
		}},
		// HPA targeting the Deployment
		{Object: map[string]interface{}{
			"apiVersion": "autoscaling/v2", "kind": "HorizontalPodAutoscaler",
			"metadata": map[string]interface{}{
				"name": "web-hpa", "namespace": "default",
			},
			"spec": map[string]interface{}{
				"scaleTargetRef": map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"name":       "web",
				},
			},
		}},
	}

	g.Load(objs)

	hpaRef := ObjectRef{Group: "autoscaling", Kind: "HorizontalPodAutoscaler", Namespace: "default", Name: "web-hpa"}
	deps := g.DependenciesOf(hpaRef)
	if len(deps) != 1 {
		t.Fatalf("expected 1 HPA dep, got %d: %v", len(deps), deps)
	}
	expected := ObjectRef{Group: "apps", Kind: "Deployment", Namespace: "default", Name: "web"}
	if deps[0].To != expected {
		t.Fatalf("expected HPA -> Deployment, got %v", deps[0].To)
	}

	// Deployment should be a dependent of... nothing (it has no deps),
	// but HPA should appear as a dependent OF the Deployment.
	depRef := ObjectRef{Group: "apps", Kind: "Deployment", Namespace: "default", Name: "web"}
	dependents := g.DependentsOf(depRef)
	if len(dependents) != 1 {
		t.Fatalf("expected 1 dependent of Deployment, got %d: %v", len(dependents), dependents)
	}
	if dependents[0].From != hpaRef {
		t.Fatalf("expected HPA as dependent, got %v", dependents[0].From)
	}
}

func TestIntegration_HPAScaleTargetRef_ReverseAdd(t *testing.T) {
	g := NewDefault()

	// Add HPA first, then the Deployment — tests reverse resolution via Add.
	hpa := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "autoscaling/v2", "kind": "HorizontalPodAutoscaler",
		"metadata": map[string]interface{}{
			"name": "web-hpa", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"scaleTargetRef": map[string]interface{}{
				"apiVersion": "apps/v1",
				"kind":       "Deployment",
				"name":       "web",
			},
		},
	}}
	g.Add(hpa)

	// No edge yet — Deployment isn't in the graph.
	hpaRef := ObjectRef{Group: "autoscaling", Kind: "HorizontalPodAutoscaler", Namespace: "default", Name: "web-hpa"}
	if len(g.DependenciesOf(hpaRef)) != 0 {
		t.Fatal("expected 0 deps before Deployment is added")
	}

	deploy := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1", "kind": "Deployment",
		"metadata": map[string]interface{}{
			"name": "web", "namespace": "default",
		},
	}}
	g.Add(deploy)

	// Now the reverse path should have created the edge.
	deps := g.DependenciesOf(hpaRef)
	if len(deps) != 1 {
		t.Fatalf("expected 1 HPA dep after reverse add, got %d: %v", len(deps), deps)
	}
}

func TestIntegration_RoleBindingTypedRef(t *testing.T) {
	g := NewDefault()

	objs := []unstructured.Unstructured{
		// Role
		{Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "Role",
			"metadata": map[string]interface{}{
				"name": "pod-reader", "namespace": "default",
			},
		}},
		// ServiceAccount
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "ServiceAccount",
			"metadata": map[string]interface{}{
				"name": "reader-sa", "namespace": "default",
			},
		}},
		// RoleBinding -> Role + ServiceAccount
		{Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "RoleBinding",
			"metadata": map[string]interface{}{
				"name": "read-pods", "namespace": "default",
			},
			"roleRef": map[string]interface{}{
				"apiGroup": "rbac.authorization.k8s.io",
				"kind":     "Role",
				"name":     "pod-reader",
			},
			"subjects": []interface{}{
				map[string]interface{}{
					"kind":      "ServiceAccount",
					"name":      "reader-sa",
					"namespace": "default",
				},
			},
		}},
	}

	g.Load(objs)

	rbRef := ObjectRef{Group: "rbac.authorization.k8s.io", Kind: "RoleBinding", Namespace: "default", Name: "read-pods"}
	deps := g.DependenciesOf(rbRef)
	if len(deps) != 2 {
		t.Fatalf("expected 2 RoleBinding deps (Role + SA), got %d: %v", len(deps), deps)
	}

	roleRef := ObjectRef{Group: "rbac.authorization.k8s.io", Kind: "Role", Namespace: "default", Name: "pod-reader"}
	saRef := ObjectRef{Kind: "ServiceAccount", Namespace: "default", Name: "reader-sa"}

	found := map[ObjectRef]bool{}
	for _, e := range deps {
		found[e.To] = true
	}
	if !found[roleRef] {
		t.Fatal("expected RoleBinding -> Role edge")
	}
	if !found[saRef] {
		t.Fatal("expected RoleBinding -> ServiceAccount edge")
	}
}

func TestIntegration_ClusterRoleBindingTypedRef(t *testing.T) {
	g := NewDefault()

	objs := []unstructured.Unstructured{
		// ClusterRole (cluster-scoped)
		{Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "ClusterRole",
			"metadata": map[string]interface{}{
				"name": "admin",
			},
		}},
		// ServiceAccount in "kube-system"
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "ServiceAccount",
			"metadata": map[string]interface{}{
				"name": "admin-sa", "namespace": "kube-system",
			},
		}},
		// ClusterRoleBinding (cluster-scoped) -> ClusterRole + SA in kube-system
		{Object: map[string]interface{}{
			"apiVersion": "rbac.authorization.k8s.io/v1", "kind": "ClusterRoleBinding",
			"metadata": map[string]interface{}{
				"name": "admin-binding",
			},
			"roleRef": map[string]interface{}{
				"apiGroup": "rbac.authorization.k8s.io",
				"kind":     "ClusterRole",
				"name":     "admin",
			},
			"subjects": []interface{}{
				map[string]interface{}{
					"kind":      "ServiceAccount",
					"name":      "admin-sa",
					"namespace": "kube-system",
				},
			},
		}},
	}

	g.Load(objs)

	crbRef := ObjectRef{Group: "rbac.authorization.k8s.io", Kind: "ClusterRoleBinding", Name: "admin-binding"}
	deps := g.DependenciesOf(crbRef)
	if len(deps) != 2 {
		t.Fatalf("expected 2 ClusterRoleBinding deps, got %d: %v", len(deps), deps)
	}

	crRef := ObjectRef{Group: "rbac.authorization.k8s.io", Kind: "ClusterRole", Name: "admin"}
	saRef := ObjectRef{Kind: "ServiceAccount", Namespace: "kube-system", Name: "admin-sa"}

	found := map[ObjectRef]bool{}
	for _, e := range deps {
		found[e.To] = true
	}
	if !found[crRef] {
		t.Fatal("expected ClusterRoleBinding -> ClusterRole edge")
	}
	if !found[saRef] {
		t.Fatal("expected ClusterRoleBinding -> ServiceAccount edge")
	}
}

func TestIntegration_PVClaimRef(t *testing.T) {
	g := NewDefault()

	objs := []unstructured.Unstructured{
		// PVC in "production" namespace
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "PersistentVolumeClaim",
			"metadata": map[string]interface{}{
				"name": "data-pvc", "namespace": "production",
			},
		}},
		// PV (cluster-scoped) with claimRef pointing to the PVC
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "PersistentVolume",
			"metadata": map[string]interface{}{
				"name": "data-pv",
			},
			"spec": map[string]interface{}{
				"claimRef": map[string]interface{}{
					"apiVersion": "v1",
					"kind":       "PersistentVolumeClaim",
					"name":       "data-pvc",
					"namespace":  "production",
				},
			},
		}},
	}

	g.Load(objs)

	pvRef := ObjectRef{Kind: "PersistentVolume", Name: "data-pv"}
	deps := g.DependenciesOf(pvRef)
	if len(deps) != 1 {
		t.Fatalf("expected 1 PV dep, got %d: %v", len(deps), deps)
	}
	pvcRef := ObjectRef{Kind: "PersistentVolumeClaim", Namespace: "production", Name: "data-pvc"}
	if deps[0].To != pvcRef {
		t.Fatalf("expected PV -> PVC, got %v", deps[0].To)
	}

	// PVC should also have PV as dependent
	dependents := g.DependentsOf(pvcRef)
	if len(dependents) != 1 {
		t.Fatalf("expected 1 dependent of PVC, got %d: %v", len(dependents), dependents)
	}
}

func TestIntegration_WebhookService(t *testing.T) {
	g := NewDefault()

	objs := []unstructured.Unstructured{
		// Service in "webhook-system" namespace
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "Service",
			"metadata": map[string]interface{}{
				"name": "webhook-svc", "namespace": "webhook-system",
			},
		}},
		// ValidatingWebhookConfiguration (cluster-scoped) -> Service
		{Object: map[string]interface{}{
			"apiVersion": "admissionregistration.k8s.io/v1", "kind": "ValidatingWebhookConfiguration",
			"metadata": map[string]interface{}{
				"name": "validate-pods",
			},
			"webhooks": []interface{}{
				map[string]interface{}{
					"name": "validate-pods.example.com",
					"clientConfig": map[string]interface{}{
						"service": map[string]interface{}{
							"name":      "webhook-svc",
							"namespace": "webhook-system",
							"port":      int64(443),
						},
					},
				},
			},
		}},
	}

	g.Load(objs)

	vwcRef := ObjectRef{Group: "admissionregistration.k8s.io", Kind: "ValidatingWebhookConfiguration", Name: "validate-pods"}
	deps := g.DependenciesOf(vwcRef)
	if len(deps) != 1 {
		t.Fatalf("expected 1 ValidatingWebhookConfiguration dep, got %d: %v", len(deps), deps)
	}
	svcRef := ObjectRef{Kind: "Service", Namespace: "webhook-system", Name: "webhook-svc"}
	if deps[0].To != svcRef {
		t.Fatalf("expected VWC -> Service, got %v", deps[0].To)
	}

	// Service should have the webhook config as a dependent.
	dependents := g.DependentsOf(svcRef)
	if len(dependents) != 1 {
		t.Fatalf("expected 1 dependent of Service, got %d: %v", len(dependents), dependents)
	}
}

func TestIntegration_MutatingWebhookService_ReverseAdd(t *testing.T) {
	g := NewDefault()

	// Add MutatingWebhookConfiguration first, then the Service.
	mwc := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "admissionregistration.k8s.io/v1", "kind": "MutatingWebhookConfiguration",
		"metadata": map[string]interface{}{
			"name": "inject-sidecar",
		},
		"webhooks": []interface{}{
			map[string]interface{}{
				"name": "inject.example.com",
				"clientConfig": map[string]interface{}{
					"service": map[string]interface{}{
						"name":      "injector-svc",
						"namespace": "istio-system",
					},
				},
			},
		},
	}}
	g.Add(mwc)

	mwcRef := ObjectRef{Group: "admissionregistration.k8s.io", Kind: "MutatingWebhookConfiguration", Name: "inject-sidecar"}
	if len(g.DependenciesOf(mwcRef)) != 0 {
		t.Fatal("expected 0 deps before Service is added")
	}

	svc := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{
			"name": "injector-svc", "namespace": "istio-system",
		},
	}}
	g.Add(svc)

	deps := g.DependenciesOf(mwcRef)
	if len(deps) != 1 {
		t.Fatalf("expected 1 MWC dep after reverse add, got %d: %v", len(deps), deps)
	}
	svcRef := ObjectRef{Kind: "Service", Namespace: "istio-system", Name: "injector-svc"}
	if deps[0].To != svcRef {
		t.Fatalf("expected MWC -> Service, got %v", deps[0].To)
	}
}

func TestIntegration_APIServiceService(t *testing.T) {
	g := NewDefault()

	objs := []unstructured.Unstructured{
		// Service in "monitoring" namespace
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "Service",
			"metadata": map[string]interface{}{
				"name": "prometheus-adapter", "namespace": "monitoring",
			},
		}},
		// APIService (cluster-scoped) -> Service
		{Object: map[string]interface{}{
			"apiVersion": "apiregistration.k8s.io/v1", "kind": "APIService",
			"metadata": map[string]interface{}{
				"name": "v1beta1.metrics.k8s.io",
			},
			"spec": map[string]interface{}{
				"service": map[string]interface{}{
					"name":      "prometheus-adapter",
					"namespace": "monitoring",
				},
			},
		}},
	}
	g.Load(objs)

	apiSvcRef := ObjectRef{Group: "apiregistration.k8s.io", Kind: "APIService", Name: "v1beta1.metrics.k8s.io"}
	deps := g.DependenciesOf(apiSvcRef)
	if len(deps) != 1 {
		t.Fatalf("expected 1 APIService dep, got %d: %v", len(deps), deps)
	}
	svcRef := ObjectRef{Kind: "Service", Namespace: "monitoring", Name: "prometheus-adapter"}
	if deps[0].To != svcRef {
		t.Fatalf("expected APIService -> Service, got %v", deps[0].To)
	}

	// Service should have the APIService as a dependent.
	dependents := g.DependentsOf(svcRef)
	if len(dependents) != 1 {
		t.Fatalf("expected 1 dependent of Service, got %d: %v", len(dependents), dependents)
	}
}

func TestIntegration_ServiceAccountSecrets(t *testing.T) {
	g := NewDefault()

	objs := []unstructured.Unstructured{
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "Secret",
			"metadata": map[string]interface{}{
				"name": "sa-token-abc", "namespace": "default",
			},
		}},
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "Secret",
			"metadata": map[string]interface{}{
				"name": "docker-reg", "namespace": "default",
			},
		}},
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "ServiceAccount",
			"metadata": map[string]interface{}{
				"name": "deploy-sa", "namespace": "default",
			},
			"secrets": []interface{}{
				map[string]interface{}{"name": "sa-token-abc"},
			},
			"imagePullSecrets": []interface{}{
				map[string]interface{}{"name": "docker-reg"},
			},
		}},
	}

	g.Load(objs)

	saRef := ObjectRef{Kind: "ServiceAccount", Namespace: "default", Name: "deploy-sa"}
	deps := g.DependenciesOf(saRef)
	if len(deps) != 2 {
		t.Fatalf("expected 2 SA deps, got %d: %v", len(deps), deps)
	}

	found := map[string]bool{}
	for _, e := range deps {
		found[e.To.Name] = true
	}
	if !found["sa-token-abc"] {
		t.Fatal("expected SA -> Secret (sa-token-abc) edge")
	}
	if !found["docker-reg"] {
		t.Fatal("expected SA -> Secret (docker-reg) edge")
	}
}

func TestIntegration_VolumeAttachment(t *testing.T) {
	g := NewDefault()

	objs := []unstructured.Unstructured{
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "PersistentVolume",
			"metadata": map[string]interface{}{"name": "pv-0"},
		}},
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "Node",
			"metadata": map[string]interface{}{"name": "node-1"},
		}},
		{Object: map[string]interface{}{
			"apiVersion": "storage.k8s.io/v1", "kind": "VolumeAttachment",
			"metadata": map[string]interface{}{"name": "csi-attach-xyz"},
			"spec": map[string]interface{}{
				"nodeName": "node-1",
				"source": map[string]interface{}{
					"persistentVolumeName": "pv-0",
				},
			},
		}},
	}

	g.Load(objs)

	vaRef := ObjectRef{Group: "storage.k8s.io", Kind: "VolumeAttachment", Name: "csi-attach-xyz"}
	deps := g.DependenciesOf(vaRef)
	if len(deps) != 2 {
		t.Fatalf("expected 2 VolumeAttachment deps (PV + Node), got %d: %v", len(deps), deps)
	}

	found := map[string]bool{}
	for _, e := range deps {
		found[e.To.Kind] = true
	}
	if !found["PersistentVolume"] {
		t.Fatal("expected VolumeAttachment -> PV edge")
	}
	if !found["Node"] {
		t.Fatal("expected VolumeAttachment -> Node edge")
	}
}

func TestIntegration_CSIDriverRefs(t *testing.T) {
	g := NewDefault()

	objs := []unstructured.Unstructured{
		{Object: map[string]interface{}{
			"apiVersion": "storage.k8s.io/v1", "kind": "CSIDriver",
			"metadata": map[string]interface{}{"name": "ebs.csi.aws.com"},
		}},
		{Object: map[string]interface{}{
			"apiVersion": "v1", "kind": "PersistentVolume",
			"metadata": map[string]interface{}{"name": "pv-ebs"},
			"spec": map[string]interface{}{
				"csi": map[string]interface{}{
					"driver": "ebs.csi.aws.com",
				},
			},
		}},
		{Object: map[string]interface{}{
			"apiVersion": "storage.k8s.io/v1", "kind": "StorageClass",
			"metadata":    map[string]interface{}{"name": "gp3"},
			"provisioner": "ebs.csi.aws.com",
		}},
	}

	g.Load(objs)

	// PV -> CSIDriver
	pvRef := ObjectRef{Kind: "PersistentVolume", Name: "pv-ebs"}
	pvDeps := g.DependenciesOf(pvRef)
	found := false
	for _, e := range pvDeps {
		if e.To.Group == "storage.k8s.io" && e.To.Kind == "CSIDriver" && e.To.Name == "ebs.csi.aws.com" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected PV -> CSIDriver edge, got deps: %v", pvDeps)
	}

	// StorageClass -> CSIDriver
	scRef := ObjectRef{Group: "storage.k8s.io", Kind: "StorageClass", Name: "gp3"}
	scDeps := g.DependenciesOf(scRef)
	found = false
	for _, e := range scDeps {
		if e.To.Group == "storage.k8s.io" && e.To.Kind == "CSIDriver" && e.To.Name == "ebs.csi.aws.com" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected StorageClass -> CSIDriver edge, got deps: %v", scDeps)
	}

	// CSIDriver should have both as dependents.
	csiRef := ObjectRef{Group: "storage.k8s.io", Kind: "CSIDriver", Name: "ebs.csi.aws.com"}
	dependents := g.DependentsOf(csiRef)
	if len(dependents) != 2 {
		t.Fatalf("expected 2 dependents of CSIDriver, got %d: %v", len(dependents), dependents)
	}
}

func TestIntegration_VolumeAttachment_ReverseAdd(t *testing.T) {
	g := NewDefault()

	// Add VolumeAttachment first, then its targets.
	va := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "storage.k8s.io/v1", "kind": "VolumeAttachment",
		"metadata": map[string]interface{}{"name": "csi-attach-rev"},
		"spec": map[string]interface{}{
			"nodeName": "node-2",
			"source": map[string]interface{}{
				"persistentVolumeName": "pv-rev",
			},
		},
	}}
	g.Add(va)

	vaRef := ObjectRef{Group: "storage.k8s.io", Kind: "VolumeAttachment", Name: "csi-attach-rev"}
	if len(g.DependenciesOf(vaRef)) != 0 {
		t.Fatal("expected 0 deps before targets are added")
	}

	g.Add(unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "PersistentVolume",
		"metadata": map[string]interface{}{"name": "pv-rev"},
	}})
	g.Add(unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Node",
		"metadata": map[string]interface{}{"name": "node-2"},
	}})

	deps := g.DependenciesOf(vaRef)
	if len(deps) != 2 {
		t.Fatalf("expected 2 VolumeAttachment deps after reverse add, got %d: %v", len(deps), deps)
	}
}

func TestIntegration_APIServiceService_ReverseAdd(t *testing.T) {
	g := NewDefault()

	// Add APIService first, then the Service.
	apiSvc := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apiregistration.k8s.io/v1", "kind": "APIService",
		"metadata": map[string]interface{}{
			"name": "v1beta1.custom.metrics.k8s.io",
		},
		"spec": map[string]interface{}{
			"service": map[string]interface{}{
				"name":      "custom-metrics-server",
				"namespace": "kube-system",
			},
		},
	}}
	g.Add(apiSvc)

	apiSvcRef := ObjectRef{Group: "apiregistration.k8s.io", Kind: "APIService", Name: "v1beta1.custom.metrics.k8s.io"}
	if len(g.DependenciesOf(apiSvcRef)) != 0 {
		t.Fatal("expected 0 deps before Service is added")
	}

	svc := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1", "kind": "Service",
		"metadata": map[string]interface{}{
			"name": "custom-metrics-server", "namespace": "kube-system",
		},
	}}
	g.Add(svc)

	deps := g.DependenciesOf(apiSvcRef)
	if len(deps) != 1 {
		t.Fatalf("expected 1 APIService dep after reverse add, got %d: %v", len(deps), deps)
	}
	svcRef := ObjectRef{Kind: "Service", Namespace: "kube-system", Name: "custom-metrics-server"}
	if deps[0].To != svcRef {
		t.Fatalf("expected APIService -> Service, got %v", deps[0].To)
	}
}
