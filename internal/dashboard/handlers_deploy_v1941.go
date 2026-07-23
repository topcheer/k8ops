package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v19.41 — Deployment Dimension (Round 10)
// 1. Deployment Pause Detector — paused rollouts & stale updates
// 2. Image Tag Compliance — versioned tags vs floating tags
// 3. Rollout Strategy Analyzer — RollingUpdate vs Recreate analysis
// ============================================================

// ---------------------------------------------------------------
// 1. Deployment Pause Detector
// ---------------------------------------------------------------

type DeployPauseResult1941 struct {
	ScannedAt         time.Time              `json:"scannedAt"`
	HealthScore       int                    `json:"healthScore"`
	Grade             string                 `json:"grade"`
	Summary           DeployPauseSummary1941 `json:"summary"`
	PausedDeployments []DeployPauseEntry1941 `json:"pausedDeployments"`
	StaleDeployments  []DeployStaleEntry1941 `json:"staleDeployments"`
	Recommendations   []string               `json:"recommendations"`
}

type DeployPauseSummary1941 struct {
	TotalDeployments  int     `json:"totalDeployments"`
	PausedCount       int     `json:"pausedCount"`
	StaleCount        int     `json:"staleCount"`
	UpdatedReplicas   int     `json:"deploymentsWithUpdatedReplicas"`
	IncompleteRollout int     `json:"incompleteRollout"`
	MaxStaleDays      float64 `json:"maxStaleDays"`
}

type DeployPauseEntry1941 struct {
	Name            string `json:"name"`
	Namespace       string `json:"namespace"`
	Replicas        int32  `json:"replicas"`
	ReadyReplicas   int32  `json:"readyReplicas"`
	UpdatedReplicas int32  `json:"updatedReplicas"`
	Age             string `json:"age"`
}

type DeployStaleEntry1941 struct {
	Name      string  `json:"name"`
	Namespace string  `json:"namespace"`
	AgeDays   float64 `json:"ageDays"`
	Reason    string  `json:"reason"`
}

func (s *Server) handleDeployPauseDetect(w http.ResponseWriter, r *http.Request) {
	result := DeployPauseResult1941{ScannedAt: time.Now()}
	score := 100

	depList, err := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	for _, dep := range depList.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalDeployments++

		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}
		ready := dep.Status.ReadyReplicas
		updated := dep.Status.UpdatedReplicas
		ageDays := time.Since(dep.CreationTimestamp.Time).Hours() / 24

		// Check paused
		if dep.Spec.Paused {
			result.Summary.PausedCount++
			result.PausedDeployments = append(result.PausedDeployments, DeployPauseEntry1941{
				Name: dep.Name, Namespace: dep.Namespace,
				Replicas: replicas, ReadyReplicas: ready, UpdatedReplicas: updated,
				Age: fmt.Sprintf("%.0fd", ageDays),
			})
			score -= 5
		}

		// Check incomplete rollout
		if updated < replicas && replicas > 0 && !dep.Spec.Paused {
			result.Summary.IncompleteRollout++
			result.StaleDeployments = append(result.StaleDeployments, DeployStaleEntry1941{
				Name: dep.Name, Namespace: dep.Namespace, AgeDays: ageDays,
				Reason: fmt.Sprintf("Rollout incomplete: %d/%d updated", updated, replicas),
			})
			score -= 3
		}

		// Stale: not updated in 90 days
		if ageDays > 90 {
			result.Summary.StaleCount++
			if ageDays > result.Summary.MaxStaleDays {
				result.Summary.MaxStaleDays = ageDays
			}
			result.StaleDeployments = append(result.StaleDeployments, DeployStaleEntry1941{
				Name: dep.Name, Namespace: dep.Namespace, AgeDays: ageDays,
				Reason: fmt.Sprintf("Not updated in %.0f days — review for modernization", ageDays),
			})
			score -= 1
		}

		if updated > 0 && updated == replicas {
			result.Summary.UpdatedReplicas++
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.PausedCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d paused deployments — resume or delete", result.Summary.PausedCount))
	}
	if result.Summary.IncompleteRollout > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d incomplete rollouts — check pod health", result.Summary.IncompleteRollout))
	}
	if result.Summary.StaleCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d stale deployments (>90 days) — review for updates", result.Summary.StaleCount))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Image Tag Compliance
