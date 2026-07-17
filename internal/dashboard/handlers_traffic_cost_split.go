package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TrafficCostSplitResult splits cluster traffic cost by Service and Ingress,
// attributing compute costs to API endpoints for FinOps visibility.
type TrafficCostSplitResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         TrafficCostSummary `json:"summary"`
	ByService       []TrafficCostEntry `json:"byService"`
	TopCostPaths    []TrafficCostEntry `json:"topCostPaths"`
	CostScore       int                `json:"costScore"`
	Grade           string             `json:"grade"`
	Recommendations []string           `json:"recommendations"`
}

type TrafficCostSummary struct {
	TotalServices   int     `json:"totalServices"`
	TotalIngresses  int     `json:"totalIngresses"`
	TotalPodCost    float64 `json:"totalPodCostUSD"`
	AttributedCost  float64 `json:"attributedCostUSD"`
	UnattributedPct float64 `json:"unattributedPct"`
	TopServiceShare float64 `json:"topServiceSharePct"`
}

type TrafficCostEntry struct {
	ServiceName    string  `json:"serviceName"`
	Namespace      string  `json:"namespace"`
	ServiceType    string  `json:"serviceType"`
	HasIngress     bool    `json:"hasIngress"`
	IngressHost    string  `json:"ingressHost"`
	BackingPods    int     `json:"backingPods"`
	CPUCores       float64 `json:"cpuCores"`
	MemoryGB       float64 `json:"memoryGB"`
	MonthlyCostUSD float64 `json:"monthlyCostUSD"`
	TrafficShare   float64 `json:"trafficSharePct"`
	CostTier       string  `json:"costTier"`
}

