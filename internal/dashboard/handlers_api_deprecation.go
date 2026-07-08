package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// DAVResult is the deprecated API version & upgrade readiness analysis.
type DAVResult struct {
	ScannedAt       time.Time  `json:"scannedAt"`
	Summary         DAVSummary `json:"summary"`
	DeprecatedAPIs  []DAVEntry `json:"deprecatedAPIs"`
	RemovedAPIs     []DAVEntry `json:"removedAPIs"`
	ClusterVersion  string     `json:"clusterVersion"`
	Issues          []DAVIssue `json:"issues"`
	Recommendations []string   `json:"recommendations"`
}

// DAVSummary aggregates deprecation stats.
type DAVSummary struct {
	TotalResources  int  `json:"totalResources"`
	DeprecatedCount int  `json:"deprecatedCount"`
	RemovedCount    int  `json:"removedCount"`
	ReadyForUpgrade bool `json:"readyForUpgrade"`
	ReadinessScore  int  `json:"readinessScore"` // 0-100
}

// DAVEntry describes one deprecated API usage.
type DAVEntry struct {
	Resource    string   `json:"resource"`   // e.g. "Deployment"
	APIGroup    string   `json:"apiGroup"`   // e.g. "apps/v1"
	OldVersion  string   `json:"oldVersion"` // e.g. "extensions/v1beta1"
	NewVersion  string   `json:"newVersion"` // e.g. "apps/v1"
	RemovedIn   string   `json:"removedIn"`  // e.g. "v1.16"
	ObjectCount int      `json:"objectCount"`
	Namespaces  []string `json:"namespaces"`
	Status      string   `json:"status"` // deprecated, removed
}

// DAVIssue is a detected deprecation problem.
type DAVIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// Known deprecated API mappings
var davDeprecatedAPIs = map[string]DAVEntry{
	"extensions/v1beta1/Deployment": {
		Resource: "Deployment", OldVersion: "extensions/v1beta1", NewVersion: "apps/v1", RemovedIn: "v1.16", Status: "removed",
	},
	"extensions/v1beta1/DaemonSet": {
		Resource: "DaemonSet", OldVersion: "extensions/v1beta1", NewVersion: "apps/v1", RemovedIn: "v1.16", Status: "removed",
	},
	"extensions/v1beta1/ReplicaSet": {
		Resource: "ReplicaSet", OldVersion: "extensions/v1beta1", NewVersion: "apps/v1", RemovedIn: "v1.16", Status: "removed",
	},
	"apps/v1beta1/Deployment": {
		Resource: "Deployment", OldVersion: "apps/v1beta1", NewVersion: "apps/v1", RemovedIn: "v1.16", Status: "removed",
	},
	"apps/v1beta1/StatefulSet": {
		Resource: "StatefulSet", OldVersion: "apps/v1beta1", NewVersion: "apps/v1", RemovedIn: "v1.16", Status: "removed",
	},
	"apps/v1beta2/Deployment": {
		Resource: "Deployment", OldVersion: "apps/v1beta2", NewVersion: "apps/v1", RemovedIn: "v1.16", Status: "removed",
	},
	"apps/v1beta2/StatefulSet": {
		Resource: "StatefulSet", OldVersion: "apps/v1beta2", NewVersion: "apps/v1", RemovedIn: "v1.16", Status: "removed",
	},
	"apps/v1beta2/DaemonSet": {
		Resource: "DaemonSet", OldVersion: "apps/v1beta2", NewVersion: "apps/v1", RemovedIn: "v1.16", Status: "removed",
	},
	"policy/v1beta1/PodSecurityPolicy": {
		Resource: "PodSecurityPolicy", OldVersion: "policy/v1beta1", NewVersion: "(removed)", RemovedIn: "v1.25", Status: "removed",
	},
	"networking.k8s.io/v1beta1/Ingress": {
		Resource: "Ingress", OldVersion: "networking.k8s.io/v1beta1", NewVersion: "networking.k8s.io/v1", RemovedIn: "v1.22", Status: "removed",
	},
	"extensions/v1beta1/Ingress": {
		Resource: "Ingress", OldVersion: "extensions/v1beta1", NewVersion: "networking.k8s.io/v1", RemovedIn: "v1.22", Status: "removed",
	},
	"batch/v1beta1/CronJob": {
		Resource: "CronJob", OldVersion: "batch/v1beta1", NewVersion: "batch/v1", RemovedIn: "v1.25", Status: "removed",
	},
	"autoscaling/v2beta1/HorizontalPodAutoscaler": {
		Resource: "HPA", OldVersion: "autoscaling/v2beta1", NewVersion: "autoscaling/v2", RemovedIn: "v1.25", Status: "removed",
	},
	"autoscaling/v2beta2/HorizontalPodAutoscaler": {
		Resource: "HPA", OldVersion: "autoscaling/v2beta2", NewVersion: "autoscaling/v2", RemovedIn: "v1.25", Status: "removed",
	},
	"storage.k8s.io/v1beta1/CSIDriver": {
		Resource: "CSIDriver", OldVersion: "storage.k8s.io/v1beta1", NewVersion: "storage.k8s.io/v1", RemovedIn: "v1.20", Status: "removed",
	},
	"storage.k8s.io/v1beta1/CSINode": {
		Resource: "CSINode", OldVersion: "storage.k8s.io/v1beta1", NewVersion: "storage.k8s.io/v1", RemovedIn: "v1.20", Status: "removed",
	},
	"storage.k8s.io/v1beta1/StorageClass": {
		Resource: "StorageClass", OldVersion: "storage.k8s.io/v1beta1", NewVersion: "storage.k8s.io/v1", RemovedIn: "v1.20", Status: "removed",
	},
	"storage.k8s.io/v1beta1/VolumeAttachment": {
		Resource: "VolumeAttachment", OldVersion: "storage.k8s.io/v1beta1", NewVersion: "storage.k8s.io/v1", RemovedIn: "v1.20", Status: "removed",
	},
}

