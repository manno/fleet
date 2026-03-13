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

type deploymentUpdateState struct {
	DeploymentID        string
	AppliedDeploymentID string
	ForceSyncGeneration int64
	SyncGeneration      *int64
	Ready               bool
}

func hasLabel(obj client.Object, key, value string) bool {
	labels := obj.GetLabels()
	if labels == nil {
		return false
	}
	return labels[key] == value
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

func collectGitRepoTargetingState(group string, expectedTargets int, expectedForceSync int64) (map[string]deploymentUpdateState, error) {
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

	allBundles := &v1alpha1.BundleList{}
	if err := k8sClient.List(ctx, allBundles, client.InNamespace(workspace)); err != nil {
		return nil, err
	}

	bundles := make([]v1alpha1.Bundle, 0, len(allBundles.Items))
	for _, bundle := range allBundles.Items {
		if hasLabel(&bundle, "fleet.cattle.io/group", group) {
			bundles = append(bundles, bundle)
		}
	}

	if len(bundles) != gitRepoTargetingExpectedBundles {
		return nil, fmt.Errorf("expected %d bundles, got %d", gitRepoTargetingExpectedBundles, len(bundles))
	}

	for _, bundle := range bundles {
		if bundle.Generation != bundle.Status.ObservedGeneration {
			return nil, fmt.Errorf("bundle %s generation %d has observed generation %d", bundle.Name, bundle.Generation, bundle.Status.ObservedGeneration)
		}
		if bundle.Status.Summary.DesiredReady != expectedTargets {
			return nil, fmt.Errorf("bundle %s desired ready %d, expected %d", bundle.Name, bundle.Status.Summary.DesiredReady, expectedTargets)
		}
	}

	allDeployments := &v1alpha1.BundleDeploymentList{}
	if err := k8sClient.List(ctx, allDeployments); err != nil {
		return nil, err
	}

	deployments := make([]v1alpha1.BundleDeployment, 0, len(allDeployments.Items))
	for _, deployment := range allDeployments.Items {
		if hasLabel(&deployment, "fleet.cattle.io/group", group) {
			deployments = append(deployments, deployment)
		}
	}

	expectedDeployments := gitRepoTargetingExpectedBundles * expectedTargets
	if len(deployments) != expectedDeployments {
		return nil, fmt.Errorf("expected %d bundledeployments, got %d", expectedDeployments, len(deployments))
	}

	deploymentStates := make(map[string]deploymentUpdateState, len(deployments))
	for _, deployment := range deployments {
		if deployment.Spec.DeploymentID == "" {
			return nil, fmt.Errorf("bundledeployment %s/%s has empty deployment ID", deployment.Namespace, deployment.Name)
		}
		if deployment.Spec.Options.ForceSyncGeneration != expectedForceSync {
			return nil, fmt.Errorf(
				"bundledeployment %s/%s force sync generation %d, expected %d",
				deployment.Namespace,
				deployment.Name,
				deployment.Spec.Options.ForceSyncGeneration,
				expectedForceSync,
			)
		}
		if expectedForceSync > 0 {
			if deployment.Status.SyncGeneration == nil {
				return nil, fmt.Errorf("bundledeployment %s/%s has nil sync generation", deployment.Namespace, deployment.Name)
			}
			if *deployment.Status.SyncGeneration != expectedForceSync {
				return nil, fmt.Errorf(
					"bundledeployment %s/%s sync generation %d, expected %d",
					deployment.Namespace,
					deployment.Name,
					*deployment.Status.SyncGeneration,
					expectedForceSync,
				)
			}
		}
		if deployment.Status.AppliedDeploymentID != deployment.Spec.DeploymentID {
			return nil, fmt.Errorf(
				"bundledeployment %s/%s applied deployment ID %q does not match spec deployment ID %q",
				deployment.Namespace,
				deployment.Name,
				deployment.Status.AppliedDeploymentID,
				deployment.Spec.DeploymentID,
			)
		}
		if !deployment.Status.Ready {
			return nil, fmt.Errorf("bundledeployment %s/%s is not ready", deployment.Namespace, deployment.Name)
		}
		key := fmt.Sprintf("%s/%s", deployment.Namespace, deployment.Name)
		deploymentStates[key] = deploymentUpdateState{
			DeploymentID:        deployment.Spec.DeploymentID,
			AppliedDeploymentID: deployment.Status.AppliedDeploymentID,
			ForceSyncGeneration: deployment.Spec.Options.ForceSyncGeneration,
			SyncGeneration:      deployment.Status.SyncGeneration,
			Ready:               deployment.Status.Ready,
		}
	}

	return deploymentStates, nil
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

				Eventually(func() error {
					_, err := collectGitRepoTargetingState(gitRepoTargetingGroup, expectedTargets, 0)
					return err
				}).WithTimeout(LongTimeout).WithPolling(LongPollingInterval).Should(Succeed())

				for wave := 1; wave <= gitRepoTargetingUpdateWaves; wave++ {
					err := updateGitRepoForceSyncGeneration(gitRepoTargetingGroup, int64(wave))
					Expect(err).ToNot(HaveOccurred())

					Eventually(func() error {
						_, err := collectGitRepoTargetingState(gitRepoTargetingGroup, expectedTargets, int64(wave))
						return err
					}).WithTimeout(LongTimeout).WithPolling(LongPollingInterval).Should(Succeed())
				}
			}, gm.Style("{{bold}}"))
		})
	})
})
