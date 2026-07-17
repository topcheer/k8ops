package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResourceGovResult analyzes namespace resource governance: quota effectiveness,
// limit range coverage, and resource consumption policy enforcement.
type ResourceGovResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         ResourceGovSummary `json:"summary"`
	UngovernedNS    []UngovernedNS     `json:"ungovernedNamespaces"`
	QuotaAnalysis   []QuotaAnalysis    `json:"quotaAnalysis"`
	ByNamespace     []ResourceGovNS    `json:"byNamespace"`
	GovernanceScore int                `json:"governanceScore"`
	Grade           string             `json:"grade"`
	Recommendations []string           `json:"recommendations"`
}

type ResourceGovSummary struct {
	TotalNamespaces     int `json:"totalNamespaces"`
	NSWithQuota         int `json:"nsWithQuota"`
	NSWithoutQuota      int `json:"nsWithoutQuota"`
	NSWithLimitRange    int `json:"nsWithLimitRange"`
	NSWithoutLimitRange int `json:"nsWithoutLimitRange"`
	OverQuotaNS         int `json:"overQuotaNS"`
	NearQuotaNS         int `json:"nearQuotaNS"`
	TotalQuotas         int `json:"totalQuotas"`
	TotalLimitRanges    int `json:"totalLimitRanges"`
}

type UngovernedNS struct {
	Namespace string   `json:"namespace"`
	PodCount  int      `json:"podCount"`
	Missing   []string `json:"missing"`
	Severity  string   `json:"severity"`
	Impact    string   `json:"impact"`
}

type QuotaAnalysis struct {
	Namespace string  `json:"namespace"`
	QuotaName string  `json:"quotaName"`
	Resource  string  `json:"resource"`
	Hard      string  `json:"hard"`
	Used      string  `json:"used"`
	UsagePct  float64 `json:"usagePct"`
	Status    string  `json:"status"`
}

type ResourceGovNS struct {
	Namespace     string `json:"namespace"`
	HasQuota      bool   `json:"hasQuota"`
	HasLimitRange bool   `json:"hasLimitRange"`
	PodCount      int    `json:"podCount"`
	GovScore      int    `json:"govScore"`
	Status        string `json:"status"`
}

