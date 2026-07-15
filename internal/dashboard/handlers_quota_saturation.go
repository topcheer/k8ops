package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// QuotaSaturationResult is the namespace resource quota saturation & limit exhaustion predictor.
type QuotaSaturationResult struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	Summary         QuotaSatSummary  `json:"summary"`
	ByNamespace     []QuotaSatNSStat `json:"byNamespace"`
	CriticalNS      []QuotaSatNSStat `json:"criticalNS"`
	Risks           []QuotaSatRisk   `json:"risks"`
	Recommendations []string         `json:"recommendations"`
	HealthScore     int              `json:"healthScore"`
}

// QuotaSatSummary aggregates quota saturation metrics.
type QuotaSatSummary struct {
	TotalNamespaces    int `json:"totalNamespaces"`
	NSWithQuota        int `json:"nsWithQuota"`
	NSWithoutQuota     int `json:"nsWithoutQuota"`
	CriticalSaturation int `json:"criticalSaturation"` // >90% used
	HighSaturation     int `json:"highSaturation"`     // 70-90% used
	MediumSaturation   int `json:"mediumSaturation"`   // 50-70% used
	LowSaturation      int `json:"lowSaturation"`      // <50% used
	ExhaustedQuotas    int `json:"exhaustedQuotas"`    // 100% used
}

// QuotaSatNSStat per-namespace quota saturation.
type QuotaSatNSStat struct {
	Namespace     string          `json:"namespace"`
	HasQuota      bool            `json:"hasQuota"`
	QuotaItems    []QuotaItemStat `json:"quotaItems,omitempty"`
	MaxSaturation float64         `json:"maxSaturation"` // highest saturation % across all quotas
	RiskLevel     string          `json:"riskLevel"`     // low, medium, high, critical
}

// QuotaItemStat describes a single quota resource.
type QuotaItemStat struct {
	Resource   string  `json:"resource"`
	Hard       string  `json:"hard"`
	Used       string  `json:"used"`
	Saturation float64 `json:"saturation"` // used/hard %
}

// QuotaSatRisk describes a quota saturation risk.
type QuotaSatRisk struct {
	Namespace string `json:"namespace"`
	Resource  string `json:"resource"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// handleQuotaSaturation audits namespace resource quota saturation & limit exhaustion.
// GET /api/scalability/quota-saturation
func (s *Server) handleQuotaSaturation(w http.ResponseWriter, r *http.Request) {
	result := QuotaSaturationResult{
		ScannedAt: time.Now(),
	}

	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	namespaces, err := rc.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list namespaces: %v", err))
		return
	}

	quotas, err := rc.clientset.CoreV1().ResourceQuotas("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list resource quotas: %v", err))
		return
	}

	// Build namespace → quota map
	nsQuotas := map[string][]corev1.ResourceQuota{}
	for _, rq := range quotas.Items {
		nsQuotas[rq.Namespace] = append(nsQuotas[rq.Namespace], rq)
	}

	for _, ns := range namespaces.Items {
		result.Summary.TotalNamespaces++
		entry := QuotaSatNSStat{
			Namespace: ns.Name,
			RiskLevel: "low",
		}

		nsQuotaList := nsQuotas[ns.Name]
		if len(nsQuotaList) == 0 {
			entry.HasQuota = false
			result.Summary.NSWithoutQuota++
			// Don't flag system namespaces
			if ns.Name != "kube-system" && ns.Name != "kube-public" && ns.Name != "kube-node-lease" {
				result.Risks = append(result.Risks, QuotaSatRisk{
					Namespace: ns.Name,
					Issue:     fmt.Sprintf("Namespace %s has no ResourceQuota — uncontrolled resource consumption", ns.Name),
					Severity:  "warning",
				})
			}
		} else {
			entry.HasQuota = true
			result.Summary.NSWithQuota++

			// Aggregate all hard/used from all quotas in this namespace
			hardMap := map[corev1.ResourceName]resource.Quantity{}
			usedMap := map[corev1.ResourceName]resource.Quantity{}
			for _, rq := range nsQuotaList {
				for k, v := range rq.Status.Hard {
					existing := hardMap[k]
					existing.Add(v)
					hardMap[k] = existing
				}
				for k, v := range rq.Status.Used {
					existing := usedMap[k]
					existing.Add(v)
					usedMap[k] = existing
				}
			}

			for res, hard := range hardMap {
				used := usedMap[res]
				saturation := 0.0
				if hard.MilliValue() > 0 {
					saturation = float64(used.MilliValue()) / float64(hard.MilliValue()) * 100
				} else if hard.Value() > 0 {
					saturation = float64(used.Value()) / float64(hard.Value()) * 100
				}

				item := QuotaItemStat{
					Resource:   string(res),
					Hard:       hard.String(),
					Used:       used.String(),
					Saturation: saturation,
				}
				entry.QuotaItems = append(entry.QuotaItems, item)

				if saturation > entry.MaxSaturation {
					entry.MaxSaturation = saturation
				}

				// Classify
				if saturation >= 100 {
					result.Summary.ExhaustedQuotas++
					result.Risks = append(result.Risks, QuotaSatRisk{
						Namespace: ns.Name,
						Resource:  string(res),
						Issue:     fmt.Sprintf("Namespace %s has exhausted %s quota (100%% used)", ns.Name, res),
						Severity:  "critical",
					})
				} else if saturation >= 90 {
					result.Summary.CriticalSaturation++
					result.Risks = append(result.Risks, QuotaSatRisk{
						Namespace: ns.Name,
						Resource:  string(res),
						Issue:     fmt.Sprintf("Namespace %s is at %.1f%% of %s quota — near exhaustion", ns.Name, saturation, res),
						Severity:  "high",
					})
				} else if saturation >= 70 {
					result.Summary.HighSaturation++
				} else if saturation >= 50 {
					result.Summary.MediumSaturation++
				} else {
					result.Summary.LowSaturation++
				}
			}

			// Risk level
			if entry.MaxSaturation >= 100 {
				entry.RiskLevel = "critical"
				result.CriticalNS = append(result.CriticalNS, entry)
			} else if entry.MaxSaturation >= 90 {
				entry.RiskLevel = "high"
				result.CriticalNS = append(result.CriticalNS, entry)
			} else if entry.MaxSaturation >= 70 {
				entry.RiskLevel = "medium"
			}
		}

		result.ByNamespace = append(result.ByNamespace, entry)
	}

	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].MaxSaturation > result.ByNamespace[j].MaxSaturation
	})

	// Health score
	score := 100
	if result.Summary.ExhaustedQuotas > 0 {
		score -= min(30, result.Summary.ExhaustedQuotas*10)
	}
	if result.Summary.CriticalSaturation > 0 {
		score -= min(20, result.Summary.CriticalSaturation*5)
	}
	if result.Summary.NSWithoutQuota > 3 {
		score -= min(15, (result.Summary.NSWithoutQuota-3)*2)
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	// Recommendations
	if result.Summary.ExhaustedQuotas > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d quota(s) are exhausted (100%%) — increase quota limits or reduce workload", result.Summary.ExhaustedQuotas))
	}
	if result.Summary.CriticalSaturation > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d quota(s) are near exhaustion (>90%%) — plan capacity increase", result.Summary.CriticalSaturation))
	}
	if result.Summary.NSWithoutQuota > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d namespace(s) have no ResourceQuota — add quotas for resource control", result.Summary.NSWithoutQuota))
	}
	if len(result.Recommendations) == 0 {
		result.Recommendations = append(result.Recommendations,
			"Quota utilization is healthy — no saturation or exhaustion detected")
	}

	writeJSON(w, result)
}
