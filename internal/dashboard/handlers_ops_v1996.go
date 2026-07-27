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
// v19.96 — Operations Dimension (Round 19)
// 1. Node Pressure Budget — resource headroom per node
// 2. Event Budget — event rate & warning distribution per NS
// 3. Net Policy Budget — network policy enforcement coverage
// ============================================================

// ---------------------------------------------------------------
// 1. Node Pressure Budget
// ---------------------------------------------------------------

type NodeBudgetResult1996 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         NodeBudgetSummary1996 `json:"summary"`
	PerNode         []NodeBudgetEntry1996 `json:"perNode"`
	Recommendations []string              `json:"recommendations"`
}

type NodeBudgetSummary1996 struct {
	TotalNodes    int     `json:"totalNodes"`
	CPUHeadroom   float64 `json:"cpuHeadroom"`
	MemHeadroom   float64 `json:"memHeadroom"`
	PressureLevel string  `json:"pressureLevel"`
}

type NodeBudgetEntry1996 struct {
	Name     string  `json:"name"`
	CPUUtil  float64 `json:"cpuUtilPct"`
	MemUtil  float64 `json:"memUtilPct"`
	Pressure string  `json:"pressure"`
}

func (s *Server) handleNodePressureBudget(w http.ResponseWriter, r *http.Request) {
	result := NodeBudgetResult1996{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	nodeReq := make(map[string]struct{ cpu, mem float64 })
	for _, pod := range podList.Items {
		if (pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodPending) || pod.Spec.NodeName == "" {
			continue
		}
		req, ok := nodeReq[pod.Spec.NodeName]
		if !ok {
			req = struct{ cpu, mem float64 }{}
		}
		for _, c := range pod.Spec.Containers {
			req.cpu += c.Resources.Requests.Cpu().AsApproximateFloat64()
			req.mem += float64(c.Resources.Requests.Memory().Value()) / (1024 * 1024 * 1024)
		}
		nodeReq[pod.Spec.NodeName] = req
	}

	var totalCPUHead, totalMemHead float64
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++

		allocCPU := node.Status.Allocatable.Cpu().AsApproximateFloat64()
		allocMem := float64(node.Status.Allocatable.Memory().Value()) / (1024 * 1024 * 1024)
		req := nodeReq[node.Name]

		cpuUtil := 0.0
		memUtil := 0.0
		if allocCPU > 0 {
			cpuUtil = req.cpu / allocCPU * 100
		}
		if allocMem > 0 {
			memUtil = req.mem / allocMem * 100
		}

		maxUtil := cpuUtil
		if memUtil > maxUtil {
			maxUtil = memUtil
		}

		pressure := "low"
		if maxUtil > 85 {
			pressure = "critical"
			score -= 5
		} else if maxUtil > 70 {
			pressure = "high"
			score -= 2
		} else if maxUtil > 50 {
			pressure = "medium"
		}

		result.PerNode = append(result.PerNode, NodeBudgetEntry1996{
			Name: node.Name, CPUUtil: cpuUtil, MemUtil: memUtil, Pressure: pressure,
		})

		totalCPUHead += 100 - cpuUtil
		totalMemHead += 100 - memUtil
	}

	if result.Summary.TotalNodes > 0 {
		result.Summary.CPUHeadroom = totalCPUHead / float64(result.Summary.TotalNodes)
		result.Summary.MemHeadroom = totalMemHead / float64(result.Summary.TotalNodes)
	}

	if result.Summary.CPUHeadroom < 15 || result.Summary.MemHeadroom < 15 {
		result.Summary.PressureLevel = "critical"
	} else if result.Summary.CPUHeadroom < 30 || result.Summary.MemHeadroom < 30 {
		result.Summary.PressureLevel = "high"
	} else {
		result.Summary.PressureLevel = "low"
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d nodes, CPU headroom %.0f%%, Mem headroom %.0f%%, pressure: %s", result.Summary.TotalNodes, result.Summary.CPUHeadroom, result.Summary.MemHeadroom, result.Summary.PressureLevel))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Event Budget
// ---------------------------------------------------------------

type EventBudgetResult1996 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         EventBudgetSummary1996   `json:"summary"`
	TopReasons      []EventBudgetEntry1996   `json:"topReasons"`
	PerNS           []EventBudgetNSEntry1996 `json:"perNamespace"`
	Recommendations []string                 `json:"recommendations"`
}

type EventBudgetSummary1996 struct {
	TotalEvents  int     `json:"totalEvents"`
	WarningRatio float64 `json:"warningRatio"`
	NormalRatio  float64 `json:"normalRatio"`
	TopReason    string  `json:"topReason"`
}

type EventBudgetEntry1996 struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
	Type   string `json:"type"`
}

type EventBudgetNSEntry1996 struct {
	Namespace  string  `json:"namespace"`
	EventCount int     `json:"eventCount"`
	WarningPct float64 `json:"warningPct"`
}

