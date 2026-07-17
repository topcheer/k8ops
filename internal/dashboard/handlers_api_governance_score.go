package dashboard

import (
	"fmt"
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// APIGovernanceScoreResult evaluates API governance maturity.
type APIGovernanceScoreResult struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	Summary         GovSummary       `json:"summary"`
	ByVersion       []GovVersionStat `json:"byVersion"`
	DeprecatedAPIs  []GovDeprecated  `json:"deprecatedAPIs"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Recommendations []string         `json:"recommendations"`
}

type GovSummary struct {
	TotalAPIs      int `json:"totalAPIs"`
	StableAPIs     int `json:"stableAPIs"`
	BetaAPIs       int `json:"betaAPIs"`
	DeprecatedAPIs int `json:"deprecatedAPIs"`
}

type GovVersionStat struct {
	Group   string `json:"group"`
	Version string `json:"version"`
	Count   int    `json:"count"`
	Status  string `json:"status"`
}

type GovDeprecated struct {
	API      string `json:"api"`
	Resource string `json:"resource"`
	Severity string `json:"severity"`
	Action   string `json:"action"`
}

// handleAPIGovernanceScore handles GET /api/docs/api-governance-score
func (s *Server) handleAPIGovernanceScore(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := APIGovernanceScoreResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})

	verMap := make(map[string]int)
	var deprecated []GovDeprecated

	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		result.Summary.TotalAPIs++
		apiVer := string(d.APIVersion)
		if apiVer == "" {
			apiVer = "apps/v1"
		}
		verMap[apiVer]++

		switch apiVer {
		case "extensions/v1beta1", "apps/v1beta1", "apps/v1beta2":
			result.Summary.DeprecatedAPIs++
			deprecated = append(deprecated, GovDeprecated{
				API: apiVer, Resource: "Deployment",
				Severity: "critical", Action: "Migrate to apps/v1",
			})
		case "apps/v1":
			result.Summary.StableAPIs++
		default:
			if govContains(apiVer, "beta") {
				result.Summary.BetaAPIs++
			} else {
				result.Summary.StableAPIs++
			}
		}
	}

	for ver, count := range verMap {
		status := "stable"
		if govContains(ver, "beta") {
			status = "beta"
		}
		if govContains(ver, "deprecated") || ver == "extensions/v1beta1" || ver == "apps/v1beta1" {
			status = "deprecated"
		}
		result.ByVersion = append(result.ByVersion, GovVersionStat{
			Group: ver, Version: ver, Count: count, Status: status,
		})
	}

	result.DeprecatedAPIs = deprecated

	if result.Summary.TotalAPIs > 0 {
		result.HealthScore = result.Summary.StableAPIs * 100 / result.Summary.TotalAPIs
	} else {
		result.HealthScore = 100
	}

	switch {
	case result.HealthScore >= 90:
		result.Grade = "A"
	case result.HealthScore >= 75:
		result.Grade = "B"
	case result.HealthScore >= 50:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	result.Recommendations = buildGovScoreRecs(&result)
	writeJSON(w, result)
}

func govContains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

func buildGovScoreRecs(r *APIGovernanceScoreResult) []string {
	recs := []string{
		fmt.Sprintf("API 治理: %d stable, %d beta, %d deprecated", r.Summary.StableAPIs, r.Summary.BetaAPIs, r.Summary.DeprecatedAPIs),
	}
	if r.Summary.DeprecatedAPIs > 0 {
		recs = append(recs, fmt.Sprintf("%d APIs using deprecated versions", r.Summary.DeprecatedAPIs))
	}
	if r.Summary.BetaAPIs > 5 {
		recs = append(recs, fmt.Sprintf("%d APIs using beta versions", r.Summary.BetaAPIs))
	}
	return recs
}
