package benchmarks_test

import (
	"context"
	"encoding/hex"
	"fmt"
	"math/rand"
	"os"
	"path"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gm "github.com/onsi/gomega/gmeasure"

	"github.com/rancher/fleet/e2e/testenv/kubectl"
	"github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"

	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// cannot use v1alpha1.RepoLabel because fleet 0.9 deletes bundles with an invalid repo label.
// However, bundle labels are propagated to bundledeployments.
const GroupLabel = "fleet.cattle.io/bench-group"

var (
	ctx    context.Context
	cancel context.CancelFunc

	k8sClient client.Client
	k         kubectl.Command

	root   = ".."
	scheme = apiruntime.NewScheme()
	s      = rand.New(rand.NewSource(GinkgoRandomSeed()))

	// experiments
	name       string
	info       string
	experiment *gm.Experiment

	// cluster registration namespace, contains clusters
	workspace string
)

// TestBenchmarkSuite runs the benchmark suite for Fleet.
//
// Inputs for this benchmark suite via env vars:
// * cluster registration namespace, contains clusters
// * timeout for eventually
func TestBenchmarkSuite(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Fleet Benchmark Suite")
}

// this will run after BeforeEach, but before the actual experiment
var _ = JustBeforeEach(func() {
	experiment = gm.NewExperiment(name)
	AddReportEntry(experiment.Name, experiment)
	experiment.RecordNote(info, gm.Style("{{green}}"), gm.Annotation("info"))
	recordMemoryUsage(experiment, "MemBefore")
	recordMetrics(experiment, "Before")
	recordResourceCount(experiment, "ResourceCountBefore")
})

// this will run after DeferClean, so clean up is not included in the measurements
var _ = AfterEach(func() {
	recordMemoryUsage(experiment, "MemAfter")
	recordMetrics(experiment, "After")
	recordResourceCount(experiment, "ResourceCountAfter")
})

var _ = BeforeSuite(func() {
	tm := os.Getenv("FLEET_BENCH_TIMEOUT")
	if tm == "" {
		tm = "2m"
	}
	dur, err := time.ParseDuration(tm)
	Expect(err).NotTo(HaveOccurred(), "failed to parse timeout duration: "+tm)
	SetDefaultEventuallyTimeout(dur)
	SetDefaultEventuallyPollingInterval(1 * time.Second)

	ctx, cancel = context.WithCancel(context.TODO())

	workspace = os.Getenv("FLEET_BENCH_NAMESPACE")

	// client for assets
	k = kubectl.New("", workspace)

	// client for assertions
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(v1alpha1.AddToScheme(scheme))
	utilruntime.Must(apiextv1.AddToScheme(scheme))

	cfg := ctrl.GetConfigOrDie()

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme, Cache: nil})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// describe the e this suite is running against
	e := gm.NewExperiment("before")
	recordMetrics(e, "")
	recordMemoryUsage(e, "MemBefore")
	recordResourceCount(e, "ResourceCountBefore")
	recordCRDCount(e, "CRDCount")
	recordNodes(e)
	recordClusters(e)

	version, err := k.Run("version")
	Expect(err).NotTo(HaveOccurred())
	e.RecordNote(version, gm.Annotation("Kubernetes Version"))
	AddReportEntry("setup", e)
})

var _ = AfterSuite(func() {
	e := gm.NewExperiment("after")
	recordMemoryUsage(e, "MemAfter")
	recordResourceCount(e, "ResourceCountAfter")
	AddReportEntry("setup", e)

	cancel()
})

// atLeastOneClusterReady validates that the workspace has at least one cluster.
// All clusters need at least one ready bundle. Assuming that bundle is the agent.
func atLeastOneClusterReady() {
	GinkgoHelper()

	list := &v1alpha1.ClusterList{}
	err := k8sClient.List(ctx, list, client.InNamespace(workspace))
	Expect(err).ToNot(HaveOccurred(), "failed to list clusters")
	Expect(len(list.Items)).To(BeNumerically(">=", 1))
	for _, cluster := range list.Items {
		Expect(cluster.Status.Summary.Ready).To(BeNumerically(">=", 1), "expected one ready bundle in cluster")
	}
}

// AssetPath returns the path to an asset
func AssetPath(p ...string) string {
	parts := append([]string{root, "benchmarks", "assets"}, p...)
	return path.Join(parts...)
}

// AddRandomSuffix adds a random suffix to a given name.
func AddRandomSuffix(name string, s rand.Source) string {
	p := make([]byte, 6)
	r := rand.New(s) // nolint:gosec // non-crypto usage
	_, err := r.Read(p)
	if err != nil {
		panic(err)
	}
	return fmt.Sprintf("%s-%s", name, hex.EncodeToString(p))
}
