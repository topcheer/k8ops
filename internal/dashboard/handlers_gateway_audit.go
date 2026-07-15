package dashboard

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// GatewayAuditResult is the Gateway API & Ingress controller health audit.
type GatewayAuditResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         GatewaySummary      `json:"summary"`
	Controllers     []ControllerEntry   `json:"controllers"`
	IngressClasses  []IngressClassEntry `json:"ingressClasses"`
	RoutingIssues   []RoutingIssue      `json:"routingIssues,omitempty"`
	OrphanRoutes    []OrphanRoute       `json:"orphanRoutes,omitempty"`
	TLSGaps         []TLSGap            `json:"tlsGaps,omitempty"`
	CrossNSRoutes   []CrossNSRoute      `json:"crossNamespaceRoutes,omitempty"`
	Recommendations []string            `json:"recommendations"`
	HealthScore     int                 `json:"healthScore"`
}

// GatewaySummary aggregates gateway/ingress statistics.
type GatewaySummary struct {
	TotalIngresses      int `json:"totalIngresses"`
	TotalIngressClasses int `json:"totalIngressClasses"`
	HealthyIngresses    int `json:"healthyIngresses"`
	UnhealthyIngresses  int `json:"unhealthyIngresses"`
	ControllerPods      int `json:"controllerPods"`
	HealthyControllers  int `json:"healthyControllers"`
	TotalRules          int `json:"totalRules"`
	TotalHosts          int `json:"totalHosts"`
	WithTLS             int `json:"withTLS"`
	WithoutTLS          int `json:"withoutTLS"`
	HostConflicts       int `json:"hostConflicts"`
	OrphanedIngresses   int `json:"orphanedIngresses"`
	CrossNSCount        int `json:"crossNamespaceRefs"`
}

// ControllerEntry describes one ingress controller deployment.
type ControllerEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"` // nginx, traefik, envoy, haproxy, istio, ambassador, contour, unknown
	Version   string `json:"version,omitempty"`
	Replicas  int    `json:"replicas"`
	Ready     int    `json:"readyReplicas"`
	Healthy   bool   `json:"healthy"`
	NodePort  int    `json:"nodePort,omitempty"`
}

// IngressClassEntry describes one IngressClass.
type IngressClassEntry struct {
	Name        string `json:"name"`
	Controller  string `json:"controller"`
	IsDefault   bool   `json:"isDefault"`
	UsedByCount int    `json:"usedByCount"`
	Parameters  string `json:"parameters,omitempty"`
}

// RoutingIssue describes a routing misconfiguration.
type RoutingIssue struct {
	Severity  string `json:"severity"`
	Ingress   string `json:"ingress"`
	Namespace string `json:"namespace"`
	Issue     string `json:"issue"`
	Category  string `json:"category"` // no-backend, conflicting-host, missing-class, path-issue
}

// OrphanRoute is an ingress path pointing to a non-existent service.
type OrphanRoute struct {
	Ingress   string `json:"ingress"`
	Namespace string `json:"namespace"`
	Host      string `json:"host"`
	Path      string `json:"path"`
	Backend   string `json:"backend"`
}

