package apiserver

import (
	"context"
	"fmt"

	"github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	apiregistrationv1 "k8s.io/kube-aggregator/pkg/apis/apiregistration/v1"
	"k8s.io/kube-aggregator/pkg/client/clientset_generated/clientset"
)

// configureAPIService updates the auto-managed APIService to point to our aggregation server
func configureAPIService(ctx context.Context, config *rest.Config, namespace, serviceName string) error {
	// Create aggregation client
	aggregatorClient, err := clientset.NewForConfig(config)
	if err != nil {
		return fmt.Errorf("failed to create aggregator client: %w", err)
	}

	apiServiceName := "v1alpha1.storage.fleet.cattle.io"

	// Get the existing APIService (auto-created by Kubernetes for CRDs)
	apiService, err := aggregatorClient.ApiregistrationV1().APIServices().Get(ctx, apiServiceName, metav1.GetOptions{})
	if err != nil {
		if errors.IsNotFound(err) {
			logrus.Info("APIService does not exist yet, will be created by Kubernetes")
			return nil
		}
		return fmt.Errorf("failed to get APIService: %w", err)
	}

	// Update the APIService to point to our service
	apiService.Spec.Service = &apiregistrationv1.ServiceReference{
		Namespace: namespace,
		Name:      serviceName,
	}
	apiService.Spec.InsecureSkipTLSVerify = true

	// Remove the automanaged label so Kubernetes doesn't revert our changes
	if apiService.Labels == nil {
		apiService.Labels = make(map[string]string)
	}
	delete(apiService.Labels, "kube-aggregator.kubernetes.io/automanaged")

	_, err = aggregatorClient.ApiregistrationV1().APIServices().Update(ctx, apiService, metav1.UpdateOptions{})
	if err != nil {
		return fmt.Errorf("failed to update APIService: %w", err)
	}

	logrus.Infof("Successfully configured APIService %s to use service %s/%s", apiServiceName, namespace, serviceName)
	return nil
}
