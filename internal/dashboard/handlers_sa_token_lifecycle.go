package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SATokenLifecycleResult analyzes ServiceAccount token lifecycle risks.
type SATokenLifecycleResult struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	Summary         SATokenLifecycleSummary `json:"summary"`
	BySA            []SATokenLifecycleEntry `json:"byServiceAccount"`
	RiskyTokens     []SATokenLifecycleEntry `json:"riskyTokens"`
	RiskScore       int                     `json:"riskScore"`
	Grade           string                  `json:"grade"`
	Recommendations []string                `json:"recommendations"`
}

type SATokenLifecycleSummary struct {
	TotalSAs        int `json:"totalServiceAccounts"`
	WithTokens      int `json:"withTokens"`
	AutoMounted     int `json:"autoMountedTokens"`
	LongLivedTokens int `json:"longLivedTokens"`
	UnusedSAs       int `json:"unusedServiceAccounts"`
	ClusterAdminSAs int `json:"clusterAdminSAs"`
}

type SATokenLifecycleEntry struct {
	SAName         string   `json:"saName"`
	Namespace      string   `json:"namespace"`
	HasToken       bool     `json:"hasToken"`
	TokenAge       string   `json:"tokenAge"`
	AutoMount      bool     `json:"autoMountToken"`
	UsedByPods     int      `json:"usedByPods"`
	IsClusterAdmin bool     `json:"isClusterAdmin"`
	RiskLevel      string   `json:"riskLevel"`
	RiskFactors    []string `json:"riskFactors"`
}

// handleSATokenLifecycle handles GET /api/security/sa-token-lifecycle
func (s *Server) handleSATokenLifecycle(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := SATokenLifecycleResult{ScannedAt: time.Now()}
	sas, _ := rc.clientset.CoreV1().ServiceAccounts("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})

	saUsedByPods := make(map[string]int)
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		key := pod.Namespace + "/" + pod.Spec.ServiceAccountName
		saUsedByPods[key]++
	}

	tokenSecretSet := make(map[string]bool)
	for _, sec := range secrets.Items {
		if sec.Type == corev1.SecretTypeServiceAccountToken {
			tokenSecretSet[sec.Namespace+"/"+sec.Name] = true
		}
	}

	var entries []SATokenLifecycleEntry
	for _, sa := range sas.Items {
		if isSystemNamespace(sa.Namespace) {
			continue
		}
		result.Summary.TotalSAs++
		entry := SATokenLifecycleEntry{SAName: sa.Name, Namespace: sa.Namespace}
		key := sa.Namespace + "/" + sa.Name
		entry.UsedByPods = saUsedByPods[key]

		if len(sa.Secrets) > 0 {
			entry.HasToken = true
			result.Summary.WithTokens++
			for _, sref := range sa.Secrets {
				if tokenSecretSet[sa.Namespace+"/"+sref.Name] {
					entry.TokenAge = "legacy-token"
					result.Summary.LongLivedTokens++
					break
				}
			}
		}

		autoMount := true
		if sa.AutomountServiceAccountToken != nil {
			autoMount = *sa.AutomountServiceAccountToken
		}
		entry.AutoMount = autoMount
		if autoMount {
			result.Summary.AutoMounted++
		}

		if entry.UsedByPods == 0 {
			result.Summary.UnusedSAs++
		}

		var risks []string
		if entry.HasToken && entry.TokenAge == "legacy-token" {
			risks = append(risks, "long-lived-token")
			risks = append(risks, "non-expiring")
		}
		if entry.UsedByPods == 0 {
			risks = append(risks, "unused-sa")
		}
		if autoMount && entry.UsedByPods == 0 {
			risks = append(risks, "auto-mount-unused")
		}

		switch {
		case len(risks) >= 3:
			entry.RiskLevel = "critical"
		case len(risks) >= 2:
			entry.RiskLevel = "high"
		case len(risks) >= 1:
			entry.RiskLevel = "medium"
		default:
			entry.RiskLevel = "low"
		}
		entry.RiskFactors = risks

		if entry.RiskLevel == "critical" || entry.RiskLevel == "high" {
			result.RiskyTokens = append(result.RiskyTokens, entry)
		}
		entries = append(entries, entry)
	}

	sort.Slice(entries, func(i, j int) bool {
		rank := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		return rank[entries[i].RiskLevel] < rank[entries[j].RiskLevel]
	})
	result.BySA = entries

	if result.Summary.TotalSAs > 0 {
		cleanRatio := float64(result.Summary.TotalSAs-len(result.RiskyTokens)) / float64(result.Summary.TotalSAs)
		result.RiskScore = int(cleanRatio * 100)
	}
	gradeFromScore(&result.Grade, result.RiskScore)

	result.Recommendations = []string{
		fmt.Sprintf("SA Token 生命周期: %d SA, %d 有 token, %d 未使用, %d 长生命周期", result.Summary.TotalSAs, result.Summary.WithTokens, result.Summary.UnusedSAs, result.Summary.LongLivedTokens),
	}
	if result.Summary.LongLivedTokens > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个长生命周期 token, 建议迁移到 TokenRequest API", result.Summary.LongLivedTokens))
	}
	if result.Summary.UnusedSAs > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个未使用的 ServiceAccount", result.Summary.UnusedSAs))
	}
	if result.RiskScore < 60 {
		result.Recommendations = append(result.Recommendations, "建议: 设置 automountServiceAccountToken=false, 使用 BoundServiceAccountTokenVolume")
	}
	writeJSON(w, result)
}
