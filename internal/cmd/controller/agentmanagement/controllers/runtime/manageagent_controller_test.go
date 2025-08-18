package runtime

import (
	"context"
	"testing"

	"github.com/rancher/fleet/internal/config"
	fleet "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestManageAgentReconciler_Reconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, fleet.AddToScheme(scheme))

	// Set up required environment variables for agent manifests
	t.Setenv("FLEET_AGENT_REPLICA_COUNT", "1")
	t.Setenv("FLEET_AGENT_ELECTION_LEASE_DURATION", "15s")
	t.Setenv("FLEET_AGENT_ELECTION_RENEW_DEADLINE", "10s")
	t.Setenv("FLEET_AGENT_ELECTION_RETRY_PERIOD", "2s")

	// Set up test config
	config.Set(&config.Config{
		ManageAgent: ptr.To(true),
	})

	systemNamespace := "cattle-fleet-system"
	testNamespace := "fleet-local"

	tests := []struct {
		name                string
		namespace           *corev1.Namespace
		clusters            []fleet.Cluster
		expectBundleCreated bool
		expectError         bool
	}{
		{
			name: "creates agent bundle for cluster",
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: testNamespace,
				},
			},
			clusters: []fleet.Cluster{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-cluster",
						Namespace: testNamespace,
					},
					Spec: fleet.ClusterSpec{
						AgentEnvVars: []corev1.EnvVar{
							{Name: "TEST_VAR", Value: "test-value"},
						},
					},
				},
			},
			expectBundleCreated: true,
			expectError:         false,
		},
		{
			name: "skips cluster with management label",
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: testNamespace,
				},
			},
			clusters: []fleet.Cluster{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "managed-cluster",
						Namespace: testNamespace,
						Labels: map[string]string{
							fleet.ClusterManagementLabel: "managed",
						},
					},
				},
			},
			expectBundleCreated: false,
			expectError:         false,
		},
		{
			name: "handles namespace with no clusters",
			namespace: &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: testNamespace,
				},
			},
			clusters:            []fleet.Cluster{},
			expectBundleCreated: false,
			expectError:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objects []client.Object
			if tt.namespace != nil {
				objects = append(objects, tt.namespace)
			}
			for i := range tt.clusters {
				objects = append(objects, &tt.clusters[i])
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				Build()

			reconciler := &ManageAgentReconciler{
				Client:          fakeClient,
				Log:             zap.New(zap.UseDevMode(true)),
				Scheme:          scheme,
				SystemNamespace: systemNamespace,
			}

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name: testNamespace,
				},
			}

			result, err := reconciler.Reconcile(context.TODO(), req)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.False(t, result.Requeue)
			}

			// Check if bundle was created
			var bundles fleet.BundleList
			err = fakeClient.List(context.TODO(), &bundles, client.InNamespace(testNamespace))
			require.NoError(t, err)

			if tt.expectBundleCreated {
				assert.Len(t, bundles.Items, 1)
				bundle := bundles.Items[0]
				assert.Equal(t, "fleet-agent-test-cluster", bundle.Name)
				assert.Len(t, bundle.Spec.Resources, 1)
				assert.Equal(t, "agent.yaml", bundle.Spec.Resources[0].Name)
			} else {
				assert.Len(t, bundles.Items, 0)
			}
		})
	}
}

func TestClusterReconciler_Reconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, fleet.AddToScheme(scheme))

	systemNamespace := "cattle-fleet-system"
	testNamespace := "fleet-local"

	tests := []struct {
		name               string
		cluster            *fleet.Cluster
		expectStatusUpdate bool
		expectError        bool
	}{
		{
			name: "updates status when agent env vars change",
			cluster: &fleet.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: testNamespace,
				},
				Spec: fleet.ClusterSpec{
					AgentEnvVars: []corev1.EnvVar{
						{Name: "NEW_VAR", Value: "new-value"},
					},
				},
				Status: fleet.ClusterStatus{
					AgentEnvVarsHash: "old-hash",
				},
			},
			expectStatusUpdate: true,
			expectError:        false,
		},
		{
			name: "skips cluster with management label",
			cluster: &fleet.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "managed-cluster",
					Namespace: testNamespace,
					Labels: map[string]string{
						fleet.ClusterManagementLabel: "managed",
					},
				},
			},
			expectStatusUpdate: false,
			expectError:        false,
		},
		{
			name: "handles cluster without changes",
			cluster: &fleet.Cluster{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-cluster",
					Namespace: testNamespace,
				},
				Spec: fleet.ClusterSpec{
					AgentEnvVars: []corev1.EnvVar{
						{Name: "SAME_VAR", Value: "same-value"},
					},
				},
				Status: fleet.ClusterStatus{
					// Hash will match the current env vars
				},
			},
			expectStatusUpdate: false, // Status won't change if hash matches
			expectError:        false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objects []client.Object
			if tt.cluster != nil {
				objects = append(objects, tt.cluster)
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				WithStatusSubresource(&fleet.Cluster{}).
				Build()

			reconciler := &ClusterReconciler{
				Client:          fakeClient,
				Log:             zap.New(zap.UseDevMode(true)),
				Scheme:          scheme,
				SystemNamespace: systemNamespace,
			}

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: testNamespace,
					Name:      tt.cluster.Name,
				},
			}

			originalStatus := tt.cluster.Status.DeepCopy()
			result, err := reconciler.Reconcile(context.TODO(), req)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.False(t, result.Requeue)
			}

			// Check if status was updated
			var updatedCluster fleet.Cluster
			err = fakeClient.Get(context.TODO(), req.NamespacedName, &updatedCluster)
			require.NoError(t, err)

			if tt.expectStatusUpdate {
				// Status should be different from original
				assert.NotEqual(t, originalStatus, &updatedCluster.Status)
				assert.True(t, updatedCluster.Status.AgentConfigChanged)
			}
		})
	}
}

