package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v19.98 — Documentation Dimension (Round 19)
// 1. Topology Spread Catalog — topology spread constraints inventory
// 2. LimitRange Catalog — LimitRange resource inventory
// 3. Lease Holder Catalog — leader election lease inventory
// ============================================================

// ---------------------------------------------------------------
// 1. Topology Spread Catalog
// ---------------------------------------------------------------

type TopoSpreadResult1998 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         TopoSpreadSummary1998 `json:"summary"`
	Constraints     []TopoSpreadEntry1998 `json:"constraints"`
	Recommendations []string              `json:"recommendations"`
}

type TopoSpreadSummary1998 struct {
	TotalPods  int `json:"totalPods"`
	WithSpread int `json:"withTopologySpread"`
	ZoneSpread int `json:"withZoneSpread"`
	HostSpread int `json:"withHostSpread"`
}

type TopoSpreadEntry1998 struct {
	Pod               string `json:"pod"`
	Namespace         string `json:"namespace"`
	TopologyKey       string `json:"topologyKey"`
	MaxSkew           int32  `json:"maxSkew"`
	WhenUnsatisfiable string `json:"whenUnsatisfiable"`
}

func (s *Server) handleTopologySpreadCatalog(w http.ResponseWriter, r *http.Request) {
	result := TopoSpreadResult1998{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		for _, tsc := range pod.Spec.TopologySpreadConstraints {
			result.Summary.WithSpread++

			entry := TopoSpreadEntry1998{
				Pod: pod.Name, Namespace: pod.Namespace,
				TopologyKey:       tsc.TopologyKey,
				MaxSkew:           tsc.MaxSkew,
				WhenUnsatisfiable: string(tsc.WhenUnsatisfiable),
			}

			if tsc.TopologyKey == "topology.kubernetes.io/zone" || tsc.TopologyKey == "zone" {
				result.Summary.ZoneSpread++
			} else if tsc.TopologyKey == "kubernetes.io/hostname" {
				result.Summary.HostSpread++
			}

			result.Constraints = append(result.Constraints, entry)
		}
	}

	if result.Summary.WithSpread == 0 {
		result.Recommendations = append(result.Recommendations, "No topology spread constraints — add for HA distribution")
	}

	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods: %d with topology spread (%d zone, %d host)", result.Summary.TotalPods, result.Summary.WithSpread, result.Summary.ZoneSpread, result.Summary.HostSpread))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. LimitRange Catalog
// ---------------------------------------------------------------

type LimitRangeCatResult1998 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         LimitRangeCatSummary1998 `json:"summary"`
	LimitRanges     []LimitRangeCatEntry1998 `json:"limitRanges"`
	Recommendations []string                 `json:"recommendations"`
}

type LimitRangeCatSummary1998 struct {
	TotalLimitRanges int `json:"totalLimitRanges"`
	NamespacesWith   int `json:"namespacesWithLimitRange"`
	WithCPULimit     int `json:"withCPULimit"`
	WithMemLimit     int `json:"withMemLimit"`
}

type LimitRangeCatEntry1998 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	RuleCount int    `json:"ruleCount"`
}

func (s *Server) handleLimitRangeCatalog(w http.ResponseWriter, r *http.Request) {
	result := LimitRangeCatResult1998{ScannedAt: time.Now()}
	score := 100

	lrList, _ := s.clientset.CoreV1().LimitRanges("").List(r.Context(), metav1.ListOptions{})
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})

	nsWithLR := make(map[string]bool)

	for _, lr := range lrList.Items {
		result.Summary.TotalLimitRanges++

		entry := LimitRangeCatEntry1998{
			Name: lr.Name, Namespace: lr.Namespace,
			RuleCount: len(lr.Spec.Limits),
		}

		nsWithLR[lr.Namespace] = true

		for _, limit := range lr.Spec.Limits {
			if limit.Type == corev1.LimitTypeContainer {
				if limit.Max.Cpu() != nil || limit.Default.Cpu() != nil {
					result.Summary.WithCPULimit++
				}
				if limit.Max.Memory() != nil || limit.Default.Memory() != nil {
					result.Summary.WithMemLimit++
				}
			}
		}

		result.LimitRanges = append(result.LimitRanges, entry)
	}

	result.Summary.NamespacesWith = len(nsWithLR)

	totalNS := len(nsList.Items)
	unprotected := totalNS - result.Summary.NamespacesWith
	if unprotected > totalNS/2 {
		score -= 5
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d LimitRanges across %d/%d namespaces (%d CPU, %d Mem)", result.Summary.TotalLimitRanges, result.Summary.NamespacesWith, totalNS, result.Summary.WithCPULimit, result.Summary.WithMemLimit))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Lease Holder Catalog
// ---------------------------------------------------------------

type LeaseResult1998 struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Summary         LeaseSummary1998 `json:"summary"`
	Leases          []LeaseEntry1998 `json:"leases"`
	Recommendations []string         `json:"recommendations"`
}

type LeaseSummary1998 struct {
	TotalLeases   int     `json:"totalLeases"`
	ActiveLeases  int     `json:"activeLeases"`
	ExpiredLeases int     `json:"expiredLeases"`
	AvgRenewSec   float64 `json:"avgRenewSec"`
}

type LeaseEntry1998 struct {
	Name       string  `json:"name"`
	Namespace  string  `json:"namespace"`
	Holder     string  `json:"holderIdentity"`
	AgeSeconds float64 `json:"ageSeconds"`
}

func (s *Server) handleLeaseHolderCatalog(w http.ResponseWriter, r *http.Request) {
	result := LeaseResult1998{ScannedAt: time.Now()}
	score := 100

	leaseList, _ := s.clientset.CoordinationV1().Leases("").List(r.Context(), metav1.ListOptions{})

	var totalAge float64
	var count int
	now := time.Now()

	for _, lease := range leaseList.Items {
		result.Summary.TotalLeases++

		entry := LeaseEntry1998{
			Name: lease.Name, Namespace: lease.Namespace,
		}

		if lease.Spec.HolderIdentity != nil {
			entry.Holder = *lease.Spec.HolderIdentity
		}

		// Check lease age from renew time
		if lease.Spec.RenewTime != nil {
			age := now.Sub(lease.Spec.RenewTime.Time).Seconds()
			entry.AgeSeconds = age
			totalAge += age
			count++

			if lease.Spec.LeaseDurationSeconds != nil {
				if age > float64(*lease.Spec.LeaseDurationSeconds) {
					result.Summary.ExpiredLeases++
				} else {
					result.Summary.ActiveLeases++
				}
			} else {
				result.Summary.ActiveLeases++
			}
		} else {
			result.Summary.ActiveLeases++
		}

		result.Leases = append(result.Leases, entry)
	}

	if count > 0 {
		result.Summary.AvgRenewSec = totalAge / float64(count)
	}

	_ = coordinationv1.Lease{} // suppress unused import

	if result.Summary.ExpiredLeases > 5 {
		score -= 3
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d leases: %d active, %d expired, avg renew %.0fs ago", result.Summary.TotalLeases, result.Summary.ActiveLeases, result.Summary.ExpiredLeases, result.Summary.AvgRenewSec))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
