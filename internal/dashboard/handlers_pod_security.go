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

// PodSecSeverity ranks findings by importance.
type PodSecSeverity string

const (
	PodSecCritical PodSecSeverity = "critical"
	PodSecWarning  PodSecSeverity = "warning"
	PodSecInfo     PodSecSeverity = "info"
)

// PodSecCheck is a category of security finding.
type PodSecCheck string

const (
	SecCheckPrivileged      PodSecCheck = "privileged"
	SecCheckHostNetwork     PodSecCheck = "host-network"
	SecCheckHostPID         PodSecCheck = "host-pid"
	SecCheckHostIPC         PodSecCheck = "host-ipc"
	SecCheckHostPath        PodSecCheck = "host-path"
	SecCheckCapabilities    PodSecCheck = "dangerous-capabilities"
	SecCheckRunAsRoot       PodSecCheck = "runs-as-root"
	SecCheckPrivEscalation  PodSecCheck = "privilege-escalation"
	SecCheckReadOnlyFS      PodSecCheck = "writable-rootfs"
	SecCheckNoSecContext    PodSecCheck = "missing-security-context"
	SecCheckImageLatest     PodSecCheck = "image-latest"
	SecCheckImageNoTag      PodSecCheck = "image-no-tag"
	SecCheckImageDigest     PodSecCheck = "image-no-digest"
	SecCheckSecretEnv       PodSecCheck = "secret-env-vars"
	SecCheckNoResourceLimit PodSecCheck = "no-resource-limits"
	SecCheckHostPort        PodSecCheck = "host-port"
)

// PodSecFinding describes one security issue in a pod.
type PodSecFinding struct {
	Check      PodSecCheck    `json:"check"`
	Severity   PodSecSeverity `json:"severity"`
	Container  string         `json:"container,omitempty"`
	Message    string         `json:"message"`
	Suggestion string         `json:"suggestion"`
}

// PodSecPod represents one pod with its security findings.
type PodSecPod struct {
	Name      string          `json:"name"`
	Namespace string          `json:"namespace"`
	Kind      string          `json:"kind"`
	Phase     string          `json:"phase"`
	NodeName  string          `json:"node"`
	AgeHours  float64         `json:"ageHours"`
	Images    []string        `json:"images"`
	Findings  []PodSecFinding `json:"findings"`
	RiskScore int             `json:"riskScore"` // 0 (safe) to 100 (extremely risky)
}

// PodSecResult is the full scan output.
type PodSecResult struct {
	ScannedAt   time.Time          `json:"scannedAt"`
	Summary     PodSecSummary      `json:"summary"`
	Pods        []PodSecPod        `json:"pods"`
	TopFindings []PodSecFindingAgg `json:"topFindings"`
	ByNamespace []PodSecNamespace  `json:"byNamespace"`
}

// PodSecSummary aggregates cluster-wide pod security metrics.
type PodSecSummary struct {
	TotalPods       int `json:"totalPods"`
	PodsWithIssues  int `json:"podsWithIssues"`
	CriticalCount   int `json:"criticalCount"`
	WarningCount    int `json:"warningCount"`
	InfoCount       int `json:"infoCount"`
	PrivilegedPods  int `json:"privilegedPods"`
	HostNetworkPods int `json:"hostNetworkPods"`
	HostPathPods    int `json:"hostPathPods"`
	RootPods        int `json:"rootPods"`
	AvgRiskScore    int `json:"avgRiskScore"`
}

// PodSecFindingAgg aggregates finding counts per check type.
type PodSecFindingAgg struct {
	Check    PodSecCheck    `json:"check"`
	Severity PodSecSeverity `json:"severity"`
	Count    int            `json:"count"`
}

// PodSecNamespace summarizes findings per namespace.
type PodSecNamespace struct {
	Namespace string `json:"namespace"`
	PodCount  int    `json:"podCount"`
	Findings  int    `json:"findings"`
	Critical  int    `json:"critical"`
	RiskScore int    `json:"riskScore"`
}

