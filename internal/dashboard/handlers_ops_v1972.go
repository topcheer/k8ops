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
// v19.72 — Operations Dimension (Round 15)
// 1. Pod Age Distribution — lifecycle stage classification
// 2. Node Condition Flap — condition oscillation detection
// 3. CSI Attach Latency — volume attach/detach performance estimator
// ============================================================

// ---------------------------------------------------------------
// 1. Pod Age Distribution
// ---------------------------------------------------------------

type PodAgeDistResult1972 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         PodAgeDistSummary1972  `json:"summary"`
	Buckets         []PodAgeBucket1972     `json:"buckets"`
	StalePods       []PodAgeStaleEntry1972 `json:"stalePods"`
	Recommendations []string               `json:"recommendations"`
}

type PodAgeDistSummary1972 struct {
	TotalPods   int     `json:"totalPods"`
	AvgAgeHours float64 `json:"avgAgeHours"`
	MaxAgeDays  float64 `json:"maxAgeDays"`
	NewPods     int     `json:"newPods1h"`
	OldPods     int     `json:"oldPods7d"`
	StalePods   int     `json:"stalePods30d"`
}

type PodAgeBucket1972 struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

type PodAgeStaleEntry1972 struct {
	Name      string  `json:"name"`
	Namespace string  `json:"namespace"`
	AgeDays   float64 `json:"ageDays"`
}

