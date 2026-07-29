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
// v20.96 — Scalability & HA Dimension (Round 35)
// 1. Resource Limit Coverage — containers with limits ratio
// 2. Node Failure Impact — pods affected by single node failure
// 3. PVC Storage Distribution — storage per namespace
// ============================================================

type LimitCovResult2096 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         LimitCovSummary2096 `json:"summary"`
	Recommendations []string            `json:"recommendations"`
}

type LimitCovSummary2096 struct {
	TotalContainers int `json:"totalContainers"`
	WithCPULimit    int `json:"withCPULimit"`
	WithMemLimit    int `json:"withMemLimit"`
	NoLimits        int `json:"noLimits"`
}

func (s *Server) handleLimitCov2096(w http.ResponseWriter, r *http.Request) {
	result := LimitCovResult2096{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			hasCPU := !c.Resources.Limits.Cpu().IsZero()
			hasMem := !c.Resources.Limits.Memory().IsZero()
			if hasCPU {
				result.Summary.WithCPULimit++
			}
			if hasMem {
				result.Summary.WithMemLimit++
			}
			if !hasCPU && !hasMem {
				result.Summary.NoLimits++
			}
		}
	}
	if result.Summary.NoLimits > 20 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.NoLimits > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d containers without resource limits", result.Summary.NoLimits))
	}
	writeJSON(w, result)
}

// 2. Node Failure Impact
type NodeFailResult2096 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         NodeFailSummary2096 `json:"summary"`
	Nodes           []NodeFailEntry2096 `json:"nodes"`
	Recommendations []string            `json:"recommendations"`
}

type NodeFailSummary2096 struct {
	TotalNodes     int `json:"totalNodes"`
	MaxPodsPerNode int `json:"maxPodsPerNode"`
}

type NodeFailEntry2096 struct {
	Node     string `json:"node"`
	PodCount int    `json:"podCount"`
}

func (s *Server) handleNodeFail2096(w http.ResponseWriter, r *http.Request) {
	result := NodeFailResult2096{ScannedAt: time.Now()}
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
	for _, node := range nodeList.Items {
		cnt := podsPerNode[node.Name]
		if cnt > result.Summary.MaxPodsPerNode {
			result.Summary.MaxPodsPerNode = cnt
		}
		result.Nodes = append(result.Nodes, NodeFailEntry2096{Node: node.Name, PodCount: cnt})
	}
	sort.Slice(result.Nodes, func(i, j int) bool { return result.Nodes[i].PodCount > result.Nodes[j].PodCount })
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. PVC Storage Distribution
type PVCStorResult2096 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         PVCStorSummary2096 `json:"summary"`
	TopNS           []PVCStorEntry2096 `json:"topNamespaces"`
	Recommendations []string           `json:"recommendations"`
}

type PVCStorSummary2096 struct {
	TotalPVCs    int `json:"totalPVCs"`
	TotalStorage int `json:"totalStorageGB"`
}

type PVCStorEntry2096 struct {
	Namespace string `json:"namespace"`
	PVCCount  int    `json:"pvcCount"`
	StorageGB int    `json:"storageGB"`
}

func (s *Server) handlePVCStor2096(w http.ResponseWriter, r *http.Request) {
	result := PVCStorResult2096{ScannedAt: time.Now()}
	score := 100
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})

	nsPVC := make(map[string]*PVCStorEntry2096)
	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		storage := 0
		if req := pvc.Spec.Resources.Requests.Storage(); req != nil {
			storage = int(req.AsApproximateFloat64() / 1e9)
		}
		result.Summary.TotalStorage += storage

		if nsPVC[pvc.Namespace] == nil {
			nsPVC[pvc.Namespace] = &PVCStorEntry2096{Namespace: pvc.Namespace}
		}
		nsPVC[pvc.Namespace].PVCCount++
		nsPVC[pvc.Namespace].StorageGB += storage
	}
	for _, entry := range nsPVC {
		result.TopNS = append(result.TopNS, *entry)
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].StorageGB > result.TopNS[j].StorageGB })
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
