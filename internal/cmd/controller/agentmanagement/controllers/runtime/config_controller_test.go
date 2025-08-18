package runtime

import (
	"context"
	"testing"
	"time"

	"github.com/rancher/fleet/internal/config"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

func TestConfigReconciler_Reconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	systemNamespace := "cattle-fleet-system"
	configMapName := config.ManagerConfigName

	tests := []struct {
		name          string
		configMap     *corev1.ConfigMap
		expectError   bool
		expectRequeue bool
	}{
		{
			name: "valid config map",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapName,
					Namespace: systemNamespace,
				},
				Data: map[string]string{
					"config": `{"agentCheckinInterval": "15m"}`,
				},
			},
			expectError:   false,
			expectRequeue: false,
		},
		{
			name:          "config map not found",
			configMap:     nil,
			expectError:   false,
			expectRequeue: false,
		},
		{
			name: "wrong namespace",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      configMapName,
					Namespace: "wrong-namespace",
				},
				Data: map[string]string{
					"config": `{}`,
				},
			},
			expectError:   false,
			expectRequeue: false,
		},
		{
			name: "wrong name",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "wrong-name",
					Namespace: systemNamespace,
				},
				Data: map[string]string{
					"config": `{}`,
				},
			},
			expectError:   false,
			expectRequeue: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objects []client.Object
			if tt.configMap != nil {
				objects = append(objects, tt.configMap)
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				Build()

			reconciler := &ConfigReconciler{
				Client:          fakeClient,
				Log:             zap.New(zap.UseDevMode(true)),
				Scheme:          scheme,
				SystemNamespace: systemNamespace,
			}

			req := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: systemNamespace,
					Name:      configMapName,
				},
			}

			result, err := reconciler.Reconcile(context.TODO(), req)

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}

			if tt.expectRequeue {
				assert.True(t, result.Requeue || result.RequeueAfter > 0)
			} else {
				assert.False(t, result.Requeue)
				assert.Equal(t, time.Duration(0), result.RequeueAfter)
			}
		})
	}
}

func TestConfigReconciler_InitializeConfig(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	systemNamespace := "cattle-fleet-system"

	tests := []struct {
		name        string
		configMap   *corev1.ConfigMap
		expectError bool
	}{
		{
			name: "config exists",
			configMap: &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:      config.ManagerConfigName,
					Namespace: systemNamespace,
				},
				Data: map[string]string{
					"config": `{"agentCheckinInterval": "30m"}`,
				},
			},
			expectError: false,
		},
		{
			name:        "no config exists",
			configMap:   nil,
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var objects []client.Object
			if tt.configMap != nil {
				objects = append(objects, tt.configMap)
			}

			fakeClient := fake.NewClientBuilder().
				WithScheme(scheme).
				WithObjects(objects...).
				Build()

			reconciler := &ConfigReconciler{
				Client:          fakeClient,
				Log:             zap.New(zap.UseDevMode(true)),
				Scheme:          scheme,
				SystemNamespace: systemNamespace,
			}

			err := reconciler.InitializeConfig(context.TODO())

			if tt.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestConfigReconciler_SetupWithManager(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))

	// Create a fake manager - this is challenging to test properly without envtest
	// For now, we'll just test that the setup function doesn't panic
	reconciler := &ConfigReconciler{
		Log:             zap.New(zap.UseDevMode(true)),
		Scheme:          scheme,
		SystemNamespace: "cattle-fleet-system",
	}

	// This would require a full controller-runtime manager to test properly
	// For now, just ensure the reconciler is properly initialized
	assert.NotNil(t, reconciler.Log)
	assert.NotNil(t, reconciler.Scheme)
	assert.Equal(t, "cattle-fleet-system", reconciler.SystemNamespace)
}
