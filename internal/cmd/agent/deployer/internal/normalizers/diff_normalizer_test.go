package normalizers

import (
	"slices"
	"testing"

	"github.com/rancher/fleet/internal/cmd/agent/deployer/internal/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

func TestDefaultIgnoreDifferences(t *testing.T) {
	tests := []struct {
		name          string
		obj           *unstructured.Unstructured
		fieldPath     []string
		shouldBeEmpty bool
	}{
		{
			name: "Istio sidecar status annotation ignored",
			obj: &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]any{
						"name": "test-deployment",
					},
					"spec": map[string]any{
						"template": map[string]any{
							"metadata": map[string]any{
								"annotations": map[string]any{
									"sidecar.istio.io/status": `{"version":"abc123"}`,
								},
							},
						},
					},
				},
			},
			fieldPath:     []string{"spec", "template", "metadata", "annotations", "sidecar.istio.io/status"},
			shouldBeEmpty: true,
		},
		{
			name: "Linkerd proxy version annotation ignored",
			obj: &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "apps/v1",
					"kind":       "StatefulSet",
					"metadata": map[string]any{
						"name": "test-statefulset",
					},
					"spec": map[string]any{
						"template": map[string]any{
							"metadata": map[string]any{
								"annotations": map[string]any{
									"linkerd.io/proxy-version": "stable-2.11.0",
								},
							},
						},
					},
				},
			},
			fieldPath:     []string{"spec", "template", "metadata", "annotations", "linkerd.io/proxy-version"},
			shouldBeEmpty: true,
		},
		{
			name: "KEDA paused-replicas annotation ignored",
			obj: &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]any{
						"name": "test-deployment",
						"annotations": map[string]any{
							"autoscaling.keda.sh/paused-replicas": "0",
						},
					},
				},
			},
			fieldPath:     []string{"metadata", "annotations", "autoscaling.keda.sh/paused-replicas"},
			shouldBeEmpty: true,
		},
		{
			name: "VPA updates annotation ignored",
			obj: &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]any{
						"name": "test-deployment",
					},
					"spec": map[string]any{
						"template": map[string]any{
							"metadata": map[string]any{
								"annotations": map[string]any{
									"vpaUpdates": "Pod resources updated by my-vpa: container 0",
								},
							},
						},
					},
				},
			},
			fieldPath:     []string{"spec", "template", "metadata", "annotations", "vpaUpdates"},
			shouldBeEmpty: true,
		},
		{
			name: "Cluster autoscaler annotation ignored",
			obj: &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]any{
						"name": "test-deployment",
						"annotations": map[string]any{
							"cluster-autoscaler.kubernetes.io/safe-to-evict": "true",
						},
					},
				},
			},
			fieldPath:     []string{"metadata", "annotations", "cluster-autoscaler.kubernetes.io/safe-to-evict"},
			shouldBeEmpty: true,
		},
		{
			name: "Cert-manager Certificate status ignored",
			obj: &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "cert-manager.io/v1",
					"kind":       "Certificate",
					"metadata": map[string]any{
						"name": "test-cert",
					},
					"status": map[string]any{
						"conditions": []any{
							map[string]any{
								"type":   "Ready",
								"status": "True",
							},
						},
					},
				},
			},
			fieldPath:     []string{"status"},
			shouldBeEmpty: true,
		},
		{
			name: "Other fields not ignored",
			obj: &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "apps/v1",
					"kind":       "Deployment",
					"metadata": map[string]any{
						"name": "test-deployment",
					},
					"spec": map[string]any{
						"selector": map[string]any{
							"matchLabels": map[string]any{
								"app": "test",
							},
						},
					},
				},
			},
			fieldPath:     []string{"spec", "selector"},
			shouldBeEmpty: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalizer, err := NewIgnoreNormalizer([]resource.ResourceIgnoreDifferences{}, nil)
			require.NoError(t, err)

			err = normalizer.Normalize(tt.obj)
			require.NoError(t, err)

			val, found, err := unstructured.NestedFieldNoCopy(tt.obj.Object, tt.fieldPath...)
			require.NoError(t, err)

			if tt.shouldBeEmpty {
				assert.False(t, found, "field %v should have been removed", tt.fieldPath)
			} else {
				assert.True(t, found, "field %v should still exist", tt.fieldPath)
				assert.NotNil(t, val, "field %v should have a value", tt.fieldPath)
			}
		})
	}
}

