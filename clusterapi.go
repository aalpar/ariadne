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

// NewClusterAPIResolver returns a resolver for Cluster API resource references.
// Handles infrastructureRef, bootstrap.configRef, and controlPlaneRef across
// Machine, Cluster, and MachineDeployment resources.
//
// All rules use unconstrained typed-refs (ToGroup/ToKind empty) because the
// target kind varies by infrastructure provider (e.g., DockerMachine, AWSCluster).
//
// Not registered by NewDefault() — opt in with WithResolver(NewClusterAPIResolver()).
func NewClusterAPIResolver() Resolver {
	return NewRuleResolver("cluster-api",
		// Machine -> infrastructure provider (e.g., DockerMachine)
		RefRule{
			FromGroup: "cluster.x-k8s.io", FromKind: "Machine",
			FieldPath: "spec.infrastructureRef",
		},
		// Machine -> bootstrap config (e.g., KubeadmConfig)
		RefRule{
			FromGroup: "cluster.x-k8s.io", FromKind: "Machine",
			FieldPath: "spec.bootstrap.configRef",
		},
		// Cluster -> infrastructure provider (e.g., DockerCluster)
		RefRule{
			FromGroup: "cluster.x-k8s.io", FromKind: "Cluster",
			FieldPath: "spec.infrastructureRef",
		},
		// Cluster -> control plane (e.g., KubeadmControlPlane)
		RefRule{
			FromGroup: "cluster.x-k8s.io", FromKind: "Cluster",
			FieldPath: "spec.controlPlaneRef",
		},
		// MachineDeployment -> infrastructure provider (template)
		RefRule{
			FromGroup: "cluster.x-k8s.io", FromKind: "MachineDeployment",
			FieldPath: "spec.template.spec.infrastructureRef",
		},
		// MachineDeployment -> bootstrap config (template)
		RefRule{
			FromGroup: "cluster.x-k8s.io", FromKind: "MachineDeployment",
			FieldPath: "spec.template.spec.bootstrap.configRef",
		},
	)
}