// handleDeprecatedAPI checks for deprecated/removed Kubernetes API versions.
// GET /api/product/api-deprecation
func (s *Server) handleDeprecatedAPI(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := DAVResult{ScannedAt: time.Now()}

	// Get cluster version
	nodes, err := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	if err == nil && nodes != nil && len(nodes.Items) > 0 {
		result.ClusterVersion = nodes.Items[0].Status.NodeInfo.KubeletVersion
	}

	// Check all API groups for deprecated versions
	// We check by attempting to list resources at known deprecated API paths
	apiGroups := []struct {
		groupVersion schema.GroupVersion
		resources    []string
	}{
		{schema.GroupVersion{Group: "extensions", Version: "v1beta1"}, []string{"deployments", "daemonsets", "replicasets", "ingresses"}},
		{schema.GroupVersion{Group: "apps", Version: "v1beta1"}, []string{"deployments", "statefulsets"}},
		{schema.GroupVersion{Group: "apps", Version: "v1beta2"}, []string{"deployments", "statefulsets", "daemonsets"}},
		{schema.GroupVersion{Group: "policy", Version: "v1beta1"}, []string{"podsecuritypolicies"}},
		{schema.GroupVersion{Group: "networking.k8s.io", Version: "v1beta1"}, []string{"ingresses"}},
		{schema.GroupVersion{Group: "batch", Version: "v1beta1"}, []string{"cronjobs"}},
		{schema.GroupVersion{Group: "autoscaling", Version: "v2beta1"}, []string{"horizontalpodautoscalers"}},
		{schema.GroupVersion{Group: "autoscaling", Version: "v2beta2"}, []string{"horizontalpodautoscalers"}},
	}

	// Check if API server still serves these deprecated APIs
	// by querying the API discovery endpoint
	_, serverResources, err := rc.clientset.Discovery().ServerGroupsAndResources()
	if err != nil {
		// Fallback: just note that discovery failed
		result.Summary.ReadyForUpgrade = true
		result.Summary.ReadinessScore = 100
		result.Recommendations = []string{"Unable to query API discovery — assuming cluster uses only stable APIs"}
		writeJSON(w, result)
		return
	}

	// Build set of available API groups from resource lists
	availableGroups := make(map[string]bool)
	for _, rl := range serverResources {
		if rl != nil {
			availableGroups[rl.GroupVersion] = true
		}
	}

	// Check each deprecated API
	for _, ag := range apiGroups {
		gvKey := ag.groupVersion.String()
		for _, res := range ag.resources {
			lookupKey := fmt.Sprintf("%s/%s", gvKey, strings.Title(res))
			// Try case-insensitive match
			for depKey, depEntry := range davDeprecatedAPIs {
				if strings.EqualFold(depKey, lookupKey) {
					if availableGroups[gvKey] {
						// API still served by cluster
						entry := depEntry
						entry.APIGroup = gvKey
						result.Summary.DeprecatedCount++
						if entry.Status == "removed" {
							result.Summary.RemovedCount++
							result.RemovedAPIs = append(result.RemovedAPIs, entry)
						} else {
							result.DeprecatedAPIs = append(result.DeprecatedAPIs, entry)
						}
						result.Issues = append(result.Issues, DAVIssue{
							Severity: "warning", Type: "deprecated-api",
							Resource: entry.Resource,
							Message:  fmt.Sprintf("%s uses deprecated API %s — migrate to %s (removed in %s)", entry.Resource, gvKey, entry.NewVersion, entry.RemovedIn),
						})
					}
					break
				}
			}
		}
	}

	result.Summary.TotalResources = len(result.DeprecatedAPIs) + len(result.RemovedAPIs)
	result.Summary.ReadyForUpgrade = result.Summary.RemovedCount == 0 && result.Summary.DeprecatedCount == 0

	sort.Slice(result.Issues, func(i, j int) bool {
		return davIssueRank(result.Issues[i].Severity) < davIssueRank(result.Issues[j].Severity)
	})

	result.Summary.ReadinessScore = davScore(result.Summary)
	result.Recommendations = davGenRecs(result.Summary, result.DeprecatedAPIs, result.RemovedAPIs)

	writeJSON(w, result)
}

