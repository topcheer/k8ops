package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v25.06 Product: Pod PreemptionPolicy, Container Image Registry Domain, Service ExternalIPs Count
type PreemptionPolicyResult2506 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByPolicy  map[string]int `json:"byPreemptionPolicy"`
	} `json:"summary"`
}

func (s *Server) handlePreemptionPolicy2506(w http.ResponseWriter, r *http.Request) {
	result := PreemptionPolicyResult2506{ScannedAt: time.Now()}
	result.Summary.ByPolicy = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		pp := "<none>"
		if pod.Spec.PreemptionPolicy != nil {
			pp = string(*pod.Spec.PreemptionPolicy)
		}
		result.Summary.ByPolicy[pp]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ImageRegistryDomainResult2506 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalImages int            `json:"totalImages"`
		ByDomain    map[string]int `json:"byRegistryDomain"`
	} `json:"summary"`
}

func (s *Server) handleImageRegistryDomain2506(w http.ResponseWriter, r *http.Request) {
	result := ImageRegistryDomainResult2506{ScannedAt: time.Now()}
	result.Summary.ByDomain = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalImages++
			domain := "docker.io"
			for i := 0; i < len(c.Image); i++ {
				if c.Image[i] == '/' {
					domain = c.Image[:i]
					break
				}
			}
			result.Summary.ByDomain[domain]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type ExternalIPsResult2506 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSvcs   int `json:"totalServices"`
		WithExtIPs  int `json:"withExternalIPs"`
		TotalExtIPs int `json:"totalExternalIPs"`
	} `json:"summary"`
}

func (s *Server) handleExternalIPs2506(w http.ResponseWriter, r *http.Request) {
	result := ExternalIPsResult2506{ScannedAt: time.Now()}
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalSvcs++
		if len(svc.Spec.ExternalIPs) > 0 {
			result.Summary.WithExtIPs++
			result.Summary.TotalExtIPs += len(svc.Spec.ExternalIPs)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
