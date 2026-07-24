package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v19.53 — Deployment Dimension (Round 12)
// 1. Canary Release Detector — progressive delivery analysis
// 2. Init Container Overhead — startup delay & resource cost
// 3. Lifecycle Hook Compliance — preStop/postStart coverage
// ============================================================

// ---------------------------------------------------------------
// 1. Canary Release Detector
// ---------------------------------------------------------------

type CanaryResult1953 struct {
	ScannedAt         time.Time             `json:"scannedAt"`
	HealthScore       int                   `json:"healthScore"`
	Grade             string                `json:"grade"`
	Summary           CanarySummary1953     `json:"summary"`
	CanaryDeployments []CanaryEntry1953     `json:"canaryDeployments"`
	Candidates        []CanaryCandidate1953 `json:"candidates"`
	Recommendations   []string              `json:"recommendations"`
}

type CanarySummary1953 struct {
	TotalDeployments int `json:"totalDeployments"`
	CanaryEnabled    int `json:"canaryEnabled"`
	BlueGreenEnabled int `json:"blueGreenEnabled"`
	WithAnnotations  int `json:"withDeliveryAnnotations"`
	HighTrafficSplit int `json:"highTrafficSplitCount"`
}

type CanaryEntry1953 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Mechanism string `json:"mechanism"`
	CanaryPct string `json:"canaryPercent"`
}

type CanaryCandidate1953 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Replicas  int32  `json:"replicas"`
	Reason    string `json:"reason"`
}

func (s *Server) handleCanaryDetector(w http.ResponseWriter, r *http.Request) {
	result := CanaryResult1953{ScannedAt: time.Now()}
	score := 100

	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range depList.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalDeployments++

		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}

		// Check for canary/progressive delivery annotations
		mechanism := ""
		canaryPct := ""
		hasDeliveryAnnot := false

		for k, v := range dep.Annotations {
			kl := strings.ToLower(k)
			if strings.Contains(kl, "canary") {
				mechanism = "canary"
				hasDeliveryAnnot = true
				if strings.Contains(kl, "weight") || strings.Contains(kl, "percent") {
					canaryPct = v
				}
			}
			if strings.Contains(kl, "blue") && strings.Contains(kl, "green") {
				mechanism = "blue-green"
				hasDeliveryAnnot = true
			}
			if strings.Contains(kl, "argoproj.io") || strings.Contains(kl, "flagger") || strings.Contains(kl, "istio.io/weight") {
				if mechanism == "" {
					mechanism = "progressive"
				}
				hasDeliveryAnnot = true
				if v != "" {
					canaryPct = v
				}
			}
		}

		if hasDeliveryAnnot {
			result.Summary.WithAnnotations++
			if mechanism == "canary" || mechanism == "progressive" {
				result.Summary.CanaryEnabled++
				result.CanaryDeployments = append(result.CanaryDeployments, CanaryEntry1953{
					Name: dep.Name, Namespace: dep.Namespace,
					Mechanism: mechanism, CanaryPct: canaryPct,
				})
			} else if mechanism == "blue-green" {
				result.Summary.BlueGreenEnabled++
				result.CanaryDeployments = append(result.CanaryDeployments, CanaryEntry1953{
					Name: dep.Name, Namespace: dep.Namespace,
					Mechanism: mechanism, CanaryPct: "100/0",
				})
			}
		} else if replicas >= 3 {
			// Candidate for canary: multi-replica without progressive delivery
			result.Candidates = append(result.Candidates, CanaryCandidate1953{
				Name: dep.Name, Namespace: dep.Namespace,
				Replicas: replicas,
				Reason:   "Multi-replica deployment without progressive delivery — consider canary",
			})
			score -= 1
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if len(result.Candidates) > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d deployments are canary candidates (3+ replicas, no progressive delivery)", len(result.Candidates)))
	}
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d/%d deployments with progressive delivery (%d canary, %d blue-green)",
		result.Summary.CanaryEnabled+result.Summary.BlueGreenEnabled, result.Summary.TotalDeployments,
		result.Summary.CanaryEnabled, result.Summary.BlueGreenEnabled))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Init Container Overhead
// ---------------------------------------------------------------

type InitContainerResult1953 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         InitContainerSummary1953 `json:"summary"`
	Pods            []InitContainerEntry1953 `json:"pods"`
	HeavyInitConts  []InitContainerHeavy1953 `json:"heavyInitContainers"`
	Recommendations []string                 `json:"recommendations"`
}

type InitContainerSummary1953 struct {
	TotalPods        int     `json:"totalPods"`
	PodsWithInit     int     `json:"podsWithInitContainers"`
	TotalInitConts   int     `json:"totalInitContainers"`
	AvgInitPerPod    float64 `json:"avgInitPerPod"`
	HighOverheadPods int     `json:"highOverheadPods"`
}

type InitContainerEntry1953 struct {
	PodName   string `json:"podName"`
	Namespace string `json:"namespace"`
	InitCount int    `json:"initContainerCount"`
}

