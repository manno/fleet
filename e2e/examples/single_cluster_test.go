package examples_test

import (
	"time"

	"github.com/rancher/fleet/e2e/testenv"
	"github.com/rancher/fleet/e2e/testenv/kubectl"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("SingleCluster", func() {
	var (
		asset string
		k     kubectl.Command
	)

	BeforeEach(func() {
		k = env.Kubectl.Context(env.Fleet).Namespace(env.Namespace)
	})

	JustBeforeEach(func() {
		out, err := k.Apply("-f", testenv.AssetPath(asset))
		Expect(err).ToNot(HaveOccurred(), out)
	})

	AfterEach(func() {
		out, err := k.Delete("-f", testenv.AssetPath(asset))
		Expect(err).ToNot(HaveOccurred(), out)

	})

	When("creating gitrepo containing a helm chart", func() {
		BeforeEach(func() {
			asset = "single-cluster/helm/gitrepo.yaml"
		})

		It("deploys the helm in the downstream cluster", func() {
			Eventually(func() string {
				out, _ := env.Kubectl.Context(env.Downstream).Namespace("fleet-helm-example").Get("pods")
				return out
			}, 5*time.Minute).Should(ContainSubstring("frontend-"))
		})
	})
})
