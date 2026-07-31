package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.46 — Documentation Dimension (Round 60)
// 1. Node Operating System Distribution
// 2. ConfigMap Total Data Size Catalog
// 3. Pod Service Account Image Pull Secret Tracker
// ============================================================

type OSDistributionResult2246 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByOS       map[string]int `json:"byOperatingSystem"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleOSDistribution2246(w http.ResponseWriter, r *http.Request) {
	result := OSDistributionResult2246{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	result.Summary.ByOS = make(map[string]int)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.ByOS[node.Status.NodeInfo.OperatingSystem]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. CM Data Size Catalog
type CMDataSizeResult2246 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalCMs    int `json:"totalConfigMaps"`
		TotalSizeKB int `json:"totalDataSizeKB"`
		MaxSizeKB   int `json:"maxSizeKB"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleCMDataSize2246(w http.ResponseWriter, r *http.Request) {
	result := CMDataSizeResult2246{ScannedAt: time.Now()}
	score := 100
	cmList, _ := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	maxSize := 0
	for _, cm := range cmList.Items {
		result.Summary.TotalCMs++
		sizeKB := 0
		for _, v := range cm.Data {
			sizeKB += len(v) / 1024
		}
		result.Summary.TotalSizeKB += sizeKB
		if sizeKB > maxSize {
			maxSize = sizeKB
		}
	}
	result.Summary.MaxSizeKB = maxSize
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. SA Image Pull Secret Tracker
type SAPullSecretResult2246 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSAs       int `json:"totalServiceAccounts"`
		WithPullSecret int `json:"withImagePullSecrets"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleSAPullSecret2246(w http.ResponseWriter, r *http.Request) {
	result := SAPullSecretResult2246{ScannedAt: time.Now()}
	score := 100
	saList, _ := s.clientset.CoreV1().ServiceAccounts("").List(r.Context(), metav1.ListOptions{})
	for _, sa := range saList.Items {
		result.Summary.TotalSAs++
		if len(sa.ImagePullSecrets) > 0 {
			result.Summary.WithPullSecret++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
