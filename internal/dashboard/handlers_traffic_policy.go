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

// TrafficPolicyResult is the service traffic policy & routing configuration audit.
type TrafficPolicyResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	Summary         TrafficPolicySummary  `json:"summary"`
	ByNamespace     []TrafficPolicyNSStat `json:"byNamespace"`
	Issues          []TrafficPolicyIssue  `json:"issues"`
	ByServiceType   map[string]int        `json:"byServiceType"`
	Recommendations []string              `json:"recommendations"`
	HealthScore     int                   `json:"healthScore"`
}

// TrafficPolicySummary aggregates service traffic policy statistics.
type TrafficPolicySummary struct {
	TotalServices       int `json:"totalServices"`
	ClusterIP           int `json:"clusterIP"`
	NodePort            int `json:"nodePort"`
	LoadBalancer        int `json:"loadBalancer"`
	ExternalName        int `json:"externalName"`
	ExtTrafficCluster   int `json:"extTrafficCluster"`   // externalTrafficPolicy=Cluster (suboptimal)
	ExtTrafficLocal     int `json:"extTrafficLocal"`     // externalTrafficPolicy=Local (optimal)
	SessionAffinityNone int `json:"sessionAffinityNone"` // session affinity = None
	SessionAffinityIP   int `json:"sessionAffinityIP"`   // session affinity = ClientIP
	OverExposed         int `json:"overExposed"`         // LoadBalancer that should be ClusterIP
	HasExternalIPs      int `json:"hasExternalIPs"`      // services with external IPs set
	PublishNotReady     int `json:"publishNotReady"`     // publishNotReadyAddresses=true
	NoSelector          int `json:"noSelector"`          // services without selector (manual endpoints)
}

// TrafficPolicyNSStat shows service stats per namespace.
type TrafficPolicyNSStat struct {
	Namespace    string `json:"namespace"`
	ServiceCount int    `json:"serviceCount"`
	Issues       int    `json:"issues"`
	RiskLevel    string `json:"riskLevel"`
}

// TrafficPolicyIssue describes a specific traffic policy issue.
type TrafficPolicyIssue struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	IssueType string `json:"issueType"`
	Severity  string `json:"severity"`
	Detail    string `json:"detail"`
}

// trafficPolicyAuditCore performs the audit on services and pods (testable).
func trafficPolicyAuditCore(services []corev1.Service) TrafficPolicyResult {
	result := TrafficPolicyResult{
		ScannedAt:     time.Now(),
		ByServiceType: make(map[string]int),
	}

	nsStats := make(map[string]*TrafficPolicyNSStat)

	for i := range services {
		svc := &services[i]
		ns := svc.Namespace
		result.Summary.TotalServices++

		if _, ok := nsStats[ns]; !ok {
			nsStats[ns] = &TrafficPolicyNSStat{Namespace: ns}
		}
		nsStats[ns].ServiceCount++

		// Count by type
		result.ByServiceType[string(svc.Spec.Type)]++
		switch svc.Spec.Type {
		case corev1.ServiceTypeClusterIP:
			result.Summary.ClusterIP++
		case corev1.ServiceTypeNodePort:
			result.Summary.NodePort++
		case corev1.ServiceTypeLoadBalancer:
			result.Summary.LoadBalancer++
		case corev1.ServiceTypeExternalName:
			result.Summary.ExternalName++
		}

		// Check externalTrafficPolicy
		if svc.Spec.ExternalTrafficPolicy == corev1.ServiceExternalTrafficPolicyTypeCluster {
			result.Summary.ExtTrafficCluster++
			// Only an issue for NodePort/LoadBalancer
			if svc.Spec.Type == corev1.ServiceTypeLoadBalancer || svc.Spec.Type == corev1.ServiceTypeNodePort {
				result.Issues = append(result.Issues, TrafficPolicyIssue{
					Name:      svc.Name,
					Namespace: ns,
					IssueType: "ext-traffic-cluster",
					Severity:  "medium",
					Detail:    "externalTrafficPolicy=Cluster causes extra hop and hides client source IP — use Local for direct routing",
				})
				nsStats[ns].Issues++
			}
		} else if svc.Spec.ExternalTrafficPolicy == corev1.ServiceExternalTrafficPolicyTypeLocal {
			result.Summary.ExtTrafficLocal++
		}

		// Check session affinity
		if svc.Spec.SessionAffinity == corev1.ServiceAffinityClientIP {
			result.Summary.SessionAffinityIP++
		} else {
			result.Summary.SessionAffinityNone++
		}

		// Check for over-exposure: LoadBalancer services in non-system namespaces
		// that could be ClusterIP if accessed only internally
		if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
			isSystemNS := isSystemNamespace(ns)
			if !isSystemNS && len(svc.Status.LoadBalancer.Ingress) == 0 {
				// LoadBalancer without external IP — might be misconfigured
				result.Issues = append(result.Issues, TrafficPolicyIssue{
					Name:      svc.Name,
					Namespace: ns,
					IssueType: "lb-no-external-ip",
					Severity:  "low",
					Detail:    "LoadBalancer service has no external IP — may be pending or misconfigured",
				})
				nsStats[ns].Issues++
			}
			if !isSystemNS && len(svc.Status.LoadBalancer.Ingress) > 0 {
				result.Summary.OverExposed++
				result.Issues = append(result.Issues, TrafficPolicyIssue{
					Name:      svc.Name,
					Namespace: ns,
					IssueType: "over-exposed",
					Severity:  "info",
					Detail:    "LoadBalancer service exposes workload externally — verify if external access is required (use ClusterIP for internal-only)",
				})
			}
		}

		// Check external IPs
		if len(svc.Spec.ExternalIPs) > 0 {
			result.Summary.HasExternalIPs++
			result.Issues = append(result.Issues, TrafficPolicyIssue{
				Name:      svc.Name,
				Namespace: ns,
				IssueType: "external-ips",
				Severity:  "medium",
				Detail:    fmt.Sprintf("service has external IPs set (%s) — bypasses cloud provider's load balancer management", strings.Join(svc.Spec.ExternalIPs, ",")),
			})
			nsStats[ns].Issues++
		}

		// Check publishNotReadyAddresses
		if svc.Spec.PublishNotReadyAddresses {
			result.Summary.PublishNotReady++
			// This is sometimes intentional (StatefulSet headless services) but can cause issues
			if svc.Spec.Type != corev1.ServiceTypeClusterIP || svc.Spec.ClusterIP != corev1.ClusterIPNone {
				result.Issues = append(result.Issues, TrafficPolicyIssue{
					Name:      svc.Name,
					Namespace: ns,
					IssueType: "publish-not-ready",
					Severity:  "low",
					Detail:    "publishNotReadyAddresses=true — traffic may be sent to pods that are not ready",
				})
			}
		}

		// Check no selector
		if svc.Spec.Selector == nil || len(svc.Spec.Selector) == 0 {
			result.Summary.NoSelector++
			// Not necessarily bad (manual endpoints, ExternalName) but worth tracking
		}

		// Check ExternalName services
		if svc.Spec.Type == corev1.ServiceTypeExternalName {
			result.Issues = append(result.Issues, TrafficPolicyIssue{
				Name:      svc.Name,
				Namespace: ns,
				IssueType: "external-name",
				Severity:  "info",
				Detail:    fmt.Sprintf("ExternalName service points to %s — CNAME redirect, no load balancing or health checks", svc.Spec.ExternalName),
			})
		}
	}

	// Build namespace stats
	for _, stat := range nsStats {
		stat.RiskLevel = "low"
		if stat.Issues > 3 {
			stat.RiskLevel = "medium"
		}
		if stat.Issues > 7 {
			stat.RiskLevel = "high"
		}
		result.ByNamespace = append(result.ByNamespace, *stat)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].Issues > result.ByNamespace[j].Issues
	})

	// Sort issues by severity
	sort.Slice(result.Issues, func(i, j int) bool {
		return result.Issues[i].Severity > result.Issues[j].Severity
	})

	result.HealthScore = trafficPolicyScore(result.Summary)
	result.Recommendations = trafficPolicyRecommendations(result.Summary)

	return result
}

