package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.07 — Documentation Dimension (Round 37)
// 1. PVC Storage Class Distribution
// 2. Node Container Runtime Version Map
// 3. Pod Restart Policy Catalog
// ============================================================

type PVCSCResult2107 struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Summary         PVCSCSummary2107 `json:"summary"`
	Recommendations []string         `json:"recommendations"`
}

type PVCSCSummary2107 struct {
	TotalPVCs int            `json:"totalPVCs"`
	BySC      map[string]int `json:"byStorageClass"`
}

func (s *Server) handlePVCSC2107(w http.ResponseWriter, r *http.Request) {
	result := PVCSCResult2107{ScannedAt: time.Now()}
	score := 100
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})

	bySC := make(map[string]int)
	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		sc := pvc.Spec.StorageClassName
		if sc == nil || *sc == "" {
			sc = ptrString2107("default")
		}
		bySC[*sc]++
	}
	result.Summary.BySC = bySC
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

func ptrString2107(s string) *string { return &s }

// 2. Runtime Version Map
type RTVerResult2107 struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Summary         RTVerSummary2107 `json:"summary"`
	Recommendations []string         `json:"recommendations"`
}

type RTVerSummary2107 struct {
	TotalNodes int            `json:"totalNodes"`
	Versions   map[string]int `json:"runtimeVersions"`
}

func (s *Server) handleRTVer2107(w http.ResponseWriter, r *http.Request) {
	result := RTVerResult2107{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	versions := make(map[string]int)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		versions[node.Status.NodeInfo.ContainerRuntimeVersion]++
	}
	result.Summary.Versions = versions
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Restart Policy Catalog
type RestPolResult2107 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         RestPolSummary2107 `json:"summary"`
	Recommendations []string           `json:"recommendations"`
}

type RestPolSummary2107 struct {
	TotalPods int            `json:"totalPods"`
	ByPolicy  map[string]int `json:"byPolicy"`
}

func (s *Server) handleRestPol2107(w http.ResponseWriter, r *http.Request) {
	result := RestPolResult2107{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	byPol := make(map[string]int)
	for _, pod := range podList.Items {
		result.Summary.TotalPods++
		policy := string(pod.Spec.RestartPolicy)
		byPol[policy]++
	}
	result.Summary.ByPolicy = byPol
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
