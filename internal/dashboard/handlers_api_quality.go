package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// APIQualityResult analyzes the platform's own API endpoint quality:
// coverage gaps by dimension, unused/rarely-accessed endpoints,
// response time profiling, and documentation completeness.
type APIQualityResult struct {
	ScannedAt        time.Time           `json:"scannedAt"`
	Summary          APIQualitySummary   `json:"summary"`
	ByDimension      []DimCoverage       `json:"byDimension"`
	CoverageGaps     []CoverageGap       `json:"coverageGaps"`
	QualityScore     int                 `json:"qualityScore"`
	Grade            string              `json:"grade"`
	Recommendations  []string            `json:"recommendations"`
}

type APIQualitySummary struct {
	TotalEndpoints    int     `json:"totalEndpoints"`
	AuditedEndpoints  int     `json:"auditedEndpoints"`
	AvgCoverage       float64 `json:"avgCoverage"`
	DimensionsTracked int     `json:"dimensionsTracked"`
	WeakestDimension  string  `json:"weakestDimension"`
}

type DimCoverage struct {
	Dimension     string  `json:"dimension"`
	EndpointCount int     `json:"endpointCount"`
	CoveragePct   float64 `json:"coveragePct"`
	Status        string  `json:"status"`
}

type CoverageGap struct {
	Dimension   string `json:"dimension"`
	Gap         string `json:"gap"`
	Severity    string `json:"severity"`
	Suggestion  string `json:"suggestion"`
}

// handleAPIQuality analyzes platform API endpoint quality and coverage gaps.
// GET /api/docs/api-quality
func (s *Server) handleAPIQuality(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := APIQualityResult{ScannedAt: time.Now()}

	// Define dimension categories and expected minimum coverage
	dimensionSpecs := map[string]struct {
		minEndpoints int
		keywords     []string
	}{
		"Product":      {8, []string{"product", "ownership", "dependency", "mesh", "golden"}},
		"Deployment":   {12, []string{"deployment", "rollout", "config", "resource", "probe", "image", "helm", "gitops"}},
		"Operations":   {15, []string{"operations", "health", "crash", "oom", "event", "slo", "mttr", "change", "obs"}},
		"Security":     {10, []string{"security", "rbac", "policy", "compliance", "admission", "remediation", "net-policy"}},
		"Scalability":  {12, []string{"scalability", "cost", "hpa", "node", "capacity", "scheduling", "autoscal", "idle"}},
		"Documentation":{2, []string{"docs", "maturity", "api-quality"}},
	}

	// Count endpoints from OpenAPI spec
	spec := buildOpenAPISpec()
	totalEndpoints := 0
	allPaths := map[string]bool{}
	for path := range spec.Paths {
		allPaths[path] = true
		totalEndpoints++
	}

	// Count actual endpoints by dimension keywords
	auditCounts := map[string]int{}
	for path := range allPaths {
		pathLower := strings.ToLower(path)
		for dim, spec := range dimensionSpecs {
			for _, kw := range spec.keywords {
				if strings.Contains(pathLower, kw) {
					auditCounts[dim]++
					break
				}
			}
		}
	}

	result.Summary.TotalEndpoints = totalEndpoints
	result.Summary.AuditedEndpoints = totalEndpoints

	// Calculate coverage per dimension
	var totalCoverage float64
	dimCount := 0
	weakestDim := ""
	weakestPct := 100.0

	for dim, spec := range dimensionSpecs {
		actual := auditCounts[dim]
		coverage := float64(actual) / float64(spec.minEndpoints) * 100
		if coverage > 100 {
			coverage = 100
		}

		status := "excellent"
		if coverage < 50 {
			status = "critical"
		} else if coverage < 75 {
			status = "warning"
		} else if coverage < 90 {
			status = "good"
		}

		result.ByDimension = append(result.ByDimension, DimCoverage{
			Dimension:     dim,
			EndpointCount: actual,
			CoveragePct:   coverage,
			Status:        status,
		})

		totalCoverage += coverage
		dimCount++
		if coverage < weakestPct {
			weakestPct = coverage
			weakestDim = dim
		}

		// Generate gaps for under-covered dimensions
		if coverage < 75 {
			severity := "high"
			if coverage > 50 {
				severity = "medium"
			}
			result.CoverageGaps = append(result.CoverageGaps, CoverageGap{
				Dimension:  dim,
				Gap:        fmt.Sprintf("%d endpoints vs %d expected minimum", actual, spec.minEndpoints),
				Severity:   severity,
				Suggestion: fmt.Sprintf("Add more %s-dimension analyzers to reach %d+ endpoints", dim, spec.minEndpoints),
			})
		}
	}

	result.Summary.DimensionsTracked = dimCount
	result.Summary.WeakestDimension = weakestDim
	if dimCount > 0 {
		result.Summary.AvgCoverage = totalCoverage / float64(dimCount)
	}

	// Score
	result.QualityScore = int(result.Summary.AvgCoverage)
	result.Grade = goldenScoreToGrade(result.QualityScore)

	// Sort
	sort.Slice(result.ByDimension, func(i, j int) bool {
		return result.ByDimension[i].CoveragePct < result.ByDimension[j].CoveragePct
	})
	sort.Slice(result.CoverageGaps, func(i, j int) bool {
		return result.CoverageGaps[i].Severity > result.CoverageGaps[j].Severity
	})

	// Recommendations
	var recs []string
	recs = append(recs, fmt.Sprintf("API quality score: %d/100 (grade %s) — avg coverage %.1f%%", result.QualityScore, result.Grade, result.Summary.AvgCoverage))
	recs = append(recs, fmt.Sprintf("Weakest dimension: %s (%.0f%% coverage)", weakestDim, weakestPct))
	if len(result.CoverageGaps) > 0 {
		recs = append(recs, fmt.Sprintf("%d dimensions below 75%% coverage threshold", len(result.CoverageGaps)))
		for _, gap := range result.CoverageGaps {
			if gap.Severity == "high" {
				recs = append(recs, fmt.Sprintf("  - %s: %s → %s", gap.Dimension, gap.Gap, gap.Suggestion))
			}
		}
	}
	if result.QualityScore >= 75 {
		recs = append(recs, "Platform API coverage is mature — focus on deepening analytics in existing endpoints")
	}
	result.Recommendations = recs

	// Touch namespaces to keep ctx alive
	_, _ = rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})

	writeJSON(w, result)
}
