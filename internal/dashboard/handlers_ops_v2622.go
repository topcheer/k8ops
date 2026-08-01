package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v26.22 Operations: Node KubeletVersion Dist, Pod Containers vs InitContainers, Container EphemeralStorage Limit
type NodeKubelet2622Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByVersion  map[string]int `json:"byKubeletVersion"`
	} `json:"summary"`
}

func (s *Server) handleNodeKubelet2622(w http.ResponseWriter, r *http.Request) {
	result := NodeKubelet2622Result{ScannedAt: time.Now()}
	result.Summary.ByVersion = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		kv := node.Status.NodeInfo.KubeletVersion
		if kv == "" {
			kv = "<unknown>"
		}
		result.Summary.ByVersion[kv]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CtnrVsInit2622Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		TotalCtnr int `json:"totalContainers"`
		TotalInit int `json:"totalInitContainers"`
	} `json:"summary"`
}

func (s *Server) handleCtnrVsInit2622(w http.ResponseWriter, r *http.Request) {
	result := CtnrVsInit2622Result{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		result.Summary.TotalCtnr += len(pod.Spec.Containers)
		result.Summary.TotalInit += len(pod.Spec.InitContainers)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type EphemeralLimit2622Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithEphemeral   int `json:"withEphemeralLimit"`
	} `json:"summary"`
}

func (s *Server) handleEphemeralLimit2622(w http.ResponseWriter, r *http.Request) {
	result := EphemeralLimit2622Result{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if _, ok := c.Resources.Limits[corev1.ResourceEphemeralStorage]; ok {
				result.Summary.WithEphemeral++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
