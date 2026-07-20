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

// HPACooldownResult analyzes HPA scaling behavior and cooldown configuration.
type HPACooldownResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         HPACooldownSummary `json:"summary"`
	ByHPA           []HPACooldownEntry `json:"byHPA"`
	RiskyHPAs       []HPACooldownEntry `json:"riskyHPAs"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Recommendations []string           `json:"recommendations"`
}

type HPACooldownSummary struct {
	TotalHPAs           int `json:"totalHPAs"`
	WithBehavior        int `json:"withScalingBehavior"`
	WithCooldown        int `json:"withScaleDownStabilization"`
	AggressiveUpscale   int `json:"aggressiveUpscale"`
	AggressiveDownscale int `json:"aggressiveDownscale"`
	NoMinReplicas       int `json:"withoutMinReplicas"`
	HighMinReplicas     int `json:"highMinReplicas"`
}

type HPACooldownEntry struct {
	Name                string   `json:"name"`
	Namespace           string   `json:"namespace"`
	Target              string   `json:"scaleTarget"`
	MinReplicas         int32    `json:"minReplicas"`
	MaxReplicas         int32    `json:"maxReplicas"`
	MetricCount         int      `json:"metricCount"`
	HasBehavior         bool     `json:"hasBehavior"`
	UpscaleAggressive   bool     `json:"upscaleAggressive"`
	DownscaleAggressive bool     `json:"downscaleAggressive"`
	RiskLevel           string   `json:"riskLevel"`
	Issues              []string `json:"issues"`
}

// handleHPACooldownAudit handles GET /api/scalability/hpa-cooldown-audit
func (s *Server) handleHPACooldownAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := HPACooldownResult{ScannedAt: time.Now()}

	hpas, err := rc.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	for _, hpa := range hpas.Items {
		if isSystemNamespace(hpa.Namespace) {
			continue
		}
		result.Summary.TotalHPAs++

		entry := HPACooldownEntry{
			Name:      hpa.Name,
			Namespace: hpa.Namespace,
			Target:    hpa.Spec.ScaleTargetRef.Name,
		}

		minR := int32(1)
		if hpa.Spec.MinReplicas != nil {
			minR = *hpa.Spec.MinReplicas
		}
		entry.MinReplicas = minR
		entry.MaxReplicas = hpa.Spec.MaxReplicas
		entry.MetricCount = len(hpa.Spec.Metrics)

		var issues []string

		// Check min replicas
		if minR == 0 {
			result.Summary.NoMinReplicas++
			issues = append(issues, "minReplicas=0, can scale to zero")
		}
		if minR > 10 {
			result.Summary.HighMinReplicas++
			issues = append(issues, fmt.Sprintf("high minReplicas=%d", minR))
		}

		// Check behavior
		if hpa.Spec.Behavior != nil {
			entry.HasBehavior = true
			result.Summary.WithBehavior++

			// Check upscale stabilization
			if hpa.Spec.Behavior.ScaleUp.StabilizationWindowSeconds != nil {
				upscaleWindow := *hpa.Spec.Behavior.ScaleUp.StabilizationWindowSeconds
				if upscaleWindow == 0 {
					entry.UpscaleAggressive = true
					result.Summary.AggressiveUpscale++
					issues = append(issues, "no upscale stabilization")
				}
			} else {
				entry.UpscaleAggressive = true
				result.Summary.AggressiveUpscale++
			}

			// Check downscale stabilization
			if hpa.Spec.Behavior.ScaleDown.StabilizationWindowSeconds != nil {
				downWindow := *hpa.Spec.Behavior.ScaleDown.StabilizationWindowSeconds
				if downWindow < 60 {
					entry.DownscaleAggressive = true
					result.Summary.AggressiveDownscale++
					issues = append(issues, fmt.Sprintf("downscale stabilization %ds < 60s", downWindow))
				} else {
					result.Summary.WithCooldown++
				}
			} else {
				// Default is 300s (5 min) which is ok
				result.Summary.WithCooldown++
			}
		} else {
			issues = append(issues, "no scaling behavior configured")
		}

		// Check metrics
		if entry.MetricCount == 0 {
			issues = append(issues, "no metrics defined")
		}

		entry.Issues = issues
		switch {
		case len(issues) >= 3:
			entry.RiskLevel = "high"
		case len(issues) >= 1:
			entry.RiskLevel = "medium"
		default:
			entry.RiskLevel = "low"
		}

		if entry.RiskLevel != "low" {
			result.RiskyHPAs = append(result.RiskyHPAs, entry)
		}
		result.ByHPA = append(result.ByHPA, entry)
	}

	sort.Slice(result.ByHPA, func(i, j int) bool {
		rank := map[string]int{"high": 0, "medium": 1, "low": 2}
		return rank[result.ByHPA[i].RiskLevel] < rank[result.ByHPA[j].RiskLevel]
	})

	if result.Summary.TotalHPAs > 0 {
		configured := result.Summary.WithBehavior + result.Summary.WithCooldown
		result.HealthScore = configured * 100 / result.Summary.TotalHPAs
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("HPA 冷却审计: %d HPA, %d 有行为配置, %d 有冷却, %d 激进扩容, %d 激进缩容",
			result.Summary.TotalHPAs, result.Summary.WithBehavior,
			result.Summary.WithCooldown, result.Summary.AggressiveUpscale,
			result.Summary.AggressiveDownscale),
	}
	if result.Summary.AggressiveDownscale > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个 HPA 缩容过于激进, 可能导致抖动", result.Summary.AggressiveDownscale))
	}
	writeJSON(w, result)
}

var _ autoscalingv2.HorizontalPodAutoscalerSpec
var _ corev1.ResourceList