// ---------------------------------------------------------------

type TagComplianceResult1941 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         TagComplianceSummary1941 `json:"summary"`
	Violations      []TagViolationEntry1941  `json:"violations"`
	CompliantImages []TagCompliantEntry1941  `json:"compliantImages"`
	Recommendations []string                 `json:"recommendations"`
}

type TagComplianceSummary1941 struct {
	TotalImages   int `json:"totalImages"`
	CompliantTags int `json:"compliantTags"`
	LatestTags    int `json:"latestTags"`
	FloatingTags  int `json:"floatingTags"`
	PinnedDigest  int `json:"pinnedDigest"`
	NoTag         int `json:"noTag"`
	VersionedTags int `json:"versionedTags"`
}

type TagViolationEntry1941 struct {
	Image     string `json:"image"`
	Workload  string `json:"workload"`
	Namespace string `json:"namespace"`
	Violation string `json:"violation"`
	Severity  string `json:"severity"`
}

type TagCompliantEntry1941 struct {
	Image     string `json:"image"`
	TagType   string `json:"tagType"`
	Workloads int    `json:"workloadCount"`
}

func (s *Server) handleTagCompliance(w http.ResponseWriter, r *http.Request) {
	result := TagComplianceResult1941{ScannedAt: time.Now()}
	score := 100

	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	imgMap := make(map[string]map[string]bool) // image -> set of workloads

	for _, dep := range depList.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		for _, c := range dep.Spec.Template.Spec.Containers {
			img := c.Image
			if imgMap[img] == nil {
				imgMap[img] = make(map[string]bool)
			}
			imgMap[img][dep.Name] = true

			result.Summary.TotalImages++

			tagType := "versioned"
			_ = tagType // used for future tagType reporting
			violation := ""
			severity := ""

			if strings.Contains(img, "@sha256:") {
				tagType = "pinned-digest"
				result.Summary.PinnedDigest++
				result.Summary.CompliantTags++
			} else if strings.HasSuffix(img, ":latest") {
				tagType = "latest"
				violation = "Uses :latest tag — not reproducible"
				severity = "high"
				result.Summary.LatestTags++
			} else if !strings.Contains(img, ":") {
				tagType = "no-tag"
				violation = "No tag specified — defaults to :latest"
				severity = "high"
				result.Summary.NoTag++
			} else {
				// Check if versioned (has :v or :number)
				parts := strings.Split(img, ":")
				tag := ""
				if len(parts) > 1 {
					tag = parts[len(parts)-1]
				}
				if tag != "" && tag != "latest" {
					if strings.HasPrefix(tag, "v") || isNumericTag(tag) {
						tagType = "versioned"
						result.Summary.VersionedTags++
						result.Summary.CompliantTags++
					} else {
						tagType = "floating"
						violation = fmt.Sprintf("Tag '%s' is not versioned — may change", tag)
						severity = "medium"
						result.Summary.FloatingTags++
					}
				}
			}

			if violation != "" {
				result.Violations = append(result.Violations, TagViolationEntry1941{
					Image: img, Workload: dep.Name, Namespace: dep.Namespace,
					Violation: violation, Severity: severity,
				})
				if severity == "high" {
					score -= 3
				} else {
					score -= 1
				}
			}
		}
	}

	// Build compliant summary
	for img, wls := range imgMap {
		tagType := "versioned"
		if strings.Contains(img, "@sha256:") {
			tagType = "pinned-digest"
		} else if strings.HasSuffix(img, ":latest") || !strings.Contains(img, ":") {
			tagType = "floating"
		}
		result.CompliantImages = append(result.CompliantImages, TagCompliantEntry1941{
			Image: img, TagType: tagType, Workloads: len(wls),
		})
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.LatestTags > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d images use :latest — pin to specific versions", result.Summary.LatestTags))
	}
	if result.Summary.FloatingTags > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d floating tags — use semantic versioning", result.Summary.FloatingTags))
	}
	if result.Summary.PinnedDigest > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d images pinned to digest — best practice", result.Summary.PinnedDigest))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

