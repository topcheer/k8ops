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

// ContainerSecurityResult is the full container security context audit.
type ContainerSecurityResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         ContainerSecSummary `json:"summary"`
	Pods            []ContainerSecEntry `json:"pods"`
	TopRisks        []ContainerRiskItem `json:"topRisks"`
	ByNamespace     []ContainerSecNs    `json:"byNamespace"`
	Recommendations []string            `json:"recommendations"`
}

// ContainerSecSummary aggregates cluster-wide container security metrics.
type ContainerSecSummary struct {
	TotalPods           int `json:"totalPods"`
	TotalContainers     int `json:"totalContainers"`
	Privileged          int `json:"privileged"`          // running as privileged
	PrivEscalation      int `json:"privEscalation"`      // allowPrivilegeEscalation=true
	RunAsRoot           int `json:"runAsRoot"`           // runAsUser=0 or no securityContext
	RunAsNonRootFalse   int `json:"runAsNonRootFalse"`   // explicitly runAsNonRoot=false
	ReadOnlyRootFSFalse int `json:"readOnlyRootFSFalse"` // readOnlyRootFilesystem=false
	HasHostNetwork      int `json:"hasHostNetwork"`
	HasHostPID          int `json:"hasHostPID"`
	HasHostIPC          int `json:"hasHostIPC"`
	HostPathMounts      int `json:"hostPathMounts"`    // pods with hostPath volumes
	DangerousCaps       int `json:"dangerousCaps"`     // containers with dangerous capabilities
	NoSecurityContext   int `json:"noSecurityContext"` // no securityContext at all
	SecurityScore       int `json:"securityScore"`     // 0-100
}

// ContainerSecEntry describes security for one pod.
type ContainerSecEntry struct {
	Name        string          `json:"name"`
	Namespace   string          `json:"namespace"`
	Node        string          `json:"node"`
	Containers  []ContainerInfo `json:"containers"`
	HostNetwork bool            `json:"hostNetwork"`
	HostPID     bool            `json:"hostPID"`
	HostIPC     bool            `json:"hostIPC"`
	HostPaths   []string        `json:"hostPaths"`
	RiskLevel   string          `json:"riskLevel"` // critical / high / medium / low
	Issues      []string        `json:"issues"`
}

// ContainerInfo describes security context for one container.
type ContainerInfo struct {
	Name               string   `json:"name"`
	Privileged         bool     `json:"privileged"`
	AllowPrivEsc       bool     `json:"allowPrivEscalation"`
	RunAsRoot          bool     `json:"runAsRoot"`
	RunAsNonRoot       bool     `json:"runAsNonRoot"`
	ReadOnlyRootFS     bool     `json:"readOnlyRootFS"`
	HasSecurityContext bool     `json:"hasSecurityContext"`
	DangerousCaps      []string `json:"dangerousCaps,omitempty"`
}

// ContainerRiskItem is a top risk summary.
type ContainerRiskItem struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	RiskLevel string `json:"riskLevel"`
	Reason    string `json:"reason"`
}

// ContainerSecNs per-namespace security stats.
type ContainerSecNs struct {
	Namespace string `json:"namespace"`
	PodCount  int    `json:"podCount"`
	Issues    int    `json:"issues"`
}

// dangerousCapabilities are Linux capabilities that pose significant risk.
var dangerousCapabilities = map[string]bool{
	"SYS_ADMIN":    true,
	"SYS_MODULE":   true,
	"SYS_PTRACE":   true,
	"SYS_RAWIO":    true,
	"NET_ADMIN":    true,
	"NET_RAW":      true,
	"DAC_OVERRIDE": true,
	"SETUID":       true,
	"SETGID":       true,
	"CHOWN":        true,
	"ALL":          true,
}