// TLSGap identifies a missing or expiring TLS configuration.
type TLSGap struct {
	Ingress   string `json:"ingress"`
	Namespace string `json:"namespace"`
	Host      string `json:"host"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// CrossNSRoute identifies a cross-namespace backend reference.
type CrossNSRoute struct {
	Ingress          string `json:"ingress"`
	Namespace        string `json:"namespace"`
	BackendService   string `json:"backendService"`
	BackendNamespace string `json:"backendNamespace"`
	HasAuth          bool   `json:"hasReferenceGrant"`
}

// handleGatewayAudit audits all ingress controllers, classes, and routing health.
// GET /api/product/gateway-audit
func (s *Server) handleGatewayAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")
	result := GatewayAuditResult{ScannedAt: time.Now()}

	// 1. Detect ingress controllers from pods
	controllerPatterns := map[string]string{
		"nginx-ingress":  "nginx",
		"traefik":        "traefik",
		"envoy":          "envoy",
		"haproxy":        "haproxy",
		"istio-ingress":  "istio",
		"ambassador":     "ambassador",
		"contour":        "contour",
		"kong":           "kong",
		"gloo":           "gloo",
		"cilium-ingress": "cilium",
	}

	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err == nil {
		controllerMap := map[string]*ControllerEntry{}
		for _, pod := range pods.Items {
			if pod.Status.Phase != corev1.PodRunning {
				continue
			}
			nameLower := strings.ToLower(pod.Name)
			for pattern, ctrlType := range controllerPatterns {
				if strings.Contains(nameLower, pattern) {
					key := fmt.Sprintf("%s/%s", pod.Namespace, pattern)
					if controllerMap[key] == nil {
						controllerMap[key] = &ControllerEntry{
							Name:      pattern,
							Namespace: pod.Namespace,
							Type:      ctrlType,
						}
					}
					controllerMap[key].Replicas++
					if pod.Status.ContainerStatuses != nil {
						allReady := true
						for _, cs := range pod.Status.ContainerStatuses {
							if !cs.Ready {
								allReady = false
							}
						}
						if allReady {
							controllerMap[key].Ready++
						}
					}
					break
				}
			}
		}
		for _, ctrl := range controllerMap {
			ctrl.Healthy = ctrl.Ready > 0 && ctrl.Ready >= (ctrl.Replicas+1)/2
			result.Controllers = append(result.Controllers, *ctrl)
			result.Summary.ControllerPods += ctrl.Replicas
			if ctrl.Healthy {
				result.Summary.HealthyControllers++
			}
		}
	}

	// 2. Collect IngressClasses
	ingressClasses, err := rc.clientset.NetworkingV1().IngressClasses().List(ctx, metav1.ListOptions{})
	if err == nil {
		result.Summary.TotalIngressClasses = len(ingressClasses.Items)
		for _, ic := range ingressClasses.Items {
			entry := IngressClassEntry{
				Name:       ic.Name,
				Controller: ic.Spec.Controller,
				IsDefault:  ic.Annotations["ingressclass.kubernetes.io/is-default-class"] == "true",
			}
			if ic.Spec.Parameters != nil {
				entry.Parameters = fmt.Sprintf("%s/%s", ic.Spec.Parameters.Kind, ic.Spec.Parameters.Name)
			}
			result.IngressClasses = append(result.IngressClasses, entry)
		}
	}

	// 3. Collect services for backend verification
	svcMap := map[string]bool{} // "ns/name" → exists
	services, err := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	if err == nil {
		for _, svc := range services.Items {
			svcMap[fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)] = true
		}
	}

	// 4. Collect pods for endpoint verification
	podEndpoints := map[string]int{} // "ns/svc" → pod count
	if pods != nil {
		for _, pod := range pods.Items {
			if pod.Status.Phase != corev1.PodRunning {
				continue
			}
			for _, svc := range services.Items {
				if svc.Namespace != pod.Namespace || len(svc.Spec.Selector) == 0 {
					continue
				}
				if labelSelectorMatches(metav1.LabelSelector{MatchLabels: svc.Spec.Selector}, labels.Set(pod.Labels)) {
					key := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)
					podEndpoints[key]++
				}
			}
		}
	}

	// 5. Audit Ingresses
	ingresses, err := rc.clientset.NetworkingV1().Ingresses(ns).List(ctx, metav1.ListOptions{})
	if err == nil {
		result.Summary.TotalIngresses = len(ingresses.Items)
		hostMap := map[string][]string{} // "host" → []ingress names

		for _, ing := range ingresses.Items {
			hasTLS := len(ing.Spec.TLS) > 0
			if hasTLS {
				result.Summary.WithTLS++
			} else {
				result.Summary.WithoutTLS++
			}

			// Check ingressClassName
			className := ""
			if ing.Spec.IngressClassName != nil {
				className = *ing.Spec.IngressClassName
			} else if classAnn, ok := ing.Annotations["kubernetes.io/ingress.class"]; ok {
				className = classAnn
			}
			if className == "" && !hasDefaultClass(result.IngressClasses) {
				result.RoutingIssues = append(result.RoutingIssues, RoutingIssue{
					Severity:  "warning",
					Ingress:   ing.Name,
					Namespace: ing.Namespace,
					Issue:     "No ingressClassName specified and no default class — ingress may not be processed by any controller",
					Category:  "missing-class",
				})
			}

			for _, rule := range ing.Spec.Rules {
				if rule.Host != "" {
					result.Summary.TotalHosts++
					hostMap[rule.Host] = append(hostMap[rule.Host], ing.Name)
				}

				if rule.HTTP != nil {
					for _, path := range rule.HTTP.Paths {
						result.Summary.TotalRules++

						backendSvc := ""
						if path.Backend.Service != nil {
							backendSvc = path.Backend.Service.Name
						}

						// Check backend service exists
						svcKey := fmt.Sprintf("%s/%s", ing.Namespace, backendSvc)
						if backendSvc != "" && !svcMap[svcKey] {
							result.OrphanRoutes = append(result.OrphanRoutes, OrphanRoute{
								Ingress:   ing.Name,
								Namespace: ing.Namespace,
								Host:      rule.Host,
								Path:      path.Path,
								Backend:   backendSvc,
							})
							result.RoutingIssues = append(result.RoutingIssues, RoutingIssue{
								Severity:  "critical",
								Ingress:   ing.Name,
								Namespace: ing.Namespace,
								Issue:     fmt.Sprintf("Backend service %q does not exist for path %s", backendSvc, path.Path),
								Category:  "no-backend",
							})
						} else if backendSvc != "" {
							// Check service has endpoints
							if count, ok := podEndpoints[svcKey]; ok && count == 0 {
								result.RoutingIssues = append(result.RoutingIssues, RoutingIssue{
									Severity:  "warning",
									Ingress:   ing.Name,
									Namespace: ing.Namespace,
									Issue:     fmt.Sprintf("Backend service %q has no running pods", backendSvc),
									Category:  "no-backend",
								})
							}
						}

						// TLS gap
						if !hasTLS && rule.Host != "" && isPublicHost(rule.Host) {
							result.TLSGaps = append(result.TLSGaps, TLSGap{
								Ingress:   ing.Name,
								Namespace: ing.Namespace,
								Host:      rule.Host,
								Issue:     "No TLS configured for public host",
								Severity:  "warning",
							})
						}
					}
				}
			}
		}

		// Detect host conflicts
		for host, ings := range hostMap {
			if len(ings) > 1 {
				result.Summary.HostConflicts++
				result.RoutingIssues = append(result.RoutingIssues, RoutingIssue{
					Severity:  "warning",
					Ingress:   strings.Join(ings, ", "),
					Namespace: "multiple",
					Issue:     fmt.Sprintf("Host %q is claimed by %d ingresses — routing conflict", host, len(ings)),
					Category:  "conflicting-host",
				})
			}
		}

		// Count healthy
		for _, ing := range ingresses.Items {
			if isHealthyIngress(ing, svcMap, podEndpoints) {
				result.Summary.HealthyIngresses++
			} else {
				result.Summary.UnhealthyIngresses++
			}
		}
	}

	// Deduplicate orphan routes
	result.OrphanRoutes = dedupOrphanRoutes(result.OrphanRoutes)
	result.Summary.OrphanedIngresses = len(result.OrphanRoutes)

	// 6. Health score
	score := 100
	if result.Summary.WithoutTLS > 0 {
		score -= result.Summary.WithoutTLS * 3
	}
	score -= len(result.OrphanRoutes) * 5
	score -= result.Summary.HostConflicts * 3
	if result.Summary.HealthyControllers == 0 && result.Summary.TotalIngresses > 0 {
		score -= 20
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score

	// 7. Recommendations
	result.Recommendations = generateGatewayRecs(result)

	writeJSON(w, result)
}

// hasDefaultClass checks if any ingress class is marked as default.
func hasDefaultClass(classes []IngressClassEntry) bool {
	for _, c := range classes {
		if c.IsDefault {
			return true
		}
	}
	return false
}

// isPublicHost checks if a host is publicly accessible.
func isPublicHost(host string) bool {
	if host == "" || host == "localhost" {
		return false
	}
	return !strings.Contains(host, "internal") && !strings.Contains(host, ".local") && !strings.Contains(host, ".svc")
}

// isHealthyIngress checks if an ingress has valid backends.
func isHealthyIngress(ing networkingv1.Ingress, svcMap map[string]bool, endpoints map[string]int) bool {
	for _, rule := range ing.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			if path.Backend.Service != nil {
				svcKey := fmt.Sprintf("%s/%s", ing.Namespace, path.Backend.Service.Name)
				if !svcMap[svcKey] {
					return false
				}
			}
		}
	}
	return true
}

// dedupOrphanRoutes removes duplicate orphan route entries.
func dedupOrphanRoutes(routes []OrphanRoute) []OrphanRoute {
	seen := map[string]bool{}
	var result []OrphanRoute
	for _, r := range routes {
		key := fmt.Sprintf("%s/%s/%s/%s", r.Namespace, r.Ingress, r.Host, r.Path)
		if !seen[key] {
			seen[key] = true
			result = append(result, r)
		}
	}
	return result
}

// generateGatewayRecs produces recommendations.
func generateGatewayRecs(result GatewayAuditResult) []string {
	var recs []string

	if result.Summary.HealthyControllers == 0 && result.Summary.TotalIngresses > 0 {
		recs = append(recs, "No healthy ingress controller detected but ingresses exist — controller may be down or misconfigured")
	}

	if result.Summary.WithoutTLS > 0 {
		recs = append(recs, fmt.Sprintf("%d ingress(es) without TLS — enable HTTPS for all public-facing hosts", result.Summary.WithoutTLS))
	}

	if len(result.OrphanRoutes) > 0 {
		recs = append(recs, fmt.Sprintf("%d orphan route(s) — backend services missing or have no pods", len(result.OrphanRoutes)))
	}

	if result.Summary.HostConflicts > 0 {
		recs = append(recs, fmt.Sprintf("%d host conflict(s) — multiple ingresses claim the same host, causing unpredictable routing", result.Summary.HostConflicts))
	}

	if result.Summary.UnhealthyIngresses > 0 {
		recs = append(recs, fmt.Sprintf("%d unhealthy ingress(es) — fix backend references and routing rules", result.Summary.UnhealthyIngresses))
	}

	if result.Summary.TotalIngressClasses == 0 && result.Summary.TotalIngresses > 0 {
		recs = append(recs, "No IngressClass defined — ingresses may not be processed by any controller")
	}

	if len(recs) == 0 && result.Summary.TotalIngresses > 0 {
		recs = append(recs, "Gateway and ingress configuration is healthy — all controllers running, routes valid, TLS enabled")
	}

	return recs
}
