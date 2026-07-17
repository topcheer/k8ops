package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HPABehaviorResult analyzes HPA scaling behavior and effectiveness.
// Unlike autoscale-recommendations (which suggests HPA configs), this engine
// evaluates existing HPA configurations: are scaling policies well-tuned?
// Is there flapping? Are scale-down delays appropriate? Are targets optimal?
type HPABehaviorResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         HPABehaviorSummary  `json:"summary"`
	HPAs            []HPABehaviorEntry  `json:"hpas"`
	Issues          []HPABehaviorIssue  `json:"issues"`
	ByNamespace     []HPABehaviorNSStat `json:"byNamespace"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

// HPABehaviorSummary aggregates HPA behavior statistics.
type HPABehaviorSummary struct {
	TotalHPAs         int     `json:"totalHPAs"`
	WithBehavior      int     `json:"withBehavior"`      // has scalingBehavior configured
	WithPolicies      int     `json:"withPolicies"`      // has scaleUp/scaleDown policies
	FlapRisk          int     `json:"flapRisk"`          // likely to flap (fast up + fast down)
	AggressiveScaleUp int     `json:"aggressiveScaleUp"` // very fast scale-up
	SlowScaleDown     int     `json:"slowScaleDown"`     // very slow scale-down
	AvgMinReplicas    float64 `json:"avgMinReplicas"`
	AvgMaxReplicas    float64 `json:"avgMaxReplicas"`
	AvgTargetCPU      float64 `json:"avgTargetCPU"`
	WithoutBehavior   int     `json:"withoutBehavior"`
}

// HPABehaviorEntry describes one HPA's behavior analysis.
type HPABehaviorEntry struct {
	Name            string `json:"name"`
	Namespace       string `json:"namespace"`
	TargetRef       string `json:"targetRef"`
	MinReplicas     int    `json:"minReplicas"`
	MaxReplicas     int    `json:"maxReplicas"`
	TargetCPU       int    `json:"targetCPU"`
	HasBehavior     bool   `json:"hasBehavior"`
	ScaleUpPolicy   string `json:"scaleUpPolicy"` // aggressive, moderate, conservative
	ScaleDownPolicy string `json:"scaleDownPolicy"`
	FlapRisk        string `json:"flapRisk"` // high, medium, low
	Issues          int    `json:"issues"`
	Score           int    `json:"score"`
}

// HPABehaviorIssue describes a configuration issue.
type HPABehaviorIssue struct {
	HPA        string `json:"hpa"`
	Namespace  string `json:"namespace"`
	Issue      string `json:"issue"`
	Severity   string `json:"severity"`
	Suggestion string `json:"suggestion"`
}

// HPABehaviorNSStat per-namespace HPA stats.
type HPABehaviorNSStat struct {
	Namespace    string  `json:"namespace"`
	HPACount     int     `json:"hpaCount"`
	WithBehavior int     `json:"withBehavior"`
	AvgTarget    float64 `json:"avgTargetCPU"`
}

// handleHPABehavior handles GET /api/scalability/hpa-behavior
func (s *Server) handleHPABehavior(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := HPABehaviorResult{ScannedAt: time.Now()}

	hpas, _ := rc.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Build pod count per workload
	podCounts := map[string]int{} // ns/name → pod count
	for _, pod := range pods.Items {
		if len(pod.OwnerReferences) > 0 {
			key := pod.Namespace + "/" + pod.OwnerReferences[0].Name
			podCounts[key]++
		}
	}

	nsStats := map[string]*HPABehaviorNSStat{}
	totalMinReps := 0
	totalMaxReps := 0
	totalTargetCPU := 0
	targetCount := 0

	for _, hpa := range hpas.Items {
		if isSystemNamespace(hpa.Namespace) {
			continue
		}

		result.Summary.TotalHPAs++
		entry := HPABehaviorEntry{
			Name:      hpa.Name,
			Namespace: hpa.Namespace,
			TargetRef: fmt.Sprintf("%s/%s", hpa.Spec.ScaleTargetRef.Kind, hpa.Spec.ScaleTargetRef.Name),
		}

		// Min/Max replicas
		minRep := 1
		if hpa.Spec.MinReplicas != nil {
			minRep = int(*hpa.Spec.MinReplicas)
		}
		maxRep := 1
		if hpa.Spec.MaxReplicas > 0 {
			maxRep = int(hpa.Spec.MaxReplicas)
		}
		entry.MinReplicas = minRep
		entry.MaxReplicas = maxRep
		totalMinReps += minRep
		totalMaxReps += maxRep

		// Target CPU
		targetCPU := 0
		for _, metric := range hpa.Spec.Metrics {
			if metric.Type == autoscalingv2.ResourceMetricSourceType && metric.Resource.Name == corev1.ResourceCPU {
				if metric.Resource.Target.AverageUtilization != nil {
					targetCPU = int(*metric.Resource.Target.AverageUtilization)
				}
			}
		}
		entry.TargetCPU = targetCPU
		if targetCPU > 0 {
			totalTargetCPU += targetCPU
			targetCount++
		}

		// Behavior analysis
		entry.HasBehavior = hpa.Spec.Behavior != nil
		if entry.HasBehavior {
			result.Summary.WithBehavior++

			// Scale-up policy
			if hpa.Spec.Behavior.ScaleUp != nil {
				entry.ScaleUpPolicy = classifyScalePolicy(hpa.Spec.Behavior.ScaleUp)
				if entry.ScaleUpPolicy == "aggressive" {
					result.Summary.AggressiveScaleUp++
				}
			}

			// Scale-down policy
			if hpa.Spec.Behavior.ScaleDown != nil {
				entry.ScaleDownPolicy = classifyScaleDownPolicy(hpa.Spec.Behavior.ScaleDown)
				if entry.ScaleDownPolicy == "slow" {
					result.Summary.SlowScaleDown++
				}
				// Check for stabilization
				if hpa.Spec.Behavior.ScaleDown.StabilizationWindowSeconds != nil &&
					*hpa.Spec.Behavior.ScaleDown.StabilizationWindowSeconds == 0 {
					entry.FlapRisk = "high"
					result.Summary.FlapRisk++
				}
			}

			// Check policies configured
			if len(hpa.Spec.Behavior.ScaleUp.Policies) > 0 || len(hpa.Spec.Behavior.ScaleDown.Policies) > 0 {
				result.Summary.WithPolicies++
			}
		} else {
			result.Summary.WithoutBehavior++
			entry.FlapRisk = "medium" // default behavior can flap
		}

		// Detect flapping risk: aggressive scale-up + no stabilization
		if entry.ScaleUpPolicy == "aggressive" && entry.ScaleDownPolicy != "slow" {
			entry.FlapRisk = "high"
			result.Issues = append(result.Issues, HPABehaviorIssue{
				HPA: hpa.Name, Namespace: hpa.Namespace,
				Issue:      "Flapping risk: aggressive scale-up without slow scale-down",
				Severity:   "warning",
				Suggestion: "Add stabilizationWindowSeconds to scaleDown (e.g., 300s)",
			})
			entry.Issues++
		}

		// Detect min=max (no scaling possible)
		if minRep == maxRep {
			result.Issues = append(result.Issues, HPABehaviorIssue{
				HPA: hpa.Name, Namespace: hpa.Namespace,
				Issue:      fmt.Sprintf("minReplicas == maxReplicas (%d) — HPA cannot scale", minRep),
				Severity:   "warning",
				Suggestion: "Increase maxReplicas to allow scaling",
			})
			entry.Issues++
		}

		// Detect very high target CPU (won't scale proactively)
		if targetCPU > 90 {
			result.Issues = append(result.Issues, HPABehaviorIssue{
				HPA: hpa.Name, Namespace: hpa.Namespace,
				Issue:      fmt.Sprintf("Target CPU %d%% is very high — HPA only reacts to saturation", targetCPU),
				Severity:   "info",
				Suggestion: "Lower target to 60-80% for proactive scaling",
			})
			entry.Issues++
		}

		// Score
		entry.Score = computeHPAScore(entry)

		result.HPAs = append(result.HPAs, entry)

		// Namespace stats
		if nsStats[hpa.Namespace] == nil {
			nsStats[hpa.Namespace] = &HPABehaviorNSStat{Namespace: hpa.Namespace}
		}
		nsStats[hpa.Namespace].HPACount++
		if entry.HasBehavior {
			nsStats[hpa.Namespace].WithBehavior++
		}
	}

	// Finalize summary averages
	if result.Summary.TotalHPAs > 0 {
		result.Summary.AvgMinReplicas = float64(totalMinReps) / float64(result.Summary.TotalHPAs)
		result.Summary.AvgMaxReplicas = float64(totalMaxReps) / float64(result.Summary.TotalHPAs)
	}
	if targetCount > 0 {
		result.Summary.AvgTargetCPU = float64(totalTargetCPU) / float64(targetCount)
	}

	// Sort HPAs by score (lowest first)
	sort.Slice(result.HPAs, func(i, j int) bool { return result.HPAs[i].Score < result.HPAs[j].Score })

	// Namespace stats
	for _, ns := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}

	// Score
	result.HealthScore = computeOverallHPAScore(result.Summary)
	result.Grade = scoreToGrade(result.HealthScore)

	// Recs
	result.Recommendations = generateHPABehaviorRecs(result)

	writeJSON(w, result)
}

// classifyScalePolicy classifies scale-up behavior.
func classifyScalePolicy(scaleUp *autoscalingv2.HPAScalingRules) string {
	if scaleUp == nil || len(scaleUp.Policies) == 0 {
		return "default"
	}
	for _, p := range scaleUp.Policies {
		if p.Type == autoscalingv2.PodsScalingPolicy && p.Value > 4 {
			return "aggressive"
		}
		if p.Type == autoscalingv2.PercentScalingPolicy && p.Value > 50 {
			return "aggressive"
		}
	}
	return "moderate"
}

// classifyScaleDownPolicy classifies scale-down behavior.
func classifyScaleDownPolicy(scaleDown *autoscalingv2.HPAScalingRules) string {
	if scaleDown == nil {
		return "default"
	}
	if scaleDown.StabilizationWindowSeconds != nil && *scaleDown.StabilizationWindowSeconds > 300 {
		return "slow"
	}
	return "fast"
}

// computeHPAScore computes per-HPA score.
func computeHPAScore(e HPABehaviorEntry) int {
	score := 100
	if !e.HasBehavior {
		score -= 20
	}
	if e.FlapRisk == "high" {
		score -= 25
	} else if e.FlapRisk == "medium" {
		score -= 10
	}
	if e.MinReplicas == e.MaxReplicas {
		score -= 20
	}
	if e.TargetCPU > 90 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	return score
}

// computeOverallHPAScore computes aggregate HPA health.
func computeOverallHPAScore(s HPABehaviorSummary) int {
	score := 100
	if s.TotalHPAs == 0 {
		return 100 // no HPAs = neutral
	}
	// Without behavior is suboptimal
	if s.WithoutBehavior > 0 {
		behaviorRatio := float64(s.WithoutBehavior) / float64(s.TotalHPAs)
		score -= int(behaviorRatio * 20)
	}
	// Flapping risk
	if s.FlapRisk > 0 {
		flapRatio := float64(s.FlapRisk) / float64(s.TotalHPAs)
		score -= int(flapRatio * 30)
	}
	// Aggressive scaling without stabilization
	if s.AggressiveScaleUp > 0 {
		score -= minInt(s.AggressiveScaleUp*5, 15)
	}
	if score < 0 {
		score = 0
	}
	return score
}

// generateHPABehaviorRecs produces recommendations.
func generateHPABehaviorRecs(r HPABehaviorResult) []string {
	var recs []string

	recs = append(recs, fmt.Sprintf("HPA behavior: %d HPAs (%d with behavior config, %d flap risk) — score %d/100",
		r.Summary.TotalHPAs, r.Summary.WithBehavior, r.Summary.FlapRisk, r.HealthScore))

	if r.Summary.WithoutBehavior > 0 {
		recs = append(recs, fmt.Sprintf("%d HPA(s) without behavior config — add scaleDown stabilization to prevent flapping", r.Summary.WithoutBehavior))
	}

	if r.Summary.FlapRisk > 0 {
		recs = append(recs, fmt.Sprintf("%d HPA(s) with high flap risk — configure stabilizationWindowSeconds >= 300", r.Summary.FlapRisk))
	}

	for _, issue := range r.Issues {
		if issue.Severity == "warning" {
			recs = append(recs, fmt.Sprintf("%s/%s: %s — %s", issue.Namespace, issue.HPA, issue.Issue, issue.Suggestion))
		}
	}

	return recs
}
