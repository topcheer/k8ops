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

// EndpointDNSHealthResult is the service endpoint & DNS resolution health analysis.
type EndpointDNSHealthResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         EndpointDNSSummary  `json:"summary"`
	Services        []EndpointDNSEntry  `json:"services"`
	ByNamespace     []EndpointDNSNSStat `json:"byNamespace"`
	Issues          []EndpointDNSIssue  `json:"issues"`
	Recommendations []string            `json:"recommendations"`
	HealthScore     int                 `json:"healthScore"`
}

// EndpointDNSSummary aggregates service endpoint & DNS statistics.
type EndpointDNSSummary struct {
	TotalServices    int `json:"totalServices"`
	WithEndpoints    int `json:"withEndpoints"`
	NoEndpoints      int `json:"noEndpoints"`
	HeadlessServices int `json:"headlessServices"`
	ExternalName     int `json:"externalNameServices"`
	NoSelector       int `json:"noSelectorServices"`
	DeprecatedType   int `json:"deprecatedTypeWarning"`
	MultiPortNoName  int `json:"multiPortNoName"`
}

// EndpointDNSEntry describes one service's endpoint health.
type EndpointDNSEntry struct {
	Name           string `json:"name"`
	Namespace      string `json:"namespace"`
	Type           string `json:"type"`
	HasEndpoints   bool   `json:"hasEndpoints"`
	EndpointCount  int    `json:"endpointCount"`
	ReadyEndpoints int    `json:"readyEndpoints"`
	IsHeadless     bool   `json:"isHeadless"`
	HasSelector    bool   `json:"hasSelector"`
	PortCount      int    `json:"portCount"`
	PortsNamed     bool   `json:"portsNamed"`
	ClusterIP      string `json:"clusterIP"`
	Age            string `json:"age"`
	RiskLevel      string `json:"riskLevel"`
}

// EndpointDNSNSStat per-namespace stats.
type EndpointDNSNSStat struct {
	Namespace    string `json:"namespace"`
	ServiceCount int    `json:"serviceCount"`
	NoEndpoints  int    `json:"noEndpoints"`
}

