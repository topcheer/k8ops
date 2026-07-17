package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PrivilegeMapResult builds a cluster-wide privilege exposure map identifying
// containers running with elevated permissions: privileged, root UID,
// hostPID/hostIPC/hostNetwork, dangerous capabilities, and writable root FS.
type PrivilegeMapResult struct {
	ScannedAt        time.Time         `json:"scannedAt"`
	Summary          PrivilegeSummary  `json:"summary"`
	ExposedWorkloads []PrivilegeEntry  `json:"exposedWorkloads"`
	ByNamespace      []PrivilegeNsStat `json:"byNamespace"`
	ExposureScore    int               `json:"exposureScore"`
	Grade            string            `json:"grade"`
	Recommendations  []string          `json:"recommendations"`
}

type PrivilegeSummary struct {
	TotalContainers     int `json:"totalContainers"`
	Privileged          int `json:"privileged"`
	RunAsRoot           int `json:"runAsRoot"`
	HostPID             int `json:"hostPID"`
	HostIPC             int `json:"hostIPC"`
	HostNetwork         int `json:"hostNetwork"`
	DangerousCaps       int `json:"dangerousCapabilities"`
	NoReadOnlyRootFS    int `json:"noReadOnlyRootFS"`
	AllowPrivEscalation int `json:"allowPrivilegeEscalation"`
}

type PrivilegeEntry struct {
	Workload     string   `json:"workload"`
	Namespace    string   `json:"namespace"`
	Container    string   `json:"container"`
	Findings     []string `json:"findings"`
	RiskLevel    string   `json:"riskLevel"`
	FindingCount int      `json:"findingCount"`
}

type PrivilegeNsStat struct {
	Namespace    string `json:"namespace"`
	ExposedCount int    `json:"exposedCount"`
	MaxRiskLevel string `json:"maxRiskLevel"`
}

// privilegeDangerousCaps lists Linux capabilities considered dangerous
var privilegeDangerousCaps = map[string]bool{
	"CAP_SYS_ADMIN":       true,
	"CAP_SYS_PTRACE":      true,
	"CAP_SYS_MODULE":      true,
	"CAP_NET_ADMIN":       true,
	"CAP_DAC_OVERRIDE":    true,
	"CAP_SETFCAP":         true,
	"CAP_SETUID":          true,
	"CAP_SETGID":          true,
	"CAP_AUDIT_WRITE":     true,
	"CAP_LINUX_IMMUTABLE": true,
}

