package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EEResult is the service endpoint exposure & attack surface analysis.
type EEResult struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	Summary         EESummary        `json:"summary"`
	ExposedServices []EEEntry        `json:"exposedServices"`
	IngressRoutes   []EEIngressEntry `json:"ingressRoutes"`
	InternalOnly    []EEEntry        `json:"internalOnly"`
	ByNamespace     []EENSEntry      `json:"byNamespace"`
	Issues          []EEIssue        `json:"issues"`
	Recommendations []string         `json:"recommendations"`
}

// EESummary aggregates endpoint exposure.
type EESummary struct {
	TotalServices      int `json:"totalServices"`
	ExposedExternal    int `json:"exposedExternal"` // LoadBalancer, NodePort, ExternalIP
	InternalOnly       int `json:"internalOnly"`    // ClusterIP only
	TotalIngress       int `json:"totalIngress"`
	IngressWithTLS     int `json:"ingressWithTLS"`
	IngressNoTLS       int `json:"ingressNoTLS"` // HTTP only, no TLS
	NodePorts          int `json:"nodePorts"`
	LoadBalancers      int `json:"loadBalancers"`
	ExternalIPs        int `json:"externalIPs"`
	HTTPSOnly          int `json:"httpsOnly"`          // Services with only HTTPS port
	HTTPOnly           int `json:"httpOnly"`           // Services with only HTTP port
	NoNetworkPolicy    int `json:"noNetworkPolicy"`    // Exposed services without NP
	AttackSurfaceScore int `json:"attackSurfaceScore"` // 0-100 (higher = safer, less exposure)
}

// EEEntry describes one service's exposure.
type EEEntry struct {
	Name             string   `json:"name"`
	Namespace        string   `json:"namespace"`
	Type             string   `json:"type"` // ClusterIP / NodePort / LoadBalancer
	ClusterIP        string   `json:"clusterIP"`
	ExternalIPs      []string `json:"externalIPs,omitempty"`
	Ports            []EEPort `json:"ports"`
	HasNetworkPolicy bool     `json:"hasNetworkPolicy"`
	HasSelector      bool     `json:"hasSelector"`  // false = external endpoints
	ExposedLevel     string   `json:"exposedLevel"` // public / node / internal
	RiskLevel        string   `json:"riskLevel"`
}

// EEPort describes a service port.
type EEPort struct {
	Name       string `json:"name"`
	Port       int32  `json:"port"`
	TargetPort string `json:"targetPort"`
	NodePort   int32  `json:"nodePort,omitempty"`
	Protocol   string `json:"protocol"`
	IsHTTP     bool   `json:"isHTTP"`
	IsHTTPS    bool   `json:"isHTTPS"`
}

// EEIngressEntry describes one ingress route.
type EEIngressEntry struct {
	Name       string   `json:"name"`
	Namespace  string   `json:"namespace"`
	Hosts      []string `json:"hosts"`
	HasTLS     bool     `json:"hasTLS"`
	Backend    string   `json:"backend"`
	HTTPRoutes int      `json:"httpRoutes"`
	TLSRoutes  int      `json:"tlsRoutes"`
	RiskLevel  string   `json:"riskLevel"`
}

// EENSEntry per-namespace exposure stats.
type EENSEntry struct {
	Namespace    string `json:"namespace"`
	ServiceCount int    `json:"serviceCount"`
	ExposedCount int    `json:"exposedCount"`
	IngressCount int    `json:"ingressCount"`
	NoTLSIngress int    `json:"noTLSIngress"`
	RiskLevel    string `json:"riskLevel"`
}

