package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.18 Documentation: Secret Age Distribution, PVC Finalizer Catalog, Node MachineID Census
type SecretAgeResult2318 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int            `json:"totalSecrets"`
		ByAgeBucket  map[string]int `json:"byAgeBucket"`
	} `json:"summary"`
}

func (s *Server) handleSecretAge2318(w http.ResponseWriter, r *http.Request) {
	result := SecretAgeResult2318{ScannedAt: time.Now()}
	result.Summary.ByAgeBucket = make(map[string]int)
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	now := time.Now()
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		age := now.Sub(secret.CreationTimestamp.Time)
		var bucket string
		switch {
		case age < 7*24*time.Hour:
			bucket = "<7d"
		case age < 30*24*time.Hour:
			bucket = "7-30d"
		case age < 90*24*time.Hour:
			bucket = "30-90d"
		case age < 365*24*time.Hour:
			bucket = "90-365d"
		default:
			bucket = "365d+"
		}
		result.Summary.ByAgeBucket[bucket]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PVCFinResult2318 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPVCs     int `json:"totalPVCs"`
		WithFinalizer int `json:"withFinalizer"`
	} `json:"summary"`
}

func (s *Server) handlePVCFin2318(w http.ResponseWriter, r *http.Request) {
	result := PVCFinResult2318{ScannedAt: time.Now()}
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		if len(pvc.ObjectMeta.Finalizers) > 0 {
			result.Summary.WithFinalizer++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeMachineIDResult2318 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes       int `json:"totalNodes"`
		UniqueMachineIDs int `json:"uniqueMachineIDs"`
	} `json:"summary"`
}

func (s *Server) handleNodeMachineID2318(w http.ResponseWriter, r *http.Request) {
	result := NodeMachineIDResult2318{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	seen := make(map[string]bool)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		seen[node.Status.NodeInfo.MachineID] = true
	}
	result.Summary.UniqueMachineIDs = len(seen)
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
