package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v26.37 Scalability: Top Namespace by HPA, Node Storage Allocatable vs Capacity, Cluster PDB MinAvailable
type TopNSByHPA2637Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS  int `json:"totalNamespaces"`
		TotalHPA int `json:"totalHPA"`
	}
}

func (s *Server) handleTopNSByHPA2637(w http.ResponseWriter, r *http.Request) {
	result := TopNSByHPA2637Result{ScannedAt: time.Now()}
	hpaList, _ := s.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(r.Context(), metav1.ListOptions{})
	nsHPA := make(map[string]int)
	for _, hpa := range hpaList.Items {
		nsHPA[hpa.Namespace]++
	}
	result.Summary.TotalNS = len(nsHPA)
	for _, count := range nsHPA {
		result.Summary.TotalHPA += count
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeStorAllocVsCap2637Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalAlloc float64 `json:"totalAllocGB"`
		TotalCap   float64 `json:"totalCapGB"`
	} `json:"summary"`
}

func (s *Server) handleNodeStorAllocVsCap2637(w http.ResponseWriter, r *http.Request) {
	result := NodeStorAllocVsCap2637Result{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalAlloc += node.Status.Allocatable.StorageEphemeral().AsApproximateFloat64() / (1024 * 1024 * 1024)
		result.Summary.TotalCap += node.Status.Capacity.StorageEphemeral().AsApproximateFloat64() / (1024 * 1024 * 1024)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PDBMinAvailable2637Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPDBs int            `json:"totalPDBs"`
		ByType    map[string]int `json:"byType"`
	} `json:"summary"`
}

func (s *Server) handlePDBMinAvailable2637(w http.ResponseWriter, r *http.Request) {
	result := PDBMinAvailable2637Result{ScannedAt: time.Now()}
	result.Summary.ByType = make(map[string]int)
	pdbList, _ := s.clientset.PolicyV1().PodDisruptionBudgets("").List(r.Context(), metav1.ListOptions{})
	for _, pdb := range pdbList.Items {
		result.Summary.TotalPDBs++
		t := "MinAvailable"
		if pdb.Spec.MaxUnavailable != nil {
			t = "MaxUnavailable"
		}
		result.Summary.ByType[t]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
