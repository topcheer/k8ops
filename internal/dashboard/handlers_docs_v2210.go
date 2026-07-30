package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.10 — Documentation Dimension (Round 54)
// 1. Node MachineID Catalog
// 2. PVC Phase Distribution
// 3. Pod Restart Count Distribution
// ============================================================

type MachineIDResult2210 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes  int            `json:"totalNodes"`
		ByMachineID map[string]int `json:"byMachineID"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleMachineID2210(w http.ResponseWriter, r *http.Request) {
	result := MachineIDResult2210{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	result.Summary.ByMachineID = make(map[string]int)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		mid := node.Status.NodeInfo.MachineID
		if len(mid) > 12 {
			mid = mid[:12]
		}
		result.Summary.ByMachineID[mid]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. PVC Phase Distribution
type PVCPhaseDistResult2210 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPVCs int            `json:"totalPVCs"`
		ByPhase   map[string]int `json:"byPhase"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handlePVCPhaseDist2210(w http.ResponseWriter, r *http.Request) {
	result := PVCPhaseDistResult2210{ScannedAt: time.Now()}
	score := 100
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByPhase = make(map[string]int)
	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		result.Summary.ByPhase[string(pvc.Status.Phase)]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Pod Restart Count Distribution
type RestartCountDistResult2210 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods       int            `json:"totalPods"`
		ByRestartBucket map[string]int `json:"byRestartBucket"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleRestartCountDist2210(w http.ResponseWriter, r *http.Request) {
	result := RestartCountDistResult2210{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByRestartBucket = make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		var maxR int32
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.RestartCount > maxR {
				maxR = cs.RestartCount
			}
		}
		bucket := "0"
		if maxR > 0 {
			bucket = "1-3"
		}
		if maxR > 3 {
			bucket = "4-10"
		}
		if maxR > 10 {
			bucket = "10+"
		}
		result.Summary.ByRestartBucket[bucket]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
