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

// OperatorHealthResult is the cluster operator & OLM health audit.
type OperatorHealthResult struct {
	ScannedAt        time.Time             `json:"scannedAt"`
	Summary          OperatorHealthSummary `json:"summary"`
	ByNamespace      []OperatorNSStat      `json:"byNamespace"`
	Operators        []OperatorEntry       `json:"operators"`
	FailingOperators []OperatorEntry       `json:"failingOperators"`
	Risks            []OperatorRisk        `json:"risks"`
	Recommendations  []string              `json:"recommendations"`
	HealthScore      int                   `json:"healthScore"`
}

// OperatorHealthSummary aggregates operator health metrics.
type OperatorHealthSummary struct {
	TotalOperators    int      `json:"totalOperators"` // detected operator deployments
	OLMDetected       bool     `json:"olmDetected"`    // OLM installed
	OLMNamespaces     []string `json:"olmNamespaces"`  // namespaces with OLM
	HealthyOperators  int      `json:"healthyOperators"`
	DegradedOperators int      `json:"degradedOperators"`
	FailedOperators   int      `json:"failedOperators"`
	NoOperatorNS      int      `json:"noOperatorNS"` // operator deployments without dedicated namespace
	TotalPods         int      `json:"totalPods"`
	ReadyPods         int      `json:"readyPods"`
	CrashLoopPods     int      `json:"crashLoopPods"`
	HighRestartPods   int      `json:"highRestartPods"` // pods with >5 restarts
	StaleOperators    int      `json:"staleOperators"`  // no deployment update in 90d (via image age heuristic)
}

// OperatorNSStat per-namespace operator stats.
type OperatorNSStat struct {
	Namespace     string `json:"namespace"`
	OperatorCount int    `json:"operatorCount"`
	Healthy       int    `json:"healthy"`
	Degraded      int    `json:"degraded"`
	Failed        int    `json:"failed"`
	RiskLevel     string `json:"riskLevel"`
}

// OperatorEntry describes a detected operator.
type OperatorEntry struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Kind        string `json:"kind"`   // deployment, statefulset
	Status      string `json:"status"` // healthy, degraded, failed
	PodsReady   int    `json:"podsReady"`
	PodsTotal   int    `json:"podsTotal"`
	Restarts    int    `json:"restarts"`
	HasOLM      bool   `json:"hasOLM"` // managed by OLM
	OLMVersion  string `json:"olmVersion,omitempty"`
	Image       string `json:"image,omitempty"`
	LastRestart string `json:"lastRestart,omitempty"`
}

