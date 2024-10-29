package reconciler

import (
	"encoding/json"
	"sort"
	"strings"

	fleet "github.com/rancher/fleet/pkg/apis/fleet.cattle.io/v1alpha1"
)

func setResources(list *fleet.BundleDeploymentList, gitrepo *fleet.GitRepo) {
	s := summaryState(gitrepo.Status.Summary)
	r, counts, errors := fromResources(list, s)
	gitrepo.Status.ResourceErrors = errors
	gitrepo.Status.ResourceCounts = counts
	gitrepo.Status.Resources = r
}

func summaryState(summary fleet.BundleSummary) string {
	if summary.WaitApplied > 0 {
		return "WaitApplied"
	}
	if summary.ErrApplied > 0 {
		return "ErrApplied"
	}
	return ""
}

// fromResources inspects all bundledeployments for this GitRepo and returns a list of
// GitRepoResources and error messages.
//
// It populates gitrepo status resources from bundleDeployments. BundleDeployment.Status.Resources is the list of deployed resources.
func fromResources(list *fleet.BundleDeploymentList, summaryState string) ([]fleet.GitRepoResource, fleet.GitRepoResourceCounts, []string) {
	var errors []string

	counts := fleet.GitRepoResourceCounts{}
	allResources := map[fleet.ResourceKey]fleet.GitRepoResource{}
	for _, bd := range list.Items {
		bdResources := []fleet.GitRepoResource{}
		for _, resource := range bd.Status.Resources {
			key := fleet.ResourceKey{
				Kind:       resource.Kind,
				APIVersion: resource.APIVersion,
				Name:       resource.Name,
				Namespace:  resource.Namespace,
			}
			if _, ok := allResources[key]; !ok {
				resource := fleet.GitRepoResource{
					APIVersion:      key.APIVersion,
					Kind:            key.Kind,
					Namespace:       key.Namespace,
					Name:            key.Name,
					PerClusterState: []fleet.ResourcePerClusterState{},
				}
				resource.Type, resource.ID = toType(resource)
				allResources[key] = resource
				bdResources = append(bdResources, resource)
			}
		}

		incomplete, err := addState(bd, allResources)
		if len(err) > 0 {
			incomplete = true
			for _, err := range err {
				errors = append(errors, err.Error())
			}
		}
		if incomplete {
			for i := range bdResources {
				bdResources[i].IncompleteState = true
			}
		}
	}

	var resources []fleet.GitRepoResource
	for _, resource := range allResources {
		// fallback to state from gitrepo summary
		if resource.State == "" {
			if resource.IncompleteState {
				if summaryState != "" {
					resource.State = summaryState
				} else {
					resource.State = "Unknown"
				}
			} else if summaryState != "" {
				resource.State = summaryState
			} else {
				resource.State = "Ready"
			}
		}
		countResources(&counts, resource)

		sort.Slice(resource.PerClusterState, func(i, j int) bool {
			return resource.PerClusterState[i].ClusterID < resource.PerClusterState[j].ClusterID
		})
		resources = append(resources, resource)
	}

	sort.Strings(errors)
	sort.Slice(resources, func(i, j int) bool {
		return resources[i].Type+"/"+resources[i].ID < resources[j].Type+"/"+resources[j].ID
	})

	return resources, counts, errors
}

func toType(resource fleet.GitRepoResource) (string, string) {
	group := strings.Split(resource.APIVersion, "/")[0]
	if group == "v1" {
		group = ""
	} else if len(group) > 0 {
		group += "."
	}
	t := group + strings.ToLower(resource.Kind)
	if resource.Namespace == "" {
		return t, resource.Name
	}
	return t, resource.Namespace + "/" + resource.Name
}

// addState adds per-cluster state information for nonReady and modified resources in a bundleDeployment.
// It only adds up to 10 entries to not overwhelm the status.
// It mutates resources and returns whether the reported state is incomplete and any errors encountered.
func addState(bd fleet.BundleDeployment, allResources map[fleet.ResourceKey]fleet.GitRepoResource) (bool, []error) {
	var (
		incomplete bool
		errors     []error
	)

	if len(bd.Status.NonReadyStatus) >= 10 || len(bd.Status.ModifiedStatus) >= 10 {
		incomplete = true
	}

	cluster := bd.Labels[fleet.ClusterNamespaceLabel] + "/" + bd.Labels[fleet.ClusterLabel]
	for _, nonReady := range bd.Status.NonReadyStatus {
		key := fleet.ResourceKey{
			Kind:       nonReady.Kind,
			APIVersion: nonReady.APIVersion,
			Namespace:  nonReady.Namespace,
			Name:       nonReady.Name,
		}
		state := fleet.ResourcePerClusterState{
			State:         nonReady.Summary.State,
			Error:         nonReady.Summary.Error,
			Transitioning: nonReady.Summary.Transitioning,
			Message:       strings.Join(nonReady.Summary.Message, "; "),
			ClusterID:     cluster,
		}
		appendState(allResources, key, state)
	}

	for _, modified := range bd.Status.ModifiedStatus {
		key := fleet.ResourceKey{
			Kind:       modified.Kind,
			APIVersion: modified.APIVersion,
			Namespace:  modified.Namespace,
			Name:       modified.Name,
		}
		state := fleet.ResourcePerClusterState{
			State:     "Modified",
			ClusterID: cluster,
		}
		if modified.Delete {
			state.State = "Orphaned"
		} else if modified.Create {
			state.State = "Missing"
		} else if len(modified.Patch) > 0 {
			state.Patch = &fleet.GenericMap{}
			err := json.Unmarshal([]byte(modified.Patch), state.Patch)
			if err != nil {
				errors = append(errors, err)
			}
		}
		appendState(allResources, key, state)
	}
	return incomplete, errors
}

func appendState(allResources map[fleet.ResourceKey]fleet.GitRepoResource, key fleet.ResourceKey, state fleet.ResourcePerClusterState) {
	r := allResources[key]
	if r.PerClusterState != nil {
		r.PerClusterState = append(r.PerClusterState, state)
	} else {
		// this is strange
		r.PerClusterState = []fleet.ResourcePerClusterState{state}
	}

	// what was the namespace fix?
	allResources[key] = r
}

func bundleDeploymentResources(bd fleet.BundleDeployment) map[fleet.ResourceKey][]fleet.ResourcePerClusterState {
	allResources := map[fleet.ResourceKey][]fleet.ResourcePerClusterState{}
	for _, resource := range bd.Status.Resources {
		key := fleet.ResourceKey{
			Kind:       resource.Kind,
			APIVersion: resource.APIVersion,
			Name:       resource.Name,
			Namespace:  resource.Namespace,
		}
		allResources[key] = []fleet.ResourcePerClusterState{}
	}
	return allResources
}

func countResources(counts *fleet.GitRepoResourceCounts, resource fleet.GitRepoResource) {
	counts.DesiredReady++
	switch resource.State {
	case "Ready":
		counts.Ready++
	case "WaitApplied":
		counts.WaitApplied++
	case "Modified":
		counts.Modified++
	case "Orphan":
		counts.Orphaned++
	case "Missing":
		counts.Missing++
	case "Unknown":
		counts.Unknown++
	default:
		counts.NotReady++
	}
}
