package storage

import (
	"context"
	"os"
	"testing"

	metainternalversion "k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"

	fleetv1alpha1 "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	storagev1alpha1 "github.com/rancher/fleet/pkg/apis/storage.fleet.cattle.io/v1alpha1"
)

func TestDatabaseBasicOperations(t *testing.T) {
	// Create temporary database
	tmpfile, err := os.CreateTemp("", "test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	// Initialize database
	db, err := NewDatabase(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	// Test resource version
	rv1, err := db.NextResourceVersion()
	if err != nil {
		t.Fatalf("Failed to get next resource version: %v", err)
	}
	if rv1 < 1 {
		t.Errorf("Expected resource version >= 1, got %d", rv1)
	}

	rv2, err := db.NextResourceVersion()
	if err != nil {
		t.Fatalf("Failed to get next resource version: %v", err)
	}
	if rv2 != rv1+1 {
		t.Errorf("Expected resource version %d, got %d", rv1+1, rv2)
	}

	// Test ping
	if err := db.Ping(); err != nil {
		t.Errorf("Ping failed: %v", err)
	}
}

func TestBundleDeploymentCRUD(t *testing.T) {
	// Create temporary database
	tmpfile, err := os.CreateTemp("", "test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	// Initialize database and storage
	db, err := NewDatabase(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	storage, err := NewBundleDeploymentStorage(db)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Destroy()

	ctx := context.WithValue(context.Background(), "namespace", "test-namespace")

	// Test Create
	bd := &storagev1alpha1.BundleDeployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-bd",
			Namespace: "test-namespace",
			Labels: map[string]string{
				"test": "label",
			},
		},
		Spec: fleetv1alpha1.BundleDeploymentSpec{
			DeploymentID: "test-deployment-id",
		},
	}

	created, err := storage.Create(ctx, bd, nil, &metav1.CreateOptions{})
	if err != nil {
		t.Fatalf("Failed to create BundleDeployment: %v", err)
	}

	createdBD := created.(*storagev1alpha1.BundleDeployment)
	if createdBD.Name != "test-bd" {
		t.Errorf("Expected name 'test-bd', got '%s'", createdBD.Name)
	}
	if createdBD.UID == "" {
		t.Error("Expected UID to be set")
	}
	if createdBD.ResourceVersion == "" {
		t.Error("Expected ResourceVersion to be set")
	}

	// Test Get
	retrieved, err := storage.Get(ctx, "test-bd", &metav1.GetOptions{})
	if err != nil {
		t.Fatalf("Failed to get BundleDeployment: %v", err)
	}

	retrievedBD := retrieved.(*storagev1alpha1.BundleDeployment)
	if retrievedBD.Name != "test-bd" {
		t.Errorf("Expected name 'test-bd', got '%s'", retrievedBD.Name)
	}
	if retrievedBD.Spec.DeploymentID != "test-deployment-id" {
		t.Errorf("Expected deployment ID 'test-deployment-id', got '%s'", retrievedBD.Spec.DeploymentID)
	}

	// Test List
	listResult, err := storage.List(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to list BundleDeployments: %v", err)
	}

	list := listResult.(*storagev1alpha1.BundleDeploymentList)
	if len(list.Items) != 1 {
		t.Errorf("Expected 1 item in list, got %d", len(list.Items))
	}

	// Test Delete
	_, _, err = storage.Delete(ctx, "test-bd", nil, &metav1.DeleteOptions{})
	if err != nil {
		t.Fatalf("Failed to delete BundleDeployment: %v", err)
	}

	// Verify deletion
	listResult, err = storage.List(ctx, nil)
	if err != nil {
		t.Fatalf("Failed to list BundleDeployments after delete: %v", err)
	}

	list = listResult.(*storagev1alpha1.BundleDeploymentList)
	if len(list.Items) != 0 {
		t.Errorf("Expected 0 items after delete, got %d", len(list.Items))
	}
}

func TestBundleDeploymentListWithLabelSelector(t *testing.T) {
	// Create temporary database
	tmpfile, err := os.CreateTemp("", "test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	// Initialize database and storage
	db, err := NewDatabase(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	storage, err := NewBundleDeploymentStorage(db)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Destroy()

	ctx := context.WithValue(context.Background(), "namespace", "test-namespace")

	// Create multiple BundleDeployments with different labels
	for i := 0; i < 3; i++ {
		bd := &storagev1alpha1.BundleDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-bd-" + string(rune('a'+i)),
				Namespace: "test-namespace",
				Labels: map[string]string{
					"app": "test",
				},
			},
		}
		if i == 1 {
			bd.Labels["env"] = "prod"
		}

		_, err := storage.Create(ctx, bd, nil, &metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Failed to create BundleDeployment %d: %v", i, err)
		}
	}

	// Test list with label selector
	selector, _ := labels.Parse("env=prod")
	listOptions := &metainternalversion.ListOptions{
		LabelSelector: selector,
	}

	listResult, err := storage.List(ctx, listOptions)
	if err != nil {
		t.Fatalf("Failed to list with selector: %v", err)
	}

	list := listResult.(*storagev1alpha1.BundleDeploymentList)
	if len(list.Items) != 1 {
		t.Errorf("Expected 1 item with env=prod, got %d", len(list.Items))
	}
}

func TestDatabaseQueryAll(t *testing.T) {
	// Create temporary database
	tmpfile, err := os.CreateTemp("", "test-*.db")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())
	tmpfile.Close()

	// Initialize database and storage
	db, err := NewDatabase(tmpfile.Name())
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}
	defer db.Close()

	storage, err := NewBundleDeploymentStorage(db)
	if err != nil {
		t.Fatalf("Failed to create storage: %v", err)
	}
	defer storage.Destroy()

	ctx := context.WithValue(context.Background(), "namespace", "test-namespace")

	// Create test BundleDeployments
	for i := 0; i < 3; i++ {
		bd := &storagev1alpha1.BundleDeployment{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-queryall-" + string(rune('a'+i)),
				Namespace: "test-namespace",
				Labels: map[string]string{
					"test": "queryall",
				},
			},
			Spec: fleetv1alpha1.BundleDeploymentSpec{
				DeploymentID: "queryall-test-id",
			},
		}

		_, err := storage.Create(ctx, bd, nil, &metav1.CreateOptions{})
		if err != nil {
			t.Fatalf("Failed to create BundleDeployment %d: %v", i, err)
		}
	}

	// Test QueryAll
	results, err := db.QueryAll()
	if err != nil {
		t.Fatalf("Failed to QueryAll: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 rows, got %d", len(results))
	}

	// Verify data integrity
	for i, row := range results {
		if row["namespace"] != "test-namespace" {
			t.Errorf("Row %d: Expected namespace 'test-namespace', got '%v'", i, row["namespace"])
		}

		if row["resource_version"] == nil || row["resource_version"].(int64) == 0 {
			t.Errorf("Row %d: Expected non-zero resource_version", i)
		}

		if row["uid"] == nil || row["uid"].(string) == "" {
			t.Errorf("Row %d: Expected non-empty uid", i)
		}

		if row["generation"] == nil || row["generation"].(int64) != 1 {
			t.Errorf("Row %d: Expected generation 1, got %v", i, row["generation"])
		}

		// Verify spec is present
		if row["spec"] == nil || row["spec"].(string) == "" {
			t.Errorf("Row %d: Expected non-empty spec", i)
		}
	}
}

