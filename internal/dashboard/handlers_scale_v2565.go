package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v25.65 Scalability: Top Namespace by PVC, Node Memory Allocatable Detail, Cluster Job Active
type TopNSByPVC2Result2565 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
	}
	TopNS []struct {
		Namespace string `json:"namespace"`
		PVCCount  int    `json:"pvcCount"`
	} `json:"topNamespaces"`
}

func (s *Server) handleTopNSByPVC2Result2565(w http.ResponseWriter, r *http.Request) {
	result := TopNSByPVC2Result2565{ScannedAt: time.Now()}
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

type NodeMemAllocDetailResult2565 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		MinMemGB   float64 `json:"minAllocMemGB"`
		MaxMemGB   float64 `json:"maxAllocMemGB"`
	}
}

func (s *Server) handleNodeMemAllocDetail2565(w http.ResponseWriter, r *http.Request) {
	result := NodeMemAllocDetailResult2565{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		memGB := node.Status.Allocatable.Memory().AsApproximateFloat64() / (1024 * 1024 * 1024)
		if result.Summary.MinMemGB == 0 || memGB < result.Summary.MinMemGB {
			result.Summary.MinMemGB = memGB
		}
		if memGB > result.Summary.MaxMemGB {
			result.Summary.MaxMemGB = memGB
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type JobActiveResult2565 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalJobs int `json:"totalJobs"`
		Active    int `json:"activeJobs"`
	}
}

func (s *Server) handleJobActive2565(w http.ResponseWriter, r *http.Request) {
	result := JobActiveResult2565{ScannedAt: time.Now()}
	jobList, _ := s.clientset.BatchV1().Jobs("").List(r.Context(), metav1.ListOptions{})
	for _, job := range jobList.Items {
		result.Summary.TotalJobs++
		if job.Status.Active > 0 {
			result.Summary.Active++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
