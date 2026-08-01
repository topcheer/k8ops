package dashboard

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v24.33 Scalability: Top Namespace by SA, Node Storage Capacity, Cluster Secret Data Total Bytes
type TopNSSAResult2433 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNS"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string `json:"namespace"`
		SACount   int    `json:"saCount"`
	} `json:"topNS"`
}

func (s *Server) handleTopNSSA2433(w http.ResponseWriter, r *http.Request) {
	result := TopNSSAResult2433{ScannedAt: time.Now()}
	saList, _ := s.clientset.CoreV1().ServiceAccounts("").List(r.Context(), metav1.ListOptions{})
	nsSAs := make(map[string]int)
	for _, sa := range saList.Items {
		nsSAs[sa.Namespace]++
	}
	result.Summary.TotalNS = len(nsSAs)
	for ns, count := range nsSAs {
		result.TopNS = append(result.TopNS, struct {
			Namespace string `json:"namespace"`
			SACount   int    `json:"saCount"`
		}{ns, count})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].SACount > result.TopNS[j].SACount })
	if len(result.TopNS) > 10 {
		result.TopNS = result.TopNS[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeStorCapResult2433 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int     `json:"totalNodes"`
		TotalGB    float64 `json:"totalCapacityGB"`
	} `json:"summary"`
}

func (s *Server) handleNodeStorCap2433(w http.ResponseWriter, r *http.Request) {
	result := NodeStorCapResult2433{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalGB += node.Status.Capacity.Storage().AsApproximateFloat64() / 1e9
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SecretBytesResult2433 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int `json:"totalSecrets"`
		TotalBytes   int `json:"totalDataBytes"`
	} `json:"summary"`
}

func (s *Server) handleSecretBytes2433(w http.ResponseWriter, r *http.Request) {
	result := SecretBytesResult2433{ScannedAt: time.Now()}
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		for _, v := range secret.Data {
			result.Summary.TotalBytes += len(v)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
