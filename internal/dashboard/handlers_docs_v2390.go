package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.90 Documentation: Node Kernel Commit, Pod Finalizer Count, PVC Size Summary
type NodeKernelCommitResult2390 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByKernel   map[string]int `json:"byKernelVersion"`
	} `json:"summary"`
}

func (s *Server) handleNodeKernelCommit2390(w http.ResponseWriter, r *http.Request) {
	result := NodeKernelCommitResult2390{ScannedAt: time.Now()}
	result.Summary.ByKernel = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.ByKernel[node.Status.NodeInfo.KernelVersion]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodFinalizerResult2390 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods     int `json:"totalPods"`
		WithFinalizer int `json:"withFinalizer"`
	} `json:"summary"`
}

func (s *Server) handlePodFinalizer2390(w http.ResponseWriter, r *http.Request) {
	result := PodFinalizerResult2390{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.Finalizers) > 0 {
			result.Summary.WithFinalizer++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PVCSizeResult2390 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPVCs   int     `json:"totalPVCs"`
		TotalSizeGB float64 `json:"totalRequestedGB"`
	} `json:"summary"`
}

func (s *Server) handlePVCSize2390(w http.ResponseWriter, r *http.Request) {
	result := PVCSizeResult2390{ScannedAt: time.Now()}
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		result.Summary.TotalSizeGB += pvc.Spec.Resources.Requests.Storage().AsApproximateFloat64() / 1e9
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