// EEIssue is a detected exposure problem.
type EEIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleEndpointExposure audits service endpoint exposure and attack surface.
// GET /api/security/endpoint-exposure
func (s *Server) handleEndpointExposure(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")
	services, err := rc.clientset.CoreV1().Services(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	ingresses, err := rc.clientset.NetworkingV1().Ingresses(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Get NetworkPolicies to check coverage
	policies, err := rc.clientset.NetworkingV1().NetworkPolicies(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Build namespace → has NP map
	nsHasNP := make(map[string]bool)
	for _, np := range policies.Items {
		nsHasNP[np.Namespace] = true
	}

	result := EEResult{ScannedAt: time.Now()}
	nsMap := make(map[string]*EENSEntry)

	for _, svc := range services.Items {
		result.Summary.TotalServices++

		entry := EEEntry{
			Name:        svc.Name,
			Namespace:   svc.Namespace,
			Type:        string(svc.Spec.Type),
			ClusterIP:   svc.Spec.ClusterIP,
			ExternalIPs: svc.Spec.ExternalIPs,
			HasSelector: len(svc.Spec.Selector) > 0,
		}

		// Has NetworkPolicy in namespace?
		entry.HasNetworkPolicy = nsHasNP[svc.Namespace]

		// Determine exposure level
		entry.ExposedLevel = eeExposedLevel(svc)

		// Ports
		for _, p := range svc.Spec.Ports {
			port := EEPort{
				Name:       p.Name,
				Port:       p.Port,
				TargetPort: p.TargetPort.String(),
				Protocol:   string(p.Protocol),
				NodePort:   p.NodePort,
			}
			if p.Port == 80 || p.Port == 8080 || p.Port == 3000 || p.Port == 5000 {
				port.IsHTTP = true
			}
			if p.Port == 443 || p.Port == 8443 {
				port.IsHTTPS = true
			}
			entry.Ports = append(entry.Ports, port)
		}

		// Risk assessment
		entry.RiskLevel = eeAssessRisk(entry)

		// Aggregate
		switch svc.Spec.Type {
		case corev1.ServiceTypeNodePort:
			result.Summary.NodePorts++
			result.Summary.ExposedExternal++
		case corev1.ServiceTypeLoadBalancer:
			result.Summary.LoadBalancers++
			result.Summary.ExposedExternal++
		}
		if len(svc.Spec.ExternalIPs) > 0 {
			result.Summary.ExternalIPs++
			result.Summary.ExposedExternal++
		}
		if entry.ExposedLevel == "internal" {
			result.Summary.InternalOnly++
			result.InternalOnly = append(result.InternalOnly, entry)
		} else {
			result.ExposedServices = append(result.ExposedServices, entry)

			// Check for missing NetworkPolicy on exposed services
			if !entry.HasNetworkPolicy {
				result.Summary.NoNetworkPolicy++
				result.Issues = append(result.Issues, EEIssue{
					Severity: "critical", Type: "exposed-no-netpol",
					Resource: fmt.Sprintf("%s/%s", svc.Namespace, svc.Name),
					Message:  fmt.Sprintf("Service %s/%s is %s but namespace has NO NetworkPolicy — unrestricted access", svc.Namespace, svc.Name, entry.ExposedLevel),
				})
			}
		}

		// Namespace tracking
		nsStat := eeGetOrCreateNS(nsMap, svc.Namespace)
		nsStat.ServiceCount++
		if entry.ExposedLevel != "internal" {
			nsStat.ExposedCount++
		}
	}

	// Ingress analysis
	for _, ing := range ingresses.Items {
		result.Summary.TotalIngress++

		entry := EEIngressEntry{
			Name:      ing.Name,
			Namespace: ing.Namespace,
			Backend:   eeIngressBackend(ing),
		}

		// Hosts
		for _, rule := range ing.Spec.Rules {
			if rule.Host != "" {
				entry.Hosts = append(entry.Hosts, rule.Host)
			} else {
				entry.Hosts = append(entry.Hosts, "*")
			}
			if rule.HTTP != nil {
				entry.HTTPRoutes += len(rule.HTTP.Paths)
			}
		}

		// TLS
		hasTLS := len(ing.Spec.TLS) > 0
		entry.HasTLS = hasTLS
		if hasTLS {
			result.Summary.IngressWithTLS++
			entry.TLSRoutes = entry.HTTPRoutes
			entry.HTTPRoutes = 0
		} else {
			result.Summary.IngressNoTLS++
			nsStat := eeGetOrCreateNS(nsMap, ing.Namespace)
			nsStat.IngressCount++
			nsStat.NoTLSIngress++
			result.Issues = append(result.Issues, EEIssue{
				Severity: "warning", Type: "ingress-no-tls",
				Resource: fmt.Sprintf("%s/%s", ing.Namespace, ing.Name),
				Message:  fmt.Sprintf("Ingress %s/%s has NO TLS — traffic sent in plaintext", ing.Namespace, ing.Name),
			})
		}

		entry.RiskLevel = eeIngressRisk(entry)
		result.IngressRoutes = append(result.IngressRoutes, entry)

		nsStat := eeGetOrCreateNS(nsMap, ing.Namespace)
		nsStat.IngressCount++
	}

	// Finalize namespace stats
	for _, nsStat := range nsMap {
		nsStat.RiskLevel = eeNSRisk(*nsStat)
		result.ByNamespace = append(result.ByNamespace, *nsStat)
	}

	// Sort
	sort.Slice(result.ExposedServices, func(i, j int) bool {
		return eeRiskRank(result.ExposedServices[i].RiskLevel) < eeRiskRank(result.ExposedServices[j].RiskLevel)
	})
	sort.Slice(result.IngressRoutes, func(i, j int) bool {
		return eeRiskRank(result.IngressRoutes[i].RiskLevel) < eeRiskRank(result.IngressRoutes[j].RiskLevel)
	})
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].ExposedCount > result.ByNamespace[j].ExposedCount
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return eeIssueRank(result.Issues[i].Severity) < eeIssueRank(result.Issues[j].Severity)
	})

	result.Summary.AttackSurfaceScore = eeScore(result.Summary)
	result.Recommendations = eeGenRecs(result.Summary, result.ExposedServices, result.IngressRoutes)

	writeJSON(w, result)
}

