package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.86 — Documentation Dimension (Round 50)
// 1. Node Boot ID Catalog
// 2. PVC Selected Node Tracker
// 3. Pod HostAliases Distribution
// ============================================================

type BootIDResult2186 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByBootID   map[string]int `json:"byBootID"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleBootID2186(w http.ResponseWriter, r *http.Request) {
	result := BootIDResult2186{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	result.Summary.ByBootID = make(map[string]int)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		bootID := node.Status.NodeInfo.BootID
		if len(bootID) > 8 {
			bootID = bootID[:8]
		}
		result.Summary.ByBootID[bootID]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. PVC Selected Node
type PVCNodeResult2186 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPVCs int `json:"totalPVCs"`
		BoundPVCs int `json:"boundPVCs"`
		Pending   int `json:"pendingPVCs"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handlePVCNode2186(w http.ResponseWriter, r *http.Request) {
	result := PVCNodeResult2186{ScannedAt: time.Now()}
	score := 100
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		if pvc.Status.Phase == corev1.ClaimBound {
			result.Summary.BoundPVCs++
		} else {
			result.Summary.Pending++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. HostAliases Distribution
type HostAliasResult2186 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods    int `json:"totalPods"`
		WithAliases  int `json:"withHostAliases"`
		TotalAliases int `json:"totalAliases"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleHostAlias2186(w http.ResponseWriter, r *http.Request) {
	result := HostAliasResult2186{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.Spec.HostAliases) > 0 {
			result.Summary.WithAliases++
			result.Summary.TotalAliases += len(pod.Spec.HostAliases)
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
