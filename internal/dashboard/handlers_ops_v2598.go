package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.98 Operations: Node Status NodeInfo Summary, Pod Spec Volume ConfigMap Count, Container Resource CPU Request Detail
type NodeInfoSummaryResult2598 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByKubelet  map[string]int `json:"byKubeletVersion"`
	}
}

func (s *Server) handleNodeInfoSummary2598(w http.ResponseWriter, r *http.Request) {
	result := NodeInfoSummaryResult2598{ScannedAt: time.Now()}
	result.Summary.ByKubelet = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		kv := node.Status.NodeInfo.KubeletVersion
		if kv == "" {
			kv = "<unknown>"
		}
		result.Summary.ByKubelet[kv]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodVolCMResult2598 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods   int `json:"totalPods"`
		TotalCMVols int `json:"totalConfigMapVolumes"`
	}
}

func (s *Server) handlePodVolCM2598(w http.ResponseWriter, r *http.Request) {
	result := PodVolCMResult2598{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for _, vol := range pod.Spec.Volumes {
			if vol.ConfigMap != nil {
				result.Summary.TotalCMVols++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CPUReqDetailResult2598 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int     `json:"totalContainers"`
		TotalCPUReq     float64 `json:"totalCPUReqCores"`
	}
}

func (s *Server) handleCPUReqDetail2598(w http.ResponseWriter, r *http.Request) {
	result := CPUReqDetailResult2598{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.TotalCPUReq += c.Resources.Requests.Cpu().AsApproximateFloat64()
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