// handlePodSecurityScan audits all running pods for security posture.
// GET /api/security/pods?namespace=xxx&severity=critical
func (s *Server) handlePodSecurityScan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")
	severityFilter := r.URL.Query().Get("severity")

	pods, err := rc.clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{Limit: 1000})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	result := PodSecResult{
		ScannedAt: time.Now(),
	}

	// Per-finding aggregation
	findingAgg := make(map[string]*PodSecFindingAgg)
	// Per-namespace aggregation
	nsMap := make(map[string]*PodSecNamespace)
	totalScore := 0

	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}

		psp := auditPodSecurity(pod)
		result.Pods = append(result.Pods, psp)

		result.Summary.TotalPods++
		nsEntry := getOrCreateNs(nsMap, pod.Namespace)
		nsEntry.PodCount++

		if len(psp.Findings) > 0 {
			result.Summary.PodsWithIssues++
			nsEntry.Findings++
		}

		for _, f := range psp.Findings {
			switch f.Severity {
			case PodSecCritical:
				result.Summary.CriticalCount++
				nsEntry.Critical++
			case PodSecWarning:
				result.Summary.WarningCount++
			case PodSecInfo:
				result.Summary.InfoCount++
			}

			key := string(f.Check)
			if agg, ok := findingAgg[key]; ok {
				agg.Count++
			} else {
				findingAgg[key] = &PodSecFindingAgg{
					Check:    f.Check,
					Severity: f.Severity,
					Count:    1,
				}
			}
		}

		totalScore += psp.RiskScore
		nsEntry.RiskScore += psp.RiskScore

		// Track specific risky patterns
		for _, f := range psp.Findings {
			switch f.Check {
			case SecCheckPrivileged:
				result.Summary.PrivilegedPods++
			case SecCheckHostNetwork:
				result.Summary.HostNetworkPods++
			case SecCheckHostPath:
				result.Summary.HostPathPods++
			case SecCheckRunAsRoot:
				result.Summary.RootPods++
			}
		}
	}

	// Average risk score
	if result.Summary.TotalPods > 0 {
		result.Summary.AvgRiskScore = totalScore / result.Summary.TotalPods
	}

	// Build top findings
	for _, agg := range findingAgg {
		result.TopFindings = append(result.TopFindings, *agg)
	}
	sort.Slice(result.TopFindings, func(i, j int) bool {
		if result.TopFindings[i].Severity != result.TopFindings[j].Severity {
			return podSecSevRank(result.TopFindings[i].Severity) < podSecSevRank(result.TopFindings[j].Severity)
		}
		return result.TopFindings[i].Count > result.TopFindings[j].Count
	})

	// Build namespace summary
	for _, ns := range nsMap {
		if ns.PodCount > 0 {
			ns.RiskScore = ns.RiskScore / ns.PodCount
		}
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].RiskScore > result.ByNamespace[j].RiskScore
	})

	// Sort pods by risk score descending
	sort.Slice(result.Pods, func(i, j int) bool {
		if result.Pods[i].RiskScore != result.Pods[j].RiskScore {
			return result.Pods[i].RiskScore > result.Pods[j].RiskScore
		}
		return result.Pods[i].Name < result.Pods[j].Name
	})

	// Apply severity filter
	if severityFilter != "" {
		filtered := make([]PodSecPod, 0, len(result.Pods))
		for _, psp := range result.Pods {
			for _, f := range psp.Findings {
				if string(f.Severity) == severityFilter {
					filtered = append(filtered, psp)
					break
				}
			}
		}
		result.Pods = filtered
	}

	writeJSON(w, result)
}

