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
// v20.72 — Scalability & HA Dimension (Round 31)
// 1. Pod Distribution Balance — pods per node variance
// 2. Resource Request Saturation — total requests vs allocatable
// 3. Volume Attachment Spread — PVC attachment distribution
// ============================================================

type PodBalResult2072 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         PodBalSummary2072 `json:"summary"`
	Unbalanced      []PodBalEntry2072 `json:"unbalancedNodes"`
	Recommendations []string          `json:"recommendations"`
}

type PodBalSummary2072 struct {
	TotalNodes int `json:"totalNodes"`
	TotalPods  int `json:"totalPods"`
	MaxPerNode int `json:"maxPerNode"`
	MinPerNode int `json:"minPerNode"`
}

type PodBalEntry2072 struct {
	Node     string `json:"node"`
	PodCount int    `json:"podCount"`
}

func (s *Server) handlePodDistBalance(w http.ResponseWriter, r *http.Request) {
	result := PodBalResult2072{ScannedAt: time.Now()}
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
	maxP := 0
	minP := 999999
	for _, cnt := range podsPerNode {
		result.Summary.TotalPods += cnt
		if cnt > maxP {
			maxP = cnt
		}
		if cnt < minP {
			minP = cnt
		}
	}
	if minP == 999999 {
		minP = 0
	}
	result.Summary.MaxPerNode = maxP
	result.Summary.MinPerNode = minP

	// Flag nodes significantly above average
	avg := result.Summary.TotalPods
	if result.Summary.TotalNodes > 0 {
		avg = result.Summary.TotalPods / result.Summary.TotalNodes
	}
	for _, node := range nodeList.Items {
		cnt := podsPerNode[node.Name]
		if avg > 0 && cnt > avg*2 {
			result.Unbalanced = append(result.Unbalanced, PodBalEntry2072{Node: node.Name, PodCount: cnt})
			score -= 5
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.Unbalanced, func(i, j int) bool { return result.Unbalanced[i].PodCount > result.Unbalanced[j].PodCount })

	if len(result.Unbalanced) > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d nodes significantly above average pod count", len(result.Unbalanced)))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Resource Request Saturation
// ---------------------------------------------------------------

type ReqSatResult2072 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         ReqSatSummary2072 `json:"summary"`
	Recommendations []string          `json:"recommendations"`
}

type ReqSatSummary2072 struct {
	AllocatableCPU float64 `json:"allocatableCPU"`
	RequestedCPU   float64 `json:"requestedCPU"`
	CPUSaturation  int     `json:"cpuSaturationPct"`
	AllocatableMem float64 `json:"allocatableMemGB"`
	RequestedMem   float64 `json:"requestedMemGB"`
	MemSaturation  int     `json:"memSaturationPct"`
}

func (s *Server) handleReqSaturation2072(w http.ResponseWriter, r *http.Request) {
	result := ReqSatResult2072{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, node := range nodeList.Items {
		result.Summary.AllocatableCPU += node.Status.Allocatable.Cpu().AsApproximateFloat64()
		result.Summary.AllocatableMem += node.Status.Allocatable.Memory().AsApproximateFloat64() / 1e9
	}

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.RequestedCPU += c.Resources.Requests.Cpu().AsApproximateFloat64()
			result.Summary.RequestedMem += c.Resources.Requests.Memory().AsApproximateFloat64() / 1e9
		}
	}

	if result.Summary.AllocatableCPU > 0 {
		result.Summary.CPUSaturation = int(result.Summary.RequestedCPU / result.Summary.AllocatableCPU * 100)
	}
	if result.Summary.AllocatableMem > 0 {
		result.Summary.MemSaturation = int(result.Summary.RequestedMem / result.Summary.AllocatableMem * 100)
	}

	if result.Summary.CPUSaturation > 80 {
		score -= 20
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.CPUSaturation > 80 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("CPU request saturation at %d%% — add capacity", result.Summary.CPUSaturation))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Volume Attachment Spread
// ---------------------------------------------------------------

type VolAttachResult2072 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         VolAttachSummary2072 `json:"summary"`
	Recommendations []string             `json:"recommendations"`
}

type VolAttachSummary2072 struct {
	TotalPVCs    int `json:"totalPVCs"`
	AttachedPVCs int `json:"attachedPVCs"`
	DetachedPVCs int `json:"detachedPVCs"`
}

func (s *Server) handleVolAttachSpread(w http.ResponseWriter, r *http.Request) {
	result := VolAttachResult2072{ScannedAt: time.Now()}
	score := 100

	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	attachedPVCs := make(map[string]bool)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, vol := range pod.Spec.Volumes {
			if vol.PersistentVolumeClaim != nil {
				attachedPVCs[pod.Namespace+"/"+vol.PersistentVolumeClaim.ClaimName] = true
			}
		}
	}

	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		key := pvc.Namespace + "/" + pvc.Name
		if attachedPVCs[key] {
			result.Summary.AttachedPVCs++
		} else {
			result.Summary.DetachedPVCs++
		}
	}

	if result.Summary.DetachedPVCs > 10 {
		score -= 10
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.DetachedPVCs > 10 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d detached PVCs — review for cleanup", result.Summary.DetachedPVCs))
	}
	writeJSON(w, result)
}
