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

// MACResult is the Mandatory Access Control (AppArmor/SELinux) audit result.
type MACResult struct {
	ScannedAt       time.Time      `json:"scannedAt"`
	Summary         MACSummary     `json:"summary"`
	ByNamespace     []MACNamespace `json:"byNamespace"`
	NonCompliant    []MACPodEntry  `json:"nonCompliantPods"`
	MissingProfiles []MACPodEntry  `json:"missingProfiles"`
	Recommendations []string       `json:"recommendations"`
}

// MACSummary aggregates cluster-wide MAC statistics.
type MACSummary struct {
	TotalPods          int  `json:"totalPods"`
	WithAppArmor       int  `json:"withAppArmor"`       // pods with AppArmor profile set
	WithSELinux        int  `json:"withSELinux"`        // pods with SELinux context set
	MissingAppArmor    int  `json:"missingAppArmor"`    // pods missing AppArmor in user ns
	MissingSELinux     int  `json:"missingSELinux"`     // pods missing SELinux context
	UnconfinedAppArmor int  `json:"unconfinedAppArmor"` // explicitly set to unconfined
	PermissiveSELinux  int  `json:"permissiveSELinux"`  // SELinux with permissive/disabled
	HasNodeAppArmor    bool `json:"hasNodeAppArmor"`    // at least one node reports AppArmor
	HasNodeSELinux     bool `json:"hasNodeSELinux"`     // at least one node reports SELinux
	ComplianceScore    int  `json:"complianceScore"`    // 0-100
}

// MACNamespace shows MAC compliance per namespace.
type MACNamespace struct {
	Namespace       string  `json:"namespace"`
	TotalPods       int     `json:"totalPods"`
	WithAppArmor    int     `json:"withAppArmor"`
	WithSELinux     int     `json:"withSELinux"`
	MissingAppArmor int     `json:"missingAppArmor"`
	IsSystem        bool    `json:"isSystem"`
	ComplianceRate  float64 `json:"complianceRate"` // %
}

// MACPodEntry describes a pod's MAC configuration.
type MACPodEntry struct {
	PodName       string `json:"podName"`
	Namespace     string `json:"namespace"`
	WorkloadType  string `json:"workloadType"`
	Issue         string `json:"issue"`
	ContainerName string `json:"containerName,omitempty"`
	Severity      string `json:"severity"`
}

// handleMACAudit analyzes AppArmor and SELinux mandatory access control configuration.
// GET /api/security/mac-audit
func (s *Server) handleMACAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	now := time.Now()
	result := MACResult{ScannedAt: now}
	result.Summary.TotalPods = len(pods.Items)

	// Check node capabilities
	for _, node := range nodes.Items {
		annotations := node.Annotations
		if annotations != nil {
			if v, ok := annotations["container.apparmor.security.beta.kubernetes.io"]; ok && v != "" {
				result.Summary.HasNodeAppArmor = true
			}
		}
		// Check for SELinux in OSImage or kernel features
		osImage := strings.ToLower(node.Status.NodeInfo.OSImage)
		if strings.Contains(osImage, "fedora") || strings.Contains(osImage, "rhel") || strings.Contains(osImage, "centos") || strings.Contains(osImage, "rocky") || strings.Contains(osImage, "amzn") {
			result.Summary.HasNodeSELinux = true
		}
	}

	nsStats := map[string]*MACNamespace{}

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		isSys := isSystemNamespace(pod.Namespace)
		wlType := inferWorkloadTypeFromPod(&pod)
		nsName := pod.Namespace

		// Initialize namespace stats
		nsStat, ok := nsStats[nsName]
		if !ok {
			nsStat = &MACNamespace{Namespace: nsName, IsSystem: isSys}
			nsStats[nsName] = nsStat
		}
		nsStat.TotalPods++

		// Check AppArmor at pod level (annotation-based)
		podAppArmor := ""
		if pod.Annotations != nil {
			podAppArmor = pod.Annotations["container.apparmor.security.beta.kubernetes.io"]
		}

		// Check SELinux at pod and container level
		podHasSELinux := pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.SELinuxOptions != nil

		// Check per-container AppArmor and SELinux
		podHasAppArmor := false
		podHasSELinuxAny := podHasSELinux
		hasUnconfinedAppArmor := false
		hasPermissiveSELinux := false

		for _, c := range pod.Spec.Containers {
			// AppArmor (container-level annotation or pod-level)
			cAppArmor := ""
			if pod.Annotations != nil {
				cAppArmor = pod.Annotations["container.apparmor.security.beta.kubernetes.io/"+c.Name]
			}
			if cAppArmor == "" {
				cAppArmor = podAppArmor
			}

			if cAppArmor == "runtime/default" || cAppArmor == "localhost/" {
				podHasAppArmor = true
			} else if cAppArmor == "unconfined" {
				hasUnconfinedAppArmor = true
			}

			// SELinux (container-level)
			if c.SecurityContext != nil && c.SecurityContext.SELinuxOptions != nil {
				podHasSELinuxAny = true
				// Check for permissive/disabled SELinux
				seOpts := c.SecurityContext.SELinuxOptions
				if seOpts.Type != "" && (strings.Contains(seOpts.Type, "unconfined") || strings.Contains(seOpts.Type, "permissive")) {
					hasPermissiveSELinux = true
				}
				if seOpts.Level == "s0" && seOpts.Type == "" {
					// No meaningful SELinux level set
				}
			}
		}

		// Also check init containers
		for _, c := range pod.Spec.InitContainers {
			if c.SecurityContext != nil && c.SecurityContext.SELinuxOptions != nil {
				podHasSELinuxAny = true
				seOpts := c.SecurityContext.SELinuxOptions
				if seOpts.Type != "" && (strings.Contains(seOpts.Type, "unconfined") || strings.Contains(seOpts.Type, "permissive")) {
					hasPermissiveSELinux = true
				}
			}
		}

		// Update summary
		if podHasAppArmor {
			result.Summary.WithAppArmor++
			nsStat.WithAppArmor++
		}
		if podHasSELinuxAny {
			result.Summary.WithSELinux++
			nsStat.WithSELinux++
		}

		if hasUnconfinedAppArmor {
			result.Summary.UnconfinedAppArmor++
			result.NonCompliant = append(result.NonCompliant, MACPodEntry{
				PodName:      pod.Name,
				Namespace:    pod.Namespace,
				WorkloadType: wlType,
				Issue:        "AppArmor profile explicitly set to 'unconfined' — container runs without MAC protection",
				Severity:     "high",
			})
		}

		if hasPermissiveSELinux {
			result.Summary.PermissiveSELinux++
			result.NonCompliant = append(result.NonCompliant, MACPodEntry{
				PodName:      pod.Name,
				Namespace:    pod.Namespace,
				WorkloadType: wlType,
				Issue:        "SELinux context set to permissive/unconfined type — container bypasses SELinux policy enforcement",
				Severity:     "high",
			})
		}

		// Flag missing AppArmor in user namespaces
		if !podHasAppArmor && !isSys {
			result.Summary.MissingAppArmor++
			nsStat.MissingAppArmor++
			result.MissingProfiles = append(result.MissingProfiles, MACPodEntry{
				PodName:      pod.Name,
				Namespace:    pod.Namespace,
				WorkloadType: wlType,
				Issue:        "No AppArmor profile set — container runs without mandatory access control",
				Severity:     "medium",
			})
		}

		// Flag missing SELinux if nodes support it
		if !podHasSELinuxAny && result.Summary.HasNodeSELinux && !isSys {
			result.Summary.MissingSELinux++
		}
	}

	// Build namespace stats
	for _, stat := range nsStats {
		if stat.TotalPods > 0 {
			stat.ComplianceRate = float64(stat.WithAppArmor+stat.WithSELinux) / float64(stat.TotalPods) * 100
			stat.ComplianceRate = float64(int(stat.ComplianceRate*100)) / 100
		}
		result.ByNamespace = append(result.ByNamespace, *stat)
	}

	// Sort: user namespaces with low compliance first
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		if result.ByNamespace[i].IsSystem != result.ByNamespace[j].IsSystem {
			return !result.ByNamespace[i].IsSystem
		}
		return result.ByNamespace[i].ComplianceRate < result.ByNamespace[j].ComplianceRate
	})

	// Sort non-compliant by severity
	sort.Slice(result.NonCompliant, func(i, j int) bool {
		sevOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		return sevOrder[result.NonCompliant[i].Severity] < sevOrder[result.NonCompliant[j].Severity]
	})
	if len(result.NonCompliant) > 30 {
		result.NonCompliant = result.NonCompliant[:30]
	}
	if len(result.MissingProfiles) > 30 {
		result.MissingProfiles = result.MissingProfiles[:30]
	}

	result.Summary.ComplianceScore = macScore(result.Summary)
	result.Recommendations = macRecommendations(&result)

	writeJSON(w, result)
}

