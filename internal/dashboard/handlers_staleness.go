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

// StalenessResult is the full workload staleness analysis.
type StalenessResult struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	Summary         StalenessSummary `json:"summary"`
	Workloads       []StaleWorkload  `json:"workloads"`
	ByNamespace     []StaleNsStat    `json:"byNamespace"`
	ImageAgeBuckets []ImageAgeBucket `json:"imageAgeBuckets"`
	Recommendations []string         `json:"recommendations"`
}

// StalenessSummary aggregates cluster-wide staleness metrics.
type StalenessSummary struct {
	TotalWorkloads  int `json:"totalWorkloads"`
	FreshWorkloads  int `json:"freshWorkloads"`     // updated < 7 days
	StaleWorkloads  int `json:"staleWorkloads"`     // updated > 30 days
	VeryStaleWL     int `json:"veryStaleWorkloads"` // updated > 90 days
	AncientWL       int `json:"ancientWorkloads"`   // updated > 180 days
	UsingLatest     int `json:"usingLatestTags"`
	UsingDigest     int `json:"usingDigestPins"`
	NoRecentDeploys int `json:"noRecentDeploys"` // no deploy in 30+ days
	AvgAgeDays      int `json:"avgAgeDays"`
	FreshnessScore  int `json:"freshnessScore"` // 0-100 (100 = all fresh)
}

// StaleWorkload describes staleness info for one workload.
type StaleWorkload struct {
	Name           string   `json:"name"`
	Namespace      string   `json:"namespace"`
	Kind           string   `json:"kind"`
	AgeDays        int      `json:"ageDays"`
	UpdatedDaysAgo int      `json:"updatedDaysAgo"` // days since last update
	Replicas       int32    `json:"replicas"`
	ReadyReplicas  int32    `json:"readyReplicas"`
	Images         []string `json:"images"`
	ImageTags      []string `json:"imageTags"`
	UsesLatest     bool     `json:"usesLatest"`
	UsesDigest     bool     `json:"usesDigest"`
	UsesNoTag      bool     `json:"usesNoTag"`
	Status         string   `json:"status"`    // fresh, recent, stale, very-stale, ancient
	RiskLevel      string   `json:"riskLevel"` // low, medium, high, critical
}

// StaleNsStat aggregates per-namespace staleness.
type StaleNsStat struct {
	Namespace  string `json:"namespace"`
	Total      int    `json:"total"`
	Stale      int    `json:"stale"`
	VeryStale  int    `json:"veryStale"`
	AvgAgeDays int    `json:"avgAgeDays"`
}

// ImageAgeBucket groups workloads by age range.
type ImageAgeBucket struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

