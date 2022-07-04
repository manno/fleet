// extracted from argoproj/argo-cd/util/argo/diff/normalize.go
package diff

import (
	"github.com/rancher/fleet/modules/agent/pkg/deployer/diff/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
)

// NewDiffNormalizer creates normalizer that uses Argo CD and application settings to normalize the resource prior to diffing.
func NewDiffNormalizer(ignore []resource.ResourceIgnoreDifferences, overrides map[string]resource.ResourceOverride) (Normalizer, error) {
	ignoreNormalizer, err := NewIgnoreNormalizer(ignore, overrides)
	if err != nil {
		return nil, err
	}
	knownTypesNorm, err := newKnownTypesNormalizer(overrides)
	if err != nil {
		return nil, err
	}

	return &composableNormalizer{normalizers: []Normalizer{ignoreNormalizer, knownTypesNorm}}, nil
}

type composableNormalizer struct {
	normalizers []Normalizer
}

// Normalize performs resource normalization.
func (n *composableNormalizer) Normalize(un *unstructured.Unstructured) error {
	for i := range n.normalizers {
		if err := n.normalizers[i].Normalize(un); err != nil {
			return err
		}
	}
	return nil
}