// trafficPolicyScore calculates the health score.
func trafficPolicyScore(s TrafficPolicySummary) int {
	if s.TotalServices == 0 {
		return 100
	}
	base := 100
	base -= s.ExtTrafficCluster * 3
	base -= s.HasExternalIPs * 5
	base -= s.OverExposed * 1 // info-level, minimal penalty
	base -= (s.PublishNotReady * 1)
	if base < 0 {
		base = 0
	}
	return base
}

// trafficPolicyRecommendations generates actionable recommendations.
func trafficPolicyRecommendations(s TrafficPolicySummary) []string {
	var recs []string
	if s.ExtTrafficCluster > 0 {
		recs = append(recs, fmt.Sprintf("%d service(s) use externalTrafficPolicy=Cluster — switch to Local to preserve client source IP and reduce hops", s.ExtTrafficCluster))
	}
	if s.HasExternalIPs > 0 {
		recs = append(recs, fmt.Sprintf("%d service(s) have external IPs set — use cloud provider LoadBalancer or Ingress instead of manual external IPs", s.HasExternalIPs))
	}
	if s.OverExposed > 0 {
		recs = append(recs, fmt.Sprintf("%d LoadBalancer service(s) expose workloads externally — verify each needs external access, convert to ClusterIP if internal-only", s.OverExposed))
	}
	if s.PublishNotReady > 0 {
		recs = append(recs, fmt.Sprintf("%d service(s) publish not-ready pod addresses — traffic may reach pods that fail health checks", s.PublishNotReady))
	}
	if s.ExternalName > 0 {
		recs = append(recs, fmt.Sprintf("%d ExternalName service(s) — CNAME redirects bypass k8s load balancing, consider migrating to Endpoints for health checking", s.ExternalName))
	}
	if s.ExtTrafficCluster == 0 && s.HasExternalIPs == 0 {
		recs = append(recs, fmt.Sprintf("service traffic policies are properly configured — %d services across %d types", s.TotalServices, len([]int{s.ClusterIP, s.NodePort, s.LoadBalancer, s.ExternalName})))
	}
	return recs
}

// handleTrafficPolicy audits service traffic policy and routing configuration.
// GET /api/product/traffic-policy
func (s *Server) handleTrafficPolicy(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	services, err := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	result := trafficPolicyAuditCore(services.Items)
	writeJSON(w, result)
}
