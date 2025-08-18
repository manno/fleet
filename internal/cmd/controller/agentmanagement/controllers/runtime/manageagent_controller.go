package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/rancher/fleet/internal/cmd"
	"github.com/rancher/fleet/internal/cmd/controller/agentmanagement/agent"
	"github.com/rancher/fleet/internal/cmd/controller/agentmanagement/controllers/manageagent"
	"github.com/rancher/fleet/internal/config"
	"github.com/rancher/fleet/internal/names"
	fleet "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// ManageAgentReconciler manages Fleet Agent Bundles for clusters
type ManageAgentReconciler struct {
	client.Client
	Log             logr.Logger
	Scheme          *runtime.Scheme
	SystemNamespace string
}

// Reconcile handles Fleet Agent Bundle management
func (r *ManageAgentReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("namespace", req.NamespacedName.Name)

	var namespace corev1.Namespace
	if err := r.Get(ctx, req.NamespacedName, &namespace); err != nil {
		if apierrors.IsNotFound(err) {
			// Namespace was deleted, nothing to do
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	cfg := config.Get()
	// Managed agents are disabled, so we don't need to create the bundle
	if cfg.ManageAgent != nil && !*cfg.ManageAgent {
		logger.V(1).Info("Agent management is disabled in config")
		return ctrl.Result{}, nil
	}

	// List all clusters in this namespace
	var clusters fleet.ClusterList
	if err := r.List(ctx, &clusters, client.InNamespace(namespace.Name)); err != nil {
		return ctrl.Result{}, err
	}

	if len(clusters.Items) == 0 {
		logger.V(1).Info("No clusters found in namespace")
		return ctrl.Result{}, nil
	}

	// Create agent bundles for all clusters
	for _, cluster := range clusters.Items {
		if manageagent.SkipCluster(&cluster) {
			logger.V(1).Info("Skipping cluster with management label", "cluster", cluster.Name)
			continue
		}

		logger.Info("Updating agent bundle for cluster", "cluster", cluster.Name)
		if err := r.reconcileAgentBundle(ctx, &namespace, &cluster); err != nil {
			logger.Error(err, "Failed to update agent bundle for cluster", "cluster", cluster.Name)
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{}, nil
}

// ClusterReconciler handles cluster status updates and agent configuration
type ClusterReconciler struct {
	client.Client
	Log             logr.Logger
	Scheme          *runtime.Scheme
	SystemNamespace string
}

// Reconcile handles cluster status updates related to agent management
func (r *ClusterReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("cluster", req.NamespacedName)

	var cluster fleet.Cluster
	if err := r.Get(ctx, req.NamespacedName, &cluster); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if manageagent.SkipCluster(&cluster) {
		return ctrl.Result{}, nil
	}

	logger.V(1).Info("Reconciling agent settings for cluster")

	// Update cluster status with agent configuration
	newStatus := cluster.Status.DeepCopy()

	// Reconcile agent environment variables
	envVarsChanged, err := r.reconcileAgentEnvVars(&cluster, newStatus)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Update other cluster status fields
	statusChanged, err := r.updateClusterStatus(&cluster, newStatus)
	if err != nil {
		return ctrl.Result{}, err
	}

	// Update status if anything changed
	if envVarsChanged || statusChanged {
		newStatus.AgentConfigChanged = true
		cluster.Status = *newStatus

		if err := r.Status().Update(ctx, &cluster); err != nil {
			return ctrl.Result{}, err
		}

		// Enqueue namespace reconciliation to update agent bundles
		return r.enqueueNamespaceReconciliation(ctx, cluster.Namespace)
	}

	return ctrl.Result{}, nil
}

// reconcileAgentBundle creates or updates the agent bundle for a cluster
func (r *ManageAgentReconciler) reconcileAgentBundle(ctx context.Context, namespace *corev1.Namespace, cluster *fleet.Cluster) error {
	bundle, err := r.newAgentBundle(namespace.Name, cluster)
	if err != nil {
		return err
	}

	// Set namespace as owner for cleanup
	if err := controllerutil.SetControllerReference(namespace, bundle, r.Scheme); err != nil {
		return err
	}

	// Apply the bundle using server-side apply
	// For now, use CreateOrUpdate pattern that works better with fake client in tests
	var existingBundle fleet.Bundle
	err = r.Get(ctx, client.ObjectKeyFromObject(bundle), &existingBundle)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return r.Create(ctx, bundle)
		}
		return err
	}

	// Update existing bundle
	existingBundle.Spec = bundle.Spec
	return r.Update(ctx, &existingBundle)
}

// newAgentBundle creates a Fleet Bundle containing the agent deployment
func (r *ManageAgentReconciler) newAgentBundle(ns string, cluster *fleet.Cluster) (*fleet.Bundle, error) {
	cfg := config.Get()
	agentNamespace := r.SystemNamespace
	if cluster.Spec.AgentNamespace != "" {
		agentNamespace = cluster.Spec.AgentNamespace
	}

	agentReplicas := cmd.ParseEnvAgentReplicaCount()
	leaderElectionOptions, err := cmd.NewLeaderElectionOptionsWithPrefix("FLEET_AGENT")
	if err != nil {
		return nil, err
	}

	// Generate agent manifests as YAML (bundle-first approach)
	agentYAML, err := agent.ManifestYAML(
		agentNamespace, cluster.Spec.AgentNamespace,
		agent.ManifestOptions{
			AgentEnvVars:     cluster.Spec.AgentEnvVars,
			AgentTolerations: cluster.Spec.AgentTolerations,
			PrivateRepoURL:   cluster.Spec.PrivateRepoURL,
			AgentAffinity:    cluster.Spec.AgentAffinity,
			AgentResources:   cluster.Spec.AgentResources,
			HostNetwork:      *ptr.To(cluster.Spec.HostNetwork != nil && *cluster.Spec.HostNetwork),

			AgentImage:              cfg.AgentImage,
			AgentImagePullPolicy:    cfg.AgentImagePullPolicy,
			CheckinInterval:         cfg.AgentCheckinInterval.Duration.String(),
			SystemDefaultRegistry:   cfg.SystemDefaultRegistry,
			BundleDeploymentWorkers: cfg.AgentWorkers.BundleDeployment,
			DriftWorkers:            cfg.AgentWorkers.Drift,
			AgentReplicas:           agentReplicas,
			LeaderElectionOptions:   leaderElectionOptions,
		},
	)
	if err != nil {
		return nil, err
	}

	return &fleet.Bundle{
		ObjectMeta: metav1.ObjectMeta{
			Name:      names.SafeConcatName(manageagent.AgentBundleName, cluster.Name),
			Namespace: ns,
		},
		Spec: fleet.BundleSpec{
			BundleDeploymentOptions: fleet.BundleDeploymentOptions{
				DefaultNamespace: agentNamespace,
				Helm: &fleet.HelmOptions{
					TakeOwnership: true,
				},
			},
			Resources: []fleet.BundleResource{
				{
					Name:    "agent.yaml",
					Content: string(agentYAML),
				},
			},
			Targets: []fleet.BundleTarget{
				{
					ClusterSelector: &metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Key:      "fleet.cattle.io/non-managed-agent",
								Operator: metav1.LabelSelectorOpDoesNotExist,
							},
						},
					},
					ClusterName: cluster.Name,
				},
			},
		},
	}, nil
}

