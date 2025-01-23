package bundle

import (
	"os"
	"strconv"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gm "github.com/onsi/gomega/gmeasure"

	"github.com/rancher/fleet/integrationtests/utils"
	fleet "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	yaml "sigs.k8s.io/yaml"
)

const (
	clusterCount = 50
	bundleCount  = 300
)

var _ = FDescribe("Bundle targets Benchmark", Label("bench"), func() {

	var (
		bundle  *fleet.Bundle
		exp     *gm.Experiment
		timeout = 3 * time.Minute
	)

	BeforeEach(func() {
		var err error
		namespace, err = utils.NewNamespaceName()
		Expect(err).ToNot(HaveOccurred())

		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
		Expect(k8sClient.Create(ctx, ns)).ToNot(HaveOccurred())

		createClusters(namespace)

		Eventually(func() bool {
			clusters := &fleet.ClusterList{}
			err := k8sClient.List(ctx, clusters, client.InNamespace(namespace))
			Expect(err).NotTo(HaveOccurred())

			return len(clusters.Items) == clusterCount
		}).Should(BeTrue())

		clusterGroupAll, err := createClusterGroup("all", namespace, &metav1.LabelSelector{
			MatchLabels: map[string]string{"env": "test"},
		})
		Expect(err).NotTo(HaveOccurred())
		Expect(clusterGroupAll).To(Not(BeNil()))

		DeferCleanup(func() {
			Expect(k8sClient.Delete(ctx, ns)).ToNot(HaveOccurred())
		})
	})

	Describe("Target many clusters", func() {
		BeforeEach(func() {
			exp = gm.NewExperiment("create bundle")
			AddReportEntry(exp.Name, exp)

			b, err := os.ReadFile("../assets/bundle.yaml")
			Expect(err).NotTo(HaveOccurred())
			bundle = &fleet.Bundle{}
			err = yaml.Unmarshal(b, bundle)
			Expect(err).NotTo(HaveOccurred())
			Expect(bundle.Name).To(Not(BeEmpty()))

			bundle.Namespace = namespace
			bundle.Spec.Targets = []fleet.BundleTarget{
				{
					ClusterSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"env": "test"},
					},
					// ClusterName: "cluster0",
				},
			}
		})

		It("Targets many bundles at many clusters", func() {
			act := func(idx int) {
				bundles := make([]*fleet.Bundle, bundleCount)
				for i := 0; i < bundleCount; i++ {
					tmp := bundle.DeepCopy()
					tmp.Name = "bundle-" + strconv.Itoa(idx) + "-" + strconv.Itoa(i)
					err := k8sClient.Create(ctx, tmp)
					Expect(err).NotTo(HaveOccurred())
					bundles[i] = tmp
				}

				exp.MeasureDuration("waiting for bundle", func() {
					Eventually(func(g Gomega) {
						tmp := &fleet.Bundle{}
						for _, b := range bundles {
							err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: b.Name}, tmp)
							g.Expect(err).NotTo(HaveOccurred())
							g.Expect(tmp.Status.Summary.DesiredReady).To(Equal(clusterCount), b.Name)
						}
					}, timeout, 1*time.Second).Should(Succeed())
				})

				// for _, b := range bundles {
				// 	err := k8sClient.Delete(ctx, b)
				// 	Expect(err).NotTo(HaveOccurred())
				// }
			}
			exp.SampleDuration("targets", act, gm.SamplingConfig{N: 10})
		})

		XIt("Update targets many bundles at many clusters", func() {
			// this includes bundle create/delete in measurement
			act := func(idx int) {
				bundles := make([]*fleet.Bundle, bundleCount)
				for i := 0; i < bundleCount-10; i++ {
					tmp := bundle.DeepCopy()
					tmp.Name = "bundle" + strconv.Itoa(idx) + "-" + strconv.Itoa(i)
					err := k8sClient.Create(ctx, tmp)
					Expect(err).NotTo(HaveOccurred())
					bundles[i] = tmp
				}
				Eventually(func(g Gomega) {
					tmp := &fleet.Bundle{}
					for i := 0; i < bundleCount-10; i++ {
						b := bundles[i]
						err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: b.Name}, tmp)
						g.Expect(err).NotTo(HaveOccurred())
						g.Expect(tmp.Status.Summary.DesiredReady).To(Equal(clusterCount))
					}
				}, timeout)

				// create remaining bundles
				for i := bundleCount - 10; i < bundleCount; i++ {
					tmp := bundle.DeepCopy()
					tmp.Name = "bundle" + strconv.Itoa(idx) + "-" + strconv.Itoa(i)
					err := k8sClient.Create(ctx, tmp)
					Expect(err).NotTo(HaveOccurred())
					bundles[i] = tmp
				}
				exp.MeasureDuration("waiting for bundle after update", func() {
					Eventually(func(g Gomega) {
						tmp := &fleet.Bundle{}
						for _, b := range bundles {
							err := k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace, Name: b.Name}, tmp)
							g.Expect(err).NotTo(HaveOccurred())
							g.Expect(tmp.Status.Summary.DesiredReady).To(Equal(clusterCount), b.Name)
						}
					}, timeout, 1*time.Second).Should(Succeed())
				})

				for _, b := range bundles {
					err := k8sClient.Delete(ctx, b)
					Expect(err).NotTo(HaveOccurred())
				}
			}
			exp.SampleDuration("update targets", act, gm.SamplingConfig{N: 10})
		})
	})
})

func createClusters(namespace string) {
	base, err := utils.NewNamespaceName()
	Expect(err).ToNot(HaveOccurred())
	for i := 0; i < clusterCount; i++ {
		name := "cluster" + strconv.Itoa(i)

		clusterNs := base + "-" + name
		ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: clusterNs}}
		Expect(k8sClient.Create(ctx, ns)).ToNot(HaveOccurred())
		DeferCleanup(func() {
			Expect(k8sClient.Delete(ctx, ns)).ToNot(HaveOccurred())
		})

		c, err := utils.CreateCluster(ctx, k8sClient, name, namespace, map[string]string{
			"cluster": name, "env": "test",
		}, clusterNs)
		Expect(err).NotTo(HaveOccurred())
		Expect(c).To(Not(BeNil()))
		DeferCleanup(func() {
			Expect(k8sClient.Delete(ctx, c)).ToNot(HaveOccurred())
		})
	}

}
