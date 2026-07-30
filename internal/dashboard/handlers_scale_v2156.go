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
// v21.56 — Scalability & HA Dimension (Round 45)
// 1. Node CPU Request Concentration
// 2. Pod Scheduling Affinity Cost Estimate
// 3. Namespace Multi-Workload HA Score
// ============================================================

type CPUConcResult2156 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         CPUConcSummary2156 `json:"summary"`
	Nodes           []CPUConcEntry2156 `json:"nodes"`
	Recommendations []string           `json:"recommendations"`
}

type CPUConcSummary2156 struct {
	TotalNodes int `json:"totalNodes"`
}

type CPUConcEntry2156 struct {
	Node   string  `json:"node"`
	ReqCPU float64 `json:"requestedCPU"`
}

func (s *Server) handleCPUConc2156(w http.ResponseWriter, r *http.Request) {
	result := CPUConcResult2156{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	reqPerNode := make(map[string]float64)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning || pod.Spec.NodeName == "" {
			continue
		}
		for _, c := range pod.Spec.Containers {
			reqPerNode[pod.Spec.NodeName] += c.Resources.Requests.Cpu().AsApproximateFloat64()
		}
	}
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		result.Nodes = append(result.Nodes, CPUConcEntry2156{Node: node.Name, ReqCPU: reqPerNode[node.Name]})
	}
	sort.Slice(result.Nodes, func(i, j int) bool { return result.Nodes[i].ReqCPU > result.Nodes[j].ReqCPU })
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Affinity Cost Estimate
type AffCostResult2156 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         AffCostSummary2156 `json:"summary"`
	Recommendations []string           `json:"recommendations"`
}

type AffCostSummary2156 struct {
	TotalDeploys int `json:"totalDeployments"`
	WithAffinity int `json:"withAffinityRules"`
	WithAntiAff  int `json:"withAntiAffinityRules"`
}

func (s *Server) handleAffCost2156(w http.ResponseWriter, r *http.Request) {
	result := AffCostResult2156{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		aff := dep.Spec.Template.Spec.Affinity
		if aff == nil {
			continue
		}
		if aff.NodeAffinity != nil || aff.PodAffinity != nil {
			result.Summary.WithAffinity++
		}
		if aff.PodAntiAffinity != nil {
			result.Summary.WithAntiAff++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.WithAffinity+result.Summary.WithAntiAff > 20 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d deployments with affinity rules — monitor scheduling latency", result.Summary.WithAffinity+result.Summary.WithAntiAff))
	}
	writeJSON(w, result)
}

// 3. NS Multi-Workload HA Score
type NSMultiHAResult2156 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         NSMultiHASummary2156 `json:"summary"`
	LowHA           []NSMultiHAEntry2156 `json:"lowHANamespaces"`
	Recommendations []string             `json:"recommendations"`
}

type NSMultiHASummary2156 struct {
	TotalNS int `json:"totalNamespaces"`
}

type NSMultiHAEntry2156 struct {
	Namespace    string `json:"namespace"`
	SingleRepDep int    `json:"singleReplicaDeployments"`
}

func (s *Server) handleNSMultiHA2156(w http.ResponseWriter, r *http.Request) {
	result := NSMultiHAResult2156{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	nsSingleRep := make(map[string]int)
	for _, dep := range deployList.Items {
		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		if replicas <= 1 {
			nsSingleRep[dep.Namespace]++
		}
	}
	result.Summary.TotalNS = len(nsSingleRep)
	for ns, cnt := range nsSingleRep {
		if cnt >= 2 {
			result.LowHA = append(result.LowHA, NSMultiHAEntry2156{Namespace: ns, SingleRepDep: cnt})
			score -= 1
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.LowHA, func(i, j int) bool { return result.LowHA[i].SingleRepDep > result.LowHA[j].SingleRepDep })
	writeJSON(w, result)
}
