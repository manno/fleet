package v1alpha1

import (
	fleetv1alpha1 "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func init() {
	InternalSchemeBuilder.Register(&BundleDeployment{}, &BundleDeploymentList{})
}

const (
	BundleDeploymentResourceNamePlural = "bundledeployments"

	// MaxHelmReleaseNameLen is the maximum length of a Helm release name.
	// See https://github.com/helm/helm/blob/293b50c65d4d56187cd4e2f390f0ada46b4c4737/pkg/chartutil/validate_name.go#L54-L61
	MaxHelmReleaseNameLen = 53

	// SecretTypeBundleDeploymentOptions is the type of the secret that stores the deployment values options.
	SecretTypeBundleDeploymentOptions = "fleet.cattle.io/bundle-deployment/v1alpha1"

	BundleDeploymentOwnershipLabel = "fleet.cattle.io/bundledeployment"
	ContentNameLabel               = "fleet.cattle.io/content-name"
)

const IgnoreOp = "ignore"

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Deployed",type=string,JSONPath=`.status.display.deployed`
// +kubebuilder:printcolumn:name="Monitored",type=string,JSONPath=`.status.display.monitored`
// +kubebuilder:printcolumn:name="Status",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].message`

// BundleDeployment is used internally by Fleet and should not be used directly.
// When a Bundle is deployed to a cluster an instance of a Bundle is called a
// BundleDeployment. A BundleDeployment represents the state of that Bundle on
// a specific cluster with its cluster-specific customizations. The Fleet agent
// is only aware of BundleDeployment resources that are created for the cluster
// the agent is managing.
type BundleDeployment struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   fleetv1alpha1.BundleDeploymentSpec   `json:"spec,omitempty"`
	Status fleetv1alpha1.BundleDeploymentStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// BundleDeploymentList contains a list of BundleDeployment
type BundleDeploymentList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BundleDeployment `json:"items"`
}
