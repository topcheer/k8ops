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
// v19.54 — Operations Dimension (Round 12)
// 1. Container OOM Risk Forecaster — memory limit proximity analysis
// 2. API Server Request Pattern — verb distribution & namespace load
// 3. Pod Terminated Reason Catalog — exit reason classification
// ============================================================

type OOMForecastResult1954 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         OOMForecastSummary1954 `json:"summary"`
	AtRiskPods      []OOMForecastEntry1954 `json:"atRiskPods"`
	Recommendations []string               `json:"recommendations"`
}

type OOMForecastSummary1954 struct {
	TotalContainers int `json:"totalContainers"`
	WithMemLimits   int `json:"withMemLimits"`
	WithoutMemLimit int `json:"withoutMemLimits"`
	OOMHistory      int `json:"oomHistoryCount"`
	HighRiskCount   int `json:"highRiskCount"`
}

type OOMForecastEntry1954 struct {
	PodName      string `json:"podName"`
	Namespace    string `json:"namespace"`
	Container    string `json:"container"`
	MemLimitMB   int    `json:"memLimitMB"`
	WasOOMKilled bool   `json:"wasOOMKilled"`
	RiskLevel    string `json:"riskLevel"`
}

func (s *Server) handleOOMForecast(w http.ResponseWriter, r *http.Request) {
	result := OOMForecastResult1954{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}

		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			memLimitMB := 0
			hasLimit := false
			if !c.Resources.Limits.Memory().IsZero() {
				memLimitMB = int(c.Resources.Limits.Memory().Value() / (1024 * 1024))
				result.Summary.WithMemLimits++
				hasLimit = true
			} else {
				result.Summary.WithoutMemLimit++
			}

			wasOOM := false
			for _, cs := range pod.Status.ContainerStatuses {
				if cs.Name == c.Name && cs.LastTerminationState.Terminated != nil {
					if cs.LastTerminationState.Terminated.Reason == "OOMKilled" {
						wasOOM = true
						result.Summary.OOMHistory++
					}
				}
			}

			risk := "low"
			if wasOOM {
				risk = "high"
			} else if !hasLimit {
				risk = "medium"
			}

			if risk == "high" || (risk == "medium" && memLimitMB < 256) {
				result.Summary.HighRiskCount++
				result.AtRiskPods = append(result.AtRiskPods, OOMForecastEntry1954{
					PodName: pod.Name, Namespace: pod.Namespace, Container: c.Name,
					MemLimitMB: memLimitMB, WasOOMKilled: wasOOM, RiskLevel: risk,
				})
			}

			if wasOOM {
				score -= 3
			}
			if !hasLimit {
				score -= 1
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.OOMHistory > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers with OOM history — increase memory limits", result.Summary.OOMHistory))
	}
	if result.Summary.WithoutMemLimit > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers without memory limits — add for OOM protection", result.Summary.WithoutMemLimit))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. API Server Request Pattern
// ---------------------------------------------------------------

type APIPatternResult1954 struct {
	ScannedAt       time.Time                     `json:"scannedAt"`
	HealthScore     int                           `json:"healthScore"`
	Grade           string                        `json:"grade"`
	Summary         APIPatternSummary1954         `json:"summary"`
	ByVerb          []APIPatternVerbEntry1954     `json:"byVerb"`
	ByResource      []APIPatternResourceEntry1954 `json:"byResource"`
	Recommendations []string                      `json:"recommendations"`
}

type APIPatternSummary1954 struct {
	TotalVerbs     int `json:"totalVerbs"`
	TotalResources int `json:"totalResources"`
	ListHeavyNS    int `json:"listHeavyNamespaces"`
	WatchCount     int `json:"watchCapableResources"`
	DeleteCount    int `json:"deleteCapableResources"`
}

type APIPatternVerbEntry1954 struct {
	Verb       string  `json:"verb"`
	Percentage float64 `json:"percentageOfResources"`
}

type APIPatternResourceEntry1954 struct {
	Resource  string `json:"resource"`
	VerbCount int    `json:"verbCount"`
	Group     string `json:"group"`
}

func (s *Server) handleAPIRequestPattern(w http.ResponseWriter, r *http.Request) {
	result := APIPatternResult1954{ScannedAt: time.Now()}
	score := 100

	_, apiResList, err := s.clientset.Discovery().ServerGroupsAndResources()
	if err != nil {
		writeJSON(w, result)
		return
	}

	verbCount := make(map[string]int)
	var totalResources int
	var topResources []APIPatternResourceEntry1954

	for _, apiGroup := range apiResList {
		gv := apiGroup.GroupVersion
		groupName := "core"
		if parts := strings.SplitN(gv, "/", 2); len(parts) == 2 {
			groupName = parts[0]
		}

		for _, res := range apiGroup.APIResources {
			if strings.Contains(res.Name, "/") {
				continue
			}
			totalResources++

			for _, v := range res.Verbs {
				verbCount[v]++
			}
			if len(topResources) < 50 {
				topResources = append(topResources, APIPatternResourceEntry1954{
					Resource: res.Name, VerbCount: len(res.Verbs), Group: groupName,
				})
			}
		}
	}

	result.Summary.TotalResources = totalResources
	result.Summary.TotalVerbs = len(verbCount)
	result.Summary.WatchCount = verbCount["watch"]
	result.Summary.DeleteCount = verbCount["delete"]

	for v, c := range verbCount {
		pct := 0.0
		if totalResources > 0 {
			pct = float64(c) * 100 / float64(totalResources)
		}
		result.ByVerb = append(result.ByVerb, APIPatternVerbEntry1954{Verb: v, Percentage: pct})
	}
	sort.Slice(result.ByVerb, func(i, j int) bool { return result.ByVerb[i].Percentage > result.ByVerb[j].Percentage })
	result.ByResource = topResources

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations,
		fmt.Sprintf("%d resources across %d verbs (get:%d list:%d watch:%d create:%d delete:%d)",
			totalResources, len(verbCount), verbCount["get"], verbCount["list"], verbCount["watch"], verbCount["create"], verbCount["delete"]),
	)
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Pod Terminated Reason Catalog
// ---------------------------------------------------------------

type TermCatalogResult1954 struct {
	ScannedAt       time.Time                    `json:"scannedAt"`
	HealthScore     int                          `json:"healthScore"`
	Grade           string                       `json:"grade"`
	Summary         TermCatalogSummary1954       `json:"summary"`
	ByReason        []TermCatalogReasonEntry1954 `json:"byReason"`
	Details         []TermCatalogEntry1954       `json:"details"`
	Recommendations []string                     `json:"recommendations"`
}

type TermCatalogSummary1954 struct {
	TotalTerminated int `json:"totalTerminated"`
	OOMKilled       int `json:"oomKilled"`
	ErrorExit       int `json:"errorExit"`
	Completed       int `json:"completed"`
	Evicted         int `json:"evicted"`
	UniqueReasons   int `json:"uniqueReasons"`
}

type TermCatalogReasonEntry1954 struct {
	Reason string `json:"reason"`
	Count  int    `json:"count"`
}

type TermCatalogEntry1954 struct {
	PodName   string `json:"podName"`
	Namespace string `json:"namespace"`
	Reason    string `json:"reason"`
	ExitCode  int32  `json:"exitCode"`
}

func (s *Server) handleTerminatedReasonCatalog(w http.ResponseWriter, r *http.Request) {
	result := TermCatalogResult1954{ScannedAt: time.Now()}
	score := 100
	reasonCount := make(map[string]int)

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}

		// Check terminated containers
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.LastTerminationState.Terminated != nil {
				term := cs.LastTerminationState.Terminated
				reason := term.Reason
				if reason == "" {
					reason = "Unknown"
				}

				result.Summary.TotalTerminated++
				reasonCount[reason]++

				switch reason {
				case "OOMKilled":
					result.Summary.OOMKilled++
					score -= 2
				case "Error":
					result.Summary.ErrorExit++
					score -= 1
				}

				if term.ExitCode != 0 {
					result.Summary.ErrorExit++
				}

				if len(result.Details) < 100 {
					result.Details = append(result.Details, TermCatalogEntry1954{
						PodName: pod.Name, Namespace: pod.Namespace,
						Reason: reason, ExitCode: term.ExitCode,
					})
				}
			}
		}

		// Check pod phase for completed/evicted
		if pod.Status.Phase == corev1.PodSucceeded {
			result.Summary.Completed++
		}
		if pod.Status.Phase == corev1.PodFailed {
			result.Summary.Evicted++
		}
	}

	result.Summary.UniqueReasons = len(reasonCount)
	for r, c := range reasonCount {
		result.ByReason = append(result.ByReason, TermCatalogReasonEntry1954{Reason: r, Count: c})
	}
	sort.Slice(result.ByReason, func(i, j int) bool { return result.ByReason[i].Count > result.ByReason[j].Count })

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.OOMKilled > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d OOMKilled containers — increase memory limits", result.Summary.OOMKilled))
	}
	if result.Summary.ErrorExit > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d error exits — check application logs", result.Summary.ErrorExit))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