func isNumericTag(tag string) bool {
	for _, c := range tag {
		if c >= '0' && c <= '9' {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------
// 3. Rollout Strategy Analyzer
// ---------------------------------------------------------------

type RolloutStrategyResult1941 struct {
	ScannedAt       time.Time                  `json:"scannedAt"`
	HealthScore     int                        `json:"healthScore"`
	Grade           string                     `json:"grade"`
	Summary         RolloutStrategySummary1941 `json:"summary"`
	Deployments     []RolloutStrategyEntry1941 `json:"deployments"`
	Risks           []RolloutStrategyRisk1941  `json:"risks"`
	Recommendations []string                   `json:"recommendations"`
}

type RolloutStrategySummary1941 struct {
	TotalDeployments    int `json:"totalDeployments"`
	RollingUpdate       int `json:"rollingUpdate"`
	Recreate            int `json:"recreate"`
	WithMaxSurge        int `json:"withMaxSurge"`
	WithMaxUnavailable  int `json:"withMaxUnavailable"`
	HighSurgeCount      int `json:"highSurgeCount"`
	ZeroDowntimeCapable int `json:"zeroDowntimeCapable"`
}

type RolloutStrategyEntry1941 struct {
	Name           string `json:"name"`
	Namespace      string `json:"namespace"`
	Strategy       string `json:"strategy"`
	MaxSurge       string `json:"maxSurge"`
	MaxUnavailable string `json:"maxUnavailable"`
	Replicas       int32  `json:"replicas"`
	ZeroDowntime   bool   `json:"zeroDowntimeCapable"`
}

type RolloutStrategyRisk1941 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	RiskType  string `json:"riskType"`
	Severity  string `json:"severity"`
	Detail    string `json:"detail"`
}

func (s *Server) handleRolloutStrategy(w http.ResponseWriter, r *http.Request) {
	result := RolloutStrategyResult1941{ScannedAt: time.Now()}
	score := 100

	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range depList.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalDeployments++

		strategy := string(dep.Spec.Strategy.Type)
		maxSurge := "25%"
		maxUnavailable := "25%"

		if dep.Spec.Strategy.RollingUpdate != nil {
			if dep.Spec.Strategy.RollingUpdate.MaxSurge != nil {
				maxSurge = dep.Spec.Strategy.RollingUpdate.MaxSurge.String()
			}
			if dep.Spec.Strategy.RollingUpdate.MaxUnavailable != nil {
				maxUnavailable = dep.Spec.Strategy.RollingUpdate.MaxUnavailable.String()
			}
		}

		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}

		zeroDowntime := false
		if strategy == "RollingUpdate" && maxUnavailable == "0" {
			zeroDowntime = true
			result.Summary.ZeroDowntimeCapable++
		}

		entry := RolloutStrategyEntry1941{
			Name: dep.Name, Namespace: dep.Namespace,
			Strategy: strategy, MaxSurge: maxSurge, MaxUnavailable: maxUnavailable,
			Replicas: replicas, ZeroDowntime: zeroDowntime,
		}
		result.Deployments = append(result.Deployments, entry)

		if strategy == "RollingUpdate" {
			result.Summary.RollingUpdate++
			result.Summary.WithMaxSurge++
			result.Summary.WithMaxUnavailable++

			// High surge risk
			if strings.Contains(maxSurge, "100") || maxSurge == "0" {
				result.Summary.HighSurgeCount++
			}
		} else if strategy == "Recreate" {
			result.Summary.Recreate++
			if replicas > 1 {
				result.Risks = append(result.Risks, RolloutStrategyRisk1941{
					Name: dep.Name, Namespace: dep.Namespace,
					RiskType: "recreate-downtime", Severity: "high",
					Detail: fmt.Sprintf("Recreate strategy with %d replicas — causes downtime during rollout", replicas),
				})
				score -= 3
			}
		}

		// MaxUnavailable=0 + single replica = can't rollout
		if maxUnavailable == "0" && replicas == 1 {
			result.Risks = append(result.Risks, RolloutStrategyRisk1941{
				Name: dep.Name, Namespace: dep.Namespace,
				RiskType: "single-replica-zero-unavailable", Severity: "high",
				Detail: "maxUnavailable=0 with single replica — rollout deadlock",
			})
			score -= 5
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.Recreate > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d Recreate deployments — switch to RollingUpdate for zero-downtime", result.Summary.Recreate))
	}
	if result.Summary.ZeroDowntimeCapable > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d zero-downtime capable deployments — best practice", result.Summary.ZeroDowntimeCapable))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// Suppress unused import
var _ appsv1.Deployment = appsv1.Deployment{}