// EndpointDNSIssue is a detected service endpoint problem.
type EndpointDNSIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleEndpointDNSHealth audits service endpoints & DNS resolution health.
// GET /api/product/endpoint-dns-health
func (s *Server) handleEndpointDNSHealth(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	ctx := r.Context()

	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := &EndpointDNSHealthResult{
		ScannedAt: time.Now(),
	}

	services, err := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	endpoints, err := rc.clientset.CoreV1().Endpoints("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Build endpoints map: namespace/name -> ready count
	endpointMap := make(map[string]int)
	for i := range endpoints.Items {
		ep := &endpoints.Items[i]
		key := fmt.Sprintf("%s/%s", ep.Namespace, ep.Name)
		readyCount := 0
		for _, subset := range ep.Subsets {
			readyCount += len(subset.Addresses)
		}
		endpointMap[key] = readyCount
	}

	var entries []EndpointDNSEntry
	var issues []EndpointDNSIssue
	nsStats := make(map[string]*EndpointDNSNSStat)

	noEndpoints := 0
	headlessCount := 0
	externalNameCount := 0
	noSelectorCount := 0
	multiPortNoName := 0

	for i := range services.Items {
		svc := &services.Items[i]
		if isSystemNamespace(svc.Namespace) {
			continue
		}

		entry := EndpointDNSEntry{
			Name:      svc.Name,
			Namespace: svc.Namespace,
			Type:      string(svc.Spec.Type),
			PortCount: len(svc.Spec.Ports),
		}

		// ClusterIP
		entry.ClusterIP = svc.Spec.ClusterIP
		if svc.Spec.ClusterIP == "None" {
			entry.IsHeadless = true
			headlessCount++
		}

		// ExternalName
		if svc.Spec.Type == corev1.ServiceTypeExternalName {
			externalNameCount++
		}

		// Selector
		entry.HasSelector = len(svc.Spec.Selector) > 0
		if !entry.HasSelector && svc.Spec.Type != corev1.ServiceTypeExternalName {
			noSelectorCount++
		}

		// Check endpoints
		key := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)
		readyCount := endpointMap[key]
		entry.EndpointCount = readyCount
		entry.ReadyEndpoints = readyCount
		entry.HasEndpoints = readyCount > 0

		if !entry.HasEndpoints && svc.Spec.Type != corev1.ServiceTypeExternalName && entry.HasSelector {
			noEndpoints++
			issues = append(issues, EndpointDNSIssue{
				Severity: "warning",
				Type:     "no-endpoints",
				Resource: key,
				Message:  fmt.Sprintf("Service %s has selector but no ready endpoints — pods may be down or not matching selector", svc.Name),
			})
		}

		// Check if multi-port service has unnamed ports
		if len(svc.Spec.Ports) > 1 {
			allNamed := true
			for _, p := range svc.Spec.Ports {
				if p.Name == "" {
					allNamed = false
					break
				}
			}
			entry.PortsNamed = allNamed
			if !allNamed {
				multiPortNoName++
				issues = append(issues, EndpointDNSIssue{
					Severity: "info",
					Type:     "unnamed-ports",
					Resource: key,
					Message:  fmt.Sprintf("Service %s has %d ports but some are unnamed — name ports for DNS SRV records and clarity", svc.Name, len(svc.Spec.Ports)),
				})
			}
		} else if len(svc.Spec.Ports) == 1 {
			entry.PortsNamed = svc.Spec.Ports[0].Name != ""
		}

		// Age
		entry.Age = time.Since(svc.CreationTimestamp.Time).Round(time.Hour).String()

		// Deprecated service type warning
		if svc.Spec.Type == corev1.ServiceTypeNodePort {
			// NodePort is not deprecated but has considerations
			if len(svc.Spec.Ports) > 0 && svc.Spec.Ports[0].NodePort > 30000 {
				// This is fine, just tracking
			}
		}

		entry.RiskLevel = assessEndpointDNSRisk(entry)
		entries = append(entries, entry)

		// Namespace stats
		if _, ok := nsStats[svc.Namespace]; !ok {
			nsStats[svc.Namespace] = &EndpointDNSNSStat{Namespace: svc.Namespace}
		}
		nsStats[svc.Namespace].ServiceCount++
		if !entry.HasEndpoints && entry.HasSelector {
			nsStats[svc.Namespace].NoEndpoints++
		}
	}

	// Convert namespace stats
	for _, ns := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].NoEndpoints > result.ByNamespace[j].NoEndpoints
	})

	sort.Slice(entries, func(i, j int) bool {
		riskOrder := map[string]int{"critical": 0, "warning": 1, "info": 2, "healthy": 3}
		return riskOrder[entries[i].RiskLevel] < riskOrder[entries[j].RiskLevel]
	})

	// Recommendations
	var recommendations []string
	if noEndpoints > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d service(s) have no ready endpoints — check pod health and selector matching", noEndpoints))
	}
	if noSelectorCount > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d service(s) have no selector — ensure endpoints are manually configured or add a selector", noSelectorCount))
	}
	if multiPortNoName > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d multi-port service(s) have unnamed ports — name ports for DNS SRV record support", multiPortNoName))
	}
	if len(recommendations) == 0 {
		recommendations = append(recommendations, "All service endpoints are healthy — DNS resolution should work correctly")
	}

	result.Services = entries
	result.Issues = issues
	result.Recommendations = recommendations
	result.Summary = EndpointDNSSummary{
		TotalServices:    len(entries),
		WithEndpoints:    len(entries) - noEndpoints,
		NoEndpoints:      noEndpoints,
		HeadlessServices: headlessCount,
		ExternalName:     externalNameCount,
		NoSelector:       noSelectorCount,
		MultiPortNoName:  multiPortNoName,
	}
	result.HealthScore = computeEndpointDNSScore(result.Summary, len(issues))

	writeJSON(w, result)
}

// assessEndpointDNSRisk determines risk level.
func assessEndpointDNSRisk(entry EndpointDNSEntry) string {
	if entry.HasSelector && !entry.HasEndpoints {
		return "warning"
	}
	if !entry.HasSelector && !entry.IsHeadless && entry.Type != "ExternalName" {
		return "info"
	}
	return "healthy"
}

// computeEndpointDNSScore computes a 0-100 health score.
func computeEndpointDNSScore(s EndpointDNSSummary, issueCount int) int {
	if s.TotalServices == 0 {
		return 100
	}
	score := 100
	score -= s.NoEndpoints * 5
	score -= s.NoSelector * 2
	score -= s.MultiPortNoName * 1
	score -= issueCount * 1
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}

// suppress unused
var _ = strings.TrimSpace
