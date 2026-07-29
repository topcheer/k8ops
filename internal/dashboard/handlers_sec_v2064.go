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
// v20.64 — Security Dimension (Round 30)
// 1. Capability Drop Audit — dropped Linux capabilities
// 2. Seccomp Profile Coverage — seccomp profile compliance
// 3. Namespace Isolation Score — NS isolation via RBAC + NetworkPolicy
// ============================================================

type CapDropResult2064 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         CapDropSummary2064 `json:"summary"`
	NoDropPods      []CapDropEntry2064 `json:"noDropPods"`
	Recommendations []string           `json:"recommendations"`
}

type CapDropSummary2064 struct {
	TotalContainers int `json:"totalContainers"`
	WithCapDrop     int `json:"withCapDrop"`
	NoCapDrop       int `json:"noCapDrop"`
}

type CapDropEntry2064 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleCapDropAudit2064(w http.ResponseWriter, r *http.Request) {
	result := CapDropResult2064{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			hasDrop := false
			if c.SecurityContext != nil && c.SecurityContext.Capabilities != nil {
				if len(c.SecurityContext.Capabilities.Drop) > 0 {
					hasDrop = true
					result.Summary.WithCapDrop++
				}
			}
			if !hasDrop {
				result.Summary.NoCapDrop++
				result.NoDropPods = append(result.NoDropPods, CapDropEntry2064{
					Pod: pod.Name, Namespace: pod.Namespace,
				})
				score -= 1
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.NoDropPods, func(i, j int) bool { return result.NoDropPods[i].Namespace < result.NoDropPods[j].Namespace })

	if result.Summary.NoCapDrop > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d containers without capability drop — add ALL capabilities drop", result.Summary.NoCapDrop))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Seccomp Profile Coverage
// ---------------------------------------------------------------

type SeccompResult2064 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         SeccompSummary2064 `json:"summary"`
	NoSeccomp       []SeccompEntry2064 `json:"noSeccompPods"`
	Recommendations []string           `json:"recommendations"`
}

type SeccompSummary2064 struct {
	TotalPods   int `json:"totalPods"`
	WithSeccomp int `json:"withSeccomp"`
	NoSeccomp   int `json:"noSeccomp"`
}

type SeccompEntry2064 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleSeccompCoverage2064(w http.ResponseWriter, r *http.Request) {
	result := SeccompResult2064{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		hasSeccomp := false
		if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.SeccompProfile != nil {
			hasSeccomp = true
		}
		for _, c := range pod.Spec.Containers {
			if c.SecurityContext != nil && c.SecurityContext.SeccompProfile != nil {
				hasSeccomp = true
			}
		}

		if hasSeccomp {
			result.Summary.WithSeccomp++
		} else {
			result.Summary.NoSeccomp++
			result.NoSeccomp = append(result.NoSeccomp, SeccompEntry2064{
				Pod: pod.Name, Namespace: pod.Namespace,
			})
			score -= 1
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.NoSeccomp, func(i, j int) bool { return result.NoSeccomp[i].Namespace < result.NoSeccomp[j].Namespace })

	if result.Summary.NoSeccomp > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pods without seccomp profile — set RuntimeDefault", result.Summary.NoSeccomp))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Namespace Isolation Score
// ---------------------------------------------------------------

type NSIsoResult2064 struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Summary         NSIsoSummary2064 `json:"summary"`
	IsolatedNS      []NSIsoEntry2064 `json:"isolatedNamespaces"`
	Recommendations []string         `json:"recommendations"`
}

type NSIsoSummary2064 struct {
	TotalNS    int `json:"totalNamespaces"`
	IsolatedNS int `json:"isolatedNamespaces"`
	OpenNS     int `json:"openNamespaces"`
}

type NSIsoEntry2064 struct {
	Namespace string `json:"namespace"`
	HasNetPol bool   `json:"hasNetworkPolicy"`
	HasQuota  bool   `json:"hasResourceQuota"`
}

func (s *Server) handleNSIsoScore(w http.ResponseWriter, r *http.Request) {
	result := NSIsoResult2064{ScannedAt: time.Now()}
	score := 100

	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	npList, _ := s.clientset.NetworkingV1().NetworkPolicies("").List(r.Context(), metav1.ListOptions{})
	rqList, _ := s.clientset.CoreV1().ResourceQuotas("").List(r.Context(), metav1.ListOptions{})

	nsNetPol := make(map[string]bool)
	for _, np := range npList.Items {
		nsNetPol[np.Namespace] = true
	}
	nsQuota := make(map[string]bool)
	for _, rq := range rqList.Items {
		nsQuota[rq.Namespace] = true
	}

	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true, "k8ops-system": true}
	for _, ns := range nsList.Items {
		if systemNS[ns.Name] {
			continue
		}
		result.Summary.TotalNS++

		hasNP := nsNetPol[ns.Name]
		hasQ := nsQuota[ns.Name]
		isolated := hasNP && hasQ

		if isolated {
			result.Summary.IsolatedNS++
		} else {
			result.Summary.OpenNS++
			score -= 3
		}
		result.IsolatedNS = append(result.IsolatedNS, NSIsoEntry2064{
			Namespace: ns.Name, HasNetPol: hasNP, HasQuota: hasQ,
		})
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.OpenNS > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d namespaces lack isolation (NetworkPolicy + ResourceQuota)", result.Summary.OpenNS))
	}
	writeJSON(w, result)
}
