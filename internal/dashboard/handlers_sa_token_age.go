package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	authv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SATokenAgeResult audits service account token ages and rotation status.
type SATokenAgeResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         SATokenAgeSummary   `json:"summary"`
	ByNamespace     []SATokenAgeNSEntry `json:"byNamespace"`
	OldTokens       []SATokenAgeEntry   `json:"oldTokens"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

type SATokenAgeSummary struct {
	TotalSAs      int     `json:"totalServiceAccounts"`
	WithAutoMount int     `json:"automountEnabled"`
	NoAutoMount   int     `json:"automountDisabled"`
	OldSAs        int     `json:"oldServiceAccounts"` // > 90 days
	UnusedSAs     int     `json:"unusedServiceAccounts"`
	AvgAgeDays    float64 `json:"avgAgeDays"`
}

type SATokenAgeNSEntry struct {
	Namespace   string `json:"namespace"`
	SACount     int    `json:"saCount"`
	OldCount    int    `json:"oldCount"`
	UnusedCount int    `json:"unusedCount"`
}

type SATokenAgeEntry struct {
	Name       string  `json:"name"`
	Namespace  string  `json:"namespace"`
	AgeDays    float64 `json:"ageDays"`
	AutoMount  bool    `json:"automountEnabled"`
	UsedByPods int     `json:"usedByPods"`
	RiskLevel  string  `json:"riskLevel"`
}

// handleSATokenAge handles GET /api/security/sa-token-age
func (s *Server) handleSATokenAge(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := SATokenAgeResult{ScannedAt: time.Now()}

	sas, _ := rc.clientset.CoreV1().ServiceAccounts("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Build SA usage map
	saUsageMap := make(map[string]int) // ns/sa -> pod count
	for _, pod := range pods.Items {
		if pod.Spec.ServiceAccountName != "" && pod.Spec.ServiceAccountName != "default" {
			key := pod.Namespace + "/" + pod.Spec.ServiceAccountName
			saUsageMap[key]++
		}
	}

	nsMap := make(map[string]*SATokenAgeNSEntry)
	totalAge := 0.0
	saCount := 0

	for _, sa := range sas.Items {
		if isSystemNamespace(sa.Namespace) || sa.Name == "default" {
			continue
		}
		result.Summary.TotalSAs++

		ageDays := time.Since(sa.CreationTimestamp.Time).Hours() / 24
		totalAge += ageDays
		saCount++

		autoMount := true
		if sa.AutomountServiceAccountToken != nil {
			autoMount = *sa.AutomountServiceAccountToken
		}
		if autoMount {
			result.Summary.WithAutoMount++
		} else {
			result.Summary.NoAutoMount++
		}

		key := sa.Namespace + "/" + sa.Name
		usedBy := saUsageMap[key]

		entry := SATokenAgeEntry{
			Name: sa.Name, Namespace: sa.Namespace,
			AgeDays: ageDays, AutoMount: autoMount, UsedByPods: usedBy,
		}

		if nsMap[sa.Namespace] == nil {
			nsMap[sa.Namespace] = &SATokenAgeNSEntry{Namespace: sa.Namespace}
		}
		nsMap[sa.Namespace].SACount++

		switch {
		case ageDays > 365 && usedBy == 0:
			entry.RiskLevel = "high"
			result.Summary.UnusedSAs++
			result.Summary.OldSAs++
			result.OldTokens = append(result.OldTokens, entry)
			nsMap[sa.Namespace].OldCount++
			nsMap[sa.Namespace].UnusedCount++
		case ageDays > 90 && usedBy == 0:
			entry.RiskLevel = "medium"
			result.Summary.UnusedSAs++
			result.OldTokens = append(result.OldTokens, entry)
			nsMap[sa.Namespace].UnusedCount++
		case ageDays > 365:
			entry.RiskLevel = "medium"
			result.Summary.OldSAs++
			result.OldTokens = append(result.OldTokens, entry)
			nsMap[sa.Namespace].OldCount++
		default:
			entry.RiskLevel = "low"
		}
	}

	for _, e := range nsMap {
		result.ByNamespace = append(result.ByNamespace, *e)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].UnusedCount > result.ByNamespace[j].UnusedCount
	})
	sort.Slice(result.OldTokens, func(i, j int) bool {
		return result.OldTokens[i].AgeDays > result.OldTokens[j].AgeDays
	})

	if saCount > 0 {
		result.Summary.AvgAgeDays = totalAge / float64(saCount)
	}

	if result.Summary.TotalSAs > 0 {
		result.HealthScore = (result.Summary.TotalSAs - result.Summary.UnusedSAs) * 100 / result.Summary.TotalSAs
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("SA Token 审计: %d SA, %d 自动挂载, %d 老旧(>90d), %d 未使用, 平均 %.0f 天",
			result.Summary.TotalSAs, result.Summary.WithAutoMount,
			result.Summary.OldSAs, result.Summary.UnusedSAs, result.Summary.AvgAgeDays),
	}
	if result.Summary.UnusedSAs > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个未使用的 ServiceAccount, 建议清理", result.Summary.UnusedSAs))
	}
	writeJSON(w, result)

	_ = authv1.TokenRequest{}
}
