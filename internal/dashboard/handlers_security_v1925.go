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
// v19.25 — Security Dimension (Round 7)
// 1. Token Projection Audit — projected vs legacy SA tokens
// 2. Sysctl Risk Audit — dangerous kernel sysctls
// 3. HostPort Exposure Map — hostPort bypass isolation
// ============================================================

// ---------------------------------------------------------------
// 1. Token Projection Audit — projected vs legacy SA tokens
// ---------------------------------------------------------------

type TokenProjectionResult1925 struct {
	ScannedAt       time.Time                  `json:"scannedAt"`
	HealthScore     int                        `json:"healthScore"`
	Grade           string                     `json:"grade"`
	Summary         TokenProjectionSummary1925 `json:"summary"`
	Pods            []TokenProjectionEntry1925 `json:"pods"`
	Namespaces      []TokenProjectionNS1925    `json:"namespaces"`
	Risks           []TokenProjectionRisk1925  `json:"risks"`
	Recommendations []string                   `json:"recommendations"`
}

type TokenProjectionSummary1925 struct {
	TotalPods         int `json:"totalPods"`
	ProjectedTokens   int `json:"projectedTokens"`
	LegacyAutoMount   int `json:"legacyAutoMountTokens"`
	DisabledAutoMount int `json:"disabledAutoMount"`
	DefaultSAUsed     int `json:"defaultSAUsed"`
	TokenAudienceSet  int `json:"tokenAudienceSet"`
}

type TokenProjectionEntry1925 struct {
	PodName        string `json:"podName"`
	Namespace      string `json:"namespace"`
	ServiceAccount string `json:"serviceAccount"`
	IsProjected    bool   `json:"isProjected"`
	AutoMountToken *bool  `json:"autoMountToken"`
	UsesDefaultSA  bool   `json:"usesDefaultSA"`
	TokenAge       string `json:"tokenAge"`
}

type TokenProjectionNS1925 struct {
	Namespace string `json:"namespace"`
	Projected int    `json:"projected"`
	Legacy    int    `json:"legacy"`
	Total     int    `json:"total"`
}

type TokenProjectionRisk1925 struct {
	PodName   string `json:"podName"`
	Namespace string `json:"namespace"`
	RiskType  string `json:"riskType"`
	Severity  string `json:"severity"`
	Detail    string `json:"detail"`
}

func (s *Server) handleTokenProjection(w http.ResponseWriter, r *http.Request) {
	result := TokenProjectionResult1925{
		ScannedAt: time.Now(),
	}
	score := 100

	podList, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	nsStats := make(map[string]*TokenProjectionNS1925)

	for _, pod := range podList.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}

		saName := pod.Spec.ServiceAccountName
		if saName == "" {
			saName = "default"
		}
		usesDefaultSA := saName == "default"
		isProjected := false

		// Check for projected service account token volume
		for _, vol := range pod.Spec.Volumes {
			if vol.Projected != nil {
				for _, src := range vol.Projected.Sources {
					if src.ServiceAccountToken != nil {
						isProjected = true
					}
				}
			}
		}

		// Check automount setting
		autoMount := pod.Spec.AutomountServiceAccountToken
		disabled := autoMount != nil && !*autoMount

		// Determine token age from pod creation
		tokenAge := fmt.Sprintf("%.0fd", time.Since(pod.CreationTimestamp.Time).Hours()/24)

		entry := TokenProjectionEntry1925{
			PodName:        pod.Name,
			Namespace:      pod.Namespace,
			ServiceAccount: saName,
			IsProjected:    isProjected,
			AutoMountToken: autoMount,
			UsesDefaultSA:  usesDefaultSA,
			TokenAge:       tokenAge,
		}
		result.Pods = append(result.Pods, entry)
		result.Summary.TotalPods++

		// Namespace stats
		ns, exists := nsStats[pod.Namespace]
		if !exists {
			ns = &TokenProjectionNS1925{Namespace: pod.Namespace}
			nsStats[pod.Namespace] = ns
		}
		ns.Total++
		if isProjected {
			result.Summary.ProjectedTokens++
			ns.Projected++
		} else {
			result.Summary.LegacyAutoMount++
			ns.Legacy++
		}
		if disabled {
			result.Summary.DisabledAutoMount++
		}
		if usesDefaultSA {
			result.Summary.DefaultSAUsed++
			result.Risks = append(result.Risks, TokenProjectionRisk1925{
				PodName: pod.Name, Namespace: pod.Namespace,
				RiskType: "default-sa", Severity: "medium",
				Detail: "Pod uses default service account — excessive permissions risk",
			})
			score -= 2
		}

		// Risk: legacy auto-mount with long-lived token
		if !isProjected && !disabled {
			result.Risks = append(result.Risks, TokenProjectionRisk1925{
				PodName: pod.Name, Namespace: pod.Namespace,
				RiskType: "legacy-token", Severity: "warning",
				Detail: "Uses legacy auto-mounted SA token — switch to projected token for rotation",
			})
			score -= 1
		}
	}

	for _, ns := range nsStats {
		result.Namespaces = append(result.Namespaces, *ns)
	}

	// Score
	if result.Summary.DefaultSAUsed > 10 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.DefaultSAUsed > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods use default service account — create dedicated SAs", result.Summary.DefaultSAUsed))
	}
	if result.Summary.LegacyAutoMount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods use legacy auto-mounted tokens — migrate to projected tokens", result.Summary.LegacyAutoMount))
	}
	sort.Strings(result.Recommendations)

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Sysctl Risk Audit — dangerous kernel sysctls
// ---------------------------------------------------------------

