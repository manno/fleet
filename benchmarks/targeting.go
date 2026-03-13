package benchmarks

import (
	"fmt"
	"sort"

	"github.com/rancher/fleet/benchmarks/record"
	"github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gm "github.com/onsi/gomega/gmeasure"
)

const (
	manyBundleBenchmarkCount            = 500
	manyBundleBenchmarkUpdates          = 3
	manyBundleBenchmarkMaxTargetCluster = 2
)

type bundleManifestData struct {
	Max          int
	Group        string
	Revision     int
	ClusterNames []string
}

func selectBenchmarkClusterNames(clusters []v1alpha1.Cluster, max int) []string {
	names := make([]string, 0, len(clusters))
	for _, cluster := range clusters {
		names = append(names, cluster.Name)
	}

	sort.Strings(names)
	if len(names) > max {
		names = names[:max]
	}

	return names
}

func collectBundleTargetingStateForGroup(group string, expectedBundles, expectedTargets int, requireResourceHash bool) (map[string]string, error) {
	bundles := &v1alpha1.BundleList{}
	if err := k8sClient.List(ctx, bundles, client.InNamespace(workspace), client.MatchingLabels{
		GroupLabel: group,
	}); err != nil {
		return nil, err
	}

	if len(bundles.Items) != expectedBundles {
		return nil, fmt.Errorf("expected %d bundles, got %d", expectedBundles, len(bundles.Items))
	}

	for _, bundle := range bundles.Items {
		if bundle.Generation != bundle.Status.ObservedGeneration {
			return nil, fmt.Errorf("bundle %s generation %d has observed generation %d", bundle.Name, bundle.Generation, bundle.Status.ObservedGeneration)
		}
		if bundle.Status.Summary.DesiredReady != expectedTargets {
			return nil, fmt.Errorf("bundle %s desired ready %d, expected %d", bundle.Name, bundle.Status.Summary.DesiredReady, expectedTargets)
		}
		if requireResourceHash && bundle.Status.ResourcesSHA256Sum == "" {
			return nil, fmt.Errorf("bundle %s has empty resource hash", bundle.Name)
		}
	}

	deployments := &v1alpha1.BundleDeploymentList{}
	if err := k8sClient.List(ctx, deployments, client.MatchingLabels{
		GroupLabel: group,
	}); err != nil {
		return nil, err
	}

	expectedDeployments := expectedBundles * expectedTargets
	if len(deployments.Items) != expectedDeployments {
		return nil, fmt.Errorf("expected %d bundledeployments, got %d", expectedDeployments, len(deployments.Items))
	}

	deploymentIDs := make(map[string]string, len(deployments.Items))
	for _, deployment := range deployments.Items {
		if deployment.Spec.DeploymentID == "" {
			return nil, fmt.Errorf("bundledeployment %s/%s has empty deployment ID", deployment.Namespace, deployment.Name)
		}
		key := fmt.Sprintf("%s/%s", deployment.Namespace, deployment.Name)
		deploymentIDs[key] = deployment.Spec.DeploymentID
	}

	return deploymentIDs, nil
}

func collectBundleTargetingState(group string, expectedBundles, expectedTargets int) (map[string]string, error) {
	return collectBundleTargetingStateForGroup(group, expectedBundles, expectedTargets, true)
}

func deploymentIDsChanged(current, previous map[string]string) error {
	if previous == nil {
		return nil
	}

	if len(current) != len(previous) {
		return fmt.Errorf("expected %d bundledeployments after update, got %d", len(previous), len(current))
	}

	for key, previousID := range previous {
		currentID, ok := current[key]
		if !ok {
			return fmt.Errorf("bundledeployment %s missing after update", key)
		}
		if currentID == previousID {
			return fmt.Errorf("bundledeployment %s deployment ID did not change", key)
		}
	}

	return nil
}

