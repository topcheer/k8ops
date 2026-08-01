package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.54 Product: Pod Spec ServiceAccount Dist, Container Resource Request CPU, Service Port Range
type SADistResult2554 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		BySA      map[string]int `json:"byServiceAccount"`
	}
}

func (s *Server) handleSADist2554(w http.ResponseWriter, r *http.Request) {
	result := SADistResult2554{ScannedAt: time.Now()}
	result.Summary.BySA = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		sa := pod.Spec.ServiceAccountName
		if sa == "" {
			sa = "<default>"
		}
		result.Summary.BySA[sa]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CPUReqContainerResult2554 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int     `json:"totalContainers"`
		TotalCPUReq     float64 `json:"totalCPUReqCores"`
	}
}

func (s *Server) handleCPUReqContainer2554(w http.ResponseWriter, r *http.Request) {
	result := CPUReqContainerResult2554{ScannedAt: time.Now()}
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

type ServicePortRangeResult2554 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSvcs  int `json:"totalServices"`
		TotalPorts int `json:"totalPorts"`
		MinPort    int `json:"minPort"`
		MaxPort    int `json:"maxPort"`
	}
}

func (s *Server) handleServicePortRange2554(w http.ResponseWriter, r *http.Request) {
	result := ServicePortRangeResult2554{ScannedAt: time.Now()}
	result.Summary.MinPort = 65536
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalSvcs++
		for _, p := range svc.Spec.Ports {
			result.Summary.TotalPorts++
			if int(p.Port) < result.Summary.MinPort {
				result.Summary.MinPort = int(p.Port)
			}
			if int(p.Port) > result.Summary.MaxPort {
				result.Summary.MaxPort = int(p.Port)
			}
		}
	}
	if result.Summary.MinPort == 65536 {
		result.Summary.MinPort = 0
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
