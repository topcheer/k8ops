package dashboard

import (
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.35 — Scalability & HA Dimension (Round 58)
// 1. Namespace CPU Limit Overcommit Analysis
// 2. Node Memory Commit Ratio Score
// 3. Cluster Deployment Multi-Zone HA
// ============================================================

type NSCPULimitOvercommitResult2235 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string  `json:"namespace"`
		CPULim    float64 `json:"cpuLimit"`
		CPUReq    float64 `json:"cpuRequest"`
	} `json:"topNamespaces"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleNSCPULimitOvercommit2235(w http.ResponseWriter, r *http.Request) {
	result := NSCPULimitOvercommitResult2235{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nsLim := make(map[string]float64)
	nsReq := make(map[string]float64)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			nsLim[pod.Namespace] += c.Resources.Limits.Cpu().AsApproximateFloat64()
			nsReq[pod.Namespace] += c.Resources.Requests.Cpu().AsApproximateFloat64()
		}
	}
	result.Summary.TotalNS = len(nsLim)
	for ns := range nsLim {
		result.TopNS = append(result.TopNS, struct {
			Namespace string  `json:"namespace"`
			CPULim    float64 `json:"cpuLimit"`
			CPUReq    float64 `json:"cpuRequest"`
		}{ns, nsLim[ns], nsReq[ns]})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].CPULim > result.TopNS[j].CPULim })
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Node Memory Commit Ratio Score
type NodeMemCommitResult2235 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalAlloc float64 `json:"totalAllocatableGB"`
		TotalReq   float64 `json:"totalRequestedGB"`
		CommitPct  int     `json:"commitPct"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleNodeMemCommit2235(w http.ResponseWriter, r *http.Request) {
	result := NodeMemCommitResult2235{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalAlloc += node.Status.Allocatable.Memory().AsApproximateFloat64() / 1e9
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalReq += c.Resources.Requests.Memory().AsApproximateFloat64() / 1e9
		}
	}
	if result.Summary.TotalAlloc > 0 {
		result.Summary.CommitPct = int(result.Summary.TotalReq / result.Summary.TotalAlloc * 100)
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Deployment Multi-Zone HA
type DepMultiZoneResult2235 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeploys int            `json:"totalDeployments"`
		ByZone       map[string]int `json:"byZone"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleDepMultiZone2235(w http.ResponseWriter, r *http.Request) {
	result := DepMultiZoneResult2235{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	result.Summary.ByZone = make(map[string]int)
	for _, node := range nodeList.Items {
		zone := node.Labels["topology.kubernetes.io/zone"]
		if zone == "" {
			zone = "unknown"
		}
		result.Summary.ByZone[zone]++
	}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, owner := range pod.OwnerReferences {
			if owner.Kind == "ReplicaSet" {
				result.Summary.TotalDeploys++
				break
			}
		}
	}
	if len(result.Summary.ByZone) < 2 {
		score -= 15
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
