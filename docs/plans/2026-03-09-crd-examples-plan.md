# CRD Example Resolvers — Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Ship five example resolvers demonstrating how users extend Ariadne for their own CRDs, using both declarative rules and custom Resolver implementations.

**Architecture:** Each ecosystem gets an exported constructor (`NewXxxResolver()`) in its own file, not registered by `NewDefault()`. Three use pure `RefRule` configuration (Gateway API, Cluster API, Argo CD). Two use custom `Resolver` implementations (Kyverno, Crossplane). Tests validate forward and reverse resolution via `Graph.Load` and `Graph.Add`.

**Tech Stack:** Go, `k8s.io/apimachinery/pkg/apis/meta/v1/unstructured`, existing Ariadne rule primitives (`RefRule`, `LabelSelectorRule`, `NewRuleResolver`).

**Reference files:**
- Rule primitives: `rules.go` — `RefRule`, `NewRuleResolver`, `resolveRef`, `resolveTypedRef`, `parseTypedRef`
- Existing resolver pattern: `structural.go` — `NewStructuralResolver()`, `structuralResolver` wrapper
- Existing resolver pattern: `selector.go` — `NewSelectorResolver()`, `namedResolver` wrapper
- Test helpers: `graph_test.go:24-37` — `newObj(group, version, kind, ns, name)`, `newCoreObj(kind, ns, name)`
- Integration test examples: `integration_test.go` — realistic K8s objects with `Graph.Load`
- Resolver interface: `resolver.go` — `Resolver`, `Lookup`
- Graph construction: `graph.go` — `New()`, `WithResolver()`, `Graph.Add()`, `Graph.Load()`
- Types: `types.go` — `ObjectRef`, `Edge`, `EdgeType` (including `EdgeCustom`)

**Test helpers:** Tests use `newObj` and `newCoreObj` from `graph_test.go`. These are available because all files are in `package ariadne`. Objects with spec fields are constructed using raw `unstructured.Unstructured{Object: map[string]interface{}{...}}` (see `integration_test.go` for examples).

---

### Task 1: Gateway API resolver + tests

**Files:**
- Create: `gateway.go`
- Create: `gateway_test.go`

**Step 1: Write `gateway.go`**

```go
package ariadne

// NewGatewayAPIResolver returns a resolver for Gateway API resource references.
// Handles HTTPRoute backendRefs and parentRefs (typed-refs) and
// Gateway gatewayClassName (bare name ref).
//
// Not registered by NewDefault() — opt in with WithResolver(NewGatewayAPIResolver()).
func NewGatewayAPIResolver() Resolver {
	return NewRuleResolver("gateway-api",
		// HTTPRoute -> backend services (typed-ref: kind/name/group/namespace)
		RefRule{
			FromGroup: "gateway.networking.k8s.io", FromKind: "HTTPRoute",
			FieldPath: "spec.rules[*].backendRefs[*]",
		},
		// HTTPRoute -> parent Gateway (typed-ref)
		RefRule{
			FromGroup: "gateway.networking.k8s.io", FromKind: "HTTPRoute",
			FieldPath: "spec.parentRefs[*]",
		},
		// Gateway -> GatewayClass (bare name, cluster-scoped)
		RefRule{
			FromGroup: "gateway.networking.k8s.io", FromKind: "Gateway",
			ToGroup: "gateway.networking.k8s.io", ToKind: "GatewayClass",
			FieldPath: "spec.gatewayClassName",
		},
	)
}
```

**Step 2: Write `gateway_test.go`**

Three test functions:

1. `TestGatewayAPI_HTTPRouteBackendRef` — Load HTTPRoute + Service, verify HTTPRoute → Service edge via backendRef typed-ref.
2. `TestGatewayAPI_HTTPRouteParentRef` — Load HTTPRoute + Gateway, verify HTTPRoute → Gateway edge. Also test reverse: add Gateway first, then HTTPRoute via `Add`.
3. `TestGatewayAPI_GatewayClassName` — Load Gateway + GatewayClass, verify Gateway → GatewayClass edge.

