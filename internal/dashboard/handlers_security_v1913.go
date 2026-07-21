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

// ============================================================
// v19.13 — Security Dimension (Round 5)
// 1. DNS Exfiltration Risk
// 2. Port Forward Audit
// 3. Image Supply Chain Provenance
// ============================================================

// ---------------------------------------------------------------
// 1. DNS Exfiltration Risk — detect potential data exfil via DNS
// ---------------------------------------------------------------

type DNSExfilResult struct {
	ScannedAt        time.Time         `json:"scannedAt"`
	HealthScore      int               `json:"healthScore"`
	Grade            string            `json:"grade"`
	Summary          DNSExfilSummary   `json:"summary"`
	HighRiskNS       []DNSExfilEntry   `json:"highRiskNamespaces"`
	SuspiciousEgress []DNSExfilEntry   `json:"suspiciousEgress"`
	ByNamespace      []DNSExfilNSEntry `json:"byNamespace"`
	Recommendations  []string          `json:"recommendations"`
}

type DNSExfilSummary struct {
	TotalWorkloads    int `json:"totalWorkloads"`
	WithEgressDNS     int `json:"withEgressDNS"`
	WithoutNetPolicy  int `json:"withoutNetPolicy"`
	UnrestrictedDNS   int `json:"unrestrictedDNS"`
	SuspiciousEnvs    int `json:"suspiciousEnvs"`
	HighRiskWorkloads int `json:"highRiskWorkloads"`
}

type DNSExfilEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	RiskLevel string `json:"riskLevel"`
	Issue     string `json:"issue"`
}

type DNSExfilNSEntry struct {
	Namespace    string `json:"namespace"`
	Workloads    int    `json:"workloads"`
	Unrestricted int    `json:"unrestricted"`
	RiskLevel    string `json:"riskLevel"`
}

func (s *Server) handleDNSExfilRisk(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := DNSExfilResult{ScannedAt: time.Now()}

	// Check which namespaces have NetworkPolicy covering DNS egress
	netpols, _ := rc.clientset.NetworkingV1().NetworkPolicies("").List(ctx, metav1.ListOptions{})
	nsHasDNSPolicy := map[string]bool{}
	for _, np := range netpols.Items {
		if isSystemNamespace(np.Namespace) {
			continue
		}
		// Check if policy restricts DNS (port 53) egress
		for _, rule := range np.Spec.Egress {
			for _, port := range rule.Ports {
				if port.Port != nil && (port.Port.IntVal == 53 || port.Port.StrVal == "domain") {
					nsHasDNSPolicy[np.Namespace] = true
				}
			}
		}
	}

	nsMap := map[string]*DNSExfilNSEntry{}

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++
		nsE, ok := nsMap[dep.Namespace]
		if !ok {
			nsE = &DNSExfilNSEntry{Namespace: dep.Namespace}
			nsMap[dep.Namespace] = nsE
		}
		nsE.Workloads++

		hasDNSRisk := false
		entry := DNSExfilEntry{Name: dep.Name, Namespace: dep.Namespace}

		// Check for suspicious env vars that might contain exfil targets
		for _, c := range dep.Spec.Template.Spec.Containers {
			for _, env := range c.Env {
				if env.Value == "" {
					continue
				}
				envNameLower := strings.ToLower(env.Name)
				// Look for webhook URLs, external endpoints
				if strings.Contains(envNameLower, "webhook") || strings.Contains(envNameLower, "callback") {
					result.Summary.SuspiciousEnvs++
					hasDNSRisk = true
					entry.Issue = "has webhook/callback URL in env vars"
					entry.RiskLevel = "medium"
				}
				// Look for base64-encoded data that could be exfil payloads
				if len(env.Value) > 200 && strings.HasPrefix(env.Value, "http") {
					if hasDNSRisk {
						entry.Issue += "; long URL in env var"
					} else {
						entry.Issue = "long URL in env var (potential data exfil channel)"
						hasDNSRisk = true
					}
					entry.RiskLevel = "medium"
				}
			}
		}

		// Check if namespace lacks DNS NetworkPolicy
		if !nsHasDNSPolicy[dep.Namespace] {
			result.Summary.WithoutNetPolicy++
			result.Summary.UnrestrictedDNS++
			nsE.Unrestricted++
			if entry.Issue == "" {
				entry.Issue = "no DNS egress NetworkPolicy - unrestricted outbound DNS"
				entry.RiskLevel = "high"
			} else {
				entry.Issue += "; no DNS egress restriction"
				if entry.RiskLevel == "medium" {
					entry.RiskLevel = "high"
				}
			}
			hasDNSRisk = true
		}

		if hasDNSRisk {
			result.Summary.HighRiskWorkloads++
			result.HighRiskNS = append(result.HighRiskNS, entry)
		}

		// Check for DNS config in pod spec
		if dep.Spec.Template.Spec.DNSPolicy == corev1.DNSClusterFirst ||
			dep.Spec.Template.Spec.DNSPolicy == corev1.DNSDefault {
			result.Summary.WithEgressDNS++
		}
	}

	for _, ns := range nsMap {
		if ns.Unrestricted > 0 {
			if ns.Unrestricted == ns.Workloads {
				ns.RiskLevel = "high"
			} else {
				ns.RiskLevel = "medium"
			}
		} else {
			ns.RiskLevel = "low"
		}
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].Unrestricted > result.ByNamespace[j].Unrestricted
	})

	// Score
	if result.Summary.TotalWorkloads > 0 {
		restrictedPct := (result.Summary.TotalWorkloads - result.Summary.HighRiskWorkloads) * 100 / result.Summary.TotalWorkloads
		result.HealthScore = restrictedPct
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildDNSExfilRecs1913(&result)
	writeJSON(w, result)
}

