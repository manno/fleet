package apiserver

import (
	"context"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"

	fleetv1alpha1 "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	storagev1alpha1 "github.com/rancher/fleet/pkg/apis/storage.fleet.cattle.io/v1alpha1"
)

var _ = Describe("API Server Storage Layer", func() {
	var (
		testCtx context.Context
	)

	BeforeEach(func() {
		// Use the shared db and store from suite_test.go
		testCtx = request.WithNamespace(context.Background(), "test-namespace")
	})

	AfterEach(func() {
		// Don't close db or store here - they're shared across tests
	})

	Describe("BundleDeployment CRUD Operations", func() {
		It("should create a BundleDeployment", func() {
			bd := &storagev1alpha1.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-bd",
					// Namespace is ignored, taken from context
					Labels: map[string]string{
						"test": "label",
					},
				},
				Spec: fleetv1alpha1.BundleDeploymentSpec{
					DeploymentID: "test-deployment-id",
					Options: fleetv1alpha1.BundleDeploymentOptions{
						DefaultNamespace: "default",
					},
				},
			}

			obj, err := store.Create(testCtx, bd, nil, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(obj).NotTo(BeNil())

			created := obj.(*storagev1alpha1.BundleDeployment)
			Expect(created.Name).To(Equal("test-bd"))
			Expect(created.Namespace).To(Equal("test-namespace"))
			Expect(created.ResourceVersion).NotTo(BeEmpty())
			Expect(created.UID).NotTo(BeEmpty())
			Expect(created.APIVersion).To(Equal("storage.fleet.cattle.io/v1alpha1"))
		})

		It("should get a BundleDeployment", func() {
			bd := &storagev1alpha1.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-get",
				},
				Spec: fleetv1alpha1.BundleDeploymentSpec{
					DeploymentID: "get-test-id",
				},
			}

			_, err := store.Create(testCtx, bd, nil, nil)
			Expect(err).NotTo(HaveOccurred())

			obj, err := store.Get(testCtx, "test-get", nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(obj).NotTo(BeNil())

			retrieved := obj.(*storagev1alpha1.BundleDeployment)
			Expect(retrieved.Name).To(Equal("test-get"))
			Expect(retrieved.Spec.DeploymentID).To(Equal("get-test-id"))
		})

		It("should update a BundleDeployment", func() {
			bd := &storagev1alpha1.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-update",
				},
				Spec: fleetv1alpha1.BundleDeploymentSpec{
					DeploymentID: "original-id",
				},
			}

			obj, err := store.Create(testCtx, bd, nil, nil)
			Expect(err).NotTo(HaveOccurred())

			created := obj.(*storagev1alpha1.BundleDeployment)
			created.Spec.DeploymentID = "updated-id"
			created.Spec.Options.DefaultNamespace = "updated-namespace"

			updated, _, err := store.Update(testCtx, "test-update", rest.DefaultUpdatedObjectInfo(created), nil, nil, false, nil)
			Expect(err).NotTo(HaveOccurred())

			updatedBD := updated.(*storagev1alpha1.BundleDeployment)
			Expect(updatedBD.Spec.DeploymentID).To(Equal("updated-id"))
			Expect(updatedBD.Spec.Options.DefaultNamespace).To(Equal("updated-namespace"))
		})

		It("should delete a BundleDeployment", func() {
			bd := &storagev1alpha1.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-delete",
				},
				Spec: fleetv1alpha1.BundleDeploymentSpec{
					DeploymentID: "delete-test-id",
				},
			}

			_, err := store.Create(testCtx, bd, nil, nil)
			Expect(err).NotTo(HaveOccurred())

			_, _, err = store.Delete(testCtx, "test-delete", nil, nil)
			Expect(err).NotTo(HaveOccurred())

			_, err = store.Get(testCtx, "test-delete", nil)
			Expect(err).To(HaveOccurred())
		})

		It("should list BundleDeployments", func() {
			// Create multiple BundleDeployments
			for i := 0; i < 3; i++ {
				bd := &storagev1alpha1.BundleDeployment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-list-" + string(rune('a'+i)),
						Labels: map[string]string{
							"test": "list",
						},
					},
					Spec: fleetv1alpha1.BundleDeploymentSpec{
						DeploymentID: "list-test-id",
					},
				}
				_, err := store.Create(testCtx, bd, nil, nil)
				Expect(err).NotTo(HaveOccurred())
			}

			obj, err := store.List(testCtx, nil)
			Expect(err).NotTo(HaveOccurred())
			Expect(obj).NotTo(BeNil())

			list := obj.(*storagev1alpha1.BundleDeploymentList)
			Expect(len(list.Items)).To(BeNumerically(">=", 3))
		})

		It("should list BundleDeployments with label selector", func() {
			// Create BundleDeployments with different labels
			bd1 := &storagev1alpha1.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-selector-1",
					Labels: map[string]string{
						"env": "prod",
					},
				},
			}
			_, err := store.Create(testCtx, bd1, nil, nil)
			Expect(err).NotTo(HaveOccurred())

			bd2 := &storagev1alpha1.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-selector-2",
					Labels: map[string]string{
						"env": "dev",
					},
				},
			}
			_, err = store.Create(testCtx, bd2, nil, nil)
			Expect(err).NotTo(HaveOccurred())

			// List with label selector
			selector, err := labels.Parse("env=prod")
			Expect(err).NotTo(HaveOccurred())

			obj, err := store.List(testCtx, &metainternalversion.ListOptions{
				LabelSelector: selector,
			})
			Expect(err).NotTo(HaveOccurred())

			list := obj.(*storagev1alpha1.BundleDeploymentList)
			Expect(len(list.Items)).To(BeNumerically(">=", 1))
			for _, item := range list.Items {
				Expect(item.Labels["env"]).To(Equal("prod"))
			}
		})
	})

	Describe("BundleDeployment Status Operations", func() {
		It("should update status subresource", func() {
			bd := &storagev1alpha1.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-status",
				},
				Spec: fleetv1alpha1.BundleDeploymentSpec{
					DeploymentID: "status-test-id",
				},
			}

			obj, err := store.Create(testCtx, bd, nil, nil)
			Expect(err).NotTo(HaveOccurred())

			created := obj.(*storagev1alpha1.BundleDeployment)
			created.Status.Ready = true
			created.Status.AppliedDeploymentID = "status-test-id"
			created.Status.Display.State = "Ready"

			statusStore := store.Status()
			updated, _, err := statusStore.Update(testCtx, "test-status", rest.DefaultUpdatedObjectInfo(created), nil, nil, false, nil)
			Expect(err).NotTo(HaveOccurred())

			updatedBD := updated.(*storagev1alpha1.BundleDeployment)
			Expect(updatedBD.Status.Ready).To(BeTrue())
			Expect(updatedBD.Status.AppliedDeploymentID).To(Equal("status-test-id"))
			Expect(updatedBD.Status.Display.State).To(Equal("Ready"))
		})
	})

	Describe("API Group Verification", func() {
		It("should use storage.fleet.cattle.io API group", func() {
			Expect(storagev1alpha1.SchemeGroupVersion.Group).To(Equal("storage.fleet.cattle.io"))
			Expect(storagev1alpha1.SchemeGroupVersion.Version).To(Equal("v1alpha1"))
		})

		It("should return correct GroupVersionKind", func() {
			gvk := store.GroupVersionKind(storagev1alpha1.SchemeGroupVersion)
			Expect(gvk.Group).To(Equal("storage.fleet.cattle.io"))
			Expect(gvk.Version).To(Equal("v1alpha1"))
			Expect(gvk.Kind).To(Equal("BundleDeployment"))
		})
	})

	Describe("Database Verification", func() {
		It("should have actual data in the database", func() {
			// Create a test BundleDeployment
			bd := &storagev1alpha1.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-db-verification",
					Labels: map[string]string{
						"test": "db-verification",
					},
				},
				Spec: fleetv1alpha1.BundleDeploymentSpec{
					DeploymentID: "db-verification-id",
					Options: fleetv1alpha1.BundleDeploymentOptions{
						DefaultNamespace: "default",
					},
				},
			}

			_, err := store.Create(testCtx, bd, nil, nil)
			Expect(err).NotTo(HaveOccurred())

			// Query the database directly using QueryAll
			results, err := db.QueryAll()
			Expect(err).NotTo(HaveOccurred())
			Expect(results).NotTo(BeEmpty(), "database should contain bundledeployments")

			// Verify our created BundleDeployment is in the results
			found := false
			for _, row := range results {
				if row["name"] == "test-db-verification" && row["namespace"] == "test-namespace" {
					found = true
					Expect(row["resource_version"]).NotTo(BeZero())
					Expect(row["uid"]).NotTo(BeEmpty())
					Expect(row["creation_timestamp"]).NotTo(BeZero())
					Expect(row["generation"]).To(Equal(int64(1)))
					break
				}
			}
			Expect(found).To(BeTrue(), "created bundledeployment should exist in database")
		})
	})
})
