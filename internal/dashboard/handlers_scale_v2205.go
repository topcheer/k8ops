package dashboard

import (
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.05 — Scalability & HA Dimension (Round 53)
// 1. Node CPU Commit Ratio Analysis
// 2. Namespace HA Multiplier Score
// 3. Cluster Storage Efficiency
// ============================================================

type CPUCommitRatioResult2205 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalAlloc float64 `json:"totalAllocatableCPU"`
		TotalReq   float64 `json:"totalRequestedCPU"`
		CommitPct  int     `json:"commitPct"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleCPUCommitRatio2205(w http.ResponseWriter, r *http.Request) {
	result := CPUCommitRatioResult2205{ScannedAt: time.Now()}
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
		result.Summary.CommitPct = int(result.Summary.TotalReq / result.Summary.TotalAlloc * 100)
	}
	if result.Summary.CommitPct > 100 {
		score -= 15
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. NS HA Multiplier Score
type NSHAMultScoreResult2205 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string `json:"namespace"`
		MultiRep  int    `json:"multiReplicaDeployments"`
		SingleRep int    `json:"singleReplicaDeployments"`
	} `json:"namespaces"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleNSHAMultScore2205(w http.ResponseWriter, r *http.Request) {
	result := NSHAMultScoreResult2205{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	nsMulti := make(map[string]int)
	nsSingle := make(map[string]int)
	for _, dep := range deployList.Items {
		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		if replicas >= 2 {
			nsMulti[dep.Namespace]++
		} else {
			nsSingle[dep.Namespace]++
		}
	}
	allNS := make(map[string]bool)
	for ns := range nsMulti {
		allNS[ns] = true
	}
	for ns := range nsSingle {
		allNS[ns] = true
	}
	result.Summary.TotalNS = len(allNS)
	for ns := range allNS {
		result.TopNS = append(result.TopNS, struct {
			Namespace string `json:"namespace"`
			MultiRep  int    `json:"multiReplicaDeployments"`
			SingleRep int    `json:"singleReplicaDeployments"`
		}{ns, nsMulti[ns], nsSingle[ns]})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].SingleRep > result.TopNS[j].SingleRep })
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Cluster Storage Efficiency
type ClusterStorageEffResult2205 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPVCs  int     `json:"totalPVCs"`
		TotalReqGB float64 `json:"totalRequestedGB"`
		AvgPerNS   float64 `json:"avgPerNamespaceGB"`
		TotalNS    int     `json:"namespacesWithPVC"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleClusterStorageEff2205(w http.ResponseWriter, r *http.Request) {
	result := ClusterStorageEffResult2205{ScannedAt: time.Now()}
	score := 100
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	nsStorage := make(map[string]float64)
	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		var gb float64
		if req := pvc.Spec.Resources.Requests.Storage(); req != nil {
			gb = req.AsApproximateFloat64() / 1e9
		}
		result.Summary.TotalReqGB += gb
		nsStorage[pvc.Namespace] += gb
	}
	result.Summary.TotalNS = len(nsStorage)
	if result.Summary.TotalNS > 0 {
		result.Summary.AvgPerNS = result.Summary.TotalReqGB / float64(result.Summary.TotalNS)
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