func buildDNSExfilRecs1913(r *DNSExfilResult) []string {
	recs := []string{fmt.Sprintf("DNS exfil risk: %d workloads, %d high-risk (%d without DNS NetworkPolicy, %d suspicious env vars)",
		r.Summary.TotalWorkloads, r.Summary.HighRiskWorkloads,
		r.Summary.WithoutNetPolicy, r.Summary.SuspiciousEnvs)}
	if r.Summary.WithoutNetPolicy > 0 {
		recs = append(recs, fmt.Sprintf("%d workloads without DNS egress restrictions - add NetworkPolicy to limit outbound DNS", r.Summary.WithoutNetPolicy))
	}
	return recs
}

// ---------------------------------------------------------------
// 2. Port Forward Audit — detect port-forward sessions & risk
// ---------------------------------------------------------------

type PortForwardResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         PortForwardSummary   `json:"summary"`
	ExposedPods     []PortForwardEntry   `json:"exposedPods"`
	HighRiskPorts   []PortForwardEntry   `json:"highRiskPorts"`
	ByNamespace     []PortForwardNSEntry `json:"byNamespace"`
	Recommendations []string             `json:"recommendations"`
}

type PortForwardSummary struct {
	TotalContainers  int `json:"totalContainers"`
	HostPortCount    int `json:"hostPortCount"`
	HighRiskPorts    int `json:"highRiskPorts"`
	NodePortSvcs     int `json:"nodePortServices"`
	LoadBalancerSvcs int `json:"loadBalancerServices"`
	PrivilegedPorts  int `json:"privilegedPorts"`
}

type PortForwardEntry struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	Port         int32  `json:"port"`
	Protocol     string `json:"protocol"`
	ExposureType string `json:"exposureType"`
	RiskLevel    string `json:"riskLevel"`
}

type PortForwardNSEntry struct {
	Namespace string `json:"namespace"`
	HostPorts int    `json:"hostPorts"`
	NodePorts int    `json:"nodePorts"`
	RiskLevel string `json:"riskLevel"`
}

