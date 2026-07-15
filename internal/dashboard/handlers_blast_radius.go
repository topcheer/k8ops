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

// BlastRadiusResult is the workload attack surface & blast radius analysis.
type BlastRadiusResult struct {
	Timestamp       time.Time          `json:"timestamp"`
	Score           int                `json:"score"`
	Status          string             `json:"status"`
	Summary         BlastSummary       `json:"summary"`
	HighRiskPods    []BlastRadiusEntry `json:"highRiskPods"`
	ByNamespace     []BlastNSStat      `json:"byNamespace"`
	RiskHeatmap     []RiskHeatEntry    `json:"riskHeatmap"`
	AttackVectors   []AttackVector     `json:"attackVectors"`
	Recommendations []string           `json:"recommendations"`
}

// BlastSummary holds aggregate blast radius metrics.
type BlastSummary struct {
	TotalPods          int `json:"totalPods"`
	CriticalRiskPods   int `json:"criticalRiskPods"`
	HighRiskPods       int `json:"highRiskPods"`
	MediumRiskPods     int `json:"mediumRiskPods"`
	LowRiskPods        int `json:"lowRiskPods"`
	PrivilegedPods     int `json:"privilegedPods"`
	HostNetworkPods    int `json:"hostNetworkPods"`
	HostPIDPods        int `json:"hostPIDPods"`
	HostPathPods       int `json:"hostPathPods"`
	HostIPCPods        int `json:"hostIPCPods"`
	PrivEscalationPods int `json:"privEscalationPods"`
}

// BlastRadiusEntry describes one pod's blast radius score.
type BlastRadiusEntry struct {
	Namespace      string   `json:"namespace"`
	Pod            string   `json:"pod"`
	RiskScore      int      `json:"riskScore"`
	RiskLevel      string   `json:"riskLevel"`
	Vectors        []string `json:"vectors"`
	SAName         string   `json:"serviceAccount"`
	HostAccess     string   `json:"hostAccess"`
	SecretsMounted int      `json:"secretsMounted"`
}

// BlastNSStat shows blast radius per namespace.
type BlastNSStat struct {
	Namespace string `json:"namespace"`
	PodCount  int    `json:"podCount"`
	AvgScore  int    `json:"avgScore"`
	MaxScore  int    `json:"maxScore"`
	HighRisk  int    `json:"highRisk"`
}

// RiskHeatEntry maps a risk range to pod count.
type RiskHeatEntry struct {
	Range     string `json:"range"`
	Count     int    `json:"count"`
	RiskLevel string `json:"riskLevel"`
}

// AttackVector describes a common attack vector found.
type AttackVector struct {
	Vector     string `json:"vector"`
	Count      int    `json:"count"`
	Severity   string `json:"severity"`
	Mitigation string `json:"mitigation"`
}

func (s *Server) handleBlastRadius(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	pods, err := rc.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list pods: %v", err))
		return
	}

	result := analyzeBlastRadius(pods.Items)
	writeJSON(w, result)
}

