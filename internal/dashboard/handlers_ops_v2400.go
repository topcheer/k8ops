package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.00 Operations: Pod OOMKilled, Node Cond kubelet, Container Volume Device Count
type OOMKilledResult2400 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalTerminated int `json:"totalTerminated"`
		OOMKilled       int `json:"oomKilled"`
	} `json:"summary"`
}

func (s *Server) handleOOMKilled2400(w http.ResponseWriter, r *http.Request) {
	result := OOMKilledResult2400{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.LastTerminationState.Terminated != nil {
				result.Summary.TotalTerminated++
				if cs.LastTerminationState.Terminated.Reason == "OOMKilled" {
					result.Summary.OOMKilled++
				}
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeCondKubeletResult2400 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByVer      map[string]int `json:"byKubeletVersion"`
	} `json:"summary"`
}

func (s *Server) handleNodeCondKubelet2400(w http.ResponseWriter, r *http.Request) {
	result := NodeCondKubeletResult2400{ScannedAt: time.Now()}
	result.Summary.ByVer = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.ByVer[node.Status.NodeInfo.KubeletVersion]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type VolDeviceCountResult2400 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		TotalVolDevices int `json:"totalVolumeDevices"`
	} `json:"summary"`
}

func (s *Server) handleVolDeviceCount2400(w http.ResponseWriter, r *http.Request) {
	result := VolDeviceCountResult2400{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.TotalVolDevices += len(c.VolumeDevices)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
