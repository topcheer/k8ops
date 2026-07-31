package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.68 Product: Pod Host IPC Audit, Container Timeout, Service Cluster IP Type
type HostIPCResult2368 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
		HostIPC   int `json:"hostIPC"`
	} `json:"summary"`
}

func (s *Server) handleHostIPC2368(w http.ResponseWriter, r *http.Request) {
	result := HostIPCResult2368{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.HostIPC {
			result.Summary.HostIPC++
		}
	}
	score := 100
	if result.Summary.TotalPods > 0 && result.Summary.HostIPC > 0 {
		score = 100 - (result.Summary.HostIPC*30)/result.Summary.TotalPods
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type CtnrTimeoutResult2368 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int            `json:"totalContainers"`
		ByProbeTimeout  map[string]int `json:"byProbeTimeout"`
	} `json:"summary"`
}

func (s *Server) handleCtnrTimeout2368(w http.ResponseWriter, r *http.Request) {
	result := CtnrTimeoutResult2368{ScannedAt: time.Now()}
	result.Summary.ByProbeTimeout = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.LivenessProbe != nil {
				result.Summary.ByProbeTimeout["liveness"]++
			}
			if c.ReadinessProbe != nil {
				result.Summary.ByProbeTimeout["readiness"]++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ClusterIPTypeResult2368 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices  int `json:"totalServices"`
		ClusterIPEmpty int `json:"clusterIPEmpty"`
	} `json:"summary"`
}

func (s *Server) handleClusterIPType2368(w http.ResponseWriter, r *http.Request) {
	result := ClusterIPTypeResult2368{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		if svc.Spec.ClusterIP == "" || svc.Spec.ClusterIP == "None" {
			result.Summary.ClusterIPEmpty++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
