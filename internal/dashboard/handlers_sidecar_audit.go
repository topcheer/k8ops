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

// SidecarResult is the sidecar container overhead & injection analysis.
type SidecarResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	Summary         SidecarSummary    `json:"summary"`
	ByType          []SidecarTypeStat `json:"bySidecarType"`
	ByNamespace     []SidecarNSStat   `json:"byNamespace"`
	HighOverhead    []SidecarEntry    `json:"highOverheadPods"` // pods with >30% sidecar overhead
	InjectedOnly    []SidecarEntry    `json:"injectedOnlyPods"` // pods where all containers are injected
	Recommendations []string          `json:"recommendations"`
}

// SidecarSummary aggregates sidecar statistics.
type SidecarSummary struct {
	TotalPods        int     `json:"totalPods"`
	PodsWithSidecars int     `json:"podsWithSidecars"`
	TotalSidecars    int     `json:"totalSidecars"`
	TotalContainers  int     `json:"totalContainers"`
	SidecarRatio     float64 `json:"sidecarRatio"` // sidecars / total containers
	CPUTotalMilli    int64   `json:"sidecarCPUMilli"`
	MemTotalMi       int64   `json:"sidecarMemMi"`
	CPUOverheadPct   float64 `json:"cpuOverheadPct"` // sidecar CPU / total CPU requests
	MemOverheadPct   float64 `json:"memOverheadPct"`
	InjectedPods     int     `json:"injectedPods"`   // pods with auto-injected sidecars
	ManualSidecars   int     `json:"manualSidecars"` // manually added sidecars
	HealthScore      int     `json:"healthScore"`
}

// SidecarTypeStat shows stats by detected sidecar type.
type SidecarTypeStat struct {
	Type      string `json:"type"` // istio-proxy, vault-agent, fluentd, etc.
	Count     int    `json:"count"`
	CPUMilli  int64  `json:"cpuMilli"`
	MemMi     int64  `json:"memMi"`
	Detection string `json:"detection"` // "injected" or "manual"
}

// SidecarNSStat shows sidecar overhead per namespace.
type SidecarNSStat struct {
	Namespace    string  `json:"namespace"`
	TotalPods    int     `json:"totalPods"`
	PodsWithSide int     `json:"podsWithSidecars"`
	SidecarCount int     `json:"sidecarCount"`
	CPUOverhead  float64 `json:"cpuOverheadPct"`
	MemOverhead  float64 `json:"memOverheadPct"`
}

// SidecarEntry describes a pod with notable sidecar configuration.
type SidecarEntry struct {
	PodName        string  `json:"podName"`
	Namespace      string  `json:"namespace"`
	SidecarType    string  `json:"sidecarType"`
	CPUOverheadPct float64 `json:"cpuOverheadPct"`
	MemOverheadPct float64 `json:"memOverheadPct"`
	Severity       string  `json:"severity"`
}

