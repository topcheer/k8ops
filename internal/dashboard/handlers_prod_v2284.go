package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v22.84 Product: Service Port Mapping Catalog, Pod Subdomain DNS, Container Working Dir Audit
type SvcPortMapResult2284 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices int            `json:"totalServices"`
		TotalPorts    int            `json:"totalPorts"`
		ByProtocol    map[string]int `json:"byProtocol"`
	} `json:"summary"`
}

func (s *Server) handleSvcPortMap2284(w http.ResponseWriter, r *http.Request) {
	result := SvcPortMapResult2284{ScannedAt: time.Now()}
	result.Summary.ByProtocol = make(map[string]int)
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		for _, p := range svc.Spec.Ports {
			result.Summary.TotalPorts++
			result.Summary.ByProtocol[string(p.Protocol)]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SubdomainDNSResult2284 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods     int `json:"totalPods"`
		WithSubdomain int `json:"withSubdomain"`
		WithDNSConfig int `json:"withDNSConfig"`
	} `json:"summary"`
}

func (s *Server) handleSubdomainDNS2284(w http.ResponseWriter, r *http.Request) {
	result := SubdomainDNSResult2284{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.Subdomain != "" {
			result.Summary.WithSubdomain++
		}
		if pod.Spec.DNSConfig != nil {
			result.Summary.WithDNSConfig++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type WorkDirResult2284 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int            `json:"totalContainers"`
		ByWorkDir       map[string]int `json:"byWorkDir"`
	} `json:"summary"`
}

func (s *Server) handleWorkDir2284(w http.ResponseWriter, r *http.Request) {
	result := WorkDirResult2284{ScannedAt: time.Now()}
	result.Summary.ByWorkDir = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			wd := c.WorkingDir
			if wd == "" {
				wd = "<default>"
			}
			result.Summary.ByWorkDir[wd]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
