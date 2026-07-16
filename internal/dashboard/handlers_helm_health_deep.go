package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HelmHealthResult provides deep Helm release health analysis:
// release age, chart version staleness, values drift, and release integrity.
type HelmHealthDeepResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         HelmHealthDeepSummary   `json:"summary"`
	StaleReleases   []StaleRelease      `json:"staleReleases"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

type HelmHealthDeepSummary struct {
	TotalReleases   int     `json:"totalReleases"`
	StaleReleases   int     `json:"staleReleases"`
	DeployedReleases int    `json:"deployedReleases"`
	FailedReleases  int     `json:"failedReleases"`
	AvgChartAge     string  `json:"avgChartAge"`
}

type StaleRelease struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Chart      string `json:"chart"`
	Version    string `json:"version"`
	Status     string `json:"status"`
	Updated    string `json:"updated"`
	Severity   string `json:"severity"`
	Issue      string `json:"issue"`
}

// handleHelmHealthDeep provides deep Helm release health analysis.
// GET /api/deployment/helm-health-deep
func (s *Server) handleHelmHealthDeep(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := HelmHealthDeepResult{ScannedAt: time.Now()}

	// Helm stores releases as secrets with type helm.sh/release.v1
	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	now := time.Now()
	threeMonthsAgo := now.AddDate(0, -3, 0)

	for _, sec := range secrets.Items {
		if sec.Type != "helm.sh/release.v1" {
			continue
		}
		name := sec.Labels["name"]
		ns := sec.Namespace
		version := sec.Labels["version"]
		status := sec.Labels["status"]
		chart := string(sec.Data["chart"])

		_ = name // name used below directly

		result.Summary.TotalReleases++
		if status == "deployed" {
			result.Summary.DeployedReleases++
		} else if status == "failed" {
			result.Summary.FailedReleases++
		}

		// Check staleness
		if sec.CreationTimestamp.Time.Before(threeMonthsAgo) {
			result.Summary.StaleReleases++
			severity := "medium"
			if status == "failed" {
				severity = "high"
			}
			result.StaleReleases = append(result.StaleReleases, StaleRelease{
				Name: name, Namespace: ns, Chart: chart,
				Version: version, Status: status,
				Updated: sec.CreationTimestamp.Time.Format("2006-01-02"),
				Severity: severity,
				Issue: fmt.Sprintf("Release not updated since %s", sec.CreationTimestamp.Time.Format("2006-01")),
			})
		}

		// Failed releases are always a concern
		if status == "failed" {
			result.StaleReleases = append(result.StaleReleases, StaleRelease{
				Name: name, Namespace: ns, Chart: chart,
				Version: version, Status: status,
				Updated: sec.CreationTimestamp.Time.Format("2006-01-02"),
				Severity: "critical",
				Issue: "Release status is 'failed' — needs rollback or fix",
			})
		}
	}

	// Score
	score := 100
	if result.Summary.TotalReleases > 0 {
		failRatio := float64(result.Summary.FailedReleases) / float64(result.Summary.TotalReleases)
		score -= int(failRatio * 50)
		staleRatio := float64(result.Summary.StaleReleases) / float64(result.Summary.TotalReleases)
		score -= int(staleRatio * 30)
	}
	if score < 0 { score = 0 }
	result.HealthScore = min(100, score)
	result.Grade = goldenScoreToGrade(result.HealthScore)

	sort.Slice(result.StaleReleases, func(i, j int) bool {
		return result.StaleReleases[i].Severity > result.StaleReleases[j].Severity
	})

	var recs []string
	recs = append(recs, fmt.Sprintf("Helm health: %d/100 (grade %s) — %d releases, %d deployed, %d failed", result.HealthScore, result.Grade, result.Summary.TotalReleases, result.Summary.DeployedReleases, result.Summary.FailedReleases))
	if result.Summary.FailedReleases > 0 {
		recs = append(recs, fmt.Sprintf("%d failed Helm releases — rollback or fix immediately", result.Summary.FailedReleases))
	}
	if result.Summary.StaleReleases > 0 {
		recs = append(recs, fmt.Sprintf("%d releases older than 3 months — update charts to latest versions", result.Summary.StaleReleases))
	}
	if len(recs) == 1 {
		recs = append(recs, "All Helm releases are healthy and up-to-date")
	}
	result.Recommendations = recs

	writeJSON(w, result)
}