// handleTrafficCostSplit handles GET /api/product/traffic-cost-split
func (s *Server) handleTrafficCostSplit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := TrafficCostSplitResult{ScannedAt: time.Now()}

	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Try to list ingresses
	ingressMap := make(map[string]string) // ns/svcName -> host
	ingresses, err := rc.clientset.NetworkingV1().Ingresses("").List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, ing := range ingresses.Items {
			for _, rule := range ing.Spec.Rules {
				host := rule.Host
				if host == "" {
					host = "*"
				}
				for _, path := range rule.HTTP.Paths {
					key := ing.Namespace + "/" + path.Backend.Service.Name
					if existing, ok := ingressMap[key]; ok {
						ingressMap[key] = existing + "," + host
					} else {
						ingressMap[key] = host
					}
					result.Summary.TotalIngresses++
				}
			}
		}
	}

	// Build pod resource map by labels
	type podResource struct {
		cpu float64
		mem float64
	}
	podResList := []struct {
		namespace string
		labels    map[string]string
		cpu       float64
		mem       float64
	}{}

	totalPodCPU := 0.0
	totalPodMem := 0.0
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		cpu := 0.0
		mem := 0.0
		for _, c := range pod.Spec.Containers {
			if req, ok := c.Resources.Requests[corev1.ResourceCPU]; ok {
				cpu += req.AsApproximateFloat64()
			}
			if req, ok := c.Resources.Requests[corev1.ResourceMemory]; ok {
				mem += float64(req.ScaledValue(resource.Mega)) / 1024.0
			}
		}
		if cpu > 0 || mem > 0 {
			podResList = append(podResList, struct {
				namespace string
				labels    map[string]string
				cpu       float64
				mem       float64
			}{pod.Namespace, pod.Labels, cpu, mem})
			totalPodCPU += cpu
			totalPodMem += mem
		}
	}

	totalCost := totalPodCPU*costPerCPUCoreHour*hoursPerMonth + totalPodMem*costPerGBHour*hoursPerMonth
	result.Summary.TotalPodCost = totalCost

	// Attribute costs to services
	var entries []TrafficCostEntry
	attributedCost := 0.0

	for _, svc := range services.Items {
		if isSystemNamespace(svc.Namespace) {
			continue
		}
		result.Summary.TotalServices++

		entry := TrafficCostEntry{
			ServiceName: svc.Name,
			Namespace:   svc.Namespace,
			ServiceType: string(svc.Spec.Type),
		}

		// Check ingress
		if host, ok := ingressMap[svc.Namespace+"/"+svc.Name]; ok {
			entry.HasIngress = true
			entry.IngressHost = host
		}

		// Find backing pods by selector
		if svc.Spec.Selector != nil && len(svc.Spec.Selector) > 0 {
			for _, pr := range podResList {
				if pr.namespace != svc.Namespace {
					continue
				}
				match := true
				for k, v := range svc.Spec.Selector {
					if pr.labels[k] != v {
						match = false
						break
					}
				}
				if match {
					entry.BackingPods++
					entry.CPUCores += pr.cpu
					entry.MemoryGB += pr.mem
				}
			}
		}

		// Calculate cost
		entry.MonthlyCostUSD = entry.CPUCores*costPerCPUCoreHour*hoursPerMonth + entry.MemoryGB*costPerGBHour*hoursPerMonth
		attributedCost += entry.MonthlyCostUSD

		// Cost tier
		switch {
		case entry.MonthlyCostUSD > 100:
			entry.CostTier = "expensive"
		case entry.MonthlyCostUSD > 30:
			entry.CostTier = "moderate"
		case entry.MonthlyCostUSD > 0:
			entry.CostTier = "low"
		default:
			entry.CostTier = "free"
		}

		entries = append(entries, entry)
	}

	result.Summary.AttributedCost = attributedCost
	if totalCost > 0 {
		result.Summary.UnattributedPct = (1 - attributedCost/totalCost) * 100
	}

	// Sort by cost descending
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].MonthlyCostUSD > entries[j].MonthlyCostUSD
	})
	result.ByService = entries

	// Calculate traffic share (approximated by cost share)
	if attributedCost > 0 {
		for i := range entries {
			entries[i].TrafficShare = entries[i].MonthlyCostUSD / attributedCost * 100
		}
	}

	// Top 5 cost paths
	topN := 5
	if len(entries) < topN {
		topN = len(entries)
	}
	result.TopCostPaths = entries[:topN]
	if topN > 0 && attributedCost > 0 {
		topSum := 0.0
		for i := 0; i < topN; i++ {
			topSum += entries[i].MonthlyCostUSD
		}
		result.Summary.TopServiceShare = topSum / attributedCost * 100
	}

	// Cost score: higher attribution ratio = better
	result.CostScore = int(100 - result.Summary.UnattributedPct)
	if result.CostScore < 0 {
		result.CostScore = 0
	}

	switch {
	case result.CostScore >= 80:
		result.Grade = "A"
	case result.CostScore >= 60:
		result.Grade = "B"
	case result.CostScore >= 40:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	result.Recommendations = buildTrafficCostRecs(&result)
	writeJSON(w, result)
}

func buildTrafficCostRecs(r *TrafficCostSplitResult) []string {
	recs := []string{
		fmt.Sprintf("流量成本拆分: %d Service, %d Ingress, 总成本 $%.2f/月", r.Summary.TotalServices, r.Summary.TotalIngresses, r.Summary.TotalPodCost),
	}
	if r.Summary.UnattributedPct > 50 {
		recs = append(recs, fmt.Sprintf("警告: %.1f%% 成本无法归属到 Service (缺少 selector)", r.Summary.UnattributedPct))
	}
	if r.Summary.TopServiceShare > 60 {
		recs = append(recs, fmt.Sprintf("成本集中: Top5 Service 占 %.1f%% 已归属成本", r.Summary.TopServiceShare))
	}
	if len(r.TopCostPaths) > 0 {
		top := r.TopCostPaths[0]
		recs = append(recs, fmt.Sprintf("最高成本路径: %s/%s ($%.2f/月, %d Pod)", top.Namespace, top.ServiceName, top.MonthlyCostUSD, top.BackingPods))
	}
	return recs
}
