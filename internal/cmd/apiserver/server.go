package apiserver

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	"k8s.io/apiserver/pkg/endpoints/openapi"
	"k8s.io/apiserver/pkg/registry/rest"
	genericapiserver "k8s.io/apiserver/pkg/server"
	"k8s.io/apiserver/pkg/server/options"
	basecompatibility "k8s.io/component-base/compatibility"
	baseversion "k8s.io/component-base/version"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/rancher/fleet/internal/cmd/apiserver/storage"
	storagev1alpha1 "github.com/rancher/fleet/pkg/apis/storage.fleet.cattle.io/v1alpha1"
	fleetopenapi "github.com/rancher/fleet/pkg/generated/openapi"
)

var (
	// Scheme defines methods for serializing and deserializing API objects
	Scheme = runtime.NewScheme()
	// Codecs provides methods for retrieving codecs and serializers for specific groups and versions
	Codecs = serializer.NewCodecFactory(Scheme)
)

func init() {
	// Register the storage.fleet.cattle.io types with the scheme
	if err := storagev1alpha1.AddToScheme(Scheme); err != nil {
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
	recommendedOptions := options.NewRecommendedOptions("", Codecs.LegacyCodec(storagev1alpha1.SchemeGroupVersion))
	recommendedOptions.SecureServing.BindPort = opts.SecurePort
	recommendedOptions.SecureServing.ServerCert.CertDirectory = opts.CertDir
	// Disable etcd since we're using SQLite
	recommendedOptions.Etcd = nil
	// Disable admission since we don't need it for this aggregated API server
	recommendedOptions.Admission = nil
	// Disable authentication and authorization to avoid requiring in-cluster config
	// When deployed in-cluster with proper APIService, the main kube-apiserver handles this
	recommendedOptions.Authentication = nil
	recommendedOptions.Authorization = nil
	// Disable CoreAPI since we don't need it
	recommendedOptions.CoreAPI = nil
	// Disable priority and fairness feature
	recommendedOptions.Features.EnablePriorityAndFairness = false

	// Get kubeconfig using controller-runtime (handles both in-cluster and out-of-cluster)
	kubeconfig := ctrl.GetConfigOrDie()

	// Create server config
	serverConfig := genericapiserver.NewRecommendedConfig(Codecs)

	// Set the LoopbackClientConfig to use the detected kubeconfig
	serverConfig.LoopbackClientConfig = kubeconfig

	// Setup OpenAPI configuration (required by Complete())
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

	// Set the EffectiveVersion (required by Complete())
	serverConfig.EffectiveVersion = basecompatibility.NewEffectiveVersionFromString(
		baseversion.DefaultKubeBinaryVersion,
		"",
		"",
	)

	// Apply recommended options (handles auth, authz, loopback client, etc.)
	if err := recommendedOptions.ApplyTo(serverConfig); err != nil {
		return fmt.Errorf("failed to apply recommended options: %w", err)
	}

	// Create generic API server
	genericServer, err := serverConfig.Complete().New("fleet-apiserver", genericapiserver.NewEmptyDelegate())
	if err != nil {
		return fmt.Errorf("failed to create generic API server: %w", err)
	}

	// Install the API group
	apiGroupInfo := genericapiserver.NewDefaultAPIGroupInfo(storagev1alpha1.SchemeGroupVersion.Group, Scheme, metav1.ParameterCodec, Codecs)

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

	// Start the server
	return genericServer.PrepareRun().Run(ctx.Done())
}
