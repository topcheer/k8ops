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
// v20.56 — Deployment Dimension (Round 29)
// 1. Deployment Condition Drift — stale conditions analysis
// 2. Pod Security Standard Validator — PSS profile enforcement
// 3. Container Resource Equality — consistent resource requests
// ============================================================

// ---------------------------------------------------------------
// 1. Deployment Condition Drift
// ---------------------------------------------------------------

type CondDriftResult2056 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         CondDriftSummary2056 `json:"summary"`
	Drifted         []CondDriftEntry2056 `json:"driftedDeployments"`
	Recommendations []string             `json:"recommendations"`
}

type CondDriftSummary2056 struct {
	TotalDeploys int `json:"totalDeployments"`
	Healthy      int `json:"healthy"`
	Drifted      int `json:"drifted"`
}

type CondDriftEntry2056 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Issue     string `json:"issue"`
}

func (s *Server) handleCondDrift2056(w http.ResponseWriter, r *http.Request) {
	result := CondDriftResult2056{ScannedAt: time.Now()}
	score := 100

	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++

		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}

		ready := dep.Status.ReadyReplicas
		updated := dep.Status.UpdatedReplicas
		available := dep.Status.AvailableReplicas

		issue := ""
		if ready != replicas && replicas > 0 {
			issue = fmt.Sprintf("ready=%d/want=%d", ready, replicas)
		} else if updated != replicas && replicas > 0 {
			issue = fmt.Sprintf("updated=%d/want=%d", updated, replicas)
		} else if available != replicas && replicas > 0 {
			issue = fmt.Sprintf("available=%d/want=%d", available, replicas)
		}

		if issue != "" {
			result.Summary.Drifted++
			result.Drifted = append(result.Drifted, CondDriftEntry2056{
				Name: dep.Name, Namespace: dep.Namespace, Issue: issue,
			})
			score -= 3
		} else {
			result.Summary.Healthy++
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.Drifted, func(i, j int) bool {
		return result.Drifted[i].Namespace < result.Drifted[j].Namespace
	})

	if result.Summary.Drifted > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d deployments have condition drift — check rollout status", result.Summary.Drifted))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Pod Security Standard Validator
// ---------------------------------------------------------------

type PSSResult2056 struct {
	ScannedAt       time.Time      `json:"scannedAt"`
	HealthScore     int            `json:"healthScore"`
	Grade           string         `json:"grade"`
	Summary         PSSSummary2056 `json:"summary"`
	Violations      []PSSEntry2056 `json:"violations"`
	Recommendations []string       `json:"recommendations"`
}

type PSSSummary2056 struct {
	TotalNS       int `json:"totalNamespaces"`
	EnforcedNS    int `json:"enforcedNamespaces"`
	ViolatingPods int `json:"violatingPods"`
}

type PSSEntry2056 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Violation string `json:"violation"`
}

func (s *Server) handlePSSValidator(w http.ResponseWriter, r *http.Request) {
	result := PSSResult2056{ScannedAt: time.Now()}
	score := 100

	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	// Check namespace PSS labels
	enforcedNS := make(map[string]bool)
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true, "k8ops-system": true}
	for _, ns := range nsList.Items {
		if systemNS[ns.Name] {
			continue
		}
		result.Summary.TotalNS++
		if ns.Labels["pod-security.kubernetes.io/enforce"] != "" {
			enforcedNS[ns.Name] = true
			result.Summary.EnforcedNS++
		}
	}

	// Check pods for PSS violations (privileged, hostNetwork, hostPID)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning || systemNS[pod.Namespace] {
			continue
		}
		violations := []string{}
		for _, c := range pod.Spec.Containers {
			if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
				violations = append(violations, "privileged:"+c.Name)
			}
		}
		if pod.Spec.HostNetwork {
			violations = append(violations, "hostNetwork")
		}
		if pod.Spec.HostPID {
			violations = append(violations, "hostPID")
		}

		if len(violations) > 0 {
			result.Summary.ViolatingPods++
			result.Violations = append(result.Violations, PSSEntry2056{
				Pod: pod.Name, Namespace: pod.Namespace,
				Violation: fmt.Sprintf("%v", violations),
			})
			score -= 2
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.Violations, func(i, j int) bool {
		return result.Violations[i].Namespace < result.Violations[j].Namespace
	})

	if result.Summary.EnforcedNS < result.Summary.TotalNS {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d/%d namespaces without PSS enforcement — add pod-security labels", result.Summary.TotalNS-result.Summary.EnforcedNS, result.Summary.TotalNS))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Container Resource Equality
// ---------------------------------------------------------------

type ResEqResult2056 struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Summary         ResEqSummary2056 `json:"summary"`
	Inconsistent    []ResEqEntry2056 `json:"inconsistentDeployments"`
	Recommendations []string         `json:"recommendations"`
}

type ResEqSummary2056 struct {
	TotalDeploys int `json:"totalDeployments"`
	Consistent   int `json:"consistent"`
	Inconsistent int `json:"inconsistent"`
}

type ResEqEntry2056 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleResEquality(w http.ResponseWriter, r *http.Request) {
	result := ResEqResult2056{ScannedAt: time.Now()}
	score := 100

	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++

		containers := dep.Spec.Template.Spec.Containers
		if len(containers) <= 1 {
			result.Summary.Consistent++
			continue
		}

		// Check if all containers have same CPU/memory requests
		firstCPU := containers[0].Resources.Requests.Cpu()
		firstMem := containers[0].Resources.Requests.Memory()
		consistent := true

		for _, c := range containers[1:] {
			if c.Resources.Requests.Cpu().Cmp(*firstCPU) != 0 ||
				c.Resources.Requests.Memory().Cmp(*firstMem) != 0 {
				consistent = false
				break
			}
		}

		if consistent {
			result.Summary.Consistent++
		} else {
			result.Summary.Inconsistent++
			result.Inconsistent = append(result.Inconsistent, ResEqEntry2056{
				Name: dep.Name, Namespace: dep.Namespace,
			})
			score -= 1
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.Inconsistent, func(i, j int) bool {
		return result.Inconsistent[i].Namespace < result.Inconsistent[j].Namespace
	})

	if result.Summary.Inconsistent > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d deployments have inconsistent container resources — standardize for predictability", result.Summary.Inconsistent))
	}

	writeJSON(w, result)
}
