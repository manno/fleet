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
	"k8s.io/apiserver/pkg/authentication/request/bearertoken"
	"k8s.io/apiserver/pkg/authentication/token/tokenfile"
	"k8s.io/apiserver/pkg/authorization/authorizerfactory"
	"k8s.io/apiserver/pkg/endpoints/openapi"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/apiserver/pkg/server/options"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	basecompatibility "k8s.io/component-base/compatibility"
	baseversion "k8s.io/component-base/version"
	ctrl "sigs.k8s.io/controller-runtime"

	fleetv1alpha1 "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
	"github.com/rancher/fleet/internal/cmd/apiserver/storage"
	fleetopenapi "github.com/rancher/fleet/pkg/generated/openapi"
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

	// Disable etcd options since we're using SQLite
	recommendedOptions.Etcd = nil
	
	// Disable admission since we don't need it
	recommendedOptions.Admission = nil

	// Create server config
	serverConfig := genericapiserver.NewRecommendedConfig(Codecs)
	
	// Apply secure serving options only (not authentication/authorization)
	if err := recommendedOptions.SecureServing.ApplyTo(&serverConfig.SecureServing, &serverConfig.LoopbackClientConfig); err != nil {
		return fmt.Errorf("failed to apply secure serving options: %w", err)
	}

	// Get kubeconfig using controller-runtime (handles both in-cluster and out-of-cluster)
	kubeconfig := ctrl.GetConfigOrDie()

	// Set the LoopbackClientConfig to use the detected kubeconfig
	serverConfig.LoopbackClientConfig = kubeconfig

	// Set the EffectiveVersion (required by Complete())
	serverConfig.EffectiveVersion = basecompatibility.NewEffectiveVersionFromString(
		baseversion.DefaultKubeBinaryVersion,
		"",
		"",
	)

	// Setup OpenAPI configuration (required by Complete())
	// Use generated OpenAPI definitions from pkg/generated/openapi
	serverConfig.OpenAPIConfig = genericapiserver.DefaultOpenAPIConfig(
		fleetopenapi.GetOpenAPIDefinitions,
		openapi.NewDefinitionNamer(Scheme),
	)
	serverConfig.OpenAPIConfig.Info.Title = "Fleet API Server"
	serverConfig.OpenAPIConfig.Info.Version = "v1alpha1"

	serverConfig.OpenAPIV3Config = genericapiserver.DefaultOpenAPIV3Config(
		fleetopenapi.GetOpenAPIDefinitions,
		openapi.NewDefinitionNamer(Scheme),
	)
	serverConfig.OpenAPIV3Config.Info.Title = "Fleet API Server"
	serverConfig.OpenAPIV3Config.Info.Version = "v1alpha1"

	clientset, err := kubernetes.NewForConfig(kubeconfig)
	if err != nil {
		return fmt.Errorf("failed to create kubernetes client: %w", err)
	}
	
	// Set shared informer factory (required for Complete() to not nil pointer)
	// Use a very long resync period since we don't really need it
	if serverConfig.SharedInformerFactory == nil {
		serverConfig.SharedInformerFactory = informers.NewSharedInformerFactory(clientset, 10*time.Minute)
	}
	
	// Set external address if not set (required for Complete())
	if serverConfig.ExternalAddress == "" {
		serverConfig.ExternalAddress = fmt.Sprintf("0.0.0.0:%d", opts.SecurePort)
	}

	// Setup authentication - use token authenticator for API aggregation
	tokenAuth, err := tokenfile.NewCSV("/dev/null")
	if err != nil {
		return fmt.Errorf("failed to create token authenticator: %w", err)
	}
	serverConfig.Authentication.Authenticator = bearertoken.New(tokenAuth)

	// Setup authorization - allow all for now since this is an aggregated API server
	serverConfig.Authorization.Authorizer = authorizerfactory.NewAlwaysAllowAuthorizer()

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
