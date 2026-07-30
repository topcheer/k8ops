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
// v21.63 — Scalability & HA Dimension (Round 46)
// 1. Node Pod Capacity Utilization Forecast
// 2. Namespace Deployment HA Coverage
// 3. PVC Storage Quota Headroom
// ============================================================

type PodCapForecastResult2163 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes     int `json:"totalNodes"`
		TotalPods      int `json:"runningPods"`
		TotalCap       int `json:"totalPodCapacity"`
		UtilizationPct int `json:"utilizationPct"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handlePodCapForecast2163(w http.ResponseWriter, r *http.Request) {
	result := PodCapForecastResult2163{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		pods := node.Status.Allocatable.Pods()
		if pods != nil {
			result.Summary.TotalCap += int(pods.AsApproximateFloat64())
		}
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			result.Summary.TotalPods++
		}
	}
	if result.Summary.TotalCap > 0 {
		result.Summary.UtilizationPct = result.Summary.TotalPods * 100 / result.Summary.TotalCap
	}
	if result.Summary.UtilizationPct > 80 {
		score -= 15
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. NS Deployment HA Coverage
type NSDepHAResult2163 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS        int `json:"totalNamespaces"`
		MultiReplicaNS int `json:"namespacesWithMultiReplica"`
	} `json:"summary"`
	LowHA []struct {
		Namespace string `json:"namespace"`
		SingleRep int    `json:"singleReplicaDeployments"`
	} `json:"lowHANamespaces"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleNSDepHA2163(w http.ResponseWriter, r *http.Request) {
	result := NSDepHAResult2163{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	nsSingle := make(map[string]int)
	nsMulti := make(map[string]bool)
	for _, dep := range deployList.Items {
		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		if replicas > 1 {
			nsMulti[dep.Namespace] = true
		} else {
			nsSingle[dep.Namespace]++
		}
	}
	for ns := range nsSingle {
		if nsSingle[ns] >= 3 && !nsMulti[ns] {
			result.LowHA = append(result.LowHA, struct {
				Namespace string `json:"namespace"`
				SingleRep int    `json:"singleReplicaDeployments"`
			}{ns, nsSingle[ns]})
			score -= 2
		}
	}
	result.Summary.TotalNS = len(nsSingle)
	result.Summary.MultiReplicaNS = len(nsMulti)
	sort.Slice(result.LowHA, func(i, j int) bool { return result.LowHA[i].SingleRep > result.LowHA[j].SingleRep })
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	if len(result.LowHA) > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d namespaces with only single-replica deployments", len(result.LowHA)))
	}
	writeJSON(w, result)
}

// 3. PVC Storage Quota Headroom
type PVCQuotaHRResult2163 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPVCs   int     `json:"totalPVCs"`
		TotalReqGB  float64 `json:"totalRequestedGB"`
		AvgPerPVCGB float64 `json:"avgPerPVCGB"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handlePVCQuotaHR2163(w http.ResponseWriter, r *http.Request) {
	result := PVCQuotaHRResult2163{ScannedAt: time.Now()}
	score := 100
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		if req := pvc.Spec.Resources.Requests.Storage(); req != nil {
			result.Summary.TotalReqGB += req.AsApproximateFloat64() / 1e9
		}
	}
	if result.Summary.TotalPVCs > 0 {
		result.Summary.AvgPerPVCGB = result.Summary.TotalReqGB / float64(result.Summary.TotalPVCs)
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