// auditPodSecurity scans a single pod for security issues.
func auditPodSecurity(pod *corev1.Pod) PodSecPod {
	psp := PodSecPod{
		Name:      pod.Name,
		Namespace: pod.Namespace,
		Kind:      podKind(pod),
		Phase:     string(pod.Status.Phase),
		NodeName:  pod.Spec.NodeName,
		AgeHours:  hoursSince(pod.CreationTimestamp),
	}

	// Pod-level security context defaults
	podSC := pod.Spec.SecurityContext
	podRunAsNonRoot := false
	podRunAsUser := int64(0)
	if podSC != nil {
		if podSC.RunAsNonRoot != nil && *podSC.RunAsNonRoot {
			podRunAsNonRoot = true
		}
		if podSC.RunAsUser != nil {
			podRunAsUser = *podSC.RunAsUser
		}
	}

	// Pod-level host settings
	if pod.Spec.HostNetwork {
		psp.addFinding(PodSecCritical, SecCheckHostNetwork, "",
			"Pod uses hostNetwork — shares the node's network namespace",
			"Disable hostNetwork unless absolutely necessary; use NodePort/LoadBalancer instead")
	}
	if pod.Spec.HostPID {
		psp.addFinding(PodSecCritical, SecCheckHostPID, "",
			"Pod uses hostPID — can see all processes on the node",
			"Disable hostPID to prevent process visibility across pods")
	}
	if pod.Spec.HostIPC {
		psp.addFinding(PodSecCritical, SecCheckHostIPC, "",
			"Pod uses hostIPC — shares inter-process communication namespace",
			"Disable hostIPC unless required for shared memory")
	}

	// HostPath volumes
	for _, vol := range pod.Spec.Volumes {
		if vol.HostPath != nil {
			path := vol.HostPath.Path
			psp.addFinding(PodSecCritical, SecCheckHostPath, "",
				fmt.Sprintf("HostPath volume %q mounts %s from the node", vol.Name, path),
				"Use persistent volumes or projected volumes instead of hostPath")
		}
	}

	// Image analysis
	for _, c := range pod.Spec.Containers {
		psp.Images = append(psp.Images, c.Image)
		auditImageSecurity(&psp, c)
		auditContainerSecurity(&psp, c, podRunAsNonRoot, podRunAsUser)
	}

	// Init containers
	for _, c := range pod.Spec.InitContainers {
		auditContainerSecurity(&psp, c, podRunAsNonRoot, podRunAsUser)
	}

	// Check for secrets in env vars
	for _, c := range pod.Spec.Containers {
		for _, env := range c.Env {
			if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
				psp.addFinding(PodSecInfo, SecCheckSecretEnv, c.Name,
					fmt.Sprintf("Container %q injects secret %q as env var %q", c.Name, env.ValueFrom.SecretKeyRef.Name, env.Name),
					"Prefer mounting secrets as files rather than env vars for better rotation and auditability")
			}
		}
	}

	return psp
}

// auditImageSecurity checks image tag/digest practices.
func auditImageSecurity(psp *PodSecPod, c corev1.Container) {
	image := c.Image
	if image == "" {
		return
	}
	// Strip registry prefix
	ref := image
	if idx := strings.LastIndex(ref, "/"); idx >= 0 {
		ref = ref[idx+1:]
	}
	// Check for tag
	parts := strings.Split(ref, ":")
	hasTag := len(parts) > 1 && parts[len(parts)-1] != ""
	hasDigest := strings.Contains(ref, "@")

	if hasDigest {
		return // pinned by digest — best practice
	}
	if !hasTag {
		psp.addFinding(PodSecWarning, SecCheckImageNoTag, c.Name,
			fmt.Sprintf("Container %q uses image %q without a tag (defaults to :latest)", c.Name, c.Image),
			"Pin to a specific version tag or use a digest for reproducibility and security")
		return
	}
	tag := parts[len(parts)-1]
	if tag == "latest" {
		psp.addFinding(PodSecWarning, SecCheckImageLatest, c.Name,
			fmt.Sprintf("Container %q uses :latest tag — image content can change without notice", c.Name),
			"Pin to a specific version tag or use a digest for immutability")
	}
}

