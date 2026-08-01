package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.98 Documentation: Node Capacity Pods, Pod Spec ShareProcessNamespace, Namespace Creation Timestamp
type NodeCapPodsResult2498 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
		TotalCap   int `json:"totalPodCapacity"`
	} `json:"summary"`
}

func (s *Server) handleNodeCapPods2498(w http.ResponseWriter, r *http.Request) {
	result := NodeCapPodsResult2498{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalCap += int(node.Status.Capacity.Pods().Value())
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ShareProcNSResult2498 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		Shared    int `json:"shareProcessNamespace"`
	} `json:"summary"`
}

func (s *Server) handleShareProcNS2498(w http.ResponseWriter, r *http.Request) {
	result := ShareProcNSResult2498{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.ShareProcessNamespace != nil && *pod.Spec.ShareProcessNamespace {
			result.Summary.Shared++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NSCreationResult2498 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS  int   `json:"totalNamespaces"`
		OldestNS int64 `json:"oldestAgeHours"`
	} `json:"summary"`
}

func (s *Server) handleNSCreation2498(w http.ResponseWriter, r *http.Request) {
	result := NSCreationResult2498{ScannedAt: time.Now()}
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	now := time.Now()
	var oldest time.Time
	for _, ns := range nsList.Items {
		result.Summary.TotalNS++
		ct := ns.CreationTimestamp.Time
		if oldest.IsZero() || ct.Before(oldest) {
			oldest = ct
		}
	}
	if !oldest.IsZero() {
		result.Summary.OldestNS = int64(now.Sub(oldest).Hours())
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
