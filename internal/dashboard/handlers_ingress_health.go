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
)

// IngressHealthResult is the full ingress traffic routing analysis.
type IngressHealthResult struct {
	ScannedAt       time.Time       `json:"scannedAt"`
	Summary         IngressSummary  `json:"summary"`
	Ingresses       []IngressEntry  `json:"ingresses"`
	Issues          []IngressIssue  `json:"issues"`
	ByNamespace     []IngressNsStat `json:"byNamespace"`
	Recommendations []string        `json:"recommendations"`
}

// IngressSummary aggregates cluster-wide ingress health.
type IngressSummary struct {
	TotalIngresses   int `json:"totalIngresses"`
	HealthyIngresses int `json:"healthyIngresses"`
	IngressWithTLS   int `json:"ingressWithTLS"`
	IngressNoTLS     int `json:"ingressNoTLS"`
	NoBackend        int `json:"noBackend"`      // backend service missing or no endpoints
	NoRules          int `json:"noRules"`        // ingress with no routing rules
	HostConflicts    int `json:"hostConflicts"`  // duplicate host+path in different ingresses
	DefaultBackend   int `json:"defaultBackend"` // has default backend
	MissingClass     int `json:"missingClass"`   // no ingressClassName and no kubernetes.io/ingress.class annotation
	HealthScore      int `json:"healthScore"`    // 0-100
}

// IngressEntry describes health for one ingress resource.
type IngressEntry struct {
	Name              string                 `json:"name"`
	Namespace         string                 `json:"namespace"`
	IngressClass      string                 `json:"ingressClass"`
	Hosts             []string               `json:"hosts"`
	TLSHosts          []string               `json:"tlsHosts"`
	HasTLS            bool                   `json:"hasTLS"`
	RuleCount         int                    `json:"ruleCount"`
	Backends          []IngressBackendStatus `json:"backends"`
	HasDefaultBackend bool                   `json:"hasDefaultBackend"`
	Status            string                 `json:"status"` // healthy / warning / critical
	Issues            []string               `json:"issues"`
}

// IngressBackendStatus describes one backend service in an ingress rule.
type IngressBackendStatus struct {
	ServiceName   string `json:"serviceName"`
	ServicePort   string `json:"servicePort"`
	Path          string `json:"path"`
	PathType      string `json:"pathType"`
	Host          string `json:"host"`
	ServiceExists bool   `json:"serviceExists"`
	HasEndpoints  bool   `json:"hasEndpoints"`
}

// IngressIssue is a specific detected problem.
type IngressIssue struct {
	Ingress   string `json:"ingress"`
	Namespace string `json:"namespace"`
	Severity  string `json:"severity"` // critical / warning / info
	Type      string `json:"type"`     // no-backend / no-tls / no-class / conflict / no-rules
	Message   string `json:"message"`
}

// IngressNsStat per-namespace ingress stats.
type IngressNsStat struct {
	Namespace string `json:"namespace"`
	Total     int    `json:"total"`
	Healthy   int    `json:"healthy"`
	WithTLS   int    `json:"withTLS"`
	Issues    int    `json:"issues"`
}

