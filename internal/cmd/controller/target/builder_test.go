package target_test

import (
	"context"
	"os"
	"strconv"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gm "github.com/onsi/gomega/gmeasure"
	"gopkg.in/yaml.v2"

	"github.com/rancher/fleet/internal/cmd/controller/target"
	fleet "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

var _ = Describe("Targets", func() {

	var (
		client       client.WithWatch
		mgr          *target.Manager
		exp          *gm.Experiment
		clusterCount = 500
	)

	newCluster := func(name string) *fleet.Cluster {
		return &fleet.Cluster{
			ObjectMeta: v1.ObjectMeta{
				Name:      name,
				Namespace: "fleet-default",
				Labels:    map[string]string{"env": "dev"},
			},
			Status: fleet.ClusterStatus{
				Namespace: "fleet-cluster-namespace-for-" + name,
			},
		}
	}

	newBD := func(bundleName string, clusterName string) *fleet.BundleDeployment {
		return &fleet.BundleDeployment{
			ObjectMeta: v1.ObjectMeta{
				Name:      "bd",
				Namespace: "fleet-cluster-namespace-for-" + clusterName,
				Labels: map[string]string{
					fleet.BundleLabel:          bundleName,
					fleet.BundleNamespaceLabel: "fleet-default",
				},
			},
		}
	}

	BeforeEach(func() {
		objs := []runtime.Object{}
		for i := 0; i < clusterCount; i++ {
			objs = append(objs, newCluster("cluster"+strconv.Itoa(i)))
		}
		for i := 0; i < clusterCount-1; i++ {
			objs = append(objs, newBD("bundle123", "cluster"+strconv.Itoa(i)))
		}

		scheme := runtime.NewScheme()
		utilruntime.Must(fleet.AddToScheme(scheme))
		utilruntime.Must(clientgoscheme.AddToScheme(scheme))
		client = fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()

		mgr = target.New(client)

		exp = gm.NewExperiment("create bundle")
		AddReportEntry(exp.Name, exp)
	})

	It("matching many clusters", func() {
		bundle := &fleet.Bundle{
			ObjectMeta: v1.ObjectMeta{
				Name:      "bundle123",
				Namespace: "fleet-default",
			},
			Spec: fleet.BundleSpec{
				Targets: []fleet.BundleTarget{
					{
						ClusterSelector: &v1.LabelSelector{
							MatchLabels: map[string]string{"env": "dev"},
						},
						// ClusterName: "cluster0",
					},
				},
			},
		}

		b, err := os.ReadFile("../../../../integrationtests/controller/assets/bundle.yaml")
		Expect(err).NotTo(HaveOccurred())
		tmp := &fleet.Bundle{}
		err = yaml.Unmarshal(b, tmp)
		Expect(err).NotTo(HaveOccurred())
		bundle.Spec.Resources = tmp.Spec.Resources

		act := func(idx int) {
			targets, err := mgr.Targets(context.TODO(), bundle, "manifestID")
			Expect(err).NotTo(HaveOccurred())
			Expect(targets).To(HaveLen(clusterCount))
			// for _, t := range targets {
			// 	pretty, _ := json.MarshalIndent(t, "", "\t")
			// 	fmt.Println(string(pretty))
			// }
		}
		exp.SampleDuration("targets", act, gm.SamplingConfig{N: 300})
	})
})
