package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v22.83 Scalability: Top Namespace by Pod Count, Node CPU Oversubscription, Storage by Namespace
type NSPodTopResult2283 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNS"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string `json:"namespace"`
		PodCount  int    `json:"podCount"`
	} `json:"topNS"`
}

func (s *Server) handleNSPodTop2283(w http.ResponseWriter, r *http.Request) {
	result := NSPodTopResult2283{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nsPods := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		nsPods[pod.Namespace]++
	}
	result.Summary.TotalNS = len(nsPods)
	for ns, count := range nsPods {
		result.TopNS = append(result.TopNS, struct {
			Namespace string `json:"namespace"`
			PodCount  int    `json:"podCount"`
		}{ns, count})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].PodCount > result.TopNS[j].PodCount })
	if len(result.TopNS) > 10 {
		result.TopNS = result.TopNS[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CPUOversubResult2283 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes     int `json:"totalNodes"`
		Oversubscribed int `json:"oversubscribedNodes"`
		MaxOversubPct  int `json:"maxOversubPct"`
	} `json:"summary"`
}

func (s *Server) handleCPUOversub2283(w http.ResponseWriter, r *http.Request) {
	result := CPUOversubResult2283{ScannedAt: time.Now()}
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
	for _, node := range nodeList.Items {
		alloc := nodeAlloc[node.Name]
		req := nodeReq[node.Name]
		if alloc > 0 && req > alloc {
			result.Summary.Oversubscribed++
			pct := int((req - alloc) * 100 / alloc)
			if pct > result.Summary.MaxOversubPct {
				result.Summary.MaxOversubPct = pct
			}
		}
	}
	score := 100
	if result.Summary.TotalNodes > 0 {
		score = 100 - (result.Summary.Oversubscribed*100)/result.Summary.TotalNodes
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type StorageByNSResult2283 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPVCs int `json:"totalPVCs"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string  `json:"namespace"`
		SizeGB    float64 `json:"sizeGB"`
	} `json:"topNS"`
}

func (s *Server) handleStorageByNS2283(w http.ResponseWriter, r *http.Request) {
	result := StorageByNSResult2283{ScannedAt: time.Now()}
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	nsSize := make(map[string]float64)
	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		nsSize[pvc.Namespace] += pvc.Spec.Resources.Requests.Storage().AsApproximateFloat64() / 1e9
	}
	for ns, size := range nsSize {
		result.TopNS = append(result.TopNS, struct {
			Namespace string  `json:"namespace"`
			SizeGB    float64 `json:"sizeGB"`
		}{ns, size})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].SizeGB > result.TopNS[j].SizeGB })
	if len(result.TopNS) > 10 {
		result.TopNS = result.TopNS[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