// handleIngressHealth analyzes ingress traffic routing health.
// GET /api/product/ingress-health?namespace=xxx
func (s *Server) handleIngressHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")

	ingresses, err := rc.clientset.NetworkingV1().Ingresses(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	services, _ := rc.clientset.CoreV1().Services(ns).List(ctx, metav1.ListOptions{})
	endpoints, _ := rc.clientset.CoreV1().Endpoints(ns).List(ctx, metav1.ListOptions{})
	ingressClasses, _ := rc.clientset.NetworkingV1().IngressClasses().List(ctx, metav1.ListOptions{})

	// Build service lookup: ns/name → exists
	svcExists := make(map[string]bool)
	for _, svc := range services.Items {
		svcExists[fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)] = true
	}

	// Build endpoint lookup: ns/name → has ready endpoints
	epReady := make(map[string]bool)
	for _, ep := range endpoints.Items {
		for _, subset := range ep.Subsets {
			if len(subset.Addresses) > 0 {
				epReady[fmt.Sprintf("%s/%s", ep.Namespace, ep.Name)] = true
				break
			}
		}
	}

	// Build valid ingress class lookup
	validClasses := make(map[string]bool)
	defaultClass := ""
	for _, ic := range ingressClasses.Items {
		validClasses[ic.Name] = true
		if ic.Annotations["ingressclass.kubernetes.io/is-default-class"] == "true" {
			defaultClass = ic.Name
		}
	}

	// Build host+path conflict detection
	hostPathMap := make(map[string][]string) // "host|path" → []ingress names

	result := IngressHealthResult{ScannedAt: time.Now()}
	nsMap := make(map[string]*IngressNsStat)

	for _, ing := range ingresses.Items {
		entry := IngressEntry{
			Name:      ing.Name,
			Namespace: ing.Namespace,
			RuleCount: len(ing.Spec.Rules),
		}

		// Ingress class
		if ing.Spec.IngressClassName != nil {
			entry.IngressClass = *ing.Spec.IngressClassName
		} else if ann, ok := ing.Annotations["kubernetes.io/ingress.class"]; ok {
			entry.IngressClass = ann
		}

		// Default backend
		entry.HasDefaultBackend = ing.Spec.DefaultBackend != nil

		// TLS
		entry.HasTLS = len(ing.Spec.TLS) > 0
		for _, tls := range ing.Spec.TLS {
			entry.TLSHosts = append(entry.TLSHosts, tls.Hosts...)
		}

		// Rules and backends
		for _, rule := range ing.Spec.Rules {
			entry.Hosts = append(entry.Hosts, rule.Host)
			if rule.HTTP == nil {
				continue
			}
			for _, path := range rule.HTTP.Paths {
				backend := IngressBackendStatus{
					Host:        rule.Host,
					Path:        path.Path,
					PathType:    "ImplementationSpecific",
					ServiceName: "",
					ServicePort: "",
				}
				if path.PathType != nil {
					backend.PathType = string(*path.PathType)
				}

				// Check backend service
				if path.Backend.Service != nil {
					backend.ServiceName = path.Backend.Service.Name
					if path.Backend.Service.Port.Name != "" {
						backend.ServicePort = path.Backend.Service.Port.Name
					} else {
						backend.ServicePort = fmt.Sprintf("%d", path.Backend.Service.Port.Number)
					}

					svcKey := fmt.Sprintf("%s/%s", ing.Namespace, backend.ServiceName)
					backend.ServiceExists = svcExists[svcKey]
					backend.HasEndpoints = epReady[svcKey]
				}

				entry.Backends = append(entry.Backends, backend)

				// Track host+path for conflict detection
				hpKey := fmt.Sprintf("%s|%s", rule.Host, path.Path)
				hostPathMap[hpKey] = append(hostPathMap[hpKey], fmt.Sprintf("%s/%s", ing.Namespace, ing.Name))
			}
		}

		// Analyze issues
		var issues []string
		var ingIssues []IngressIssue
		issueCount := 0

		// No rules
		if len(ing.Spec.Rules) == 0 && !entry.HasDefaultBackend {
			issues = append(issues, "no routing rules and no default backend — ingress does nothing")
			ingIssues = append(ingIssues, IngressIssue{
				Ingress: ing.Name, Namespace: ing.Namespace,
				Severity: "warning", Type: "no-rules",
				Message: "Ingress has no rules and no default backend",
			})
			issueCount++
			result.Summary.NoRules++
		}

		// Backend issues
		for _, be := range entry.Backends {
			if !be.ServiceExists {
				msg := fmt.Sprintf("backend service %s does not exist for path %s", be.ServiceName, be.Path)
				issues = append(issues, msg)
				ingIssues = append(ingIssues, IngressIssue{
					Ingress: ing.Name, Namespace: ing.Namespace,
					Severity: "critical", Type: "no-backend",
					Message: msg,
				})
				issueCount++
				result.Summary.NoBackend++
			} else if !be.HasEndpoints {
				msg := fmt.Sprintf("backend service %s has no ready endpoints for path %s", be.ServiceName, be.Path)
				issues = append(issues, msg)
				ingIssues = append(ingIssues, IngressIssue{
					Ingress: ing.Name, Namespace: ing.Namespace,
					Severity: "warning", Type: "no-backend",
					Message: msg,
				})
				issueCount++
			}
		}

		// No TLS
		if !entry.HasTLS && len(entry.Hosts) > 0 {
			issues = append(issues, "no TLS configured — traffic is unencrypted")
			ingIssues = append(ingIssues, IngressIssue{
				Ingress: ing.Name, Namespace: ing.Namespace,
				Severity: "warning", Type: "no-tls",
				Message: "Ingress has hosts but no TLS configuration",
			})
			issueCount++
			result.Summary.IngressNoTLS++
		} else if entry.HasTLS {
			result.Summary.IngressWithTLS++
		}

		// Missing ingress class
		if entry.IngressClass == "" && defaultClass == "" {
			issues = append(issues, "no ingressClassName specified and no default IngressClass")
			ingIssues = append(ingIssues, IngressIssue{
				Ingress: ing.Name, Namespace: ing.Namespace,
				Severity: "warning", Type: "no-class",
				Message: "No IngressClass specified and no default class found",
			})
			issueCount++
			result.Summary.MissingClass++
		} else if entry.IngressClass != "" && !validClasses[entry.IngressClass] {
			issues = append(issues, fmt.Sprintf("ingressClass %q does not exist in cluster", entry.IngressClass))
			ingIssues = append(ingIssues, IngressIssue{
				Ingress: ing.Name, Namespace: ing.Namespace,
				Severity: "critical", Type: "no-class",
				Message: fmt.Sprintf("IngressClass %q not found", entry.IngressClass),
			})
			issueCount++
			result.Summary.MissingClass++
		}

		entry.Issues = issues
		entry.Status = assessIngressStatus(issueCount, ingIssues)

		result.Ingresses = append(result.Ingresses, entry)
		result.Issues = append(result.Issues, ingIssues...)

		// Summary
		result.Summary.TotalIngresses++
		if entry.Status == "healthy" {
			result.Summary.HealthyIngresses++
		}
		if entry.HasDefaultBackend {
			result.Summary.DefaultBackend++
		}

		// Namespace stats
		nsStat := getOrCreateIngressNs(nsMap, ing.Namespace)
		nsStat.Total++
		if entry.Status == "healthy" {
			nsStat.Healthy++
		}
		if entry.HasTLS {
			nsStat.WithTLS++
		}
		nsStat.Issues += issueCount
	}

	// Check host+path conflicts
	for hpKey, owners := range hostPathMap {
		if len(owners) > 1 {
			parts := strings.SplitN(hpKey, "|", 2)
			host, path := parts[0], parts[1]
			msg := fmt.Sprintf("host %q path %q claimed by %d ingresses: %s", host, path, len(owners), strings.Join(owners, ", "))
			result.Issues = append(result.Issues, IngressIssue{
				Severity: "warning",
				Type:     "conflict",
				Message:  msg,
			})
			result.Summary.HostConflicts++
		}
	}

	// Sort issues by severity
	sort.Slice(result.Issues, func(i, j int) bool {
		return ingressSeverityRank(result.Issues[i].Severity) < ingressSeverityRank(result.Issues[j].Severity)
	})

	// Build namespace stats
	for _, nsStat := range nsMap {
		result.ByNamespace = append(result.ByNamespace, *nsStat)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].Issues > result.ByNamespace[j].Issues
	})

	// Calculate health score
	result.Summary.HealthScore = calculateIngressScore(result.Summary)

	// Recommendations
	result.Recommendations = generateIngressRecommendations(result)

	writeJSON(w, result)
}

