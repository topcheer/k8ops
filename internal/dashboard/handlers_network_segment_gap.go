package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NetworkSegmentGapResult analyzes network segmentation gaps between namespaces.
type NetworkSegmentGapResult struct {
	ScannedAt        time.Time             `json:"scannedAt"`
	Summary          NetworkSegmentSummary `json:"summary"`
	ByNamespace      []NetworkSegmentEntry `json:"byNamespace"`
	UnsegmentedPairs []NetworkSegmentPair  `json:"unsegmentedPairs"`
	HealthScore      int                   `json:"healthScore"`
	Grade            string                `json:"grade"`
	Recommendations  []string              `json:"recommendations"`
}

type NetworkSegmentSummary struct {
	TotalNamespaces   int `json:"totalNamespaces"`
	WithNetPol        int `json:"namespacesWithNetPol"`
	WithoutNetPol     int `json:"namespacesWithoutNetPol"`
	DefaultDenyNS     int `json:"defaultDenyNamespaces"`
	IsolatedNS        int `json:"fullyIsolatedNamespaces"`
	EgressRestricted  int `json:"egressRestrictedNamespaces"`
	IngressRestricted int `json:"ingressRestrictedNamespaces"`
}

type NetworkSegmentEntry struct {
	Namespace      string `json:"namespace"`
	NetPolCount    int    `json:"netPolCount"`
	HasDefaultDeny bool   `json:"hasDefaultDenyIngress"`
	HasEgressDeny  bool   `json:"hasDefaultDenyEgress"`
	IsIsolated     bool   `json:"isFullyIsolated"`
	ExposureLevel  string `json:"exposureLevel"`
	RiskLevel      string `json:"riskLevel"`
}

type NetworkSegmentPair struct {
	Namespace1 string `json:"namespace1"`
	Namespace2 string `json:"namespace2"`
	Reason     string `json:"reason"`
}

// handleNetworkSegmentGap handles GET /api/security/network-segment-gap
func (s *Server) handleNetworkSegmentGap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := NetworkSegmentGapResult{ScannedAt: time.Now()}

	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	netpols, _ := rc.clientset.NetworkingV1().NetworkPolicies("").List(ctx, metav1.ListOptions{})

	// Build namespace network policy map
	nsNetPolMap := make(map[string][]networkingv1.NetworkPolicy)
	for _, np := range netpols.Items {
		nsNetPolMap[np.Namespace] = append(nsNetPolMap[np.Namespace], np)
	}

	var userNamespaces []string
	for _, ns := range namespaces.Items {
		if isSystemNamespace(ns.Name) {
			continue
		}
		result.Summary.TotalNamespaces++
		userNamespaces = append(userNamespaces, ns.Name)

		entry := NetworkSegmentEntry{Namespace: ns.Name}
		policies := nsNetPolMap[ns.Name]
		entry.NetPolCount = len(policies)

		if len(policies) > 0 {
			result.Summary.WithNetPol++
		}

		// Check for default deny ingress
		for _, np := range policies {
			if len(np.Spec.PodSelector.MatchLabels) == 0 && len(np.Spec.PodSelector.MatchExpressions) == 0 {
				// Applies to all pods in namespace
				for _, rule := range np.Spec.Ingress {
					if len(rule.From) == 0 {
						entry.HasDefaultDeny = true
						result.Summary.IngressRestricted++
					}
				}
				for _, rule := range np.Spec.Egress {
					if len(rule.To) == 0 {
						entry.HasEgressDeny = true
						result.Summary.EgressRestricted++
					}
				}
			}
		}

		if entry.HasDefaultDeny {
			result.Summary.DefaultDenyNS++
		}
		entry.IsIsolated = entry.HasDefaultDeny && entry.HasEgressDeny
		if entry.IsIsolated {
			result.Summary.IsolatedNS++
		}

		// Exposure assessment
		switch {
		case entry.NetPolCount == 0:
			entry.ExposureLevel = "fully-exposed"
			entry.RiskLevel = "critical"
			result.Summary.WithoutNetPol++
		case entry.IsIsolated:
			entry.ExposureLevel = "isolated"
			entry.RiskLevel = "low"
		case entry.HasDefaultDeny || entry.HasEgressDeny:
			entry.ExposureLevel = "partially-restricted"
			entry.RiskLevel = "medium"
		default:
			entry.ExposureLevel = "permissive-policies"
			entry.RiskLevel = "high"
		}

		result.ByNamespace = append(result.ByNamespace, entry)
	}

	// Identify unsegmented pairs (namespaces without any netpol between them)
	for i := 0; i < len(userNamespaces) && len(result.UnsegmentedPairs) < 50; i++ {
		for j := i + 1; j < len(userNamespaces) && len(result.UnsegmentedPairs) < 50; j++ {
			ns1 := userNamespaces[i]
			ns2 := userNamespaces[j]
			np1 := nsNetPolMap[ns1]
			np2 := nsNetPolMap[ns2]
			if len(np1) == 0 && len(np2) == 0 {
				result.UnsegmentedPairs = append(result.UnsegmentedPairs, NetworkSegmentPair{
					Namespace1: ns1,
					Namespace2: ns2,
					Reason:     "neither namespace has NetworkPolicy - unrestricted east-west traffic",
				})
			}
		}
	}

	sort.Slice(result.ByNamespace, func(i, j int) bool {
		rank := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		return rank[result.ByNamespace[i].RiskLevel] < rank[result.ByNamespace[j].RiskLevel]
	})

	if result.Summary.TotalNamespaces > 0 {
		protected := result.Summary.IsolatedNS + result.Summary.DefaultDenyNS
		result.HealthScore = protected * 100 / result.Summary.TotalNamespaces
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("网络分段缺口: %d 命名空间, %d 有 NetPol, %d 无 NetPol, %d 默认拒绝, %d 完全隔离",
			result.Summary.TotalNamespaces, result.Summary.WithNetPol, result.Summary.WithoutNetPol,
			result.Summary.DefaultDenyNS, result.Summary.IsolatedNS),
	}
	if result.Summary.WithoutNetPol > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个命名空间完全没有 NetworkPolicy, 东西向流量不受限制", result.Summary.WithoutNetPol))
	}
	if len(result.UnsegmentedPairs) > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 对命名空间之间无网络隔离", len(result.UnsegmentedPairs)))
	}
	if result.HealthScore < 50 {
		result.Recommendations = append(result.Recommendations, "建议: 为每个命名空间实施 default-deny NetworkPolicy, 按需开放白名单规则")
	}

	_ = strings.TrimSpace
	writeJSON(w, result)
}
