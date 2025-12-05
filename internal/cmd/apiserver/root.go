// Package apiserver provides the Fleet API aggregation server.
package apiserver

import (
	"fmt"

	"github.com/spf13/cobra"
	ctrl "sigs.k8s.io/controller-runtime"
	clog "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	command "github.com/rancher/fleet/internal/cmd"
	"github.com/rancher/fleet/pkg/version"
)

type FleetAPIServer struct {
	command.DebugConfig
	SecurePort               int    `usage:"HTTPS port for serving API" default:"8443" name:"secure-port"`
	CertDir                  string `usage:"Directory for TLS certificates" default:"/var/run/fleet-apiserver/serving-cert" name:"cert-dir"`
	DBPath                   string `usage:"Path to SQLite database file" default:"/var/lib/fleet-apiserver/bundledeployments.db" name:"db-path"`
	AuthenticationKubeconfig string `usage:"Kubeconfig for authentication delegation" name:"authentication-kubeconfig"`
	AuthorizationKubeconfig  string `usage:"Kubeconfig for authorization delegation" name:"authorization-kubeconfig"`
}

var (
	setupLog = ctrl.Log.WithName("setup")
	zopts    = &zap.Options{
		Development: true,
	}
)

func (f *FleetAPIServer) PersistentPre(_ *cobra.Command, _ []string) error {
	if err := f.SetupDebug(); err != nil {
		return fmt.Errorf("failed to setup debug logging: %w", err)
	}
	zopts = f.OverrideZapOpts(zopts)

	return nil
}

func (f *FleetAPIServer) Run(cmd *cobra.Command, args []string) error {
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(zopts)))
	ctx := clog.IntoContext(cmd.Context(), ctrl.Log)

	if err := run(ctx, f); err != nil {
		return err
	}

	<-cmd.Context().Done()
	return nil
}

func App() *cobra.Command {
	root := command.Command(&FleetAPIServer{}, cobra.Command{
		Version: version.FriendlyVersion(),
		Use:     "fleetapiserver",
		Short:   "Fleet API Aggregation Server",
		Long:    "API aggregation server for Fleet BundleDeployment resources with SQLite storage backend",
	})

	return root
}
