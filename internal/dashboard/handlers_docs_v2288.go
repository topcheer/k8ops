package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"strings"
	"time"
)

// v22.88 Documentation: Endpoint Subset Catalog, Node Allocatable IP Range, Service Session Affinity
type EndpointSubsetResult2288 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalEndpoints int `json:"totalEndpoints"`
		TotalAddresses int `json:"totalAddresses"`
		TotalNotReady  int `json:"totalNotReadyAddresses"`
	} `json:"summary"`
}

func (s *Server) handleEndpointSubset2288(w http.ResponseWriter, r *http.Request) {
	result := EndpointSubsetResult2288{ScannedAt: time.Now()}
	epList, _ := s.clientset.CoreV1().Endpoints("").List(r.Context(), metav1.ListOptions{})
	for _, ep := range epList.Items {
		result.Summary.TotalEndpoints++
		for _, sub := range ep.Subsets {
			result.Summary.TotalAddresses += len(sub.Addresses)
			result.Summary.TotalNotReady += len(sub.NotReadyAddresses)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeIPRangeResult2288 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByCIDR     map[string]int `json:"byInternalIPPrefix"`
	} `json:"summary"`
}

func (s *Server) handleNodeIPRange2288(w http.ResponseWriter, r *http.Request) {
	result := NodeIPRangeResult2288{ScannedAt: time.Now()}
	result.Summary.ByCIDR = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		for _, addr := range node.Status.Addresses {
			if addr.Type == corev1.NodeInternalIP {
				// Extract /24 prefix
				parts := strings.SplitN(addr.Address, ".", 3)
				if len(parts) >= 3 {
					result.Summary.ByCIDR[parts[0]+"."+parts[1]+"."+parts[2]+".0/24"]++
				}
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SessionAffinityResult2288 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalServices int            `json:"totalServices"`
		ByAffinity    map[string]int `json:"bySessionAffinity"`
	} `json:"summary"`
}

func (s *Server) handleSessionAffinity2288(w http.ResponseWriter, r *http.Request) {
	result := SessionAffinityResult2288{ScannedAt: time.Now()}
	result.Summary.ByAffinity = make(map[string]int)
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		result.Summary.ByAffinity[string(svc.Spec.SessionAffinity)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
