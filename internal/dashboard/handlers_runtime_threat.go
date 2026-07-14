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

// RuntimeThreatResult is the runtime threat detection & container anomaly audit.
type RuntimeThreatResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Summary         RuntimeThreatSummary `json:"summary"`
	Detectors       []RuntimeDetector    `json:"detectors"`
	AnomalousPods   []AnomalousPod       `json:"anomalousPods"`
	Gaps            []RuntimeThreatGap   `json:"gaps"`
	Recommendations []string             `json:"recommendations"`
	HealthScore     int                  `json:"healthScore"`
}

// RuntimeThreatSummary aggregates runtime threat statistics.
type RuntimeThreatSummary struct {
	HasFalco              bool `json:"hasFalco"`
	HasTracee             bool `json:"hasTracee"`
	HasTetragon           bool `json:"hasTetragon"`
	HasCilium             bool `json:"hasCilium"` // Cilium runtime security
	TotalDetectors        int  `json:"totalDetectors"`
	HealthyDetectors      int  `json:"healthyDetectors"`
	NamespacesWithRuntime int  `json:"namespacesWithRuntime"` // namespaces with runtime security
	NamespacesWithout     int  `json:"namespacesWithout"`
	PodsWithAnomaly       int  `json:"podsWithAnomaly"`
	HighRestartPods       int  `json:"highRestartPods"`
	PrivilegedPods        int  `json:"privilegedPods"` // pods with privileged containers (runtime risk)
}

// RuntimeDetector describes a runtime security tool deployment.
type RuntimeDetector struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"` // DaemonSet or Deployment
	Image     string `json:"image"`
	Ready     int    `json:"ready"`
	Desired   int    `json:"desired"`
	Status    string `json:"status"`
	Type      string `json:"type"` // falco, tracee, tetragon, cilium
}

// AnomalousPod describes a pod with runtime anomalies.
type AnomalousPod struct {
	Namespace string   `json:"namespace"`
	PodName   string   `json:"podName"`
	OwnerKind string   `json:"ownerKind"`
	OwnerName string   `json:"ownerName"`
	Anomalies []string `json:"anomalies"`
	Severity  string   `json:"severity"`
}