// handleStalenessCheck analyzes workload deployment staleness.
// GET /api/product/staleness?namespace=xxx
func (s *Server) handleStalenessCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")

	deployments, _ := rc.clientset.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
	statefulsets, _ := rc.clientset.AppsV1().StatefulSets(ns).List(ctx, metav1.ListOptions{})
	daemonsets, _ := rc.clientset.AppsV1().DaemonSets(ns).List(ctx, metav1.ListOptions{})

	result := StalenessResult{ScannedAt: time.Now()}
	nsMap := make(map[string]*StaleNsStat)
	now := time.Now()
	totalAgeDays := 0

	// Helper to process each workload
	process := func(kind string, name, namespace string, creationTime, updateTime metav1.Time, replicas, readyReplicas int32, images []string) {
		ageDays := int(now.Sub(creationTime.Time).Hours() / 24)
		updatedDaysAgo := int(now.Sub(updateTime.Time).Hours() / 24)

		wl := StaleWorkload{
			Name:           name,
			Namespace:      namespace,
			Kind:           kind,
			AgeDays:        ageDays,
			UpdatedDaysAgo: updatedDaysAgo,
			Replicas:       replicas,
			ReadyReplicas:  readyReplicas,
			Images:         images,
		}

		// Analyze images
		for _, img := range images {
			tag := extractImageTag(img)
			wl.ImageTags = append(wl.ImageTags, tag)
			if tag == "latest" {
				wl.UsesLatest = true
			}
			if tag == "" {
				wl.UsesNoTag = true
			}
			if strings.Contains(img, "@sha256:") {
				wl.UsesDigest = true
			}
		}

		// Determine staleness status
		switch {
		case updatedDaysAgo <= 7:
			wl.Status = "fresh"
		case updatedDaysAgo <= 30:
			wl.Status = "recent"
		case updatedDaysAgo <= 90:
			wl.Status = "stale"
		case updatedDaysAgo <= 180:
			wl.Status = "very-stale"
		default:
			wl.Status = "ancient"
		}

		// Risk level
		wl.RiskLevel = assessStalenessRisk(wl)

		result.Workloads = append(result.Workloads, wl)
		totalAgeDays += ageDays

		// Summary updates
		result.Summary.TotalWorkloads++
		switch wl.Status {
		case "fresh":
			result.Summary.FreshWorkloads++
		case "stale":
			result.Summary.StaleWorkloads++
		case "very-stale":
			result.Summary.VeryStaleWL++
			result.Summary.StaleWorkloads++
		case "ancient":
			result.Summary.AncientWL++
			result.Summary.VeryStaleWL++
			result.Summary.StaleWorkloads++
		}
		if wl.UsesLatest {
			result.Summary.UsingLatest++
		}
		if wl.UsesDigest {
			result.Summary.UsingDigest++
		}
		if updatedDaysAgo > 30 {
			result.Summary.NoRecentDeploys++
		}

		// Namespace stats
		nsStat := getOrCreateStaleNs(nsMap, namespace)
		nsStat.Total++
		nsStat.AvgAgeDays += ageDays
		if wl.Status == "stale" || wl.Status == "very-stale" || wl.Status == "ancient" {
			nsStat.Stale++
		}
		if wl.Status == "very-stale" || wl.Status == "ancient" {
			nsStat.VeryStale++
		}
	}

	for i := range deployments.Items {
		d := &deployments.Items[i]
		var images []string
		for _, c := range d.Spec.Template.Spec.Containers {
			images = append(images, c.Image)
		}
		process("Deployment", d.Name, d.Namespace, d.CreationTimestamp, getLatestUpdateTime(d.Annotations, d.CreationTimestamp), *d.Spec.Replicas, d.Status.ReadyReplicas, images)
	}

	for i := range statefulsets.Items {
		sts := &statefulsets.Items[i]
		var images []string
		for _, c := range sts.Spec.Template.Spec.Containers {
			images = append(images, c.Image)
		}
		process("StatefulSet", sts.Name, sts.Namespace, sts.CreationTimestamp, getLatestUpdateTime(sts.Annotations, sts.CreationTimestamp), *sts.Spec.Replicas, sts.Status.ReadyReplicas, images)
	}

	for i := range daemonsets.Items {
		ds := &daemonsets.Items[i]
		var images []string
		for _, c := range ds.Spec.Template.Spec.Containers {
			images = append(images, c.Image)
		}
		process("DaemonSet", ds.Name, ds.Namespace, ds.CreationTimestamp, getLatestUpdateTime(ds.Annotations, ds.CreationTimestamp), ds.Status.DesiredNumberScheduled, ds.Status.NumberReady, images)
	}

	// Calculate averages
	if result.Summary.TotalWorkloads > 0 {
		result.Summary.AvgAgeDays = totalAgeDays / result.Summary.TotalWorkloads
		result.Summary.FreshnessScore = calculateFreshnessScore(result.Summary)
	}

	// Build namespace stats
	for _, nsStat := range nsMap {
		if nsStat.Total > 0 {
			nsStat.AvgAgeDays = nsStat.AvgAgeDays / nsStat.Total
		}
		result.ByNamespace = append(result.ByNamespace, *nsStat)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		if result.ByNamespace[i].Stale != result.ByNamespace[j].Stale {
			return result.ByNamespace[i].Stale > result.ByNamespace[j].Stale
		}
		return result.ByNamespace[i].AvgAgeDays > result.ByNamespace[j].AvgAgeDays
	})

	// Build age buckets
	result.ImageAgeBuckets = buildAgeBuckets(result.Workloads)

	// Sort workloads by staleness
	sort.Slice(result.Workloads, func(i, j int) bool {
		return result.Workloads[i].UpdatedDaysAgo > result.Workloads[j].UpdatedDaysAgo
	})

	// Recommendations
	result.Recommendations = generateStalenessRecommendations(result)

	writeJSON(w, result)
}

// extractImageTag extracts the tag from an image reference.
func extractImageTag(image string) string {
	// Strip digest first
	if idx := strings.Index(image, "@"); idx >= 0 {
		return "sha256"
	}
	// Find last colon after last slash
	lastSlash := strings.LastIndex(image, "/")
	searchPart := image
	if lastSlash >= 0 {
		searchPart = image[lastSlash+1:]
	}
	if idx := strings.LastIndex(searchPart, ":"); idx >= 0 {
		return searchPart[idx+1:]
	}
	return ""
}