// OperatorRisk describes an operator-related risk.
type OperatorRisk struct {
	Namespace string `json:"namespace,omitempty"`
	Operator  string `json:"operator,omitempty"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// handleOperatorHealth audits cluster operator & OLM health.
// GET /api/operations/operator-health
func (s *Server) handleOperatorHealth(w http.ResponseWriter, r *http.Request) {
	result := OperatorHealthResult{
		ScannedAt: time.Now(),
	}

	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	// 1. Detect OLM by checking for OLM-specific namespaces and pods
	pods, err := rc.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to list pods: %v", err))
		return
	}

	olmDetected := false
	olmNamespaces := []string{}
	olmKeywords := []string{"olm", "operator-lifecycle-manager", "catalog", "marketplace", "operatorhub"}

	// Detect OLM namespaces
	olmNsMap := map[string]bool{}
	for _, pod := range pods.Items {
		podLower := strings.ToLower(pod.Name)
		nsLower := strings.ToLower(pod.Namespace)
		for _, kw := range olmKeywords {
			if strings.Contains(podLower, kw) || strings.Contains(nsLower, kw) {
				olmDetected = true
				if !olmNsMap[pod.Namespace] {
					olmNsMap[pod.Namespace] = true
					olmNamespaces = append(olmNamespaces, pod.Namespace)
				}
				break
			}
		}
	}
	result.Summary.OLMDetected = olmDetected
	result.Summary.OLMNamespaces = olmNamespaces

	// 2. Detect operator deployments
	// Operators are typically deployments with "operator" in name/labels, or in OLM namespaces
	operatorKeywords := []string{"operator", "controller", "manager"}
	deploymentMap := map[string]*OperatorEntry{} // key: namespace/name

	for _, pod := range pods.Items {
		podLower := strings.ToLower(pod.Name)
		labels := pod.Labels

		isOperator := false
		// Check pod name
		for _, kw := range operatorKeywords {
			if strings.Contains(podLower, kw) {
				isOperator = true
				break
			}
		}
		// Check labels
		if !isOperator {
			for k, v := range labels {
				labelLower := strings.ToLower(k + "=" + v)
				if strings.Contains(labelLower, "operator") || strings.Contains(labelLower, "managed-by=olm") {
					isOperator = true
					break
				}
			}
		}
		// Check if in OLM namespace
		if !isOperator && olmNsMap[pod.Namespace] {
			isOperator = true
		}

		if !isOperator {
			continue
		}

		// Try to extract operator base name (strip pod hash suffix)
		opName := pod.Name
		if idx := strings.LastIndex(opName, "-"); idx > 0 {
			// Check if last segment is a hash (all hex/numeric)
			lastSeg := opName[idx+1:]
			if isHashSegment(lastSeg) && idx > 0 {
				opName = opName[:idx]
				// Check again for ReplicaSet hash
				if idx2 := strings.LastIndex(opName, "-"); idx2 > 0 {
					lastSeg2 := opName[idx2+1:]
					if isHashSegment(lastSeg2) {
						opName = opName[:idx2]
					}
				}
			}
		}

		opKey := fmt.Sprintf("%s/%s", pod.Namespace, opName)
		if deploymentMap[opKey] == nil {
			deploymentMap[opKey] = &OperatorEntry{
				Name:      opName,
				Namespace: pod.Namespace,
				Kind:      "deployment",
				Status:    "healthy",
				HasOLM:    olmNsMap[pod.Namespace],
			}
		}
		entry := deploymentMap[opKey]
		entry.PodsTotal++

		result.Summary.TotalPods++
		if pod.Status.Phase == corev1.PodRunning {
			isReady := true
			for _, cs := range pod.Status.ContainerStatuses {
				if !cs.Ready {
					isReady = false
				}
				entry.Restarts += int(cs.RestartCount)
			}
			if isReady {
				entry.PodsReady++
				result.Summary.ReadyPods++
			}
		}

		// Check for CrashLoopBackOff
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Waiting != nil && cs.State.Waiting.Reason == "CrashLoopBackOff" {
				result.Summary.CrashLoopPods++
				entry.Status = "failed"
			}
		}

		// Check for high restart count
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.RestartCount > 5 {
				result.Summary.HighRestartPods++
				if entry.Status != "failed" {
					entry.Status = "degraded"
				}
			}
		}

		// Get image for display
		if entry.Image == "" && len(pod.Spec.Containers) > 0 {
			entry.Image = pod.Spec.Containers[0].Image
		}
	}

	// 3. Build operator entries and namespace stats
	nsStats := map[string]*OperatorNSStat{}
	for _, entry := range deploymentMap {
		result.Summary.TotalOperators++

		if entry.Status == "healthy" {
			result.Summary.HealthyOperators++
		} else if entry.Status == "degraded" {
			result.Summary.DegradedOperators++
			result.Risks = append(result.Risks, OperatorRisk{
				Namespace: entry.Namespace,
				Operator:  entry.Name,
				Issue:     fmt.Sprintf("Operator %s is degraded — high restart count detected", entry.Name),
				Severity:  "warning",
			})
		} else if entry.Status == "failed" {
			result.Summary.FailedOperators++
			result.FailingOperators = append(result.FailingOperators, *entry)
			result.Risks = append(result.Risks, OperatorRisk{
				Namespace: entry.Namespace,
				Operator:  entry.Name,
				Issue:     fmt.Sprintf("Operator %s is failing — CrashLoopBackOff detected", entry.Name),
				Severity:  "critical",
			})
		}

		// Check if operator is in a dedicated namespace
		nsLower := strings.ToLower(entry.Namespace)
		if !strings.Contains(nsLower, "operator") && !strings.Contains(nsLower, "olm") &&
			!strings.Contains(nsLower, "system") && !olmNsMap[entry.Namespace] {
			result.Summary.NoOperatorNS++
			result.Risks = append(result.Risks, OperatorRisk{
				Namespace: entry.Namespace,
				Operator:  entry.Name,
				Issue:     fmt.Sprintf("Operator %s runs in non-dedicated namespace %s — consider isolating operators", entry.Name, entry.Namespace),
				Severity:  "low",
			})
		}

		// Namespace stats
		if nsStats[entry.Namespace] == nil {
			nsStats[entry.Namespace] = &OperatorNSStat{Namespace: entry.Namespace, RiskLevel: "low"}
		}
		nsStats[entry.Namespace].OperatorCount++
		if entry.Status == "healthy" {
			nsStats[entry.Namespace].Healthy++
		} else if entry.Status == "degraded" {
			nsStats[entry.Namespace].Degraded++
		} else if entry.Status == "failed" {
			nsStats[entry.Namespace].Failed++
		}

		result.Operators = append(result.Operators, *entry)
	}

	// 4. Finalize namespace stats and risk levels
	for _, stat := range nsStats {
		if stat.Failed > 0 {
			stat.RiskLevel = "critical"
		} else if stat.Degraded > 0 {
			stat.RiskLevel = "medium"
		} else {
			stat.RiskLevel = "low"
		}
		result.ByNamespace = append(result.ByNamespace, *stat)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].OperatorCount > result.ByNamespace[j].OperatorCount
	})

	// Sort operators by status (failed first)
	sort.Slice(result.Operators, func(i, j int) bool {
		statusOrder := map[string]int{"failed": 0, "degraded": 1, "healthy": 2}
		return statusOrder[result.Operators[i].Status] < statusOrder[result.Operators[j].Status]
	})

	// 5. Calculate health score
	score := 100
	if result.Summary.FailedOperators > 0 {
		score -= 30
	}
	if result.Summary.DegradedOperators > 0 {
		score -= 15
	}
	if result.Summary.CrashLoopPods > 0 {
		score -= 10
	}
	if result.Summary.HighRestartPods > 0 {
		score -= 10
	}
	if !olmDetected && result.Summary.TotalOperators > 0 {
		result.Risks = append(result.Risks, OperatorRisk{
			Issue:    "Operators detected but no Operator Lifecycle Manager (OLM) found — consider installing OLM for centralized operator management",
			Severity: "low",
		})
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	// 6. Recommendations
	if result.Summary.FailedOperators > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d operator(s) are failing — investigate CrashLoopBackOff and check operator logs", result.Summary.FailedOperators))
	}
	if result.Summary.DegradedOperators > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d operator(s) have high restart counts — check resource limits and dependencies", result.Summary.DegradedOperators))
	}
	if result.Summary.NoOperatorNS > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d operator(s) run in non-dedicated namespaces — isolate operators for better security and management", result.Summary.NoOperatorNS))
	}
	if len(result.Recommendations) == 0 {
		result.Recommendations = append(result.Recommendations,
			"All operators are healthy — no failing or degraded operators detected")
	}

	writeJSON(w, result)
}

// isHashSegment checks if a string looks like a Kubernetes pod hash suffix.
func isHashSegment(s string) bool {
	if len(s) < 5 {
		return false
	}
	for _, c := range s {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}
