package dashboard

import (
	"fmt"
	"net/http"
	"sort"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// HPAInfo represents an HPA with its scaling metrics and status.
type HPAInfo struct {
	Name            string      `json:"name"`
	Namespace       string      `json:"namespace"`
	TargetKind      string      `json:"targetKind"`
	TargetName      string      `json:"targetName"`
	MinReplicas     int32       `json:"minReplicas"`
	MaxReplicas     int32       `json:"maxReplicas"`
	CurrentReplicas int32       `json:"currentReplicas"`
	DesiredReplicas int32       `json:"desiredReplicas"`
	ScalingActive   bool        `json:"scalingActive"`
	Metrics         []HPAMetric `json:"metrics"`
	Age             string      `json:"age"`
}

// HPAMetric represents a single HPA scaling metric.
type HPAMetric struct {
	Type         string  `json:"type"`           // Resource, Pods, External, ContainerResource
	Name         string  `json:"name"`           // e.g. "cpu", "memory", or custom metric name
	TargetType   string  `json:"targetType"`     // Utilization, Value, AverageValue
	TargetValue  string  `json:"targetValue"`    // human-readable target
	CurrentValue string  `json:"currentValue"`   // human-readable current
	Utilization  float64 `json:"utilizationPct"` // percentage of target (0-100+)
}

// handleHPAList returns detailed HPA data with scaling metrics.
// GET /api/hpa
func (s *Server) handleHPAList(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil || s.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")
	if ns == "" {
		ns = ""
	}

	hpas, err := rc.clientset.AutoscalingV2().HorizontalPodAutoscalers(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	items := make([]HPAInfo, 0, len(hpas.Items))
	scalingActive := 0

	for _, h := range hpas.Items {
		minReps := int32(1)
		if h.Spec.MinReplicas != nil {
			minReps = *h.Spec.MinReplicas
		}

		info := HPAInfo{
			Name:            h.Name,
			Namespace:       h.Namespace,
			TargetKind:      h.Spec.ScaleTargetRef.Kind,
			TargetName:      h.Spec.ScaleTargetRef.Name,
			MinReplicas:     minReps,
			MaxReplicas:     h.Spec.MaxReplicas,
			CurrentReplicas: h.Status.CurrentReplicas,
			DesiredReplicas: h.Status.DesiredReplicas,
			ScalingActive:   h.Status.CurrentReplicas != h.Status.DesiredReplicas,
			Age:             ageTime(h.CreationTimestamp.Time),
			Metrics:         extractHPAMetrics(h),
		}

		if info.ScalingActive {
			scalingActive++
		}

		items = append(items, info)
	}

	// Sort by namespace then name
	sort.Slice(items, func(i, j int) bool {
		if items[i].Namespace != items[j].Namespace {
			return items[i].Namespace < items[j].Namespace
		}
		return items[i].Name < items[j].Name
	})

	// Summary
	totalCurrent, totalDesired := int32(0), int32(0)
	for _, i := range items {
		totalCurrent += i.CurrentReplicas
		totalDesired += i.DesiredReplicas
	}

	writeJSON(w, map[string]any{
		"count": len(items),
		"summary": map[string]any{
			"totalHPAs":       len(items),
			"scalingActive":   scalingActive,
			"currentReplicas": totalCurrent,
			"desiredReplicas": totalDesired,
		},
		"items": items,
	})
}

// extractHPAMetrics parses HPA spec and status into readable metric entries.
func extractHPAMetrics(h autoscalingv2.HorizontalPodAutoscaler) []HPAMetric {
	metrics := make([]HPAMetric, 0)

	// Map current metrics by descriptor for lookup
	currentMap := map[string]autoscalingv2.MetricStatus{}
	for _, cm := range h.Status.CurrentMetrics {
		key := metricStatusKey(cm)
		if key != "" {
			currentMap[key] = cm
		}
	}

	for _, spec := range h.Spec.Metrics {
		m := HPAMetric{Type: string(spec.Type)}

		switch spec.Type {
		case autoscalingv2.ResourceMetricSourceType:
			name := string(spec.Resource.Name)
			m.Name = name
			if spec.Resource.Target.Type == autoscalingv2.UtilizationMetricType && spec.Resource.Target.AverageUtilization != nil {
				targetUtil := *spec.Resource.Target.AverageUtilization
				m.TargetType = "Utilization"
				m.TargetValue = fmt.Sprintf("%d%%", targetUtil)
				// Find current
				key := "resource:" + name
				if cur, ok := currentMap[key]; ok && cur.Resource != nil {
					if cur.Resource.Current.AverageUtilization != nil {
						m.CurrentValue = fmt.Sprintf("%d%%", *cur.Resource.Current.AverageUtilization)
						if targetUtil > 0 {
							m.Utilization = float64(*cur.Resource.Current.AverageUtilization) /
								float64(targetUtil) * 100
						}
					}
				}
			} else if spec.Resource.Target.Type == autoscalingv2.AverageValueMetricType {
				m.TargetType = "AverageValue"
				m.TargetValue = spec.Resource.Target.AverageValue.String()
				key := "resource:" + name
				if cur, ok := currentMap[key]; ok && cur.Resource != nil {
					m.CurrentValue = cur.Resource.Current.AverageValue.String()
					if !spec.Resource.Target.AverageValue.IsZero() {
						ratio := cur.Resource.Current.AverageValue.AsApproximateFloat64() /
							spec.Resource.Target.AverageValue.AsApproximateFloat64()
						m.Utilization = ratio * 100
					}
				}
			}

		case autoscalingv2.PodsMetricSourceType:
			m.Name = spec.Pods.Metric.Name
			m.TargetType = string(spec.Pods.Target.Type)
			m.TargetValue = spec.Pods.Target.AverageValue.String()
			key := "pods:" + spec.Pods.Metric.Name
			if cur, ok := currentMap[key]; ok && cur.Pods != nil {
				m.CurrentValue = cur.Pods.Current.AverageValue.String()
				if !spec.Pods.Target.AverageValue.IsZero() {
					m.Utilization = cur.Pods.Current.AverageValue.AsApproximateFloat64() /
						spec.Pods.Target.AverageValue.AsApproximateFloat64() * 100
				}
			}

		case autoscalingv2.ExternalMetricSourceType:
			m.Name = spec.External.Metric.Name
			m.TargetType = string(spec.External.Target.Type)
			if spec.External.Target.Type == autoscalingv2.AverageValueMetricType {
				m.TargetValue = spec.External.Target.AverageValue.String()
			} else {
				m.TargetValue = spec.External.Target.Value.String()
			}
			key := "external:" + spec.External.Metric.Name
			if cur, ok := currentMap[key]; ok && cur.External != nil {
				if cur.External.Current.AverageValue != nil {
					m.CurrentValue = cur.External.Current.AverageValue.String()
				} else if cur.External.Current.Value != nil {
					m.CurrentValue = cur.External.Current.Value.String()
				}
			}

		case autoscalingv2.ContainerResourceMetricSourceType:
			name := string(spec.ContainerResource.Name)
			m.Name = name + " (container: " + spec.ContainerResource.Container + ")"
			if spec.ContainerResource.Target.Type == autoscalingv2.UtilizationMetricType && spec.ContainerResource.Target.AverageUtilization != nil {
				m.TargetType = "Utilization"
				m.TargetValue = fmt.Sprintf("%d%%", *spec.ContainerResource.Target.AverageUtilization)
			}
		}

		metrics = append(metrics, m)
	}

	return metrics
}

// metricStatusKey generates a lookup key for matching spec to status metrics.
func metricStatusKey(m autoscalingv2.MetricStatus) string {
	switch m.Type {
	case autoscalingv2.ResourceMetricSourceType:
		if m.Resource != nil {
			return "resource:" + string(m.Resource.Name)
		}
	case autoscalingv2.PodsMetricSourceType:
		if m.Pods != nil {
			return "pods:" + m.Pods.Metric.Name
		}
	case autoscalingv2.ExternalMetricSourceType:
		if m.External != nil {
			return "external:" + m.External.Metric.Name
		}
	case autoscalingv2.ContainerResourceMetricSourceType:
		if m.ContainerResource != nil {
			return "container:" + string(m.ContainerResource.Name)
		}
	}
	return ""
}
