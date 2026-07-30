package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.17 — Operations Dimension (Round 39)
// 1. Node Machine Info Catalog
// 2. Pod Container Exit Code Distribution
// 3. Service Load Balancer Health
// ============================================================

type MachineInfoResult2117 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         MachineInfoSummary2117 `json:"summary"`
	Recommendations []string               `json:"recommendations"`
}

type MachineInfoSummary2117 struct {
	TotalNodes  int            `json:"totalNodes"`
	ByMachineID map[string]int `json:"byMachineID"`
}

func (s *Server) handleMachineInfo2117(w http.ResponseWriter, r *http.Request) {
	result := MachineInfoResult2117{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	byMID := make(map[string]int)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		byMID[node.Status.NodeInfo.MachineID[:8]]++
	}
	result.Summary.ByMachineID = byMID
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Container Exit Code Distribution
type ExitCodeResult2117 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         ExitCodeSummary2117 `json:"summary"`
	Recommendations []string            `json:"recommendations"`
}

type ExitCodeSummary2117 struct {
	TotalTerminated int         `json:"totalTerminated"`
	ByExitCode      map[int]int `json:"byExitCode"`
}

func (s *Server) handleExitCode2117(w http.ResponseWriter, r *http.Request) {
	result := ExitCodeResult2117{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	byExit := make(map[int]int)
	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.State.Terminated != nil {
				result.Summary.TotalTerminated++
				byExit[int(cs.State.Terminated.ExitCode)]++
			}
		}
	}
	result.Summary.ByExitCode = byExit
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Service LB Health
type LBHealthResult2117 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         LBHealthSummary2117 `json:"summary"`
	Recommendations []string            `json:"recommendations"`
}

type LBHealthSummary2117 struct {
	TotalLB   int `json:"totalLoadBalancers"`
	HealthyLB int `json:"healthyLB"`
	PendingLB int `json:"pendingLB"`
}

func (s *Server) handleLBHealth2117(w http.ResponseWriter, r *http.Request) {
	result := LBHealthResult2117{ScannedAt: time.Now()}
	score := 100
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})

	for _, svc := range svcList.Items {
		if svc.Spec.Type != corev1.ServiceTypeLoadBalancer {
			continue
		}
		result.Summary.TotalLB++
		hasIngress := false
		for _, ing := range svc.Status.LoadBalancer.Ingress {
			if ing.IP != "" || ing.Hostname != "" {
				hasIngress = true
			}
		}
		if hasIngress {
			result.Summary.HealthyLB++
		} else {
			result.Summary.PendingLB++
		}
	}
	if result.Summary.PendingLB > 0 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
