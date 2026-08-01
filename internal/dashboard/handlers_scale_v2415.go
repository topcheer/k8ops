package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v24.15 Scalability: Top Namespace by PVC, Node Storage Allocatable, Cluster Secret by Type
type TopNSPVCResult2415 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNS"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string `json:"namespace"`
		PVCCount  int    `json:"pvcCount"`
	} `json:"topNS"`
}

func (s *Server) handleTopNSPVC2415(w http.ResponseWriter, r *http.Request) {
	result := TopNSPVCResult2415{ScannedAt: time.Now()}
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	nsPVCs := make(map[string]int)
	for _, pvc := range pvcList.Items {
		nsPVCs[pvc.Namespace]++
	}
	result.Summary.TotalNS = len(nsPVCs)
	for ns, count := range nsPVCs {
		result.TopNS = append(result.TopNS, struct {
			Namespace string `json:"namespace"`
			PVCCount  int    `json:"pvcCount"`
		}{ns, count})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].PVCCount > result.TopNS[j].PVCCount })
	if len(result.TopNS) > 10 {
		result.TopNS = result.TopNS[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeStorAllocResult2415 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalGB    float64 `json:"totalAllocatableStorageGB"`
	} `json:"summary"`
}

func (s *Server) handleNodeStorAlloc2415(w http.ResponseWriter, r *http.Request) {
	result := NodeStorAllocResult2415{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalGB += node.Status.Allocatable.Storage().AsApproximateFloat64() / 1e9
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SecretByTypeResult2415 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int            `json:"totalSecrets"`
		ByType       map[string]int `json:"bySecretType"`
	} `json:"summary"`
}

func (s *Server) handleSecretByType2415(w http.ResponseWriter, r *http.Request) {
	result := SecretByTypeResult2415{ScannedAt: time.Now()}
	result.Summary.ByType = make(map[string]int)
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		result.Summary.ByType[string(secret.Type)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
