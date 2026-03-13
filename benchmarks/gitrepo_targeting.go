package benchmarks

import (
	"fmt"

	"github.com/rancher/fleet/benchmarks/record"
	"github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gm "github.com/onsi/gomega/gmeasure"
)

const (
	gitRepoTargetingBenchmarkName     = "create-update-1-gitrepo-100-gitrepo-3-bundledeployment"
	gitRepoTargetingGroup             = "bm-update-1-gitrepo-100-gitrepo-3-bundledeployment"
	gitRepoTargetingChildCount        = 100
	gitRepoTargetingBundlesPerChild   = 3
	gitRepoTargetingUpdateWaves       = 3
	gitRepoTargetingMaxTargetClusters = 2
	gitRepoTargetingClusterLabel      = "fleet.cattle.io/benchmark-gitops-target-100x3"
	gitRepoTargetingLocalCluster      = "local"
	gitRepoTargetingExpectedBundles   = gitRepoTargetingChildCount * gitRepoTargetingBundlesPerChild
)

type clusterLabelState struct {
	exists bool
	value  string
}

type gitRepoManifestData struct {
	Workspace string
}

func setClusterLabel(clusterNames []string, label, value string) (map[string]clusterLabelState, error) {
	originalStates := make(map[string]clusterLabelState, len(clusterNames))

	for _, clusterName := range clusterNames {
		cluster := &v1alpha1.Cluster{}
		if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: workspace, Name: clusterName}, cluster); err != nil {
			return nil, err
		}

		orig := cluster.DeepCopy()
		if cluster.Labels == nil {
			cluster.Labels = map[string]string{}
		}

		state := clusterLabelState{}
		if currentValue, ok := cluster.Labels[label]; ok {
			state.exists = true
			state.value = currentValue
		}
		originalStates[clusterName] = state

		cluster.Labels[label] = value
		if err := k8sClient.Patch(ctx, cluster, client.MergeFrom(orig)); err != nil {
			return nil, err
		}
	}

	return originalStates, nil
}

func restoreClusterLabel(clusterNames []string, label string, states map[string]clusterLabelState) error {
	for _, clusterName := range clusterNames {
		cluster := &v1alpha1.Cluster{}
		if err := k8sClient.Get(ctx, client.ObjectKey{Namespace: workspace, Name: clusterName}, cluster); err != nil {
			return err
		}

		orig := cluster.DeepCopy()
		if cluster.Labels == nil {
			cluster.Labels = map[string]string{}
		}

		state := states[clusterName]
		if state.exists {
			cluster.Labels[label] = state.value
		} else {
			delete(cluster.Labels, label)
		}

		if err := k8sClient.Patch(ctx, cluster, client.MergeFrom(orig)); err != nil {
			return err
		}
	}

	return nil
}

func listGitReposByGroup(group string) (*v1alpha1.GitRepoList, error) {
	list := &v1alpha1.GitRepoList{}
	if err := k8sClient.List(ctx, list, client.InNamespace(workspace), client.MatchingLabels{
		"fleet.cattle.io/group": group,
	}); err != nil {
		return nil, err
	}

	return list, nil
}

func updateGitRepoForceSyncGeneration(group string, generation int64) error {
	list, err := listGitReposByGroup(group)
	if err != nil {
		return err
	}

	if len(list.Items) != gitRepoTargetingChildCount {
		return fmt.Errorf("expected %d child gitrepos before update, got %d", gitRepoTargetingChildCount, len(list.Items))
	}

	for _, gitrepo := range list.Items {
		orig := gitrepo.DeepCopy()
		gitrepo.Spec.ForceSyncGeneration = generation
		if err := k8sClient.Patch(ctx, &gitrepo, client.MergeFrom(orig)); err != nil {
			return err
		}
	}

	return nil
}

