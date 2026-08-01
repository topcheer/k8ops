package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v24.75 Scalability: Top Namespace by PVC, Node Allocatable Pods Total, Cluster StorageClass Distribution
type TopNSByPVCResult2475 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string `json:"namespace"`
		PVCCount  int    `json:"pvcCount"`
	} `json:"topNamespaces"`
}

func (s *Server) handleTopNSByPVC2475(w http.ResponseWriter, r *http.Request) {
	result := TopNSByPVCResult2475{ScannedAt: time.Now()}
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

type NodeAllocPodsTotalResult2475 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes  int `json:"totalNodes"`
		TotalPodCap int `json:"totalPodAllocatable"`
	} `json:"summary"`
}

func (s *Server) handleNodeAllocPodsTotal2475(w http.ResponseWriter, r *http.Request) {
	result := NodeAllocPodsTotalResult2475{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalPodCap += int(node.Status.Allocatable.Pods().Value())
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type StorageClassDistResult2475 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPVCs int            `json:"totalPVCs"`
		BySC      map[string]int `json:"byStorageClass"`
	} `json:"summary"`
}

func (s *Server) handleStorageClassDist2475(w http.ResponseWriter, r *http.Request) {
	result := StorageClassDistResult2475{ScannedAt: time.Now()}
	result.Summary.BySC = make(map[string]int)
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		sc := "<none>"
		if pvc.Spec.StorageClassName != nil {
			sc = *pvc.Spec.StorageClassName
		}
		result.Summary.BySC[sc]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