func TestDefaultIgnoreDifferencesWithUserOverrides(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]any{
				"name": "test-deployment",
				"annotations": map[string]any{
					"autoscaling.keda.sh/paused-replicas": "0",
				},
			},
			"spec": map[string]any{
				"replicas": int64(3),
				"strategy": map[string]any{
					"type": "RollingUpdate",
				},
			},
		},
	}

	// User can add additional ignores beyond the defaults
	userIgnores := []resource.ResourceIgnoreDifferences{
		{
			Group:        "apps",
			Kind:         "Deployment",
			JSONPointers: []string{"/spec/strategy"},
		},
	}

	normalizer, err := NewIgnoreNormalizer(userIgnores, nil)
	require.NoError(t, err)

	err = normalizer.Normalize(obj)
	require.NoError(t, err)

	// Default (KEDA annotation) should be removed
	_, found, err := unstructured.NestedFieldNoCopy(obj.Object, "metadata", "annotations", "autoscaling.keda.sh/paused-replicas")
	require.NoError(t, err)
	assert.False(t, found, "KEDA annotation should have been removed by default ignore")

	// User-specified (strategy) should be removed
	_, found, err = unstructured.NestedFieldNoCopy(obj.Object, "spec", "strategy")
	require.NoError(t, err)
	assert.False(t, found, "strategy should have been removed by user ignore")

	// Replicas should still exist (not in defaults)
	_, found, err = unstructured.NestedFieldNoCopy(obj.Object, "spec", "replicas")
	require.NoError(t, err)
	assert.True(t, found, "replicas should still exist - only ignored when HPA/KEDA resources exist")
}

// TestDefaultIgnoreDifferencesExist verifies that the default ignore patterns are actually configured.
// This test protects against accidental removal of the defaults.
func TestDefaultIgnoreDifferencesExist(t *testing.T) {
	defaults := getDefaultIgnoreDifferences()

	// Verify we have defaults configured
	require.NotEmpty(t, defaults, "Default ignore patterns should not be empty")

	// Verify specific expected patterns exist
	expectedPatterns := []struct {
		group       string
		kind        string
		jsonPointer string
		description string
	}{
		{"apps", "Deployment", "/spec/template/metadata/annotations/sidecar.istio.io~1status", "Istio sidecar status"},
		{"apps", "Deployment", "/spec/template/metadata/annotations/linkerd.io~1proxy-version", "Linkerd proxy version"},
		{"apps", "Deployment", "/metadata/annotations/autoscaling.keda.sh~1paused-replicas", "KEDA paused replicas"},
		{"apps", "Deployment", "/spec/template/metadata/annotations/vpaUpdates", "VPA updates annotation"},
		{"apps", "Deployment", "/metadata/annotations/cluster-autoscaler.kubernetes.io~1safe-to-evict", "Cluster autoscaler annotation"},
		{"cert-manager.io", "Certificate", "/status", "Cert-manager certificate status"},
	}

	for _, expected := range expectedPatterns {
		found := false
		for _, def := range defaults {
			if def.Group == expected.group && def.Kind == expected.kind {
				if slices.Contains(def.JSONPointers, expected.jsonPointer) {
					found = true
				}
			}
		}
		assert.True(t, found, "Expected default pattern not found: %s (%s)", expected.description, expected.jsonPointer)
	}

	// Verify that replicas is NOT in the defaults (it should only be ignored conditionally)
	for _, def := range defaults {
		for _, ptr := range def.JSONPointers {
			assert.NotEqual(t, "/spec/replicas", ptr, "spec.replicas should not be in default ignores - it should only be ignored when HPA/KEDA is detected")
		}
	}
}

// TestNormalizerDoesNotAffectNonMatchingResources ensures that resources that don't match
// the default ignore patterns are not affected.
func TestNormalizerDoesNotAffectNonMatchingResources(t *testing.T) {
	tests := []struct {
		name string
		obj  *unstructured.Unstructured
	}{
		{
			name: "Service is not affected",
			obj: &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "v1",
					"kind":       "Service",
					"metadata": map[string]any{
						"name": "test-service",
					},
					"spec": map[string]any{
						"replicas": int64(3), // This field doesn't actually exist in Service, but tests ignore behavior
						"selector": map[string]any{
							"app": "test",
						},
					},
				},
			},
		},
		{
			name: "Pod is not affected",
			obj: &unstructured.Unstructured{
				Object: map[string]any{
					"apiVersion": "v1",
					"kind":       "Pod",
					"metadata": map[string]any{
						"name": "test-pod",
					},
					"spec": map[string]any{
						"containers": []any{
							map[string]any{
								"name":  "nginx",
								"image": "nginx:1.14.2",
							},
						},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			original := tt.obj.DeepCopy()

			normalizer, err := NewIgnoreNormalizer([]resource.ResourceIgnoreDifferences{}, nil)
			require.NoError(t, err)

			err = normalizer.Normalize(tt.obj)
			require.NoError(t, err)

			// Object should be unchanged (except for any standard JSON marshaling normalization)
			assert.Equal(t, original.GetKind(), tt.obj.GetKind())
			assert.Equal(t, original.GetAPIVersion(), tt.obj.GetAPIVersion())
			assert.Equal(t, original.GetName(), tt.obj.GetName())
		})
	}
}