// handlePrivilegeMap handles GET /api/security/privilege-map
func (s *Server) handlePrivilegeMap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := PrivilegeMapResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	nsStats := make(map[string]*PrivilegeNsStat)
	riskRank := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		wlName := ""
		for _, ref := range pod.OwnerReferences {
			if ref.Controller != nil && *ref.Controller {
				wlName = ref.Name
				break
			}
		}
		if wlName == "" {
			wlName = pod.Name
		}

		hostPID := pod.Spec.HostPID
		hostIPC := pod.Spec.HostIPC
		hostNet := pod.Spec.HostNetwork

		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			var findings []string
			risk := "low"

			// Check privileged flag
			if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
				findings = append(findings, "privileged=true")
				result.Summary.Privileged++
				risk = "critical"
			}

			// Check runAsRoot
			if c.SecurityContext != nil && c.SecurityContext.RunAsUser != nil {
				if *c.SecurityContext.RunAsUser == 0 {
					findings = append(findings, "runAsUser=0 (root)")
					result.Summary.RunAsRoot++
					if riskRank[risk] > riskRank["high"] {
						risk = "high"
					}
				}
			} else if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.RunAsUser != nil {
				if *pod.Spec.SecurityContext.RunAsUser == 0 {
					findings = append(findings, "pod runAsUser=0 (root)")
					result.Summary.RunAsRoot++
					if riskRank[risk] > riskRank["high"] {
						risk = "high"
					}
				}
			} else {
				// No runAsUser specified - defaults to root in many runtimes
				findings = append(findings, "no runAsUser (default may be root)")
				result.Summary.RunAsRoot++
			}

			// Check host namespace access
			if hostPID {
				findings = append(findings, "hostPID=true")
				result.Summary.HostPID++
				if riskRank[risk] > riskRank["high"] {
					risk = "high"
				}
			}
			if hostIPC {
				findings = append(findings, "hostIPC=true")
				result.Summary.HostIPC++
				if riskRank[risk] > riskRank["high"] {
					risk = "high"
				}
			}
			if hostNet {
				findings = append(findings, "hostNetwork=true")
				result.Summary.HostNetwork++
				if riskRank[risk] > riskRank["medium"] {
					risk = "medium"
				}
			}

			// Check dangerous capabilities
			if c.SecurityContext != nil && c.SecurityContext.Capabilities != nil {
				for _, cap := range c.SecurityContext.Capabilities.Add {
					capUpper := strings.ToUpper(string(cap))
					if privilegeDangerousCaps[capUpper] {
						findings = append(findings, fmt.Sprintf("cap=%s", capUpper))
						result.Summary.DangerousCaps++
						if capUpper == "CAP_SYS_ADMIN" && riskRank[risk] > riskRank["critical"] {
							risk = "critical"
						} else if riskRank[risk] > riskRank["high"] {
							risk = "high"
						}
					}
				}
			}

			// Check readOnlyRootFilesystem
			if c.SecurityContext == nil || c.SecurityContext.ReadOnlyRootFilesystem == nil || !*c.SecurityContext.ReadOnlyRootFilesystem {
				result.Summary.NoReadOnlyRootFS++
				if len(findings) == 0 {
					findings = append(findings, "readOnlyRootFilesystem=false")
					risk = "low"
				}
			}

			// Check allowPrivilegeEscalation
			if c.SecurityContext != nil && c.SecurityContext.AllowPrivilegeEscalation != nil && *c.SecurityContext.AllowPrivilegeEscalation {
				findings = append(findings, "allowPrivilegeEscalation=true")
				result.Summary.AllowPrivEscalation++
				if riskRank[risk] > riskRank["high"] {
					risk = "high"
				}
			}

			if len(findings) > 0 {
				entry := PrivilegeEntry{
					Workload:     wlName,
					Namespace:    pod.Namespace,
					Container:    c.Name,
					Findings:     findings,
					RiskLevel:    risk,
					FindingCount: len(findings),
				}
				result.ExposedWorkloads = append(result.ExposedWorkloads, entry)

				// Update namespace stats
				if _, ok := nsStats[pod.Namespace]; !ok {
					nsStats[pod.Namespace] = &PrivilegeNsStat{Namespace: pod.Namespace}
				}
				ns := nsStats[pod.Namespace]
				ns.ExposedCount++
				if riskRank[risk] < riskRank[ns.MaxRiskLevel] || ns.MaxRiskLevel == "" {
					ns.MaxRiskLevel = risk
				}
			}
		}
	}

	// Sort by risk level
	sort.Slice(result.ExposedWorkloads, func(i, j int) bool {
		return riskRank[result.ExposedWorkloads[i].RiskLevel] < riskRank[result.ExposedWorkloads[j].RiskLevel]
	})

	// Namespace stats
	for _, ns := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].ExposedCount > result.ByNamespace[j].ExposedCount
	})

	// Exposure score: lower is better
	exposedCount := len(result.ExposedWorkloads)
	if result.Summary.TotalContainers > 0 {
		result.ExposureScore = 100 - (exposedCount * 100 / result.Summary.TotalContainers)
		if result.ExposureScore < 0 {
			result.ExposureScore = 0
		}
	}

	switch {
	case result.ExposureScore >= 80:
		result.Grade = "A"
	case result.ExposureScore >= 60:
		result.Grade = "B"
	case result.ExposureScore >= 40:
		result.Grade = "C"
	case result.ExposureScore >= 20:
		result.Grade = "D"
	default:
		result.Grade = "F"
	}

	result.Recommendations = buildPrivilegeMapRecs(&result)
	writeJSON(w, result)
}

func buildPrivilegeMapRecs(r *PrivilegeMapResult) []string {
	recs := []string{
		fmt.Sprintf("权限暴露: %d/%d 容器存在风险", len(r.ExposedWorkloads), r.Summary.TotalContainers),
	}
	if r.Summary.Privileged > 0 {
		recs = append(recs, fmt.Sprintf("紧急: %d 个特权容器 (--privileged)", r.Summary.Privileged))
	}
	if r.Summary.HostPID+r.Summary.HostIPC > 0 {
		recs = append(recs, fmt.Sprintf("警告: %d 个容器使用 hostPID/hostIPC", r.Summary.HostPID+r.Summary.HostIPC))
	}
	if r.Summary.DangerousCaps > 0 {
		recs = append(recs, fmt.Sprintf("%d 个容器持有危险 Linux capabilities", r.Summary.DangerousCaps))
	}
	if r.Summary.AllowPrivEscalation > 0 {
		recs = append(recs, fmt.Sprintf("%d 个容器允许权限提升 (allowPrivilegeEscalation=true)", r.Summary.AllowPrivEscalation))
	}
	if r.ExposureScore < 60 {
		recs = append(recs, "建议: 为所有容器设置 runAsNonRoot=true, 移除不必要的 capabilities")
	}
	return recs
}
