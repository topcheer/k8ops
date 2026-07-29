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
// v20.32 — Operations Dimension (Round 25)
// 1. Pod Restart Budget — restart rate vs pod age ratio
// 2. Volume IOPS Estimate — PVC I/O pressure estimation
// 3. Node Allocatable Budget — node allocatable vs capacity ratio
// ============================================================

// ---------------------------------------------------------------
// 1. Pod Restart Budget
// ---------------------------------------------------------------

type RestartBudgetResult2032 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         RestartBudgetSummary2032 `json:"summary"`
	HighRestart     []RestartBudgetEntry2032 `json:"highRestart"`
	Recommendations []string                 `json:"recommendations"`
}

type RestartBudgetSummary2032 struct {
	TotalPods       int     `json:"totalPods"`
	WithRestarts    int     `json:"withRestarts"`
	HighRestartRate int     `json:"highRestartRate"`
	AvgRestartRate  float64 `json:"avgRestartRate"`
}

type RestartBudgetEntry2032 struct {
	Pod         string  `json:"pod"`
	Namespace   string  `json:"namespace"`
	Restarts    int32   `json:"restarts"`
	PodAgeHours float64 `json:"podAgeHours"`
	RestartRate float64 `json:"restartRatePerHour"`
}

func (s *Server) handleRestartBudget(w http.ResponseWriter, r *http.Request) {
	result := RestartBudgetResult2032{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	now := time.Now()
	var totalRate float64
	var rateCount int

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		totalRestarts := int32(0)
		for _, cs := range pod.Status.ContainerStatuses {
			totalRestarts += cs.RestartCount
		}

		if totalRestarts > 0 {
			result.Summary.WithRestarts++

			podAgeHours := now.Sub(pod.Status.StartTime.Time).Hours()
			if podAgeHours < 1 {
				podAgeHours = 1
			}
			restartRate := float64(totalRestarts) / podAgeHours
			totalRate += restartRate
			rateCount++

			if restartRate > 0.5 {
				result.Summary.HighRestartRate++
				result.HighRestart = append(result.HighRestart, RestartBudgetEntry2032{
					Pod: pod.Name, Namespace: pod.Namespace,
					Restarts: totalRestarts, PodAgeHours: podAgeHours,
					RestartRate: restartRate,
				})
				score -= 2
			}
		}
	}

	if rateCount > 0 {
		result.Summary.AvgRestartRate = totalRate / float64(rateCount)
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.HighRestart, func(i, j int) bool {
		return result.HighRestart[i].RestartRate > result.HighRestart[j].RestartRate
	})

	if result.Summary.HighRestartRate > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pods have high restart rate (>0.5/hour) — investigate app health", result.Summary.HighRestartRate))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Volume IOPS Estimate
// ---------------------------------------------------------------

type VolIOPSResult2032 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         VolIOPSSummary2032 `json:"summary"`
	HighIOPSVols    []VolIOPSEntry2032 `json:"highIOPSVolumes"`
	Recommendations []string           `json:"recommendations"`
}

type VolIOPSSummary2032 struct {
	TotalPVCs  int `json:"totalPVCs"`
	BoundPVCs  int `json:"boundPVCs"`
	HighIOPS   int `json:"highIOPS"`
	SharedPVCs int `json:"sharedPVCs"`
}

type VolIOPSEntry2032 struct {
	PVC        string `json:"pvc"`
	Namespace  string `json:"namespace"`
	Size       string `json:"size"`
	MountCount int    `json:"mountCount"`
}

func (s *Server) handleVolIOPSEstimate(w http.ResponseWriter, r *http.Request) {
	result := VolIOPSResult2032{ScannedAt: time.Now()}
	score := 100

	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	// Count how many pods mount each PVC
	pvcMountCount := make(map[string]int)
	for _, pod := range podList.Items {
		for _, vol := range pod.Spec.Volumes {
			if vol.PersistentVolumeClaim != nil {
				key := pod.Namespace + "/" + vol.PersistentVolumeClaim.ClaimName
				pvcMountCount[key]++
			}
		}
	}

	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		if pvc.Status.Phase == corev1.ClaimBound {
			result.Summary.BoundPVCs++
		}

		key := pvc.Namespace + "/" + pvc.Name
		mountCount := pvcMountCount[key]

		size := "unknown"
		if pvc.Spec.Resources.Requests.Storage() != nil {
			size = pvc.Spec.Resources.Requests.Storage().String()
		}

		if mountCount > 1 {
			result.Summary.SharedPVCs++
			result.Summary.HighIOPS++
			result.HighIOPSVols = append(result.HighIOPSVols, VolIOPSEntry2032{
				PVC: pvc.Name, Namespace: pvc.Namespace,
				Size: size, MountCount: mountCount,
			})
			score -= 1
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.HighIOPSVols, func(i, j int) bool {
		return result.HighIOPSVols[i].MountCount > result.HighIOPSVols[j].MountCount
	})

	if result.Summary.SharedPVCs > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d PVCs are shared across multiple pods — potential I/O contention", result.Summary.SharedPVCs))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Node Allocatable Budget
// ---------------------------------------------------------------

type NodeAllocResult2032 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         NodeAllocSummary2032 `json:"summary"`
	TightNodes      []NodeAllocEntry2032 `json:"tightNodes"`
	Recommendations []string             `json:"recommendations"`
}

type NodeAllocSummary2032 struct {
	TotalNodes    int `json:"totalNodes"`
	OverAllocated int `json:"overAllocated"`
	TightNodes    int `json:"tightNodes"`
}

type NodeAllocEntry2032 struct {
	Node        string `json:"node"`
	AllocPct    int    `json:"allocatablePercent"`
	ReservedPct int    `json:"reservedPercent"`
}

func (s *Server) handleNodeAllocBudget(w http.ResponseWriter, r *http.Request) {
	result := NodeAllocResult2032{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++

		capacity := node.Status.Capacity
		allocatable := node.Status.Allocatable

		capCPU := capacity.Cpu().AsApproximateFloat64()
		allocCPU := allocatable.Cpu().AsApproximateFloat64()

		capMem := capacity.Memory().AsApproximateFloat64()
		allocMem := allocatable.Memory().AsApproximateFloat64()

		var cpuPct, memPct int
		if capCPU > 0 {
			cpuPct = int(allocCPU / capCPU * 100)
		}
		if capMem > 0 {
			memPct = int(allocMem / capMem * 100)
		}

		allocPct := (cpuPct + memPct) / 2
		reservedPct := 100 - allocPct

		if reservedPct > 30 {
			result.Summary.OverAllocated++
			result.TightNodes = append(result.TightNodes, NodeAllocEntry2032{
				Node: node.Name, AllocPct: allocPct, ReservedPct: reservedPct,
			})
			score -= 5
		} else if reservedPct > 15 {
			result.Summary.TightNodes++
			result.TightNodes = append(result.TightNodes, NodeAllocEntry2032{
				Node: node.Name, AllocPct: allocPct, ReservedPct: reservedPct,
			})
			score -= 2
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.TightNodes, func(i, j int) bool {
		return result.TightNodes[i].ReservedPct > result.TightNodes[j].ReservedPct
	})

	if result.Summary.OverAllocated > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d nodes have >30%% reserved — system overhead too high", result.Summary.OverAllocated))
	}

	writeJSON(w, result)
}
