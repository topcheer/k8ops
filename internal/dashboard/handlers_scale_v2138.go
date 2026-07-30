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
// v21.38 — Scalability & HA Dimension (Round 42)
// 1. CPU Limit Overcommit Per Node
// 2. Pod Scheduling Bin Pack Score
// 3. Namespace Workload HA Multiplier
// ============================================================

type CPUOvercommitNodeResult2138 struct {
	ScannedAt       time.Time                    `json:"scannedAt"`
	HealthScore     int                          `json:"healthScore"`
	Grade           string                       `json:"grade"`
	Summary         CPUOvercommitNodeSummary2138 `json:"summary"`
	OverNodes       []CPUOvercommitNodeEntry2138 `json:"overcommittedNodes"`
	Recommendations []string                     `json:"recommendations"`
}

type CPUOvercommitNodeSummary2138 struct {
	TotalNodes int `json:"totalNodes"`
	OverNodes  int `json:"overcommittedNodes"`
}

type CPUOvercommitNodeEntry2138 struct {
	Node     string  `json:"node"`
	LimCPU   float64 `json:"limitCPU"`
	AllocCPU float64 `json:"allocatableCPU"`
}

func (s *Server) handleCPUOvercommitNode2138(w http.ResponseWriter, r *http.Request) {
	result := CPUOvercommitNodeResult2138{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	limPerNode := make(map[string]float64)
	allocPerNode := make(map[string]float64)
	for _, node := range nodeList.Items {
		allocPerNode[node.Name] = node.Status.Allocatable.Cpu().AsApproximateFloat64()
	}
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning || pod.Spec.NodeName == "" {
			continue
		}
		for _, c := range pod.Spec.Containers {
			limPerNode[pod.Spec.NodeName] += c.Resources.Limits.Cpu().AsApproximateFloat64()
		}
	}

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		lim := limPerNode[node.Name]
		alloc := allocPerNode[node.Name]
		if alloc > 0 && lim > alloc {
			result.Summary.OverNodes++
			result.OverNodes = append(result.OverNodes, CPUOvercommitNodeEntry2138{Node: node.Name, LimCPU: lim, AllocCPU: alloc})
			score -= 5
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.OverNodes, func(i, j int) bool { return result.OverNodes[i].LimCPU > result.OverNodes[j].LimCPU })
	writeJSON(w, result)
}

// 2. Bin Pack Score
type BinPackResult2138 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         BinPackSummary2138 `json:"summary"`
	Recommendations []string           `json:"recommendations"`
}

type BinPackSummary2138 struct {
	TotalNodes   int `json:"totalNodes"`
	TotalPods    int `json:"totalPods"`
	BinPackScore int `json:"binPackScore"`
}

func (s *Server) handleBinPack2138(w http.ResponseWriter, r *http.Request) {
	result := BinPackResult2138{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	podsPerNode := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning && pod.Spec.NodeName != "" {
			podsPerNode[pod.Spec.NodeName]++
		}
	}

	result.Summary.TotalNodes = len(nodeList.Items)
	maxPods := 110
	for _, cnt := range podsPerNode {
		result.Summary.TotalPods += cnt
		if cnt > maxPods {
			maxPods = cnt
		}
	}
	if result.Summary.TotalNodes > 0 {
		result.Summary.BinPackScore = result.Summary.TotalPods * 100 / (result.Summary.TotalNodes * maxPods)
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.BinPackScore > 80 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("Bin pack score %d%% — high density", result.Summary.BinPackScore))
	}
	writeJSON(w, result)
}

// 3. NS HA Multiplier
type NSHAMultResult2138 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         NSHAMultSummary2138 `json:"summary"`
	LowHA           []NSHAMultEntry2138 `json:"lowHANamespaces"`
	Recommendations []string            `json:"recommendations"`
}

type NSHAMultSummary2138 struct {
	TotalNS int `json:"totalNamespaces"`
	LowHA   int `json:"lowHANamespaces"`
}

type NSHAMultEntry2138 struct {
	Namespace string `json:"namespace"`
	Replicas  int32  `json:"singleReplicaDeployments"`
}

func (s *Server) handleNSHAMult2138(w http.ResponseWriter, r *http.Request) {
	result := NSHAMultResult2138{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	nsSingleRep := make(map[string]int32)
	for _, dep := range deployList.Items {
		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		if replicas <= 1 {
			nsSingleRep[dep.Namespace]++
		}
	}

	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true, "k8ops-system": true}
	for _, dep := range deployList.Items {
		if systemNS[dep.Namespace] {
			continue
		}
	}
	for ns, cnt := range nsSingleRep {
		if systemNS[ns] {
			continue
		}
		result.Summary.TotalNS++
		if cnt >= 3 {
			result.Summary.LowHA++
			result.LowHA = append(result.LowHA, NSHAMultEntry2138{Namespace: ns, Replicas: cnt})
			score -= 2
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.LowHA, func(i, j int) bool { return result.LowHA[i].Replicas > result.LowHA[j].Replicas })
	writeJSON(w, result)
}