// handleContainerSecurityAudit scans all pods for container security context risks.
// GET /api/security/containers?namespace=xxx
func (s *Server) handleContainerSecurityAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")

	pods, err := rc.clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	result := ContainerSecurityResult{ScannedAt: time.Now()}
	nsMap := make(map[string]*ContainerSecNs)

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		entry := ContainerSecEntry{
			Name:        pod.Name,
			Namespace:   pod.Namespace,
			Node:        pod.Spec.NodeName,
			HostNetwork: pod.Spec.HostNetwork,
			HostPID:     pod.Spec.HostPID,
			HostIPC:     pod.Spec.HostIPC,
		}

		var podIssues []string

		// Host namespace sharing
		if pod.Spec.HostNetwork {
			podIssues = append(podIssues, "uses hostNetwork — can see all host network traffic")
			result.Summary.HasHostNetwork++
		}
		if pod.Spec.HostPID {
			podIssues = append(podIssues, "uses hostPID — can see all host processes")
			result.Summary.HasHostPID++
		}
		if pod.Spec.HostIPC {
			podIssues = append(podIssues, "uses hostIPC — shares host IPC namespace")
			result.Summary.HasHostIPC++
		}

		// HostPath volumes
		for _, vol := range pod.Spec.Volumes {
			if vol.HostPath != nil {
				path := vol.HostPath.Path
				entry.HostPaths = append(entry.HostPaths, path)
				if isSensitiveHostPath(path) {
					podIssues = append(podIssues, fmt.Sprintf("sensitive hostPath mount: %s", path))
				} else {
					podIssues = append(podIssues, fmt.Sprintf("hostPath mount: %s", path))
				}
				result.Summary.HostPathMounts++
			}
		}

		// Pod-level security context
		podSC := pod.Spec.SecurityContext
		podRunAsNonRoot := false
		podHasSC := podSC != nil
		if podHasSC {
			if podSC.RunAsNonRoot != nil {
				podRunAsNonRoot = *podSC.RunAsNonRoot
			}
		}

		// Container-level analysis
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			ci := ContainerInfo{
				Name: c.Name,
			}

			sc := c.SecurityContext
			if sc == nil {
				ci.HasSecurityContext = false
				// Inherit pod context
				if !podRunAsNonRoot {
					ci.RunAsRoot = true
				}
				result.Summary.NoSecurityContext++
				podIssues = append(podIssues, fmt.Sprintf("container %q has no securityContext", c.Name))
			} else {
				ci.HasSecurityContext = true

				// Privileged
				if sc.Privileged != nil && *sc.Privileged {
					ci.Privileged = true
					result.Summary.Privileged++
					podIssues = append(podIssues, fmt.Sprintf("container %q runs as privileged", c.Name))
				}

				// AllowPrivilegeEscalation
				if sc.AllowPrivilegeEscalation != nil && *sc.AllowPrivilegeEscalation {
					ci.AllowPrivEsc = true
					result.Summary.PrivEscalation++
					podIssues = append(podIssues, fmt.Sprintf("container %q allows privilege escalation", c.Name))
				}

				// RunAsUser
				if sc.RunAsUser != nil {
					if *sc.RunAsUser == 0 {
						ci.RunAsRoot = true
						result.Summary.RunAsRoot++
						podIssues = append(podIssues, fmt.Sprintf("container %q runs as root (UID 0)", c.Name))
					}
				} else if !podRunAsNonRoot && (podSC == nil || podSC.RunAsUser == nil) {
					// No runAsUser at all — defaults to root in many runtimes
					ci.RunAsRoot = true
					result.Summary.RunAsRoot++
				}

				// RunAsNonRoot
				if sc.RunAsNonRoot != nil {
					if *sc.RunAsNonRoot {
						ci.RunAsNonRoot = true
					} else {
						result.Summary.RunAsNonRootFalse++
						podIssues = append(podIssues, fmt.Sprintf("container %q explicitly sets runAsNonRoot=false", c.Name))
					}
				} else if podRunAsNonRoot {
					ci.RunAsNonRoot = true
				}

				// ReadOnlyRootFilesystem
				if sc.ReadOnlyRootFilesystem != nil {
					if *sc.ReadOnlyRootFilesystem {
						ci.ReadOnlyRootFS = true
					}
				} else {
					result.Summary.ReadOnlyRootFSFalse++
				}

				// Capabilities
				if sc.Capabilities != nil {
					for _, cap := range sc.Capabilities.Add {
						capStr := string(cap)
						if dangerousCapabilities[capStr] {
							ci.DangerousCaps = append(ci.DangerousCaps, capStr)
							result.Summary.DangerousCaps++
							podIssues = append(podIssues, fmt.Sprintf("container %q adds dangerous capability %s", c.Name, capStr))
						}
					}
				}
			}

			entry.Containers = append(entry.Containers, ci)
		}

		// Assess risk
		entry.Issues = podIssues
		entry.RiskLevel = assessContainerRisk(entry)

		// Namespace stats
		nsStat := getOrCreateContainerSecNs(nsMap, pod.Namespace)
		nsStat.PodCount++
		nsStat.Issues += len(podIssues)

		result.Summary.TotalPods++
		result.Pods = append(result.Pods, entry)

		// Top risks
		if entry.RiskLevel == "critical" || entry.RiskLevel == "high" {
			result.TopRisks = append(result.TopRisks, ContainerRiskItem{
				Pod:       pod.Name,
				Namespace: pod.Namespace,
				RiskLevel: entry.RiskLevel,
				Reason:    strings.Join(podIssues[:min(3, len(podIssues))], "; "),
			})
		}
	}

	// Sort pods by risk
	sort.Slice(result.Pods, func(i, j int) bool {
		return containerRiskRank(result.Pods[i].RiskLevel) < containerRiskRank(result.Pods[j].RiskLevel)
	})

	// Sort top risks
	sort.Slice(result.TopRisks, func(i, j int) bool {
		return containerRiskRank(result.TopRisks[i].RiskLevel) < containerRiskRank(result.TopRisks[j].RiskLevel)
	})
	if len(result.TopRisks) > 30 {
		result.TopRisks = result.TopRisks[:30]
	}

	// Namespace stats
	for _, nsStat := range nsMap {
		result.ByNamespace = append(result.ByNamespace, *nsStat)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].Issues > result.ByNamespace[j].Issues
	})

	// Score
	result.Summary.SecurityScore = calculateContainerSecScore(result.Summary)

	// Recommendations
	result.Recommendations = generateContainerSecRecs(result.Summary)

	writeJSON(w, result)
}

