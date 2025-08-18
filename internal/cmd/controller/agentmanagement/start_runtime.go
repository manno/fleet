package agentmanagement

import (
	"context"
	"os"
	"strconv"

	runtimecontrollers "github.com/rancher/fleet/internal/cmd/controller/agentmanagement/controllers/runtime"
	fleet "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"

	"github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	clog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

// useControllerRuntime determines whether to use controller-runtime or wrangler
func useControllerRuntime() bool {
	env := os.Getenv("FLEET_USE_CONTROLLER_RUNTIME_AGENTMANAGEMENT")
	if env == "" {
		return false // Default to wrangler for now
	}

	enabled, err := strconv.ParseBool(env)
	if err != nil {
		logrus.Warnf("Invalid value for FLEET_USE_CONTROLLER_RUNTIME_AGENTMANAGEMENT: %s, defaulting to false", env)
		return false
	}

	return enabled
}

func startControllerRuntime(ctx context.Context, kubeConfig, namespace string, disableBootstrap bool) error {
	// Setup logging
	clog.SetLogger(zap.New(zap.UseDevMode(true)))
	logger := ctrl.Log.WithName("agentmanagement")

	// Get the kubeconfig
	config, err := ctrl.GetConfig()
	if err != nil {
		return err
	}

	// Create manager
	mgr, err := ctrl.NewManager(config, ctrl.Options{
		Scheme: GetScheme(),
		Metrics: metricsserver.Options{
			BindAddress: ":8080",
		},
		HealthProbeBindAddress:  ":8081",
		LeaderElection:          true,
		LeaderElectionID:        "fleet-agentmanagement-lock",
		LeaderElectionConfig:    config,
		LeaderElectionNamespace: namespace,
	})
	if err != nil {
		return err
	}

	// Setup health checks
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return err
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return err
	}

	// Setup controllers
	if err := runtimecontrollers.SetupControllers(ctx, mgr, namespace, disableBootstrap); err != nil {
		return err
	}

	logger.Info("Starting Fleet agentmanagement manager with controller-runtime")
	return mgr.Start(ctx)
}

// GetScheme returns the runtime scheme with all required types
func GetScheme() *runtime.Scheme {
	scheme := runtime.NewScheme()

	// Add core types
	_ = corev1.AddToScheme(scheme)
	_ = rbacv1.AddToScheme(scheme)

	// Add Fleet types
	_ = fleet.AddToScheme(scheme)

	return scheme
}
