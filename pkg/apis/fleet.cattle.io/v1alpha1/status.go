package v1alpha1

import (
	corev1 "k8s.io/api/core/v1"
)

// GenericCondition is a copy of wrangler's GenericCondition to avoid external dependencies in OpenAPI generation.
// This allows the Fleet API server to generate complete OpenAPI definitions without requiring
// the wrangler package to have openapi-gen markers.
// +k8s:openapi-gen=true
type GenericCondition struct {
	// Type of condition.
	Type string `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// The last time this condition was updated.
	LastUpdateTime string `json:"lastUpdateTime,omitempty"`
	// Last time the condition transitioned from one status to another.
	LastTransitionTime string `json:"lastTransitionTime,omitempty"`
	// The reason for the condition's last transition.
	Reason string `json:"reason,omitempty"`
	// Human-readable message indicating details about last transition
	Message string `json:"message,omitempty"`
}

type StatusBase struct {
	// ReadyClusters is the lowest number of clusters that are ready over
	// all the bundles of this resource.
	// +optional
	ReadyClusters int `json:"readyClusters"`
	// DesiredReadyClusters	is the number of clusters that should be ready for bundles of this resource.
	// +optional
	DesiredReadyClusters int `json:"desiredReadyClusters"`
	// Summary contains the number of bundle deployments in each state and a list of non-ready resources.
	Summary BundleSummary `json:"summary,omitempty"`
	// Display contains a human readable summary of the status.
	Display StatusDisplay `json:"display,omitempty"`
	// Conditions is a list of Wrangler conditions that describe the state
	// of the resource.
	Conditions []GenericCondition `json:"conditions,omitempty"`
	// Resources contains metadata about the resources of each bundle.
	Resources []Resource `json:"resources,omitempty"`
	// ResourceCounts contains the number of resources in each state over all bundles.
	ResourceCounts ResourceCounts `json:"resourceCounts,omitempty"`
	// PerClusterResourceCounts contains the number of resources in each state over all bundles, per cluster.
	PerClusterResourceCounts map[string]*ResourceCounts `json:"perClusterResourceCounts,omitempty"`
}

type StatusDisplay struct {
	// ReadyBundleDeployments is a string in the form "%d/%d", that describes the
	// number of ready bundledeployments over the total number of bundledeployments.
	ReadyBundleDeployments string `json:"readyBundleDeployments,omitempty"`
	// State is the state of the resource, e.g. "GitUpdating" or the maximal
	// BundleState according to StateRank.
	State string `json:"state,omitempty"`
	// Message contains the relevant message from the deployment conditions.
	Message string `json:"message,omitempty"`
	// Error is true if a message is present.
	Error bool `json:"error,omitempty"`
}
