package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v24.74 Documentation: Node BootID Distribution, Pod Subdomain Usage, Ingress Hostname Count
type NodeBootIDResult2474 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int `json:"totalNodes"`
		UniqueBoot int `json:"uniqueBootIDs"`
	} `json:"summary"`
}

func (s *Server) handleNodeBootID2474(w http.ResponseWriter, r *http.Request) {
	result := NodeBootIDResult2474{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	seen := make(map[string]bool)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		bid := node.Status.NodeInfo.BootID
		if bid != "" && !seen[bid] {
			seen[bid] = true
			result.Summary.UniqueBoot++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodSubdomainResult2474 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods  int `json:"totalPods"`
		WithSubdom int `json:"withSubdomain"`
	} `json:"summary"`
}

func (s *Server) handlePodSubdomain2474(w http.ResponseWriter, r *http.Request) {
	result := PodSubdomainResult2474{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.Subdomain != "" {
			result.Summary.WithSubdom++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type IngressHostnameResult2474 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalIngress int `json:"totalIngress"`
		TotalHosts   int `json:"totalHostnames"`
	} `json:"summary"`
}

func (s *Server) handleIngressHostname2474(w http.ResponseWriter, r *http.Request) {
	result := IngressHostnameResult2474{ScannedAt: time.Now()}
	ingList, _ := s.clientset.NetworkingV1().Ingresses("").List(r.Context(), metav1.ListOptions{})
	for _, ing := range ingList.Items {
		result.Summary.TotalIngress++
		for _, rule := range ing.Spec.Rules {
			if rule.Host != "" {
				result.Summary.TotalHosts++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