// macScore computes a 0-100 MAC compliance score.
func macScore(s MACSummary) int {
	if s.TotalPods == 0 {
		return 100
	}

	score := 0
	totalPods := s.TotalPods

	// AppArmor coverage
	if totalPods > 0 {
		appArmorRatio := float64(s.WithAppArmor) / float64(totalPods)
		score += int(appArmorRatio * 40)
	}

	// SELinux coverage (only if nodes support it)
	if s.HasNodeSELinux && totalPods > 0 {
		seLinuxRatio := float64(s.WithSELinux) / float64(totalPods)
		score += int(seLinuxRatio * 20)
	} else {
		// Nodes don't support SELinux — give partial credit
		score += 10
	}

	// Penalize unconfined/permissive
	score -= min(20, s.UnconfinedAppArmor*5)
	score -= min(15, s.PermissiveSELinux*5)

	// Penalize missing profiles in user namespaces
	score -= min(25, s.MissingAppArmor*3)

	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}

// macRecommendations generates actionable recommendations.
func macRecommendations(r *MACResult) []string {
	var recs []string

	if r.Summary.UnconfinedAppArmor > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d pod(s) explicitly set AppArmor to 'unconfined' — change to 'runtime/default' for MAC protection",
			r.Summary.UnconfinedAppArmor,
		))
	}

	if r.Summary.PermissiveSELinux > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d pod(s) use permissive/unconfined SELinux type — set a restrictive type like 'container_t'",
			r.Summary.PermissiveSELinux,
		))
	}

	if r.Summary.MissingAppArmor > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d pod(s) in user namespaces have no AppArmor profile — add annotation 'container.apparmor.security.beta.kubernetes.io: runtime/default'",
			r.Summary.MissingAppArmor,
		))
	}

	if r.Summary.HasNodeSELinux && r.Summary.MissingSELinux > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d pod(s) on SELinux-enabled nodes have no SELinux context — set securityContext.seLinuxOptions with appropriate type",
			r.Summary.MissingSELinux,
		))
	}

	if !r.Summary.HasNodeAppArmor {
		recs = append(recs, "No nodes report AppArmor support — enable AppArmor on nodes (requires Linux kernel with AppArmor module loaded)")
	}

	if len(recs) == 0 {
		recs = append(recs, "Mandatory access control (AppArmor/SELinux) configuration is healthy — all workloads have appropriate MAC profiles")
	}

	return recs
}
