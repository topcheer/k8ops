package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.98 — Documentation Dimension (Round 52)
// 1. Node Container Runtime ID Catalog
// 2. Service Port Name Distribution
// 3. PVC Data Source Ref Catalog
// ============================================================

type CRIDResult2198 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByCRID     map[string]int `json:"byContainerRuntimeID"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleCRID2198(w http.ResponseWriter, r *http.Request) {
	result := CRIDResult2198{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	result.Summary.ByCRID = make(map[string]int)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		crID := node.Status.NodeInfo.ContainerRuntimeVersion
		if len(crID) > 20 {
			crID = crID[:20]
		}
		result.Summary.ByCRID[crID]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Service Port Name Distribution
type PortNameResult2198 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPorts int            `json:"totalPorts"`
		WithName   int            `json:"withName"`
		ByName     map[string]int `json:"byName"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handlePortName2198(w http.ResponseWriter, r *http.Request) {
	result := PortNameResult2198{ScannedAt: time.Now()}
	score := 100
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByName = make(map[string]int)
	for _, svc := range svcList.Items {
		for _, p := range svc.Spec.Ports {
			result.Summary.TotalPorts++
			if p.Name != "" {
				result.Summary.WithName++
				result.Summary.ByName[p.Name]++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. PVC Data Source Ref
type PVCDataSourceResult2198 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPVCs  int `json:"totalPVCs"`
		WithSource int `json:"withDataSource"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handlePVCDataSource2198(w http.ResponseWriter, r *http.Request) {
	result := PVCDataSourceResult2198{ScannedAt: time.Now()}
	score := 100
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		if pvc.Spec.DataSource != nil {
			result.Summary.WithSource++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
