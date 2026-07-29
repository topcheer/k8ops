package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v20.30 — Scalability & HA Dimension (Round 24)
// 1. HPA Headroom Analyzer — HPA scaling headroom & ceiling analysis
// 2. Multi-AZ Spread Validator — pod anti-affinity & topology spread
// 3. Leader Election Audit — controller HA lease holder inventory
// ============================================================

// ---------------------------------------------------------------
// 1. HPA Headroom Analyzer
// ---------------------------------------------------------------

type HPAHeadroomResult2030 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         HPAHeadroomSummary2030 `json:"summary"`
	NearCeiling     []HPAHeadroomEntry2030 `json:"nearCeiling"`
	Recommendations []string               `json:"recommendations"`
}

type HPAHeadroomSummary2030 struct {
	TotalHPAs       int `json:"totalHPAs"`
	NearMaxReplicas int `json:"nearMaxReplicas"`
	NoMaxSet        int `json:"noMaxSet"`
	AtMinReplicas   int `json:"atMinReplicas"`
}

type HPAHeadroomEntry2030 struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	CurrentReps int32  `json:"currentReplicas"`
	MaxReps     int32  `json:"maxReplicas"`
	HeadroomPct int    `json:"headroomPercent"`
}

func (s *Server) handleHPAHeadroom(w http.ResponseWriter, r *http.Request) {
	result := HPAHeadroomResult2030{ScannedAt: time.Now()}
	score := 100

	hpaList, _ := s.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(r.Context(), metav1.ListOptions{})

	for _, hpa := range hpaList.Items {
		result.Summary.TotalHPAs++

		maxReplicas := hpa.Spec.MaxReplicas
		minReplicas := int32(1)
		if hpa.Spec.MinReplicas != nil {
			minReplicas = *hpa.Spec.MinReplicas
		}
		currentReps := hpa.Status.CurrentReplicas

		entry := HPAHeadroomEntry2030{
			Name: hpa.Name, Namespace: hpa.Namespace,
			CurrentReps: currentReps, MaxReps: maxReplicas,
		}

		if maxReplicas > 0 {
			headroom := int(float64(maxReplicas-currentReps) / float64(maxReplicas) * 100)
			entry.HeadroomPct = headroom

			if currentReps >= maxReplicas {
				result.Summary.NearMaxReplicas++
				score -= 5
				result.NearCeiling = append(result.NearCeiling, entry)
			} else if headroom < 20 {
				result.Summary.NearMaxReplicas++
				score -= 2
				result.NearCeiling = append(result.NearCeiling, entry)
			}
		} else {
			result.Summary.NoMaxSet++
		}

		if currentReps <= minReplicas {
			result.Summary.AtMinReplicas++
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.NearCeiling, func(i, j int) bool {
		return result.NearCeiling[i].HeadroomPct < result.NearCeiling[j].HeadroomPct
	})

	if result.Summary.NearMaxReplicas > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d HPAs are near max replicas — increase maxReplicas or optimize resource usage", result.Summary.NearMaxReplicas))
	}
	if result.Summary.NoMaxSet > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d HPAs have maxReplicas=0 — configure scaling ceiling", result.Summary.NoMaxSet))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Multi-AZ Spread Validator
// ---------------------------------------------------------------

type AZSpreadResult2030 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         AZSpreadSummary2030 `json:"summary"`
	PoorSpread      []AZSpreadEntry2030 `json:"poorSpread"`
	Recommendations []string            `json:"recommendations"`
}

type AZSpreadSummary2030 struct {
	TotalDeployments   int `json:"totalDeployments"`
	WithAntiAffinity   int `json:"withAntiAffinity"`
	WithTopologySpread int `json:"withTopologySpread"`
	PoorSpread         int `json:"poorSpread"`
}

type AZSpreadEntry2030 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Issue     string `json:"issue"`
}

