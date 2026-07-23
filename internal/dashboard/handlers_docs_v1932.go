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

// ============================================================
// v19.32 — Documentation Dimension (Round 8)
// 1. Label Standardization Report — label governance & consistency
// 2. Resource Age Distribution — workload lifecycle age analysis
// 3. Namespace Isolation Matrix — namespace boundary documentation
// ============================================================

// ---------------------------------------------------------------
// 1. Label Standardization Report
// ---------------------------------------------------------------

type LabelStdResult1932 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         LabelStdSummary1932  `json:"summary"`
	TopLabels       []LabelStat1932      `json:"topLabels"`
	Violations      []LabelViolation1932 `json:"violations"`
	ByResourceKind  []LabelKindStat1932  `json:"byResourceKind"`
	Recommendations []string             `json:"recommendations"`
}

type LabelStdSummary1932 struct {
	TotalResources  int     `json:"totalResources"`
	WithLabels      int     `json:"withLabels"`
	WithoutLabels   int     `json:"withoutLabels"`
	UniqueLabelKeys int     `json:"uniqueLabelKeys"`
	StandardKeys    int     `json:"standardKeys"`
	CustomKeys      int     `json:"customKeys"`
	ComplianceRate  float64 `json:"complianceRate"`
}

type LabelStat1932 struct {
	Key      string `json:"key"`
	Count    int    `json:"count"`
	Category string `json:"category"`
}

type LabelViolation1932 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Violation string `json:"violation"`
	Severity  string `json:"severity"`
}

type LabelKindStat1932 struct {
	Kind       string `json:"kind"`
	Total      int    `json:"total"`
	WithLabels int    `json:"withLabels"`
}

