package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.56 Operations: Node Allocatable vs Capacity Memory, Pod Spec Volume Count Detail, Container Liveness Probe Detail
type NodeMemVsCapResult2556 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes   int     `json:"totalNodes"`
		TotalCapGB   float64 `json:"totalCapacityGB"`
		TotalAllocGB float64 `json:"totalAllocatableGB"`
	}
}

func (s *Server) handleNodeMemVsCap2556(w http.ResponseWriter, r *http.Request) {
	result := NodeMemVsCapResult2556{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalCapGB += node.Status.Capacity.Memory().AsApproximateFloat64() / (1024 * 1024 * 1024)
		result.Summary.TotalAllocGB += node.Status.Allocatable.Memory().AsApproximateFloat64() / (1024 * 1024 * 1024)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodVolCountResult2556 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByVolType map[string]int `json:"byVolumeType"`
	}
}

func (s *Server) handlePodVolCount2556(w http.ResponseWriter, r *http.Request) {
	result := PodVolCountResult2556{ScannedAt: time.Now()}
	result.Summary.ByVolType = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for _, vol := range pod.Spec.Volumes {
			vt := "<other>"
			if vol.ConfigMap != nil {
				vt = "ConfigMap"
			} else if vol.Secret != nil {
				vt = "Secret"
			} else if vol.EmptyDir != nil {
				vt = "EmptyDir"
			} else if vol.PersistentVolumeClaim != nil {
				vt = "PVC"
			} else if vol.HostPath != nil {
				vt = "HostPath"
			}
			result.Summary.ByVolType[vt]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type LivenessProbeResult2556 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithLiveness    int `json:"withLiveness"`
		WithHTTPGet     int `json:"withHTTPGetProbe"`
		WithExec        int `json:"withExecProbe"`
		WithTCPSocket   int `json:"withTCPSocketProbe"`
	}
}

func (s *Server) handleLivenessProbe2556(w http.ResponseWriter, r *http.Request) {
	result := LivenessProbeResult2556{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.LivenessProbe != nil {
				result.Summary.WithLiveness++
				if c.LivenessProbe.HTTPGet != nil {
					result.Summary.WithHTTPGet++
				}
				if c.LivenessProbe.Exec != nil {
					result.Summary.WithExec++
				}
				if c.LivenessProbe.TCPSocket != nil {
					result.Summary.WithTCPSocket++
				}
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
