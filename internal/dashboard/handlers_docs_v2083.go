package dashboard

import (
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v20.83 — Documentation Dimension (Round 33)
// 1. Node Architecture Map — arch/os per node catalog
// 2. Event Age Distribution — event recency histogram
// 3. Service ClusterIP Range Usage — allocated ClusterIPs
// ============================================================

type NodeArchResult2083 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         NodeArchSummary2083 `json:"summary"`
	Nodes           []NodeArchEntry2083 `json:"nodes"`
	Recommendations []string            `json:"recommendations"`
}

type NodeArchSummary2083 struct {
	TotalNodes int `json:"totalNodes"`
	UniqueArch int `json:"uniqueArchitectures"`
}

type NodeArchEntry2083 struct {
	Node string `json:"node"`
	Arch string `json:"arch"`
	OS   string `json:"os"`
}

func (s *Server) handleNodeArch2083(w http.ResponseWriter, r *http.Request) {
	result := NodeArchResult2083{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	archSet := make(map[string]bool)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		arch := node.Status.NodeInfo.Architecture
		archSet[arch] = true
		result.Nodes = append(result.Nodes, NodeArchEntry2083{
			Node: node.Name, Arch: arch, OS: node.Status.NodeInfo.OperatingSystem,
		})
	}
	result.Summary.UniqueArch = len(archSet)
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.Nodes, func(i, j int) bool { return result.Nodes[i].Arch < result.Nodes[j].Arch })
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Event Age Distribution
// ---------------------------------------------------------------

type EventAgeResult2083 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         EventAgeSummary2083 `json:"summary"`
	Recommendations []string            `json:"recommendations"`
}

type EventAgeSummary2083 struct {
	TotalEvents int `json:"totalEvents"`
	LastHour    int `json:"lastHour"`
	LastDay     int `json:"lastDay"`
	Older       int `json:"older"`
}

func (s *Server) handleEventAgeDist2083(w http.ResponseWriter, r *http.Request) {
	result := EventAgeResult2083{ScannedAt: time.Now()}
	score := 100
	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})

	now := time.Now()
	for _, evt := range eventList.Items {
		result.Summary.TotalEvents++
		ageHours := now.Sub(evt.CreationTimestamp.Time).Hours()
		if ageHours < 1 {
			result.Summary.LastHour++
		} else if ageHours < 24 {
			result.Summary.LastDay++
		} else {
			result.Summary.Older++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Service ClusterIP Range Usage
// ---------------------------------------------------------------

type ClusterIPResult2083 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         ClusterIPSummary2083 `json:"summary"`
	Recommendations []string             `json:"recommendations"`
}

type ClusterIPSummary2083 struct {
	TotalServices int      `json:"totalServices"`
	ClusterIPs    []string `json:"clusterIPs"`
	NoneServices  int      `json:"noneClusterIPServices"`
}

func (s *Server) handleClusterIPUsage2083(w http.ResponseWriter, r *http.Request) {
	result := ClusterIPResult2083{ScannedAt: time.Now()}
	score := 100
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})

	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		if svc.Spec.ClusterIP == "None" || svc.Spec.ClusterIP == "" {
			result.Summary.NoneServices++
		} else {
			result.Summary.ClusterIPs = append(result.Summary.ClusterIPs, svc.Spec.ClusterIP)
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// keep import
var _ = corev1.Pod{}
