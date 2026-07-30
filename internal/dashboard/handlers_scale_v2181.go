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
// v21.81 — Scalability & HA Dimension (Round 49)
// 1. CPU Request Fragmentation Per Node
// 2. Namespace Multi-Replica HA Coverage
// 3. PVC Storage Capacity Utilization
// ============================================================

type CPUFragPerNodeResult2181 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
		AvgFragPct int `json:"avgFragmentationPct"`
	} `json:"summary"`
	Nodes []struct {
		Node     string  `json:"node"`
		ReqCPU   float64 `json:"requestedCPU"`
		AllocCPU float64 `json:"allocatableCPU"`
		FragPct  int     `json:"fragmentationPct"`
	} `json:"nodes"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleCPUFragPerNode2181(w http.ResponseWriter, r *http.Request) {
	result := CPUFragPerNodeResult2181{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	reqPerNode := make(map[string]float64)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning || pod.Spec.NodeName == "" {
			continue
		}
		for _, c := range pod.Spec.Containers {
			reqPerNode[pod.Spec.NodeName] += c.Resources.Requests.Cpu().AsApproximateFloat64()
		}
	}
	var totalFrag int
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		alloc := node.Status.Allocatable.Cpu().AsApproximateFloat64()
		req := reqPerNode[node.Name]
		fragPct := 0
		if alloc > 0 {
			fragPct = int((1 - req/alloc) * 100)
		}
		totalFrag += fragPct
		result.Nodes = append(result.Nodes, struct {
			Node     string  `json:"node"`
			ReqCPU   float64 `json:"requestedCPU"`
			AllocCPU float64 `json:"allocatableCPU"`
			FragPct  int     `json:"fragmentationPct"`
		}{node.Name, req, alloc, fragPct})
	}
	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgFragPct = totalFrag / result.Summary.TotalNodes
	}
	sort.Slice(result.Nodes, func(i, j int) bool { return result.Nodes[i].FragPct > result.Nodes[j].FragPct })
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. NS Multi-Replica HA Coverage
type NSMultiReplicaResult2181 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS        int `json:"totalNamespaces"`
		MultiReplicaNS int `json:"multiReplicaNamespaces"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleNSMultiReplica2181(w http.ResponseWriter, r *http.Request) {
	result := NSMultiReplicaResult2181{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	nsMulti := make(map[string]bool)
	nsAll := make(map[string]bool)
	for _, dep := range deployList.Items {
		nsAll[dep.Namespace] = true
		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		if replicas >= 2 {
			nsMulti[dep.Namespace] = true
		}
	}
	result.Summary.TotalNS = len(nsAll)
	result.Summary.MultiReplicaNS = len(nsMulti)
	if result.Summary.TotalNS > 0 && result.Summary.MultiReplicaNS < result.Summary.TotalNS/2 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. PVC Storage Utilization
type PVCStorageUtilResult2181 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPVCs  int     `json:"totalPVCs"`
		TotalReqGB float64 `json:"totalRequestedGB"`
		AvgPerPVC  float64 `json:"avgPerPVCGB"`
		MaxPVCGB   float64 `json:"maxPVCGB"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handlePVCStorageUtil2181(w http.ResponseWriter, r *http.Request) {
	result := PVCStorageUtilResult2181{ScannedAt: time.Now()}
	score := 100
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	var maxGB float64
	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		var gb float64
		if req := pvc.Spec.Resources.Requests.Storage(); req != nil {
			gb = req.AsApproximateFloat64() / 1e9
		}
		result.Summary.TotalReqGB += gb
		if gb > maxGB {
			maxGB = gb
		}
	}
	if result.Summary.TotalPVCs > 0 {
		result.Summary.AvgPerPVC = result.Summary.TotalReqGB / float64(result.Summary.TotalPVCs)
	}
	result.Summary.MaxPVCGB = maxGB
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	if maxGB > 1000 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("Largest PVC is %.0fGB — monitor growth", maxGB))
	}
	writeJSON(w, result)
}
