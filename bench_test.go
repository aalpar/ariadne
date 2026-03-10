package ariadne

import (
	"fmt"
	"testing"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// generateCluster creates a realistic mix of K8s objects distributed across
// namespaces. Distribution per namespace (roughly):
//
//	40% Pods (with ownerRefs, serviceAccount, configMap/secret refs)
//	10% ReplicaSets
//	15% ConfigMaps
//	15% Secrets
//	 5% ServiceAccounts
//	 5% Services (with label selectors)
//	 5% Roles
//	 5% RoleBindings (with subjects)
func generateCluster(n int) []unstructured.Unstructured {
	nsCount := max(1, n/100)
	perNS := n / nsCount

	rsCount := max(1, perNS/10)
	cmCount := max(1, perNS*15/100)
	secretCount := max(1, perNS*15/100)
	saCount := max(1, perNS*5/100)
	svcCount := max(1, perNS*5/100)
	roleCount := max(1, perNS*5/100)
	rbCount := max(1, perNS*5/100)
	podCount := perNS - rsCount - cmCount - secretCount - saCount - svcCount - roleCount - rbCount
	if podCount < 1 {
		podCount = 1
	}

	objs := make([]unstructured.Unstructured, 0, n)

	for i := 0; i < nsCount; i++ {
		ns := fmt.Sprintf("ns-%d", i)

		for j := 0; j < rsCount; j++ {
			objs = append(objs, benchReplicaSet(ns, fmt.Sprintf("rs-%d", j)))
		}
		for j := 0; j < cmCount; j++ {
			objs = append(objs, benchConfigMap(ns, fmt.Sprintf("cm-%d", j)))
		}
		for j := 0; j < secretCount; j++ {
			objs = append(objs, benchSecret(ns, fmt.Sprintf("secret-%d", j)))
		}
		for j := 0; j < saCount; j++ {
			objs = append(objs, benchServiceAccount(ns, fmt.Sprintf("sa-%d", j)))
		}
		for j := 0; j < svcCount; j++ {
			objs = append(objs, benchService(ns, fmt.Sprintf("svc-%d", j), j))
		}
		for j := 0; j < roleCount; j++ {
			objs = append(objs, benchRole(ns, fmt.Sprintf("role-%d", j)))
		}
		for j := 0; j < rbCount; j++ {
			objs = append(objs, benchRoleBinding(ns,
				fmt.Sprintf("rb-%d", j),
				fmt.Sprintf("role-%d", j%roleCount),
				fmt.Sprintf("sa-%d", j%saCount)))
		}
		for j := 0; j < podCount; j++ {
			objs = append(objs, benchPod(ns,
				fmt.Sprintf("pod-%d", j),
				fmt.Sprintf("rs-%d", j%rsCount),
				fmt.Sprintf("sa-%d", j%saCount),
				fmt.Sprintf("cm-%d", j%cmCount),
				fmt.Sprintf("secret-%d", j%secretCount),
				j%svcCount))
		}
	}

	return objs
}

func BenchmarkLoad(b *testing.B) {
	for _, n := range []int{100, 1000, 10000} {
		objs := generateCluster(n)
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				g := NewDefault()
				g.Load(objs)
			}
		})
	}
}

func BenchmarkAddAll(b *testing.B) {
	for _, n := range []int{100, 1000, 10000} {
		objs := generateCluster(n)
		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				g := NewDefault()
				for j := range objs {
					g.Add(objs[j])
				}
			}
		})
	}
}

func BenchmarkAddSingle(b *testing.B) {
	for _, n := range []int{100, 1000, 10000} {
		objs := generateCluster(n)
		pod := benchPod("ns-0", "bench-extra", "rs-0", "sa-0", "cm-0", "secret-0", 0)
		ref := RefFromUnstructured(&pod)
		b.Run(fmt.Sprintf("graph=%d", n), func(b *testing.B) {
			g := NewDefault()
			g.Load(objs)
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				g.Add(pod)
				g.Remove(ref)
			}
		})
	}
}

// --- object generators ---

func benchPod(ns, name, owner, sa, cm, secret string, svcGroup int) unstructured.Unstructured {
	return unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Pod",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": ns,
			"labels": map[string]interface{}{
				"app":       name,
				"svc-group": fmt.Sprintf("%d", svcGroup),
			},
			"ownerReferences": []interface{}{
				map[string]interface{}{
					"apiVersion": "apps/v1",
					"kind":       "ReplicaSet",
					"name":       owner,
				},
			},
		},
		"spec": map[string]interface{}{
			"serviceAccountName": sa,
			"containers": []interface{}{
				map[string]interface{}{
					"name": "main",
					"envFrom": []interface{}{
						map[string]interface{}{
							"configMapRef": map[string]interface{}{"name": cm},
						},
						map[string]interface{}{
							"secretRef": map[string]interface{}{"name": secret},
						},
					},
				},
			},
			"volumes": []interface{}{
				map[string]interface{}{
					"name":      "config",
					"configMap": map[string]interface{}{"name": cm},
				},
			},
		},
	}}
}

func benchReplicaSet(ns, name string) unstructured.Unstructured {
	return unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "apps/v1",
		"kind":       "ReplicaSet",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": ns,
		},
	}}
}

func benchConfigMap(ns, name string) unstructured.Unstructured {
	return unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ConfigMap",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": ns,
		},
	}}
}

func benchSecret(ns, name string) unstructured.Unstructured {
	return unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Secret",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": ns,
		},
	}}
}

func benchServiceAccount(ns, name string) unstructured.Unstructured {
	return unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "ServiceAccount",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": ns,
		},
	}}
}

func benchService(ns, name string, group int) unstructured.Unstructured {
	return unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "v1",
		"kind":       "Service",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": ns,
		},
		"spec": map[string]interface{}{
			"selector": map[string]interface{}{
				"svc-group": fmt.Sprintf("%d", group),
			},
		},
	}}
}

func benchRole(ns, name string) unstructured.Unstructured {
	return unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "Role",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": ns,
		},
	}}
}

func benchRoleBinding(ns, name, role, sa string) unstructured.Unstructured {
	return unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": "rbac.authorization.k8s.io/v1",
		"kind":       "RoleBinding",
		"metadata": map[string]interface{}{
			"name":      name,
			"namespace": ns,
		},
		"roleRef": map[string]interface{}{
			"apiGroup": "rbac.authorization.k8s.io",
			"kind":     "Role",
			"name":     role,
		},
		"subjects": []interface{}{
			map[string]interface{}{
				"kind":      "ServiceAccount",
				"name":      sa,
				"namespace": ns,
			},
		},
	}}
}
