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

// DNSRecordAuditResult checks DNS records for services and ingresses.
type DNSRecordAuditResult struct {
	ScannedAt       time.Time       `json:"scannedAt"`
	Summary         DNSAuditSummary `json:"summary"`
	ByService       []DNSAuditEntry `json:"byService"`
	Issues          []DNSAuditIssue `json:"issues"`
	HealthScore     int             `json:"healthScore"`
	Grade           string          `json:"grade"`
	Recommendations []string        `json:"recommendations"`
}

type DNSAuditSummary struct {
	TotalServices   int `json:"totalServices"`
	ExternalName    int `json:"externalNameServices"`
	LoadBalancer    int `json:"loadBalancerServices"`
	ClusterIP       int `json:"clusterIPServices"`
	Headless        int `json:"headlessServices"`
	NoEndpoints     int `json:"servicesWithoutEndpoints"`
	StaleIngresses  int `json:"staleIngresses"`
	DNSConfigIssues int `json:"dnsConfigIssues"`
}

type DNSAuditEntry struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	ServiceType string `json:"serviceType"`
	ClusterIP   string `json:"clusterIP"`
	ExternalIP  string `json:"externalIP"`
	HasEndpoint bool   `json:"hasEndpoints"`
	IngressRefs int    `json:"ingressRefs"`
	RiskLevel   string `json:"riskLevel"`
}

type DNSAuditIssue struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// handleDNSRecordAudit handles GET /api/product/dns-record-audit
func (s *Server) handleDNSRecordAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := DNSRecordAuditResult{ScannedAt: time.Now()}

	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	ingresses, _ := rc.clientset.NetworkingV1().Ingresses("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	// Build service -> endpoints map
	svcHasEndpoints := make(map[string]bool)
	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning && pod.Spec.NodeName != "" {
			// Mark all services in this namespace as potentially having endpoints
			// (simplified - real implementation would check selectors)
		}
	}

	// Build ingress reference map
	ingressSvcMap := make(map[string]int) // ns/svc -> ingress count
	for _, ing := range ingresses.Items {
		for _, rule := range ing.Spec.Rules {
			if rule.HTTP != nil {
				for _, path := range rule.HTTP.Paths {
					if path.Backend.Service != nil {
						key := ing.Namespace + "/" + path.Backend.Service.Name
						ingressSvcMap[key]++
					}
				}
			}
		}
	}

	for _, svc := range services.Items {
		if isSystemNamespace(svc.Namespace) {
			continue
		}
		result.Summary.TotalServices++

		entry := DNSAuditEntry{
			Name:        svc.Name,
			Namespace:   svc.Namespace,
			ServiceType: string(svc.Spec.Type),
			ClusterIP:   svc.Spec.ClusterIP,
		}

		// Categorize service type
		switch svc.Spec.Type {
		case corev1.ServiceTypeExternalName:
			entry.ExternalIP = svc.Spec.ExternalName
			result.Summary.ExternalName++
		case corev1.ServiceTypeLoadBalancer:
			result.Summary.LoadBalancer++
			if len(svc.Status.LoadBalancer.Ingress) > 0 {
				if svc.Status.LoadBalancer.Ingress[0].IP != "" {
					entry.ExternalIP = svc.Status.LoadBalancer.Ingress[0].IP
				} else {
					entry.ExternalIP = svc.Status.LoadBalancer.Ingress[0].Hostname
				}
			} else {
				result.Issues = append(result.Issues, DNSAuditIssue{
					Name: svc.Name, Namespace: svc.Namespace,
					Issue: "LoadBalancer pending external IP", Severity: "medium",
				})
			}
		case corev1.ServiceTypeClusterIP:
			if svc.Spec.ClusterIP == "None" {
				result.Summary.Headless++
			} else {
				result.Summary.ClusterIP++
			}
		}

		// Check endpoint coverage
		key := svc.Namespace + "/" + svc.Name
		entry.IngressRefs = ingressSvcMap[key]
		entry.HasEndpoint = svcHasEndpoints[key]
		if !entry.HasEndpoint && svc.Spec.Type != corev1.ServiceTypeExternalName {
			// Check if any pod matches selector
			if svc.Spec.Selector != nil && len(svc.Spec.Selector) > 0 {
				for _, pod := range pods.Items {
					if pod.Namespace != svc.Namespace {
						continue
					}
					match := true
					for k, v := range svc.Spec.Selector {
						if pod.Labels[k] != v {
							match = false
							break
						}
					}
					if match && pod.Status.Phase == corev1.PodRunning {
						entry.HasEndpoint = true
						break
					}
				}
			}
			if !entry.HasEndpoint {
				result.Summary.NoEndpoints++
				result.Issues = append(result.Issues, DNSAuditIssue{
					Name: svc.Name, Namespace: svc.Namespace,
					Issue: "no backing pods for selector", Severity: "high",
				})
			}
		}

		// Risk
		switch {
		case !entry.HasEndpoint && svc.Spec.Type != corev1.ServiceTypeExternalName:
			entry.RiskLevel = "high"
		case entry.ExternalIP == "" && svc.Spec.Type == corev1.ServiceTypeLoadBalancer:
			entry.RiskLevel = "medium"
		default:
			entry.RiskLevel = "low"
		}

		result.ByService = append(result.ByService, entry)
	}

	// Check for stale ingresses
	for _, ing := range ingresses.Items {
		if isSystemNamespace(ing.Namespace) {
			continue
		}
		if len(ing.Spec.Rules) == 0 {
			result.Summary.StaleIngresses++
			result.Issues = append(result.Issues, DNSAuditIssue{
				Name: ing.Name, Namespace: ing.Namespace,
				Issue: "ingress has no rules", Severity: "low",
			})
		}
		// Check for DNS issues in hostnames
		for _, tls := range ing.Spec.TLS {
			for _, host := range tls.Hosts {
				if strings.Contains(host, " ") || len(host) > 253 {
					result.Summary.DNSConfigIssues++
					result.Issues = append(result.Issues, DNSAuditIssue{
						Name: ing.Name, Namespace: ing.Namespace,
						Issue: fmt.Sprintf("invalid hostname: %s", host), Severity: "medium",
					})
				}
			}
		}
	}

	sort.Slice(result.ByService, func(i, j int) bool {
		rank := map[string]int{"high": 0, "medium": 1, "low": 2}
		return rank[result.ByService[i].RiskLevel] < rank[result.ByService[j].RiskLevel]
	})

	if result.Summary.TotalServices > 0 {
		goodSvc := result.Summary.TotalServices - result.Summary.NoEndpoints
		result.HealthScore = goodSvc * 100 / result.Summary.TotalServices
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("DNS 记录审计: %d 服务, %d LB, %d ClusterIP, %d Headless, %d 无端点, %d 过期 Ingress",
			result.Summary.TotalServices, result.Summary.LoadBalancer,
			result.Summary.ClusterIP, result.Summary.Headless,
			result.Summary.NoEndpoints, result.Summary.StaleIngresses),
	}
	if result.Summary.NoEndpoints > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个服务无后端 Pod, DNS 解析成功但连接失败", result.Summary.NoEndpoints))
	}
	writeJSON(w, result)
}
