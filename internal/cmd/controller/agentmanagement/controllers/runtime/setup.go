package runtime

import (
	"context"

	ctrl "sigs.k8s.io/controller-runtime"
)

// SetupControllers sets up all controller-runtime based agentmanagement controllers
func SetupControllers(ctx context.Context, mgr ctrl.Manager, systemNamespace string, disableBootstrap bool) error {
	logger := ctrl.Log.WithName("agentmanagement")

	// Setup config controller
	configReconciler := &ConfigReconciler{
		Client:          mgr.GetClient(),
		Log:             logger.WithName("config"),
		Scheme:          mgr.GetScheme(),
		SystemNamespace: systemNamespace,
	}

	if err := configReconciler.SetupWithManager(mgr); err != nil {
		return err
	}

	// Initialize configuration on startup
	if err := configReconciler.InitializeConfig(ctx); err != nil {
		return err
	}

	// Setup manageagent controllers
	manageAgentReconciler := &ManageAgentReconciler{
		Client:          mgr.GetClient(),
		Log:             logger.WithName("manageagent"),
		Scheme:          mgr.GetScheme(),
		SystemNamespace: systemNamespace,
	}

	if err := manageAgentReconciler.SetupWithManager(mgr); err != nil {
		return err
	}

	clusterReconciler := &ClusterReconciler{
		Client:          mgr.GetClient(),
		Log:             logger.WithName("cluster"),
		Scheme:          mgr.GetScheme(),
		SystemNamespace: systemNamespace,
	}

	if err := clusterReconciler.SetupWithManager(mgr); err != nil {
		return err
	}

	logger.Info("Controller-runtime agentmanagement controllers registered successfully")
	return nil
}
