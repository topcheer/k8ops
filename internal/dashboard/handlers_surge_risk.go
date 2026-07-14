package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
)

// SurgeRiskResult is the rolling update risk & surge configuration audit.
type SurgeRiskResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	Summary         SurgeRiskSummary  `json:"summary"`
	Deployments     []SurgeDeployment `json:"deployments"`
	Risks           []SurgeRisk       `json:"risks"`
	Recommendations []string          `json:"recommendations"`
	HealthScore     int               `json:"healthScore"`
}

// SurgeRiskSummary aggregates surge risk statistics.
type SurgeRiskSummary struct {
	TotalDeployments   int `json:"totalDeployments"`
	WithSurge          int `json:"withSurge"`
	WithMaxUnavailable int `json:"withMaxUnavailable"`
	HighSurge          int `json:"highSurge"`       // surge > 50%
	HighUnavailable    int `json:"highUnavailable"` // maxUnavailable > 50%
	NoStrategy         int `json:"noStrategy"`      // no update strategy set
	RollingStrategy    int `json:"rollingStrategy"`
	RecreateStrategy   int `json:"recreateStrategy"`
	RiskyConfigs       int `json:"riskyConfigs"`
}

// SurgeDeployment describes a deployment's surge configuration.
type SurgeDeployment struct {
	Name               string  `json:"name"`
	Namespace          string  `json:"namespace"`
	Strategy           string  `json:"strategy"`
	MaxSurge           string  `json:"maxSurge"`
	MaxUnavailable     string  `json:"maxUnavailable"`
	SurgePercent       float64 `json:"surgePercent"`
	UnavailablePercent float64 `json:"unavailablePercent"`
	Replicas           int     `json:"replicas"`
	RiskLevel          string  `json:"riskLevel"`
	Issue              string  `json:"issue,omitempty"`
}

// SurgeRisk describes a specific risk.
type SurgeRisk struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	Severity  string `json:"severity"`
	Issue     string `json:"issue"`
}

