// Package benchmarks is used to benchmark the performance of the controllers
// against an existing Fleet installation. Each experiment aligns to a bundle
// lifecycles. Experiments might have requirements, like the number of clusters
// in an installation. The experiments create a resource and wait for Fleet to
// reconcile it. Experiments collect multiple metrics, like the number and
// duration of reconciliations, the overall duration of the experiment, the
// number of created k8s resources and the CPU and memory usage of the
// controllers.
package benchmarks_test

import (
	gm "github.com/onsi/gomega/gmeasure"

	"github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// create-1-gitrepo-1-bundle
// create-1-gitrepo-1-big-bundle
// create-1-gitrepo-50-bundle
// create-50-gitrepo-50-bundle
var _ = Context("Benchmarks GitOps", func() {
	Describe("Adding 1 GitRepo results in 1 Bundle", Label("create-1-gitrepo-1-bundle"), func() {
		BeforeEach(func() {
			name = "create-1-gitrepo-1-bundle"
			info = `creating one bundle from one gitrepo

		This test is influenced by the network connection to the Git repository server.
		`
		})

		It("creates a Bundle", func() {
			DeferCleanup(func() {
				_, _ = k.Delete("-f", AssetPath(name, "gitrepo.yaml"))
			})

			experiment.MeasureDuration("TotalDuration", func() {
				recordMemoryUsage(experiment, "MemDuring")

				_, _ = k.Apply("-f", AssetPath(name, "gitrepo.yaml"))
				Eventually(func(g Gomega) {
					err := k8sClient.Get(ctx, client.ObjectKey{
						Namespace: workspace,
						Name:      "bm-1-gitrepo-1-bundle-bench-create-1-gitrepo-1-bundle",
					}, &v1alpha1.Bundle{})
					g.Expect(err).ToNot(HaveOccurred())
				}).Should(Succeed())
			}, gm.Style("{{yellow}}"))

		})
	})

	Describe("Adding 1 GitRepo results in 1 big Bundle", Label("create-1-gitrepo-1-big-bundle"), func() {
		BeforeEach(func() {
			name = "create-1-gitrepo-1-big-bundle"
			info = "creating one big bundle from one GitRepo"
		})

		It("creates a big bundle", func() {
			DeferCleanup(func() {
				_, _ = k.Delete("-f", AssetPath(name, "gitrepo.yaml"))
			})

			experiment.MeasureDuration("TotalDuration", func() {
				recordMemoryUsage(experiment, "MemDuring")

				_, _ = k.Apply("-f", AssetPath(name, "gitrepo.yaml"))
				Eventually(func(g Gomega) {
					err := k8sClient.Get(ctx, client.ObjectKey{
						Namespace: workspace,
						Name:      "bm-1-gitrepo-1-big-bundle-bench-create-1-gitrep-3303d",
					}, &v1alpha1.Bundle{})
					g.Expect(err).ToNot(HaveOccurred())
				}).Should(Succeed())
			})

		})
	})

	Describe("Adding 1 GitRepo results in 50 Bundles", Label("create-1-gitrepo-50-bundle"), func() {
		BeforeEach(func() {
			name = "create-1-gitrepo-50-bundle"
			info = "creating 50 bundles from one GitRepo"
		})

		It("creates 50 bundles", func() {
			DeferCleanup(func() {
				_, _ = k.Delete("-f", AssetPath(name, "gitrepo.yaml"))
			})

			experiment.MeasureDuration("TotalDuration", func() {
				recordMemoryUsage(experiment, "MemDuring")

				_, _ = k.Apply("-f", AssetPath(name, "gitrepo.yaml"))
				Eventually(func(g Gomega) {
					list := &v1alpha1.BundleList{}
					err := k8sClient.List(ctx, list, client.InNamespace(workspace), client.MatchingLabels{
						v1alpha1.RepoLabel: "bm-1-gitrepo-50-bundle",
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(len(list.Items)).To(Equal(50))
				}).Should(Succeed())
			})
		})
	})

	Describe("Adding 50 GitRepos results in 50 Bundles", Label("create-50-gitrepo-50-bundle"), func() {
		BeforeEach(func() {
			name = "create-50-gitrepo-50-bundle"
			info = "creating 50 bundles from 50 GitRepos"
		})

		It("creates 50 bundles", func() {
			DeferCleanup(func() {
				_, _ = k.Delete("-f", AssetPath(name, "gitrepos.yaml"))
			})

			experiment.MeasureDuration("TotalDuration", func() {
				recordMemoryUsage(experiment, "MemDuring")

				_, _ = k.Apply("-f", AssetPath(name, "gitrepos.yaml"))
				Eventually(func(g Gomega) {
					list := &v1alpha1.BundleList{}
					err := k8sClient.List(ctx, list, client.InNamespace(workspace), client.MatchingLabels{
						"fleet.cattle.io/group": "bm-50-gitrepo-50-bundle",
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(len(list.Items)).To(Equal(50))
				}).Should(Succeed())
			})
		})
	})
})
