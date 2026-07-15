package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// ExposureMapResult is the cluster-wide external exposure surface risk map.
type ExposureMapResult struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	Summary         ExposureSummary  `json:"summary"`
	EntryPoints     []ExposureEntry  `json:"entryPoints"`
	ByNamespace     []ExposureNSStat `json:"byNamespace"`
	HighRiskPaths   []HighRiskPath   `json:"highRiskPaths,omitempty"`
	OrphanExposure  []OrphanEndpoint `json:"orphanExposure,omitempty"`
	Recommendations []string         `json:"recommendations"`
	RiskScore       int              `json:"riskScore"`
}

// ExposureSummary aggregates exposure surface statistics.
type ExposureSummary struct {
	TotalIngresses        int `json:"totalIngresses"`
	TotalLoadBalancers    int `json:"totalLoadBalancers"`
	TotalNodePorts        int `json:"totalNodePorts"`
	TotalExternalIPs      int `json:"totalExternalIPs"`
	TotalExposedPorts     int `json:"totalExposedPorts"`
	WithTLS               int `json:"withTLS"`
	WithoutTLS            int `json:"withoutTLS"`
	WithAuth              int `json:"withAuth"` // has auth-related annotations
	WithoutAuth           int `json:"withoutAuth"`
	ExposedNamespaces     int `json:"exposedNamespaces"`
	TotalBackendWorkloads int `json:"totalBackendWorkloads"`
}

// ExposureEntry describes a single external exposure point.
type ExposureEntry struct {
	Type            string   `json:"type"` // ingress, loadbalancer, nodeport, externalip
	Name            string   `json:"name"`
	Namespace       string   `json:"namespace"`
	Hosts           []string `json:"hosts,omitempty"`
	Paths           []string `json:"paths,omitempty"`
	Ports           []string `json:"ports,omitempty"`
	Backend         string   `json:"backend"` // backing service
	BackendWorkload string   `json:"backendWorkload"`
	HasTLS          bool     `json:"hasTLS"`
	HasAuth         bool     `json:"hasAuth"`
	RiskLevel       string   `json:"riskLevel"`
	ExposureType    string   `json:"exposureType"` // public, internal, unknown
}

// ExposureNSStat is per-namespace exposure statistics.
type ExposureNSStat struct {
	Namespace     string `json:"namespace"`
	IngressCount  int    `json:"ingressCount"`
	LBCount       int    `json:"lbCount"`
	NodePortCount int    `json:"nodePortCount"`
	ExposedPorts  int    `json:"exposedPorts"`
	WithoutTLS    int    `json:"withoutTLS"`
	RiskLevel     string `json:"riskLevel"`
}

// HighRiskPath identifies an exposed path with elevated risk.
type HighRiskPath struct {
	Host        string `json:"host"`
	Path        string `json:"path"`
	Namespace   string `json:"namespace"`
	IngressName string `json:"ingressName"`
	Workload    string `json:"workload"`
	Reason      string `json:"reason"`
	Severity    string `json:"severity"`
}

