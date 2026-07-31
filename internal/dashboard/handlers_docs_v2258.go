package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v22.58 Documentation: Node Status Conditions Summary, PVC VolumeName Catalog, Pod Image Count
type NodeCondSummaryResult2258 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes  int            `json:"totalNodes"`
		ByCondition map[string]int `json:"byCondition"`
	} `json:"summary"`
}

func (s *Server) handleNodeCondSummary2258(w http.ResponseWriter, r *http.Request) {
	result := NodeCondSummaryResult2258{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	result.Summary.ByCondition = make(map[string]int)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		for _, cond := range node.Status.Conditions {
			if cond.Status == corev1.ConditionTrue {
				result.Summary.ByCondition[string(cond.Type)]++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PVCVolNameResult2258 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPVCs   int `json:"totalPVCs"`
		WithVolName int `json:"withVolumeName"`
	} `json:"summary"`
}

func (s *Server) handlePVCVolName2258(w http.ResponseWriter, r *http.Request) {
	result := PVCVolNameResult2258{ScannedAt: time.Now()}
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		if pvc.Spec.VolumeName != "" {
			result.Summary.WithVolName++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodImgCountResult2258 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods    int `json:"totalPods"`
		TotalImages  int `json:"totalImageRefs"`
		UniqueImages int `json:"uniqueImages"`
	} `json:"summary"`
}

func (s *Server) handlePodImgCount2258(w http.ResponseWriter, r *http.Request) {
	result := PodImgCountResult2258{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	seen := make(map[string]bool)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalImages++
			if !seen[c.Image] {
				seen[c.Image] = true
				result.Summary.UniqueImages++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
