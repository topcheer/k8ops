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

// TrafficSpikeGuardResult monitors for anomalous traffic patterns by analyzing
// pod distribution, service endpoint counts, and connection indicators.
type TrafficSpikeGuardResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         TrafficSpikeSummary `json:"summary"`
	ByService       []TrafficSpikeEntry `json:"byService"`
	HotServices     []TrafficSpikeEntry `json:"hotServices"`
	GuardScore      int                 `json:"guardScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

type TrafficSpikeSummary struct {
	TotalServices   int     `json:"totalServices"`
	TotalEndpoints  int     `json:"totalEndpoints"`
	AvgEndpoints    float64 `json:"avgEndpointsPerService"`
	SingleEndpoint  int     `json:"singleEndpointServices"`
	HighFanout      int     `json:"highFanoutServices"`
	HotspotServices int     `json:"hotspotServices"`
	TotalNodePorts  int     `json:"totalNodePorts"`
	TotalLBs        int     `json:"totalLoadBalancers"`
}

type TrafficSpikeEntry struct {
	ServiceName   string   `json:"serviceName"`
	Namespace     string   `json:"namespace"`
	ServiceType   string   `json:"serviceType"`
	EndpointCount int      `json:"endpointCount"`
	PortCount     int      `json:"portCount"`
	HasNodePort   bool     `json:"hasNodePort"`
	IsExternal    bool     `json:"isExternal"`
	RiskLevel     string   `json:"riskLevel"`
	RiskFactors   []string `json:"riskFactors"`
}

// handleTrafficSpikeGuard handles GET /api/product/traffic-spike-guard
func (s *Server) handleTrafficSpikeGuard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := TrafficSpikeGuardResult{ScannedAt: time.Now()}
	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Build pod-per-namespace counts for traffic estimation
	nsPodCount := make(map[string]int)
	for _, pod := range pods.Items {
		if !isSystemNamespace(pod.Namespace) && pod.Status.Phase == corev1.PodRunning {
			nsPodCount[pod.Namespace]++
		}
	}

	totalEndpoints := 0
	for _, svc := range services.Items {
		if isSystemNamespace(svc.Namespace) {
			continue
		}
		result.Summary.TotalServices++

		entry := TrafficSpikeEntry{
			ServiceName: svc.Name,
			Namespace:   svc.Namespace,
			ServiceType: string(svc.Spec.Type),
			PortCount:   len(svc.Spec.Ports),
		}

		// Count backing pods
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
					entry.EndpointCount++
				}
			}
		}

		totalEndpoints += entry.EndpointCount

		// Check for NodePort / LoadBalancer
		if svc.Spec.Type == corev1.ServiceTypeNodePort {
			entry.HasNodePort = true
			entry.IsExternal = true
			result.Summary.TotalNodePorts++
		}
		if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
			entry.IsExternal = true
			result.Summary.TotalLBs++
		}

		// Risk factors
		var risks []string
		if entry.EndpointCount == 0 {
			risks = append(risks, "no-endpoints")
			entry.RiskLevel = "high"
		} else if entry.EndpointCount == 1 {
			result.Summary.SingleEndpoint++
			risks = append(risks, "single-endpoint")
			entry.RiskLevel = "medium"
		}
		if entry.EndpointCount > 20 {
			result.Summary.HighFanout++
			risks = append(risks, fmt.Sprintf("high-fanout(%d)", entry.EndpointCount))
			if entry.RiskLevel == "" {
				entry.RiskLevel = "medium"
			}
		}
		if entry.HasNodePort {
			risks = append(risks, "nodeport-exposed")
		}
		if entry.RiskLevel == "" {
			entry.RiskLevel = "low"
		}
		entry.RiskFactors = risks

		if entry.RiskLevel == "medium" || entry.RiskLevel == "high" {
			result.Summary.HotspotServices++
			result.HotServices = append(result.HotServices, entry)
		}

		result.ByService = append(result.ByService, entry)
	}

	result.Summary.TotalEndpoints = totalEndpoints
	if result.Summary.TotalServices > 0 {
		result.Summary.AvgEndpoints = float64(totalEndpoints) / float64(result.Summary.TotalServices)
	}

	sort.Slice(result.ByService, func(i, j int) bool {
		return result.ByService[i].EndpointCount > result.ByService[j].EndpointCount
	})
	sort.Slice(result.HotServices, func(i, j int) bool {
		rank := map[string]int{"high": 0, "medium": 1, "low": 2}
		return rank[result.HotServices[i].RiskLevel] < rank[result.HotServices[j].RiskLevel]
	})

	// Score: lower hotspots = higher score
	if result.Summary.TotalServices > 0 {
		hotRatio := float64(result.Summary.HotspotServices) / float64(result.Summary.TotalServices)
		result.GuardScore = int((1 - hotRatio) * 100)
	}
	gradeFromScore(&result.Grade, result.GuardScore)

	result.Recommendations = []string{
		fmt.Sprintf("流量守护: %d 服务, %d 端点, %.1f 平均, %d 单端点, %d 高扇出", result.Summary.TotalServices, totalEndpoints, result.Summary.AvgEndpoints, result.Summary.SingleEndpoint, result.Summary.HighFanout),
		fmt.Sprintf("外部暴露: %d NodePort, %d LoadBalancer", result.Summary.TotalNodePorts, result.Summary.TotalLBs),
	}
	if result.Summary.SingleEndpoint > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个单端点服务存在单点故障风险", result.Summary.SingleEndpoint))
	}
	if result.Summary.TotalNodePorts > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个 NodePort 暴露在集群外", result.Summary.TotalNodePorts))
	}
	if result.GuardScore < 60 {
		result.Recommendations = append(result.Recommendations, "建议: 增加副本数减少单端点, 使用 Ingress 替代 NodePort")
	}

	_ = strings.Contains // keep import
	writeJSON(w, result)
}
