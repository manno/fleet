package agent

import (
	"context"

	"github.com/rancher/fleet/internal/names"
	fleet "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	DefaultName = "fleet-agent"

	Kubeconfig          = "kubeconfig"
	DeploymentNamespace = "deploymentNamespace"
	ClusterNamespace    = "clusterNamespace"
	ClusterName         = "clusterName"
)

type ClusterOpts struct {
	ClusterName string `usage:"Name of the downstream cluster"`
	// ClusterRegistrationNamespace contains the cluster resource,
	// clusterregistration and bundles. Also the import service account.
	ClusterRegistrationNamespace string `usage:"Registration namespace of the downstream cluster" short:"n" default:"fleet-default"`
	// ClusterNamespace of the format
	// cluster-${namespace}-${cluster}-${random}. It contains the request
	// service account and bundledeployments.
	ClusterNamespace string `usage:"Internal namespace for the downstream cluster's bundledeployments" default:"cluster-fleet-default-name-1234"`
}

func DeleteDownstream(ctx context.Context, cl client.Client, opts ClusterOpts) error {
	// exit if this exists, needs to be unique
	if err := cl.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: opts.ClusterNamespace}}); client.IgnoreNotFound(err) != nil {
		return err
	}

	return nil
}

func Delete(ctx context.Context, cl client.Client, opts ClusterOpts) error {
	// exit if this exists, needs to be unique
	if err := cl.Delete(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: opts.ClusterNamespace}}); client.IgnoreNotFound(err) != nil {
		return err
	}

	cluster := &fleet.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      opts.ClusterName,
			Namespace: opts.ClusterRegistrationNamespace,
		},
	}
	if err := cl.Delete(ctx, cluster); client.IgnoreNotFound(err) != nil {
		return err
	}

	request := &fleet.ClusterRegistration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "request-" + opts.ClusterName,
			Namespace: opts.ClusterRegistrationNamespace,
		},
	}
	_ = cl.Get(ctx, client.ObjectKeyFromObject(request), request)

	if err := cl.Delete(ctx, request); client.IgnoreNotFound(err) != nil {
		return err
	}

	saName := names.SafeConcatName(request.Name, string(request.UID))
	objs := []client.Object{
		&corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      saName,
				Namespace: opts.ClusterNamespace,
			},
		},
		&rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:      request.Name,
				Namespace: opts.ClusterRegistrationNamespace,
			},
		},
		&rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      request.Name,
				Namespace: opts.ClusterRegistrationNamespace,
			},
		},
		&rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      request.Name,
				Namespace: opts.ClusterNamespace,
			},
		},
		&rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: names.SafeConcatName(request.Name, "content"),
			},
		},
	}
	for _, obj := range objs {
		if err := cl.Delete(ctx, obj); client.IgnoreNotFound(err) != nil {
			return err
		}
	}

	// TODO delete on downstream too

	return nil
}