// isSensitiveHostPath checks if a hostPath mount is to a sensitive directory.
func isSensitiveHostPath(path string) bool {
	sensitive := []string{"/etc", "/var/run", "/root", "/proc", "/sys", "/dev", "/boot", "/lib", "/usr/lib"}
	for _, s := range sensitive {
		if path == s || strings.HasPrefix(path, s+"/") {
			return true
		}
	}
	return false
}

// assessContainerRisk determines risk level for a pod.
func assessContainerRisk(entry ContainerSecEntry) string {
	risk := 0

	for _, c := range entry.Containers {
		if c.Privileged {
			risk += 30
		}
		if c.AllowPrivEsc {
			risk += 15
		}
		if c.RunAsRoot {
			risk += 10
		}
		for range c.DangerousCaps {
			risk += 10
		}
	}

	if entry.HostNetwork {
		risk += 10
	}
	if entry.HostPID {
		risk += 15
	}
	if entry.HostIPC {
		risk += 10
	}
	for _, hp := range entry.HostPaths {
		if isSensitiveHostPath(hp) {
			risk += 15
		} else {
			risk += 5
		}
	}

	switch {
	case risk >= 40:
		return "critical"
	case risk >= 20:
		return "high"
	case risk >= 10:
		return "medium"
	default:
		return "low"
	}
}

// calculateContainerSecScore computes 0-100.
func calculateContainerSecScore(s ContainerSecSummary) int {
	if s.TotalContainers == 0 {
		return 100
	}
	score := 100
	score -= s.Privileged * 8
	score -= s.PrivEscalation * 5
	score -= s.RunAsRoot * 2
	score -= s.DangerousCaps * 3
	score -= s.HasHostPID * 5
	score -= s.HasHostNetwork * 3
	score -= s.HasHostIPC * 3
	score -= s.HostPathMounts * 2
	score -= s.NoSecurityContext * 1
	if score < 0 {
		score = 0
	}
	return score
}

// generateContainerSecRecs produces actionable recommendations.
func generateContainerSecRecs(s ContainerSecSummary) []string {
	var recs []string

	if s.Privileged > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) run as privileged — remove privileged flag unless absolutely necessary", s.Privileged))
	}
	if s.PrivEscalation > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) allow privilege escalation — set allowPrivilegeEscalation=false", s.PrivEscalation))
	}
	if s.RunAsRoot > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) run as root — set runAsNonRoot=true and use non-zero UID", s.RunAsRoot))
	}
	if s.DangerousCaps > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) add dangerous Linux capabilities — remove unless explicitly needed", s.DangerousCaps))
	}
	if s.HasHostPID > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) use hostPID — can see all host processes, remove if not needed", s.HasHostPID))
	}
	if s.HasHostNetwork > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) use hostNetwork — can observe all host network traffic", s.HasHostNetwork))
	}
	if s.HostPathMounts > 0 {
		recs = append(recs, fmt.Sprintf("%d hostPath mount(s) detected — use volumes/PVCs instead for isolation", s.HostPathMounts))
	}
	if s.NoSecurityContext > 0 {
		recs = append(recs, fmt.Sprintf("%d container(s) have no securityContext — add security constraints for defense in depth", s.NoSecurityContext))
	}
	if s.SecurityScore < 50 {
		recs = append(recs, fmt.Sprintf("Container security score is %d/100 — enforce Pod Security Standards (restricted/baseline)", s.SecurityScore))
	}

	return recs
}

func getOrCreateContainerSecNs(m map[string]*ContainerSecNs, ns string) *ContainerSecNs {
	if e, ok := m[ns]; ok {
		return e
	}
	e := &ContainerSecNs{Namespace: ns}
	m[ns] = e
	return e
}

func containerRiskRank(level string) int {
	switch level {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	default:
		return 3
	}
}