func analyzeBlastRadius(pods []corev1.Pod) BlastRadiusResult {
	now := time.Now()

	skipNS := map[string]bool{"kube-system": true, "k8ops-system": true}
	var entries []BlastRadiusEntry
	vectorCounts := make(map[string]*AttackVector)
	nsStats := make(map[string]*BlastNSStat)

	summary := BlastSummary{}

	for _, pod := range pods {
		if skipNS[pod.Namespace] {
			continue
		}
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		summary.TotalPods++
		score := 0
		var vectors []string
		secretsMounted := 0

		// Pod-level security
		if pod.Spec.HostNetwork {
			score += 30
			vectors = append(vectors, "hostNetwork")
			summary.HostNetworkPods++
		}
		if pod.Spec.HostPID {
			score += 25
			vectors = append(vectors, "hostPID")
			summary.HostPIDPods++
		}
		if pod.Spec.HostIPC {
			score += 20
			vectors = append(vectors, "hostIPC")
			summary.HostIPCPods++
		}

		// Container-level security
		for _, c := range pod.Spec.Containers {
			if c.SecurityContext != nil {
				sc := c.SecurityContext
				if sc.Privileged != nil && *sc.Privileged {
					score += 40
					vectors = append(vectors, "privileged")
					summary.PrivilegedPods++
				}
				if sc.AllowPrivilegeEscalation != nil && *sc.AllowPrivilegeEscalation {
					score += 15
					vectors = append(vectors, "allowPrivilegeEscalation")
					summary.PrivEscalationPods++
				}
				if sc.RunAsNonRoot == nil || (sc.RunAsNonRoot != nil && !*sc.RunAsNonRoot) {
					if sc.RunAsUser == nil || *sc.RunAsUser == 0 {
						score += 10
						vectors = append(vectors, "runsAsRoot")
					}
				}
				// Dangerous capabilities
				if sc.Capabilities != nil {
					for _, cap := range sc.Capabilities.Add {
						capLower := strings.ToLower(string(cap))
						if capLower == "sys_admin" {
							score += 25
							vectors = append(vectors, "CAP_SYS_ADMIN")
						} else if capLower == "net_admin" {
							score += 15
							vectors = append(vectors, "CAP_NET_ADMIN")
						} else if capLower == "all" {
							score += 20
							vectors = append(vectors, "CAP_ALL")
						}
					}
				}
			}

			// Host path mounts
			for _, vm := range c.VolumeMounts {
				_ = vm
			}
		}

		// Pod-level volume mounts
		for _, vol := range pod.Spec.Volumes {
			if vol.HostPath != nil {
				score += 15
				if !blastContains(vectors, "hostPath") {
					vectors = append(vectors, "hostPath")
				}
				summary.HostPathPods++
				// Especially dangerous paths
				path := vol.HostPath.Path
				if strings.HasPrefix(path, "/") && (strings.Contains(path, "docker") || strings.Contains(path, "containerd") || strings.Contains(path, "/var/run")) {
					score += 10
					vectors = append(vectors, "hostPath:containerRuntime")
				}
			}
			if vol.Secret != nil {
				secretsMounted++
			}
			if vol.Projected != nil {
				for _, src := range vol.Projected.Sources {
					if src.Secret != nil {
						secretsMounted++
					}
					if src.ServiceAccountToken != nil {
						score += 3
					}
				}
			}
		}

		// Multiple secrets = higher blast radius
		if secretsMounted > 3 {
			score += 5
			vectors = append(vectors, "manySecrets")
		}

		// Cap score
		if score > 100 {
			score = 100
		}

		// Risk level
		riskLevel := "low"
		if score >= 70 {
			riskLevel = "critical"
			summary.CriticalRiskPods++
		} else if score >= 40 {
			riskLevel = "high"
			summary.HighRiskPods++
		} else if score >= 15 {
			riskLevel = "medium"
			summary.MediumRiskPods++
		} else {
			summary.LowRiskPods++
		}

		entry := BlastRadiusEntry{
			Namespace:      pod.Namespace,
			Pod:            pod.Name,
			RiskScore:      score,
			RiskLevel:      riskLevel,
			Vectors:        vectors,
			SAName:         pod.Spec.ServiceAccountName,
			SecretsMounted: secretsMounted,
		}
		hostAccess := "none"
		if pod.Spec.HostNetwork {
			hostAccess = "network"
		}
		if pod.Spec.HostPID {
			hostAccess += "+pid"
		}
		entry.HostAccess = hostAccess

		entries = append(entries, entry)

		// Track vectors
		for _, v := range vectors {
			if av, ok := vectorCounts[v]; ok {
				av.Count++
			} else {
				vectorCounts[v] = &AttackVector{
					Vector:     v,
					Count:      1,
					Severity:   vectorSeverity(v),
					Mitigation: vectorMitigation(v),
				}
			}
		}

		// Namespace stats
		ns, ok := nsStats[pod.Namespace]
		if !ok {
			ns = &BlastNSStat{Namespace: pod.Namespace}
			nsStats[pod.Namespace] = ns
		}
		ns.PodCount++
		ns.AvgScore += score
		if score > ns.MaxScore {
			ns.MaxScore = score
		}
		if riskLevel == "high" || riskLevel == "critical" {
			ns.HighRisk++
		}
	}

	// Build heatmap
	heatBuckets := map[string]int{"0-14 (low)": 0, "15-39 (medium)": 0, "40-69 (high)": 0, "70-100 (critical)": 0}
	riskLevels := map[string]string{"0-14 (low)": "low", "15-39 (medium)": "medium", "40-69 (high)": "high", "70-100 (critical)": "critical"}
	for _, e := range entries {
		switch {
		case e.RiskScore < 15:
			heatBuckets["0-14 (low)"]++
		case e.RiskScore < 40:
			heatBuckets["15-39 (medium)"]++
		case e.RiskScore < 70:
			heatBuckets["40-69 (high)"]++
		default:
			heatBuckets["70-100 (critical)"]++
		}
	}
	var heatmap []RiskHeatEntry
	for _, k := range []string{"0-14 (low)", "15-39 (medium)", "40-69 (high)", "70-100 (critical)"} {
		heatmap = append(heatmap, RiskHeatEntry{Range: k, Count: heatBuckets[k], RiskLevel: riskLevels[k]})
	}

	// Sort entries by score desc
	sort.Slice(entries, func(i, j int) bool { return entries[i].RiskScore > entries[j].RiskScore })

	// Top 50 high-risk pods
	highRisk := make([]BlastRadiusEntry, 0, 50)
	for _, e := range entries {
		if e.RiskLevel == "high" || e.RiskLevel == "critical" {
			highRisk = append(highRisk, e)
			if len(highRisk) >= 50 {
				break
			}
		}
	}

	// NS stats
	var nsList []BlastNSStat
	for _, ns := range nsStats {
		if ns.PodCount > 0 {
			ns.AvgScore = ns.AvgScore / ns.PodCount
		}
		nsList = append(nsList, *ns)
	}
	sort.Slice(nsList, func(i, j int) bool { return nsList[i].MaxScore > nsList[j].MaxScore })

	// Attack vectors sorted by count
	var avs []AttackVector
	for _, av := range vectorCounts {
		avs = append(avs, *av)
	}
	sort.Slice(avs, func(i, j int) bool { return avs[i].Count > avs[j].Count })

	// Score
	score := 100
	score -= summary.CriticalRiskPods * 5
	score -= summary.HighRiskPods * 3
	if score < 0 {
		score = 0
	}

	status := "healthy"
	if score < 50 {
		status = "critical"
	} else if score < 80 {
		status = "warning"
	}

	// Recommendations
	var recs []string
	if summary.PrivilegedPods > 0 {
		recs = append(recs, fmt.Sprintf("%d privileged pod(s); remove privileged flag or use targeted capabilities", summary.PrivilegedPods))
	}
	if summary.HostNetworkPods > 0 || summary.HostPIDPods > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) with hostNetwork/hostPID/hostIPC access; restrict to system components only", summary.HostNetworkPods+summary.HostPIDPods+summary.HostIPCPods))
	}
	if summary.HostPathPods > 0 {
		recs = append(recs, fmt.Sprintf("%d pod(s) with hostPath mounts; use projected volumes or typed CSI drivers instead", summary.HostPathPods))
	}
	if summary.CriticalRiskPods > 0 {
		recs = append(recs, fmt.Sprintf("%d critical-risk pod(s) identified; prioritize remediation", summary.CriticalRiskPods))
	}
	if len(recs) == 0 {
		recs = append(recs, "Workload attack surface looks well-contained; no critical blast radius risks detected")
	}

	return BlastRadiusResult{
		Timestamp:       now,
		Score:           score,
		Status:          status,
		Summary:         summary,
		HighRiskPods:    highRisk,
		ByNamespace:     nsList,
		RiskHeatmap:     heatmap,
		AttackVectors:   avs,
		Recommendations: recs,
	}
}

