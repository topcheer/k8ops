package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.43 — Documentation Dimension (Round 43)
// 1. Node Address Type Catalog
// 2. PVC DataSource Origin Tracker
// 3. Pod Subdomain Catalog
// ============================================================

type NodeAddrResult2143 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         NodeAddrSummary2143 `json:"summary"`
	Recommendations []string            `json:"recommendations"`
}

type NodeAddrSummary2143 struct {
	TotalNodes int            `json:"totalNodes"`
	ByAddrType map[string]int `json:"byAddressType"`
}

func (s *Server) handleNodeAddr2143(w http.ResponseWriter, r *http.Request) {
	result := NodeAddrResult2143{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	byAddr := make(map[string]int)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		for _, addr := range node.Status.Addresses {
			byAddr[string(addr.Type)]++
		}
	}
	result.Summary.ByAddrType = byAddr
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. PVC DataSource Origin
type PVCSourceResult2143 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         PVCSourceSummary2143 `json:"summary"`
	Recommendations []string             `json:"recommendations"`
}

type PVCSourceSummary2143 struct {
	TotalPVCs    int            `json:"totalPVCs"`
	WithSource   int            `json:"withDataSource"`
	BySourceKind map[string]int `json:"bySourceKind"`
}

func (s *Server) handlePVCSource2143(w http.ResponseWriter, r *http.Request) {
	result := PVCSourceResult2143{ScannedAt: time.Now()}
	score := 100
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})

	byKind := make(map[string]int)
	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		if pvc.Spec.DataSource != nil {
			result.Summary.WithSource++
			byKind[pvc.Spec.DataSource.Kind]++
		}
	}
	result.Summary.BySourceKind = byKind
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Pod Subdomain Catalog
type SubdomainResult2143 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         SubdomainSummary2143 `json:"summary"`
	Recommendations []string             `json:"recommendations"`
}

type SubdomainSummary2143 struct {
	TotalPods     int `json:"totalPods"`
	WithSubdomain int `json:"withSubdomain"`
}

func (s *Server) handleSubdomain2143(w http.ResponseWriter, r *http.Request) {
	result := SubdomainResult2143{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.Subdomain != "" {
			result.Summary.WithSubdomain++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
