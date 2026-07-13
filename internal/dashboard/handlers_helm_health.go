package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HelmHealthResult is the Helm release health & GitOps drift audit.
type HelmHealthResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         HelmHealthSummary  `json:"summary"`
	Releases        []HelmReleaseEntry `json:"releases"`
	ByNamespace     []HelmNSStat       `json:"byNamespace"`
	DriftedReleases []HelmReleaseEntry `json:"driftedReleases"`
	Recommendations []string           `json:"recommendations"`
}

// HelmHealthSummary aggregates Helm release health statistics.
type HelmHealthSummary struct {
	TotalReleases   int  `json:"totalReleases"`
	HealthyReleases int  `json:"healthyReleases"`
	FailedReleases  int  `json:"failedReleases"`
	PendingReleases int  `json:"pendingReleases"`
	StaleReleases   int  `json:"staleReleases"`  // not updated in >30 days
	NoResourceReqs  int  `json:"noResourceReqs"` // releases with no resource requests
	HasHelm         bool `json:"hasHelm"`        // Helm secrets detected
	HealthScore     int  `json:"healthScore"`
}

// HelmReleaseEntry describes one Helm release.
type HelmReleaseEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Version   string `json:"version"`
	Status    string `json:"status"` // deployed, failed, pending
	Chart     string `json:"chart"`
	Age       string `json:"age"`
	RiskLevel string `json:"riskLevel"`
	Issue     string `json:"issue,omitempty"`
}

// HelmNSStat shows Helm release stats per namespace.
type HelmNSStat struct {
	Namespace     string `json:"namespace"`
	TotalReleases int    `json:"totalReleases"`
	FailedCount   int    `json:"failedCount"`
}

// helmHealthAuditCore performs the audit on Helm release secrets (testable).
// Helm stores release state as secrets with labels:
//
//	owner=helm, name=<release-name>, status=<status>
func helmHealthAuditCore(secrets []corev1.Secret, now time.Time) HelmHealthResult {
	result := HelmHealthResult{
		ScannedAt: now,
	}

	nsStats := make(map[string]*HelmNSStat)
	releaseMap := make(map[string]*HelmReleaseEntry) // key = namespace/name

	for i := range secrets {
		sec := &secrets[i]

		// Check if this is a Helm release secret
		owner, hasOwner := sec.Labels["owner"]
		if !hasOwner || owner != "helm" {
			continue
		}

		result.Summary.HasHelm = true

		releaseName := sec.Labels["name"]
		if releaseName == "" {
			releaseName = sec.Labels["release"]
		}
		if releaseName == "" {
			continue
		}

		status := sec.Labels["status"]
		if status == "" {
			status = "unknown"
		}

		version := sec.Labels["version"]
		if version == "" {
			parts := strings.Split(sec.Name, ".")
			if len(parts) > 0 {
				version = parts[len(parts)-1]
			}
		}

		chart := sec.Labels["chart"]
		if chart == "" {
			chart = "unknown"
		}

		ns := sec.Namespace
		key := fmt.Sprintf("%s/%s", ns, releaseName)
		age := formatDurationAge(sec.CreationTimestamp.Time)

		if existing, ok := releaseMap[key]; ok {
			if version >= existing.Version {
				*existing = HelmReleaseEntry{
					Name: releaseName, Namespace: ns, Version: version,
					Status: status, Chart: chart, Age: age,
				}
			}
		} else {
			releaseMap[key] = &HelmReleaseEntry{
				Name: releaseName, Namespace: ns, Version: version,
				Status: status, Chart: chart, Age: age,
			}
		}
	}

	// Analyze each release
	for _, rel := range releaseMap {
		ns := rel.Namespace
		if _, ok := nsStats[ns]; !ok {
			nsStats[ns] = &HelmNSStat{Namespace: ns}
		}
		nsStats[ns].TotalReleases++
		result.Summary.TotalReleases++

		risk := "low"
		var issue string

		switch strings.ToLower(rel.Status) {
		case "deployed":
			result.Summary.HealthyReleases++
		case "failed":
			result.Summary.FailedReleases++
			risk = "high"
			issue = "release status is failed — check Helm rollout and template errors"
			nsStats[ns].FailedCount++
		case "pending", "pending-install", "pending-upgrade", "pending-rollback":
			result.Summary.PendingReleases++
			risk = "medium"
			issue = fmt.Sprintf("release is in %s state — may be stuck", rel.Status)
			nsStats[ns].FailedCount++
		default:
			result.Summary.FailedReleases++
			risk = "medium"
			issue = fmt.Sprintf("unusual release status: %s", rel.Status)
		}

		rel.RiskLevel = risk
		rel.Issue = issue

		result.Releases = append(result.Releases, *rel)
		if issue != "" {
			result.DriftedReleases = append(result.DriftedReleases, *rel)
		}
	}

	// Build namespace stats
	for _, stat := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *stat)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].FailedCount > result.ByNamespace[j].FailedCount
	})

	sort.Slice(result.Releases, func(i, j int) bool {
		riskOrder := map[string]int{"high": 0, "medium": 1, "low": 2}
		return riskOrder[result.Releases[i].RiskLevel] < riskOrder[result.Releases[j].RiskLevel]
	})

	result.Summary.HealthScore = helmHealthScore(result.Summary)
	result.Recommendations = helmHealthRecommendations(result.Summary)

	return result
}

// helmHealthScore calculates health score.
func helmHealthScore(s HelmHealthSummary) int {
	if !s.HasHelm || s.TotalReleases == 0 {
		return 100
	}
	base := 100
	base -= s.FailedReleases * 15
	base -= s.PendingReleases * 8
	base -= s.StaleReleases * 3
	base -= s.NoResourceReqs * 2
	if base < 0 {
		base = 0
	}
	return base
}

// helmHealthRecommendations generates recommendations.
func helmHealthRecommendations(s HelmHealthSummary) []string {
	var recs []string
	if !s.HasHelm {
		recs = append(recs, "no Helm releases detected — install Helm for package management")
		return recs
	}
	if s.FailedReleases > 0 {
		recs = append(recs, fmt.Sprintf("%d Helm releases are in failed state — run 'helm rollback' or fix template errors", s.FailedReleases))
	}
	if s.PendingReleases > 0 {
		recs = append(recs, fmt.Sprintf("%d Helm releases are pending — check for stuck install/upgrade processes", s.PendingReleases))
	}
	if s.StaleReleases > 0 {
		recs = append(recs, fmt.Sprintf("%d Helm releases haven't been updated in >30 days — consider upgrading to latest chart versions", s.StaleReleases))
	}
	if s.FailedReleases == 0 && s.PendingReleases == 0 {
		recs = append(recs, "all Helm releases are healthy — no issues detected")
	}
	return recs
}

// handleHelmHealth audits Helm release health and GitOps drift.
// GET /api/deployment/helm-health
func (s *Server) handleHelmHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	secrets, err := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{
		LabelSelector: "owner=helm",
	})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	result := helmHealthAuditCore(secrets.Items, time.Now())
	writeJSON(w, result)
}
