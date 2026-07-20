package dashboard

import (
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// CoreDNSConfigAuditResult audits CoreDNS configuration health and performance.
type CoreDNSConfigAuditResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Summary         CoreDNSConfigSummary `json:"summary"`
	Issues          []CoreDNSConfigIssue `json:"issues"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Recommendations []string             `json:"recommendations"`
}

type CoreDNSConfigSummary struct {
	CoreDNSPodCount     int  `json:"corednsPodCount"`
	Ready               bool `json:"allReady"`
	MemoryLimitSet      bool `json:"memoryLimitSet"`
	HasCustomConfig     bool `json:"hasCustomConfig"`
	NodeLocalDNSEnabled bool `json:"nodeLocalDNSEnabled"`
	StubDomains         int  `json:"stubDomains"`
	UpstreamResolvers   int  `json:"upstreamResolvers"`
}

type CoreDNSConfigIssue struct {
	Severity string `json:"severity"`
	Area     string `json:"area"`
	Detail   string `json:"detail"`
}

// handleCoreDNSConfigAudit handles GET /api/operations/coredns-config-audit
func (s *Server) handleCoreDNSConfigAudit(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	ctx := r.Context()
	result := CoreDNSConfigAuditResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	cm, _ := rc.clientset.CoreV1().ConfigMaps("kube-system").Get(ctx, "coredns", metav1.GetOptions{})

	// Count CoreDNS pods
	for _, pod := range pods.Items {
		if pod.Namespace != "kube-system" {
			continue
		}
		if containsStr1876(pod.Labels["k8s-app"], "kube-dns") || containsStr1876(pod.Name, "coredns") {
			result.Summary.CoreDNSPodCount++
			ready := true
			for _, cs := range pod.Status.ContainerStatuses {
				if !cs.Ready {
					ready = false
				}
			}
			if !ready {
				result.Issues = append(result.Issues, CoreDNSConfigIssue{
					Severity: "critical", Area: "pod-health",
					Detail: fmt.Sprintf("CoreDNS pod %s not ready", pod.Name),
				})
			}
			// Check memory limit
			for _, c := range pod.Spec.Containers {
				if c.Name == "coredns" {
					if _, ok := c.Resources.Limits[corev1.ResourceMemory]; ok {
						result.Summary.MemoryLimitSet = true
					} else {
						result.Issues = append(result.Issues, CoreDNSConfigIssue{
							Severity: "high", Area: "resource-limits",
							Detail: "CoreDNS has no memory limit - OOM can crash node",
						})
					}
				}
			}
		}
	}

	result.Summary.Ready = result.Summary.CoreDNSPodCount > 0 && len(result.Issues) == 0

	// Analyze CoreDNS ConfigMap
	if cm != nil {
		corefile := cm.Data["Corefile"]
		if corefile != "" {
			result.Summary.HasCustomConfig = true

			// Check for stub domains
			result.Summary.StubDomains = countLines1876(corefile, ".:")
			result.Summary.UpstreamResolvers = countLines1876(corefile, "forward")

			// Check for caching
			if !containsStr1876(corefile, "cache") {
				result.Issues = append(result.Issues, CoreDNSConfigIssue{
					Severity: "medium", Area: "performance",
					Detail: "No cache plugin in CoreDNS Corefile - DNS queries not cached",
				})
			}

			// Check for ready plugin
			if !containsStr1876(corefile, "ready") {
				result.Issues = append(result.Issues, CoreDNSConfigIssue{
					Severity: "low", Area: "monitoring",
					Detail: "No ready plugin - health check endpoint may be missing",
				})
			}

			// Check for prometheus plugin
			if !containsStr1876(corefile, "prometheus") {
				result.Issues = append(result.Issues, CoreDNSConfigIssue{
					Severity: "medium", Area: "monitoring",
					Detail: "No prometheus plugin - DNS metrics not exported",
				})
			}
		}
	}

	// Check for NodeLocalDNS
	for _, pod := range pods.Items {
		if containsStr1876(pod.Name, "node-local-dns") {
			result.Summary.NodeLocalDNSEnabled = true
			break
		}
	}
	if !result.Summary.NodeLocalDNSEnabled {
		result.Issues = append(result.Issues, CoreDNSConfigIssue{
			Severity: "low", Area: "performance",
			Detail: "NodeLocalDNS not deployed - every DNS query hits CoreDNS",
		})
	}

	// Score
	result.HealthScore = 100
	for _, issue := range result.Issues {
		switch issue.Severity {
		case "critical":
			result.HealthScore -= 25
		case "high":
			result.HealthScore -= 15
		case "medium":
			result.HealthScore -= 8
		case "low":
			result.HealthScore -= 3
		}
	}
	if result.HealthScore < 0 {
		result.HealthScore = 0
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("CoreDNS 配置审计: %d pods, 就绪: %v, 内存限制: %v, 自定义配置: %v, NodeLocalDNS: %v",
			result.Summary.CoreDNSPodCount, result.Summary.Ready,
			result.Summary.MemoryLimitSet, result.Summary.HasCustomConfig,
			result.Summary.NodeLocalDNSEnabled),
	}
	if len(result.Issues) > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("发现 %d 个配置问题", len(result.Issues)))
	}
	writeJSON(w, result)
}

func containsStr1876(s, sub string) bool {
	return len(s) > 0 && len(sub) > 0 && indexOf1876(s, sub) >= 0
}

func indexOf1876(s, sub string) int {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func countLines1876(s, sub string) int {
	count := 0
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			count++
			i += len(sub)
		}
	}
	return count
}
