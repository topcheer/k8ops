package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RuntimeThreatResult analyzes runtime security threats:
// privileged pods, host namespace access, hostPath mounts,
// dangerous capabilities, and shared host network/PID/IPC.
type RuntimeThreatResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Summary         RuntimeThreatSummary `json:"summary"`
	Threats         []RuntimeThreat      `json:"threats"`
	PrivilegedPods  []PrivilegedPod      `json:"privilegedPods"`
	ThreatScore     int                  `json:"threatScore"`
	Grade           string               `json:"grade"`
	Recommendations []string             `json:"recommendations"`
}

type RuntimeThreatSummary struct {
	TotalPods       int `json:"totalPods"`
	PrivilegedPods  int `json:"privilegedPods"`
	HostNetworkPods int `json:"hostNetworkPods"`
	HostPIDPods     int `json:"hostPIDPods"`
	HostIPCPods     int `json:"hostIPCPods"`
	HostPathMounts  int `json:"hostPathMounts"`
	DangerousCaps   int `json:"dangerousCaps"`
	RunAsRoot       int `json:"runAsRoot"`
}

type RuntimeThreat struct {
	Pod       string   `json:"pod"`
	Namespace string   `json:"namespace"`
	Threats   []string `json:"threats"`
	Severity  string   `json:"severity"`
}

type PrivilegedPod struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Reason    string `json:"reason"`
}

// dangerous security capabilities
var dangerousCaps = map[string]bool{
	"SYS_ADMIN": true, "NET_ADMIN": true, "SYS_PTRACE": true,
	"SYS_MODULE": true, "DAC_OVERRIDE": true, "SETUID": true,
	"SETGID": true, "CHOWN": true, "FOWNER": true,
}

// handleRuntimeThreat analyzes runtime security threats.
// GET /api/security/runtime-threat
func (s *Server) handleRuntimeThreat(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := RuntimeThreatResult{ScannedAt: time.Now()}
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	for _, pod := range pods.Items {
		if systemNS[pod.Namespace] {
			continue
		}
		if pod.Status.Phase != "Running" {
			continue
		}
		result.Summary.TotalPods++

		var threats []string
		severity := "low"

		// Host namespace access
		if pod.Spec.HostNetwork {
			result.Summary.HostNetworkPods++
			threats = append(threats, "hostNetwork: true")
			severity = "high"
		}
		if pod.Spec.HostPID {
			result.Summary.HostPIDPods++
			threats = append(threats, "hostPID: true")
			severity = "high"
		}
		if pod.Spec.HostIPC {
			result.Summary.HostIPCPods++
			threats = append(threats, "hostIPC: true")
			severity = "medium"
		}

		for _, c := range pod.Spec.Containers {
			// Privileged
			if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
				result.Summary.PrivilegedPods++
				threats = append(threats, fmt.Sprintf("container '%s' is privileged", c.Name))
				severity = "critical"
				result.PrivilegedPods = append(result.PrivilegedPods, PrivilegedPod{
					Pod: pod.Name, Namespace: pod.Namespace,
					Reason: fmt.Sprintf("Container '%s' runs privileged", c.Name),
				})
			}

			// Run as root
			if c.SecurityContext != nil && c.SecurityContext.RunAsUser != nil {
				if *c.SecurityContext.RunAsUser == 0 {
					result.Summary.RunAsRoot++
					threats = append(threats, fmt.Sprintf("'%s' runs as root (uid=0)", c.Name))
					if severity == "low" {
						severity = "medium"
					}
				}
			} else if c.SecurityContext == nil || c.SecurityContext.RunAsUser == nil {
				// No security context = defaults to root
				result.Summary.RunAsRoot++
				threats = append(threats, fmt.Sprintf("'%s' has no runAsUser (defaults to root)", c.Name))
				if severity == "low" {
					severity = "medium"
				}
			}

			// Dangerous capabilities
			if c.SecurityContext != nil && c.SecurityContext.Capabilities != nil {
				for _, cap := range c.SecurityContext.Capabilities.Add {
					capStr := string(cap)
					if dangerousCaps[capStr] {
						result.Summary.DangerousCaps++
						threats = append(threats, fmt.Sprintf("'%s' adds dangerous capability %s", c.Name, capStr))
						if severity != "critical" {
							severity = "high"
						}
					}
				}
			}
		}

		// HostPath mounts
		for _, vol := range pod.Spec.Volumes {
			if vol.HostPath != nil {
				result.Summary.HostPathMounts++
				threats = append(threats, fmt.Sprintf("hostPath mount: %s -> %s", vol.HostPath.Path, vol.Name))
				if severity == "low" {
					severity = "medium"
				}
			}
		}

		if len(threats) > 0 {
			result.Threats = append(result.Threats, RuntimeThreat{
				Pod: pod.Name, Namespace: pod.Namespace,
				Threats: threats, Severity: severity,
			})
		}
	}

	// Score: start at 100, subtract for each threat category
	score := 100
	score -= result.Summary.PrivilegedPods * 15
	score -= result.Summary.HostNetworkPods * 8
	score -= result.Summary.HostPIDPods * 8
	score -= result.Summary.HostPathMounts * 3
	score -= result.Summary.DangerousCaps * 5
	score -= result.Summary.RunAsRoot * 2
	if score < 0 {
		score = 0
	}
	result.ThreatScore = min(100, score)
	result.Grade = goldenScoreToGrade(result.ThreatScore)

	sort.Slice(result.Threats, func(i, j int) bool {
		return result.Threats[i].Severity > result.Threats[j].Severity
	})

	var recs []string
	recs = append(recs, fmt.Sprintf("Runtime threat score: %d/100 (grade %s) — %d/%d pods with threats", result.ThreatScore, result.Grade, len(result.Threats), result.Summary.TotalPods))
	if result.Summary.PrivilegedPods > 0 {
		recs = append(recs, fmt.Sprintf("%d privileged pods — remove privileged mode or use fine-grained capabilities", result.Summary.PrivilegedPods))
	}
	if result.Summary.HostNetworkPods > 0 {
		recs = append(recs, fmt.Sprintf("%d pods use hostNetwork — restrict to system namespaces only", result.Summary.HostNetworkPods))
	}
	if result.Summary.HostPathMounts > 0 {
		recs = append(recs, fmt.Sprintf("%d hostPath mounts — use PVs/PVCs instead for persistence", result.Summary.HostPathMounts))
	}
	if result.Summary.RunAsRoot > 0 {
		recs = append(recs, fmt.Sprintf("%d pods run as root — set runAsUser to non-zero in securityContext", result.Summary.RunAsRoot))
	}
	if len(recs) == 1 {
		recs = append(recs, "Runtime security posture is clean — no privileged pods or host access detected")
	}
	result.Recommendations = recs

	writeJSON(w, result)
}

// init to ensure package-level map is initialized
func init() {
	if dangerousCaps == nil {
		dangerousCaps = map[string]bool{}
	}
}
