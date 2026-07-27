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
// v20.06 — Scalability & HA Dimension (Round 20 Final)
// 1. Node Cordon Readiness — cordon/drain state & schedulability
// 2. PV Reclaim Gap — released PVs & reclaim policy gap
// 3. Cluster Object Budget — total object count per type for API scaling
// ============================================================

// ---------------------------------------------------------------
// 1. Node Cordon Readiness
// ---------------------------------------------------------------

type CordoneResult2006 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         CordoneSummary2006 `json:"summary"`
	Nodes           []CordoneEntry2006 `json:"nodes"`
	Recommendations []string           `json:"recommendations"`
}

type CordoneSummary2006 struct {
	TotalNodes  int `json:"totalNodes"`
	Schedulable int `json:"schedulableNodes"`
	Cordoned    int `json:"cordonedNodes"`
	NotReady    int `json:"notReadyNodes"`
}

type CordoneEntry2006 struct {
	Name        string `json:"name"`
	Schedulable bool   `json:"schedulable"`
	Ready       bool   `json:"ready"`
	Status      string `json:"status"`
}

func (s *Server) handleNodeCordoneReadiness(w http.ResponseWriter, r *http.Request) {
	result := CordoneResult2006{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++

		isReady := false
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				isReady = true
			}
		}

		schedulable := !node.Spec.Unschedulable

		entry := CordoneEntry2006{
			Name: node.Name, Schedulable: schedulable, Ready: isReady,
		}

		if !schedulable {
			result.Summary.Cordoned++
			entry.Status = "cordoned"
			score -= 3
		} else if isReady {
			result.Summary.Schedulable++
			entry.Status = "ready"
		} else {
			result.Summary.NotReady++
			entry.Status = "not-ready"
			score -= 5
		}

		result.Nodes = append(result.Nodes, entry)
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d nodes: %d ready, %d cordoned, %d not-ready", result.Summary.TotalNodes, result.Summary.Schedulable, result.Summary.Cordoned, result.Summary.NotReady))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. PV Reclaim Gap
// ---------------------------------------------------------------

type PVReclaimResult2006 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         PVReclaimSummary2006 `json:"summary"`
	ReleasedPVs     []PVReclaimEntry2006 `json:"releasedPVs"`
	Recommendations []string             `json:"recommendations"`
}

type PVReclaimSummary2006 struct {
	TotalPVs     int `json:"totalPVs"`
	BoundPVs     int `json:"boundPVs"`
	ReleasedPVs  int `json:"releasedPVs"`
	AvailablePVs int `json:"availablePVs"`
	FailedPVs    int `json:"failedPVs"`
	DeletePolicy int `json:"deletePolicyCount"`
}

type PVReclaimEntry2006 struct {
	Name    string `json:"name"`
	Phase   string `json:"phase"`
	Reclaim string `json:"reclaimPolicy"`
	Size    string `json:"capacity"`
}

func (s *Server) handlePVReclaimGap(w http.ResponseWriter, r *http.Request) {
	result := PVReclaimResult2006{ScannedAt: time.Now()}
	score := 100

	pvList, _ := s.clientset.CoreV1().PersistentVolumes().List(r.Context(), metav1.ListOptions{})

	for _, pv := range pvList.Items {
		result.Summary.TotalPVs++

		entry := PVReclaimEntry2006{
			Name: pv.Name, Phase: string(pv.Status.Phase),
			Reclaim: string(pv.Spec.PersistentVolumeReclaimPolicy),
		}
		if pv.Spec.Capacity != nil {
			if cap, ok := pv.Spec.Capacity[corev1.ResourceStorage]; ok {
				entry.Size = cap.String()
			}
		}

		switch pv.Status.Phase {
		case corev1.VolumeBound:
			result.Summary.BoundPVs++
		case corev1.VolumeReleased:
			result.Summary.ReleasedPVs++
			result.ReleasedPVs = append(result.ReleasedPVs, entry)
			score -= 2
		case corev1.VolumeAvailable:
			result.Summary.AvailablePVs++
		case corev1.VolumeFailed:
			result.Summary.FailedPVs++
			result.ReleasedPVs = append(result.ReleasedPVs, entry)
			score -= 5
		}

		if pv.Spec.PersistentVolumeReclaimPolicy == corev1.PersistentVolumeReclaimDelete {
			result.Summary.DeletePolicy++
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d PVs: %d bound, %d released, %d available, %d failed", result.Summary.TotalPVs, result.Summary.BoundPVs, result.Summary.ReleasedPVs, result.Summary.AvailablePVs, result.Summary.FailedPVs))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Cluster Object Budget
// ---------------------------------------------------------------

type ObjBudgetResult2006 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         ObjBudgetSummary2006 `json:"summary"`
	PerType         []ObjBudgetEntry2006 `json:"perType"`
	Recommendations []string             `json:"recommendations"`
}

type ObjBudgetSummary2006 struct {
	TotalObjects int    `json:"totalObjects"`
	TopType      string `json:"topObjectType"`
	ScalingRisk  string `json:"scalingRisk"`
}

type ObjBudgetEntry2006 struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

func (s *Server) handleClusterObjBudget(w http.ResponseWriter, r *http.Request) {
	result := ObjBudgetResult2006{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	cmList, _ := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})

	types := []ObjBudgetEntry2006{
		{Type: "Pods", Count: len(podList.Items)},
		{Type: "Services", Count: len(svcList.Items)},
		{Type: "Namespaces", Count: len(nsList.Items)},
		{Type: "ConfigMaps", Count: len(cmList.Items)},
		{Type: "Secrets", Count: len(secretList.Items)},
		{Type: "PVCs", Count: len(pvcList.Items)},
	}

	total := 0
	topType := ""
	topCount := 0
	for _, t := range types {
		total += t.Count
		if t.Count > topCount {
			topCount = t.Count
			topType = t.Type
		}
		result.PerType = append(result.PerType, t)
	}

	sort.Slice(result.PerType, func(i, j int) bool {
		return result.PerType[i].Count > result.PerType[j].Count
	})

	result.Summary.TotalObjects = total
	result.Summary.TopType = topType

	if total > 5000 {
		result.Summary.ScalingRisk = "high"
		score -= 5
	} else if total > 2000 {
		result.Summary.ScalingRisk = "medium"
	} else {
		result.Summary.ScalingRisk = "low"
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d total objects, top: %s (%d), risk: %s", total, topType, topCount, result.Summary.ScalingRisk))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
