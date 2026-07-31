package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v22.82 Documentation: ConfigMap Age Distribution, Namespace Phase Inventory, PVC Access Mode Catalog
type ConfigMapAgeResult2282 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalConfigMaps int            `json:"totalConfigMaps"`
		ByAgeBucket     map[string]int `json:"byAgeBucket"`
	} `json:"summary"`
}

func (s *Server) handleConfigMapAge2282(w http.ResponseWriter, r *http.Request) {
	result := ConfigMapAgeResult2282{ScannedAt: time.Now()}
	result.Summary.ByAgeBucket = make(map[string]int)
	cmList, _ := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	now := time.Now()
	for _, cm := range cmList.Items {
		result.Summary.TotalConfigMaps++
		age := now.Sub(cm.CreationTimestamp.Time)
		var bucket string
		switch {
		case age < 24*time.Hour:
			bucket = "<1d"
		case age < 7*24*time.Hour:
			bucket = "1-7d"
		case age < 30*24*time.Hour:
			bucket = "7-30d"
		case age < 90*24*time.Hour:
			bucket = "30-90d"
		default:
			bucket = "90d+"
		}
		result.Summary.ByAgeBucket[bucket]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NSPhaseResult2282 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int            `json:"totalNS"`
		ByPhase map[string]int `json:"byPhase"`
	} `json:"summary"`
}

func (s *Server) handleNSPhase2282(w http.ResponseWriter, r *http.Request) {
	result := NSPhaseResult2282{ScannedAt: time.Now()}
	result.Summary.ByPhase = make(map[string]int)
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	for _, ns := range nsList.Items {
		result.Summary.TotalNS++
		result.Summary.ByPhase[string(ns.Status.Phase)]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PVCAccessModeResult2282 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPVCs    int            `json:"totalPVCs"`
		ByAccessMode map[string]int `json:"byAccessMode"`
	} `json:"summary"`
}

func (s *Server) handlePVCAccessMode2282(w http.ResponseWriter, r *http.Request) {
	result := PVCAccessModeResult2282{ScannedAt: time.Now()}
	result.Summary.ByAccessMode = make(map[string]int)
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		for _, am := range pvc.Spec.AccessModes {
			result.Summary.ByAccessMode[string(am)]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