func collectGitRepoTargetingState(group string, expectedTargets int, expectedForceSync int64) (map[string]string, error) {
	list, err := listGitReposByGroup(group)
	if err != nil {
		return nil, err
	}

	if len(list.Items) != gitRepoTargetingChildCount {
		return nil, fmt.Errorf("expected %d child gitrepos, got %d", gitRepoTargetingChildCount, len(list.Items))
	}

	expectedReady := expectedTargets * gitRepoTargetingBundlesPerChild
	for _, gitrepo := range list.Items {
		if gitrepo.Generation != gitrepo.Status.ObservedGeneration {
			return nil, fmt.Errorf("gitrepo %s generation %d has observed generation %d", gitrepo.Name, gitrepo.Generation, gitrepo.Status.ObservedGeneration)
		}
		if gitrepo.Status.UpdateGeneration != expectedForceSync {
			return nil, fmt.Errorf("gitrepo %s update generation %d, expected %d", gitrepo.Name, gitrepo.Status.UpdateGeneration, expectedForceSync)
		}
		if gitrepo.Status.Summary.DesiredReady != expectedReady {
			return nil, fmt.Errorf("gitrepo %s desired ready %d, expected %d", gitrepo.Name, gitrepo.Status.Summary.DesiredReady, expectedReady)
		}
	}

	return collectBundleTargetingStateForGroup(group, gitRepoTargetingExpectedBundles, expectedTargets, false)
}

// create-update-1-gitrepo-100-gitrepo-3-bundledeployment
var _ = Context("Benchmarks GitOps Targeting", func() {
	var (
		manifest       string
		targetClusters []string
	)

	BeforeEach(func() {
		localCluster := &v1alpha1.Cluster{}
		err := k8sClient.Get(ctx, client.ObjectKey{Namespace: workspace, Name: gitRepoTargetingLocalCluster}, localCluster)
		if apierrors.IsNotFound(err) {
			Skip("Skipping benchmark because the local cluster is not registered in the benchmark workspace")
		}
		Expect(err).ToNot(HaveOccurred())

		clusters := &v1alpha1.ClusterList{}
		Expect(k8sClient.List(ctx, clusters, client.InNamespace(workspace), client.MatchingLabels{
			BenchmarkLabel: "true",
		})).To(Succeed())
		targetClusters = selectBenchmarkClusterNames(clusters.Items, gitRepoTargetingMaxTargetClusters)
		Expect(targetClusters).ToNot(BeEmpty(), "you need at least one cluster labeled with fleet.cattle.io/benchmark=true")

		manifest = assetPath(gitRepoTargetingBenchmarkName, "gitrepo.yaml")
		err = generateAsset(
			manifest,
			assetPath(gitRepoTargetingBenchmarkName, "gitrepo.tmpl.yaml"),
			gitRepoManifestData{Workspace: workspace},
		)
		Expect(err).ToNot(HaveOccurred())
	})

	Describe("Adding 1 GitRepo creates 100 GitRepos with 3 Helm bundles each and updates their BundleDeployments", Label(gitRepoTargetingBenchmarkName), func() {
		BeforeEach(func() {
			name = gitRepoTargetingBenchmarkName
			info = fmt.Sprintf(
				"creating 100 child gitrepos from one parent gitrepo, each with %d helm bundles targeting up to %d benchmark clusters, then forcing %d update waves",
				gitRepoTargetingBundlesPerChild,
				gitRepoTargetingMaxTargetClusters,
				gitRepoTargetingUpdateWaves,
			)
		})

		It("creates and updates child bundledeployments end to end", func() {
			originalLabelStates, err := setClusterLabel(targetClusters, gitRepoTargetingClusterLabel, "true")
			Expect(err).ToNot(HaveOccurred())

			DeferCleanup(func() {
				Expect(restoreClusterLabel(targetClusters, gitRepoTargetingClusterLabel, originalLabelStates)).To(Succeed())
			})
			DeferCleanup(func() {
				_, _ = k.Delete("-f", manifest)
			})

			expectedTargets := len(targetClusters)

			experiment.MeasureDuration("TotalDuration", func() {
				record.MemoryUsage(experiment, "MemDuring")

				_, err := k.Apply("-f", manifest)
				Expect(err).ToNot(HaveOccurred())

				var deploymentIDs map[string]string
				Eventually(func() error {
					current, err := collectGitRepoTargetingState(gitRepoTargetingGroup, expectedTargets, 0)
					if err != nil {
						return err
					}
					deploymentIDs = current
					return nil
				}).WithTimeout(LongTimeout).WithPolling(LongPollingInterval).Should(Succeed())

				for wave := 1; wave <= gitRepoTargetingUpdateWaves; wave++ {
					previousDeploymentIDs := deploymentIDs

					err := updateGitRepoForceSyncGeneration(gitRepoTargetingGroup, int64(wave))
					Expect(err).ToNot(HaveOccurred())

					Eventually(func() error {
						current, err := collectGitRepoTargetingState(gitRepoTargetingGroup, expectedTargets, int64(wave))
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
