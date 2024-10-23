package helmcache

import (
	"context"
	"errors"

	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/watch"
	applycorev1 "k8s.io/client-go/applyconfigurations/core/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// SecretClient implements methods to handle secrets. It maps to the controller runtime client.
// Keep options in sync with helm's pkg/storage/driver/secrets.go
type SecretClient struct {
	cache     client.Client
	namespace string
}

var _ corev1.SecretInterface = &SecretClient{}

func NewSecretClient(cache client.Client, namespace string) *SecretClient {
	return &SecretClient{cache, namespace}
}

// Create creates a secret using a k8s client that calls the Kubernetes API server
func (s *SecretClient) Create(ctx context.Context, secret *v1.Secret, _ metav1.CreateOptions) (*v1.Secret, error) {
	secret.Namespace = s.namespace
	return secret, s.cache.Create(ctx, secret)
}

// Update updates a secret using a k8s client that calls the Kubernetes API server
func (s *SecretClient) Update(ctx context.Context, secret *v1.Secret, _ metav1.UpdateOptions) (*v1.Secret, error) {
	return secret, s.cache.Update(ctx, secret)
}

// Delete deletes a secret using a k8s client that calls the Kubernetes API server
func (s *SecretClient) Delete(ctx context.Context, name string, _ metav1.DeleteOptions) error {
	secret := &v1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: s.namespace,
			Name:      name,
		},
	}
	return s.cache.Delete(ctx, secret)
}

// Get gets a secret from the cache.
func (s *SecretClient) Get(ctx context.Context, name string, _ metav1.GetOptions) (*v1.Secret, error) {
	secret := &v1.Secret{}
	err := s.cache.Get(ctx, types.NamespacedName{Namespace: s.namespace, Name: name}, secret)
	return secret, err
}

// List lists secrets from the cache.
func (s *SecretClient) List(ctx context.Context, opts metav1.ListOptions) (*v1.SecretList, error) {
	labels, err := labels.Parse(opts.LabelSelector)
	if err != nil {
		return nil, err
	}
	secrets := v1.SecretList{}
	err = s.cache.List(ctx, &secrets, &client.ListOptions{
		Namespace:     s.namespace,
		LabelSelector: labels,
	})
	if err != nil {
		return nil, err
	}

	return &secrets, nil
}

// DeleteCollection deletes a secret collection using a k8s client that calls the Kubernetes API server
func (s *SecretClient) DeleteCollection(_ context.Context, _ metav1.DeleteOptions, _ metav1.ListOptions) error {
	return errors.New("not implemented")
}

// Watch watches a secret using a k8s client that calls the Kubernetes API server
func (s *SecretClient) Watch(_ context.Context, _ metav1.ListOptions) (watch.Interface, error) {
	return nil, errors.New("not implemented")
}

// Patch patches a secret using a k8s client that calls the Kubernetes API server
func (s *SecretClient) Patch(_ context.Context, _ string, _ types.PatchType, _ []byte, _ metav1.PatchOptions, _ ...string) (*v1.Secret, error) {
	return nil, errors.New("not implemented")
}

// Apply applies a secret using a k8s client that calls the Kubernetes API server
func (s *SecretClient) Apply(_ context.Context, _ *applycorev1.SecretApplyConfiguration, _ metav1.ApplyOptions) (*v1.Secret, error) {
	return nil, errors.New("not implemented")
}