type InitContainerHeavy1953 struct {
	PodName   string `json:"podName"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
	Issue     string `json:"issue"`
}

func (s *Server) handleInitContainerOverhead(w http.ResponseWriter, r *http.Request) {
	result := InitContainerResult1953{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		initCount := len(pod.Spec.InitContainers)
		if initCount > 0 {
			result.Summary.PodsWithInit++
			result.Summary.TotalInitConts += initCount

			result.Pods = append(result.Pods, InitContainerEntry1953{
				PodName: pod.Name, Namespace: pod.Namespace, InitCount: initCount,
			})

			// Check for heavy init containers
			for _, ic := range pod.Spec.InitContainers {
				cpuReq := ic.Resources.Requests.Cpu().AsApproximateFloat64()
				memReqMB := ic.Resources.Requests.Memory().Value() / (1024 * 1024)
				if cpuReq > 1 || memReqMB > 512 {
					issue := fmt.Sprintf("Heavy init container: %.1f CPU, %dMB memory", cpuReq, memReqMB)
					if len(result.HeavyInitConts) < 50 {
						result.HeavyInitConts = append(result.HeavyInitConts, InitContainerHeavy1953{
							PodName: pod.Name, Namespace: pod.Namespace,
							Container: ic.Name, Issue: issue,
						})
					}
					score -= 1
				}
			}

			if initCount > 3 {
				score -= 1
			}
		}
	}

	if result.Summary.PodsWithInit > 0 {
		result.Summary.AvgInitPerPod = float64(result.Summary.TotalInitConts) / float64(result.Summary.PodsWithInit)
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if len(result.HeavyInitConts) > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d heavy init containers (>1 CPU or >512MB) — optimize for faster startup", len(result.HeavyInitConts)))
	}
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods with %d init containers (avg %.1f per pod)",
		result.Summary.PodsWithInit, result.Summary.TotalInitConts, result.Summary.AvgInitPerPod))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Lifecycle Hook Compliance
// ---------------------------------------------------------------

type LifecycleHookResult1953 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         LifecycleHookSummary1953 `json:"summary"`
	Missing         []LifecycleHookEntry1953 `json:"missingHooks"`
	WithHooks       []LifecycleHookEntry1953 `json:"withHooks"`
	Recommendations []string                 `json:"recommendations"`
}

type LifecycleHookSummary1953 struct {
	TotalContainers  int `json:"totalContainers"`
	WithPreStop      int `json:"withPreStop"`
	WithoutPreStop   int `json:"withoutPreStop"`
	WithPostStart    int `json:"withPostStart"`
	WithoutPostStart int `json:"withoutPostStart"`
	GracePeriod30    int `json:"gracePeriodLt30"`
}

type LifecycleHookEntry1953 struct {
	PodName   string   `json:"podName"`
	Namespace string   `json:"namespace"`
	Container string   `json:"container"`
	Missing   []string `json:"missing"`
}

func (s *Server) handleLifecycleHookComp(w http.ResponseWriter, r *http.Request) {
	result := LifecycleHookResult1953{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}

		gracePeriod := int64(30)
		if pod.Spec.TerminationGracePeriodSeconds != nil {
			gracePeriod = *pod.Spec.TerminationGracePeriodSeconds
		}
		if gracePeriod < 30 {
			result.Summary.GracePeriod30++
			score -= 1
		}

		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			hasPreStop := false
			hasPostStart := false

			if c.Lifecycle != nil {
				if c.Lifecycle.PreStop != nil {
					hasPreStop = true
					result.Summary.WithPreStop++
				}
				if c.Lifecycle.PostStart != nil {
					hasPostStart = true
					result.Summary.WithPostStart++
				}
			}

			var missing []string
			if !hasPreStop {
				result.Summary.WithoutPreStop++
				missing = append(missing, "preStop")
			}
			if !hasPostStart {
				result.Summary.WithoutPostStart++
				missing = append(missing, "postStart")
			}

			if len(missing) > 0 {
				if len(result.Missing) < 100 {
					result.Missing = append(result.Missing, LifecycleHookEntry1953{
						PodName: pod.Name, Namespace: pod.Namespace,
						Container: c.Name, Missing: missing,
					})
				}
				if len(missing) == 2 {
					score -= 1
				}
			} else {
				if len(result.WithHooks) < 50 {
					result.WithHooks = append(result.WithHooks, LifecycleHookEntry1953{
						PodName: pod.Name, Namespace: pod.Namespace,
						Container: c.Name, Missing: []string{},
					})
				}
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.WithoutPreStop > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers without preStop hook — add graceful shutdown", result.Summary.WithoutPreStop))
	}
	if result.Summary.GracePeriod30 > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods with grace period <30s — increase for clean shutdown", result.Summary.GracePeriod30))
	}
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d/%d containers with preStop, %d/%d with postStart",
		result.Summary.WithPreStop, result.Summary.TotalContainers,
		result.Summary.WithPostStart, result.Summary.TotalContainers))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
