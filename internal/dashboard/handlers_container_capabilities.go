package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ContainerCapabilitiesResult audits Linux capabilities granted to containers.
type ContainerCapabilitiesResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	Summary         CapAuditSummary   `json:"summary"`
	ByNamespace     []CapAuditNSEntry `json:"byNamespace"`
	RiskyContainers []CapAuditEntry   `json:"riskyContainers"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Recommendations []string          `json:"recommendations"`
}

type CapAuditSummary struct {
	TotalContainers int `json:"totalContainers"`
	WithCapDrop     int `json:"withCapDrop"`
	WithDropAll     int `json:"withDropAll"`
	WithCapAdd      int `json:"withCapAdd"`
	Privileged      int `json:"privilegedContainers"`
	DangerousCaps   int `json:"containersWithDangerousCaps"`
}

type CapAuditNSEntry struct {
	Namespace      string `json:"namespace"`
	ContainerCount int    `json:"containerCount"`
	RiskyCount     int    `json:"riskyCount"`
	RiskLevel      string `json:"riskLevel"`
}

type CapAuditEntry struct {
	PodName       string   `json:"podName"`
	Namespace     string   `json:"namespace"`
	Container     string   `json:"container"`
	AddedCaps     []string `json:"addedCaps"`
	DroppedCaps   []string `json:"droppedCaps"`
	IsPrivileged  bool     `json:"isPrivileged"`
	DangerousCaps []string `json:"dangerousCaps"`
	RiskLevel     string   `json:"riskLevel"`
}

// Capabilities that grant significant host-level access
var dangerousCaps1880 = map[string]bool{
	"SYS_ADMIN": true, "NET_ADMIN": true, "SYS_PTRACE": true,
	"SYS_MODULE": true, "DAC_READ_SEARCH": true, "DAC_OVERRIDE": true,
	"SETUID": true, "SETGID": true, "CHOWN": true, "FOWNER": true,
	"NET_RAW": true, "NET_BIND_SERVICE": true,
}

// handleContainerCapabilities handles GET /api/security/container-capabilities
func (s *Server) handleContainerCapabilities(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := ContainerCapabilitiesResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	nsMap := make(map[string]*CapAuditNSEntry)

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			entry := CapAuditEntry{
				PodName:   pod.Name,
				Namespace: pod.Namespace,
				Container: c.Name,
			}

			sc := c.SecurityContext
			if sc != nil {
				if sc.Privileged != nil && *sc.Privileged {
					entry.IsPrivileged = true
					result.Summary.Privileged++
				}
				if sc.Capabilities != nil {
					entry.AddedCaps = capsToStrings(sc.Capabilities.Add)
					entry.DroppedCaps = capsToStrings(sc.Capabilities.Drop)
					if len(sc.Capabilities.Drop) > 0 {
						result.Summary.WithCapDrop++
						for _, cap := range sc.Capabilities.Drop {
							if cap == "ALL" {
								result.Summary.WithDropAll++
							}
						}
					}
					if len(sc.Capabilities.Add) > 0 {
						result.Summary.WithCapAdd++
						for _, cap := range sc.Capabilities.Add {
							if dangerousCaps1880[string(cap)] {
								entry.DangerousCaps = append(entry.DangerousCaps, string(cap))
							}
						}
					}
				}
			}

			// Determine risk
			hasDangerous := len(entry.DangerousCaps) > 0
			switch {
			case entry.IsPrivileged:
				entry.RiskLevel = "critical"
			case hasDangerous && len(entry.DangerousCaps) >= 3:
				entry.RiskLevel = "critical"
			case hasDangerous:
				entry.RiskLevel = "high"
			case len(entry.DroppedCaps) == 0:
				entry.RiskLevel = "medium"
			default:
				entry.RiskLevel = "low"
			}

			if entry.RiskLevel != "low" {
				result.RiskyContainers = append(result.RiskyContainers, entry)
				result.Summary.DangerousCaps += len(entry.DangerousCaps)
				if nsMap[pod.Namespace] == nil {
					nsMap[pod.Namespace] = &CapAuditNSEntry{Namespace: pod.Namespace}
				}
				nsMap[pod.Namespace].ContainerCount++
				nsMap[pod.Namespace].RiskyCount++
			}
		}
	}

	for _, e := range nsMap {
		switch {
		case e.RiskyCount > 10:
			e.RiskLevel = "critical"
		case e.RiskyCount > 5:
			e.RiskLevel = "high"
		default:
			e.RiskLevel = "medium"
		}
		result.ByNamespace = append(result.ByNamespace, *e)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].RiskyCount > result.ByNamespace[j].RiskyCount
	})
	sort.Slice(result.RiskyContainers, func(i, j int) bool {
		rank := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		return rank[result.RiskyContainers[i].RiskLevel] < rank[result.RiskyContainers[j].RiskLevel]
	})

	if result.Summary.TotalContainers > 0 {
		result.HealthScore = result.Summary.WithCapDrop * 100 / result.Summary.TotalContainers
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("容器能力审计: %d 容器, %d drop cap, %d drop ALL, %d add cap, %d privileged",
			result.Summary.TotalContainers, result.Summary.WithCapDrop,
			result.Summary.WithDropAll, result.Summary.WithCapAdd, result.Summary.Privileged),
	}
	if result.Summary.Privileged > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个特权容器, 完全绕过安全隔离", result.Summary.Privileged))
	}
	if result.HealthScore < 50 {
		result.Recommendations = append(result.Recommendations, "建议: 添加 securityContext.capabilities.drop: [ALL], 仅按需 add 必要能力")
	}
	writeJSON(w, result)
}

func capsToStrings(caps []corev1.Capability) []string {
	if caps == nil {
		return nil
	}
	result := make([]string, len(caps))
	for i, c := range caps {
		result[i] = string(c)
	}
	return result
}