// assessIngressStatus determines overall status.
func assessIngressStatus(issueCount int, issues []IngressIssue) string {
	if issueCount == 0 {
		return "healthy"
	}
	for _, iss := range issues {
		if iss.Severity == "critical" {
			return "critical"
		}
	}
	return "warning"
}

// calculateIngressScore computes 0-100.
func calculateIngressScore(s IngressSummary) int {
	if s.TotalIngresses == 0 {
		return 100
	}
	score := 100
	score -= s.NoBackend * 15
	score -= s.NoRules * 5
	score -= s.MissingClass * 5
	score -= s.HostConflicts * 8
	score -= s.IngressNoTLS * 2
	if score < 0 {
		score = 0
	}
	return score
}

// generateIngressRecommendations produces actionable advice.
func generateIngressRecommendations(result IngressHealthResult) []string {
	var recs []string
	s := result.Summary

	if s.NoBackend > 0 {
		recs = append(recs, fmt.Sprintf("%d ingress(es) reference non-existent backend services — fix or remove stale rules", s.NoBackend))
	}
	if s.HostConflicts > 0 {
		recs = append(recs, fmt.Sprintf("%d host+path conflict(s) detected — multiple ingresses claim the same route, only one will receive traffic", s.HostConflicts))
	}
	if s.MissingClass > 0 {
		recs = append(recs, fmt.Sprintf("%d ingress(es) have no valid IngressClass — traffic will not be routed", s.MissingClass))
	}
	if s.IngressNoTLS > 0 {
		recs = append(recs, fmt.Sprintf("%d ingress(es) have no TLS — add TLS certificates for encrypted traffic", s.IngressNoTLS))
	}
	if s.NoRules > 0 {
		recs = append(recs, fmt.Sprintf("%d ingress(es) have no rules — add routing rules or a default backend", s.NoRules))
	}
	if s.HealthScore < 60 {
		recs = append(recs, fmt.Sprintf("Ingress health score is %d/100 — review routing configuration", s.HealthScore))
	}

	return recs
}

func getOrCreateIngressNs(m map[string]*IngressNsStat, ns string) *IngressNsStat {
	if e, ok := m[ns]; ok {
		return e
	}
	e := &IngressNsStat{Namespace: ns}
	m[ns] = e
	return e
}

func ingressSeverityRank(s string) int {
	switch s {
	case "critical":
		return 0
	case "warning":
		return 1
	default:
		return 2
	}
}

// Ensure imports are used.
var _ = networkingv1.IngressSpec{}
var _ = corev1.ServiceSpec{}
