package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.60 Operations: Node Kubelet Version Check, Pod Ready Ratio, Container Port Exposure
type NodeKubeletVerResult2460 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByKubelet  map[string]int `json:"byKubeletVersion"`
	} `json:"summary"`
}

func (s *Server) handleNodeKubeletVer2460(w http.ResponseWriter, r *http.Request) {
	result := NodeKubeletVerResult2460{ScannedAt: time.Now()}
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

type PodReadyRatioResult2460 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		ReadyPods int `json:"readyPods"`
	} `json:"summary"`
}

func (s *Server) handlePodReadyRatio2460(w http.ResponseWriter, r *http.Request) {
	result := PodReadyRatioResult2460{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		isReady := true
		for _, cs := range pod.Status.ContainerStatuses {
			if !cs.Ready {
				isReady = false
				break
			}
		}
		if isReady {
			result.Summary.ReadyPods++
		}
	}
	score := 100
	if result.Summary.TotalPods > 0 {
		score = result.Summary.ReadyPods * 100 / result.Summary.TotalPods
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type CtnrPortExposureResult2460 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		TotalPorts      int `json:"totalPorts"`
		HostPorts       int `json:"hostPorts"`
	} `json:"summary"`
}

func (s *Server) handleCtnrPortExposure2460(w http.ResponseWriter, r *http.Request) {
	result := CtnrPortExposureResult2460{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			for _, p := range c.Ports {
				result.Summary.TotalPorts++
				if p.HostPort != 0 {
					result.Summary.HostPorts++
				}
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
