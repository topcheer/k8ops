package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AdmissionBypassAuditResult audits workloads that may bypass admission control
// through privileged specs, host namespace sharing, or direct API access.
type AdmissionBypassAuditResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         BypassAuditSummary `json:"summary"`
	ByNamespace     []BypassNsStat     `json:"byNamespace"`
	Violations      []BypassEntry      `json:"violations"`
	BypassScore     int                `json:"bypassScore"`
	Grade           string             `json:"grade"`
	Recommendations []string           `json:"recommendations"`
}

type BypassAuditSummary struct {
	TotalPods       int `json:"totalPods"`
	BypassPods      int `json:"bypassPods"`
	PrivilegedPods  int `json:"privilegedPods"`
	HostNetworkPods int `json:"hostNetworkPods"`
	HostPIDPods     int `json:"hostPIDPods"`
	HostIPCPods     int `json:"hostIPCPods"`
	HostPathPods    int `json:"hostPathPods"`
	DirectAPITokens int `json:"directAPITokens"`
}

type BypassNsStat struct {
	Namespace   string `json:"namespace"`
	BypassCount int    `json:"bypassCount"`
	MaxSeverity string `json:"maxSeverity"`
}

type BypassEntry struct {
	PodName      string   `json:"podName"`
	Namespace    string   `json:"namespace"`
	Workload     string   `json:"workload"`
	Findings     []string `json:"findings"`
	Severity     string   `json:"severity"`
	FindingCount int      `json:"findingCount"`
}

// handleAdmissionBypassAudit handles GET /api/security/admission-bypass-audit
func (s *Server) handleAdmissionBypassAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := AdmissionBypassAuditResult{ScannedAt: time.Now()}
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	saTokens, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})

	nsStats := make(map[string]*BypassNsStat)
	riskRank := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}

	// Count SA token secrets that could bypass admission
	saTokenCount := 0
	for _, sec := range saTokens.Items {
		if sec.Type == corev1.SecretTypeServiceAccountToken {
			saTokenCount++
		}
	}
	result.Summary.DirectAPITokens = saTokenCount

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		result.Summary.TotalPods++

		wlName := ""
		for _, ref := range pod.OwnerReferences {
			if ref.Controller != nil && *ref.Controller {
				wlName = ref.Name
				break
			}
		}

		var findings []string
		severity := "low"

		// Host namespace checks
		if pod.Spec.HostPID {
			findings = append(findings, "hostPID=true")
			result.Summary.HostPIDPods++
			if riskRank[severity] > riskRank["high"] {
				severity = "high"
			}
		}
		if pod.Spec.HostIPC {
			findings = append(findings, "hostIPC=true")
			result.Summary.HostIPCPods++
			if riskRank[severity] > riskRank["high"] {
				severity = "high"
			}
		}
		if pod.Spec.HostNetwork {
			findings = append(findings, "hostNetwork=true")
			result.Summary.HostNetworkPods++
			if riskRank[severity] > riskRank["medium"] {
				severity = "medium"
			}
		}

		// HostPath volumes
		for _, vol := range pod.Spec.Volumes {
			if vol.HostPath != nil {
				findings = append(findings, fmt.Sprintf("hostPath=%s", vol.Name))
				result.Summary.HostPathPods++
				if riskRank[severity] > riskRank["high"] {
					severity = "high"
				}
				break
			}
		}

		// Privileged containers
		for _, c := range pod.Spec.Containers {
			if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
				findings = append(findings, fmt.Sprintf("privileged container: %s", c.Name))
				result.Summary.PrivilegedPods++
				severity = "critical"
			}
		}

		// ServiceAccount with cluster-admin
		if pod.Spec.ServiceAccountName == "cluster-admin" || pod.Spec.ServiceAccountName == "default" {
			if pod.Spec.ServiceAccountName == "default" {
				findings = append(findings, "uses default service account")
				if riskRank[severity] > riskRank["medium"] {
					severity = "medium"
				}
			}
		}

		if len(findings) > 0 {
			result.Summary.BypassPods++
			entry := BypassEntry{
				PodName: pod.Name, Namespace: pod.Namespace, Workload: wlName,
				Findings: findings, Severity: severity, FindingCount: len(findings),
			}
			result.Violations = append(result.Violations, entry)

			if _, ok := nsStats[pod.Namespace]; !ok {
				nsStats[pod.Namespace] = &BypassNsStat{Namespace: pod.Namespace}
			}
			ns := nsStats[pod.Namespace]
			ns.BypassCount++
			if riskRank[severity] < riskRank[ns.MaxSeverity] || ns.MaxSeverity == "" {
				ns.MaxSeverity = severity
			}
		}
	}

	// Sort violations by severity
	sort.Slice(result.Violations, func(i, j int) bool {
		return riskRank[result.Violations[i].Severity] < riskRank[result.Violations[j].Severity]
	})

	for _, ns := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].BypassCount > result.ByNamespace[j].BypassCount
	})

	if result.Summary.TotalPods > 0 {
		cleanRatio := float64(result.Summary.TotalPods-result.Summary.BypassPods) / float64(result.Summary.TotalPods)
		result.BypassScore = int(cleanRatio * 100)
	}
	gradeFromScore(&result.Grade, result.BypassScore)

	result.Recommendations = buildBypassRecs(&result)
	writeJSON(w, result)
}

func buildBypassRecs(r *AdmissionBypassAuditResult) []string {
	recs := []string{
		fmt.Sprintf("准入绕过审计: %d/%d Pod 存在绕过风险 (%d 特权, %d hostNetwork, %d hostPath)",
			r.Summary.BypassPods, r.Summary.TotalPods, r.Summary.PrivilegedPods, r.Summary.HostNetworkPods, r.Summary.HostPathPods),
	}
	if r.Summary.PrivilegedPods > 0 {
		recs = append(recs, fmt.Sprintf("紧急: %d 个特权容器可能绕过准入控制", r.Summary.PrivilegedPods))
	}
	if r.Summary.HostPIDPods+r.Summary.HostIPCPods > 0 {
		recs = append(recs, fmt.Sprintf("警告: %d 个 Pod 使用 hostPID/hostIPC", r.Summary.HostPIDPods+r.Summary.HostIPCPods))
	}
	if r.Summary.DirectAPITokens > 10 {
		recs = append(recs, fmt.Sprintf("%d 个 SA token secret, 审查是否有过度权限", r.Summary.DirectAPITokens))
	}
	if r.BypassScore < 80 {
		recs = append(recs, "建议: 启用 Pod Security Admission (PSA) enforce=restricted, 部署 OPA Gatekeeper")
	}
	return recs
}
