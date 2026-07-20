package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// IngressConflictResult detects ingress rule conflicts, overlapping paths and TLS issues.
type IngressConflictResult struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	Summary         IngressConflictSummary `json:"summary"`
	Conflicts       []IngressConflictEntry `json:"conflicts"`
	ByIngress       []IngressHealthEntry   `json:"byIngress"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Recommendations []string               `json:"recommendations"`
}

type IngressConflictSummary struct {
	TotalIngresses int `json:"totalIngresses"`
	WithTLS        int `json:"withTLS"`
	PathConflicts  int `json:"pathConflicts"`
	HostConflicts  int `json:"hostConflicts"`
	NoBackend      int `json:"noBackendService"`
	StaleRules     int `json:"staleRules"`
}

type IngressConflictEntry struct {
	Ingress1     string `json:"ingress1"`
	Ingress2     string `json:"ingress2"`
	Namespace    string `json:"namespace"`
	Host         string `json:"host"`
	Path         string `json:"path"`
	ConflictType string `json:"conflictType"`
}

type IngressHealthEntry struct {
	Name       string   `json:"name"`
	Namespace  string   `json:"namespace"`
	RuleCount  int      `json:"ruleCount"`
	HasTLS     bool     `json:"hasTLS"`
	BackendSvc string   `json:"backendService"`
	Issues     []string `json:"issues"`
	RiskLevel  string   `json:"riskLevel"`
}

// handleIngressConflict handles GET /api/product/ingress-conflict
func (s *Server) handleIngressConflict(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := IngressConflictResult{ScannedAt: time.Now()}

	ingresses, _ := rc.clientset.NetworkingV1().Ingresses("").List(ctx, metav1.ListOptions{})
	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})

	// Build service lookup
	svcSet := make(map[string]bool)
	for _, svc := range services.Items {
		svcSet[svc.Namespace+"/"+svc.Name] = true
	}

	// Path-host map for conflict detection: host+path -> []ingressName
	pathMap := make(map[string][]string)

	for _, ing := range ingresses.Items {
		if isSystemNamespace(ing.Namespace) {
			continue
		}
		result.Summary.TotalIngresses++
		entry := IngressHealthEntry{
			Name:      ing.Name,
			Namespace: ing.Namespace,
		}

		// Check TLS
		if len(ing.Spec.TLS) > 0 {
			entry.HasTLS = true
			result.Summary.WithTLS++
		}

		// Check rules
		for _, rule := range ing.Spec.Rules {
			host := rule.Host
			if host == "" {
				host = "*"
			}
			if rule.HTTP != nil {
				for _, path := range rule.HTTP.Paths {
					entry.RuleCount++
					fullPath := host + path.Path
					backendName := path.Backend.Service.Name
					if entry.BackendSvc == "" {
						entry.BackendSvc = backendName
					}

					// Check path conflicts
					pathMap[fullPath] = append(pathMap[fullPath], ing.Namespace+"/"+ing.Name)

					// Check backend service exists
					if backendName != "" && !svcSet[ing.Namespace+"/"+backendName] {
						entry.Issues = append(entry.Issues, fmt.Sprintf("backend %s not found", backendName))
						result.Summary.NoBackend++
					}
				}
			}
		}

		// Check for stale rules (empty rules)
		if entry.RuleCount == 0 {
			entry.Issues = append(entry.Issues, "no rules defined")
			result.Summary.StaleRules++
		}

		// Risk assessment
		switch {
		case len(entry.Issues) >= 2:
			entry.RiskLevel = "high"
		case len(entry.Issues) >= 1:
			entry.RiskLevel = "medium"
		default:
			entry.RiskLevel = "low"
		}

		result.ByIngress = append(result.ByIngress, entry)
	}

	// Detect path conflicts
	for fullPath, ings := range pathMap {
		if len(ings) > 1 {
			host := strings.SplitN(fullPath, "/", 2)[0]
			path := "/" + strings.SplitN(fullPath, "/", 2)[1]
			for i := 1; i < len(ings); i++ {
				result.Conflicts = append(result.Conflicts, IngressConflictEntry{
					Ingress1:     ings[0],
					Ingress2:     ings[i],
					Namespace:    strings.SplitN(ings[0], "/", 2)[0],
					Host:         host,
					Path:         path,
					ConflictType: "path-overlap",
				})
				result.Summary.PathConflicts++
			}
		}
	}

	sort.Slice(result.ByIngress, func(i, j int) bool {
		rank := map[string]int{"high": 0, "medium": 1, "low": 2}
		return rank[result.ByIngress[i].RiskLevel] < rank[result.ByIngress[j].RiskLevel]
	})

	if result.Summary.TotalIngresses > 0 {
		healthy := result.Summary.TotalIngresses - result.Summary.NoBackend - result.Summary.StaleRules
		result.HealthScore = healthy * 100 / result.Summary.TotalIngresses
		result.HealthScore -= result.Summary.PathConflicts * 5
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("Ingress 冲突: %d 总计, %d 有 TLS, %d 路径冲突, %d 无后端, %d 过期规则",
			result.Summary.TotalIngresses, result.Summary.WithTLS,
			result.Summary.PathConflicts, result.Summary.NoBackend, result.Summary.StaleRules),
	}
	if result.Summary.PathConflicts > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个路径冲突, 多个 Ingress 指向相同 host+path", result.Summary.PathConflicts))
	}
	if result.Summary.NoBackend > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个 Ingress 后端 Service 不存在", result.Summary.NoBackend))
	}
	if result.HealthScore < 80 {
		result.Recommendations = append(result.Recommendations, "建议: 修复路径冲突, 清理过期规则, 为所有 Ingress 配置 TLS")
	}
	writeJSON(w, result)
}
