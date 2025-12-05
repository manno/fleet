package apiserver

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/rancher/fleet/integrationtests/utils"
	"github.com/rancher/fleet/internal/cmd/apiserver/storage"
	storagev1alpha1 "github.com/rancher/fleet/pkg/apis/storage.fleet.cattle.io/v1alpha1"
)

const (
	timeout = 30 * time.Second
)

var (
	cfg       *rest.Config
	testEnv   *envtest.Environment
	ctx       context.Context
	cancel    context.CancelFunc
	k8sClient client.Client
	tmpDir    string
	dbPath    string
	db        *storage.Database
	store     *storage.BundleDeploymentStorage
)

func TestAPIServer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "API Server Integration Suite")
}

var _ = BeforeSuite(func() {
	SetDefaultEventuallyTimeout(timeout)
	ctx, cancel = context.WithCancel(context.TODO())

	utils.SuppressLogs()
	ctrl.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	// Create temporary directories
	var err error
	tmpDir, err = os.MkdirTemp("", "fleet-apiserver-test-*")
	Expect(err).NotTo(HaveOccurred())

	dbPath = filepath.Join(tmpDir, "bundledeployments.db")

	// Start test environment
	testEnv = utils.NewEnvTest("../..")
	cfg, err = utils.StartTestEnv(testEnv)
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	// Add storage.fleet.cattle.io API to scheme
	err = storagev1alpha1.AddToScheme(testEnv.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// Create client
	k8sClient, err = client.New(cfg, client.Options{Scheme: testEnv.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	// Initialize database and storage once for all tests
	db, err = storage.NewDatabase(dbPath)
	Expect(err).NotTo(HaveOccurred())

	store, err = storage.NewBundleDeploymentStorage(db)
	Expect(err).NotTo(HaveOccurred())
})

var _ = AfterSuite(func() {
	if store != nil {
		store.Destroy()
	}
	if db != nil {
		db.Close()
	}
	cancel()
	if tmpDir != "" {
		os.RemoveAll(tmpDir)
	}
	Expect(testEnv.Stop()).ToNot(HaveOccurred())
})