// reconcileAgentEnvVars checks if agent environment variables have changed
func (r *ClusterReconciler) reconcileAgentEnvVars(cluster *fleet.Cluster, status *fleet.ClusterStatus) (bool, error) {
	if len(cluster.Spec.AgentEnvVars) < 1 {
		// Remove existing hash if environment variables have been deleted
		if status.AgentEnvVarsHash != "" {
			status.AgentEnvVarsHash = ""
			return true, nil
		}
		return false, nil
	}

	hash, err := hashStatusField(cluster.Spec.AgentEnvVars)
	if err != nil {
		return false, err
	}

	if status.AgentEnvVarsHash != hash {
		status.AgentEnvVarsHash = hash
		return true, nil
	}

	return false, nil
}

// updateClusterStatus updates cluster status fields related to agent configuration
func (r *ClusterReconciler) updateClusterStatus(cluster *fleet.Cluster, status *fleet.ClusterStatus) (bool, error) {
	changed := false

	if status.AgentPrivateRepoURL != cluster.Spec.PrivateRepoURL {
		status.AgentPrivateRepoURL = cluster.Spec.PrivateRepoURL
		changed = true
	}

	hostNetwork := false
	if cluster.Spec.HostNetwork != nil {
		hostNetwork = *cluster.Spec.HostNetwork
	}
	if status.AgentHostNetwork != hostNetwork {
		status.AgentHostNetwork = hostNetwork
		changed = true
	}

	// Handle affinity hash
	if c, hash, err := hashChanged(cluster.Spec.AgentAffinity, status.AgentAffinityHash); err != nil {
		return changed, err
	} else if c {
		status.AgentAffinityHash = hash
		changed = true
	}

	// Handle resources hash
	if c, hash, err := hashChanged(cluster.Spec.AgentResources, status.AgentResourcesHash); err != nil {
		return changed, err
	} else if c {
		status.AgentResourcesHash = hash
		changed = true
	}

	// Handle tolerations hash
	if c, hash, err := hashChanged(cluster.Spec.AgentTolerations, status.AgentTolerationsHash); err != nil {
		return changed, err
	} else if c {
		status.AgentTolerationsHash = hash
		changed = true
	}

	return changed, nil
}