// handleSidecarAudit analyzes sidecar containers, their overhead, and injection patterns.
// GET /api/deployment/sidecar-audit
func (s *Server) handleSidecarAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	now := time.Now()
	result := SidecarResult{ScannedAt: now}
	result.Summary.TotalPods = len(pods.Items)

	typeStats := map[string]*SidecarTypeStat{}
	nsStats := map[string]*SidecarNSStat{}
	var totalCPUAll, totalMemAll int64

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		nsStat, ok := nsStats[pod.Namespace]
		if !ok {
			nsStat = &SidecarNSStat{Namespace: pod.Namespace}
			nsStats[pod.Namespace] = nsStat
		}
		nsStat.TotalPods++

		mainContainers := pod.Spec.Containers
		initContainers := pod.Spec.InitContainers

		// Identify sidecars: typically 2nd+ containers or known sidecar patterns
		hasSidecar := false
		podSidecarCount := 0
		podSidecarCPU := int64(0)
		podSidecarMem := int64(0)
		podTotalCPU := int64(0)
		podTotalMem := int64(0)
		var detectedTypes []string

		for i, c := range mainContainers {
			cCPU := c.Resources.Requests.Cpu().MilliValue()
			cMem := c.Resources.Requests.Memory().Value() / (1024 * 1024)
			podTotalCPU += cCPU
			podTotalMem += cMem

			if isSidecarContainer(&c, i, len(mainContainers)) {
				sidecarType := classifySidecar(c.Name, c.Image)
				hasSidecar = true
				podSidecarCount++
				podSidecarCPU += cCPU
				podSidecarMem += cMem
				detectedTypes = append(detectedTypes, sidecarType)

				// Update type stats
				ts, ok := typeStats[sidecarType]
				if !ok {
					ts = &SidecarTypeStat{Type: sidecarType}
					typeStats[sidecarType] = ts
				}
				ts.Count++
				ts.CPUMilli += cCPU
				ts.MemMi += cMem

				// Determine injection method
				if isInjectedSidecar(&pod, sidecarType) {
					ts.Detection = "injected"
				} else {
					ts.Detection = "manual"
				}
			}
		}

		// Also check init container sidecars (but don't double-count)
		for _, c := range initContainers {
			if isSidecarInitContainer(&c) {
				sidecarType := classifySidecar(c.Name, c.Image)
				if _, ok := typeStats[sidecarType]; !ok {
					typeStats[sidecarType] = &SidecarTypeStat{Type: sidecarType}
				}
			}
		}

		totalCPUAll += podTotalCPU
		totalMemAll += podTotalMem

		result.Summary.TotalContainers += len(mainContainers)

		if hasSidecar {
			result.Summary.PodsWithSidecars++
			result.Summary.TotalSidecars += podSidecarCount
			result.Summary.CPUTotalMilli += podSidecarCPU
			result.Summary.MemTotalMi += podSidecarMem
			nsStat.PodsWithSide++
			nsStat.SidecarCount += podSidecarCount

			cpuOverhead := 0.0
			if podTotalCPU > 0 {
				cpuOverhead = float64(podSidecarCPU) / float64(podTotalCPU) * 100
			}
			memOverhead := 0.0
			if podTotalMem > 0 {
				memOverhead = float64(podSidecarMem) / float64(podTotalMem) * 100
			}

			nsStat.CPUOverhead += cpuOverhead
			nsStat.MemOverhead += memOverhead

			// Check for injected sidecars
			isInjected := isInjectedSidecar(&pod, detectedTypes...)
			if isInjected {
				result.Summary.InjectedPods++
			} else {
				result.Summary.ManualSidecars++
			}

			// Flag high overhead
			if cpuOverhead > 30 || memOverhead > 30 {
				severity := "medium"
				if cpuOverhead > 50 || memOverhead > 50 {
					severity = "high"
				}
				sidecarType := "mixed"
				if len(detectedTypes) == 1 {
					sidecarType = detectedTypes[0]
				}
				result.HighOverhead = append(result.HighOverhead, SidecarEntry{
					PodName:        pod.Name,
					Namespace:      pod.Namespace,
					SidecarType:    sidecarType,
					CPUOverheadPct: float64(int(cpuOverhead*100)) / 100,
					MemOverheadPct: float64(int(memOverhead*100)) / 100,
					Severity:       severity,
				})
			}

			// Flag pods where ALL containers are sidecars (no app container)
			if podSidecarCount == len(mainContainers) && len(mainContainers) > 1 {
				result.InjectedOnly = append(result.InjectedOnly, SidecarEntry{
					PodName:     pod.Name,
					Namespace:   pod.Namespace,
					SidecarType: detectedTypes[0],
					Severity:    "high",
				})
			}
		}
	}

	// Compute summary ratios
	if result.Summary.TotalContainers > 0 {
		result.Summary.SidecarRatio = float64(int(float64(result.Summary.TotalSidecars)/float64(result.Summary.TotalContainers)*10000)) / 100
	}
	if totalCPUAll > 0 {
		result.Summary.CPUOverheadPct = float64(int(float64(result.Summary.CPUTotalMilli)/float64(totalCPUAll)*10000)) / 100
	}
	if totalMemAll > 0 {
		result.Summary.MemOverheadPct = float64(int(float64(result.Summary.MemTotalMi)/float64(totalMemAll)*10000)) / 100
	}

	// Build type stats
	for _, ts := range typeStats {
		result.ByType = append(result.ByType, *ts)
	}
	sort.Slice(result.ByType, func(i, j int) bool {
		return result.ByType[i].Count > result.ByType[j].Count
	})

	// Build namespace stats
	for _, ns := range nsStats {
		if ns.PodsWithSide > 0 {
			ns.CPUOverhead = float64(int(ns.CPUOverhead/float64(ns.PodsWithSide)*100)) / 100
			ns.MemOverhead = float64(int(ns.MemOverhead/float64(ns.PodsWithSide)*100)) / 100
		}
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].SidecarCount > result.ByNamespace[j].SidecarCount
	})

	// Sort high overhead by severity
	sort.Slice(result.HighOverhead, func(i, j int) bool {
		sevOrder := map[string]int{"high": 0, "medium": 1, "low": 2}
		return sevOrder[result.HighOverhead[i].Severity] < sevOrder[result.HighOverhead[j].Severity]
	})
	if len(result.HighOverhead) > 30 {
		result.HighOverhead = result.HighOverhead[:30]
	}

	result.Summary.HealthScore = sidecarScore(result.Summary)
	result.Recommendations = sidecarRecommendations(&result)

	writeJSON(w, result)
}