// handleSurgeRisk audits rolling update risk & surge configuration.
// GET /api/deployment/surge-risk
func (s *Server) handleSurgeRisk(w http.ResponseWriter, r *http.Request) {
	result := SurgeRiskResult{
		ScannedAt: time.Now(),
	}

	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	systemNamespaces := map[string]bool{
		"kube-system":     true,
		"kube-public":     true,
		"kube-node-lease": true,
	}

	deployments, err := rc.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, dep := range deployments.Items {
			if systemNamespaces[dep.Namespace] {
				continue
			}
			result.Summary.TotalDeployments++

			replicas := 1
			if dep.Spec.Replicas != nil {
				replicas = int(*dep.Spec.Replicas)
			}

			strategy := string(dep.Spec.Strategy.Type)
			if strategy == "" {
				strategy = "RollingUpdate"
			}

			entry := SurgeDeployment{
				Name:      dep.Name,
				Namespace: dep.Namespace,
				Strategy:  strategy,
				Replicas:  replicas,
				RiskLevel: "low",
			}

			if strategy == "RollingUpdate" {
				result.Summary.RollingStrategy++
				surge := dep.Spec.Strategy.RollingUpdate.MaxSurge
				unavail := dep.Spec.Strategy.RollingUpdate.MaxUnavailable

				surgeStr := "25%"
				unavailStr := "25%"
				surgePct := 25.0
				unavailPct := 25.0

				if surge != nil {
					surgeStr = surge.String()
					if surge.Type == intstr.Int {
						surgePct = float64(surge.IntValue()) / float64(replicas) * 100
					} else if surge.Type == intstr.String {
						s := strings.TrimSuffix(surge.StrVal, "%")
						fmt.Sscanf(s, "%f", &surgePct)
					}
				}
				if unavail != nil {
					unavailStr = unavail.String()
					if unavail.Type == intstr.Int {
						unavailPct = float64(unavail.IntValue()) / float64(replicas) * 100
					} else if unavail.Type == intstr.String {
						s := strings.TrimSuffix(unavail.StrVal, "%")
						fmt.Sscanf(s, "%f", &unavailPct)
					}
				}

				entry.MaxSurge = surgeStr
				entry.MaxUnavailable = unavailStr
				entry.SurgePercent = surgePct
				entry.UnavailablePercent = unavailPct

				result.Summary.WithSurge++
				result.Summary.WithMaxUnavailable++

				// Risk assessment
				riskLevel := "low"
				issue := ""

				if surgePct > 50 {
					riskLevel = "high"
					issue = "High surge (>50%) may overwhelm resources during rollout"
					result.Summary.HighSurge++
					result.Summary.RiskyConfigs++
					result.Risks = append(result.Risks, SurgeRisk{
						Namespace: dep.Namespace, Name: dep.Name, Severity: "high", Issue: issue,
					})
				} else if surgePct > 25 {
					riskLevel = "medium"
					issue = "Moderate surge — monitor resource usage during rollout"
					result.Summary.RiskyConfigs++
				}

				if unavailPct > 50 {
					riskLevel = "high"
					if issue != "" {
						issue += "; "
					}
					issue += "High maxUnavailable (>50%) reduces availability during rollout"
					result.Summary.HighUnavailable++
					result.Summary.RiskyConfigs++
					result.Risks = append(result.Risks, SurgeRisk{
						Namespace: dep.Namespace, Name: dep.Name, Severity: "high", Issue: "High maxUnavailable reduces availability",
					})
				} else if unavailPct > 25 {
					if riskLevel == "low" {
						riskLevel = "medium"
					}
				}

				if surgePct == 0 && unavailPct == 0 {
					riskLevel = "critical"
					issue = "Both maxSurge and maxUnavailable are 0 — rollout will stall"
					result.Summary.RiskyConfigs++
					result.Risks = append(result.Risks, SurgeRisk{
						Namespace: dep.Namespace, Name: dep.Name, Severity: "critical", Issue: issue,
					})
				}

				if unavailPct >= 100 {
					riskLevel = "critical"
					issue = "maxUnavailable=100% — all pods can be down during rollout"
					result.Summary.RiskyConfigs++
					result.Risks = append(result.Risks, SurgeRisk{
						Namespace: dep.Namespace, Name: dep.Name, Severity: "critical", Issue: issue,
					})
				}

				entry.RiskLevel = riskLevel
				entry.Issue = issue

			} else if strategy == "Recreate" {
				result.Summary.RecreateStrategy++
				entry.RiskLevel = "medium"
				entry.Issue = "Recreate strategy causes downtime during rollout"
				result.Summary.RiskyConfigs++
				result.Risks = append(result.Risks, SurgeRisk{
					Namespace: dep.Namespace, Name: dep.Name, Severity: "medium", Issue: "Recreate strategy causes downtime",
				})
			}

			result.Deployments = append(result.Deployments, entry)
		}
	}

	sort.Slice(result.Deployments, func(i, j int) bool {
		return result.Deployments[i].RiskLevel > result.Deployments[j].RiskLevel
	})
	sort.Slice(result.Risks, func(i, j int) bool {
		return result.Risks[i].Severity > result.Risks[j].Severity
	})

	// Recommendations
	if result.Summary.HighSurge > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d deployments have high surge (>50%%) — reduce maxSurge to avoid resource pressure", result.Summary.HighSurge))
	}
	if result.Summary.HighUnavailable > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d deployments have high maxUnavailable (>50%%) — reduce to maintain availability during rollout", result.Summary.HighUnavailable))
	}
	if result.Summary.RecreateStrategy > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d deployments use Recreate strategy — switch to RollingUpdate for zero-downtime", result.Summary.RecreateStrategy))
	}

	// Health score
	score := 100
	score -= result.Summary.HighSurge * 5
	score -= result.Summary.HighUnavailable * 8
	score -= result.Summary.RecreateStrategy * 3
	if result.Summary.RiskyConfigs > 0 {
		score -= 5
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	writeJSON(w, result)
}