// getLatestUpdateTime checks deployment.kubernetes.io/revision or kubectl.kubernetes.io/restartedAt annotation.
func getLatestUpdateTime(annotations map[string]string, creationTime metav1.Time) metav1.Time {
	if annotations != nil {
		// Check kubectl restart annotation
		if restartedAt, ok := annotations["kubectl.kubernetes.io/restartedAt"]; ok {
			if t, err := time.Parse(time.RFC3339, restartedAt); err == nil {
				return metav1.Time{Time: t}
			}
		}
		// Check deployment revision
		if revision, ok := annotations["deployment.kubernetes.io/revision"]; ok && revision != "" {
			// Can't parse exact time from revision, use creation time as fallback
		}
	}
	return creationTime
}

// assessStalenessRisk determines risk level.
func assessStalenessRisk(wl StaleWorkload) string {
	risk := 0
	if wl.Status == "ancient" {
		risk += 20
	} else if wl.Status == "very-stale" {
		risk += 12
	} else if wl.Status == "stale" {
		risk += 6
	}
	if wl.UsesLatest {
		risk += 10
	}
	if wl.UsesNoTag {
		risk += 10
	}
	if !wl.UsesDigest && wl.Status != "fresh" {
		risk += 3
	}

	switch {
	case risk >= 25:
		return "critical"
	case risk >= 15:
		return "high"
	case risk >= 8:
		return "medium"
	default:
		return "low"
	}
}

// calculateFreshnessScore computes 0-100.
func calculateFreshnessScore(s StalenessSummary) int {
	if s.TotalWorkloads == 0 {
		return 100
	}
	score := 100
	score -= s.AncientWL * 10
	score -= (s.VeryStaleWL - s.AncientWL) * 5
	score -= (s.StaleWorkloads - s.VeryStaleWL) * 2
	score -= s.UsingLatest * 3
	if score < 0 {
		score = 0
	}
	return score
}

// buildAgeBuckets groups workloads by age ranges.
func buildAgeBuckets(workloads []StaleWorkload) []ImageAgeBucket {
	buckets := []ImageAgeBucket{
		{Label: "<7d", Count: 0},
		{Label: "7-30d", Count: 0},
		{Label: "30-90d", Count: 0},
		{Label: "90-180d", Count: 0},
		{Label: ">180d", Count: 0},
	}

	for _, wl := range workloads {
		age := wl.UpdatedDaysAgo
		switch {
		case age <= 7:
			buckets[0].Count++
		case age <= 30:
			buckets[1].Count++
		case age <= 90:
			buckets[2].Count++
		case age <= 180:
			buckets[3].Count++
		default:
			buckets[4].Count++
		}
	}

	return buckets
}

// generateStalenessRecommendations produces actionable advice.
func generateStalenessRecommendations(result StalenessResult) []string {
	var recs []string
	s := result.Summary

	if s.AncientWL > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) haven't been updated in >180 days — review for security patches and dependency updates", s.AncientWL))
	}
	if s.VeryStaleWL > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) are very stale (>90 days) — schedule maintenance windows for updates", s.VeryStaleWL))
	}
	if s.UsingLatest > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) use :latest tags — image content may change without notice, pin versions for reproducibility", s.UsingLatest))
	}
	if s.NoRecentDeploys > 0 {
		recs = append(recs, fmt.Sprintf("%d workload(s) have no deploy in 30+ days — verify they're still needed and compatible with current cluster version", s.NoRecentDeploys))
	}
	if s.FreshnessScore < 50 {
		recs = append(recs, fmt.Sprintf("Freshness score is %d/100 — establish regular update cadence and automated dependency scanning", s.FreshnessScore))
	}

	// Top stale namespace
	if len(result.ByNamespace) > 0 && result.ByNamespace[0].Stale > 0 {
		recs = append(recs, fmt.Sprintf("Namespace %q has %d stale workload(s) — prioritize updates in this namespace", result.ByNamespace[0].Namespace, result.ByNamespace[0].Stale))
	}

	return recs
}

func getOrCreateStaleNs(m map[string]*StaleNsStat, ns string) *StaleNsStat {
	if e, ok := m[ns]; ok {
		return e
	}
	e := &StaleNsStat{Namespace: ns}
	m[ns] = e
	return e
}

// Ensure appsv1 and corev1 are used.
var _ appsv1.DeploymentSpec = appsv1.DeploymentSpec{}
var _ corev1.Container = corev1.Container{}
