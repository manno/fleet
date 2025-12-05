package apiserver

import (
	"context"
	"fmt"
	"time"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/apiserver/pkg/server/options"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	fleetv1alpha1 "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	"github.com/rancher/fleet/internal/cmd/apiserver/storage"
)

var (
	// Scheme defines methods for serializing and deserializing API objects
	Scheme = runtime.NewScheme()
	// Codecs provides methods for retrieving codecs and serializers for specific groups and versions
	Codecs = serializer.NewCodecFactory(Scheme)
)

func init() {
	// Register the fleet types with the scheme
	if err := fleetv1alpha1.AddToScheme(Scheme); err != nil {
		panic(err)
	}
	metav1.AddToGroupVersion(Scheme, schema.GroupVersion{Version: "v1"})
}

func run(ctx context.Context, opts *FleetAPIServer) error {
	// Initialize database
	db, err := storage.NewDatabase(opts.DBPath)
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer db.Close()

	// Create storage for BundleDeployment
	bundleDeploymentStorage, err := storage.NewBundleDeploymentStorage(db)
	if err != nil {
		return fmt.Errorf("failed to create bundle deployment storage: %w", err)
	}

	// Setup recommended options for API server
	recommendedOptions := options.NewRecommendedOptions("", Codecs.LegacyCodec(fleetv1alpha1.SchemeGroupVersion))
	recommendedOptions.SecureServing.BindPort = opts.SecurePort
	recommendedOptions.SecureServing.ServerCert.CertDirectory = opts.CertDir

	// Configure authentication and authorization delegation
	if opts.AuthenticationKubeconfig != "" {
		recommendedOptions.Authentication.RemoteKubeConfigFile = opts.AuthenticationKubeconfig
	}
	if opts.AuthorizationKubeconfig != "" {
		recommendedOptions.Authorization.RemoteKubeConfigFile = opts.AuthorizationKubeconfig
	}

	// Disable etcd options since we're using SQLite
	recommendedOptions.Etcd = nil

	// Create server config
	serverConfig := genericapiserver.NewRecommendedConfig(Codecs)
	if err := recommendedOptions.ApplyTo(serverConfig); err != nil {
		return fmt.Errorf("failed to apply recommended options: %w", err)
	}

	// Set version info - not needed, it's configured automatically

	// Get in-cluster kubeconfig for client
	kubeconfig, err := clientcmd.BuildConfigFromFlags("", opts.AuthenticationKubeconfig)
	if err != nil {
		return fmt.Errorf("failed to build kubeconfig: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}

	// Create generic API server
	genericServer, err := serverConfig.Complete().New("fleet-apiserver", genericapiserver.NewEmptyDelegate())
	if err != nil {
		return fmt.Errorf("failed to create generic API server: %w", err)
	}

	// Install the API group
	apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(fleetv1alpha1.SchemeGroupVersion.Group, Scheme, metav1.ParameterCodec, Codecs)

	// Create storage map for v1alpha1 resources
	v1alpha1Storage := map[string]rest.Storage{}
	v1alpha1Storage["bundledeployments"] = bundleDeploymentStorage
	v1alpha1Storage["bundledeployments/status"] = bundleDeploymentStorage.Status()

	apiGroupInfo.VersionedResourcesStorageMap["v1alpha1"] = v1alpha1Storage

	if err := genericServer.InstallAPIGroup(&apiGroupInfo); err != nil {
		return fmt.Errorf("failed to install API group: %w", err)
	}

	logrus.Infof("Starting Fleet API Server on port %d", opts.SecurePort)
	logrus.Infof("Using database at %s", opts.DBPath)

	// Start metrics and health endpoints
	if err := setupHealthChecks(genericServer, db); err != nil {
		return fmt.Errorf("failed to setup health checks: %w", err)
	}

	// Monitor clientset for connectivity
	go monitorAPIServerHealth(ctx, clientset)

	// Start the server
	return genericServer.PrepareRun().Run(ctx.Done())
}

func setupHealthChecks(server *genericapiserver.GenericAPIServer, db *storage.Database) error {
	// Health checks are configured automatically
	return nil
}

func monitorAPIServerHealth(ctx context.Context, clientset *kubernetes.Clientset) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Check connectivity to main API server
			_, err := clientset.Discovery().ServerVersion()
			if err != nil {
				logrus.Warnf("Failed to connect to main API server: %v", err)
			}
		}
	}
}