func (s *Server) handlePodAgeDistribution(w http.ResponseWriter, r *http.Request) {
	result := PodAgeDistResult1972{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	buckets := map[string]int{
		"<1h": 0, "1-6h": 0, "6-24h": 0, "1-7d": 0, "7-30d": 0, ">30d": 0,
	}
	var totalAge float64
	var maxAge time.Duration

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		if pod.CreationTimestamp.IsZero() {
			continue
		}
		age := time.Since(pod.CreationTimestamp.Time)
		ageH := age.Hours()
		totalAge += ageH
		if age > maxAge {
			maxAge = age
		}

		switch {
		case ageH < 1:
			buckets["<1h"]++
			result.Summary.NewPods++
		case ageH < 6:
			buckets["1-6h"]++
		case ageH < 24:
			buckets["6-24h"]++
		case ageH < 168:
			buckets["1-7d"]++
		case ageH < 720:
			buckets["7-30d"]++
			result.Summary.OldPods++
		default:
			buckets[">30d"]++
			result.Summary.StalePods++
			result.StalePods = append(result.StalePods, PodAgeStaleEntry1972{
				Name: pod.Name, Namespace: pod.Namespace,
				AgeDays: ageH / 24,
			})
		}
	}

	if result.Summary.TotalPods > 0 {
		result.Summary.AvgAgeHours = totalAge / float64(result.Summary.TotalPods)
	}
	result.Summary.MaxAgeDays = maxAge.Hours() / 24

	for _, label := range []string{"<1h", "1-6h", "6-24h", "1-7d", "7-30d", ">30d"} {
		result.Buckets = append(result.Buckets, PodAgeBucket1972{Label: label, Count: buckets[label]})
	}

	if result.Summary.StalePods > 10 {
		score -= 5
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods, avg age %.0fh, max %.0fd, %d stale (>30d)", result.Summary.TotalPods, result.Summary.AvgAgeHours, result.Summary.MaxAgeDays, result.Summary.StalePods))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Node Condition Flap
// ---------------------------------------------------------------

type NodeFlapResult1972 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         NodeFlapSummary1972 `json:"summary"`
	FlappingNodes   []NodeFlapEntry1972 `json:"flappingNodes"`
	Recommendations []string            `json:"recommendations"`
}

type NodeFlapSummary1972 struct {
	TotalNodes      int `json:"totalNodes"`
	StableNodes     int `json:"stableNodes"`
	FlappingNodes   int `json:"flappingNodes"`
	TotalConditions int `json:"totalConditions"`
}

type NodeFlapEntry1972 struct {
	Name              string   `json:"name"`
	Conditions        []string `json:"conditions"`
	RecentTransitions int      `json:"recentTransitions"`
}

func (s *Server) handleNodeConditionFlap(w http.ResponseWriter, r *http.Request) {
	result := NodeFlapResult1972{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		entry := NodeFlapEntry1972{Name: node.Name}
		flapCount := 0

		for _, cond := range node.Status.Conditions {
			result.Summary.TotalConditions++

			// Check if condition transitioned recently (last hour)
			if !cond.LastTransitionTime.IsZero() {
				since := time.Since(cond.LastTransitionTime.Time)
				if since < time.Hour && cond.Type != corev1.NodeReady {
					entry.Conditions = append(entry.Conditions, string(cond.Type))
					flapCount++
				}
				// Ready condition flipping is most concerning
				if since < time.Hour && cond.Type == corev1.NodeReady {
					entry.Conditions = append(entry.Conditions, "Ready (recently transitioned)")
					flapCount++
				}
			}
		}

		entry.RecentTransitions = flapCount
		if flapCount > 0 {
			result.Summary.FlappingNodes++
			result.FlappingNodes = append(result.FlappingNodes, entry)
			score -= 5
		} else {
			result.Summary.StableNodes++
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d nodes: %d stable, %d flapping", result.Summary.TotalNodes, result.Summary.StableNodes, result.Summary.FlappingNodes))
	if result.Summary.FlappingNodes > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d nodes with recent condition transitions — investigate hardware/resource issues", result.Summary.FlappingNodes))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. CSI Attach Latency Estimator
// ---------------------------------------------------------------

type CSIAttachResult1972 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         CSIAttachSummary1972 `json:"summary"`
	VolumeStats     []CSIAttachEntry1972 `json:"volumeStats"`
	Recommendations []string             `json:"recommendations"`
}

type CSIAttachSummary1972 struct {
	TotalPVCs        int     `json:"totalPVCs"`
	BoundPVCs        int     `json:"boundPVCs"`
	PendingPVCs      int     `json:"pendingPVCs"`
	AvgAttachTimeMin float64 `json:"estAvgAttachTimeMin"`
	TotalAttachOps   int     `json:"estimatedAttachOps"`
}

type CSIAttachEntry1972 struct {
	Namespace    string  `json:"namespace"`
	PVCName      string  `json:"pvcName"`
	Status       string  `json:"status"`
	StorageClass string  `json:"storageClass"`
	SizeGB       float64 `json:"sizeGB"`
}

func (s *Server) handleCSIAttachLatency(w http.ResponseWriter, r *http.Request) {
	result := CSIAttachResult1972{ScannedAt: time.Now()}
	score := 100

	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})

	// Estimate attach time: ~5s per GB for network storage, ~0.5s for local
	const netAttachPerGB = 0.083 // minutes (~5s per GB)
	var totalAttachTime float64

	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++

		sizeGB := 0.0
		if sz := pvc.Spec.Resources.Requests.Storage(); sz != nil {
			sizeGB = float64(sz.Value()) / (1024 * 1024 * 1024)
		}

		scName := ""
		if pvc.Spec.StorageClassName != nil {
			scName = *pvc.Spec.StorageClassName
		}

		entry := CSIAttachEntry1972{
			Namespace: pvc.Namespace, PVCName: pvc.Name,
			StorageClass: scName, SizeGB: sizeGB,
		}

		if pvc.Status.Phase == corev1.ClaimBound {
			result.Summary.BoundPVCs++
			entry.Status = "Bound"
			totalAttachTime += sizeGB * netAttachPerGB
			result.Summary.TotalAttachOps++
		} else {
			result.Summary.PendingPVCs++
			entry.Status = string(pvc.Status.Phase)
			score -= 3
		}

		result.VolumeStats = append(result.VolumeStats, entry)
	}

	if result.Summary.TotalAttachOps > 0 {
		result.Summary.AvgAttachTimeMin = totalAttachTime / float64(result.Summary.TotalAttachOps)
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d PVCs (%d bound, %d pending), est avg attach: %.1f min", result.Summary.TotalPVCs, result.Summary.BoundPVCs, result.Summary.PendingPVCs, result.Summary.AvgAttachTimeMin))
	if result.Summary.PendingPVCs > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pending PVCs — check CSI driver and storage provisioner", result.Summary.PendingPVCs))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
