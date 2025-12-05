package apiserver

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	fleetv1alpha1 "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	storagev1alpha1 "github.com/rancher/fleet/pkg/apis/storage.fleet.cattle.io/v1alpha1"
)

var _ = Describe("Full API Aggregation Integration", Ordered, func() {
	var (
		testCtx       context.Context
		testNamespace string
	)

	BeforeEach(func() {
		testCtx = context.Background()
		testNamespace = fmt.Sprintf("test-%d", time.Now().UnixNano())

		// Create test namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		}
		err := k8sclient.Create(testCtx, ns)
		Expect(err).NotTo(HaveOccurred())
	})

	AfterEach(func() {
		// Clean up test namespace
		ns := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: testNamespace,
			},
		}
		_ = k8sclient.Delete(testCtx, ns)
	})

	Describe("BundleDeployment through API Aggregation", func() {
		It("should create a BundleDeployment through kube-apiserver", func() {
			bd := &storagev1alpha1.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-bd-aggregated",
					Namespace: testNamespace,
					Labels: map[string]string{
						"test": "aggregation",
					},
				},
				Spec: fleetv1alpha1.BundleDeploymentSpec{
					DeploymentID: "aggregation-test-id",
					Options: fleetv1alpha1.BundleDeploymentOptions{
						DefaultNamespace: "default",
					},
				},
			}

			// This goes through: k8sClient → envtest apiserver → APIService → our apiserver → storage
			err := k8sclient.Create(testCtx, bd)
			Expect(err).NotTo(HaveOccurred())

			// Verify it was created and has proper metadata
			// Note: TypeMeta (Kind/APIVersion) are typically stripped by k8s clients
			Expect(bd.ResourceVersion).NotTo(BeEmpty())
			Expect(bd.UID).NotTo(BeEmpty())
			Expect(bd.Namespace).To(Equal(testNamespace))
			Expect(bd.Name).To(Equal("test-bd-aggregated"))

			GinkgoWriter.Printf("✅ Created BundleDeployment through API aggregation: %s/%s\n",
				bd.Namespace, bd.Name)
		})

		It("should get a BundleDeployment through kube-apiserver", func() {
			// Create
			bd := &storagev1alpha1.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-get-aggregated",
					Namespace: testNamespace,
				},
				Spec: fleetv1alpha1.BundleDeploymentSpec{
					DeploymentID: "get-test-id",
				},
			}
			err := k8sclient.Create(testCtx, bd)
			Expect(err).NotTo(HaveOccurred())

			// Get it back
			retrieved := &storagev1alpha1.BundleDeployment{}
			err = k8sclient.Get(testCtx, client.ObjectKey{
				Namespace: testNamespace,
				Name:      "test-get-aggregated",
			}, retrieved)
			Expect(err).NotTo(HaveOccurred())

			// Verify data
			Expect(retrieved.Name).To(Equal("test-get-aggregated"))
			Expect(retrieved.Namespace).To(Equal(testNamespace))
			Expect(retrieved.Spec.DeploymentID).To(Equal("get-test-id"))
			Expect(retrieved.ResourceVersion).NotTo(BeEmpty())

			GinkgoWriter.Printf("✅ Retrieved BundleDeployment through API aggregation\n")
		})

		It("should update a BundleDeployment through kube-apiserver", func() {
			// Create
			bd := &storagev1alpha1.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-update-aggregated",
					Namespace: testNamespace,
				},
				Spec: fleetv1alpha1.BundleDeploymentSpec{
					DeploymentID: "original-id",
				},
			}
			err := k8sclient.Create(testCtx, bd)
			Expect(err).NotTo(HaveOccurred())

			// Update
			bd.Spec.DeploymentID = "updated-id"
			bd.Spec.Options.DefaultNamespace = "updated-namespace"
			err = k8sclient.Update(testCtx, bd)
			Expect(err).NotTo(HaveOccurred())

			// Verify update
			retrieved := &storagev1alpha1.BundleDeployment{}
			err = k8sclient.Get(testCtx, client.ObjectKeyFromObject(bd), retrieved)
			Expect(err).NotTo(HaveOccurred())
			Expect(retrieved.Spec.DeploymentID).To(Equal("updated-id"))
			Expect(retrieved.Spec.Options.DefaultNamespace).To(Equal("updated-namespace"))

			GinkgoWriter.Printf("✅ Updated BundleDeployment through API aggregation\n")
		})

		It("should delete a BundleDeployment through kube-apiserver", func() {
			// Create
			bd := &storagev1alpha1.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-delete-aggregated",
					Namespace: testNamespace,
				},
				Spec: fleetv1alpha1.BundleDeploymentSpec{
					DeploymentID: "delete-test-id",
				},
			}
			err := k8sclient.Create(testCtx, bd)
			Expect(err).NotTo(HaveOccurred())

			// Delete
			err = k8sclient.Delete(testCtx, bd)
			Expect(err).NotTo(HaveOccurred())

			// Verify deletion
			Eventually(func() bool {
				retrieved := &storagev1alpha1.BundleDeployment{}
				err := k8sclient.Get(testCtx, client.ObjectKeyFromObject(bd), retrieved)
				return err != nil && client.IgnoreNotFound(err) == nil
			}, 5*time.Second).Should(BeTrue())

			GinkgoWriter.Printf("✅ Deleted BundleDeployment through API aggregation\n")
		})

		It("should list BundleDeployments through kube-apiserver", func() {
			// Create multiple BundleDeployments
			for i := 0; i < 3; i++ {
				bd := &storagev1alpha1.BundleDeployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("test-list-%d", i),
						Namespace: testNamespace,
						Labels: map[string]string{
							"test": "list",
						},
					},
					Spec: fleetv1alpha1.BundleDeploymentSpec{
						DeploymentID: fmt.Sprintf("list-test-id-%d", i),
					},
				}
				err := k8sclient.Create(testCtx, bd)
				Expect(err).NotTo(HaveOccurred())
			}

			// List them
			list := &storagev1alpha1.BundleDeploymentList{}
			err := k8sclient.List(testCtx, list, client.InNamespace(testNamespace))
			Expect(err).NotTo(HaveOccurred())
			Expect(len(list.Items)).To(BeNumerically(">=", 3))

			GinkgoWriter.Printf("✅ Listed %d BundleDeployments through API aggregation\n",
				len(list.Items))
		})

		It("should list BundleDeployments with label selector through kube-apiserver", func() {
			// Create BundleDeployments with different labels
			bd1 := &storagev1alpha1.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-selector-prod",
					Namespace: testNamespace,
					Labels: map[string]string{
						"env": "prod",
					},
				},
			}
			err := k8sclient.Create(testCtx, bd1)
			Expect(err).NotTo(HaveOccurred())

			bd2 := &storagev1alpha1.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-selector-dev",
					Namespace: testNamespace,
					Labels: map[string]string{
						"env": "dev",
					},
				},
			}
			err = k8sclient.Create(testCtx, bd2)
			Expect(err).NotTo(HaveOccurred())

			// List with label selector
			list := &storagev1alpha1.BundleDeploymentList{}
			err = k8sclient.List(testCtx, list,
				client.InNamespace(testNamespace),
				client.MatchingLabels{"env": "prod"})
			Expect(err).NotTo(HaveOccurred())

			Expect(len(list.Items)).To(BeNumerically(">=", 1))
			for _, item := range list.Items {
				Expect(item.Labels["env"]).To(Equal("prod"))
			}

			GinkgoWriter.Printf("✅ Listed BundleDeployments with label selector through API aggregation\n")
		})

		It("should update status subresource through kube-apiserver", func() {
			// Create
			bd := &storagev1alpha1.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-status-aggregated",
					Namespace: testNamespace,
				},
				Spec: fleetv1alpha1.BundleDeploymentSpec{
					DeploymentID: "status-test-id",
				},
			}
			err := k8sclient.Create(testCtx, bd)
			Expect(err).NotTo(HaveOccurred())

			// Update status
			bd.Status.Ready = true
			bd.Status.AppliedDeploymentID = "status-test-id"
			bd.Status.Display.State = "Ready"
			err = k8sclient.Status().Update(testCtx, bd)
			Expect(err).NotTo(HaveOccurred())

			// Verify status update
			retrieved := &storagev1alpha1.BundleDeployment{}
			err = k8sclient.Get(testCtx, client.ObjectKeyFromObject(bd), retrieved)
			Expect(err).NotTo(HaveOccurred())
			Expect(retrieved.Status.Ready).To(BeTrue())
			Expect(retrieved.Status.AppliedDeploymentID).To(Equal("status-test-id"))
			Expect(retrieved.Status.Display.State).To(Equal("Ready"))

			GinkgoWriter.Printf("✅ Updated status through API aggregation\n")
		})
	})
})