func (s *Server) handleEventBudget(w http.ResponseWriter, r *http.Request) {
	result := EventBudgetResult1996{ScannedAt: time.Now()}
	score := 100

	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})

	reasonMap := make(map[string]*EventBudgetEntry1996)
	nsStats := make(map[string]*EventBudgetNSEntry1996)
	warningCount := 0

	for _, evt := range eventList.Items {
		result.Summary.TotalEvents++

		if evt.Type == "Warning" {
			warningCount++
		}

		entry, ok := reasonMap[evt.Reason]
		if !ok {
			entry = &EventBudgetEntry1996{Reason: evt.Reason, Type: evt.Type}
			reasonMap[evt.Reason] = entry
		}
		entry.Count++

		ns, ok := nsStats[evt.Namespace]
		if !ok {
			ns = &EventBudgetNSEntry1996{Namespace: evt.Namespace}
			nsStats[evt.Namespace] = ns
		}
		ns.EventCount++
		if evt.Type == "Warning" {
			ns.WarningPct += 1
		}
	}

	if result.Summary.TotalEvents > 0 {
		result.Summary.WarningRatio = float64(warningCount) / float64(result.Summary.TotalEvents)
		result.Summary.NormalRatio = 1 - result.Summary.WarningRatio
	}

	for _, e := range reasonMap {
		result.TopReasons = append(result.TopReasons, *e)
	}
	sort.Slice(result.TopReasons, func(i, j int) bool {
		return result.TopReasons[i].Count > result.TopReasons[j].Count
	})
	if len(result.TopReasons) > 15 {
		result.TopReasons = result.TopReasons[:15]
	}
	if len(result.TopReasons) > 0 {
		result.Summary.TopReason = result.TopReasons[0].Reason
	}

	for _, ns := range nsStats {
		if ns.EventCount > 0 {
			ns.WarningPct = ns.WarningPct / float64(ns.EventCount) * 100
		}
		result.PerNS = append(result.PerNS, *ns)
	}
	sort.Slice(result.PerNS, func(i, j int) bool {
		return result.PerNS[i].EventCount > result.PerNS[j].EventCount
	})

	if result.Summary.WarningRatio > 0.3 {
		score -= 5
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d events (%.0f%% warning), top reason: %s", result.Summary.TotalEvents, result.Summary.WarningRatio*100, result.Summary.TopReason))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Net Policy Budget
// ---------------------------------------------------------------

type NetPolicyBudgetResult1996 struct {
	ScannedAt       time.Time                  `json:"scannedAt"`
	HealthScore     int                        `json:"healthScore"`
	Grade           string                     `json:"grade"`
	Summary         NetPolicyBudgetSummary1996 `json:"summary"`
	PerNS           []NetPolicyBudgetEntry1996 `json:"perNamespace"`
	Recommendations []string                   `json:"recommendations"`
}

type NetPolicyBudgetSummary1996 struct {
	TotalNamespaces int `json:"totalNamespaces"`
	WithDefaultDeny int `json:"withDefaultDeny"`
	Unprotected     int `json:"unprotected"`
	TotalPolicies   int `json:"totalPolicies"`
}

type NetPolicyBudgetEntry1996 struct {
	Namespace      string `json:"namespace"`
	HasEgressDeny  bool   `json:"hasEgressDeny"`
	HasIngressDeny bool   `json:"hasIngressDeny"`
	PolicyCount    int    `json:"policyCount"`
}

func (s *Server) handleNetPolicyBudget(w http.ResponseWriter, r *http.Request) {
	result := NetPolicyBudgetResult1996{ScannedAt: time.Now()}
	score := 100

	npList, _ := s.clientset.NetworkingV1().NetworkPolicies("").List(r.Context(), metav1.ListOptions{})
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})

	nsStats := make(map[string]*NetPolicyBudgetEntry1996)
	for _, ns := range nsList.Items {
		nsStats[ns.Name] = &NetPolicyBudgetEntry1996{Namespace: ns.Name}
	}

	for _, np := range npList.Items {
		result.Summary.TotalPolicies++

		entry, ok := nsStats[np.Namespace]
		if !ok {
			entry = &NetPolicyBudgetEntry1996{Namespace: np.Namespace}
			nsStats[np.Namespace] = entry
		}
		entry.PolicyCount++

		// Check for deny-all (empty ingress/egress with policy type)
		for _, pt := range np.Spec.PolicyTypes {
			if pt == "Ingress" && len(np.Spec.Ingress) == 0 {
				entry.HasIngressDeny = true
			}
			if pt == "Egress" && len(np.Spec.Egress) == 0 {
				entry.HasEgressDeny = true
			}
		}
	}

	result.Summary.TotalNamespaces = len(nsList.Items)
	for _, entry := range nsStats {
		if entry.HasIngressDeny || entry.HasEgressDeny {
			result.Summary.WithDefaultDeny++
		}
		if entry.PolicyCount == 0 {
			result.Summary.Unprotected++
			score -= 2
		}
		result.PerNS = append(result.PerNS, *entry)
	}

	sort.Slice(result.PerNS, func(i, j int) bool {
		return result.PerNS[i].PolicyCount > result.PerNS[j].PolicyCount
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d namespaces: %d with deny, %d unprotected, %d total policies", result.Summary.TotalNamespaces, result.Summary.WithDefaultDeny, result.Summary.Unprotected, result.Summary.TotalPolicies))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
