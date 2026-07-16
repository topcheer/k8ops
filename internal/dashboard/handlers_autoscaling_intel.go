package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AutoscalingIntelResult is the autoscaling intelligence & scaling behavior profiler.
// It analyzes HPA/VPA coverage, scaling gap detection, scale-up/down latency estimation,
// and provides actionable autoscaling tuning recommendations.
type AutoscalingIntelResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         AutoscalingSummary  `json:"summary"`
	ScalingProfiles []ScalingProfile    `json:"scalingProfiles"`
	ScalingGaps     []ScalingGap        `json:"scalingGaps"`
	ByNamespace     []NSAutoscaling     `json:"byNamespace"`
	TuningAdvice    []TuningAdvice      `json:"tuningAdvice"`
	Recommendations []string            `json:"recommendations"`
}

// AutoscalingSummary aggregates autoscaling statistics.
type AutoscalingSummary struct {
	TotalWorkloads   int     `json:"totalWorkloads"`
	WithHPA          int     `json:"withHPA"`
	WithoutHPA       int     `json:"withoutHPA"`
	HPACoverage      float64 `json:"hpaCoverage"` // % with HPA
	WithVPA          int     `json:"withVPA"`
	WithoutResources int     `json:"withoutResources"` // workloads missing requests
	SingleReplicaHPA int     `json:"singleReplicaHPA"` // HPA with minReplicas=1 (no HA)
	OverScaled       int     `json:"overScaled"`       // more replicas than needed
	UnderScaled      int     `json:"underScaled"`      // not enough replicas
	HPAWithMetrics   int     `json:"hpaWithMetrics"`   // HPA with resource metrics
	HPAWithCustom    int     `json:"hpaWithCustom"`    // HPA with custom metrics
	IntelScore       int     `json:"intelScore"`       // 0-100
}

// ScalingProfile profiles one workload's autoscaling posture.
type ScalingProfile struct {
	Name           string  `json:"name"`
	Namespace      string  `json:"namespace"`
	Kind           string  `json:"kind"`
	HasHPA         bool    `json:"hasHPA"`
	HasResources   bool    `json:"hasResources"`
	CurrentReplicas int32  `json:"currentReplicas"`
	MinReplicas    int32   `json:"minReplicas,omitempty"`
	MaxReplicas    int32   `json:"maxReplicas,omitempty"`
	TargetCPU      int32   `json:"targetCPUUtilization,omitempty"` // %
	TargetMemory   int32   `json:"targetMemoryUtilization,omitempty"`
	ScalingBehavior string  `json:"scalingBehavior"`               // aggressive, moderate, conservative
	HeadroomPct    float64 `json:"headroomPct"`                    // available scaling headroom
	ScaleUpTime    string  `json:"scaleUpTimeEstimate"`            // estimated time to scale up
	Verdict        string  `json:"verdict"`                        // optimal, over-provisioned, under-provisioned, no-autoscaling, misconfigured
	RiskScore      int     `json:"riskScore"`
}

// ScalingGap identifies workloads that should have autoscaling but don't.
type ScalingGap struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Kind        string `json:"kind"`
	Reason      string `json:"reason"`
	Impact      string `json:"impact"`
	Severity    string `json:"severity"`
	Suggestion  string `json:"suggestion"`
}

// NSAutoscaling shows per-namespace autoscaling stats.
type NSAutoscaling struct {
	Namespace    string  `json:"namespace"`
	TotalWorkloads int   `json:"totalWorkloads"`
	WithHPA      int     `json:"withHPA"`
	Coverage     float64 `json:"coveragePct"`
	Gaps         int     `json:"gaps"`
}

// TuningAdvice provides specific HPA tuning recommendations.
type TuningAdvice struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Issue     string `json:"issue"`
	Advice    string `json:"advice"`
	Priority  string `json:"priority"` // high, medium, low
}