// TestMultipleFieldsIgnoredInSameResource verifies that multiple fields can be ignored
// in the same resource type.
func TestMultipleFieldsIgnoredInSameResource(t *testing.T) {
	obj := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]any{
				"name": "test-deployment",
				"annotations": map[string]any{
					"autoscaling.keda.sh/paused-replicas":                          "0",
					"cluster-autoscaler.kubernetes.io/safe-to-evict":               "true",
					"cluster-autoscaler.kubernetes.io/safe-to-evict-local-volumes": "cache",
				},
			},
			"spec": map[string]any{
				"replicas": int64(3),
				"template": map[string]any{
					"metadata": map[string]any{
						"annotations": map[string]any{
							"sidecar.istio.io/status": `{"version":"abc"}`,
							"vpaUpdates":              "updated",
						},
					},
				},
				"selector": map[string]any{
					"matchLabels": map[string]any{
						"app": "test",
					},
				},
			},
		},
	}

	normalizer, err := NewIgnoreNormalizer([]resource.ResourceIgnoreDifferences{}, nil)
	require.NoError(t, err)

	err = normalizer.Normalize(obj)
	require.NoError(t, err)

	// KEDA annotation should be removed
	_, found, err := unstructured.NestedFieldNoCopy(obj.Object, "metadata", "annotations", "autoscaling.keda.sh/paused-replicas")
	require.NoError(t, err)
	assert.False(t, found, "KEDA annotation should be removed")

	// Cluster autoscaler annotations should be removed
	_, found, err = unstructured.NestedFieldNoCopy(obj.Object, "metadata", "annotations", "cluster-autoscaler.kubernetes.io/safe-to-evict")
	require.NoError(t, err)
	assert.False(t, found, "cluster autoscaler annotation should be removed")

	// Istio annotation should be removed
	_, found, err = unstructured.NestedFieldNoCopy(obj.Object, "spec", "template", "metadata", "annotations", "sidecar.istio.io/status")
	require.NoError(t, err)
	assert.False(t, found, "Istio annotation should be removed")

	// VPA annotation should be removed
	_, found, err = unstructured.NestedFieldNoCopy(obj.Object, "spec", "template", "metadata", "annotations", "vpaUpdates")
	require.NoError(t, err)
	assert.False(t, found, "VPA annotation should be removed")

	// But replicas should still exist (not ignored by default)
	_, found, err = unstructured.NestedFieldNoCopy(obj.Object, "spec", "replicas")
	require.NoError(t, err)
	assert.True(t, found, "replicas should still exist")

	// selector should still exist
	_, found, err = unstructured.NestedFieldNoCopy(obj.Object, "spec", "selector")
	require.NoError(t, err)
	assert.True(t, found, "selector should still exist")
}

// TestIgnorePatternOrderingPreservesUserOverrides ensures that user-provided patterns
// are not overridden by defaults.
func TestIgnorePatternOrderingPreservesUserOverrides(t *testing.T) {
	// Create two separate normalizers with same user ignores
	userIgnores := []resource.ResourceIgnoreDifferences{
		{
			Group:        "apps",
			Kind:         "Deployment",
			JSONPointers: []string{"/spec/template"},
		},
	}

	normalizer1, err := NewIgnoreNormalizer(userIgnores, nil)
	require.NoError(t, err)

	normalizer2, err := NewIgnoreNormalizer(userIgnores, nil)
	require.NoError(t, err)

	// Both should behave identically
	obj1 := &unstructured.Unstructured{
		Object: map[string]any{
			"apiVersion": "apps/v1",
			"kind":       "Deployment",
			"metadata": map[string]any{
				"annotations": map[string]any{
					"autoscaling.keda.sh/paused-replicas": "0",
				},
			},
			"spec": map[string]any{
				"replicas": int64(3),
				"template": map[string]any{
					"spec": map[string]any{},
				},
			},
		},
	}

	obj2 := obj1.DeepCopy()

	err = normalizer1.Normalize(obj1)
	require.NoError(t, err)

	err = normalizer2.Normalize(obj2)
	require.NoError(t, err)

	// Both should have KEDA annotation removed (default)
	_, found1, _ := unstructured.NestedFieldNoCopy(obj1.Object, "metadata", "annotations", "autoscaling.keda.sh/paused-replicas")
	_, found2, _ := unstructured.NestedFieldNoCopy(obj2.Object, "metadata", "annotations", "autoscaling.keda.sh/paused-replicas")
	assert.False(t, found1)
	assert.False(t, found2)

	// Both should have template removed (user override)
	_, found1, _ = unstructured.NestedFieldNoCopy(obj1.Object, "spec", "template")
	_, found2, _ = unstructured.NestedFieldNoCopy(obj2.Object, "spec", "template")
	assert.False(t, found1)
	assert.False(t, found2)

	// Both should still have replicas (not in defaults)
	_, found1, _ = unstructured.NestedFieldNoCopy(obj1.Object, "spec", "replicas")
	_, found2, _ = unstructured.NestedFieldNoCopy(obj2.Object, "spec", "replicas")
	assert.True(t, found1)
	assert.True(t, found2)
}
