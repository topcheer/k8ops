package dashboard

import (
	"net/http"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v20.89 — Documentation Dimension (Round 34)
// 1. Node Capacity Summary — total cluster resources catalog
// 2. Secret Key Count Inventory — keys per secret
// 3. Namespace Quota Catalog — ResourceQuota documentation
// ============================================================

type NodeCapSumResult2089 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         NodeCapSumSummary2089 `json:"summary"`
	Recommendations []string              `json:"recommendations"`
}

type NodeCapSumSummary2089 struct {
	TotalNodes int     `json:"totalNodes"`
	TotalCPU   float64 `json:"totalCPU"`
	TotalMem   float64 `json:"totalMemGB"`
	TotalPods  int     `json:"totalPodCapacity"`
}

func (s *Server) handleNodeCapSum2089(w http.ResponseWriter, r *http.Request) {
	result := NodeCapSumResult2089{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Summary.TotalCPU += node.Status.Capacity.Cpu().AsApproximateFloat64()
		result.Summary.TotalMem += node.Status.Capacity.Memory().AsApproximateFloat64() / 1e9
		pods := node.Status.Capacity.Pods()
		if pods != nil {
			result.Summary.TotalPods += int(pods.AsApproximateFloat64())
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Secret Key Count Inventory
type SecKeyCountResult2089 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         SecKeyCountSummary2089 `json:"summary"`
	LargeSecrets    []SecKeyCountEntry2089 `json:"largeSecrets"`
	Recommendations []string               `json:"recommendations"`
}

type SecKeyCountSummary2089 struct {
	TotalSecrets int `json:"totalSecrets"`
	LargeSecrets int `json:"largeSecrets"`
}

type SecKeyCountEntry2089 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	KeyCount  int    `json:"keyCount"`
}

func (s *Server) handleSecKeyCount2089(w http.ResponseWriter, r *http.Request) {
	result := SecKeyCountResult2089{ScannedAt: time.Now()}
	score := 100
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})

	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		keyCount := len(secret.Data)
		if keyCount > 20 {
			result.Summary.LargeSecrets++
			result.LargeSecrets = append(result.LargeSecrets, SecKeyCountEntry2089{
				Name: secret.Name, Namespace: secret.Namespace, KeyCount: keyCount,
			})
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.LargeSecrets, func(i, j int) bool { return result.LargeSecrets[i].KeyCount > result.LargeSecrets[j].KeyCount })
	writeJSON(w, result)
}

// 3. Namespace Quota Catalog
type QuotaCatResult2089 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         QuotaCatSummary2089 `json:"summary"`
	Recommendations []string            `json:"recommendations"`
}

type QuotaCatSummary2089 struct {
	TotalNS      int `json:"totalNamespaces"`
	WithQuota    int `json:"withQuota"`
	WithoutQuota int `json:"withoutQuota"`
}

func (s *Server) handleQuotaCat2089(w http.ResponseWriter, r *http.Request) {
	result := QuotaCatResult2089{ScannedAt: time.Now()}
	score := 100
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	rqList, _ := s.clientset.CoreV1().ResourceQuotas("").List(r.Context(), metav1.ListOptions{})

	nsWithQuota := make(map[string]bool)
	for _, rq := range rqList.Items {
		nsWithQuota[rq.Namespace] = true
	}

	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true, "k8ops-system": true}
	for _, ns := range nsList.Items {
		if systemNS[ns.Name] {
			continue
		}
		result.Summary.TotalNS++
		if nsWithQuota[ns.Name] {
			result.Summary.WithQuota++
		} else {
			result.Summary.WithoutQuota++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
