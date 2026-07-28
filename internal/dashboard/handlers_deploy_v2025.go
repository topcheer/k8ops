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
// v20.25 — Deployment Dimension (Round 24)
// 1. Pod Toleration Scope — toleration key/effect distribution
// 2. Container Port HostPort Map — hostPort mapping audit
// 3. Deployment Progress Deadline — progressDeadlineSeconds compliance
// ============================================================

// ---------------------------------------------------------------
// 1. Pod Toleration Scope
// ---------------------------------------------------------------

type TolScopeResult2025 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         TolScopeSummary2025 `json:"summary"`
	Tolerations     []TolScopeEntry2025 `json:"tolerations"`
	Recommendations []string            `json:"recommendations"`
}

type TolScopeSummary2025 struct {
	TotalPods   int `json:"totalPods"`
	WithTol     int `json:"withTolerations"`
	NoSchedule  int `json:"noScheduleTolerations"`
	NoExecute   int `json:"noExecuteTolerations"`
	CatchAllTol int `json:"catchAllTolerations"`
}

type TolScopeEntry2025 struct {
	Key    string `json:"key"`
	Effect string `json:"effect"`
	Count  int    `json:"podCount"`
}

func (s *Server) handleTolScope(w http.ResponseWriter, r *http.Request) {
	result := TolScopeResult2025{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	tolMap := make(map[string]*TolScopeEntry2025)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		if len(pod.Spec.Tolerations) == 0 {
			continue
		}
		result.Summary.WithTol++

		catchAll := false
		for _, tol := range pod.Spec.Tolerations {
			effect := string(tol.Effect)
			key := tol.Key

			if key == "" && effect == "" {
				catchAll = true
			}

			mapKey := key + ":" + effect
			entry, ok := tolMap[mapKey]
			if !ok {
				entry = &TolScopeEntry2025{Key: key, Effect: effect}
				tolMap[mapKey] = entry
			}
			entry.Count++

			if effect == "NoSchedule" {
				result.Summary.NoSchedule++
			} else if effect == "NoExecute" {
				result.Summary.NoExecute++
			}
		}

		if catchAll {
			result.Summary.CatchAllTol++
			score -= 3
		}
	}

	for _, e := range tolMap {
		result.Tolerations = append(result.Tolerations, *e)
	}
	sort.Slice(result.Tolerations, func(i, j int) bool {
		return result.Tolerations[i].Count > result.Tolerations[j].Count
	})
	if len(result.Tolerations) > 15 {
		result.Tolerations = result.Tolerations[:15]
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods: %d with tolerations, %d catch-all, %d NoExecute", result.Summary.TotalPods, result.Summary.WithTol, result.Summary.CatchAllTol, result.Summary.NoExecute))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Container Port HostPort Map
// ---------------------------------------------------------------

type HostPortResult2025 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         HostPortSummary2025 `json:"summary"`
	Mappings        []HostPortEntry2025 `json:"hostPortMappings"`
	Recommendations []string            `json:"recommendations"`
}

type HostPortSummary2025 struct {
	TotalContainers int `json:"totalContainers"`
	WithHostPort    int `json:"withHostPort"`
	TotalHostPorts  int `json:"totalHostPorts"`
	PrivilegedPorts int `json:"privilegedPorts"`
}

type HostPortEntry2025 struct {
	Pod           string `json:"pod"`
	Namespace     string `json:"namespace"`
	ContainerPort int    `json:"containerPort"`
	HostPort      int    `json:"hostPort"`
}

func (s *Server) handleHostPortMap(w http.ResponseWriter, r *http.Request) {
	result := HostPortResult2025{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			for _, p := range c.Ports {
				if p.HostPort > 0 {
					result.Summary.TotalHostPorts++
					result.Summary.WithHostPort++

					entry := HostPortEntry2025{
						Pod: pod.Name, Namespace: pod.Namespace,
						ContainerPort: int(p.ContainerPort),
						HostPort:      int(p.HostPort),
					}

					if p.HostPort < 1024 {
						result.Summary.PrivilegedPorts++
						score -= 3
					}

					result.Mappings = append(result.Mappings, entry)
				}
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers: %d with hostPort, %d total ports, %d privileged", result.Summary.TotalContainers, result.Summary.WithHostPort, result.Summary.TotalHostPorts, result.Summary.PrivilegedPorts))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Deployment Progress Deadline
// ---------------------------------------------------------------

type ProgDeadResult2025 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         ProgDeadSummary2025 `json:"summary"`
	Deployments     []ProgDeadEntry2025 `json:"deployments"`
	Recommendations []string            `json:"recommendations"`
}

type ProgDeadSummary2025 struct {
	TotalDeployments int `json:"totalDeployments"`
	WithDeadline     int `json:"withProgressDeadline"`
	UsingDefault     int `json:"usingDefault"`
	WithTimeout      int `json:"withTimeoutHit"`
}

type ProgDeadEntry2025 struct {
	Name             string `json:"name"`
	Namespace        string `json:"namespace"`
	ProgressDeadline int32  `json:"progressDeadlineSeconds"`
}

func (s *Server) handleProgDeadline(w http.ResponseWriter, r *http.Request) {
	result := ProgDeadResult2025{ScannedAt: time.Now()}
	score := 100

	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range depList.Items {
		result.Summary.TotalDeployments++

		entry := ProgDeadEntry2025{
			Name: dep.Name, Namespace: dep.Namespace,
		}

		if dep.Spec.ProgressDeadlineSeconds != nil {
			entry.ProgressDeadline = *dep.Spec.ProgressDeadlineSeconds
			result.Summary.WithDeadline++

			// Check if progress deadline was hit
			for _, cond := range dep.Status.Conditions {
				if cond.Type == "Progressing" && cond.Reason == "ProgressDeadlineExceeded" {
					result.Summary.WithTimeout++
					score -= 5
				}
			}
		} else {
			result.Summary.UsingDefault++
			entry.ProgressDeadline = 600 // default
		}

		result.Deployments = append(result.Deployments, entry)
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d deployments: %d with deadline, %d default, %d timed out", result.Summary.TotalDeployments, result.Summary.WithDeadline, result.Summary.UsingDefault, result.Summary.WithTimeout))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