type SysctlRiskResult1925 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         SysctlSummary1925  `json:"summary"`
	Pods            []SysctlEntry1925  `json:"pods"`
	Dangerous       []SysctlDanger1925 `json:"dangerous"`
	Recommendations []string           `json:"recommendations"`
}

type SysctlSummary1925 struct {
	TotalPods      int `json:"totalPods"`
	WithSysctls    int `json:"withSysctls"`
	DangerousCount int `json:"dangerousCount"`
	SafeCount      int `json:"safeCount"`
	UniqueSysctls  int `json:"uniqueSysctls"`
	UnsafeAllowed  int `json:"unsafeAllowed"`
}

type SysctlEntry1925 struct {
	PodName   string            `json:"podName"`
	Namespace string            `json:"namespace"`
	Sysctls   map[string]string `json:"sysctls"`
	IsSafe    bool              `json:"isSafe"`
}

type SysctlDanger1925 struct {
	PodName   string `json:"podName"`
	Namespace string `json:"namespace"`
	Sysctl    string `json:"sysctl"`
	Value     string `json:"value"`
	Severity  string `json:"severity"`
	Impact    string `json:"impact"`
}

func (s *Server) handleSysctlRisk(w http.ResponseWriter, r *http.Request) {
	result := SysctlRiskResult1925{
		ScannedAt: time.Now(),
	}
	score := 100

	// Known dangerous sysctls
	dangerousSysctls := map[string]string{
		"kernel.shm_rmid_forced":             "Forces shared memory cleanup — DoS risk",
		"net.ipv4.ip_forward":                "Enables IP forwarding — traffic interception",
		"net.ipv4.conf.all.accept_redirects": "Accept ICMP redirects — MITM risk",
		"kernel.core_pattern":                "Can write arbitrary files — escape risk",
		"kernel.dmesg_restrict":              "Kernel log visibility — info leak when disabled",
		"net.ipv4.tcp_tw_reuse":              "TCP connection reuse — security implications",
	}
	// Unsafe-only sysctls (not in safe list)
	unsafePrefixes := []string{"kernel.", "net.", "fs.", "vm.", "dev."}

	podList, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	allSysctls := make(map[string]bool)

	for _, pod := range podList.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		sysctls := make(map[string]string)
		for _, sc := range pod.Spec.SecurityContext.Sysctls {
			sysctls[sc.Name] = sc.Value
			allSysctls[sc.Name] = true
		}

		if len(sysctls) == 0 {
			continue
		}

		result.Summary.WithSysctls++
		isSafe := true

		for name, val := range sysctls {
			// Check if it's in the dangerous list
			if impact, dangerous := dangerousSysctls[name]; dangerous {
				isSafe = false
				severity := "high"
				if name == "net.ipv4.ip_forward" {
					severity = "medium"
				}
				result.Dangerous = append(result.Dangerous, SysctlDanger1925{
					PodName: pod.Name, Namespace: pod.Namespace,
					Sysctl: name, Value: val,
					Severity: severity, Impact: impact,
				})
				score -= 5
			}

			// Check unsafe prefix
			for _, prefix := range unsafePrefixes {
				if strings.HasPrefix(name, prefix) {
					result.Summary.UnsafeAllowed++
					break
				}
			}
		}

		if isSafe {
			result.Summary.SafeCount++
		} else {
			result.Summary.DangerousCount++
		}

		result.Pods = append(result.Pods, SysctlEntry1925{
			PodName: pod.Name, Namespace: pod.Namespace,
			Sysctls: sysctls, IsSafe: isSafe,
		})
	}

	result.Summary.UniqueSysctls = len(allSysctls)

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.DangerousCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods use dangerous sysctls — restrict via PSA enforced profile", result.Summary.DangerousCount))
	}
	if result.Summary.UnsafeAllowed > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d unsafe sysctls allowed — enforce restricted PodSecurity", result.Summary.UnsafeAllowed))
	}
	sort.Strings(result.Recommendations)

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. HostPort Exposure Map — hostPort bypass isolation
// ---------------------------------------------------------------