// auditContainerSecurity checks container-level security context and resources.
func auditContainerSecurity(psp *PodSecPod, c corev1.Container, podRunAsNonRoot bool, podRunAsUser int64) {
	// Check for resource limits
	if c.Resources.Limits.Cpu().IsZero() && c.Resources.Limits.Memory().IsZero() {
		psp.addFinding(PodSecInfo, SecCheckNoResourceLimit, c.Name,
			fmt.Sprintf("Container %q has no resource limits — can exhaust node resources", c.Name),
			"Set CPU and memory limits for resource isolation")
	}

	// Check for host ports
	for _, p := range c.Ports {
		if p.HostPort > 0 {
			psp.addFinding(PodSecWarning, SecCheckHostPort, c.Name,
				fmt.Sprintf("Container %q binds host port %d", c.Name, p.HostPort),
				"Avoid hostPort — use NodePort or LoadBalancer service instead")
		}
	}

	if c.SecurityContext == nil {
		psp.addFinding(PodSecWarning, SecCheckNoSecContext, c.Name,
			fmt.Sprintf("Container %q has no security context", c.Name),
			"Set securityContext: runAsNonRoot=true, allowPrivilegeEscalation=false, readOnlyRootFilesystem=true")
		if !podRunAsNonRoot {
			psp.addFinding(PodSecWarning, SecCheckRunAsRoot, c.Name,
				fmt.Sprintf("Container %q has no security context and defaults to running as root (UID 0)", c.Name),
				"Set runAsNonRoot=true and runAsUser to a non-zero UID")
		}
		return
	}

	sc := c.SecurityContext

	// Privileged
	if sc.Privileged != nil && *sc.Privileged {
		psp.addFinding(PodSecCritical, SecCheckPrivileged, c.Name,
			fmt.Sprintf("Container %q runs in privileged mode — full host access", c.Name),
			"Remove privileged: true; grant specific capabilities instead")
	}

	// Privilege escalation
	if sc.AllowPrivilegeEscalation == nil || *sc.AllowPrivilegeEscalation {
		psp.addFinding(PodSecWarning, SecCheckPrivEscalation, c.Name,
			fmt.Sprintf("Container %q allows privilege escalation", c.Name),
			"Set allowPrivilegeEscalation: false")
	}

	// Run as root
	if sc.RunAsNonRoot == nil || !*sc.RunAsNonRoot {
		if !podRunAsNonRoot {
			effectiveUID := podRunAsUser
			if sc.RunAsUser != nil {
				effectiveUID = *sc.RunAsUser
			}
			if effectiveUID == 0 {
				psp.addFinding(PodSecWarning, SecCheckRunAsRoot, c.Name,
					fmt.Sprintf("Container %q runs as root (UID 0)", c.Name),
					"Set runAsNonRoot=true and runAsUser to a non-zero UID")
			}
		}
	}

	// Read-only root filesystem
	if sc.ReadOnlyRootFilesystem == nil || !*sc.ReadOnlyRootFilesystem {
		psp.addFinding(PodSecInfo, SecCheckReadOnlyFS, c.Name,
			fmt.Sprintf("Container %q has a writable root filesystem", c.Name),
			"Set readOnlyRootFilesystem: true and mount writable paths explicitly")
	}

	// Dangerous capabilities
	if sc.Capabilities != nil {
		dangerous := []string{"SYS_ADMIN", "NET_ADMIN", "NET_RAW", "SYS_PTRACE", "SYS_MODULE", "DAC_OVERRIDE", "SETUID", "SETGID"}
		for _, cap := range sc.Capabilities.Add {
			upperCap := strings.ToUpper(string(cap))
			for _, dc := range dangerous {
				if upperCap == dc {
					psp.addFinding(PodSecCritical, SecCheckCapabilities, c.Name,
						fmt.Sprintf("Container %q adds dangerous Linux capability %s", c.Name, upperCap),
						fmt.Sprintf("Remove capability %s — it grants excessive kernel access", upperCap))
				}
			}
		}
	}
}

// addFinding appends a finding and updates the risk score.
func (psp *PodSecPod) addFinding(severity PodSecSeverity, check PodSecCheck, container, msg, suggestion string) {
	psp.Findings = append(psp.Findings, PodSecFinding{
		Severity:   severity,
		Check:      check,
		Container:  container,
		Message:    msg,
		Suggestion: suggestion,
	})
	switch severity {
	case PodSecCritical:
		psp.RiskScore += 25
	case PodSecWarning:
		psp.RiskScore += 8
	case PodSecInfo:
		psp.RiskScore += 2
	}
}

func podSecSevRank(s PodSecSeverity) int {
	switch s {
	case PodSecCritical:
		return 0
	case PodSecWarning:
		return 1
	case PodSecInfo:
		return 2
	}
	return 9
}

func podKind(pod *corev1.Pod) string {
	if len(pod.OwnerReferences) > 0 {
		return pod.OwnerReferences[0].Kind
	}
	return "Pod"
}

func getOrCreateNs(m map[string]*PodSecNamespace, ns string) *PodSecNamespace {
	if entry, ok := m[ns]; ok {
		return entry
	}
	entry := &PodSecNamespace{Namespace: ns}
	m[ns] = entry
	return entry
}
