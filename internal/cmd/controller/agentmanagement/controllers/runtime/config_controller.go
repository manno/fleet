// Package runtime provides controller-runtime based implementations of agentmanagement controllers
package runtime

import (
	"context"

	"github.com/go-logr/logr"
	"github.com/rancher/fleet/internal/config"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// ConfigReconciler reconciles Fleet configuration changes from ConfigMaps
type ConfigReconciler struct {
	client.Client
	Log             logr.Logger
	Scheme          *runtime.Scheme
	SystemNamespace string
}

// Reconcile handles Fleet configuration updates
func (r *ConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("configmap", req.NamespacedName)

	var configMap corev1.ConfigMap
	if err := r.Get(ctx, req.NamespacedName, &configMap); err != nil {
		if client.IgnoreNotFound(err) == nil {
			// ConfigMap was deleted - reset to default config
			logger.Info("Config ConfigMap deleted, resetting to defaults")
			return ctrl.Result{}, r.resetToDefaults()
		}
		return ctrl.Result{}, err
	}

	// Only process the fleet-controller config in the system namespace
	if configMap.Name != config.ManagerConfigName || configMap.Namespace != r.SystemNamespace {
		return ctrl.Result{}, nil
	}

	logger.Info("Processing Fleet configuration update")

	// Read and apply the new configuration
	cfg, err := config.ReadConfig(&configMap)
	if err != nil {
		logger.Error(err, "Failed to read config from ConfigMap")
		return ctrl.Result{}, err
	}

	if err := config.SetAndTrigger(cfg); err != nil {
		logger.Error(err, "Failed to apply configuration")
		return ctrl.Result{}, err
	}

	logger.Info("Successfully applied Fleet configuration")
	return ctrl.Result{}, nil
}

// resetToDefaults resets the Fleet configuration to defaults when the ConfigMap is deleted
func (r *ConfigReconciler) resetToDefaults() error {
	// Load default configuration
	cfg := &config.Config{}
	return config.SetAndTrigger(cfg)
}

// SetupWithManager sets up the controller with the Manager
func (r *ConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	// Create a predicate to only watch the fleet-controller ConfigMap in the system namespace
	configMapPredicate := predicate.NewPredicateFuncs(func(object client.Object) bool {
		return object.GetName() == config.ManagerConfigName && object.GetNamespace() == r.SystemNamespace
	})

	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.ConfigMap{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: 1, // Config changes should be processed serially
		}).
		WithEventFilter(configMapPredicate).
		Complete(r)
}

// InitializeConfig performs initial configuration loading, similar to the original Register function
func (r *ConfigReconciler) InitializeConfig(ctx context.Context) error {
	logger := r.Log.WithValues("action", "initialize")

	// Look up the existing config
	cfg, err := r.lookupConfig(ctx)
	if err != nil {
		logger.Error(err, "Failed to lookup initial configuration")
		return err
	}

	logger.Info("Initializing Fleet configuration")
	return config.SetAndTrigger(cfg)
}

// lookupConfig retrieves the current configuration from the cluster
func (r *ConfigReconciler) lookupConfig(ctx context.Context) (*config.Config, error) {
	var configMap corev1.ConfigMap
	key := client.ObjectKey{
		Name:      config.ManagerConfigName,
		Namespace: r.SystemNamespace,
	}

	if err := r.Get(ctx, key, &configMap); err != nil {
		if client.IgnoreNotFound(err) == nil {
			// No config found, return default config
			return &config.Config{}, nil
		}
		return nil, err
	}

	return config.ReadConfig(&configMap)
}
