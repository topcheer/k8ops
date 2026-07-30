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
// v21.26 — Scalability & HA Dimension (Round 40) — MILESTONE: 1000+ endpoints
// 1. Node CPU Throttle Risk — high CPU limit pods
// 2. PVC Reclaim Waste — unbound PVC storage waste
// 3. Namespace Replica Quota Spread
// ============================================================

type CPUThrottleResult2126 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         CPUThrottleSummary2126 `json:"summary"`
	HighLimit       []CPUThrottleEntry2126 `json:"highLimitPods"`
	Recommendations []string               `json:"recommendations"`
}

type CPUThrottleSummary2126 struct {
	TotalContainers int `json:"totalContainers"`
	HighLimit       int `json:"highLimitContainers"`
}

type CPUThrottleEntry2126 struct {
	Pod       string  `json:"pod"`
	Namespace string  `json:"namespace"`
	CPULimit  float64 `json:"cpuLimitCores"`
}

func (s *Server) handleCPUThrottle2126(w http.ResponseWriter, r *http.Request) {
	result := CPUThrottleResult2126{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if !c.Resources.Limits.Cpu().IsZero() {
				lim := c.Resources.Limits.Cpu().AsApproximateFloat64()
				if lim > 2 {
					result.Summary.HighLimit++
					result.HighLimit = append(result.HighLimit, CPUThrottleEntry2126{Pod: pod.Name, Namespace: pod.Namespace, CPULimit: lim})
					score -= 1
				}
			}
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.HighLimit, func(i, j int) bool { return result.HighLimit[i].CPULimit > result.HighLimit[j].CPULimit })
	writeJSON(w, result)
}

// 2. PVC Reclaim Waste
type PVCWasteResult2126 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         PVCWasteSummary2126 `json:"summary"`
	Wasted          []PVCWasteEntry2126 `json:"wastedPVCs"`
	Recommendations []string            `json:"recommendations"`
}

type PVCWasteSummary2126 struct {
	TotalPVCs  int `json:"totalPVCs"`
	WastedPVCs int `json:"wastedPVCs"`
}

type PVCWasteEntry2126 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func (s *Server) handlePVCWaste2126(w http.ResponseWriter, r *http.Request) {
	result := PVCWasteResult2126{ScannedAt: time.Now()}
	score := 100
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	usedPVC := make(map[string]bool)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, vol := range pod.Spec.Volumes {
			if vol.PersistentVolumeClaim != nil {
				usedPVC[pod.Namespace+"/"+vol.PersistentVolumeClaim.ClaimName] = true
			}
		}
	}

	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		key := pvc.Namespace + "/" + pvc.Name
		if !usedPVC[key] {
			result.Summary.WastedPVCs++
			result.Wasted = append(result.Wasted, PVCWasteEntry2126{Name: pvc.Name, Namespace: pvc.Namespace})
			score -= 1
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.Wasted, func(i, j int) bool { return result.Wasted[i].Namespace < result.Wasted[j].Namespace })

	if result.Summary.WastedPVCs > 5 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d unused PVCs wasting storage", result.Summary.WastedPVCs))
	}
	writeJSON(w, result)
}

// 3. NS Replica Spread
type NSReplicaResult2126 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         NSReplicaSummary2126 `json:"summary"`
	TopNS           []NSReplicaEntry2126 `json:"topNamespaces"`
	Recommendations []string             `json:"recommendations"`
}

type NSReplicaSummary2126 struct {
	TotalNS       int   `json:"totalNamespaces"`
	TotalReplicas int32 `json:"totalReplicas"`
}

type NSReplicaEntry2126 struct {
	Namespace string `json:"namespace"`
	Replicas  int32  `json:"replicas"`
}

func (s *Server) handleNSReplica2126(w http.ResponseWriter, r *http.Request) {
	result := NSReplicaResult2126{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	nsRep := make(map[string]int32)
	for _, dep := range deployList.Items {
		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		nsRep[dep.Namespace] += replicas
		result.Summary.TotalReplicas += replicas
	}
	result.Summary.TotalNS = len(nsRep)
	for ns, r := range nsRep {
		result.TopNS = append(result.TopNS, NSReplicaEntry2126{Namespace: ns, Replicas: r})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].Replicas > result.TopNS[j].Replicas })
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
