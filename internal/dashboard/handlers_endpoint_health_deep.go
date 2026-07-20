package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EndpointHealthDeepResult provides deep health analysis of service endpoints.
type EndpointHealthDeepResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	Summary         EndpointHealthSummary `json:"summary"`
	ByService       []EndpointHealthEntry `json:"byService"`
	UnhealthySvc    []EndpointHealthEntry `json:"unhealthyServices"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Recommendations []string              `json:"recommendations"`
}

type EndpointHealthSummary struct {
	TotalServices  int `json:"totalServices"`
	HealthySvcs    int `json:"healthyServices"`
	DegradedSvcs   int `json:"degradedServices"`
	CriticalSvcs   int `json:"criticalServices"`
	TotalEndpoints int `json:"totalEndpoints"`
	ReadyEndpoints int `json:"readyEndpoints"`
}

type EndpointHealthEntry struct {
	ServiceName  string  `json:"serviceName"`
	Namespace    string  `json:"namespace"`
	ServiceType  string  `json:"serviceType"`
	BackingPods  int     `json:"backingPods"`
	ReadyPods    int     `json:"readyPods"`
	ReadyRatio   float64 `json:"readyRatio"`
	HasEndpoints bool    `json:"hasEndpoints"`
	HealthStatus string  `json:"healthStatus"`
	RiskLevel    string  `json:"riskLevel"`
}

// handleEndpointHealthDeep handles GET /api/product/endpoint-health-deep
func (s *Server) handleEndpointHealthDeep(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := EndpointHealthDeepResult{ScannedAt: time.Now()}
	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	for _, svc := range services.Items {
		if isSystemNamespace(svc.Namespace) {
			continue
		}
		if svc.Spec.ClusterIP == "None" && len(svc.Spec.ClusterIPs) == 0 {
			continue
		}
		result.Summary.TotalServices++

		entry := EndpointHealthEntry{ServiceName: svc.Name, Namespace: svc.Namespace, ServiceType: string(svc.Spec.Type)}

		if svc.Spec.Selector != nil && len(svc.Spec.Selector) > 0 {
			for _, pod := range pods.Items {
				if pod.Namespace != svc.Namespace || pod.Status.Phase != corev1.PodRunning {
					continue
				}
				match := true
				for k, v := range svc.Spec.Selector {
					if pod.Labels[k] != v {
						match = false
						break
					}
				}
				if match {
					entry.BackingPods++
					ready := true
					for _, cs := range pod.Status.ContainerStatuses {
						if !cs.Ready {
							ready = false
							break
						}
					}
					if ready {
						entry.ReadyPods++
					}
				}
			}
		}

		result.Summary.TotalEndpoints += entry.BackingPods
		result.Summary.ReadyEndpoints += entry.ReadyPods

		if entry.BackingPods > 0 {
			entry.HasEndpoints = true
			entry.ReadyRatio = float64(entry.ReadyPods) / float64(entry.BackingPods)
		}

		switch {
		case entry.BackingPods == 0:
			entry.HealthStatus = "no-endpoints"
			entry.RiskLevel = "critical"
			result.Summary.CriticalSvcs++
		case entry.ReadyRatio < 0.5:
			entry.HealthStatus = "critical"
			entry.RiskLevel = "critical"
			result.Summary.CriticalSvcs++
		case entry.ReadyRatio < 1.0:
			entry.HealthStatus = "degraded"
			entry.RiskLevel = "medium"
			result.Summary.DegradedSvcs++
		default:
			entry.HealthStatus = "healthy"
			entry.RiskLevel = "low"
			result.Summary.HealthySvcs++
		}

		if entry.RiskLevel == "critical" || entry.RiskLevel == "medium" {
			result.UnhealthySvc = append(result.UnhealthySvc, entry)
		}
		result.ByService = append(result.ByService, entry)
	}

	sort.Slice(result.ByService, func(i, j int) bool {
		return result.ByService[i].ReadyRatio < result.ByService[j].ReadyRatio
	})

	if result.Summary.TotalServices > 0 {
		result.HealthScore = result.Summary.HealthySvcs * 100 / result.Summary.TotalServices
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("端点健康深度: %d 服务, %d 健康, %d 降级, %d 严重", result.Summary.TotalServices, result.Summary.HealthySvcs, result.Summary.DegradedSvcs, result.Summary.CriticalSvcs),
		fmt.Sprintf("端点: %d 总计, %d 就绪 (%.1f%%)", result.Summary.TotalEndpoints, result.Summary.ReadyEndpoints, func() float64 {
			if result.Summary.TotalEndpoints > 0 {
				return float64(result.Summary.ReadyEndpoints) / float64(result.Summary.TotalEndpoints) * 100
			}
			return 0
		}()),
	}
	if len(result.UnhealthySvc) > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个服务不健康", len(result.UnhealthySvc)))
	}
	if result.HealthScore < 60 {
		result.Recommendations = append(result.Recommendations, "建议: 检查 Pod readiness probe, 排查容器启动失败")
	}
	writeJSON(w, result)
}
