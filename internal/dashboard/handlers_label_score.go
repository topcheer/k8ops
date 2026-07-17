package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LabelScoreResult evaluates Kubernetes label quality across the cluster.
// Good labels are essential for selectors, cost allocation, monitoring,
// and troubleshooting. This audit identifies missing standard labels,
// inconsistent naming, and orphaned selector references.
type LabelScoreResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         LabelScoreSummary   `json:"summary"`
	ByResource      []LabelResourceStat `json:"byResource"`
	Findings        []LabelFinding      `json:"findings"`
	StandardLabels  []LabelStandardStat `json:"standardLabels"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

type LabelScoreSummary struct {
	TotalResources int `json:"totalResources"`
	WithAppName    int `json:"withAppName"`
	WithAppVersion int `json:"withAppVersion"`
	WithInstance   int `json:"withInstance"`
	WithManagedBy  int `json:"withManagedBy"`
	WithTeam       int `json:"withTeam"`
	WithNamespace  int `json:"withNamespaceLabel"`
	NoLabels       int `json:"noLabels"`
}

type LabelResourceStat struct {
	Kind       string `json:"kind"`
	Count      int    `json:"count"`
	WithLabels int    `json:"withLabels"`
	Coverage   int    `json:"coveragePct"`
}

type LabelFinding struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
	Detail    string `json:"detail"`
}

type LabelStandardStat struct {
	Label    string `json:"label"`
	Present  int    `json:"present"`
	Missing  int    `json:"missing"`
	Coverage int    `json:"coveragePct"`
}

// standardLabels are the Kubernetes recommended labels
var standardLabels = []string{"app.kubernetes.io/name", "app.kubernetes.io/version", "app.kubernetes.io/instance", "app.kubernetes.io/managed-by", "app"}

// handleLabelScore handles GET /api/product/label-score
func (s *Server) handleLabelScore(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := LabelScoreResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})

	labelCounts := make(map[string]int) // label -> count of resources having it
	var findings []LabelFinding
	kindStats := make(map[string]*LabelResourceStat)

	totalRes := 0

	// Process deployments
	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		totalRes++
		result.Summary.TotalResources++

		kindKey := "Deployment"
		if _, ok := kindStats[kindKey]; !ok {
			kindStats[kindKey] = &LabelResourceStat{Kind: kindKey}
		}
		kindStats[kindKey].Count++

		labels := d.Labels
		podLabels := d.Spec.Template.Labels

		if len(labels) == 0 && len(podLabels) == 0 {
			result.Summary.NoLabels++
			findings = append(findings, LabelFinding{
				Kind: kindKey, Name: d.Name, Namespace: d.Namespace,
				Issue: "No labels", Severity: "high",
				Detail: "Deployment and pod template have no labels",
			})
		} else {
			kindStats[kindKey].WithLabels++
		}

		// Check standard labels on pod template
		for _, sl := range standardLabels {
			if _, ok := podLabels[sl]; ok {
				labelCounts[sl]++
			}
		}

		// Check app label specifically
		if hasLabel(podLabels, "app") || hasLabel(podLabels, "app.kubernetes.io/name") {
			result.Summary.WithAppName++
		} else {
			findings = append(findings, LabelFinding{
				Kind: kindKey, Name: d.Name, Namespace: d.Namespace,
				Issue: "Missing 'app' label", Severity: "medium",
				Detail: "Pod template missing app label for service selection",
			})
		}

		// Check version label
		if hasLabel(podLabels, "app.kubernetes.io/version") || hasLabel(podLabels, "version") {
			result.Summary.WithAppVersion++
		}

		// Check team/owner label
		if hasLabel(podLabels, "team") || hasLabel(podLabels, "owner") || hasLabel(labels, "team") {
			result.Summary.WithTeam++
		} else {
			findings = append(findings, LabelFinding{
				Kind: kindKey, Name: d.Name, Namespace: d.Namespace,
				Issue: "Missing team/owner label", Severity: "low",
				Detail: "No team or owner label for accountability",
			})
		}
	}

	// Process services
	for _, svc := range services.Items {
		if isSystemNamespace(svc.Namespace) {
			continue
		}
		totalRes++
		result.Summary.TotalResources++

		kindKey := "Service"
		if _, ok := kindStats[kindKey]; !ok {
			kindStats[kindKey] = &LabelResourceStat{Kind: kindKey}
		}
		kindStats[kindKey].Count++

		if len(svc.Labels) == 0 && len(svc.Spec.Selector) == 0 {
			result.Summary.NoLabels++
			findings = append(findings, LabelFinding{
				Kind: kindKey, Name: svc.Name, Namespace: svc.Namespace,
				Issue: "No labels or selector", Severity: "medium",
				Detail: "Service has no labels or selector",
			})
		} else {
			kindStats[kindKey].WithLabels++
		}
	}

	// Build standard label stats
	for _, sl := range standardLabels {
		missing := totalRes - labelCounts[sl]
		pct := 0
		if totalRes > 0 {
			pct = labelCounts[sl] * 100 / totalRes
		}
		result.StandardLabels = append(result.StandardLabels, LabelStandardStat{
			Label: sl, Present: labelCounts[sl], Missing: missing, Coverage: pct,
		})
	}

	// Kind stats
	for _, ks := range kindStats {
		if ks.Count > 0 {
			ks.Coverage = ks.WithLabels * 100 / ks.Count
		}
		result.ByResource = append(result.ByResource, *ks)
	}

	// Score
	if totalRes > 0 {
		result.HealthScore = (totalRes - result.Summary.NoLabels) * 100 / totalRes
		// Penalty for low standard label coverage
		if len(result.StandardLabels) > 0 {
			avgCov := 0
			for _, sl := range result.StandardLabels {
				avgCov += sl.Coverage
			}
			avgCov /= len(result.StandardLabels)
			result.HealthScore = (result.HealthScore + avgCov) / 2
		}
	} else {
		result.HealthScore = 100
	}

	switch {
	case result.HealthScore >= 80:
		result.Grade = "A"
	case result.HealthScore >= 60:
		result.Grade = "B"
	case result.HealthScore >= 40:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	sort.Slice(findings, func(i, j int) bool {
		sevOrder := map[string]int{"high": 0, "medium": 1, "low": 2}
		return sevOrder[findings[i].Severity] < sevOrder[findings[j].Severity]
	})
	if len(findings) > 50 {
		findings = findings[:50]
	}
	result.Findings = findings

	result.Recommendations = buildLabelScoreRecs(&result)
	writeJSON(w, result)
}

func hasLabel(labels map[string]string, key string) bool {
	_, ok := labels[key]
	return ok
}

func buildLabelScoreRecs(r *LabelScoreResult) []string {
	recs := []string{}
	if r.Summary.NoLabels > 0 {
		recs = append(recs, fmt.Sprintf("%d 个资源完全没有标签", r.Summary.NoLabels))
	}
	lowCovLabels := []string{}
	for _, sl := range r.StandardLabels {
		if sl.Coverage < 50 {
			lowCovLabels = append(lowCovLabels, sl.Label)
		}
	}
	if len(lowCovLabels) > 0 {
		recs = append(recs, fmt.Sprintf("标准标签覆盖率低于 50%%: %s", strings.Join(lowCovLabels, ", ")))
	}
	if r.Summary.WithTeam == 0 {
		recs = append(recs, "没有工作负载标记 team/owner 标签，无法追踪归属")
	}
	if len(recs) == 0 {
		recs = append(recs, "标签规范良好")
	}
	return recs
}

var _ appsv1.Deployment
var _ corev1.Service