// knownSidecarPatterns maps container name/image patterns to sidecar types.
var knownSidecarPatterns = map[string]string{
	"istio-proxy":    "Istio Proxy",
	"istio-init":     "Istio Init",
	"vault-agent":    "Vault Agent",
	"vault":          "Vault Agent",
	"fluentd":        "Fluentd",
	"fluent-bit":     "Fluent Bit",
	"filebeat":       "Filebeat",
	"prometheus":     "Prometheus Exporter",
	"jaeger-agent":   "Jaeger Agent",
	"datadog":        "Datadog Agent",
	"linkerd-proxy":  "Linkerd Proxy",
	"linkerd-init":   "Linkerd Init",
	"envoy":          "Envoy Sidecar",
	"consul-connect": "Consul Connect",
	"aws-node":       "AWS VPC CNI",
	"calico-node":    "Calico Node",
	"kube-proxy":     "Kube Proxy",
	"nginx":          "Nginx Sidecar",
	"oauth2-proxy":   "OAuth2 Proxy",
	" ambassador":    "Ambassador",
}

// isSidecarContainer determines if a container is a sidecar (not the main app).
func isSidecarContainer(c *corev1.Container, index, total int) bool {
	// Heuristic: 2nd+ container in multi-container pod is likely a sidecar
	if total > 1 && index > 0 {
		return true
	}
	// Also check known sidecar patterns even for first container
	name := strings.ToLower(c.Name)
	for pattern := range knownSidecarPatterns {
		if strings.Contains(name, pattern) {
			return true
		}
	}
	image := strings.ToLower(c.Image)
	for pattern := range knownSidecarPatterns {
		if strings.Contains(image, pattern) {
			return true
		}
	}
	return false
}

// classifySidecar identifies the sidecar type from name/image.
func classifySidecar(name, image string) string {
	searchStr := strings.ToLower(name + " " + image)
	for pattern, sidecarType := range knownSidecarPatterns {
		if strings.Contains(searchStr, pattern) {
			return sidecarType
		}
	}
	// Generic sidecar
	if strings.Contains(searchStr, "sidecar") {
		return "Generic Sidecar"
	}
	return "Unknown Sidecar"
}

// isInjectedSidecar checks if the sidecar was auto-injected (vs manually added).
func isInjectedSidecar(pod *corev1.Pod, types ...string) bool {
	if pod.Annotations == nil {
		return false
	}
	for key := range pod.Annotations {
		if strings.Contains(key, "sidecar.istio.io") || strings.Contains(key, "linkerd.io") {
			return true
		}
	}
	// Check for common injection annotations
	for _, t := range types {
		if t == "Istio Proxy" || t == "Linkerd Proxy" {
			return true
		}
	}
	return false
}

// isSidecarInitContainer checks if an init container is a known sidecar init.
func isSidecarInitContainer(c *corev1.Container) bool {
	name := strings.ToLower(c.Name)
	for pattern := range knownSidecarPatterns {
		if strings.Contains(name, pattern) {
			return true
		}
	}
	return false
}

// sidecarScore computes a 0-100 health score (100 = minimal overhead).
func sidecarScore(s SidecarSummary) int {
	if s.TotalPods == 0 {
		return 100
	}

	score := 100

	// Penalize high CPU overhead
	if s.CPUOverheadPct > 30 {
		score -= min(25, int(s.CPUOverheadPct-30))
	}

	// Penalize high memory overhead
	if s.MemOverheadPct > 30 {
		score -= min(20, int(s.MemOverheadPct-30))
	}

	// Penalize injected-only pods (no app container)
	if s.InjectedPods > 0 {
		score -= min(15, s.InjectedPods*3)
	}

	if score < 0 {
		score = 0
	}
	return score
}

// sidecarRecommendations generates actionable recommendations.
func sidecarRecommendations(r *SidecarResult) []string {
	var recs []string

	if r.Summary.CPUOverheadPct > 20 {
		recs = append(recs, fmt.Sprintf(
			"Sidecars consume %.1f%% of total CPU requests — tune sidecar resource limits or evaluate if all sidecars are necessary",
			r.Summary.CPUOverheadPct,
		))
	}

	if r.Summary.MemOverheadPct > 20 {
		recs = append(recs, fmt.Sprintf(
			"Sidecars consume %.1f%% of total memory requests — review sidecar memory limits",
			r.Summary.MemOverheadPct,
		))
	}

	if len(r.HighOverhead) > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d pod(s) have >30%% sidecar resource overhead — consider right-sizing sidecar containers",
			len(r.HighOverhead),
		))
	}

	if len(r.InjectedOnly) > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d pod(s) have no app container (all containers are sidecars) — investigate injection misconfiguration",
			len(r.InjectedOnly),
		))
	}

	// Recommend eBPF-based alternatives for service mesh sidecars
	for _, ts := range r.ByType {
		if (ts.Type == "Istio Proxy" || ts.Type == "Linkerd Proxy") && ts.Count > 10 {
			recs = append(recs, fmt.Sprintf(
				"%d pods have %s sidecars — consider sidecarless alternatives (e.g., Istio Ambient Mode, Cilium Service Mesh) to reduce overhead",
				ts.Count, ts.Type,
			))
			break
		}
	}

	if len(recs) == 0 {
		recs = append(recs, "Sidecar configuration is healthy — overhead is within acceptable limits")
	}

	return recs
}
