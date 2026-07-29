package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v20.44 — Deployment Dimension (Round 27)
// 1. Replica Set Staleness — orphaned old replica sets
// 2. Image Pull Policy Audit — Always vs IfNotPresent compliance
// 3. Max Surge Analyzer — rollout surge configuration analysis
// ============================================================

// ---------------------------------------------------------------
// 1. Replica Set Staleness
// ---------------------------------------------------------------

type RSStaleResult2044 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         RSStaleSummary2044 `json:"summary"`
	StaleRS         []RSStaleEntry2044 `json:"staleReplicaSets"`
	Recommendations []string           `json:"recommendations"`
}

type RSStaleSummary2044 struct {
	TotalRS  int `json:"totalReplicaSets"`
	ActiveRS int `json:"activeReplicaSets"`
	StaleRS  int `json:"staleReplicaSets"`
}

type RSStaleEntry2044 struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	OwnerDeploy string `json:"ownerDeployment"`
}

func (s *Server) handleRSStaleness2044(w http.ResponseWriter, r *http.Request) {
	result := RSStaleResult2044{ScannedAt: time.Now()}
	score := 100

	rsList, _ := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})

	for _, rs := range rsList.Items {
		result.Summary.TotalRS++

		// Active RS has replicas > 0
		if rs.Status.Replicas > 0 {
			result.Summary.ActiveRS++
			continue
		}

		// Stale RS: no replicas but still exists
		owner := ""
		for _, ref := range rs.OwnerReferences {
			if ref.Kind == "Deployment" {
				owner = ref.Name
			}
		}

		result.Summary.StaleRS++
		result.StaleRS = append(result.StaleRS, RSStaleEntry2044{
			Name: rs.Name, Namespace: rs.Namespace, OwnerDeploy: owner,
		})
		score -= 1
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.StaleRS, func(i, j int) bool {
		return result.StaleRS[i].Namespace < result.StaleRS[j].Namespace
	})

	if result.Summary.StaleRS > 10 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d stale ReplicaSets — reduce revisionHistoryLimit to clean up", result.Summary.StaleRS))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Image Pull Policy Audit
// ---------------------------------------------------------------

type PullPolicyResult2044 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         PullPolicySummary2044 `json:"summary"`
	RiskyPolicies   []PullPolicyEntry2044 `json:"riskyPolicies"`
	Recommendations []string              `json:"recommendations"`
}

type PullPolicySummary2044 struct {
	TotalContainers int `json:"totalContainers"`
	AlwaysPolicy    int `json:"alwaysPolicy"`
	IfNotPresent    int `json:"ifNotPresent"`
	NeverPolicy     int `json:"neverPolicy"`
}

type PullPolicyEntry2044 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
	Image     string `json:"image"`
	Policy    string `json:"policy"`
}

func (s *Server) handlePullPolicyAudit2044(w http.ResponseWriter, r *http.Request) {
	result := PullPolicyResult2044{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			policy := string(c.ImagePullPolicy)
			if policy == "" {
				policy = "IfNotPresent"
			}

			switch policy {
			case "Always":
				result.Summary.AlwaysPolicy++
			case "IfNotPresent":
				result.Summary.IfNotPresent++
			case "Never":
				result.Summary.NeverPolicy++
				result.RiskyPolicies = append(result.RiskyPolicies, PullPolicyEntry2044{
					Pod: pod.Name, Namespace: pod.Namespace,
					Container: c.Name, Image: c.Image, Policy: policy,
				})
				score -= 2
			}

			// Always policy on non-latest tags is wasteful
			if policy == "Always" && !hasLatestTag2044(c.Image) {
				result.RiskyPolicies = append(result.RiskyPolicies, PullPolicyEntry2044{
					Pod: pod.Name, Namespace: pod.Namespace,
					Container: c.Name, Image: c.Image, Policy: policy,
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

	if len(result.RiskyPolicies) > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d containers have suboptimal pull policy — use IfNotPresent for pinned tags", len(result.RiskyPolicies)))
	}

	writeJSON(w, result)
}

func hasLatestTag2044(image string) bool {
	for i := len(image) - 1; i >= 0; i-- {
		if image[i] == '/' {
			break
		}
		if image[i] == ':' {
			tag := image[i+1:]
			return tag == "latest" || tag == "main" || tag == "master"
		}
	}
	return true // no tag = latest
}

// ---------------------------------------------------------------
// 3. Max Surge Analyzer
// ---------------------------------------------------------------

type MaxSurgeResult2044 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         MaxSurgeSummary2044 `json:"summary"`
	HighSurge       []MaxSurgeEntry2044 `json:"highSurgeDeployments"`
	Recommendations []string            `json:"recommendations"`
}

type MaxSurgeSummary2044 struct {
	TotalDeploys int `json:"totalDeployments"`
	WithSurge    int `json:"withSurgeConfig"`
	HighSurge    int `json:"highSurge"`
	DefaultSurge int `json:"defaultSurge"`
}

type MaxSurgeEntry2044 struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	MaxSurge   string `json:"maxSurge"`
	MaxUnavail string `json:"maxUnavailable"`
}

func (s *Server) handleMaxSurgeAnalyzer(w http.ResponseWriter, r *http.Request) {
	result := MaxSurgeResult2044{ScannedAt: time.Now()}
	score := 100

	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++

		strategy := dep.Spec.Strategy
		if strategy.Type == appsv1.RecreateDeploymentStrategyType || strategy.Type == "" {
			continue
		}

		rolling := strategy.RollingUpdate
		if rolling == nil {
			result.Summary.DefaultSurge++
			continue
		}

		result.Summary.WithSurge++

		maxSurge := "25%"
		if rolling.MaxSurge != nil {
			maxSurge = rolling.MaxSurge.String()
		}
		maxUnavail := "25%"
		if rolling.MaxUnavailable != nil {
			maxUnavail = rolling.MaxUnavailable.String()
		}

		// High surge is risky for resource-constrained clusters
		if rolling.MaxSurge != nil && rolling.MaxSurge.IntVal > 3 {
			result.Summary.HighSurge++
			result.HighSurge = append(result.HighSurge, MaxSurgeEntry2044{
				Name: dep.Name, Namespace: dep.Namespace,
				MaxSurge: maxSurge, MaxUnavail: maxUnavail,
			})
			score -= 1
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.HighSurge, func(i, j int) bool {
		return result.HighSurge[i].Namespace < result.HighSurge[j].Namespace
	})

	if result.Summary.HighSurge > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d deployments have high maxSurge — may overwhelm nodes during rollout", result.Summary.HighSurge))
	}

	writeJSON(w, result)
}
