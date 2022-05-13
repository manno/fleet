// Package testenv contains common helpers for tests
package testenv

import (
	"path"

	"github.com/rancher/fleet/e2e/testenv/kubectl"
)

const (
	Fleet      = "k3d-k3s-default"
	Downstream = "k3d-k3s-second"
)

var root = "../.."

// SetRoot set the root path for the other relative paths, e.g. AssetPath.
// Usually set to point to the repositories root.
func SetRoot(dir string) {
	root = dir
}

// Root returns the relative path to the repositories root
func Root() string {
	return root
}

// AssetPath returns the path to an asset
func AssetPath(p ...string) string {
	parts := append([]string{root, "e2e", "assets"}, p...)
	return path.Join(parts...)
}

// ExamplePath returns the path to the fleet examples
func ExamplePath(p ...string) string {
	parts := append([]string{root, "fleet-examples"}, p...)
	return path.Join(parts...)
}

type Env struct {
	Kubectl kubectl.Command
}

func New() Env {
	return Env{Kubectl: kubectl.New("k3d-k3s-default", "default")}
}
