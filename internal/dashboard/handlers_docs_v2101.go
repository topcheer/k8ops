package dashboard

import (
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.01 — Documentation Dimension (Round 36)
// 1. Node Boot ID Catalog — boot ID per node for restart detection
// 2. Service Type Histogram — ClusterIP/NodePort/LB distribution
// 3. Pod DNS Config Catalog — custom dnsConfig documentation
// ============================================================

type BootIDResult2101 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         BootIDSummary2101 `json:"summary"`
	Nodes           []BootIDEntry2101 `json:"nodes"`
	Recommendations []string          `json:"recommendations"`
}

type BootIDSummary2101 struct {
	TotalNodes int `json:"totalNodes"`
}

type BootIDEntry2101 struct {
	Node   string `json:"node"`
	BootID string `json:"bootID"`
}

func (s *Server) handleBootID2101(w http.ResponseWriter, r *http.Request) {
	result := BootIDResult2101{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Nodes = append(result.Nodes, BootIDEntry2101{Node: node.Name, BootID: node.Status.NodeInfo.BootID})
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.Nodes, func(i, j int) bool { return result.Nodes[i].Node < result.Nodes[j].Node })
	writeJSON(w, result)
}

// 2. Service Type Histogram
type SvcHistResult2101 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         SvcHistSummary2101 `json:"summary"`
	Recommendations []string           `json:"recommendations"`
}

type SvcHistSummary2101 struct {
	TotalServices int            `json:"totalServices"`
	TypeCounts    map[string]int `json:"typeCounts"`
}

func (s *Server) handleSvcHist2101(w http.ResponseWriter, r *http.Request) {
	result := SvcHistResult2101{ScannedAt: time.Now()}
	score := 100
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})

	typeCounts := make(map[string]int)
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		typeCounts[string(svc.Spec.Type)]++
	}
	result.Summary.TypeCounts = typeCounts
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Pod DNS Config Catalog
type DNSConfigResult2101 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         DNSConfigSummary2101 `json:"summary"`
	Recommendations []string             `json:"recommendations"`
}

type DNSConfigSummary2101 struct {
	TotalPods     int `json:"totalPods"`
	WithCustomDNS int `json:"withCustomDNSConfig"`
}

func (s *Server) handleDNSConfig2101(w http.ResponseWriter, r *http.Request) {
	result := DNSConfigResult2101{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.DNSConfig != nil {
			result.Summary.WithCustomDNS++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
