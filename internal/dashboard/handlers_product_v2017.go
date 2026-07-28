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
// v20.17 — Product Dimension (Round 22)
// 1. Pod Network Policy Match — pod-netpol enforcement coverage
// 2. Deployment MaxUnavailable Config — rollout surge gap analysis
// 3. Container Image Pull Policy — pull policy compliance
// ============================================================

// ---------------------------------------------------------------
// 1. Pod Network Policy Match
// ---------------------------------------------------------------

type PodNetPolResult2017 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         PodNetPolSummary2017 `json:"summary"`
	PerNS           []PodNetPolEntry2017 `json:"perNamespace"`
	Recommendations []string             `json:"recommendations"`
}

type PodNetPolSummary2017 struct {
	TotalPods   int `json:"totalPods"`
	CoveredPods int `json:"coveredByNetPol"`
	UncoveredNS int `json:"namespacesWithoutNetPol"`
}

type PodNetPolEntry2017 struct {
	Namespace string `json:"namespace"`
	PodCount  int    `json:"podCount"`
	HasNetPol bool   `json:"hasNetworkPolicy"`
}

func (s *Server) handlePodNetPolMatch(w http.ResponseWriter, r *http.Request) {
	result := PodNetPolResult2017{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	npList, _ := s.clientset.NetworkingV1().NetworkPolicies("").List(r.Context(), metav1.ListOptions{})

	nsWithNetPol := make(map[string]bool)
	for _, np := range npList.Items {
		nsWithNetPol[np.Namespace] = true
	}

	podsPerNS := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		podsPerNS[pod.Namespace]++
	}

	for ns, count := range podsPerNS {
		hasNP := nsWithNetPol[ns]
		entry := PodNetPolEntry2017{Namespace: ns, PodCount: count, HasNetPol: hasNP}
		if hasNP {
			result.Summary.CoveredPods += count
		} else {
			result.Summary.UncoveredNS++
			score -= 2
		}
		result.PerNS = append(result.PerNS, entry)
	}

	sort.Slice(result.PerNS, func(i, j int) bool {
		return result.PerNS[i].PodCount > result.PerNS[j].PodCount
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods: %d covered, %d NS without netpol", result.Summary.TotalPods, result.Summary.CoveredPods, result.Summary.UncoveredNS))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Deployment MaxUnavailable Config
// ---------------------------------------------------------------

type MaxUnavailResult2017 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         MaxUnavailSummary2017 `json:"summary"`
	Deployments     []MaxUnavailEntry2017 `json:"deployments"`
	Recommendations []string              `json:"recommendations"`
}

type MaxUnavailSummary2017 struct {
	TotalDeployments int `json:"totalDeployments"`
	WithCustom       int `json:"withCustomStrategy"`
	UsingDefault     int `json:"usingDefaultStrategy"`
	HighSurge        int `json:"withHighSurge"`
}

type MaxUnavailEntry2017 struct {
	Name           string `json:"name"`
	Namespace      string `json:"namespace"`
	MaxUnavailable string `json:"maxUnavailable"`
	MaxSurge       string `json:"maxSurge"`
}

func (s *Server) handleMaxUnavail(w http.ResponseWriter, r *http.Request) {
	result := MaxUnavailResult2017{ScannedAt: time.Now()}
	score := 100

	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range depList.Items {
		result.Summary.TotalDeployments++

		entry := MaxUnavailEntry2017{
			Name: dep.Name, Namespace: dep.Namespace,
		}

		strategy := dep.Spec.Strategy
		if strategy.RollingUpdate != nil {
			if strategy.RollingUpdate.MaxUnavailable != nil {
				entry.MaxUnavailable = strategy.RollingUpdate.MaxUnavailable.String()
			}
			if strategy.RollingUpdate.MaxSurge != nil {
				entry.MaxSurge = strategy.RollingUpdate.MaxSurge.String()
			}
			result.Summary.WithCustom++
		} else {
			result.Summary.UsingDefault++
		}

		result.Deployments = append(result.Deployments, entry)
	}

	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d deployments: %d custom strategy, %d default", result.Summary.TotalDeployments, result.Summary.WithCustom, result.Summary.UsingDefault))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Container Image Pull Policy
// ---------------------------------------------------------------

type PullPolResult2017 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         PullPolSummary2017 `json:"summary"`
	Issues          []PullPolEntry2017 `json:"issues"`
	Recommendations []string           `json:"recommendations"`
}

type PullPolSummary2017 struct {
	TotalContainers int `json:"totalContainers"`
	Always          int `json:"alwaysPolicy"`
	IfNotPresent    int `json:"ifNotPresentPolicy"`
	Never           int `json:"neverPolicy"`
	NotSet          int `json:"notSet"`
}

type PullPolEntry2017 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
	Policy    string `json:"pullPolicy"`
}

func (s *Server) handlePullPol(w http.ResponseWriter, r *http.Request) {
	result := PullPolResult2017{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			policy := string(c.ImagePullPolicy)
			switch policy {
			case "Always":
				result.Summary.Always++
			case "IfNotPresent":
				result.Summary.IfNotPresent++
			case "Never":
				result.Summary.Never++
				result.Issues = append(result.Issues, PullPolEntry2017{
					Pod: pod.Name, Namespace: pod.Namespace,
					Container: c.Name, Policy: policy,
				})
				score -= 1
			default:
				result.Summary.NotSet++
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers: %d Always, %d IfNotPresent, %d Never, %d not set", result.Summary.TotalContainers, result.Summary.Always, result.Summary.IfNotPresent, result.Summary.Never, result.Summary.NotSet))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
