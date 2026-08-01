package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v24.21 Scalability: Top Namespace by Storage, Node CPU Capacity, Cluster NetworkPolicy by NS
type TopNSStorageResult2421 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNS"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string  `json:"namespace"`
		StorageGB float64 `json:"storageRequestGB"`
	} `json:"topNS"`
}

func (s *Server) handleTopNSStorage2421(w http.ResponseWriter, r *http.Request) {
	result := TopNSStorageResult2421{ScannedAt: time.Now()}
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	nsStorage := make(map[string]float64)
	for _, pvc := range pvcList.Items {
		nsStorage[pvc.Namespace] += pvc.Spec.Resources.Requests.Storage().AsApproximateFloat64() / 1e9
	}
	result.Summary.TotalNS = len(nsStorage)
	for ns, stg := range nsStorage {
		result.TopNS = append(result.TopNS, struct {
			Namespace string  `json:"namespace"`
			StorageGB float64 `json:"storageRequestGB"`
		}{ns, stg})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].StorageGB > result.TopNS[j].StorageGB })
	if len(result.TopNS) > 10 {
		result.TopNS = result.TopNS[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeCPUCapResult2421 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalCPU   float64 `json:"totalCPUCapacity"`
	} `json:"summary"`
}

func (s *Server) handleNodeCPUCap2421(w http.ResponseWriter, r *http.Request) {
	result := NodeCPUCapResult2421{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalCPU += node.Status.Capacity.Cpu().AsApproximateFloat64()
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NetPolByNSResult2421 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNetPols int            `json:"totalNetPols"`
		ByNS         map[string]int `json:"byNamespace"`
	} `json:"summary"`
}

func (s *Server) handleNetPolByNS2421(w http.ResponseWriter, r *http.Request) {
	result := NetPolByNSResult2421{ScannedAt: time.Now()}
	result.Summary.ByNS = make(map[string]int)
	npList, _ := s.clientset.NetworkingV1().NetworkPolicies("").List(r.Context(), metav1.ListOptions{})
	for _, np := range npList.Items {
		result.Summary.TotalNetPols++
		result.Summary.ByNS[np.Namespace]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
