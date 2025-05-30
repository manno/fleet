package agent

import (
	"context"
	"fmt"

	"github.com/rancher/fleet/internal/cmd/controller/agentmanagement/controllers/resources"
	"github.com/rancher/fleet/internal/config"
	"github.com/rancher/fleet/internal/names"
	fleet "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	networkv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

var (
	scheme  = runtime.NewScheme()
	dScheme = runtime.NewScheme()
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(dScheme))
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(fleet.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

type AgentDeploymentOpts struct {
	//KubeconfigFile   string `usage:"Path to file containing an admin kubeconfig for the downstream cluster" short:"k"`
	KubeconfigSecretName string `usage:"Existing secret for the admin kubeconfig to the downstream cluster" short:"s"`
	// SystemNamespace is the namespace, the agent runs in, e.g. cattle-fleet-local-system
	SystemNamespace string `usage:"System namespace of the downstream cluster" short:"d" default:"cattle-fleet-system"`
}

// Register re-uses an existing kubeconfig for downstream and creates a cluster
// resource, cluster registration and request service account on the upstream
// cluster.
func RegisterOnUpstream(ctx context.Context, cl client.Client, opts ClusterOpts, dOpts AgentDeploymentOpts) error {
	// if not existing,create the namespace
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: opts.ClusterRegistrationNamespace,
		},
	}
	if err := cl.Create(ctx, ns); client.IgnoreAlreadyExists(err) != nil {
		return err
	}

	// exit if this exists, needs to be unique
	clusterNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: opts.ClusterNamespace,
		},
	}
	if err := cl.Create(ctx, clusterNS); err != nil {
		return err
	}

	// TODO if uf.KubeconfigFile != "" {
	//	---
	//
	// apiVersion: v1
	// kind: Secret
	// metadata:
	//	name: kbcfg-downstream21
	//	namespace: fleet-default
	// data:
	//	value: LS0t

	// FIXME: skip this, we assume it exists already as this code is only used to
	// clone existing clusters..
	// kubeconfigSecret := &corev1.Secret{
	// 	ObjectMeta: metav1.ObjectMeta{},
	// 	Data:       map[string][]byte{},
	// }
	// if err := cl.Create(ctx, kubeconfigSecret); err != nil {
	// 	return err
	// }

	cluster := &fleet.Cluster{
		ObjectMeta: metav1.ObjectMeta{
			Name:      opts.ClusterName,
			Namespace: opts.ClusterRegistrationNamespace,
			Labels: map[string]string{
				"name":                       opts.ClusterName,
				fleet.ClusterManagementLabel: "CLI",
			},
		},
		Spec: fleet.ClusterSpec{
			KubeConfigSecret: dOpts.KubeconfigSecretName,
		},
	}
	if err := cl.Create(ctx, cluster); err != nil {
		return fmt.Errorf("cannot create %v: %w", cluster, err)
	}

	statusPatch := client.MergeFrom(cluster.DeepCopy())
	cluster.Status = fleet.ClusterStatus{
		Namespace: opts.ClusterNamespace,
	}
	if err := cl.Status().Patch(ctx, cluster, statusPatch); err != nil {
		return fmt.Errorf("cannot update status of %v: %w", cluster, err)
	}

	request := &fleet.ClusterRegistration{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "request-" + opts.ClusterName,
			Namespace: opts.ClusterRegistrationNamespace,
			Labels: map[string]string{
				"name":                       opts.ClusterName,
				fleet.ClusterManagementLabel: "CLI",
			},
		},
		Spec: fleet.ClusterRegistrationSpec{
			ClientID:     "abcde",
			ClientRandom: "abcde",
		},
	}
	if err := controllerutil.SetControllerReference(cluster, request, scheme); err != nil {
		return fmt.Errorf("cannot set owner of %v to %v: %w", request, cluster, err)
	}
	if err := cl.Create(ctx, request); err != nil {
		return err
	}
	request.Status = fleet.ClusterRegistrationStatus{
		Granted:     true,
		ClusterName: opts.ClusterName,
	}
	if err := cl.Status().Update(ctx, request); err != nil {
		return err
	}

	saName := names.SafeConcatName(request.Name, string(request.UID))

	// TODO each of those owned by request
	// if err := controllerutil.SetControllerReference(request, obj, scheme); err != nil {
	// 	return
	// }
	objs := []client.Object{
		&corev1.ServiceAccount{
			ObjectMeta: metav1.ObjectMeta{
				Name:      saName,
				Namespace: opts.ClusterNamespace,
				Labels: map[string]string{
					fleet.ManagedLabel: "true",
				},
				Annotations: map[string]string{
					fleet.ClusterAnnotation:                      cluster.Name,
					fleet.ClusterRegistrationAnnotation:          request.Name,
					fleet.ClusterRegistrationNamespaceAnnotation: opts.ClusterRegistrationNamespace,
				},
			},
		},
		&rbacv1.Role{
			ObjectMeta: metav1.ObjectMeta{
				Name:      request.Name,
				Namespace: opts.ClusterRegistrationNamespace,
				Labels: map[string]string{
					fleet.ManagedLabel: "true",
				},
			},
			Rules: []rbacv1.PolicyRule{
				{
					Verbs:         []string{"patch"},
					APIGroups:     []string{fleet.SchemeGroupVersion.Group},
					Resources:     []string{fleet.ClusterResourceNamePlural + "/status"},
					ResourceNames: []string{cluster.Name},
				},
			},
		},
		&rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      request.Name,
				Namespace: opts.ClusterRegistrationNamespace,
				Labels: map[string]string{
					fleet.ManagedLabel: "true",
				},
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      saName,
					Namespace: opts.ClusterNamespace,
				},
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: rbacv1.GroupName,
				Kind:     "Role",
				Name:     request.Name,
			},
		},
		// cluster role "fleet-bundle-deployment" created when
		// fleet-controller starts
		&rbacv1.RoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name:      request.Name,
				Namespace: opts.ClusterNamespace,
				Labels: map[string]string{
					fleet.ManagedLabel: "true",
				},
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      saName,
					Namespace: opts.ClusterNamespace,
				},
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: rbacv1.GroupName,
				Kind:     "ClusterRole",
				Name:     resources.BundleDeploymentClusterRole,
			},
		},
		// cluster role "fleet-content" created when fleet-controller
		// starts
		&rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{
				Name: names.SafeConcatName(request.Name, "content"),
				Labels: map[string]string{
					fleet.ManagedLabel: "true",
				},
			},
			Subjects: []rbacv1.Subject{
				{
					Kind:      "ServiceAccount",
					Name:      saName,
					Namespace: opts.ClusterNamespace,
				},
			},
			RoleRef: rbacv1.RoleRef{
				APIGroup: rbacv1.GroupName,
				Kind:     "ClusterRole",
				Name:     resources.ContentClusterRole,
			},
		},
	}
	for _, obj := range objs {
		if obj.GetNamespace() == request.GetNamespace() {
			if err := controllerutil.SetControllerReference(request, obj, scheme); err != nil {
				return fmt.Errorf("cannot set owner of %v to %v: %w", obj, request, err)
			}
		}

		if err := cl.Create(ctx, obj); err != nil {
			return err
		}
	}

	return nil
}