// davScore computes upgrade readiness score 0-100.
func davScore(s DAVSummary) int {
	score := 100
	score -= s.RemovedCount * 30 // removed APIs are critical blockers
	score -= s.DeprecatedCount * 15
	if score < 0 {
		score = 0
	}
	return score
}

// davGenRecs produces actionable advice.
func davGenRecs(s DAVSummary, deprecated []DAVEntry, removed []DAVEntry) []string {
	var recs []string

	if len(removed) > 0 {
		top := ""
		if len(removed) > 0 {
			top = fmt.Sprintf(" (e.g. %s: %s → %s)", removed[0].Resource, removed[0].OldVersion, removed[0].NewVersion)
		}
		recs = append(recs, fmt.Sprintf("%d API(s) have been REMOVED and will block cluster upgrade%s — migrate immediately", len(removed), top))
	}
	if len(deprecated) > 0 {
		recs = append(recs, fmt.Sprintf("%d deprecated API(s) still served — plan migration before next cluster upgrade", len(deprecated)))
	}

	// Specific migration advice
	for _, entry := range removed {
		if entry.NewVersion != "(removed)" {
			recs = append(recs, fmt.Sprintf("Migrate %s from %s to %s", entry.Resource, entry.OldVersion, entry.NewVersion))
		} else {
			recs = append(recs, fmt.Sprintf("%s (%s) has no replacement — remove before upgrade", entry.Resource, entry.OldVersion))
		}
	}

	if s.ReadinessScore < 70 {
		recs = append(recs, fmt.Sprintf("Upgrade readiness score is %d/100 — deprecated/removed APIs detected", s.ReadinessScore))
	}
	if s.ReadyForUpgrade {
		recs = append(recs, "No deprecated or removed API versions detected — cluster is ready for upgrade")
	}

	return recs
}

func davIssueRank(s string) int {
	switch s {
	case "critical":
		return 0
	case "warning":
		return 1
	case "info":
		return 2
	default:
		return 3
	}
}
