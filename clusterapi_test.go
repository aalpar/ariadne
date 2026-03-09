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

func TestClusterAPI_MachineInfraRef(t *testing.T) {
	g := New(WithResolver(NewClusterAPIResolver()))

	objs := []unstructured.Unstructured{
		// DockerMachine (infrastructure provider)
		{Object: map[string]interface{}{
			"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1", "kind": "DockerMachine",
			"metadata": map[string]interface{}{
				"name": "worker-0-docker", "namespace": "default",
			},
		}},
		// KubeadmConfig (bootstrap provider)
		{Object: map[string]interface{}{
			"apiVersion": "bootstrap.cluster.x-k8s.io/v1beta1", "kind": "KubeadmConfig",
			"metadata": map[string]interface{}{
				"name": "worker-0-config", "namespace": "default",
			},
		}},
		// Machine referencing both
		{Object: map[string]interface{}{
			"apiVersion": "cluster.x-k8s.io/v1beta1", "kind": "Machine",
			"metadata": map[string]interface{}{
				"name": "worker-0", "namespace": "default",
			},
			"spec": map[string]interface{}{
				"infrastructureRef": map[string]interface{}{
					"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
					"kind":       "DockerMachine",
					"name":       "worker-0-docker",
				},
				"bootstrap": map[string]interface{}{
					"configRef": map[string]interface{}{
						"apiVersion": "bootstrap.cluster.x-k8s.io/v1beta1",
						"kind":       "KubeadmConfig",
						"name":       "worker-0-config",
					},
				},
			},
		}},
	}

	g.Load(objs)

	machineRef := ObjectRef{Group: "cluster.x-k8s.io", Kind: "Machine", Namespace: "default", Name: "worker-0"}
	deps := g.DependenciesOf(machineRef)
	if len(deps) != 2 {
		t.Fatalf("expected 2 Machine deps, got %d: %v", len(deps), deps)
	}

	dockerRef := ObjectRef{Group: "infrastructure.cluster.x-k8s.io", Kind: "DockerMachine", Namespace: "default", Name: "worker-0-docker"}
	configRef := ObjectRef{Group: "bootstrap.cluster.x-k8s.io", Kind: "KubeadmConfig", Namespace: "default", Name: "worker-0-config"}

	found := map[ObjectRef]bool{}
	for _, e := range deps {
		found[e.To] = true
	}
	if !found[dockerRef] {
		t.Fatal("expected Machine -> DockerMachine edge")
	}
	if !found[configRef] {
		t.Fatal("expected Machine -> KubeadmConfig edge")
	}
}

func TestClusterAPI_ClusterRefs(t *testing.T) {
	g := New(WithResolver(NewClusterAPIResolver()))

	objs := []unstructured.Unstructured{
		// DockerCluster (infrastructure provider)
		{Object: map[string]interface{}{
			"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1", "kind": "DockerCluster",
			"metadata": map[string]interface{}{
				"name": "my-cluster-docker", "namespace": "default",
			},
		}},
		// KubeadmControlPlane (control plane provider)
		{Object: map[string]interface{}{
			"apiVersion": "controlplane.cluster.x-k8s.io/v1beta1", "kind": "KubeadmControlPlane",
			"metadata": map[string]interface{}{
				"name": "my-cluster-cp", "namespace": "default",
			},
		}},
		// Cluster referencing both
		{Object: map[string]interface{}{
			"apiVersion": "cluster.x-k8s.io/v1beta1", "kind": "Cluster",
			"metadata": map[string]interface{}{
				"name": "my-cluster", "namespace": "default",
			},
			"spec": map[string]interface{}{
				"infrastructureRef": map[string]interface{}{
					"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
					"kind":       "DockerCluster",
					"name":       "my-cluster-docker",
				},
				"controlPlaneRef": map[string]interface{}{
					"apiVersion": "controlplane.cluster.x-k8s.io/v1beta1",
					"kind":       "KubeadmControlPlane",
					"name":       "my-cluster-cp",
				},
			},
		}},
	}

	g.Load(objs)

	clusterRef := ObjectRef{Group: "cluster.x-k8s.io", Kind: "Cluster", Namespace: "default", Name: "my-cluster"}
	deps := g.DependenciesOf(clusterRef)
	if len(deps) != 2 {
		t.Fatalf("expected 2 Cluster deps, got %d: %v", len(deps), deps)
	}

	dockerClusterRef := ObjectRef{Group: "infrastructure.cluster.x-k8s.io", Kind: "DockerCluster", Namespace: "default", Name: "my-cluster-docker"}
	cpRef := ObjectRef{Group: "controlplane.cluster.x-k8s.io", Kind: "KubeadmControlPlane", Namespace: "default", Name: "my-cluster-cp"}

	found := map[ObjectRef]bool{}
	for _, e := range deps {
		found[e.To] = true
	}
	if !found[dockerClusterRef] {
		t.Fatal("expected Cluster -> DockerCluster edge")
	}
	if !found[cpRef] {
		t.Fatal("expected Cluster -> KubeadmControlPlane edge")
	}
}

func TestClusterAPI_ReverseAdd(t *testing.T) {
	g := New(WithResolver(NewClusterAPIResolver()))

	// Add DockerMachine first — no Machine exists yet to reference it.
	dockerMachine := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1", "kind": "DockerMachine",
		"metadata": map[string]interface{}{
			"name": "worker-0-docker", "namespace": "default",
		},
	}}
	g.Add(dockerMachine)

	dockerRef := ObjectRef{Group: "infrastructure.cluster.x-k8s.io", Kind: "DockerMachine", Namespace: "default", Name: "worker-0-docker"}
	if len(g.DependentsOf(dockerRef)) != 0 {
		t.Fatal("expected 0 dependents before Machine is added")
	}

	// Now add the Machine that references the DockerMachine.
	machine := unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "cluster.x-k8s.io/v1beta1", "kind": "Machine",
		"metadata": map[string]interface{}{
			"name": "worker-0", "namespace": "default",
		},
		"spec": map[string]interface{}{
			"infrastructureRef": map[string]interface{}{
				"apiVersion": "infrastructure.cluster.x-k8s.io/v1beta1",
				"kind":       "DockerMachine",
				"name":       "worker-0-docker",
			},
		},
	}}
	g.Add(machine)

	// Forward: Machine should depend on DockerMachine.
	machineRef := ObjectRef{Group: "cluster.x-k8s.io", Kind: "Machine", Namespace: "default", Name: "worker-0"}
	deps := g.DependenciesOf(machineRef)
	if len(deps) != 1 {
		t.Fatalf("expected 1 Machine dep, got %d: %v", len(deps), deps)
	}
	if deps[0].To != dockerRef {
		t.Fatalf("expected Machine -> DockerMachine, got %v", deps[0].To)
	}

	// Reverse: DockerMachine should have Machine as dependent.
	dependents := g.DependentsOf(dockerRef)
	if len(dependents) != 1 {
		t.Fatalf("expected 1 dependent of DockerMachine, got %d: %v", len(dependents), dependents)
	}
	if dependents[0].From != machineRef {
		t.Fatalf("expected Machine as dependent, got %v", dependents[0].From)
	}
}