type HostPortResult1925 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         HostPortSummary1925    `json:"summary"`
	Ports           []HostPortEntry1925    `json:"ports"`
	Conflicts       []HostPortConflict1925 `json:"conflicts"`
	SecurityRisks   []HostPortRisk1925     `json:"securityRisks"`
	Recommendations []string               `json:"recommendations"`
}

type HostPortSummary1925 struct {
	TotalPods        int `json:"totalPods"`
	PodsWithHostPort int `json:"podsWithHostPort"`
	TotalHostPorts   int `json:"totalHostPorts"`
	PrivilegedPorts  int `json:"privilegedPorts"`
	Conflicts        int `json:"conflicts"`
	HighRiskCount    int `json:"highRiskCount"`
}

type HostPortEntry1925 struct {
	PodName      string `json:"podName"`
	Namespace    string `json:"namespace"`
	Port         int32  `json:"port"`
	Container    string `json:"container"`
	IsPrivileged bool   `json:"isPrivileged"`
	NodeName     string `json:"nodeName"`
}

type HostPortConflict1925 struct {
	Port      int32  `json:"port"`
	Pod1      string `json:"pod1"`
	Pod2      string `json:"pod2"`
	Namespace string `json:"namespace"`
}

type HostPortRisk1925 struct {
	PodName   string `json:"podName"`
	Namespace string `json:"namespace"`
	RiskType  string `json:"riskType"`
	Severity  string `json:"severity"`
	Detail    string `json:"detail"`
}

func (s *Server) handleHostPortExposure(w http.ResponseWriter, r *http.Request) {
	result := HostPortResult1925{
		ScannedAt: time.Now(),
	}
	score := 100

	podList, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	// Track hostPort usage per node for conflict detection
	type portKey struct {
		node string
		port int32
	}
	portOwners := make(map[portKey][]string)

	for _, pod := range podList.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		for _, c := range pod.Spec.Containers {
			for _, cp := range c.Ports {
				if cp.HostPort > 0 {
					isPriv := cp.HostPort < 1024
					entry := HostPortEntry1925{
						PodName:      pod.Name,
						Namespace:    pod.Namespace,
						Port:         cp.HostPort,
						Container:    c.Name,
						IsPrivileged: isPriv,
						NodeName:     pod.Spec.NodeName,
					}
					result.Ports = append(result.Ports, entry)
					result.Summary.TotalHostPorts++
					result.Summary.PodsWithHostPort++

					if isPriv {
						result.Summary.PrivilegedPorts++
						result.SecurityRisks = append(result.SecurityRisks, HostPortRisk1925{
							PodName: pod.Name, Namespace: pod.Namespace,
							RiskType: "privileged-port", Severity: "high",
							Detail: fmt.Sprintf("HostPort %d is privileged (<1024) — root access risk", cp.HostPort),
						})
						score -= 5
					}

					// Track for conflicts
					pk := portKey{node: pod.Spec.NodeName, port: cp.HostPort}
					portOwners[pk] = append(portOwners[pk], pod.Name)

					// Risk: hostPort bypasses NetworkPolicy
					result.SecurityRisks = append(result.SecurityRisks, HostPortRisk1925{
						PodName: pod.Name, Namespace: pod.Namespace,
						RiskType: "bypass-netpol", Severity: "medium",
						Detail: fmt.Sprintf("HostPort %d bypasses NetworkPolicy isolation", cp.HostPort),
					})
					score -= 2
				}
			}
		}
	}

	// Detect conflicts (same hostPort on same node)
	for pk, owners := range portOwners {
		if len(owners) > 1 {
			result.Conflicts = append(result.Conflicts, HostPortConflict1925{
				Port: pk.port, Pod1: owners[0], Pod2: owners[1], Namespace: "",
			})
			result.Summary.Conflicts++
			score -= 5
		}
	}

	result.Summary.HighRiskCount = len(result.SecurityRisks)
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.PodsWithHostPort > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods use HostPort — prefer ClusterIP/NodePort for isolation", result.Summary.PodsWithHostPort))
	}
	if result.Summary.PrivilegedPorts > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d privileged HostPorts (<1024) — use ports >1024", result.Summary.PrivilegedPorts))
	}
	if result.Summary.Conflicts > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d HostPort conflicts detected — pods may fail to schedule", result.Summary.Conflicts))
	}
	sort.Strings(result.Recommendations)

	writeJSON(w, result)
}
