package apiserver

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apiserver/pkg/endpoints/openapi"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/apiserver/pkg/server/options"
	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	basecompatibility "k8s.io/component-base/compatibility"
	baseversion "k8s.io/component-base/version"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	"github.com/rancher/fleet/integrationtests/utils"
	fleetapiserver "github.com/rancher/fleet/internal/cmd/apiserver"
	"github.com/rancher/fleet/internal/cmd/apiserver/storage"
	fleetv1 "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	storagev1alpha1 "github.com/rancher/fleet/pkg/apis/storage.fleet.cattle.io/v1alpha1"
	fleetopenapi "github.com/rancher/fleet/pkg/generated/openapi"
)

var (
	testEnv          *envtest.Environment
	cfg              *restclient.Config
	k8sclient        client.Client
	ctx              context.Context
	cancel           context.CancelFunc
	tmpDir           string
	aggregatedServer *aggregatedServerInfo
)

type aggregatedServerInfo struct {
	host   string
	port   int
	dbPath string
	db     *storage.Database
	cancel context.CancelFunc
}

func TestFullIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "API Server Full Integration Suite")
}

var _ = BeforeSuite(func() {
	SetDefaultEventuallyTimeout(60 * time.Second)
	ctx, cancel = context.WithCancel(context.TODO())

	utils.SuppressLogs()
	ctrl.SetLogger(zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true)))

	// Create temporary directory
	var err error
	tmpDir, err = os.MkdirTemp("", "fleet-apiserver-full-test-*")
	Expect(err).NotTo(HaveOccurred())

	// Start envtest
	testEnv = utils.NewEnvTest("../..")
	cfg, err = utils.StartTestEnv(testEnv)
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	// Register APIService scheme for k8s client
	err = apiregistrationv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = apiextensionsv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = fleetv1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	err = storagev1alpha1.AddToScheme(scheme.Scheme)
	Expect(err).NotTo(HaveOccurred())

	// Create client using standard k8s scheme
	k8sclient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sclient).NotTo(BeNil())

	// CRITICAL: Delete the storage.fleet.cattle.io CRD from envtest
	// so that API aggregation takes precedence
	// In production, this CRD should not exist in fleet-crd chart - only the APIService should be installed
	GinkgoWriter.Println("🔧 Removing storage.fleet.cattle.io CRD to force API aggregation...")
	GinkgoWriter.Println("   (In production, don't install this CRD - use APIService instead)")
	err = removeCRD(ctx, k8sclient, "bundledeployments.storage.fleet.cattle.io")
	Expect(err).NotTo(HaveOccurred())
	GinkgoWriter.Println("✅ CRD removed, APIService will handle storage.fleet.cattle.io")


	// Start our aggregated API server
	aggregatedServer, err = startAggregatedAPIServer(ctx, tmpDir, cfg)
	Expect(err).NotTo(HaveOccurred())

	// Create APIService resource
	err = createAPIService(ctx, k8sclient, aggregatedServer)
	Expect(err).NotTo(HaveOccurred())

	// Wait for APIService to be available
	Eventually(func() bool {
		return isAPIServiceAvailable(ctx, k8sclient)
	}, 30*time.Second, 1*time.Second).Should(BeTrue())

	GinkgoWriter.Println("✅ Full API aggregation setup complete")
})

var _ = AfterSuite(func() {
	if cancel != nil {
		cancel()
	}
	if aggregatedServer != nil {
		if aggregatedServer.cancel != nil {
			aggregatedServer.cancel()
		}
		if aggregatedServer.db != nil {
			aggregatedServer.db.Close()
		}
	}
	if tmpDir != "" {
		os.RemoveAll(tmpDir)
	}
	if testEnv != nil {
		Expect(testEnv.Stop()).ToNot(HaveOccurred())
	}
})

