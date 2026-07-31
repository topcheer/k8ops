package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.01 Scalability: Cluster Efficiency Score, Namespace Resource Density, Node CPU Commit Ratio
type ClusterEffResult2301 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		AllocCPU   float64 `json:"totalAllocatableCPU"`
		ReqCPU     float64 `json:"totalRequestedCPU"`
		LimitCPU   float64 `json:"totalLimitedCPU"`
		EffPct     int     `json:"efficiencyPct"`
	} `json:"summary"`
}

func (s *Server) handleClusterEff2301(w http.ResponseWriter, r *http.Request) {
	result := ClusterEffResult2301{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.AllocCPU += node.Status.Allocatable.Cpu().AsApproximateFloat64()
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.ReqCPU += c.Resources.Requests.Cpu().AsApproximateFloat64()
			result.Summary.LimitCPU += c.Resources.Limits.Cpu().AsApproximateFloat64()
		}
	}
	if result.Summary.AllocCPU > 0 && result.Summary.LimitCPU > 0 {
		result.Summary.EffPct = int(result.Summary.ReqCPU * 100 / result.Summary.LimitCPU)
	}
	result.HealthScore = result.Summary.EffPct
	if result.HealthScore > 100 {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	writeJSON(w, result)
}

type NSDensityResult2301 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS   int `json:"totalNS"`
		TotalPods int `json:"totalPods"`
		TotalSvcs int `json:"totalServices"`
		TotalCMs  int `json:"totalConfigMaps"`
	} `json:"summary"`
}

func (s *Server) handleNSDensity2301(w http.ResponseWriter, r *http.Request) {
	result := NSDensityResult2301{ScannedAt: time.Now()}
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	cmList, _ := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	result.Summary.TotalNS = len(nsList.Items)
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			result.Summary.TotalPods++
		}
	}
	result.Summary.TotalSvcs = len(svcList.Items)
	result.Summary.TotalCMs = len(cmList.Items)
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeCPUCommitResult2301 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
		AvgCommit  int `json:"avgCommitPct"`
		MaxCommit  int `json:"maxCommitPct"`
	} `json:"summary"`
}

func (s *Server) handleNodeCPUCommit2301(w http.ResponseWriter, r *http.Request) {
	result := NodeCPUCommitResult2301{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nodeReq := make(map[string]float64)
	nodeAlloc := make(map[string]float64)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		nodeAlloc[node.Name] = node.Status.Allocatable.Cpu().AsApproximateFloat64()
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			nodeReq[pod.Spec.NodeName] += c.Resources.Requests.Cpu().AsApproximateFloat64()
		}
	}
	totalPct := 0
	for _, node := range nodeList.Items {
		alloc := nodeAlloc[node.Name]
		req := nodeReq[node.Name]
		if alloc > 0 {
			pct := int(req * 100 / alloc)
			totalPct += pct
			if pct > result.Summary.MaxCommit {
				result.Summary.MaxCommit = pct
			}
		}
	}
	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgCommit = totalPct / result.Summary.TotalNodes
	}
	result.HealthScore = 100
	if result.Summary.MaxCommit > 90 {
		result.HealthScore = 60
	} else if result.Summary.MaxCommit > 70 {
		result.HealthScore = 80
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	writeJSON(w, result)
}
