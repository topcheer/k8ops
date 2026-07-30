package dashboard

import (
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.23 — Scalability & HA Dimension (Round 56)
// 1. Namespace PVC Storage Distribution
// 2. Node CPU Allocatable Efficiency Score
// 3. Cluster Replicas Available Ratio
// ============================================================

type NSPVCStorageResult2223 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string  `json:"namespace"`
		PVCCount  int     `json:"pvcCount"`
		StorageGB float64 `json:"storageGB"`
	} `json:"topNamespaces"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleNSPVCStorage2223(w http.ResponseWriter, r *http.Request) {
	result := NSPVCStorageResult2223{ScannedAt: time.Now()}
	score := 100
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	nsData := make(map[string]*struct {
		Namespace string
		PVCCount  int
		StorageGB float64
	})
	for _, pvc := range pvcList.Items {
		if nsData[pvc.Namespace] == nil {
			nsData[pvc.Namespace] = &struct {
				Namespace string
				PVCCount  int
				StorageGB float64
			}{pvc.Namespace, 0, 0}
		}
		nsData[pvc.Namespace].PVCCount++
		if req := pvc.Spec.Resources.Requests.Storage(); req != nil {
			nsData[pvc.Namespace].StorageGB += req.AsApproximateFloat64() / 1e9
		}
	}
	result.Summary.TotalNS = len(nsData)
	for _, d := range nsData {
		result.TopNS = append(result.TopNS, struct {
			Namespace string  `json:"namespace"`
			PVCCount  int     `json:"pvcCount"`
			StorageGB float64 `json:"storageGB"`
		}{d.Namespace, d.PVCCount, d.StorageGB})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].StorageGB > result.TopNS[j].StorageGB })
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Node CPU Alloc Eff Score
type CPUAllocEffResult2223 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalAlloc float64 `json:"totalAllocatableCPU"`
		TotalReq   float64 `json:"totalRequestedCPU"`
		EffPct     int     `json:"efficiencyPct"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleCPUAllocEff2223(w http.ResponseWriter, r *http.Request) {
	result := CPUAllocEffResult2223{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalAlloc += node.Status.Allocatable.Cpu().AsApproximateFloat64()
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalReq += c.Resources.Requests.Cpu().AsApproximateFloat64()
		}
	}
	if result.Summary.TotalAlloc > 0 {
		result.Summary.EffPct = int(result.Summary.TotalReq / result.Summary.TotalAlloc * 100)
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Cluster Replicas Available Ratio
type ClusterReplicasRatioResult2223 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeploys int   `json:"totalDeployments"`
		TotalDesired int32 `json:"totalDesiredReplicas"`
		TotalAvail   int32 `json:"totalAvailableReplicas"`
		RatioPct     int   `json:"availabilityPct"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleClusterReplicasRatio2223(w http.ResponseWriter, r *http.Request) {
	result := ClusterReplicasRatioResult2223{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		result.Summary.TotalDesired += replicas
		result.Summary.TotalAvail += dep.Status.AvailableReplicas
	}
	if result.Summary.TotalDesired > 0 {
		result.Summary.RatioPct = int(result.Summary.TotalAvail) * 100 / int(result.Summary.TotalDesired)
	}
	if result.Summary.RatioPct < 90 {
		score -= 15
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