func startAggregatedAPIServer(ctx context.Context, tmpDir string, kubeConfig *restclient.Config) (*aggregatedServerInfo, error) {
	// Find available port
	listener, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return nil, fmt.Errorf("failed to find free port: %w", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	// Setup paths
	dbPath := filepath.Join(tmpDir, "bundledeployments.db")
	certDir := filepath.Join(tmpDir, "certs")
	err = os.MkdirAll(certDir, 0755)
	if err != nil {
		return nil, fmt.Errorf("failed to create cert dir: %w", err)
	}

	// Generate self-signed certificates
	err = generateTestCertificates(certDir)
	if err != nil {
		return nil, fmt.Errorf("failed to generate certificates: %w", err)
	}

	// Initialize database before starting server - this will be shared
	db, err := storage.NewDatabase(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize database: %w", err)
	}

	// Create server options
	opts := &fleetapiserver.FleetAPIServer{
		SecurePort: port,
		CertDir:    certDir,
		DBPath:     dbPath,
	}

	// Start server in background
	serverCtx, serverCancel := context.WithCancel(ctx)
	serverStarted := make(chan error, 1)

	go func() {
		defer GinkgoRecover()

		// Signal that we're starting
		serverStarted <- nil

		// Run server - pass the shared db instance
		err := runTestAPIServerWithDB(serverCtx, opts, kubeConfig, db)
		if err != nil && serverCtx.Err() == nil {
			GinkgoWriter.Printf("❌ API server error: %v\n", err)
		}
	}()

	// Wait for server to start
	select {
	case err := <-serverStarted:
		if err != nil {
			db.Close()
			serverCancel()
			return nil, err
		}
	case <-time.After(5 * time.Second):
		db.Close()
		serverCancel()
		return nil, fmt.Errorf("timeout waiting for server to start")
	}

	// Wait for server to be ready
	host := fmt.Sprintf("https://localhost:%d", port)
	Eventually(func() error {
		// Try to connect (ignore TLS errors - we're using self-signed certs)
		transport := &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
		client := &http.Client{Transport: transport, Timeout: 2 * time.Second}
		resp, err := client.Get(host + "/readyz")
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return fmt.Errorf("server not ready: %d", resp.StatusCode)
		}
		return nil
	}, 10*time.Second, 500*time.Millisecond).Should(Succeed())

	GinkgoWriter.Printf("✅ Aggregated API server started on %s\n", host)

	return &aggregatedServerInfo{
		host:   "localhost",
		port:   port,
		dbPath: dbPath,
		db:     db,
		cancel: serverCancel,
	}, nil
}

func runTestAPIServerWithDB(ctx context.Context, opts *fleetapiserver.FleetAPIServer, kubeConfig *restclient.Config, db *storage.Database) error {
	// This is a simplified version of internal/cmd/apiserver/server.go:run()
	// Use the provided db instance instead of creating a new one

	// Create storage
	bundleDeploymentStorage, err := storage.NewBundleDeploymentStorage(db)
	if err != nil {
		return fmt.Errorf("failed to create bundle deployment storage: %w", err)
	}

	// Setup server options using the apiserver's Scheme and Codecs
	recommendedOptions := options.NewRecommendedOptions("",
		fleetapiserver.Codecs.LegacyCodec(storagev1alpha1.SchemeGroupVersion))
	recommendedOptions.SecureServing.BindPort = opts.SecurePort
	recommendedOptions.SecureServing.ServerCert.CertDirectory = opts.CertDir
	recommendedOptions.Etcd = nil
	recommendedOptions.Admission = nil
	recommendedOptions.Authentication = nil
	recommendedOptions.Authorization = nil
	recommendedOptions.CoreAPI = nil
	recommendedOptions.Features.EnablePriorityAndFairness = false

	// Create server config using the apiserver's Codecs
	serverConfig := genericapiserver.NewRecommendedConfig(fleetapiserver.Codecs)
	serverConfig.LoopbackClientConfig = kubeConfig

	// Setup OpenAPI using the apiserver's Scheme
	serverConfig.OpenAPIConfig = genericapiserver.DefaultOpenAPIConfig(
		fleetopenapi.GetOpenAPIDefinitions,
		openapi.NewDefinitionNamer(fleetapiserver.Scheme),
	)
	serverConfig.OpenAPIConfig.Info.Title = "Fleet API Server"
	serverConfig.OpenAPIConfig.Info.Version = "v1alpha1"

	serverConfig.OpenAPIV3Config = genericapiserver.DefaultOpenAPIV3Config(
		fleetopenapi.GetOpenAPIDefinitions,
		openapi.NewDefinitionNamer(fleetapiserver.Scheme),
	)
	serverConfig.OpenAPIV3Config.Info.Title = "Fleet API Server"
	serverConfig.OpenAPIV3Config.Info.Version = "v1alpha1"

	serverConfig.EffectiveVersion = basecompatibility.NewEffectiveVersionFromString(
		baseversion.DefaultKubeBinaryVersion, "", "")

	if err := recommendedOptions.ApplyTo(serverConfig); err != nil {
		return fmt.Errorf("failed to apply recommended options: %w", err)
	}

	// Create generic API server
	genericServer, err := serverConfig.Complete().New("fleet-apiserver",
		genericapiserver.NewEmptyDelegate())
	if err != nil {
		return fmt.Errorf("failed to create generic API server: %w", err)
	}

	// Install API group using the apiserver's Scheme and Codecs
	apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(
		storagev1alpha1.SchemeGroupVersion.Group,
		fleetapiserver.Scheme,
		metav1.ParameterCodec,
		fleetapiserver.Codecs)

	v1alpha1Storage := map[string]rest.Storage{}
	v1alpha1Storage["bundledeployments"] = bundleDeploymentStorage
	v1alpha1Storage["bundledeployments/status"] = bundleDeploymentStorage.Status()

	apiGroupInfo.VersionedResourcesStorageMap["v1alpha1"] = v1alpha1Storage

	if err := genericServer.InstallAPIGroup(&apiGroupInfo); err != nil {
		return fmt.Errorf("failed to install API group: %w", err)
	}

	// Start server
	return genericServer.PrepareRun().Run(ctx.Done())
}

