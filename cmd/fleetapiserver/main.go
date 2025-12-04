// Package main provides the entrypoint for the fleet-apiserver binary.
package main

import (
	_ "net/http/pprof"

	"github.com/rancher/wrangler/v3/pkg/signals"
	"github.com/sirupsen/logrus"

	"github.com/rancher/fleet/internal/cmd/apiserver"
)

func main() {
	ctx := signals.SetupSignalContext()
	cmd := apiserver.App()
	if err := cmd.ExecuteContext(ctx); err != nil {
		logrus.Fatal(err)
	}
}
