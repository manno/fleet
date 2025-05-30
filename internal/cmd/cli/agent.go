package cli

import (
	"flag"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	command "github.com/rancher/fleet/internal/cmd"
	"github.com/rancher/fleet/internal/cmd/cli/agent"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

const (
	DefaultName = "fleet-agent"

	Kubeconfig          = "kubeconfig"
	DeploymentNamespace = "deploymentNamespace"
	ClusterNamespace    = "clusterNamespace"
	ClusterName         = "clusterName"
)

type Agent struct {
}

func (ag *Agent) Run(cmd *cobra.Command, args []string) error {
	return cmd.Help()
}

func NewAgent() *cobra.Command {
	cmd := command.Command(&Agent{}, cobra.Command{
		Short:         "",
		SilenceErrors: true,
		SilenceUsage:  true,
	})

	cmd.AddCommand(NewAgentRegister())
	cmd.AddCommand(NewDeleteAgent())
	return cmd
}

func NewAgentRegister() *cobra.Command {
	cmd := command.Command(&Register{}, cobra.Command{
		Short:         "Register and deploy the agent",
		SilenceErrors: true,
		SilenceUsage:  true,
	})

	cmd.SetOut(os.Stdout)
	// add command line flags from zap and controller-runtime, which use
	// goflags and convert them to pflags
	fs := flag.NewFlagSet("", flag.ExitOnError)
	zopts.BindFlags(fs)
	// upstream cluster kubeconfig flags
	ctrl.RegisterFlags(fs)
	cmd.Flags().AddGoFlagSet(fs)
	return cmd
}

func NewDeleteAgent() *cobra.Command {
	cmd := command.Command(&Delete{}, cobra.Command{
		Short:         "Delete the agent",
		SilenceErrors: true,
		SilenceUsage:  true,
	})

	cmd.SetOut(os.Stdout)
	// add command line flags from zap and controller-runtime, which use
	// goflags and convert them to pflags
	fs := flag.NewFlagSet("", flag.ExitOnError)
	zopts.BindFlags(fs)
	// upstream cluster kubeconfig flags
	ctrl.RegisterFlags(fs)
	cmd.Flags().AddGoFlagSet(fs)
	return cmd
}

type UpstreamFlags struct {
	agent.ClusterOpts
	//KubeconfigFile   string `usage:"Path to file containing an admin kubeconfig for the downstream cluster" short:"k"`
	KubeconfigSecretName string `usage:"Existing secret for the admin kubeconfig to the downstream cluster" short:"s"`
}

type DownstreamFlags struct {
	// SystemNamespace is the namespace, the agent runs in, e.g. cattle-fleet-local-system
	SystemNamespace string `usage:"System namespace of the downstream cluster" short:"d" default:"cattle-fleet-system"`
}

type Register struct {
	UpstreamFlags
	DownstreamFlags
}

func (a *Register) Run(cmd *cobra.Command, args []string) error {
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zopts)))
	ctx := log.IntoContext(cmd.Context(), ctrl.Log)

	if a.ClusterName == "" {
		return cmd.Help()
	}

	if a.KubeconfigSecretName == "" {
		return cmd.Help()
	}

	fmt.Printf("%#v\n", a)

	cfg := ctrl.GetConfigOrDie()
	ucl, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return err
	}

	// TODO public API for rancher controllers
	// register on upstream - pkg/agent/register.go
	// deploy to downstream - pkg/agent/deploy.go
	opts := agent.AgentDeploymentOpts{
		KubeconfigSecretName: a.KubeconfigSecretName,
		SystemNamespace:      a.SystemNamespace,
	}
	if err := agent.RegisterOnUpstream(ctx, ucl, a.ClusterOpts, opts); err != nil {
		return err
	}

	if err := agent.Deploy(ctx, ucl, cfg, a.ClusterOpts, opts); err != nil {
		return err
	}

	return nil
}

type Delete struct {
	UpstreamFlags
	DownstreamFlags
}

func (ad *Delete) Run(cmd *cobra.Command, args []string) error {
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&zopts)))
	ctx := log.IntoContext(cmd.Context(), ctrl.Log)

	cfg := ctrl.GetConfigOrDie()
	cl, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return err
	}

	return agent.Delete(ctx, cl, ad.ClusterOpts)

}
