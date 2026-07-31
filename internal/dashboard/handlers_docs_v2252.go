package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.52 — Documentation Dimension (Round 61)
// 1. Node Status Condition Heartbeat Catalog
// 2. PVC StorageClass Size Distribution
// 3. Pod Container Volume Device Mount Tracker
// ============================================================

type HeartbeatCatalogResult2252 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes  int            `json:"totalNodes"`
		ByHeartbeat map[string]int `json:"byLastHeartbeatDay"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleHeartbeatCatalog2252(w http.ResponseWriter, r *http.Request) {
	result := HeartbeatCatalogResult2252{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	result.Summary.ByHeartbeat = make(map[string]int)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady {
				result.Summary.ByHeartbeat[cond.LastHeartbeatTime.Format("2006-01-02")]++
				break
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. PVC SC Size Distribution
type PVCSCSizeResult2252 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPVCs int            `json:"totalPVCs"`
		BySC      map[string]int `json:"byStorageClass"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handlePVCSCSize2252(w http.ResponseWriter, r *http.Request) {
	result := PVCSCSizeResult2252{ScannedAt: time.Now()}
	score := 100
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	result.Summary.BySC = make(map[string]int)
	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		sc := pvc.Spec.StorageClassName
		if sc == nil || *sc == "" {
			result.Summary.BySC["default"]++
		} else {
			result.Summary.BySC[*sc]++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Volume Device Mount Tracker
type VolDeviceMountResult2252 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithDeviceMount int `json:"withDeviceMount"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleVolDeviceMount2252(w http.ResponseWriter, r *http.Request) {
	result := VolDeviceMountResult2252{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if len(c.VolumeDevices) > 0 {
				result.Summary.WithDeviceMount++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