// Deploy creates the agent manifest on downstream
// ucl is the upstream client, used to read the downstreamKubeconfig from the cluster's secret. It is used to build a client to access the downstream cluster. Instead of this client, we could/should pass the kubecofnig directly.
// uCfg is the rest.Config for the upstream cluster. The agent will later use it to connect to the upstream cluster.
func Deploy(ctx context.Context, ucl client.Client, uCfg *rest.Config, opts ClusterOpts, dOpts AgentDeploymentOpts) error {
	// read kubeconfig from secret on upstream cluster
	ksec := &corev1.Secret{}
	ucl.Get(ctx, client.ObjectKey{Namespace: opts.ClusterRegistrationNamespace, Name: dOpts.KubeconfigSecretName}, ksec)

	// FIXME this config could use an internal API server, add another flag
	// build downstream client
	downstreamCfg, err := clientcmd.NewClientConfigFromBytes(ksec.Data["value"])
	if err != nil {
		return err
	}

	dcfg, err := downstreamCfg.ClientConfig()
	if err != nil {
		return err
	}

	dcl, err := client.New(dcfg, client.Options{Scheme: dScheme})
	if err != nil {
		return fmt.Errorf("failed to create downstream client: %w", err)
	}

	// agent namespace
	// bootstrap secret (not)
	// fleet-agent secret
	// agentConfig config map
	if err := dcl.Create(ctx, &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: dOpts.SystemNamespace},
	}); err != nil {
		return fmt.Errorf("failed to create agent's namespace %s: %w", dOpts.SystemNamespace, err)
	}

	// convert kubernetes config rest.Config into a string
	value, err := newKcfg(uCfg, opts.ClusterNamespace)
	if err != nil {
		return fmt.Errorf("failed to create cluster's downstram kubeconfig: %w", err)
	}

	// upstream kubeconfig
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      DefaultName,
			Namespace: dOpts.SystemNamespace,
		},
		StringData: map[string]string{
			// Kubeconfig for upstream, from request-123
			Kubeconfig:          string(value),
			DeploymentNamespace: opts.ClusterNamespace,
			ClusterNamespace:    opts.ClusterRegistrationNamespace,
			ClusterName:         opts.ClusterName,
		},
	}
	if err := dcl.Create(ctx, secret); err != nil {
		return err
	}

	cm, err := config.ToConfigMap(dOpts.SystemNamespace, DefaultName, config.DefaultConfig())
	if err != nil {
		return err
	}
	if err := dcl.Create(ctx, cm); err != nil {
		return err
	}

	// agent.Manifest():
	// admin service account
	// cluster role
	// cluster role binding
	// network policy
	// deployment (not) opts.AgentReplicas=0
	admin := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      DefaultName,
			Namespace: dOpts.SystemNamespace,
		},
	}
	if err := dcl.Create(ctx, admin); err != nil {
		return err
	}

	defaultSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      DefaultName,
			Namespace: "default",
		},
		AutomountServiceAccountToken: new(bool),
	}
	if err := dcl.Create(ctx, defaultSA); client.IgnoreAlreadyExists(err) != nil {
		return err
	}

	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: names.SafeConcatName(dOpts.SystemNamespace, DefaultName, "role"),
		},
		Rules: []rbacv1.PolicyRule{
			{
				Verbs:     []string{rbacv1.VerbAll},
				APIGroups: []string{rbacv1.APIGroupAll},
				Resources: []string{rbacv1.ResourceAll},
			},
			{
				Verbs:           []string{rbacv1.VerbAll},
				NonResourceURLs: []string{rbacv1.ResourceAll},
			},
		},
	}
	if err := dcl.Create(ctx, cr); client.IgnoreAlreadyExists(err) != nil {
		return err
	}

	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name: names.SafeConcatName(dOpts.SystemNamespace, DefaultName, "role", "binding"),
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      DefaultName,
				Namespace: dOpts.SystemNamespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacv1.GroupName,
			Kind:     "ClusterRole",
			Name:     names.SafeConcatName(dOpts.SystemNamespace, DefaultName, "role"),
		},
	}
	if err := dcl.Create(ctx, crb); client.IgnoreAlreadyExists(err) != nil {
		return err
	}

	networkPolicy := &networkv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-allow-all",
			Namespace: dOpts.SystemNamespace,
		},
		Spec: networkv1.NetworkPolicySpec{
			PolicyTypes: []networkv1.PolicyType{
				networkv1.PolicyTypeIngress,
				networkv1.PolicyTypeEgress,
			},
			Ingress: []networkv1.NetworkPolicyIngressRule{
				{},
			},
			Egress: []networkv1.NetworkPolicyEgressRule{
				{},
			},
			PodSelector: metav1.LabelSelector{},
		},
	}
	if err := dcl.Create(ctx, networkPolicy); err != nil {
		return err
	}

	// TODO agent.agentApp(namespace string, agentScope string, opts ManifestOptions) *appsv1.Deployment
	// but we will run this outside the cluster...

	return nil
}

func newKcfg(r *rest.Config, ns string) ([]byte, error) {
	clusters := make(map[string]*clientcmdapi.Cluster)
	clusters["default-cluster"] = &clientcmdapi.Cluster{
		Server:                   r.Host,
		CertificateAuthorityData: r.CAData,
	}
	contexts := make(map[string]*clientcmdapi.Context)
	contexts["default-context"] = &clientcmdapi.Context{
		Cluster:   "default-cluster",
		AuthInfo:  "default-user",
		Namespace: ns,
	}
	authinfos := make(map[string]*clientcmdapi.AuthInfo)
	authinfos["default-user"] = &clientcmdapi.AuthInfo{
		ClientCertificateData: r.CertData,
		ClientKeyData:         r.KeyData,
	}
	clientConfig := clientcmdapi.Config{
		Kind:           "Config",
		APIVersion:     "v1",
		Clusters:       clusters,
		Contexts:       contexts,
		CurrentContext: "default-context",
		AuthInfos:      authinfos,
	}
	return clientcmd.Write(clientConfig)
}