// create-50-bundle
// create-150-bundle
// create-update-500-bundle
var _ = Context("Benchmarks Targeting", func() {
	var (
		clusters       *v1alpha1.ClusterList
		n              int
		manifest       string
		targetClusters []string
	)

	BeforeEach(func() {
		clusters = &v1alpha1.ClusterList{}
		Expect(k8sClient.List(ctx, clusters, client.InNamespace(workspace), client.MatchingLabels{
			"fleet.cattle.io/benchmark": "true",
		})).To(Succeed())
		n = len(clusters.Items)
		Expect(n).ToNot(BeZero(), "you need at least one cluster labeled with fleet.cattle.io/benchmark=true")
		targetClusters = selectBenchmarkClusterNames(clusters.Items, manyBundleBenchmarkMaxTargetCluster)
	})

	Describe("Adding 50 Bundles", Label("create-50-bundle"), func() {
		BeforeEach(func() {
			name = "create-50-bundle"
			info = "creating 50 bundles targeting each cluster"
			manifest = assetPath("create-bundle/bundles50.yaml")
			err := generateAsset(
				manifest,
				assetPath("create-bundle/bundles.tmpl.yaml"),
				struct{ Max int }{50})
			Expect(err).ToNot(HaveOccurred())
		})

		It("creates 50 bundledeployments", func() {
			DeferCleanup(func() {
				_, _ = k.Delete("-f", manifest)
			})

			experiment.MeasureDuration("TotalDuration", func() {
				record.MemoryUsage(experiment, "MemDuring")

				_, _ = k.Apply("-f", manifest)
				Eventually(func(g Gomega) {
					list := &v1alpha1.BundleDeploymentList{}
					err := k8sClient.List(ctx, list, client.MatchingLabels{
						GroupLabel: name,
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(list.Items).To(HaveLen(n * 50))
				}).WithTimeout(MediumTimeout).WithPolling(PollingInterval).Should(Succeed())
			}, gm.Style("{{bold}}"))
		})
	})

	Describe("Adding 150 Bundles", Label("create-150-bundle"), func() {
		// This test creates 150 bundles targeting all clusters. With
		// 5000 clusters this results in 750,000 BundleDeployments
		// resources, which is too much for etcd.
		BeforeEach(func() {
			name = "create-150-bundle"
			info = "creating 150 bundles targeting each cluster"
			manifest = assetPath("create-bundle/bundles150.yaml")
			err := generateAsset(
				manifest,
				assetPath("create-bundle/bundles.tmpl.yaml"),
				struct{ Max int }{150})
			Expect(err).ToNot(HaveOccurred())
		})

		It("creates 150 bundledeployments", func() {
			if n > 1000 {
				Skip("Skipping test with more than 1000 clusters, as it would create too many BundleDeployments")
			}

			DeferCleanup(func() {
				_, _ = k.Delete("-f", manifest)
			})

			experiment.MeasureDuration("TotalDuration", func() {
				record.MemoryUsage(experiment, "MemDuring")

				_, _ = k.Apply("-f", manifest)
				Eventually(func(g Gomega) {
					list := &v1alpha1.BundleDeploymentList{}
					err := k8sClient.List(ctx, list, client.MatchingLabels{
						GroupLabel: name,
					})
					g.Expect(err).ToNot(HaveOccurred())
					g.Expect(list.Items).To(HaveLen(n * 150))
				}).WithTimeout(LongTimeout).WithPolling(LongPollingInterval).Should(Succeed())
			}, gm.Style("{{bold}}"))
		})
	})

	Describe("Adding and updating 500 Bundles targeting up to two clusters", Label("create-update-500-bundle"), func() {
		BeforeEach(func() {
			name = "create-update-500-bundle"
			info = fmt.Sprintf(
				"creating %d bundles targeting up to %d benchmark clusters, then updating them %d times",
				manyBundleBenchmarkCount,
				manyBundleBenchmarkMaxTargetCluster,
				manyBundleBenchmarkUpdates,
			)
			manifest = assetPath("create-update-bundle/bundles500.yaml")
			err := generateAsset(
				manifest,
				assetPath("create-update-bundle/bundles.tmpl.yaml"),
				bundleManifestData{
					Max:          manyBundleBenchmarkCount,
					Group:        name,
					Revision:     1,
					ClusterNames: targetClusters,
				},
			)
			Expect(err).ToNot(HaveOccurred())
		})

		It("creates 500 bundles and updates them multiple times without scaling cluster fan-out", func() {
			expectedTargets := len(targetClusters)

			DeferCleanup(func() {
				_, _ = k.Delete("-f", manifest)
			})

			experiment.MeasureDuration("TotalDuration", func() {
				record.MemoryUsage(experiment, "MemDuring")

				_, err := k.Apply("-f", manifest)
				Expect(err).ToNot(HaveOccurred())

				var deploymentIDs map[string]string
				Eventually(func() error {
					current, err := collectBundleTargetingState(name, manyBundleBenchmarkCount, expectedTargets)
					if err != nil {
						return err
					}
					deploymentIDs = current
					return nil
				}).WithTimeout(LongTimeout).WithPolling(LongPollingInterval).Should(Succeed())

				for revision := 2; revision <= manyBundleBenchmarkUpdates+1; revision++ {
					previousDeploymentIDs := deploymentIDs

					err := generateAsset(
						manifest,
						assetPath("create-update-bundle/bundles.tmpl.yaml"),
						bundleManifestData{
							Max:          manyBundleBenchmarkCount,
							Group:        name,
							Revision:     revision,
							ClusterNames: targetClusters,
						},
					)
					Expect(err).ToNot(HaveOccurred())

					_, err = k.Apply("-f", manifest)
					Expect(err).ToNot(HaveOccurred())

					Eventually(func() error {
						current, err := collectBundleTargetingState(name, manyBundleBenchmarkCount, expectedTargets)
						if err != nil {
							return err
						}
						if err := deploymentIDsChanged(current, previousDeploymentIDs); err != nil {
							return err
						}
						deploymentIDs = current
						return nil
					}).WithTimeout(LongTimeout).WithPolling(LongPollingInterval).Should(Succeed())
				}
			}, gm.Style("{{bold}}"))
		})
	})
})
