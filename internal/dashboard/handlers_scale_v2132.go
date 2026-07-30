package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.32 — Scalability & HA Dimension (Round 41)
// 1. Memory Request Efficiency by NS
// 2. Pod Density Risk Zone — pods near max per node
// 3. Deployment HPA Target Match
// ============================================================

type MemEffNSResult2132 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         MemEffNSSummary2132 `json:"summary"`
	TopNS           []MemEffNSEntry2132 `json:"topNamespaces"`
	Recommendations []string            `json:"recommendations"`
}

type MemEffNSSummary2132 struct {
	TotalNS int `json:"totalNamespaces"`
}

type MemEffNSEntry2132 struct {
	Namespace string  `json:"namespace"`
	MemReq    float64 `json:"memRequestGB"`
}

func (s *Server) handleMemEffNS2132(w http.ResponseWriter, r *http.Request) {
	result := MemEffNSResult2132{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	nsMem := make(map[string]float64)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			nsMem[pod.Namespace] += c.Resources.Requests.Memory().AsApproximateFloat64() / 1e9
		}
	}
	result.Summary.TotalNS = len(nsMem)
	for ns, mem := range nsMem {
		result.TopNS = append(result.TopNS, MemEffNSEntry2132{Namespace: ns, MemReq: mem})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].MemReq > result.TopNS[j].MemReq })
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Pod Density Risk Zone
type DensityRiskResult2132 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         DensityRiskSummary2132 `json:"summary"`
	AtRiskNodes     []DensityRiskEntry2132 `json:"atRiskNodes"`
	Recommendations []string               `json:"recommendations"`
}

type DensityRiskSummary2132 struct {
	TotalNodes  int `json:"totalNodes"`
	AtRiskNodes int `json:"atRiskNodes"`
}

type DensityRiskEntry2132 struct {
	Node     string `json:"node"`
	PodCount int    `json:"podCount"`
	MaxPods  int    `json:"maxPods"`
}

func (s *Server) handleDensityRisk2132(w http.ResponseWriter, r *http.Request) {
	result := DensityRiskResult2132{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	podsPerNode := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning && pod.Spec.NodeName != "" {
			podsPerNode[pod.Spec.NodeName]++
		}
	}

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		cnt := podsPerNode[node.Name]
		maxP := 110
		pods := node.Status.Allocatable.Pods()
		if pods != nil && !pods.IsZero() {
			maxP = int(pods.AsApproximateFloat64())
		}
		if maxP > 0 && cnt >= maxP*80/100 {
			result.Summary.AtRiskNodes++
			result.AtRiskNodes = append(result.AtRiskNodes, DensityRiskEntry2132{Node: node.Name, PodCount: cnt, MaxPods: maxP})
			score -= 5
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.AtRiskNodes > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d nodes at >80%% pod density", result.Summary.AtRiskNodes))
	}
	writeJSON(w, result)
}

// 3. HPA Target Match
type HPATargetResult2132 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         HPATargetSummary2132 `json:"summary"`
	Mismatched      []HPATargetEntry2132 `json:"mismatchedHPAs"`
	Recommendations []string             `json:"recommendations"`
}

type HPATargetSummary2132 struct {
	TotalHPAs  int `json:"totalHPAs"`
	Mismatched int `json:"mismatched"`
}

type HPATargetEntry2132 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleHPATarget2132(w http.ResponseWriter, r *http.Request) {
	result := HPATargetResult2132{ScannedAt: time.Now()}
	score := 100
	hpaList, _ := s.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(r.Context(), metav1.ListOptions{})
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	depSet := make(map[string]bool)
	for _, dep := range deployList.Items {
		depSet[dep.Namespace+"/"+dep.Name] = true
	}

	for _, hpa := range hpaList.Items {
		result.Summary.TotalHPAs++
		target := hpa.Spec.ScaleTargetRef
		key := hpa.Namespace + "/" + target.Name
		if target.Kind == "Deployment" && !depSet[key] {
			result.Summary.Mismatched++
			result.Mismatched = append(result.Mismatched, HPATargetEntry2132{Name: hpa.Name, Namespace: hpa.Namespace})
			score -= 5
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.Mismatched, func(i, j int) bool { return result.Mismatched[i].Namespace < result.Mismatched[j].Namespace })
	writeJSON(w, result)
}
