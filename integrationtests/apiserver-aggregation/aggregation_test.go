package apiserver

import (
	"context"
	"fmt"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
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

	Describe("Database Persistence Verification", func() {
		It("should verify API aggregation is actually routing to our server", func() {
			// First, let's verify the APIService is properly configured
			apiService := &apiregistrationv1.APIService{}
			err := k8sclient.Get(testCtx, client.ObjectKey{
				Name: "v1alpha1.storage.fleet.cattle.io",
			}, apiService)
			Expect(err).NotTo(HaveOccurred())

			GinkgoWriter.Printf("✅ APIService status: %+v\n", apiService.Status)

			// Check if Available condition is true
			available := false
			for _, cond := range apiService.Status.Conditions {
				if cond.Type == apiregistrationv1.Available {
					available = cond.Status == apiregistrationv1.ConditionTrue
					GinkgoWriter.Printf("  Available: %v (reason: %s, message: %s)\n",
						available, cond.Reason, cond.Message)
					break
				}
			}
			Expect(available).To(BeTrue(), "APIService should be available")

			// Try creating through the client and check database immediately
			bd := &storagev1alpha1.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-api-routing",
					Namespace: testNamespace,
				},
				Spec: fleetv1alpha1.BundleDeploymentSpec{
					DeploymentID: "routing-test",
				},
			}
			err = k8sclient.Create(testCtx, bd)
			Expect(err).NotTo(HaveOccurred())

			GinkgoWriter.Printf("✅ Created BundleDeployment via k8s client\n")

			// Immediately check database (no sleep)
			results, err := aggregatedServer.db.QueryAll()
			Expect(err).NotTo(HaveOccurred())

			GinkgoWriter.Printf("DEBUG: Database has %d entries\n", len(results))
			for _, row := range results {
				GinkgoWriter.Printf("  - %s/%s\n", row["namespace"], row["name"])
			}

			if len(results) == 0 {
				// If database is empty, the request went to etcd, not our aggregated server
				Fail("Database is empty - requests are going to etcd instead of aggregated API server!")
			}
		})

		It("should persist data to the SQLite database", func() {
			// Create multiple BundleDeployments through the aggregated API
			for i := 0; i < 3; i++ {
				bd := &storagev1alpha1.BundleDeployment{
					ObjectMeta: metav1.ObjectMeta{
						Name:      fmt.Sprintf("test-db-persist-%d", i),
						Namespace: testNamespace,
						Labels: map[string]string{
							"test": "db-persistence",
						},
					},
					Spec: fleetv1alpha1.BundleDeploymentSpec{
						DeploymentID: fmt.Sprintf("db-persist-id-%d", i),
						Options: fleetv1alpha1.BundleDeploymentOptions{
							DefaultNamespace: "default",
						},
					},
				}
				err := k8sclient.Create(testCtx, bd)
				Expect(err).NotTo(HaveOccurred())
			}

			// Wait a bit for writes to flush
			time.Sleep(500 * time.Millisecond)

			// Query the database directly using the shared connection
			results, err := aggregatedServer.db.QueryAll()
			Expect(err).NotTo(HaveOccurred())

			GinkgoWriter.Printf("DEBUG: QueryAll returned %d results\n", len(results))
			for i, row := range results {
				GinkgoWriter.Printf("  Row %d: namespace=%v, name=%v\n", i, row["namespace"], row["name"])
			}

			Expect(results).NotTo(BeEmpty(), "database should contain bundledeployments")

			GinkgoWriter.Printf("✅ Found %d BundleDeployments in database\n", len(results))

			// Verify our created BundleDeployments are in the database
			foundCount := 0
			for _, row := range results {
				if row["namespace"] == testNamespace {
					name := row["name"].(string)
					if len(name) >= 16 && name[:16] == "test-db-persist-" {
						foundCount++
						// Verify essential fields
						Expect(row["resource_version"]).NotTo(BeZero())
						Expect(row["uid"]).NotTo(BeEmpty())
						Expect(row["creation_timestamp"]).NotTo(BeZero())
						Expect(row["generation"]).To(Equal(int64(1)))
						Expect(row["spec"]).NotTo(BeEmpty())

						GinkgoWriter.Printf("  - Verified %s: rv=%v, uid=%s\n",
							name, row["resource_version"], row["uid"])
					}
				}
			}

			Expect(foundCount).To(Equal(3), "should find all 3 created bundledeployments in database")
			GinkgoWriter.Printf("✅ Verified all BundleDeployments persisted to database\n")
		})

		It("should reflect updates in the database", func() {
			// Create a BundleDeployment
			bd := &storagev1alpha1.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-db-update",
					Namespace: testNamespace,
					Labels: map[string]string{
						"version": "v1",
					},
				},
				Spec: fleetv1alpha1.BundleDeploymentSpec{
					DeploymentID: "original-deployment-id",
				},
			}
			err := k8sclient.Create(testCtx, bd)
			Expect(err).NotTo(HaveOccurred())

			// Get initial state from database
			results, err := aggregatedServer.db.QueryAll()
			Expect(err).NotTo(HaveOccurred())

			var initialGen int64
			for _, row := range results {
				if row["namespace"] == testNamespace && row["name"] == "test-db-update" {
					initialGen = row["generation"].(int64)
					break
				}
			}
			Expect(initialGen).To(Equal(int64(1)))

			// Update the BundleDeployment
			bd.Spec.DeploymentID = "updated-deployment-id"
			bd.Labels["version"] = "v2"
			err = k8sclient.Update(testCtx, bd)
			Expect(err).NotTo(HaveOccurred())

			time.Sleep(500 * time.Millisecond)

			// Verify update in database
			results, err = aggregatedServer.db.QueryAll()
			Expect(err).NotTo(HaveOccurred())

			found := false
			for _, row := range results {
				if row["namespace"] == testNamespace && row["name"] == "test-db-update" {
					found = true
					// Generation should increment on spec update
					Expect(row["generation"].(int64)).To(Equal(int64(2)))
					// Labels should be updated
					labelsJSON := row["labels"].(string)
					Expect(labelsJSON).To(ContainSubstring("v2"))
					GinkgoWriter.Printf("✅ Database reflects update: generation=%v\n", row["generation"])
					break
				}
			}
			Expect(found).To(BeTrue())
		})

		It("should remove deleted resources from the database", func() {
			// Create a BundleDeployment
			bd := &storagev1alpha1.BundleDeployment{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-db-delete",
					Namespace: testNamespace,
				},
				Spec: fleetv1alpha1.BundleDeploymentSpec{
					DeploymentID: "delete-me",
				},
			}
			err := k8sclient.Create(testCtx, bd)
			Expect(err).NotTo(HaveOccurred())

			// Verify it exists in database
			results, err := aggregatedServer.db.QueryAll()
			Expect(err).NotTo(HaveOccurred())

			foundBefore := false
			for _, row := range results {
				if row["namespace"] == testNamespace && row["name"] == "test-db-delete" {
					foundBefore = true
					break
				}
			}
			Expect(foundBefore).To(BeTrue(), "bundledeployment should exist before deletion")

			// Delete it
			err = k8sclient.Delete(testCtx, bd)
			Expect(err).NotTo(HaveOccurred())

			// Wait for deletion to complete
			Eventually(func() bool {
				retrieved := &storagev1alpha1.BundleDeployment{}
				err := k8sclient.Get(testCtx, client.ObjectKeyFromObject(bd), retrieved)
				return err != nil && client.IgnoreNotFound(err) == nil
			}, 5*time.Second).Should(BeTrue())

			time.Sleep(500 * time.Millisecond)

			// Verify it's gone from database
			results, err = aggregatedServer.db.QueryAll()
			Expect(err).NotTo(HaveOccurred())

			foundAfter := false
			for _, row := range results {
				if row["namespace"] == testNamespace && row["name"] == "test-db-delete" {
					foundAfter = true
					break
				}
			}
			Expect(foundAfter).To(BeFalse(), "bundledeployment should be removed from database after deletion")
			GinkgoWriter.Printf("✅ Database reflects deletion\n")
		})
	})
})