// handleResourceGovernance provides namespace resource governance analysis.
// GET /api/deployment/resource-governance
func (s *Server) handleResourceGovernance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ResourceGovResult{ScannedAt: time.Now()}
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

	nsList, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	quotas, _ := rc.clientset.CoreV1().ResourceQuotas("").List(ctx, metav1.ListOptions{})
	limitRanges, _ := rc.clientset.CoreV1().LimitRanges("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Build maps
	nsPodCount := make(map[string]int)
	for _, pod := range pods.Items {
		if !systemNS[pod.Namespace] {
			nsPodCount[pod.Namespace]++
		}
	}
	nsHasQuota := make(map[string]bool)
	nsHasLimitRange := make(map[string]bool)

	for _, q := range quotas.Items {
		if !systemNS[q.Namespace] {
			nsHasQuota[q.Namespace] = true
			result.Summary.TotalQuotas++
		}
	}
	for _, lr := range limitRanges.Items {
		if !systemNS[lr.Namespace] {
			nsHasLimitRange[lr.Namespace] = true
			result.Summary.TotalLimitRanges++
		}
	}

	// Analyze each namespace
	for _, ns := range nsList.Items {
		if systemNS[ns.Name] {
			continue
		}
		result.Summary.TotalNamespaces++

		hasQ := nsHasQuota[ns.Name]
		hasLR := nsHasLimitRange[ns.Name]
		podCount := nsPodCount[ns.Name]

		if hasQ {
			result.Summary.NSWithQuota++
		} else {
			result.Summary.NSWithoutQuota++
		}
		if hasLR {
			result.Summary.NSWithLimitRange++
		} else {
			result.Summary.NSWithoutLimitRange++
		}

		govScore := 0
		var missing []string
		if hasQ {
			govScore += 50
		} else {
			missing = append(missing, "resource-quota")
		}
		if hasLR {
			govScore += 30
		} else {
			missing = append(missing, "limit-range")
		}
		if podCount > 0 && hasQ {
			govScore += 20 // active namespace with quota
		}

		status := "governed"
		if len(missing) > 0 {
			status = "ungoverned"
			severity := "medium"
			impact := fmt.Sprintf("Missing: %s", strings.Join(missing, ", "))
			if podCount > 5 {
				severity = "high"
				impact = fmt.Sprintf("%d pods with missing: %s — no resource boundaries", podCount, strings.Join(missing, ", "))
			}
			result.UngovernedNS = append(result.UngovernedNS, UngovernedNS{
				Namespace: ns.Name, PodCount: podCount, Missing: missing,
				Severity: severity, Impact: impact,
			})
		}

		result.ByNamespace = append(result.ByNamespace, ResourceGovNS{
			Namespace: ns.Name, HasQuota: hasQ, HasLimitRange: hasLR,
			PodCount: podCount, GovScore: govScore, Status: status,
		})
	}

	// Analyze quota usage
	for _, q := range quotas.Items {
		if systemNS[q.Namespace] {
			continue
		}
		for hardName, hardVal := range q.Status.Hard {
			used, hasUsed := q.Status.Used[hardName]
			if !hasUsed {
				continue
			}
			hardNum := float64(hardVal.MilliValue())
			usedNum := float64(used.MilliValue())
			usagePct := 0.0
			if hardNum > 0 {
				usagePct = usedNum / hardNum * 100
			}
			status := "healthy"
			if usagePct > 90 {
				status = "critical"
				result.Summary.OverQuotaNS++
			} else if usagePct > 75 {
				status = "warning"
				result.Summary.NearQuotaNS++
			}
			result.QuotaAnalysis = append(result.QuotaAnalysis, QuotaAnalysis{
				Namespace: q.Namespace, QuotaName: q.Name,
				Resource: string(hardName), Hard: hardVal.String(),
				Used: used.String(), UsagePct: usagePct, Status: status,
			})
		}
	}

	// Sort
	sort.Slice(result.UngovernedNS, func(i, j int) bool {
		return result.UngovernedNS[i].PodCount > result.UngovernedNS[j].PodCount
	})
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].GovScore < result.ByNamespace[j].GovScore
	})

	// Score
	if result.Summary.TotalNamespaces > 0 {
		score := result.Summary.NSWithQuota * 50 / result.Summary.TotalNamespaces
		score += result.Summary.NSWithLimitRange * 30 / result.Summary.TotalNamespaces
		score += (result.Summary.TotalNamespaces - result.Summary.OverQuotaNS) * 20 / result.Summary.TotalNamespaces
		result.GovernanceScore = score
	}
	result.Grade = goldenScoreToGrade(result.GovernanceScore)

	// Recommendations
	var recs []string
	recs = append(recs, fmt.Sprintf("Resource governance: %d/100 (grade %s)", result.GovernanceScore, result.Grade))
	if result.Summary.NSWithoutQuota > 0 {
		recs = append(recs, fmt.Sprintf("%d namespaces lack resource quotas — pods can consume unlimited resources", result.Summary.NSWithoutQuota))
	}
	if result.Summary.NSWithoutLimitRange > 0 {
		recs = append(recs, fmt.Sprintf("%d namespaces lack limit ranges — pods without explicit limits get no defaults", result.Summary.NSWithoutLimitRange))
	}
	if result.Summary.OverQuotaNS > 0 {
		recs = append(recs, fmt.Sprintf("%d namespaces are over quota (>90%%) — pods may fail to schedule", result.Summary.OverQuotaNS))
	}
	if len(result.UngovernedNS) > 0 {
		top := result.UngovernedNS[0]
		recs = append(recs, fmt.Sprintf("Highest risk: '%s' has %d pods with no governance (%s)", top.Namespace, top.PodCount, strings.Join(top.Missing, ", ")))
	}
	if len(recs) == 1 {
		recs = append(recs, "Resource governance is comprehensive — maintain current quota and limit range policies")
	}
	result.Recommendations = recs

	writeJSON(w, result)
}