// handleAutoscalingIntel provides autoscaling intelligence and scaling behavior profiling.
// GET /api/scalability/autoscaling-intel
func (s *Server) handleAutoscalingIntel(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := AutoscalingIntelResult{ScannedAt: time.Now()}

	systemNS := map[string]bool{
		"kube-system": true, "kube-public": true, "kube-node-lease": true,
	}

	// Collect deployments
	deploys, err := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list deployments")
		return
	}

	// Collect HPAs
	hpas, _ := rc.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(ctx, metav1.ListOptions{})

	// Build HPA lookup by target
	hpaMap := make(map[string]*autoscalingv2.HorizontalPodAutoscaler)
	for i := range hpas.Items {
		hpa := &hpas.Items[i]
		key := hpa.Namespace + "/" + hpa.Spec.ScaleTargetRef.Name
		hpaMap[key] = hpa
	}

	// Namespace stats
	type nsASData struct {
		total   int
		withHPA int
		gaps    int
	}
	nsStats := make(map[string]*nsASData)

	for _, dep := range deploys.Items {
		if systemNS[dep.Namespace] {
			continue
		}
		result.Summary.TotalWorkloads++

		nsd, ok := nsStats[dep.Namespace]
		if !ok {
			nsd = &nsASData{}
			nsStats[dep.Namespace] = nsd
		}
		nsd.total++

		key := dep.Namespace + "/" + dep.Name
		hpa := hpaMap[key]

		// Check resource requests
		hasResources := false
		for _, c := range dep.Spec.Template.Spec.Containers {
			if c.Resources.Requests != nil {
				if cpu, ok := c.Resources.Requests[corev1.ResourceCPU]; ok && !cpu.IsZero() {
					hasResources = true
				}
			}
		}

		profile := ScalingProfile{
			Name:            dep.Name,
			Namespace:       dep.Namespace,
			Kind:            "Deployment",
			HasHPA:          hpa != nil,
			HasResources:    hasResources,
			CurrentReplicas: dep.Status.Replicas,
		}

		// Track stats
		if hpa != nil {
			result.Summary.WithHPA++
			nsd.withHPA++

			if hpa.Spec.MinReplicas != nil {
				profile.MinReplicas = *hpa.Spec.MinReplicas
				if *hpa.Spec.MinReplicas == 1 {
					result.Summary.SingleReplicaHPA++
				}
			}
			profile.MaxReplicas = hpa.Spec.MaxReplicas

			// Extract target metrics
			for _, metric := range hpa.Spec.Metrics {
				if metric.Type == autoscalingv2.ResourceMetricSourceType {
					if metric.Resource.Name == corev1.ResourceCPU {
						result.Summary.HPAWithMetrics++
						if metric.Resource.Target.AverageUtilization != nil {
							profile.TargetCPU = *metric.Resource.Target.AverageUtilization
						}
					}
					if metric.Resource.Name == corev1.ResourceMemory {
						if metric.Resource.Target.AverageUtilization != nil {
							profile.TargetMemory = *metric.Resource.Target.AverageUtilization
						}
					}
				}
				if metric.Type == autoscalingv2.PodsMetricSourceType || metric.Type == autoscalingv2.ExternalMetricSourceType {
					result.Summary.HPAWithCustom++
				}
			}

			// Compute headroom
			if profile.MaxReplicas > 0 && profile.CurrentReplicas > 0 {
				profile.HeadroomPct = float64(profile.MaxReplicas-profile.CurrentReplicas) / float64(profile.MaxReplicas) * 100
			}

			// Estimate scale-up time
			behavior := "moderate"
			scaleUpTime := "2-5 min"
			if hpa.Spec.Behavior != nil && hpa.Spec.Behavior.ScaleUp != nil {
				if hpa.Spec.Behavior.ScaleUp.Policies != nil {
					for _, p := range hpa.Spec.Behavior.ScaleUp.Policies {
						if p.Type == autoscalingv2.PercentScalingPolicy && p.Value > 50 {
							behavior = "aggressive"
							scaleUpTime = "< 1 min"
						}
						if p.Type == autoscalingv2.PodsScalingPolicy && p.Value <= 1 {
							behavior = "conservative"
							scaleUpTime = "5-10 min"
						}
					}
				}
			}
			profile.ScalingBehavior = behavior
			profile.ScaleUpTime = scaleUpTime

			// Verdict
			if !hasResources {
				profile.Verdict = "misconfigured"
				profile.RiskScore = 70
				result.ScalingGaps = append(result.ScalingGaps, ScalingGap{
					Name: dep.Name, Namespace: dep.Namespace, Kind: "Deployment",
					Reason: "HPA exists but no CPU requests set — HPA cannot calculate utilization",
					Impact: "Autoscaling will not work properly",
					Severity: "critical",
					Suggestion: "Add CPU resource requests to enable HPA resource metrics",
				})
			} else if profile.TargetCPU == 0 && profile.TargetMemory == 0 && result.Summary.HPAWithCustom == 0 {
				profile.Verdict = "misconfigured"
				profile.RiskScore = 60
			} else if profile.HeadroomPct < 20 && profile.CurrentReplicas > 1 {
				profile.Verdict = "under-provisioned"
				profile.RiskScore = 40
				result.Summary.UnderScaled++
			} else if profile.HeadroomPct > 80 && profile.CurrentReplicas > 2 {
				profile.Verdict = "over-provisioned"
				profile.RiskScore = 30
				result.Summary.OverScaled++
			} else {
				profile.Verdict = "optimal"
				profile.RiskScore = 10
			}

			// Tuning advice
			if profile.TargetCPU > 0 && (profile.TargetCPU < 30 || profile.TargetCPU > 90) {
				priority := "medium"
				if profile.TargetCPU > 90 {
					priority = "high"
				}
				result.TuningAdvice = append(result.TuningAdvice, TuningAdvice{
					Name: dep.Name, Namespace: dep.Namespace,
					Issue: fmt.Sprintf("HPA target CPU utilization is %d%% (recommended: 50-80%%)", profile.TargetCPU),
					Advice: "Adjust target CPU utilization to 50-80%% for balanced scaling responsiveness",
					Priority: priority,
				})
			}
			if profile.MaxReplicas > 0 && profile.MaxReplicas == profile.MinReplicas {
				result.TuningAdvice = append(result.TuningAdvice, TuningAdvice{
					Name: dep.Name, Namespace: dep.Namespace,
					Issue: fmt.Sprintf("HPA minReplicas == maxReplicas (%d) — no scaling possible", profile.MaxReplicas),
					Advice: fmt.Sprintf("Increase maxReplicas above %d to enable scaling", profile.MaxReplicas),
					Priority: "high",
				})
			}
		} else {
			result.Summary.WithoutHPA++
			nsd.gaps++

			if !hasResources {
				result.Summary.WithoutResources++
			}

			// Check if this workload would benefit from HPA
			replicas := int32(0)
			if dep.Spec.Replicas != nil {
				replicas = *dep.Spec.Replicas
			}
			profile.CurrentReplicas = replicas

			if replicas >= 2 && hasResources {
				profile.Verdict = "no-autoscaling"
				profile.RiskScore = 50
				severity := "high"
				if replicas >= 3 {
					severity = "critical"
				}
				result.ScalingGaps = append(result.ScalingGaps, ScalingGap{
					Name: dep.Name, Namespace: dep.Namespace, Kind: "Deployment",
					Reason: "Multi-replica deployment without HPA — cannot auto-scale with demand",
					Impact: fmt.Sprintf("Traffic spikes will cause degradation (current replicas: %d)", replicas),
					Severity: severity,
					Suggestion: fmt.Sprintf("Add HPA: kubectl autoscale deployment %s --cpu-percent=70 --min=%d --max=%d", dep.Name, replicas, replicas*3),
				})
			} else if replicas == 1 {
				profile.Verdict = "single-replica"
				profile.RiskScore = 65
			} else {
				profile.Verdict = "no-autoscaling"
				profile.RiskScore = 35
			}
		}

		result.ScalingProfiles = append(result.ScalingProfiles, profile)
	}

	// Sort profiles by risk score descending
	sort.Slice(result.ScalingProfiles, func(i, j int) bool {
		return result.ScalingProfiles[i].RiskScore > result.ScalingProfiles[j].RiskScore
	})

	// HPA coverage
	if result.Summary.TotalWorkloads > 0 {
		result.Summary.HPACoverage = float64(result.Summary.WithHPA) / float64(result.Summary.TotalWorkloads) * 100
	}

	// Intel score
	score := 100
	if result.Summary.TotalWorkloads > 0 {
		noHPARate := float64(result.Summary.WithoutHPA) / float64(result.Summary.TotalWorkloads)
		score -= int(noHPARate * 40)
	}
	score -= len(result.ScalingGaps) * 3
	if result.Summary.WithoutResources > 0 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.Summary.IntelScore = score

	// By namespace
	for ns, nsd := range nsStats {
		coverage := 0.0
		if nsd.total > 0 {
			coverage = float64(nsd.withHPA) / float64(nsd.total) * 100
		}
		result.ByNamespace = append(result.ByNamespace, NSAutoscaling{
			Namespace: ns, TotalWorkloads: nsd.total,
			WithHPA: nsd.withHPA, Coverage: coverage, Gaps: nsd.gaps,
		})
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].Gaps > result.ByNamespace[j].Gaps
	})

	// Recommendations
	result.Recommendations = generateAutoscalingRecs(result)

	writeJSON(w, result)
}