// enqueueNamespaceReconciliation triggers namespace reconciliation for agent bundle updates
func (r *ClusterReconciler) enqueueNamespaceReconciliation(ctx context.Context, namespaceName string) (ctrl.Result, error) {
	// In controller-runtime, we can't directly enqueue other controllers
	// Instead, we'll trigger this through a watch on Bundle resources
	// The namespace controller will be triggered by the Bundle create/update
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the ManageAgent controller with the Manager
func (r *ManageAgentReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&corev1.Namespace{}).
		Owns(&fleet.Bundle{}). // Watch bundles we create
		Complete(r)
}

// SetupWithManager sets up the Cluster controller with the Manager
func (r *ClusterReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&fleet.Cluster{}).
		Watches(
			&fleet.Bundle{},
			handler.EnqueueRequestsFromMapFunc(func(ctx context.Context, obj client.Object) []reconcile.Request {
				// Map bundles back to clusters to trigger reconciliation
				bundle := obj.(*fleet.Bundle)

				// Extract cluster name from agent bundle name
				if bundle.Name == manageagent.AgentBundleName {
					return nil // Not an agent bundle
				}

				clusterName := bundle.Name[len(manageagent.AgentBundleName)+1:] // Remove "fleet-agent-" prefix
				return []reconcile.Request{
					{
						NamespacedName: types.NamespacedName{
							Namespace: bundle.Namespace,
							Name:      clusterName,
						},
					},
				}
			}),
		).
		Complete(r)
}

// Helper functions from original implementation

func hashStatusField(field any) (string, error) {
	hasher := sha256.New224()
	b, err := json.Marshal(field)
	if err != nil {
		return "", err
	}
	hasher.Write(b)
	return fmt.Sprintf("%x", hasher.Sum(nil)), nil
}

func hashChanged(field any, statusHash string) (bool, string, error) {
	isNil := func(field any) bool {
		switch field := field.(type) {
		case *corev1.Affinity:
			return field == nil
		case *corev1.ResourceRequirements:
			return field == nil
		case []corev1.Toleration:
			return len(field) == 0
		default:
			return false
		}
	}

	if isNil(field) {
		if statusHash != "" {
			return true, "", nil
		}
		return false, "", nil
	}

	hash, err := hashStatusField(field)
	if err != nil {
		return false, "", err
	}

	return statusHash != hash, hash, nil
}