func generateTestCertificates(certDir string) error {
	// For testing, we'll use the same approach as apiserver - let it generate its own certs
	// The SecureServing option will auto-generate self-signed certs if they don't exist
	return nil
}

func createAPIService(ctx context.Context, k8sClient client.Client, server *aggregatedServerInfo) error {
	// Create default namespace if it doesn't exist
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default",
		},
	}
	_ = k8sClient.Create(ctx, ns) // Ignore error if exists

	// Create ExternalName Service pointing to localhost
	// This bypasses endpoint validation that rejects loopback addresses
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "fleet-apiserver",
			Namespace: "default",
		},
		Spec: corev1.ServiceSpec{
			Type:         corev1.ServiceTypeExternalName,
			ExternalName: "localhost",
			Ports: []corev1.ServicePort{{
				Port:     int32(server.port),
				Protocol: corev1.ProtocolTCP,
			}},
		},
	}
	err := k8sClient.Create(ctx, svc)
	if err != nil {
		return fmt.Errorf("failed to create service: %w", err)
	}

	// Create APIService
	apiService := &apiregistrationv1.APIService{
		ObjectMeta: metav1.ObjectMeta{
			Name: "v1alpha1.storage.fleet.cattle.io",
		},
		Spec: apiregistrationv1.APIServiceSpec{
			Group:                "storage.fleet.cattle.io",
			Version:              "v1alpha1",
			GroupPriorityMinimum: 1000,
			VersionPriority:      15,
			Service: &apiregistrationv1.ServiceReference{
				Namespace: "default",
				Name:      "fleet-apiserver",
				Port:      intPtr(int32(server.port)),
			},
			InsecureSkipTLSVerify: true, // For testing with self-signed certs
		},
	}
	err = k8sClient.Create(ctx, apiService)
	if client.IgnoreAlreadyExists(err) != nil {
		return fmt.Errorf("failed to create APIService: %w", err)
	}

	GinkgoWriter.Println("✅ Created APIService and ExternalName Service")
	return nil
}

func isAPIServiceAvailable(ctx context.Context, k8sClient client.Client) bool {
	apiService := &apiregistrationv1.APIService{}
	err := k8sClient.Get(ctx, client.ObjectKey{
		Name: "v1alpha1.storage.fleet.cattle.io",
	}, apiService)
	if err != nil {
		return false
	}

	for _, cond := range apiService.Status.Conditions {
		if cond.Type == apiregistrationv1.Available {
			return cond.Status == apiregistrationv1.ConditionTrue
		}
	}
	return false
}

func intPtr(i int32) *int32 {
	return &i
}

func removeCRD(ctx context.Context, k8sClient client.Client, crdName string) error {
	// Delete the CRD
	crd := &apiextensionsv1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: crdName,
		},
	}
	
	err := k8sClient.Delete(ctx, crd)
	if client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("failed to delete CRD %s: %w", crdName, err)
	}

	// Wait for CRD to be fully removed
	Eventually(func() bool {
		err := k8sClient.Get(ctx, client.ObjectKey{Name: crdName}, crd)
		return client.IgnoreNotFound(err) == nil
	}, 30*time.Second, 1*time.Second).Should(BeTrue())

	return nil
}