// RuntimeThreatGap describes a gap in runtime security coverage.
type RuntimeThreatGap struct {
	Namespace string `json:"namespace"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// handleRuntimeThreat audits runtime threat detection & container anomalies.
// GET /api/security/runtime-threat
func (s *Server) handleRuntimeThreat(w http.ResponseWriter, r *http.Request) {
	result := RuntimeThreatResult{
		ScannedAt: time.Now(),
	}

	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	systemNamespaces := map[string]bool{
		"kube-system":     true,
		"kube-public":     true,
		"kube-node-lease": true,
	}

	// Known runtime security tool images/names
	runtimeTools := map[string]string{
		"falco":    "falco",
		"tracee":   "tracee",
		"tetragon": "tetragon",
		"cilium":   "cilium",
		"aqua":     "aquasec",
		"sysdig":   "sysdig",
		"aqua-sec": "aquasec",
	}

	// 1. Check for runtime security tool DaemonSets/Deployments
	daemonsets, err := rc.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, ds := range daemonsets.Items {
			toolType := ""
			imageName := ""
			if len(ds.Spec.Template.Spec.Containers) > 0 {
				imageName = ds.Spec.Template.Spec.Containers[0].Image
				for keyword, tt := range runtimeTools {
					if strings.Contains(strings.ToLower(imageName), keyword) || strings.Contains(strings.ToLower(ds.Name), keyword) {
						toolType = tt
						break
					}
				}
			}
			if toolType == "" {
				continue
			}

			status := "healthy"
			if ds.Status.NumberReady < ds.Status.DesiredNumberScheduled {
				status = "degraded"
			}

			result.Detectors = append(result.Detectors, RuntimeDetector{
				Name:      ds.Name,
				Namespace: ds.Namespace,
				Kind:      "DaemonSet",
				Image:     imageName,
				Ready:     int(ds.Status.NumberReady),
				Desired:   int(ds.Status.DesiredNumberScheduled),
				Status:    status,
				Type:      toolType,
			})
			result.Summary.TotalDetectors++
			if status == "healthy" {
				result.Summary.HealthyDetectors++
			}

			switch toolType {
			case "falco":
				result.Summary.HasFalco = true
			case "tracee":
				result.Summary.HasTracee = true
			case "tetragon":
				result.Summary.HasTetragon = true
			case "cilium":
				result.Summary.HasCilium = true
			}
		}
	}

	deployments, err := rc.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, dep := range deployments.Items {
			toolType := ""
			imageName := ""
			if len(dep.Spec.Template.Spec.Containers) > 0 {
				imageName = dep.Spec.Template.Spec.Containers[0].Image
				for keyword, tt := range runtimeTools {
					if strings.Contains(strings.ToLower(imageName), keyword) || strings.Contains(strings.ToLower(dep.Name), keyword) {
						toolType = tt
						break
					}
				}
			}
			if toolType == "" {
				continue
			}

			desired := 0
			if dep.Spec.Replicas != nil {
				desired = int(*dep.Spec.Replicas)
			}
			status := "healthy"
			if int(dep.Status.ReadyReplicas) < desired {
				status = "degraded"
			}

			result.Detectors = append(result.Detectors, RuntimeDetector{
				Name:      dep.Name,
				Namespace: dep.Namespace,
				Kind:      "Deployment",
				Image:     imageName,
				Ready:     int(dep.Status.ReadyReplicas),
				Desired:   desired,
				Status:    status,
				Type:      toolType,
			})
			result.Summary.TotalDetectors++
			if status == "healthy" {
				result.Summary.HealthyDetectors++
			}

			switch toolType {
			case "falco":
				result.Summary.HasFalco = true
			case "tracee":
				result.Summary.HasTracee = true
			case "tetragon":
				result.Summary.HasTetragon = true
			case "cilium":
				result.Summary.HasCilium = true
			}
		}
	}

	// 2. Check pods for runtime anomalies (high restarts, privileged containers)
	pods, err := rc.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		nsWithDetectors := make(map[string]bool)
		if result.Summary.TotalDetectors > 0 {
			// If we have runtime detectors running as DaemonSets, they cover all namespaces
			for _, d := range result.Detectors {
				if d.Kind == "DaemonSet" && d.Status == "healthy" {
					nsWithDetectors[d.Namespace] = true
				}
			}
		}

		nsPodCount := make(map[string]int)
		for _, pod := range pods.Items {
			if systemNamespaces[pod.Namespace] {
				continue
			}
			if pod.Status.Phase != corev1.PodRunning {
				continue
			}
			nsPodCount[pod.Namespace]++

			ownerKind := getOwnerKind(pod.OwnerReferences)
			ownerName := getOwnerName(pod.OwnerReferences)

			anomalies := []string{}
			severity := "low"

			// Check for high restart count
			for _, cs := range pod.Status.ContainerStatuses {
				if cs.RestartCount > 5 {
					anomalies = append(anomalies, fmt.Sprintf("Container %s has %d restarts", cs.Name, cs.RestartCount))
					result.Summary.HighRestartPods++
					if severity == "low" {
						severity = "medium"
					}
				}
				if cs.LastTerminationState.Terminated != nil {
					reason := cs.LastTerminationState.Terminated.Reason
					if reason == "OOMKilled" {
						anomalies = append(anomalies, fmt.Sprintf("Container %s was OOMKilled", cs.Name))
						if severity == "low" {
							severity = "medium"
						}
					}
				}
			}

			// Check for privileged containers (runtime security risk)
			for _, c := range pod.Spec.Containers {
				if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
					anomalies = append(anomalies, fmt.Sprintf("Container %s is privileged", c.Name))
					result.Summary.PrivilegedPods++
					severity = "high"
				}
			}

			if len(anomalies) > 0 {
				result.Summary.PodsWithAnomaly++
				result.AnomalousPods = append(result.AnomalousPods, AnomalousPod{
					Namespace: pod.Namespace,
					PodName:   pod.Name,
					OwnerKind: ownerKind,
					OwnerName: ownerName,
					Anomalies: anomalies,
					Severity:  severity,
				})
			}
		}

		// Check namespace coverage
		for ns, podCount := range nsPodCount {
			if podCount == 0 {
				continue
			}
			if result.Summary.TotalDetectors == 0 {
				result.Summary.NamespacesWithout++
				result.Gaps = append(result.Gaps, RuntimeThreatGap{
					Namespace: ns,
					Issue:     "No runtime threat detection tool installed",
					Severity:  "high",
				})
			} else if !nsWithDetectors[ns] && result.Summary.TotalDetectors > 0 {
				// DaemonSet-based tools cover all namespaces
				// Only flag if no DaemonSet detector exists
				hasDaemonSet := false
				for _, d := range result.Detectors {
					if d.Kind == "DaemonSet" && d.Status == "healthy" {
						hasDaemonSet = true
						break
					}
				}
				if !hasDaemonSet {
					result.Summary.NamespacesWithout++
				} else {
					result.Summary.NamespacesWithRuntime++
				}
			} else {
				result.Summary.NamespacesWithRuntime++
			}
		}
	}

	// Sort results
	sort.Slice(result.AnomalousPods, func(i, j int) bool {
		return result.AnomalousPods[i].Severity > result.AnomalousPods[j].Severity
	})
	sort.Slice(result.Gaps, func(i, j int) bool {
		return result.Gaps[i].Severity > result.Gaps[j].Severity
	})

	// Recommendations
	if result.Summary.TotalDetectors == 0 {
		result.Recommendations = append(result.Recommendations,
			"No runtime threat detection tool installed — install Falco, Tracee, or Tetragon for runtime security monitoring")
	}
	if result.Summary.PrivilegedPods > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pods run privileged containers — runtime security risk, remove privileged mode", result.Summary.PrivilegedPods))
	}
	if result.Summary.HighRestartPods > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pods have high restart counts — investigate application stability and potential crashes", result.Summary.HighRestartPods))
	}
	if result.Summary.TotalDetectors > 0 && result.Summary.HealthyDetectors < result.Summary.TotalDetectors {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d runtime detectors are degraded — check pod health and resource limits", result.Summary.TotalDetectors-result.Summary.HealthyDetectors))
	}

	// Health score
	score := 100
	if result.Summary.TotalDetectors == 0 {
		score -= 30
	}
	score -= result.Summary.PrivilegedPods * 5
	score -= result.Summary.HighRestartPods * 2
	if result.Summary.TotalDetectors > 0 && result.Summary.HealthyDetectors < result.Summary.TotalDetectors {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	writeJSON(w, result)
}
