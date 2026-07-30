package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.60 — Operations Dimension (Round 46)
// 1. Node Allocatable Memory Efficiency
// 2. Pod Container Port Protocol Distribution
// 3. Service Endpoint Slice Address Count
// ============================================================

type NodeMemEffResult2160 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes    int     `json:"totalNodes"`
		AllocMemGB    float64 `json:"allocatableMemGB"`
		ReqMemGB      float64 `json:"requestedMemGB"`
		EfficiencyPct int     `json:"efficiencyPct"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleNodeMemEff2160(w http.ResponseWriter, r *http.Request) {
	result := NodeMemEffResult2160{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.AllocMemGB += node.Status.Allocatable.Memory().AsApproximateFloat64() / 1e9
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.ReqMemGB += c.Resources.Requests.Memory().AsApproximateFloat64() / 1e9
		}
	}
	if result.Summary.AllocMemGB > 0 {
		result.Summary.EfficiencyPct = int(result.Summary.ReqMemGB / result.Summary.AllocMemGB * 100)
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Port Protocol Distribution
type PortProtoResult2160 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPorts int            `json:"totalPorts"`
		ByProtocol map[string]int `json:"byProtocol"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handlePortProto2160(w http.ResponseWriter, r *http.Request) {
	result := PortProtoResult2160{ScannedAt: time.Now()}
	score := 100
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByProtocol = make(map[string]int)
	for _, svc := range svcList.Items {
		for _, p := range svc.Spec.Ports {
			result.Summary.TotalPorts++
			result.Summary.ByProtocol[string(p.Protocol)]++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Endpoint Slice Address Count
type EPSAddrResult2160 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSlices int `json:"totalSlices"`
		TotalAddrs  int `json:"totalAddresses"`
		ReadyAddrs  int `json:"readyAddresses"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleEPSAddr2160(w http.ResponseWriter, r *http.Request) {
	result := EPSAddrResult2160{ScannedAt: time.Now()}
	score := 100
	epList, _ := s.clientset.DiscoveryV1().EndpointSlices("").List(r.Context(), metav1.ListOptions{})
	for _, eps := range epList.Items {
		result.Summary.TotalSlices++
		for _, ep := range eps.Endpoints {
			result.Summary.TotalAddrs += len(ep.Addresses)
			if ep.Conditions.Ready != nil && *ep.Conditions.Ready {
				result.Summary.ReadyAddrs++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