HTTPRoute object shape (for backendRefs):
```go
{Object: map[string]interface{}{
    "apiVersion": "gateway.networking.k8s.io/v1", "kind": "HTTPRoute",
    "metadata": map[string]interface{}{
        "name": "web-route", "namespace": "default",
    },
    "spec": map[string]interface{}{
        "parentRefs": []interface{}{
            map[string]interface{}{
                "group": "gateway.networking.k8s.io",
                "kind":  "Gateway",
                "name":  "main-gw",
            },
        },
        "rules": []interface{}{
            map[string]interface{}{
                "backendRefs": []interface{}{
                    map[string]interface{}{
                        "group": "",
                        "kind":  "Service",
                        "name":  "web-svc",
                    },
                },
            },
        },
    },
}}
```

**Step 3: Run tests**

Run: `go test -run TestGatewayAPI -v ./...`
Expected: all PASS

**Step 4: Commit**

```
Add Gateway API example resolver

HTTPRoute backendRefs/parentRefs (typed-ref) and Gateway gatewayClassName
(bare name ref to GatewayClass).
```

---

### Task 2: Cluster API resolver + tests

**Files:**
- Create: `clusterapi.go`
- Create: `clusterapi_test.go`

**Step 1: Write `clusterapi.go`**

```go
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
```

**Step 2: Write `clusterapi_test.go`**

Three test functions:

1. `TestClusterAPI_MachineInfraRef` — Machine → DockerMachine + KubeadmConfig via infrastructureRef and bootstrap.configRef. Forward via Load.
2. `TestClusterAPI_ClusterRefs` — Cluster → DockerCluster + KubeadmControlPlane. Forward via Load.
3. `TestClusterAPI_ReverseAdd` — Add DockerMachine first, then Machine via Add. Verify reverse resolution creates the edge.

Machine object shape:
```go
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
}}
```

**Step 3: Run tests**

Run: `go test -run TestClusterAPI -v ./...`
Expected: all PASS

**Step 4: Commit**

```
Add Cluster API example resolver

Machine, Cluster, MachineDeployment infrastructureRef/controlPlaneRef/bootstrap.configRef
— all unconstrained typed-refs.
```

---

### Task 3: Argo CD resolver + tests

**Files:**
- Create: `argocd.go`
- Create: `argocd_test.go`

**Step 1: Write `argocd.go`**

```go
package ariadne

// NewArgoCDResolver returns a resolver for Argo CD Application references.
// Handles destination namespace and project references.
//
// Not registered by NewDefault() — opt in with WithResolver(NewArgoCDResolver()).
func NewArgoCDResolver() Resolver {
	return NewRuleResolver("argocd",
		// Application -> target Namespace
		RefRule{
			FromGroup: "argoproj.io", FromKind: "Application",
			ToKind:    "Namespace",
			FieldPath: "spec.destination.namespace",
		},
		// Application -> AppProject
		RefRule{
			FromGroup: "argoproj.io", FromKind: "Application",
			ToGroup: "argoproj.io", ToKind: "AppProject",
			FieldPath: "spec.project",
		},
	)
}
```

**Step 2: Write `argocd_test.go`**

Two test functions:

1. `TestArgoCD_ApplicationRefs` — Load Application + Namespace + AppProject, verify both edges.
2. `TestArgoCD_ReverseAdd` — Add Application first, then Namespace via Add. Verify reverse resolution.

Application object shape:
```go
{Object: map[string]interface{}{
    "apiVersion": "argoproj.io/v1alpha1", "kind": "Application",
    "metadata": map[string]interface{}{
        "name": "web-app", "namespace": "argocd",
    },
    "spec": map[string]interface{}{
        "project": "default",
        "destination": map[string]interface{}{
            "namespace": "production",
        },
    },
}}
```

Note: Namespace is cluster-scoped (no namespace field). AppProject is namespaced but typically lives in the argocd namespace — however, the `spec.project` field is a bare name, so resolution tries same-namespace first (argocd), then cluster-scoped. The AppProject must be in the argocd namespace to match.

**Step 3: Run tests**

Run: `go test -run TestArgoCD -v ./...`
Expected: all PASS