// eeExposedLevel determines how exposed a service is.
func eeExposedLevel(svc corev1.Service) string {
	if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
		return "public"
	}
	if svc.Spec.Type == corev1.ServiceTypeNodePort {
		return "node"
	}
	if len(svc.Spec.ExternalIPs) > 0 {
		return "public"
	}
	if strings.HasSuffix(svc.Spec.ClusterIP, ".svc.cluster.local") || svc.Spec.ClusterIP == "None" {
		return "internal"
	}
	if svc.Spec.ClusterIP != "" && svc.Spec.Type == corev1.ServiceTypeClusterIP {
		return "internal"
	}
	return "internal"
}

// eeAssessRisk determines risk for a service.
func eeAssessRisk(entry EEEntry) string {
	switch entry.ExposedLevel {
	case "public":
		if !entry.HasNetworkPolicy {
			return "critical"
		}
		return "high"
	case "node":
		if !entry.HasNetworkPolicy {
			return "high"
		}
		return "medium"
	default:
		return "low"
	}
}

// eeIngressRisk determines risk for an ingress.
func eeIngressRisk(entry EEIngressEntry) string {
	if !entry.HasTLS {
		return "high"
	}
	return "low"
}

// eeIngressBackend extracts backend service name.
func eeIngressBackend(ing netv1.Ingress) string {
	if ing.Spec.DefaultBackend != nil && ing.Spec.DefaultBackend.Service != nil {
		return ing.Spec.DefaultBackend.Service.Name
	}
	for _, rule := range ing.Spec.Rules {
		if rule.HTTP != nil {
			for _, path := range rule.HTTP.Paths {
				if path.Backend.Service != nil {
					return path.Backend.Service.Name
				}
			}
		}
	}
	return ""
}

// eeNSRisk determines namespace risk.
func eeNSRisk(ns EENSEntry) string {
	if ns.ExposedCount > 3 && ns.NoTLSIngress > 0 {
		return "critical"
	}
	if ns.ExposedCount > 0 && ns.NoTLSIngress > 0 {
		return "high"
	}
	if ns.ExposedCount > 0 {
		return "medium"
	}
	return "low"
}

// eeScore computes 0-100 (higher = safer/less exposed).
func eeScore(s EESummary) int {
	if s.TotalServices == 0 {
		return 100
	}
	score := 100
	score -= s.ExposedExternal * 5
	score -= s.NodePorts * 3
	score -= s.IngressNoTLS * 8
	score -= s.NoNetworkPolicy * 6
	if score < 0 {
		score = 0
	}
	return score
}

// eeGenRecs produces actionable advice.
func eeGenRecs(s EESummary, exposed []EEEntry, ingress []EEIngressEntry) []string {
	var recs []string

	if s.ExposedExternal > 0 {
		recs = append(recs, fmt.Sprintf("%d service(s) externally exposed (%d LoadBalancer, %d NodePort) — minimize attack surface", s.ExposedExternal, s.LoadBalancers, s.NodePorts))
	}
	if s.IngressNoTLS > 0 {
		top := ""
		if len(ingress) > 0 {
			top = fmt.Sprintf(" (e.g. %s/%s)", ingress[0].Namespace, ingress[0].Name)
		}
		recs = append(recs, fmt.Sprintf("%d ingress route(s) have NO TLS%s — add TLS certificates immediately", s.IngressNoTLS, top))
	}
	if s.NoNetworkPolicy > 0 {
		recs = append(recs, fmt.Sprintf("%d exposed service(s) have NO NetworkPolicy — restrict ingress traffic with NetworkPolicy", s.NoNetworkPolicy))
	}
	if s.NodePorts > 0 {
		recs = append(recs, fmt.Sprintf("%d NodePort service(s) expose ports on all nodes — prefer LoadBalancer with source IP restrictions", s.NodePorts))
	}
	if s.ExternalIPs > 0 {
		recs = append(recs, fmt.Sprintf("%d service(s) with manually-set ExternalIPs — verify these are behind a proper firewall", s.ExternalIPs))
	}
	if s.AttackSurfaceScore < 50 {
		recs = append(recs, fmt.Sprintf("Attack surface score is %d/100 — many exposed endpoints without protection", s.AttackSurfaceScore))
	}
	if s.ExposedExternal == 0 && s.IngressNoTLS == 0 {
		recs = append(recs, "No externally exposed services or plaintext ingress — minimal attack surface")
	}

	return recs
}

func eeGetOrCreateNS(m map[string]*EENSEntry, ns string) *EENSEntry {
	if e, ok := m[ns]; ok {
		return e
	}
	e := &EENSEntry{Namespace: ns}
	m[ns] = e
	return e
}

func eeRiskRank(level string) int {
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

func eeIssueRank(s string) int {
	switch s {
	case "critical":
		return 0
	case "warning":
		return 1
	case "info":
		return 2
	default:
		return 3
	}
}