func (s *Server) handleAZSpreadValidator(w http.ResponseWriter, r *http.Request) {
	result := AZSpreadResult2030{ScannedAt: time.Now()}
	score := 100

	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range deployList.Items {
		result.Summary.TotalDeployments++

		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}

		hasAntiAffinity := false
		hasTopologySpread := false

		if dep.Spec.Template.Spec.Affinity != nil &&
			dep.Spec.Template.Spec.Affinity.PodAntiAffinity != nil {
			hasAntiAffinity = true
			result.Summary.WithAntiAffinity++
		}

		if len(dep.Spec.Template.Spec.TopologySpreadConstraints) > 0 {
			hasTopologySpread = true
			result.Summary.WithTopologySpread++
		}

		// Multi-replica deployments without anti-affinity or topology spread
		if replicas > 1 && !hasAntiAffinity && !hasTopologySpread {
			result.Summary.PoorSpread++
			result.PoorSpread = append(result.PoorSpread, AZSpreadEntry2030{
				Name: dep.Name, Namespace: dep.Namespace,
				Issue: "multi-replica without anti-affinity or topology spread",
			})
			score -= 3
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.PoorSpread, func(i, j int) bool {
		return result.PoorSpread[i].Namespace < result.PoorSpread[j].Namespace
	})

	if result.Summary.PoorSpread > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d multi-replica deployments lack topology spread — add podAntiAffinity or topologySpreadConstraints", result.Summary.PoorSpread))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Leader Election Audit
// ---------------------------------------------------------------

type LeaderElectionResult2030 struct {
	ScannedAt       time.Time                 `json:"scannedAt"`
	HealthScore     int                       `json:"healthScore"`
	Grade           string                    `json:"grade"`
	Summary         LeaderElectionSummary2030 `json:"summary"`
	LeaseHolders    []LeaderElectionEntry2030 `json:"leaseHolders"`
	Recommendations []string                  `json:"recommendations"`
}

type LeaderElectionSummary2030 struct {
	TotalLeases int `json:"totalLeases"`
	WithHolder  int `json:"withHolder"`
	StaleLeases int `json:"staleLeases"`
}

type LeaderElectionEntry2030 struct {
	Name       string    `json:"name"`
	Namespace  string    `json:"namespace"`
	Holder     string    `json:"holderIdentity"`
	RenewTime  time.Time `json:"renewTime"`
	AgeSeconds int       `json:"ageSeconds"`
}

func (s *Server) handleLeaderElectionAudit(w http.ResponseWriter, r *http.Request) {
	result := LeaderElectionResult2030{ScannedAt: time.Now()}
	score := 100

	// Check kube-system for lease objects
	leaseList, _ := s.clientset.CoordinationV1().Leases("").List(r.Context(), metav1.ListOptions{})

	now := time.Now()
	for _, lease := range leaseList.Items {
		result.Summary.TotalLeases++

		entry := LeaderElectionEntry2030{
			Name:      lease.Name,
			Namespace: lease.Namespace,
		}

		if lease.Spec.HolderIdentity != nil {
			entry.Holder = *lease.Spec.HolderIdentity
			result.Summary.WithHolder++
		}

		var renewTime time.Time
		if lease.Spec.RenewTime != nil {
			renewTime = lease.Spec.RenewTime.Time
			entry.RenewTime = renewTime
			entry.AgeSeconds = int(now.Sub(renewTime).Seconds())
		}

		// Stale if not renewed in 10 minutes
		if !renewTime.IsZero() && now.Sub(renewTime) > 10*time.Minute {
			result.Summary.StaleLeases++
			score -= 2
		}

		result.LeaseHolders = append(result.LeaseHolders, entry)
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.LeaseHolders, func(i, j int) bool {
		return result.LeaseHolders[i].AgeSeconds > result.LeaseHolders[j].AgeSeconds
	})

	if result.Summary.StaleLeases > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d stale leader election leases (>10min) — check controller health", result.Summary.StaleLeases))
	}

	writeJSON(w, result)
}

// keep imports used
var _ = appsv1.Deployment{}
var _ = autscalingv2.HorizontalPodAutoscaler{}
var _ = corev1.Pod{}