// generateAutoscalingRecs produces actionable recommendations.
func generateAutoscalingRecs(result AutoscalingIntelResult) []string {
	var recs []string

	if result.Summary.HPACoverage < 30 {
		recs = append(recs, fmt.Sprintf("HPA coverage is only %.0f%% — most workloads cannot auto-scale with demand", result.Summary.HPACoverage))
	}

	if result.Summary.WithoutResources > 0 {
		recs = append(recs, fmt.Sprintf("%d workloads lack CPU resource requests — HPA resource metrics cannot function without them", result.Summary.WithoutResources))
	}

	if result.Summary.SingleReplicaHPA > 0 {
		recs = append(recs, fmt.Sprintf("%d HPAs have minReplicas=1 — increase to at least 2 for high availability", result.Summary.SingleReplicaHPA))
	}

	critGaps := 0
	for _, gap := range result.ScalingGaps {
		if gap.Severity == "critical" {
			critGaps++
		}
	}
	if critGaps > 0 {
		recs = append(recs, fmt.Sprintf("%d critical autoscaling gaps detected — add HPA with resource requests immediately", critGaps))
	}

	if len(result.TuningAdvice) > 0 {
		highPrio := 0
		for _, ta := range result.TuningAdvice {
			if ta.Priority == "high" {
				highPrio++
			}
		}
		if highPrio > 0 {
			recs = append(recs, fmt.Sprintf("%d high-priority HPA tuning issues — fix target utilization and min/max replica configuration", highPrio))
		}
	}

	if result.Summary.IntelScore < 50 {
		recs = append(recs, fmt.Sprintf("Autoscaling intelligence score is %d/100 — systematic HPA adoption needed", result.Summary.IntelScore))
	}

	if len(recs) == 0 {
		recs = append(recs, "Autoscaling posture is healthy — maintain current HPA configuration and monitoring")
	}

	return recs
}

// Suppress unused import warnings
var _ = appsv1.Deployment{}
var _ = resource.Quantity{}
