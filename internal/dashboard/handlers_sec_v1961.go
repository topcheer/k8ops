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

// ============================================================
// v19.61 — Security Dimension (Round 13)
// 1. Linux Capabilities Audit — cap_add/cap_drop analysis & dangerous cap detector
// 2. Egress Traffic Audit — outbound traffic exposure & external endpoint mapping
// 3. Node Hardening Score — node security configuration & hardening posture
// ============================================================

// ---------------------------------------------------------------
// 1. Linux Capabilities Audit
// ---------------------------------------------------------------

type LinuxCapResult1961 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         LinuxCapSummary1961     `json:"summary"`
	DangerousCaps   []LinuxCapEntry1961     `json:"dangerousCaps"`
	AllContainers   []LinuxCapContainer1961 `json:"allContainers"`
	Recommendations []string                `json:"recommendations"`
}

type LinuxCapSummary1961 struct {
	TotalContainers   int `json:"totalContainers"`
	WithCapabilities  int `json:"withAddedCapabilities"`
	DroppingAll       int `json:"droppingAllCaps"`
	DangerousCapCount int `json:"dangerousCaps"`
	PrivilegedCount   int `json:"privilegedContainers"`
}

type LinuxCapEntry1961 struct {
	Container string   `json:"container"`
	Pod       string   `json:"pod"`
	Namespace string   `json:"namespace"`
	Caps      []string `json:"caps"`
	Severity  string   `json:"severity"`
}

type LinuxCapContainer1961 struct {
	Container string   `json:"container"`
	Pod       string   `json:"pod"`
	Namespace string   `json:"namespace"`
	Added     []string `json:"addedCaps"`
	Dropped   []string `json:"droppedCaps"`
	DropsAll  bool     `json:"dropsAllCaps"`
}

// dangerousCaps are capabilities that grant significant privileges
var dangerousCaps1961 = map[string]string{
	"SYS_ADMIN":       "high",
	"SYS_MODULE":      "high",
	"SYS_PTRACE":      "high",
	"SYS_RAWIO":       "critical",
	"NET_ADMIN":       "high",
	"NET_RAW":         "medium",
	"DAC_OVERRIDE":    "medium",
	"DAC_READ_SEARCH": "medium",
	"SETUID":          "medium",
	"SETGID":          "medium",
	"CHOWN":           "low",
	"FOWNER":          "low",
	"KILL":            "medium",
	"SETFCAP":         "high",
	"AUDIT_WRITE":     "low",
	"MKNOD":           "medium",
}

func (s *Server) handleLinuxCapAudit(w http.ResponseWriter, r *http.Request) {
	result := LinuxCapResult1961{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			entry := LinuxCapContainer1961{
				Container: c.Name, Pod: pod.Name, Namespace: pod.Namespace,
				Added: []string{}, Dropped: []string{},
			}

			// Check privileged
			if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
				result.Summary.PrivilegedCount++
				score -= 5
			}

			// Check capabilities
			if c.SecurityContext != nil && c.SecurityContext.Capabilities != nil {
				caps := c.SecurityContext.Capabilities
				if len(caps.Add) > 0 {
					result.Summary.WithCapabilities++
					entry.Added = make([]string, 0, len(caps.Add))
					for _, cap := range caps.Add {
						capStr := string(cap)
						entry.Added = append(entry.Added, capStr)

						// Check if dangerous
						if severity, ok := dangerousCaps1961[capStr]; ok {
							result.Summary.DangerousCapCount++
							result.DangerousCaps = append(result.DangerousCaps, LinuxCapEntry1961{
								Container: c.Name, Pod: pod.Name, Namespace: pod.Namespace,
								Caps: []string{capStr}, Severity: severity,
							})
							if severity == "critical" {
								score -= 10
							} else if severity == "high" {
								score -= 5
							} else if severity == "medium" {
								score -= 2
							}
						}
					}
				}
				if len(caps.Drop) > 0 {
					entry.Dropped = make([]string, 0, len(caps.Drop))
					for _, cap := range caps.Drop {
						entry.Dropped = append(entry.Dropped, string(cap))
					}
					// Check if dropping ALL
					for _, cap := range caps.Drop {
						if string(cap) == "ALL" {
							entry.DropsAll = true
							result.Summary.DroppingAll++
							break
						}
					}
				}
			}

			result.AllContainers = append(result.AllContainers, entry)
		}
	}

	// Sort dangerous caps by severity
	sort.Slice(result.DangerousCaps, func(i, j int) bool {
		sOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		return sOrder[result.DangerousCaps[i].Severity] < sOrder[result.DangerousCaps[j].Severity]
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.DangerousCapCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers with dangerous capabilities — remove if not needed", result.Summary.DangerousCapCount))
	}
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d/%d containers drop ALL capabilities", result.Summary.DroppingAll, result.Summary.TotalContainers))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Egress Traffic Audit
// ---------------------------------------------------------------

type EgressAuditResult1961 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         EgressAuditSummary1961 `json:"summary"`
	ExternalTargets []EgressTarget1961     `json:"externalTargets"`
	UnrestrictedNS  []string               `json:"unrestrictedNamespaces"`
	Recommendations []string               `json:"recommendations"`
}

