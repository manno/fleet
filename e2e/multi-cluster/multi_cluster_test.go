package examples_test

import (
	"github.com/rancher/fleet/e2e/testenv"
	"github.com/rancher/fleet/e2e/testenv/kubectl"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("multiCluster", func() {
	var (
		asset string
		k     kubectl.Command
		kd    kubectl.Command
	)

	BeforeEach(func() {
		k = env.Kubectl.Context(env.Fleet).Namespace(env.Namespace)
		kd = env.Kubectl.Context(env.Downstream)
	})

	JustBeforeEach(func() {
		out, err := k.Apply("-f", testenv.AssetPath(asset))
		Expect(err).ToNot(HaveOccurred(), out)
	})

	AfterEach(func() {
		out, err := k.Delete("-f", testenv.AssetPath(asset))
		Expect(err).ToNot(HaveOccurred(), out)

	})

	When("creating gitrepo resource", func() {
		Context("containing a helm chart", func() {
			BeforeEach(func() {
				asset = "multi-cluster/helm.yaml"
			})

			It("deploys the helm chart in the downstream cluster", func() {
				Eventually(func() string {
					out, _ := kd.Namespace("fleet-mc-helm-example").Get("pods")
					return out
				}, testenv.Timeout).Should(SatisfyAll(ContainSubstring("frontend-"), ContainSubstring("redis-")))
			})
		})

		Context("containing a manifest", func() {
			BeforeEach(func() {
				asset = "multi-cluster/manifests.yaml"
			})

			It("deploys the manifest", func() {
				Eventually(func() string {
					out, _ := kd.Namespace("fleet-mc-manifest-example").Get("pods")
					return out
				}, testenv.Timeout).Should(SatisfyAll(ContainSubstring("frontend-"), ContainSubstring("redis-")))
			})
		})

		Context("containing a kustomize manifest", func() {
			BeforeEach(func() {
				asset = "multi-cluster/kustomize.yaml"
			})

			It("runs kustomize and deploys the manifest", func() {
				Eventually(func() string {
					out, _ := kd.Namespace("fleet-mc-kustomize-example").Get("pods")
					return out
				}, testenv.Timeout).Should(ContainSubstring("frontend-"))
			})
		})

		Context("containing an kustomized helm chart", func() {
			BeforeEach(func() {
				asset = "multi-cluster/helm-kustomize.yaml"
			})

			It("deploys the helm chart", func() {
				Eventually(func() string {
					out, _ := kd.Namespace("fleet-mc-helm-kustomize-example").Get("pods")
					return out
				}, testenv.Timeout).Should(ContainSubstring("frontend-"))
			})
		})

		Context("containing an external helm chart", func() {
			BeforeEach(func() {
				asset = "multi-cluster/helm-external.yaml"
			})

			It("deploys the helm chart", func() {
				Eventually(func() string {
					out, _ := kd.Namespace("fleet-mc-helm-external-example").Get("pods")
					return out
				}, testenv.Timeout).Should(ContainSubstring("frontend-"))
			})
		})

	})
})