// OrphanEndpoint is an exposure point with no healthy backing workload.
type OrphanEndpoint struct {
	Type      string `json:"type"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Backend   string `json:"backend"`
	Reason    string `json:"reason"`
}

// handleExposureMap maps the entire cluster's external attack surface by
// tracing all network entry points to their backing workloads.
// GET /api/product/exposure-map
func (s *Server) handleExposureMap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")
	result := ExposureMapResult{ScannedAt: time.Now()}

	// 1. Collect services for backend tracing
	svcMap := map[string]*corev1.Service{} // "ns/name" → service
	svcToWorkload := map[string]string{}   // "ns/name" → workload name

	services, err := rc.clientset.CoreV1().Services(ns).List(ctx, metav1.ListOptions{})
	if err == nil {
		for i := range services.Items {
			svc := &services.Items[i]
			key := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)
			svcMap[key] = svc
		}
	}

	// 2. Collect pods to trace services → workloads
	pods, err := rc.clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err == nil {
		svcPods := map[string]map[string]bool{} // svcKey → set of workload names
		for i := range pods.Items {
			pod := &pods.Items[i]
			wlName, _ := extractWorkloadFromPod(pod)
			if wlName == "" {
				continue
			}
			podLabels := labels.Set(pod.Labels)
			for _, svc := range services.Items {
				if svc.Namespace != pod.Namespace || len(svc.Spec.Selector) == 0 {
					continue
				}
				if labelSelectorMatches(metav1.LabelSelector{MatchLabels: svc.Spec.Selector}, podLabels) {
					svcKey := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)
					if svcPods[svcKey] == nil {
						svcPods[svcKey] = map[string]bool{}
					}
					svcPods[svcKey][wlName] = true
				}
			}
		}
		for svcKey, wls := range svcPods {
			names := make([]string, 0, len(wls))
			for n := range wls {
				names = append(names, n)
			}
			svcToWorkload[svcKey] = strings.Join(names, ",")
		}
	}

	// 3. Process Ingresses
	var allEntries []ExposureEntry
	ingresses, err := rc.clientset.NetworkingV1().Ingresses(ns).List(ctx, metav1.ListOptions{})
	if err == nil {
		result.Summary.TotalIngresses = len(ingresses.Items)
		for _, ing := range ingresses.Items {
			hasTLS := len(ing.Spec.TLS) > 0
			hasAuth := checkIngressAuth(&ing)

			for _, rule := range ing.Spec.Rules {
				if rule.HTTP == nil {
					continue
				}
				for _, path := range rule.HTTP.Paths {
					backendSvc := ""
					if path.Backend.Service != nil {
						backendSvc = path.Backend.Service.Name
					}
					backendKey := fmt.Sprintf("%s/%s", ing.Namespace, backendSvc)
					workload := svcToWorkload[backendKey]

					entry := ExposureEntry{
						Type:            "ingress",
						Name:            ing.Name,
						Namespace:       ing.Namespace,
						Backend:         backendSvc,
						BackendWorkload: workload,
						HasTLS:          hasTLS,
						HasAuth:         hasAuth,
						ExposureType:    classifyExposure(rule.Host),
					}
					if rule.Host != "" {
						entry.Hosts = []string{rule.Host}
					}
					entry.Paths = []string{path.Path}
					entry.RiskLevel = assessExposureRisk(entry)
					allEntries = append(allEntries, entry)

					// Check for high-risk paths
					if isHighRiskPath(path.Path) {
						result.HighRiskPaths = append(result.HighRiskPaths, HighRiskPath{
							Host:        rule.Host,
							Path:        path.Path,
							Namespace:   ing.Namespace,
							IngressName: ing.Name,
							Workload:    workload,
							Reason:      "Sensitive path exposed externally",
							Severity:    "warning",
						})
					}
				}
			}

			if !hasTLS {
				result.Summary.WithoutTLS++
			} else {
				result.Summary.WithTLS++
			}
			if hasAuth {
				result.Summary.WithAuth++
			} else {
				result.Summary.WithoutAuth++
			}
		}
	}

	// 4. Process LoadBalancer and NodePort services
	if services != nil {
		for _, svc := range services.Items {
			if svc.Namespace == "kube-system" && strings.HasPrefix(svc.Name, "kube-") {
				continue // skip kube-system internal services
			}

			switch svc.Spec.Type {
			case corev1.ServiceTypeLoadBalancer:
				result.Summary.TotalLoadBalancers++
				ports := make([]string, 0)
				for _, p := range svc.Spec.Ports {
					ports = append(ports, fmt.Sprintf("%d/%s", p.Port, string(p.Protocol)))
					result.Summary.TotalExposedPorts++
				}
				svcKey := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)
				entry := ExposureEntry{
					Type:            "loadbalancer",
					Name:            svc.Name,
					Namespace:       svc.Namespace,
					Ports:           ports,
					Backend:         svc.Name,
					BackendWorkload: svcToWorkload[svcKey],
					HasTLS:          false,
					ExposureType:    "public",
					RiskLevel:       "high",
				}
				allEntries = append(allEntries, entry)

			case corev1.ServiceTypeNodePort:
				result.Summary.TotalNodePorts++
				ports := make([]string, 0)
				for _, p := range svc.Spec.Ports {
					if p.NodePort != 0 {
						ports = append(ports, fmt.Sprintf("%d", p.NodePort))
						result.Summary.TotalExposedPorts++
					}
				}
				svcKey := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)
				entry := ExposureEntry{
					Type:            "nodeport",
					Name:            svc.Name,
					Namespace:       svc.Namespace,
					Ports:           ports,
					Backend:         svc.Name,
					BackendWorkload: svcToWorkload[svcKey],
					ExposureType:    "public",
					RiskLevel:       "medium",
				}
				allEntries = append(allEntries, entry)
			}

			// External IPs
			if len(svc.Spec.ExternalIPs) > 0 {
				result.Summary.TotalExternalIPs += len(svc.Spec.ExternalIPs)
				entry := ExposureEntry{
					Type:         "externalip",
					Name:         svc.Name,
					Namespace:    svc.Namespace,
					Ports:        []string{fmt.Sprintf("%d", svc.Spec.Ports[0].Port)},
					ExposureType: "public",
					RiskLevel:    "high",
				}
				allEntries = append(allEntries, entry)
			}
		}
	}

	// 5. Detect orphan exposure (no backing workload)
	for _, entry := range allEntries {
		if entry.BackendWorkload == "" && entry.Backend != "" {
			result.OrphanExposure = append(result.OrphanExposure, OrphanEndpoint{
				Type:      entry.Type,
				Name:      entry.Name,
				Namespace: entry.Namespace,
				Backend:   entry.Backend,
				Reason:    "Service has no healthy backing pods/workload",
			})
		}
	}

	// 6. Sort entries by risk level
	sort.Slice(allEntries, func(i, j int) bool {
		return riskOrder(allEntries[i].RiskLevel) < riskOrder(allEntries[j].RiskLevel)
	})
	if len(allEntries) > 200 {
		allEntries = allEntries[:200]
	}
	result.EntryPoints = allEntries

	// 7. Build namespace stats
	nsMap := map[string]*ExposureNSStat{}
	for _, e := range allEntries {
		if nsMap[e.Namespace] == nil {
			nsMap[e.Namespace] = &ExposureNSStat{Namespace: e.Namespace}
		}
		switch e.Type {
		case "ingress":
			nsMap[e.Namespace].IngressCount++
		case "loadbalancer":
			nsMap[e.Namespace].LBCount++
		case "nodeport":
			nsMap[e.Namespace].NodePortCount++
		}
		nsMap[e.Namespace].ExposedPorts += len(e.Ports)
		if !e.HasTLS {
			nsMap[e.Namespace].WithoutTLS++
		}
	}
	for _, stat := range nsMap {
		stat.RiskLevel = assessNSRisk(*stat)
		result.ByNamespace = append(result.ByNamespace, *stat)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].WithoutTLS > result.ByNamespace[j].WithoutTLS
	})

	// 8. Count unique backend workloads
	workloadSet := map[string]bool{}
	for _, e := range allEntries {
		if e.BackendWorkload != "" {
			for _, w := range strings.Split(e.BackendWorkload, ",") {
				workloadSet[w] = true
			}
		}
	}
	result.Summary.TotalBackendWorkloads = len(workloadSet)
	result.Summary.ExposedNamespaces = len(nsMap)

	// 9. Risk score
	score := 100
	score -= result.Summary.WithoutTLS * 5
	score -= result.Summary.TotalLoadBalancers * 3
	score -= len(result.OrphanExposure) * 4
	score -= len(result.HighRiskPaths) * 5
	if result.Summary.WithoutAuth > result.Summary.WithAuth && result.Summary.TotalIngresses > 3 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.RiskScore = score

	// 10. Recommendations
	result.Recommendations = generateExposureRecommendations(result)

	writeJSON(w, result)
}

// checkIngressAuth detects auth annotations on ingress.
func checkIngressAuth(ing *networkingv1.Ingress) bool {
	for k, v := range ing.Annotations {
		kLower := strings.ToLower(k)
		if strings.Contains(kLower, "auth") || strings.Contains(kLower, "oauth") ||
			strings.Contains(kLower, "whitelist") || strings.Contains(kLower, "allowed") {
			return true
		}
		_ = v
	}
	return false
}

// classifyExposure determines if a host is public or internal.
func classifyExposure(host string) string {
	if host == "" || host == "localhost" {
		return "internal"
	}
	hostLower := strings.ToLower(host)
	if strings.Contains(hostLower, "internal") || strings.Contains(hostLower, "private") ||
		strings.Contains(hostLower, "local") || strings.HasSuffix(hostLower, ".svc") {
		return "internal"
	}
	return "public"
}

// isHighRiskPath checks if a path exposes sensitive endpoints.
func isHighRiskPath(path string) bool {
	highRiskPaths := []string{"/admin", "/debug", "/metrics", "/actuator", "/api/v1/exec", "/swagger", "/.env", "/healthz"}
	pathLower := strings.ToLower(path)
	for _, rp := range highRiskPaths {
		if strings.HasPrefix(pathLower, rp) {
			return true
		}
	}
	return false
}

// assessExposureRisk evaluates risk level for an exposure point.
func assessExposureRisk(entry ExposureEntry) string {
	risk := 0
	if !entry.HasTLS {
		risk += 30
	}
	if !entry.HasAuth && entry.ExposureType == "public" {
		risk += 20
	}
	if entry.ExposureType == "public" {
		risk += 20
	}
	if entry.BackendWorkload == "" {
		risk += 15
	}

	switch {
	case risk >= 60:
		return "critical"
	case risk >= 40:
		return "high"
	case risk >= 20:
		return "medium"
	default:
		return "low"
	}
}

// assessNSRisk evaluates risk for a namespace.
func assessNSRisk(stat ExposureNSStat) string {
	risk := stat.LBCount*5 + stat.NodePortCount*2 + stat.WithoutTLS*3
	switch {
	case risk >= 20:
		return "high"
	case risk >= 10:
		return "medium"
	default:
		return "low"
	}
}

// riskOrder returns numeric ordering for risk levels.
func riskOrder(level string) int {
	switch level {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	default:
		return 3
	}
}

// generateExposureRecommendations produces actionable recommendations.
func generateExposureRecommendations(result ExposureMapResult) []string {
	var recs []string

	if result.Summary.WithoutTLS > 0 {
		recs = append(recs, fmt.Sprintf("%d ingress endpoint(s) without TLS — enable HTTPS to prevent traffic interception", result.Summary.WithoutTLS))
	}

	if result.Summary.TotalLoadBalancers > 0 {
		recs = append(recs, fmt.Sprintf("%d LoadBalancer service(s) directly exposed — consider routing through Ingress with WAF for centralized security", result.Summary.TotalLoadBalancers))
	}

	if len(result.OrphanExposure) > 0 {
		recs = append(recs, fmt.Sprintf("%d orphan exposure point(s) — Services with no backing workload are still externally reachable", len(result.OrphanExposure)))
	}

	if len(result.HighRiskPaths) > 0 {
		recs = append(recs, fmt.Sprintf("%d high-risk path(s) exposed (admin/debug/metrics) — restrict with auth or remove from external access", len(result.HighRiskPaths)))
	}

	if result.Summary.WithoutAuth > 3 && result.Summary.TotalIngresses > 3 {
		recs = append(recs, fmt.Sprintf("%d ingress endpoint(s) without auth annotations — add auth middleware (OAuth2, basic auth, or IP whitelist)", result.Summary.WithoutAuth))
	}

	if result.RiskScore < 50 {
		recs = append(recs, fmt.Sprintf("Exposure risk score is %d/100 — significant attack surface requires hardening", result.RiskScore))
	}

	if len(recs) == 0 && result.Summary.TotalIngresses > 0 {
		recs = append(recs, "External exposure surface is well-secured — all endpoints have TLS and auth")
	}

	return recs
}