**Step 4: Commit**

```
Add Argo CD example resolver

Application destination.namespace and project refs.
```

---

### Task 4: Kyverno resolver + tests

This is a **custom Resolver** — first example that implements the `Resolver` interface directly.

**Files:**
- Create: `kyverno.go`
- Create: `kyverno_test.go`

**Step 1: Write `kyverno.go`**

The resolver needs to:
- **Forward (Policy added):** Extract kind names from `spec.rules[*].match.resources.kinds`, find all instances of each kind in the graph via `lookup.List`, emit edges.
- **Reverse (any object added):** List all ClusterPolicy and Policy objects, check if the added object's kind matches any policy's kind list, emit edges.

Key detail: Kyverno kinds strings can be `"Pod"` (core) or `"apps/v1/Deployment"` (with group/version). For simplicity, the example handles plain kind strings only. Document this limitation.

```go
package ariadne

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// NewKyvernoResolver returns a resolver for Kyverno policy-to-resource
// relationships. When a ClusterPolicy or Policy lists resource kinds in
// spec.rules[*].match.resources.kinds, this resolver creates edges from
// the policy to every instance of those kinds in the graph.
//
// Handles plain kind names only (e.g., "Pod", "Service"). Does not parse
// group-qualified kinds like "apps/v1/Deployment".
//
// Not registered by NewDefault() — opt in with WithResolver(NewKyvernoResolver()).
func NewKyvernoResolver() Resolver {
	return &kyvernoResolver{}
}

type kyvernoResolver struct{}

func (r *kyvernoResolver) Name() string { return "kyverno" }

func (r *kyvernoResolver) Resolve(obj *unstructured.Unstructured, lookup Lookup) []Edge {
	ref := RefFromUnstructured(obj)
	gvk := obj.GroupVersionKind()

	if isKyvernoPolicy(gvk.Group, gvk.Kind) {
		return r.resolveForward(ref, obj, lookup)
	}
	return r.resolveReverse(ref, obj, lookup)
}

func isKyvernoPolicy(group, kind string) bool {
	return group == "kyverno.io" && (kind == "ClusterPolicy" || kind == "Policy")
}

func (r *kyvernoResolver) resolveForward(ref ObjectRef, obj *unstructured.Unstructured, lookup Lookup) []Edge {
	kinds := extractPolicyKinds(obj)
	if len(kinds) == 0 {
		return nil
	}

	var edges []Edge
	for _, kind := range kinds {
		// Core resources have empty group — use List("", kind).
		for _, target := range lookup.List("", kind) {
			edges = append(edges, Edge{
				From:     ref,
				To:       RefFromUnstructured(target),
				Type:     EdgeCustom,
				Resolver: "kyverno",
				Field:    "spec.rules[*].match.resources.kinds",
			})
		}
	}
	return edges
}

func (r *kyvernoResolver) resolveReverse(ref ObjectRef, obj *unstructured.Unstructured, lookup Lookup) []Edge {
	var edges []Edge

	// Check ClusterPolicy (cluster-scoped)
	for _, policy := range lookup.List("kyverno.io", "ClusterPolicy") {
		kinds := extractPolicyKinds(policy)
		for _, kind := range kinds {
			if kind == ref.Kind && ref.Group == "" {
				edges = append(edges, Edge{
					From:     RefFromUnstructured(policy),
					To:       ref,
					Type:     EdgeCustom,
					Resolver: "kyverno",
					Field:    "spec.rules[*].match.resources.kinds",
				})
				break
			}
		}
	}

	// Check Policy (namespaced)
	for _, policy := range lookup.List("kyverno.io", "Policy") {
		policyRef := RefFromUnstructured(policy)
		if policyRef.Namespace != ref.Namespace {
			continue
		}
		kinds := extractPolicyKinds(policy)
		for _, kind := range kinds {
			if kind == ref.Kind && ref.Group == "" {
				edges = append(edges, Edge{
					From:     policyRef,
					To:       ref,
					Type:     EdgeCustom,
					Resolver: "kyverno",
					Field:    "spec.rules[*].match.resources.kinds",
				})
				break
			}
		}
	}

	return edges
}

// extractPolicyKinds extracts the kind strings from
// spec.rules[*].match.resources.kinds.
func extractPolicyKinds(obj *unstructured.Unstructured) []string {
	return extractFieldValues(obj.Object, "spec.rules[*].match.resources.kinds[*]")
}
```