func (s *Server) handlePortForwardAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := PortForwardResult{ScannedAt: time.Now()}

	// Known high-risk ports
	highRiskPorts := map[int32]string{
		22:    "SSH",
		2375:  "Docker API (unencrypted)",
		2376:  "Docker API",
		4422:  "SSH alternate",
		5000:  "Registry",
		6379:  "Redis",
		9200:  "Elasticsearch",
		11211: "Memcached",
		27017: "MongoDB",
	}

	nsMap := map[string]*PortForwardNSEntry{}

	// Check container hostPort usage
	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		for _, c := range dep.Spec.Template.Spec.Containers {
			result.Summary.TotalContainers++
			for _, p := range c.Ports {
				if p.HostPort > 0 {
					result.Summary.HostPortCount++
					proto := string(p.Protocol)
					if proto == "" {
						proto = "TCP"
					}
					riskLevel := "medium"
					if desc, isHigh := highRiskPorts[p.HostPort]; isHigh {
						result.Summary.HighRiskPorts++
						riskLevel = "high"
						result.HighRiskPorts = append(result.HighRiskPorts, PortForwardEntry{
							Name: dep.Name, Namespace: dep.Namespace,
							Port: p.HostPort, Protocol: proto,
							ExposureType: fmt.Sprintf("hostPort (%s)", desc),
							RiskLevel:    riskLevel,
						})
					}
					if p.HostPort < 1024 {
						result.Summary.PrivilegedPorts++
					}
					nsE, ok := nsMap[dep.Namespace]
					if !ok {
						nsE = &PortForwardNSEntry{Namespace: dep.Namespace}
						nsMap[dep.Namespace] = nsE
					}
					nsE.HostPorts++
				}
			}
		}
	}

	// Check services for NodePort/LoadBalancer exposure
	svcs, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	for _, svc := range svcs.Items {
		if isSystemNamespace(svc.Namespace) {
			continue
		}
		if svc.Spec.Type == corev1.ServiceTypeNodePort {
			result.Summary.NodePortSvcs++
			for _, port := range svc.Spec.Ports {
				if port.NodePort > 0 {
					nsE, ok := nsMap[svc.Namespace]
					if !ok {
						nsE = &PortForwardNSEntry{Namespace: svc.Namespace}
						nsMap[svc.Namespace] = nsE
					}
					nsE.NodePorts++
					riskLevel := "medium"
					if desc, isHigh := highRiskPorts[port.Port]; isHigh {
						riskLevel = "high"
						result.HighRiskPorts = append(result.HighRiskPorts, PortForwardEntry{
							Name: svc.Name, Namespace: svc.Namespace,
							Port: port.NodePort, Protocol: string(port.Protocol),
							ExposureType: fmt.Sprintf("nodePort (%s)", desc),
							RiskLevel:    riskLevel,
						})
					}
				}
			}
		}
		if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
			result.Summary.LoadBalancerSvcs++
		}
	}

	for _, ns := range nsMap {
		if ns.HostPorts > 0 {
			ns.RiskLevel = "high"
		} else if ns.NodePorts > 3 {
			ns.RiskLevel = "medium"
		} else {
			ns.RiskLevel = "low"
		}
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].HostPorts > result.ByNamespace[j].HostPorts
	})

	// Score
	if result.Summary.TotalContainers > 0 {
		exposureRate := (result.Summary.HostPortCount + result.Summary.NodePortSvcs) * 100 / result.Summary.TotalContainers
		result.HealthScore = 100 - exposureRate
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildPortForwardRecs1913(&result)
	writeJSON(w, result)
}

func buildPortForwardRecs1913(r *PortForwardResult) []string {
	recs := []string{fmt.Sprintf("Port forward audit: %d hostPorts, %d nodePort services, %d loadBalancers, %d high-risk ports exposed",
		r.Summary.HostPortCount, r.Summary.NodePortSvcs, r.Summary.LoadBalancerSvcs, r.Summary.HighRiskPorts)}
	if r.Summary.HighRiskPorts > 0 {
		recs = append(recs, fmt.Sprintf("%d high-risk ports (SSH, Redis, DB) exposed via hostPort/nodePort - restrict access", r.Summary.HighRiskPorts))
	}
	return recs
}

// ---------------------------------------------------------------
// 3. Image Supply Chain Provenance — registry trust & signing
// ---------------------------------------------------------------

type ImageProvenanceResult1913 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         ImageProvSummary1913    `json:"summary"`
	ByRegistry      []ImageProvRegistry1913 `json:"byRegistry"`
	UnsignedImages  []ImageProvEntry1913    `json:"unsignedImages"`
	UntrustedReg    []ImageProvEntry1913    `json:"untrustedRegistries"`
	Recommendations []string                `json:"recommendations"`
}

type ImageProvSummary1913 struct {
	TotalContainers   int `json:"totalContainers"`
	TrustedRegistries int `json:"trustedRegistries"`
	UntrustedRegistry int `json:"untrustedRegistry"`
	PinnedImages      int `json:"pinnedImages"`
	HasImagePolicy    int `json:"hasImagePolicy"`
	ByDigestCount     int `json:"byDigestCount"`
}

type ImageProvRegistry1913 struct {
	Registry  string `json:"registry"`
	Count     int    `json:"count"`
	Trusted   bool   `json:"trusted"`
	RiskLevel string `json:"riskLevel"`
}

type ImageProvEntry1913 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Image     string `json:"image"`
	Issue     string `json:"issue"`
	RiskLevel string `json:"riskLevel"`
}