type EgressAuditSummary1961 struct {
	TotalNamespaces  int `json:"totalNamespaces"`
	WithEgressPolicy int `json:"withEgressPolicy"`
	WithoutEgress    int `json:"withoutEgressPolicy"`
	TotalServices    int `json:"totalServices"`
	LoadBalancerSvc  int `json:"loadBalancerServices"`
	NodePortSvc      int `json:"nodePortServices"`
	ExternalIPCount  int `json:"externalIPCount"`
}

type EgressTarget1961 struct {
	Namespace string `json:"namespace"`
	Service   string `json:"service"`
	Type      string `json:"type"`
	Exposed   bool   `json:"externallyExposed"`
	Detail    string `json:"detail"`
}

func (s *Server) handleEgressTrafficAudit(w http.ResponseWriter, r *http.Request) {
	result := EgressAuditResult1961{ScannedAt: time.Now()}
	score := 100

	// Get namespaces
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	result.Summary.TotalNamespaces = len(nsList.Items)

	// Get NetworkPolicies to check egress coverage
	npList, _ := s.clientset.NetworkingV1().NetworkPolicies("").List(r.Context(), metav1.ListOptions{})
	nsWithEgressPolicy := make(map[string]bool)
	for _, np := range npList.Items {
		if len(np.Spec.Egress) > 0 || (np.Spec.PolicyTypes != nil && containsStr1961(policyTypesToStr(np.Spec.PolicyTypes), "Egress")) {
			nsWithEgressPolicy[np.Namespace] = true
		}
	}
	result.Summary.WithEgressPolicy = len(nsWithEgressPolicy)
	result.Summary.WithoutEgress = result.Summary.TotalNamespaces - len(nsWithEgressPolicy)
	if result.Summary.WithoutEgress > 0 {
		score -= result.Summary.WithoutEgress * 3
	}

	// Check services for external exposure
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++

		exposed := false
		detail := ""
		switch svc.Spec.Type {
		case corev1.ServiceTypeLoadBalancer:
			result.Summary.LoadBalancerSvc++
			exposed = true
			detail = "LoadBalancer — externally accessible"
		case corev1.ServiceTypeNodePort:
			result.Summary.NodePortSvc++
			exposed = true
			detail = "NodePort — accessible on node ports"
		}
		if len(svc.Spec.ExternalIPs) > 0 {
			result.Summary.ExternalIPCount++
			exposed = true
			detail = "ExternalIP configured"
		}

		if exposed {
			result.ExternalTargets = append(result.ExternalTargets, EgressTarget1961{
				Namespace: svc.Namespace, Service: svc.Name,
				Type: string(svc.Spec.Type), Exposed: true, Detail: detail,
			})
			score -= 2
		}
	}

	// Find namespaces without egress policy
	for _, ns := range nsList.Items {
		if !nsWithEgressPolicy[ns.Name] {
			result.UnrestrictedNS = append(result.UnrestrictedNS, ns.Name)
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d/%d namespaces with egress network policy", result.Summary.WithEgressPolicy, result.Summary.TotalNamespaces))
	if result.Summary.LoadBalancerSvc+result.Summary.NodePortSvc > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d externally exposed services (LB: %d, NodePort: %d)", result.Summary.LoadBalancerSvc+result.Summary.NodePortSvc, result.Summary.LoadBalancerSvc, result.Summary.NodePortSvc))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

func containsStr1961(s, sub string) bool {
	return strings.Contains(s, sub)
}

func policyTypesToStr(pts []networkingv1.PolicyType) string {
	var sb strings.Builder
	for _, v := range pts {
		sb.WriteString(string(v))
		sb.WriteString(" ")
	}
	return sb.String()
}

// ---------------------------------------------------------------
// 3. Node Hardening Score
// ---------------------------------------------------------------

type NodeHardenResult1961 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         NodeHardenSummary1961 `json:"summary"`
	Nodes           []NodeHardenEntry1961 `json:"nodes"`
	Checks          []NodeHardenCheck1961 `json:"checks"`
	Recommendations []string              `json:"recommendations"`
}

type NodeHardenSummary1961 struct {
	TotalNodes      int     `json:"totalNodes"`
	ReadyNodes      int     `json:"readyNodes"`
	AvgHardening    float64 `json:"avgHardeningScore"`
	NodesWithTaints int     `json:"nodesWithTaints"`
	KubeletVersion  string  `json:"kubeletVersion"`
}

type NodeHardenEntry1961 struct {
	Name           string  `json:"name"`
	Ready          bool    `json:"ready"`
	HardeningScore float64 `json:"hardeningScore"`
	Issues         int     `json:"issues"`
	Version        string  `json:"version"`
}

type NodeHardenCheck1961 struct {
	Check    string `json:"check"`
	Status   string `json:"status"`
	Severity string `json:"severity"`
	Detail   string `json:"detail"`
}

func (s *Server) handleNodeHardeningScore(w http.ResponseWriter, r *http.Request) {
	result := NodeHardenResult1961{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	var totalHardening float64
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		entry := NodeHardenEntry1961{Name: node.Name}
		nodeScore := 100.0

		// Check node ready
		ready := false
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady {
				ready = cond.Status == corev1.ConditionTrue
			}
			// Check for problematic conditions
			if cond.Status == corev1.ConditionTrue && cond.Type != corev1.NodeReady {
				entry.Issues++
				nodeScore -= 5
			}
		}
		entry.Ready = ready
		if ready {
			result.Summary.ReadyNodes++
		} else {
			score -= 10
		}

		// Node version
		entry.Version = node.Status.NodeInfo.KubeletVersion
		if result.Summary.KubeletVersion == "" {
			result.Summary.KubeletVersion = node.Status.NodeInfo.KubeletVersion
		}

		// Check taints
		if len(node.Spec.Taints) > 0 {
			result.Summary.NodesWithTaints++
			for _, taint := range node.Spec.Taints {
				if taint.Effect == corev1.TaintEffectNoSchedule || taint.Effect == corev1.TaintEffectNoExecute {
					// Not necessarily bad, just reduces capacity
				}
			}
		}

		// Check container runtime
		runtime := node.Status.NodeInfo.ContainerRuntimeVersion
		if strings.Contains(runtime, "docker") {
			entry.Issues++
			nodeScore -= 5
			result.Checks = append(result.Checks, NodeHardenCheck1961{
				Check: "container-runtime", Status: "warn", Severity: "medium",
				Detail: fmt.Sprintf("Node %s uses Docker (deprecated) — migrate to containerd", node.Name),
			})
			score -= 2
		} else {
			result.Checks = append(result.Checks, NodeHardenCheck1961{
				Check: "container-runtime", Status: "pass", Severity: "info",
				Detail: fmt.Sprintf("Node %s uses %s", node.Name, runtime),
			})
		}

		// Check OS image
		osImage := node.Status.NodeInfo.OSImage
		if strings.Contains(strings.ToLower(osImage), "alpine") {
			result.Checks = append(result.Checks, NodeHardenCheck1961{
				Check: "os-image", Status: "warn", Severity: "low",
				Detail: fmt.Sprintf("Node %s runs Alpine (minimal, check kernel hardening)", node.Name),
			})
		}

		if nodeScore < entry.HardeningScore || entry.HardeningScore == 0 {
			entry.HardeningScore = nodeScore
		}
		totalHardening += nodeScore

		result.Nodes = append(result.Nodes, entry)
	}

	if result.Summary.TotalNodes > 0 {
		result.Summary.AvgHardening = totalHardening / float64(result.Summary.TotalNodes)
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("Node hardening avg: %.1f/100 across %d nodes", result.Summary.AvgHardening, result.Summary.TotalNodes))
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("Kubelet: %s, Container runtime check done", result.Summary.KubeletVersion))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