func blastContains(s []string, v string) bool {
	for _, item := range s {
		if item == v {
			return true
		}
	}
	return false
}

func vectorSeverity(v string) string {
	switch v {
	case "privileged", "CAP_SYS_ADMIN", "hostPath:containerRuntime":
		return "critical"
	case "hostNetwork", "hostPID", "CAP_ALL":
		return "high"
	case "hostIPC", "hostPath", "CAP_NET_ADMIN", "allowPrivilegeEscalation":
		return "medium"
	default:
		return "low"
	}
}

func vectorMitigation(v string) string {
	switch v {
	case "privileged":
		return "Remove privileged: true; use specific capabilities instead"
	case "hostNetwork":
		return "Use ClusterIP service or hostPorts instead of hostNetwork"
	case "hostPID":
		return "Avoid hostPID; use pod-level process isolation"
	case "hostIPC":
		return "Avoid hostIPC; use shared volumes for IPC instead"
	case "hostPath":
		return "Replace hostPath with PVC, projected volume, or typed CSI driver"
	case "hostPath:containerRuntime":
		return "Remove container runtime socket mount; use API-based access"
	case "CAP_SYS_ADMIN":
		return "Remove CAP_SYS_ADMIN; it grants near-root access"
	case "CAP_NET_ADMIN":
		return "Remove CAP_NET_ADMIN unless doing network configuration"
	case "CAP_ALL":
		return "Remove CAP_ALL; add only specific needed capabilities"
	case "allowPrivilegeEscalation":
		return "Set allowPrivilegeEscalation: false"
	case "runsAsRoot", "runAsRoot":
		return "Set runAsNonRoot: true and use non-zero UID"
	case "manySecrets":
		return "Reduce number of mounted secrets; use IRSA/workload identity"
	default:
		return "Review and tighten security context"
	}
}
