package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v19.60 — Operations Dimension (Round 13)
// 1. Quota Waste Detector — namespace quota allocation vs actual usage gap
// 2. Admission Controller Health — MutatingWebhook & ValidatingWebhook config health
// 3. Cluster Clock Sync — node clock skew & time synchronization health
// ============================================================

// ---------------------------------------------------------------
// 1. Quota Waste Detector
// ---------------------------------------------------------------

type QuotaWasteResult1960 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         QuotaWasteSummary1960 `json:"summary"`
	WastefulNS      []QuotaWasteEntry1960 `json:"wastefulNamespaces"`
	HealthyNS       []QuotaWasteEntry1960 `json:"healthyNamespaces"`
	Recommendations []string              `json:"recommendations"`
}

type QuotaWasteSummary1960 struct {
	TotalNamespaces    int     `json:"totalNamespaces"`
	WithQuota          int     `json:"namespacesWithQuota"`
	WastefulNamespaces int     `json:"wastefulNamespaces"`
	TotalWastedCPU     float64 `json:"totalWastedCPU"`
	TotalWastedMem     float64 `json:"totalWastedMemGB"`
	AvgUtilizationPct  float64 `json:"avgQuotaUtilizationPct"`
}

type QuotaWasteEntry1960 struct {
	Namespace      string  `json:"namespace"`
	QuotaCPU       float64 `json:"quotaCPU"`
	UsedCPU        float64 `json:"usedCPU"`
	WastedCPU      float64 `json:"wastedCPU"`
	QuotaMem       float64 `json:"quotaMemGB"`
	UsedMem        float64 `json:"usedMemGB"`
	WastedMem      float64 `json:"wastedMemGB"`
	UtilizationPct float64 `json:"utilizationPct"`
}

func (s *Server) handleQuotaWasteDetector(w http.ResponseWriter, r *http.Request) {
	result := QuotaWasteResult1960{ScannedAt: time.Now()}
	score := 100

	// Get all ResourceQuotas
	quotaList, _ := s.clientset.CoreV1().ResourceQuotas("").List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	// Build per-namespace actual usage
	nsUsage := make(map[string]*QuotaWasteEntry1960)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodPending {
			continue
		}
		ns, ok := nsUsage[pod.Namespace]
		if !ok {
			ns = &QuotaWasteEntry1960{Namespace: pod.Namespace}
			nsUsage[pod.Namespace] = ns
		}
		for _, c := range pod.Spec.Containers {
			ns.UsedCPU += c.Resources.Requests.Cpu().AsApproximateFloat64()
			ns.UsedMem += float64(c.Resources.Requests.Memory().Value()) / (1024 * 1024 * 1024)
		}
	}

	// Compare with quota
	var totalUtil float64
	var countWithQuota int
	for _, rq := range quotaList.Items {
		ns, ok := nsUsage[rq.Namespace]
		if !ok {
			ns = &QuotaWasteEntry1960{Namespace: rq.Namespace}
		}

		// Extract hard limits for CPU and memory
		if cpu := rq.Status.Hard.Cpu(); cpu != nil {
			ns.QuotaCPU = cpu.AsApproximateFloat64()
		}
		if mem := rq.Status.Hard.Memory(); mem != nil {
			ns.QuotaMem = float64(mem.Value()) / (1024 * 1024 * 1024)
		}

		if ns.QuotaCPU <= 0 && ns.QuotaMem <= 0 {
			continue
		}

		ns.WastedCPU = ns.QuotaCPU - ns.UsedCPU
		if ns.WastedCPU < 0 {
			ns.WastedCPU = 0
		}
		ns.WastedMem = ns.QuotaMem - ns.UsedMem
		if ns.WastedMem < 0 {
			ns.WastedMem = 0
		}

		// Utilization percentage
		util := 0.0
		if ns.QuotaCPU > 0 {
			util = ns.UsedCPU / ns.QuotaCPU * 100
		}
		if ns.QuotaMem > 0 {
			memUtil := ns.UsedMem / ns.QuotaMem * 100
			if memUtil > util {
				util = memUtil
			}
		}
		ns.UtilizationPct = util

		countWithQuota++
		totalUtil += util

		result.Summary.TotalWastedCPU += ns.WastedCPU
		result.Summary.TotalWastedMem += ns.WastedMem

		// Wasteful = using < 40% of quota with significant allocation
		if util < 40 && (ns.QuotaCPU > 1 || ns.QuotaMem > 2) {
			result.WastefulNS = append(result.WastefulNS, *ns)
			result.Summary.WastefulNamespaces++
			score -= 3
		} else {
			result.HealthyNS = append(result.HealthyNS, *ns)
		}
	}

	result.Summary.WithQuota = countWithQuota
	if countWithQuota > 0 {
		result.Summary.AvgUtilizationPct = totalUtil / float64(countWithQuota)
	}

	// Count all namespaces
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	result.Summary.TotalNamespaces = len(nsList.Items)

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	sort.Slice(result.WastefulNS, func(i, j int) bool {
		return result.WastefulNS[i].UtilizationPct < result.WastefulNS[j].UtilizationPct
	})

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("Quota utilization avg: %.1f%% across %d namespaces", result.Summary.AvgUtilizationPct, result.Summary.WithQuota))
	if result.Summary.WastefulNamespaces > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d namespaces wasting quota: %.1f CPU cores, %.1f GB memory", result.Summary.WastefulNamespaces, result.Summary.TotalWastedCPU, result.Summary.TotalWastedMem))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Admission Controller Health