func (s *Server) handleLabelStandardization(w http.ResponseWriter, r *http.Request) {
	result := LabelStdResult1932{ScannedAt: time.Now()}
	score := 100
	labelCounts := make(map[string]int)
	labelSet := make(map[string]bool)
	kindStats := make(map[string]*LabelKindStat1932)
	standardPrefixes := []string{"app.kubernetes.io/", "app=", "tier=", "env=", "version="}

	checkResource := func(name, ns, kind string, labels map[string]string) {
		if isSystemNamespace(ns) {
			return
		}
		result.Summary.TotalResources++
		ks, exists := kindStats[kind]
		if !exists {
			ks = &LabelKindStat1932{Kind: kind}
			kindStats[kind] = ks
		}
		ks.Total++

		if len(labels) == 0 {
			result.Summary.WithoutLabels++
			result.Violations = append(result.Violations, LabelViolation1932{
				Name: name, Namespace: ns, Kind: kind,
				Violation: "No labels — difficult to select or manage",
				Severity:  "medium",
			})
			score -= 2
			return
		}
		result.Summary.WithLabels++
		ks.WithLabels++

		hasAppName := false
		for k := range labels {
			labelCounts[k]++
			labelSet[k] = true
			if k == "app" || k == "app.kubernetes.io/name" {
				hasAppName = true
			}
		}
		if !hasAppName {
			result.Violations = append(result.Violations, LabelViolation1932{
				Name: name, Namespace: ns, Kind: kind,
				Violation: "Missing 'app' or 'app.kubernetes.io/name' label",
				Severity:  "low",
			})
			score -= 1
		}
	}

	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, d := range depList.Items {
		checkResource(d.Name, d.Namespace, "Deployment", d.Labels)
	}

	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, sv := range svcList.Items {
		checkResource(sv.Name, sv.Namespace, "Service", sv.Labels)
	}

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, p := range podList.Items {
		if p.Status.Phase != corev1.PodRunning {
			continue
		}
		checkResource(p.Name, p.Namespace, "Pod", p.Labels)
	}

	result.Summary.UniqueLabelKeys = len(labelSet)
	for k := range labelSet {
		isStandard := false
		for _, sp := range standardPrefixes {
			if strings.HasPrefix(k, strings.TrimSuffix(sp, "=")) || k == strings.TrimSuffix(sp, "=") {
				isStandard = true
				break
			}
		}
		if isStandard {
			result.Summary.StandardKeys++
		} else {
			result.Summary.CustomKeys++
		}
	}

	for k, c := range labelCounts {
		category := "custom"
		if strings.HasPrefix(k, "app.kubernetes.io/") {
			category = "standard"
		}
		result.TopLabels = append(result.TopLabels, LabelStat1932{Key: k, Count: c, Category: category})
	}
	sort.Slice(result.TopLabels, func(i, j int) bool { return result.TopLabels[i].Count > result.TopLabels[j].Count })
	if len(result.TopLabels) > 20 {
		result.TopLabels = result.TopLabels[:20]
	}

	for _, ks := range kindStats {
		result.ByResourceKind = append(result.ByResourceKind, *ks)
	}

	if result.Summary.TotalResources > 0 {
		result.Summary.ComplianceRate = float64(result.Summary.WithLabels) * 100 / float64(result.Summary.TotalResources)
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.WithoutLabels > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d resources without labels — add app/name labels for discoverability", result.Summary.WithoutLabels))
	}
	if result.Summary.CustomKeys > result.Summary.StandardKeys {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d custom label keys — standardize on app.kubernetes.io/* convention", result.Summary.CustomKeys))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Resource Age Distribution
// ---------------------------------------------------------------

type ResAgeResult1932 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         ResAgeSummary1932  `json:"summary"`
	Distribution    []ResAgeBucket1932 `json:"distribution"`
	OldestResources []ResAgeEntry1932  `json:"oldestResources"`
	NewestResources []ResAgeEntry1932  `json:"newestResources"`
	Recommendations []string           `json:"recommendations"`
}

type ResAgeSummary1932 struct {
	TotalResources   int     `json:"totalResources"`
	AvgAgeDays       float64 `json:"avgAgeDays"`
	MaxAgeDays       float64 `json:"maxAgeDays"`
	MinAgeDays       float64 `json:"minAgeDays"`
	OlderThan1Year   int     `json:"olderThan1Year"`
	OlderThan6Months int     `json:"olderThan6Months"`
	NewerThan7Days   int     `json:"newerThan7Days"`
}

type ResAgeBucket1932 struct {
	Range string `json:"range"`
	Count int    `json:"count"`
}

type ResAgeEntry1932 struct {
	Name      string  `json:"name"`
	Namespace string  `json:"namespace"`
	Kind      string  `json:"kind"`
	AgeDays   float64 `json:"ageDays"`
}

func (s *Server) handleResourceAgeDistribution(w http.ResponseWriter, r *http.Request) {
	result := ResAgeResult1932{ScannedAt: time.Now()}
	score := 100
	var allAges []float64
	var entries []ResAgeEntry1932

	collectAge := func(name, ns, kind string, creationTime time.Time) {
		if isSystemNamespace(ns) {
			return
		}
		ageDays := time.Since(creationTime).Hours() / 24
		entries = append(entries, ResAgeEntry1932{Name: name, Namespace: ns, Kind: kind, AgeDays: ageDays})
		allAges = append(allAges, ageDays)
		result.Summary.TotalResources++
		if ageDays > result.Summary.MaxAgeDays {
			result.Summary.MaxAgeDays = ageDays
		}
		if result.Summary.MinAgeDays == 0 || ageDays < result.Summary.MinAgeDays {
			result.Summary.MinAgeDays = ageDays
		}
		if ageDays > 365 {
			result.Summary.OlderThan1Year++
		}
		if ageDays > 180 {
			result.Summary.OlderThan6Months++
		}
		if ageDays < 7 {
			result.Summary.NewerThan7Days++
		}
	}

	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, d := range depList.Items {
		collectAge(d.Name, d.Namespace, "Deployment", d.CreationTimestamp.Time)
	}

	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, sv := range svcList.Items {
		collectAge(sv.Name, sv.Namespace, "Service", sv.CreationTimestamp.Time)
	}

	cmList, _ := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	for _, cm := range cmList.Items {
		collectAge(cm.Name, cm.Namespace, "ConfigMap", cm.CreationTimestamp.Time)
	}

	// Build buckets
	buckets := []struct {
		min, max float64
		label    string
	}{
		{0, 7, "0-7 days"}, {7, 30, "7-30 days"}, {30, 90, "30-90 days"},
		{90, 180, "90-180 days"}, {180, 365, "180-365 days"}, {365, 99999, "365+ days"},
	}
	for _, b := range buckets {
		count := 0
		for _, age := range allAges {
			if age >= b.min && age < b.max {
				count++
			}
		}
		result.Distribution = append(result.Distribution, ResAgeBucket1932{Range: b.label, Count: count})
	}

	// Sort for oldest/newest
	sort.Slice(entries, func(i, j int) bool { return entries[i].AgeDays > entries[j].AgeDays })
	if len(entries) > 10 {
		result.OldestResources = entries[:10]
	}
	if len(entries) > 20 {
		revIdx := len(entries) - 10
		result.NewestResources = entries[revIdx:]
	} else if len(entries) > 10 {
		result.NewestResources = entries[10:]
	}

	if len(allAges) > 0 {
		sum := 0.0
		for _, a := range allAges {
			sum += a
		}
		result.Summary.AvgAgeDays = sum / float64(len(allAges))
	}

	if result.Summary.OlderThan1Year > 10 {
		score -= 5
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.OlderThan1Year > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d resources older than 1 year — review for modernization", result.Summary.OlderThan1Year))
	}
	if result.Summary.NewerThan7Days > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d resources created in last 7 days — monitor stability", result.Summary.NewerThan7Days))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Namespace Isolation Matrix
// ---------------------------------------------------------------

type NSIsolationResult1932 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         NSIsolationSummary1932 `json:"summary"`
	Namespaces      []NSIsolationEntry1932 `json:"namespaces"`
	Risks           []NSIsolationRisk1932  `json:"risks"`
	Recommendations []string               `json:"recommendations"`
}

type NSIsolationSummary1932 struct {
	TotalNamespaces int `json:"totalNamespaces"`
	WithNetPol      int `json:"withNetworkPolicy"`
	WithQuota       int `json:"withResourceQuota"`
	WithLimitRange  int `json:"withLimitRange"`
	WithPSA         int `json:"withPSA"`
	IsolatedNS      int `json:"isolatedNamespaces"`
	OpenNS          int `json:"openNamespaces"`
}

type NSIsolationEntry1932 struct {
	Namespace string `json:"namespace"`
	HasNetPol bool   `json:"hasNetworkPolicy"`
	HasQuota  bool   `json:"hasResourceQuota"`
	HasLimit  bool   `json:"hasLimitRange"`
	HasPSA    bool   `json:"hasPSA"`
	Isolation string `json:"isolationLevel"`
	PodCount  int    `json:"podCount"`
}

type NSIsolationRisk1932 struct {
	Namespace string `json:"namespace"`
	RiskType  string `json:"riskType"`
	Severity  string `json:"severity"`
	Detail    string `json:"detail"`
}

func (s *Server) handleNSIsolationMatrix(w http.ResponseWriter, r *http.Request) {
	result := NSIsolationResult1932{ScannedAt: time.Now()}
	score := 100

	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	netPolList, _ := s.clientset.NetworkingV1().NetworkPolicies("").List(r.Context(), metav1.ListOptions{})
	rqList, _ := s.clientset.CoreV1().ResourceQuotas("").List(r.Context(), metav1.ListOptions{})
	lrList, _ := s.clientset.CoreV1().LimitRanges("").List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	netPolNS := make(map[string]bool)
	for _, np := range netPolList.Items {
		netPolNS[np.Namespace] = true
	}
	rqNS := make(map[string]bool)
	for _, rq := range rqList.Items {
		rqNS[rq.Namespace] = true
	}
	lrNS := make(map[string]bool)
	for _, lr := range lrList.Items {
		lrNS[lr.Namespace] = true
	}
	podCount := make(map[string]int)
	for _, p := range podList.Items {
		if p.Status.Phase == corev1.PodRunning {
			podCount[p.Namespace]++
		}
	}

	for _, ns := range nsList.Items {
		if isSystemNamespace(ns.Name) {
			continue
		}
		result.Summary.TotalNamespaces++

		hasNP := netPolNS[ns.Name]
		hasRQ := rqNS[ns.Name]
		hasLR := lrNS[ns.Name]
		hasPSA := ns.Labels["pod-security.kubernetes.io/enforce"] != ""

		isolation := "none"
		isolatedCount := 0
		if hasNP {
			isolatedCount++
		}
		if hasRQ {
			isolatedCount++
		}
		if hasLR {
			isolatedCount++
		}
		if hasPSA {
			isolatedCount++
		}

		if isolatedCount >= 3 {
			isolation = "high"
			result.Summary.IsolatedNS++
		} else if isolatedCount >= 1 {
			isolation = "partial"
		} else {
			isolation = "none"
			result.Summary.OpenNS++
			result.Risks = append(result.Risks, NSIsolationRisk1932{
				Namespace: ns.Name, RiskType: "no-isolation", Severity: "high",
				Detail: "Namespace has no NetworkPolicy, Quota, LimitRange, or PSA — fully open",
			})
			score -= 3
		}

		if hasNP {
			result.Summary.WithNetPol++
		}
		if hasRQ {
			result.Summary.WithQuota++
		}
		if hasLR {
			result.Summary.WithLimitRange++
		}
		if hasPSA {
			result.Summary.WithPSA++
		}

		result.Namespaces = append(result.Namespaces, NSIsolationEntry1932{
			Namespace: ns.Name, HasNetPol: hasNP, HasQuota: hasRQ, HasLimit: hasLR, HasPSA: hasPSA,
			Isolation: isolation, PodCount: podCount[ns.Name],
		})
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.OpenNS > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d namespaces with no isolation controls — add NetworkPolicy + PSA", result.Summary.OpenNS))
	}
	if result.Summary.WithNetPol < result.Summary.TotalNamespaces/2 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("Only %d/%d namespaces have NetworkPolicy — expand coverage", result.Summary.WithNetPol, result.Summary.TotalNamespaces))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