func (s *Server) handleImageProvenance1913(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := ImageProvenanceResult1913{ScannedAt: time.Now()}

	// Known trusted registries
	trustedRegistries := map[string]bool{
		"registry.k8s.io":   true,
		"k8s.gcr.io":        true,
		"gcr.io":            true,
		"ghcr.io":           true,
		"quay.io":           true,
		"docker.io":         true,
		"registry.iot2.win": true,
	}

	// Check for admission webhook that enforces image policy
	webhookList, _ := rc.clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().List(ctx, metav1.ListOptions{})
	for _, wh := range webhookList.Items {
		if strings.Contains(strings.ToLower(wh.Name), "image") || strings.Contains(strings.ToLower(wh.Name), "cosign") ||
			strings.Contains(strings.ToLower(wh.Name), "policy") {
			result.Summary.HasImagePolicy++
		}
	}

	registryCount := map[string]int{}

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		for _, c := range dep.Spec.Template.Spec.Containers {
			result.Summary.TotalContainers++
			image := c.Image

			// Extract registry
			registry := "docker.io"
			if strings.Contains(image, "/") {
				parts := strings.SplitN(image, "/", 2)
				if strings.Contains(parts[0], ".") || strings.Contains(parts[0], ":") {
					registry = parts[0]
				}
			}
			registryCount[registry]++

			// Check if pinned by digest
			if strings.Contains(image, "@sha256:") {
				result.Summary.ByDigestCount++
				result.Summary.PinnedImages++
			} else if strings.Contains(image[strings.LastIndex(image, "/")+1:], ":") &&
				!strings.HasSuffix(image, ":latest") {
				result.Summary.PinnedImages++
			} else {
				result.UnsignedImages = append(result.UnsignedImages, ImageProvEntry1913{
					Name: dep.Name, Namespace: dep.Namespace,
					Image:     image,
					Issue:     "unpinned image (no digest or specific tag)",
					RiskLevel: "medium",
				})
			}

			// Check if untrusted registry
			if !trustedRegistries[registry] {
				result.Summary.UntrustedRegistry++
				result.UntrustedReg = append(result.UntrustedReg, ImageProvEntry1913{
					Name: dep.Name, Namespace: dep.Namespace,
					Image:     image,
					Issue:     fmt.Sprintf("image from untrusted registry: %s", registry),
					RiskLevel: "high",
				})
			}
		}
	}

	// Build registry summary
	for reg, count := range registryCount {
		entry := ImageProvRegistry1913{
			Registry: reg, Count: count,
			Trusted: trustedRegistries[reg],
		}
		if entry.Trusted {
			entry.RiskLevel = "low"
			result.Summary.TrustedRegistries++
		} else {
			entry.RiskLevel = "high"
		}
		result.ByRegistry = append(result.ByRegistry, entry)
	}
	sort.Slice(result.ByRegistry, func(i, j int) bool {
		return result.ByRegistry[i].Count > result.ByRegistry[j].Count
	})

	// Score
	if result.Summary.TotalContainers > 0 {
		trustedPct := (result.Summary.TotalContainers - result.Summary.UntrustedRegistry) * 100 / result.Summary.TotalContainers
		pinnedPct := result.Summary.PinnedImages * 100 / result.Summary.TotalContainers
		result.HealthScore = (trustedPct + pinnedPct) / 2
	} else {
		result.HealthScore = 100
	}
	if result.Summary.HasImagePolicy == 0 {
		result.HealthScore -= 10
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildImageProvRecs1913(&result)
	writeJSON(w, result)
}

func buildImageProvRecs1913(r *ImageProvenanceResult1913) []string {
	recs := []string{fmt.Sprintf("Image provenance: %d containers, %d trusted registries, %d untrusted, %d pinned (%d by digest), policy: %d",
		r.Summary.TotalContainers, r.Summary.TrustedRegistries,
		r.Summary.UntrustedRegistry, r.Summary.PinnedImages,
		r.Summary.ByDigestCount, r.Summary.HasImagePolicy)}
	if r.Summary.UntrustedRegistry > 0 {
		recs = append(recs, fmt.Sprintf("%d images from untrusted registries - restrict to approved registries only", r.Summary.UntrustedRegistry))
	}
	if r.Summary.HasImagePolicy == 0 {
		recs = append(recs, "No image policy admission webhook detected - install Cosign/Kyverno for image verification")
	}
	return recs
}
