package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v26.15 Deployment: RS Conditions Count, STS HasVolumeClaimTemplates, DS HasHostPID
type RSConditionsCount2615Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalRS  int `json:"totalRS"`
		WithCond int `json:"withConditions"`
	} `json:"summary"`
}

func (s *Server) handleRSConditionsCount2615(w http.ResponseWriter, r *http.Request) {
	result := RSConditionsCount2615Result{ScannedAt: time.Now()}
	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	for _, rs := range rsList.Items {
		result.Summary.TotalRS++
		if len(rs.Status.Conditions) > 0 {
			result.Summary.WithCond++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type STSHasVCT2615Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSTS int `json:"totalSTS"`
		WithVCT  int `json:"withVCT"`
	} `json:"summary"`
}

func (s *Server) handleSTSHasVCT2615(w http.ResponseWriter, r *http.Request) {
	result := STSHasVCT2615Result{ScannedAt: time.Now()}
	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++
		if len(sts.Spec.VolumeClaimTemplates) > 0 {
			result.Summary.WithVCT++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type DSHasHostPID2615Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDS int `json:"totalDS"`
		HostPID int `json:"hostPID"`
	} `json:"summary"`
}

func (s *Server) handleDSHasHostPID2615(w http.ResponseWriter, r *http.Request) {
	result := DSHasHostPID2615Result{ScannedAt: time.Now()}
	dsList, _ := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	for _, ds := range dsList.Items {
		result.Summary.TotalDS++
		if ds.Spec.Template.Spec.HostPID {
			result.Summary.HostPID++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