**Step 2: Write `kyverno_test.go`**

Three test functions:

1. `TestKyverno_ClusterPolicyForward` — Load ClusterPolicy + Pods, verify policy → each Pod edge.
2. `TestKyverno_PolicyNamespaceScoped` — Load Policy in ns "default" + Pod in "default" + Pod in "other". Verify policy only matches the same-namespace Pod.
3. `TestKyverno_ReverseAdd` — Add ClusterPolicy first, then Pod via Add. Verify edge created.

ClusterPolicy object shape:
```go
{Object: map[string]interface{}{
    "apiVersion": "kyverno.io/v1", "kind": "ClusterPolicy",
    "metadata": map[string]interface{}{
        "name": "require-labels",
    },
    "spec": map[string]interface{}{
        "rules": []interface{}{
            map[string]interface{}{
                "match": map[string]interface{}{
                    "resources": map[string]interface{}{
                        "kinds": []interface{}{"Pod"},
                    },
                },
            },
        },
    },
}}
```

**Step 3: Run tests**

Run: `go test -run TestKyverno -v ./...`
Expected: all PASS

**Step 4: Commit**

```
Add Kyverno example resolver

Custom Resolver for policy-to-resource kind-level matching.
Forward and reverse resolution for ClusterPolicy and Policy.
```

---

### Task 5: Crossplane resolver + tests

**Files:**
- Create: `crossplane.go`
- Create: `crossplane_test.go`

**Step 1: Write `crossplane.go`**

Two resolvers combined:

```go
package ariadne

import (
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// NewCrossplaneResolver returns a resolver for Crossplane resource references.
// Combines two patterns:
//   - providerConfigRef: RefRule for a specific managed resource type
//     (callers pass their managed resource group/kind)
//   - compositeTypeRef: custom resolver matching Compositions to all
//     instances of the referenced composite resource GroupKind
//
// Not registered by NewDefault() — opt in with WithResolver(NewCrossplaneResolver(...)).
func NewCrossplaneResolver(managedResources ...ManagedResource) Resolver {
	return &crossplaneResolver{managedResources: managedResources}
}

// ManagedResource identifies a Crossplane managed resource type for
// providerConfigRef resolution.
type ManagedResource struct {
	Group string
	Kind  string
}

type crossplaneResolver struct {
	managedResources []ManagedResource
	rules            Resolver // lazily built
}

func (r *crossplaneResolver) Name() string { return "crossplane" }

func (r *crossplaneResolver) ensureRules() Resolver {
	if r.rules != nil {
		return r.rules
	}
	var rules []Rule
	for _, mr := range r.managedResources {
		rules = append(rules, RefRule{
			FromGroup: mr.Group, FromKind: mr.Kind,
			ToGroup: "pkg.crossplane.io", ToKind: "ProviderConfig",
			FieldPath: "spec.providerConfigRef",
		})
	}
	r.rules = NewRuleResolver("crossplane", rules...)
	return r.rules
}

func (r *crossplaneResolver) Resolve(obj *unstructured.Unstructured, lookup Lookup) []Edge {
	var edges []Edge

	// providerConfigRef via declarative rules
	if len(r.managedResources) > 0 {
		ruleEdges := r.ensureRules().Resolve(obj, lookup)
		for i := range ruleEdges {
			ruleEdges[i].Resolver = "crossplane"
		}
		edges = append(edges, ruleEdges...)
	}

	// compositeTypeRef via custom logic
	edges = append(edges, r.resolveCompositeTypeRef(obj, lookup)...)

	return edges
}

func (r *crossplaneResolver) resolveCompositeTypeRef(obj *unstructured.Unstructured, lookup Lookup) []Edge {
	ref := RefFromUnstructured(obj)
	gvk := obj.GroupVersionKind()

	if gvk.Group == "apiextensions.crossplane.io" && gvk.Kind == "Composition" {
		return r.compositeForward(ref, obj, lookup)
	}
	return r.compositeReverse(ref, obj, lookup)
}

func (r *crossplaneResolver) compositeForward(ref ObjectRef, obj *unstructured.Unstructured, lookup Lookup) []Edge {
	group, kind := extractCompositeTypeRef(obj)
	if kind == "" {
		return nil
	}

	var edges []Edge
	for _, target := range lookup.List(group, kind) {
		edges = append(edges, Edge{
			From:     ref,
			To:       RefFromUnstructured(target),
			Type:     EdgeCustom,
			Resolver: "crossplane",
			Field:    "spec.compositeTypeRef",
		})
	}
	return edges
}

func (r *crossplaneResolver) compositeReverse(ref ObjectRef, obj *unstructured.Unstructured, lookup Lookup) []Edge {
	var edges []Edge
	for _, comp := range lookup.List("apiextensions.crossplane.io", "Composition") {
		group, kind := extractCompositeTypeRef(comp)
		if kind == "" {
			continue
		}
		if ref.Group == group && ref.Kind == kind {
			edges = append(edges, Edge{
				From:     RefFromUnstructured(comp),
				To:       ref,
				Type:     EdgeCustom,
				Resolver: "crossplane",
				Field:    "spec.compositeTypeRef",
			})
		}
	}
	return edges
}

func extractCompositeTypeRef(obj *unstructured.Unstructured) (group, kind string) {
	spec, _ := obj.Object["spec"].(map[string]interface{})
	if spec == nil {
		return "", ""
	}
	ref, _ := spec["compositeTypeRef"].(map[string]interface{})
	if ref == nil {
		return "", ""
	}
	kind, _ = ref["kind"].(string)
	group, _ = ref["apiGroup"].(string)
	return group, kind
}
```