func TestNewAgentBundle(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, fleet.AddToScheme(scheme))

	// Set up required environment variables for agent manifests
	t.Setenv("FLEET_AGENT_REPLICA_COUNT", "1")
	t.Setenv("FLEET_AGENT_ELECTION_LEASE_DURATION", "15s")
	t.Setenv("FLEET_AGENT_ELECTION_RENEW_DEADLINE", "10s")
	t.Setenv("FLEET_AGENT_ELECTION_RETRY_PERIOD", "2s")

	// Set up test config
	config.Set(&config.Config{
		AgentImage:            "rancher/fleet-agent:test",
		AgentImagePullPolicy:  "Always",
		SystemDefaultRegistry: "registry.example.com",
	})

	reconciler := &ManageAgentReconciler{
		Log:             zap.New(zap.UseDevMode(true)),
		SystemNamespace: "cattle-fleet-system",
	}

	cluster := &fleet.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-cluster",
			Namespace: "fleet-local",
		},
		Spec: fleet.ClusterSpec{
			AgentEnvVars: []corev1.EnvVar{
				{Name: "TEST_VAR", Value: "test-value"},
			},
			AgentNamespace: "custom-agent-ns",
		},
	}

	bundle, err := reconciler.newAgentBundle("fleet-local", cluster)
	require.NoError(t, err)

	assert.Equal(t, "fleet-agent-test-cluster", bundle.Name)
	assert.Equal(t, "fleet-local", bundle.Namespace)
	assert.Equal(t, "custom-agent-ns", bundle.Spec.BundleDeploymentOptions.DefaultNamespace)
	assert.Len(t, bundle.Spec.Resources, 1)
	assert.Equal(t, "agent.yaml", bundle.Spec.Resources[0].Name)
	assert.NotEmpty(t, bundle.Spec.Resources[0].Content)
	assert.Len(t, bundle.Spec.Targets, 1)
	assert.Equal(t, "test-cluster", bundle.Spec.Targets[0].ClusterName)
}

func TestHashStatusField(t *testing.T) {
	tests := []struct {
		name     string
		field    interface{}
		expected bool // whether hash should be generated without error
	}{
		{
			name: "env vars",
			field: []corev1.EnvVar{
				{Name: "TEST", Value: "value"},
			},
			expected: true,
		},
		{
			name: "affinity",
			field: &corev1.Affinity{
				NodeAffinity: &corev1.NodeAffinity{},
			},
			expected: true,
		},
		{
			name:     "nil field",
			field:    nil,
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash, err := hashStatusField(tt.field)
			if tt.expected {
				assert.NoError(t, err)
				assert.NotEmpty(t, hash)
			} else {
				assert.Error(t, err)
			}
		})
	}
}

func TestHashChanged(t *testing.T) {
	affinity := &corev1.Affinity{
		NodeAffinity: &corev1.NodeAffinity{},
	}

	tests := []struct {
		name         string
		field        interface{}
		statusHash   string
		expectChange bool
	}{
		{
			name:         "nil field with empty hash",
			field:        (*corev1.Affinity)(nil),
			statusHash:   "",
			expectChange: false,
		},
		{
			name:         "nil field with existing hash",
			field:        (*corev1.Affinity)(nil),
			statusHash:   "some-hash",
			expectChange: true,
		},
		{
			name:         "field with different hash",
			field:        affinity,
			statusHash:   "different-hash",
			expectChange: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			changed, newHash, err := hashChanged(tt.field, tt.statusHash)
			assert.NoError(t, err)
			assert.Equal(t, tt.expectChange, changed)

			if tt.field == nil {
				assert.Empty(t, newHash)
			} else if tt.expectChange {
				assert.NotEqual(t, tt.statusHash, newHash)
			}
		})
	}
}