// ---------------------------------------------------------------

type AdmissionHealthResult1960 struct {
	ScannedAt       time.Time                   `json:"scannedAt"`
	HealthScore     int                         `json:"healthScore"`
	Grade           string                      `json:"grade"`
	Summary         AdmissionHealthSummary1960  `json:"summary"`
	Webhooks        []AdmissionWebhookEntry1960 `json:"webhooks"`
	Issues          []AdmissionIssue1960        `json:"issues"`
	Recommendations []string                    `json:"recommendations"`
}

type AdmissionHealthSummary1960 struct {
	TotalMutatingWebhooks   int `json:"totalMutatingWebhooks"`
	TotalValidatingWebhooks int `json:"totalValidatingWebhooks"`
	HealthyWebhooks         int `json:"healthyWebhooks"`
	MisconfiguredWebhooks   int `json:"misconfiguredWebhooks"`
	FailurePolicyFail       int `json:"failurePolicyFail"`
	WithTimeoutBelow5s      int `json:"withTimeoutBelow5s"`
	WithNoNamespaceFilter   int `json:"withNoNamespaceFilter"`
}

type AdmissionWebhookEntry1960 struct {
	Name             string `json:"name"`
	Kind             string `json:"kind"`
	Namespace        string `json:"namespace"`
	FailurePolicy    string `json:"failurePolicy"`
	TimeoutSeconds   int32  `json:"timeoutSeconds"`
	HasCABundle      bool   `json:"hasCABundle"`
	ServiceAvailable bool   `json:"serviceAvailable"`
	IsClusterScoped  bool   `json:"isClusterScoped"`
}

type AdmissionIssue1960 struct {
	Name      string `json:"name"`
	IssueType string `json:"issueType"`
	Severity  string `json:"severity"`
	Detail    string `json:"detail"`
}