**Step 2: Write `crossplane_test.go`**

Three test functions:

1. `TestCrossplane_ProviderConfigRef` — Load RDSInstance + ProviderConfig, verify edge. Uses `NewCrossplaneResolver(ManagedResource{Group: "database.aws.crossplane.io", Kind: "RDSInstance"})`.
2. `TestCrossplane_CompositeTypeRef` — Load Composition referencing `myapp.example.org/XMyDatabase` + two XMyDatabase instances. Verify edges from Composition to both.
3. `TestCrossplane_CompositeReverseAdd` — Add Composition first, then XMyDatabase via Add. Verify reverse edge.

Composition object shape:
```go
{Object: map[string]interface{}{
    "apiVersion": "apiextensions.crossplane.io/v1", "kind": "Composition",
    "metadata": map[string]interface{}{
        "name": "mydatabase-composition",
    },
    "spec": map[string]interface{}{
        "compositeTypeRef": map[string]interface{}{
            "apiGroup": "myapp.example.org",
            "kind":     "XMyDatabase",
        },
    },
}}
```

**Step 3: Run tests**

Run: `go test -run TestCrossplane -v ./...`
Expected: all PASS

**Step 4: Commit**

```
Add Crossplane example resolver

providerConfigRef via RefRule, compositeTypeRef via custom Resolver
for type-level group+kind matching.
```

---

### Task 6: Update TODO.md and run full test suite

**Files:**
- Modify: `TODO.md`

**Step 1: Check all items in TODO.md**

Mark all five CRD example items as complete (`[x]`).

**Step 2: Run full test suite**

Run: `go test -race -v ./...`
Expected: all PASS, no races

**Step 3: Commit**

```
Mark CRD example resolvers as complete in TODO
```

---

### Task order and dependencies

```
Task 1 (Gateway API)   ──┐
Task 2 (Cluster API)   ──┤
Task 3 (Argo CD)       ──┼── all independent, can run in parallel
Task 4 (Kyverno)        ──┤
Task 5 (Crossplane)    ──┘
                          │
                          v
                    Task 6 (TODO + full suite)
```

Tasks 1-5 are independent — no shared code, no dependencies between them. Task 6 depends on all five completing.
