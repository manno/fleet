// Package normalizers contains normalizers for resources. Normalizers are used to modify resources before they are compared.
// This includes the "ignore" normalizer, which removes a matched path and the knownTypes normalizer.
//
// +vendored argoproj/argo-cd/util/argo/normalizers/diff_normalizer.go
package normalizers

import (
	"encoding/json"

	jsonpatch "github.com/evanphx/json-patch"
	log "github.com/sirupsen/logrus"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/rancher/fleet/internal/cmd/agent/deployer/internal/diff"
	"github.com/rancher/fleet/internal/cmd/agent/deployer/internal/normalizers/glob"
	"github.com/rancher/fleet/internal/cmd/agent/deployer/internal/resource"
)

type normalizerPatch struct {
	groupKind schema.GroupKind
	namespace string
	name      string
	patch     jsonpatch.Patch
}

type ignoreNormalizer struct {
	patches []normalizerPatch
}

// NewIgnoreNormalizer creates diff normalizer which removes ignored fields according to given application spec and resource overrides
func NewIgnoreNormalizer(ignore []resource.ResourceIgnoreDifferences, overrides map[string]resource.ResourceOverride) (diff.Normalizer, error) {
	// Add default ignore patterns for common Kubernetes resources that are modified by external controllers
	ignore = append(ignore, getDefaultIgnoreDifferences()...)

	for key, override := range overrides {
		group, kind, err := getGroupKindForOverrideKey(key)
		if err != nil {
			log.Warn(err)
		}
		if len(override.IgnoreDifferences.JSONPointers) > 0 {
			ignore = append(ignore, resource.ResourceIgnoreDifferences{
				Group:        group,
				Kind:         kind,
				JSONPointers: override.IgnoreDifferences.JSONPointers,
			})
		}
	}
	patches := make([]normalizerPatch, 0)
	for i := range ignore {
		for _, path := range ignore[i].JSONPointers {
			patchData, err := json.Marshal([]map[string]string{{"op": "remove", "path": path}})
			if err != nil {
				return nil, err
			}
			patch, err := jsonpatch.DecodePatch(patchData)
			if err != nil {
				return nil, err
			}
			patches = append(patches, normalizerPatch{
				groupKind: schema.GroupKind{Group: ignore[i].Group, Kind: ignore[i].Kind},
				name:      ignore[i].Name,
				namespace: ignore[i].Namespace,
				patch:     patch,
			})
		}

	}
	return &ignoreNormalizer{patches: patches}, nil
}

// Normalize removes fields from supplied resource using json paths from matching items of specified resources ignored differences list
func (n *ignoreNormalizer) Normalize(un *unstructured.Unstructured) error {
	matched := make([]normalizerPatch, 0)
	for _, patch := range n.patches {
		groupKind := un.GroupVersionKind().GroupKind()

		if glob.Match(patch.groupKind.Group, groupKind.Group) &&
			glob.Match(patch.groupKind.Kind, groupKind.Kind) &&
			(patch.name == "" || patch.name == un.GetName()) &&
			(patch.namespace == "" || patch.namespace == un.GetNamespace()) {

			matched = append(matched, patch)
		}
	}
	if len(matched) == 0 {
		return nil
	}

	docData, err := json.Marshal(un)
	if err != nil {
		return err
	}

	for _, patch := range matched {
		patchedData, err := patch.patch.Apply(docData)
		if err != nil {
			log.Debugf("Failed to apply normalization: %v", err)
			continue
		}
		docData = patchedData
	}

	err = json.Unmarshal(docData, un)
	if err != nil {
		return err
	}
	return nil
}

// getDefaultIgnoreDifferences returns default ignore patterns for common Kubernetes resources
// that are frequently modified by external controllers.
// These patterns ignore fields that are ALWAYS modified externally and don't indicate drift.
func getDefaultIgnoreDifferences() []resource.ResourceIgnoreDifferences {
	return []resource.ResourceIgnoreDifferences{
		// Service mesh sidecar injectors modify pod template annotations and containers
		// Istio injection
		{
			Group:        "apps",
			Kind:         "Deployment",
			JSONPointers: []string{"/spec/template/metadata/annotations/sidecar.istio.io~1status"},
		},
		{
			Group:        "apps",
			Kind:         "StatefulSet",
			JSONPointers: []string{"/spec/template/metadata/annotations/sidecar.istio.io~1status"},
		},

		// Linkerd injection status
		{
			Group:        "apps",
			Kind:         "Deployment",
			JSONPointers: []string{"/spec/template/metadata/annotations/linkerd.io~1proxy-version"},
		},
		{
			Group:        "apps",
			Kind:         "StatefulSet",
			JSONPointers: []string{"/spec/template/metadata/annotations/linkerd.io~1proxy-version"},
		},

		// Cert-manager modifies certificate status and annotations
		{
			Group:        "cert-manager.io",
			Kind:         "Certificate",
			JSONPointers: []string{"/status"},
		},

		// VPA modifies container resource requests/limits when update mode is Auto
		// Note: This should ideally be conditional on VPA existing, but VPA annotations
		// are added to the target, so we can check for them
		{
			Group: "apps",
			Kind:  "Deployment",
			JSONPointers: []string{
				"/spec/template/metadata/annotations/vpaUpdates",
				"/spec/template/metadata/annotations/vpaObservedContainers",
			},
		},
		{
			Group: "apps",
			Kind:  "StatefulSet",
			JSONPointers: []string{
				"/spec/template/metadata/annotations/vpaUpdates",
				"/spec/template/metadata/annotations/vpaObservedContainers",
			},
		},

		// Cluster autoscaler annotations are updated dynamically
		{
			Group: "apps",
			Kind:  "Deployment",
			JSONPointers: []string{
				"/metadata/annotations/cluster-autoscaler.kubernetes.io~1safe-to-evict",
				"/metadata/annotations/cluster-autoscaler.kubernetes.io~1safe-to-evict-local-volumes",
			},
		},
		{
			Group: "apps",
			Kind:  "StatefulSet",
			JSONPointers: []string{
				"/metadata/annotations/cluster-autoscaler.kubernetes.io~1safe-to-evict",
			},
		},

		// KEDA annotations added when ScaledObject is created
		{
			Group: "apps",
			Kind:  "Deployment",
			JSONPointers: []string{
				"/metadata/annotations/autoscaling.keda.sh~1paused-replicas",
			},
		},
		{
			Group: "apps",
			Kind:  "StatefulSet",
			JSONPointers: []string{
				"/metadata/annotations/autoscaling.keda.sh~1paused-replicas",
			},
		},
	}
}