func (s *Server) handleAdmissionHealth(w http.ResponseWriter, r *http.Request) {
	result := AdmissionHealthResult1960{ScannedAt: time.Now()}
	score := 100

	// MutatingWebhookConfigurations
	mwcList, _ := s.clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().List(r.Context(), metav1.ListOptions{})
	result.Summary.TotalMutatingWebhooks = len(mwcList.Items)

	for _, mwc := range mwcList.Items {
		for _, wh := range mwc.Webhooks {
			entry := AdmissionWebhookEntry1960{
				Name: mwc.Name,
				Kind: "MutatingWebhookConfiguration",
			}
			entry.FailurePolicy = "Ignore"
			if wh.FailurePolicy != nil {
				entry.FailurePolicy = string(*wh.FailurePolicy)
			}
			entry.TimeoutSeconds = 10
			if wh.TimeoutSeconds != nil {
				entry.TimeoutSeconds = int32(*wh.TimeoutSeconds)
			}
			entry.HasCABundle = len(wh.ClientConfig.CABundle) > 0
			entry.IsClusterScoped = len(wh.NamespaceSelector.MatchLabels) == 0 && len(wh.NamespaceSelector.MatchExpressions) == 0

			// Check for issues
			if wh.FailurePolicy != nil && *wh.FailurePolicy == "Fail" {
				result.Summary.FailurePolicyFail++
				result.Issues = append(result.Issues, AdmissionIssue1960{
					Name: mwc.Name, IssueType: "fail-policy",
					Severity: "medium",
					Detail:   "Webhook with FailurePolicy=Fail can block all admissions if webhook is down",
				})
				score -= 2
			}
			if wh.TimeoutSeconds != nil && *wh.TimeoutSeconds < 5 {
				result.Summary.WithTimeoutBelow5s++
				result.Issues = append(result.Issues, AdmissionIssue1960{
					Name: mwc.Name, IssueType: "low-timeout",
					Severity: "low",
					Detail:   fmt.Sprintf("Webhook timeout %ds is very aggressive", *wh.TimeoutSeconds),
				})
			}
			if entry.IsClusterScoped {
				result.Summary.WithNoNamespaceFilter++
			}
			if !entry.HasCABundle {
				result.Issues = append(result.Issues, AdmissionIssue1960{
					Name: mwc.Name, IssueType: "missing-ca-bundle",
					Severity: "high",
					Detail:   "Webhook missing CA bundle — TLS verification will fail",
				})
				score -= 5
				result.Summary.MisconfiguredWebhooks++
			} else {
				result.Summary.HealthyWebhooks++
			}

			result.Webhooks = append(result.Webhooks, entry)
		}
	}

	// ValidatingWebhookConfigurations
	vwcList, _ := s.clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().List(r.Context(), metav1.ListOptions{})
	result.Summary.TotalValidatingWebhooks = len(vwcList.Items)

	for _, vwc := range vwcList.Items {
		for _, wh := range vwc.Webhooks {
			entry := AdmissionWebhookEntry1960{
				Name: vwc.Name,
				Kind: "ValidatingWebhookConfiguration",
			}
			entry.FailurePolicy = "Ignore"
			if wh.FailurePolicy != nil {
				entry.FailurePolicy = string(*wh.FailurePolicy)
			}
			entry.TimeoutSeconds = 10
			if wh.TimeoutSeconds != nil {
				entry.TimeoutSeconds = int32(*wh.TimeoutSeconds)
			}
			entry.HasCABundle = len(wh.ClientConfig.CABundle) > 0

			if wh.FailurePolicy != nil && *wh.FailurePolicy == "Fail" {
				result.Summary.FailurePolicyFail++
			}
			if !entry.HasCABundle {
				result.Issues = append(result.Issues, AdmissionIssue1960{
					Name: vwc.Name, IssueType: "missing-ca-bundle",
					Severity: "high",
					Detail:   "Webhook missing CA bundle — TLS verification will fail",
				})
				score -= 5
				result.Summary.MisconfiguredWebhooks++
			} else {
				result.Summary.HealthyWebhooks++
			}

			result.Webhooks = append(result.Webhooks, entry)
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d mutating + %d validating webhooks, %d healthy", result.Summary.TotalMutatingWebhooks, result.Summary.TotalValidatingWebhooks, result.Summary.HealthyWebhooks))
	if result.Summary.FailurePolicyFail > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d webhooks with FailurePolicy=Fail — ensure webhook service is highly available", result.Summary.FailurePolicyFail))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Cluster Clock Sync
// ---------------------------------------------------------------

type ClockSyncResult1960 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         ClockSyncSummary1960     `json:"summary"`
	Nodes           []ClockSyncNodeEntry1960 `json:"nodes"`
	Risks           []ClockSyncRisk1960      `json:"risks"`
	Recommendations []string                 `json:"recommendations"`
}

type ClockSyncSummary1960 struct {
	TotalNodes     int     `json:"totalNodes"`
	SyncedNodes    int     `json:"syncedNodes"`
	SkewedNodes    int     `json:"skewedNodes"`
	MaxSkewSeconds float64 `json:"maxSkewSeconds"`
	AvgSkewSeconds float64 `json:"avgSkewSeconds"`
	HasNTPLabel    int     `json:"nodesWithNTPLabel"`
}

type ClockSyncNodeEntry1960 struct {
	Name        string  `json:"name"`
	SkewSeconds float64 `json:"skewSeconds"`
	Status      string  `json:"status"`
	HasNTPLbl   bool    `json:"hasNTPLabel"`
}

type ClockSyncRisk1960 struct {
	Node     string `json:"node"`
	RiskType string `json:"riskType"`
	Severity string `json:"severity"`
	Detail   string `json:"detail"`
}

func (s *Server) handleClockSync(w http.ResponseWriter, r *http.Request) {
	result := ClockSyncResult1960{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	now := time.Now()

	var totalSkew float64
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++

		// Calculate clock skew from node lease/heartbeat
		// Node conditions HeartbeatTime indicates last kubelet status update
		var lastHeartbeat time.Time
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady {
				lastHeartbeat = cond.LastHeartbeatTime.Time
				break
			}
		}

		skew := 0.0
		status := "synced"
		if !lastHeartbeat.IsZero() {
			skew = now.Sub(lastHeartbeat).Seconds()
			if skew < 0 {
				skew = -skew
			}
			// Heartbeat skew includes network + processing delay, not pure clock skew
			// But large skew indicates either clock drift or kubelet issues
			if skew > 60 {
				status = "skewed"
				result.Summary.SkewedNodes++
				severity := "medium"
				if skew > 300 {
					severity = "high"
				}
				result.Risks = append(result.Risks, ClockSyncRisk1960{
					Node: node.Name, RiskType: "clock-skew",
					Severity: severity,
					Detail:   fmt.Sprintf("Node heartbeat %.0fs from server time — possible clock drift or kubelet lag", skew),
				})
				if severity == "high" {
					score -= 10
				} else {
					score -= 5
				}
			} else {
				result.Summary.SyncedNodes++
			}
		} else {
			status = "unknown"
		}

		hasNTP := false
		if _, ok := node.Labels["ntp-synced"]; ok {
			hasNTP = true
			result.Summary.HasNTPLabel++
		}

		result.Nodes = append(result.Nodes, ClockSyncNodeEntry1960{
			Name: node.Name, SkewSeconds: skew,
			Status: status, HasNTPLbl: hasNTP,
		})

		if skew > result.Summary.MaxSkewSeconds {
			result.Summary.MaxSkewSeconds = skew
		}
		totalSkew += skew
	}

	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgSkewSeconds = totalSkew / float64(result.Summary.TotalNodes)
	}

	// Sort by skew descending
	sort.Slice(result.Nodes, func(i, j int) bool {
		return result.Nodes[i].SkewSeconds > result.Nodes[j].SkewSeconds
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("Clock sync: %d/%d nodes synced, max skew %.0fs", result.Summary.SyncedNodes, result.Summary.TotalNodes, result.Summary.MaxSkewSeconds))
	if result.Summary.SkewedNodes > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d nodes with significant clock skew — verify NTP/chrony configuration", result.Summary.SkewedNodes))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
