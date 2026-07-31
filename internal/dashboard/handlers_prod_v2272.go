package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v22.72 Product: Container Volume Mount Count, Pod DNS Policy Census, Init Container Audit
type VolMountCountResult2272 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		TotalMounts     int `json:"totalMounts"`
		AvgPerContainer int `json:"avgPerContainer"`
	} `json:"summary"`
}

func (s *Server) handleVolMountCount2272(w http.ResponseWriter, r *http.Request) {
	result := VolMountCountResult2272{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			result.Summary.TotalMounts += len(c.VolumeMounts)
		}
	}
	if result.Summary.TotalContainers > 0 {
		result.Summary.AvgPerContainer = result.Summary.TotalMounts / result.Summary.TotalContainers
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DNSPolicyResult2272 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods   int            `json:"totalPods"`
		ByDNSPolicy map[string]int `json:"byDNSPolicy"`
	} `json:"summary"`
}

func (s *Server) handleDNSPolicy2272(w http.ResponseWriter, r *http.Request) {
	result := DNSPolicyResult2272{ScannedAt: time.Now()}
	result.Summary.ByDNSPolicy = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		result.Summary.ByDNSPolicy[string(pod.Spec.DNSPolicy)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type InitCtnrResult2272 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods      int `json:"totalPods"`
		WithInitCtnrs  int `json:"withInitContainers"`
		TotalInitCtnrs int `json:"totalInitContainers"`
	} `json:"summary"`
}

func (s *Server) handleInitCtnr2272(w http.ResponseWriter, r *http.Request) {
	result := InitCtnrResult2272{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.Spec.InitContainers) > 0 {
			result.Summary.WithInitCtnrs++
			result.Summary.TotalInitCtnrs += len(pod.Spec.InitContainers)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
