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

// RemediationMatrixResult is the security remediation priority & risk-effort matrix.
// It collects security findings from the live cluster, scores them using CVSS-like methodology,
// and prioritizes remediation by risk × effort.
type RemediationMatrixResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	Summary         RemediationSummary   `json:"summary"`
	Findings        []RemediationFinding `json:"findings"`
	QuickWins       []RemediationFinding `json:"quickWins"`      // high risk, low effort
	StrategicFixes  []RemediationFinding `json:"strategicFixes"` // high risk, high effort
	ByCategory      []CategoryRisk       `json:"byCategory"`
	RemediationPlan []RemediationStep    `json:"remediationPlan"`
	Recommendations []string             `json:"recommendations"`
}

// RemediationSummary aggregates findings statistics.
type RemediationSummary struct {
	TotalFindings         int     `json:"totalFindings"`
	CriticalCount         int     `json:"criticalCount"`
	HighCount             int     `json:"highCount"`
	MediumCount           int     `json:"mediumCount"`
	LowCount              int     `json:"lowCount"`
	QuickWinCount         int     `json:"quickWinCount"`  // fixable in <1 hour
	TotalRiskScore        int     `json:"totalRiskScore"` // sum of all finding risk scores
	AvgRiskScore          float64 `json:"avgRiskScore"`
	EstimatedFixTimeHours float64 `json:"estimatedFixTimeHours"` // total effort to fix all
}

// RemediationFinding is a single security issue with risk scoring.
type RemediationFinding struct {
	ID         string `json:"id"`
	Category   string `json:"category"` // pod-security, rbac, network, secrets, image, admission
	Title      string `json:"title"`
	Severity   string `json:"severity"`  // critical, high, medium, low
	RiskScore  int    `json:"riskScore"` // 0-100, CVSS-like
	Effort     string `json:"effort"`    // quick (≤1h), moderate (1-4h), strategic (>4h)
	Resource   string `json:"resource"`  // affected resource
	Namespace  string `json:"namespace,omitempty"`
	Detail     string `json:"detail"`
	FixCommand string `json:"fixCommand,omitempty"` // kubectl command or config change
}

// CategoryRisk aggregates risk by category.
type CategoryRisk struct {
	Category     string `json:"category"`
	FindingCount int    `json:"findingCount"`
	TotalRisk    int    `json:"totalRiskScore"`
	AvgRisk      int    `json:"avgRiskScore"`
	TopSeverity  string `json:"topSeverity"`
}

// RemediationStep is an ordered action in the remediation plan.
type RemediationStep struct {
	Priority  int    `json:"priority"`
	FindingID string `json:"findingId"`
	Action    string `json:"action"`
	Impact    string `json:"impact"`
	Effort    string `json:"effort"`
}

// handleRemediationMatrix provides the security remediation priority matrix.
// GET /api/security/remediation-matrix
func (s *Server) handleRemediationMatrix(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := RemediationMatrixResult{ScannedAt: time.Now()}

	systemNS := map[string]bool{
		"kube-system": true, "kube-public": true, "kube-node-lease": true,
	}

	// Collect pods
	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list pods")
		return
	}

	// Collect namespaces
	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})

	// Collect service accounts
	saList, _ := rc.clientset.CoreV1().ServiceAccounts("").List(ctx, metav1.ListOptions{})

	// Collect network policies
	npList, _ := rc.clientset.NetworkingV1().NetworkPolicies("").List(ctx, metav1.ListOptions{})

	// Collect services
	svcList, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})

	findingID := 0
	nextID := func() string {
		findingID++
		return fmt.Sprintf("F%03d", findingID)
	}

	// === Pod Security Findings ===
	for _, pod := range pods.Items {
		if systemNS[pod.Namespace] {
			continue
		}
		for _, c := range pod.Spec.Containers {
			// Privileged container
			if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
				result.Findings = append(result.Findings, RemediationFinding{
					ID:         nextID(),
					Category:   "pod-security",
					Title:      "Privileged container detected",
					Severity:   "critical",
					RiskScore:  95,
					Effort:     "quick",
					Resource:   fmt.Sprintf("%s/%s (%s)", pod.Namespace, pod.Name, c.Name),
					Namespace:  pod.Namespace,
					Detail:     "Container runs in privileged mode, granting full host access",
					FixCommand: fmt.Sprintf("kubectl set resources deployment %s -n %s --containers=%s  # Set privileged: false", pod.Name, pod.Namespace, c.Name),
				})
			}

			// Runs as root
			if c.SecurityContext == nil || c.SecurityContext.RunAsNonRoot == nil || !*c.SecurityContext.RunAsNonRoot {
				if c.SecurityContext == nil || c.SecurityContext.RunAsUser == nil || *c.SecurityContext.RunAsUser == 0 {
					result.Findings = append(result.Findings, RemediationFinding{
						ID:         nextID(),
						Category:   "pod-security",
						Title:      "Container runs as root",
						Severity:   "high",
						RiskScore:  70,
						Effort:     "quick",
						Resource:   fmt.Sprintf("%s/%s (%s)", pod.Namespace, pod.Name, c.Name),
						Namespace:  pod.Namespace,
						Detail:     "Container runs as UID 0 or without runAsNonRoot=true",
						FixCommand: fmt.Sprintf("Add securityContext.runAsNonRoot: true and runAsUser: >0 to container %s", c.Name),
					})
				}
			}

			// Dangerous capabilities
			if c.SecurityContext != nil && c.SecurityContext.Capabilities != nil {
				for _, cap := range c.SecurityContext.Capabilities.Add {
					if cap == "SYS_ADMIN" || cap == "NET_ADMIN" || cap == "DAC_OVERRIDE" || cap == "ALL" {
						result.Findings = append(result.Findings, RemediationFinding{
							ID:         nextID(),
							Category:   "pod-security",
							Title:      fmt.Sprintf("Dangerous capability %s granted", cap),
							Severity:   "high",
							RiskScore:  75,
							Effort:     "quick",
							Resource:   fmt.Sprintf("%s/%s (%s)", pod.Namespace, pod.Name, c.Name),
							Namespace:  pod.Namespace,
							Detail:     fmt.Sprintf("Container has %s capability which can be used for container escape or privilege escalation", cap),
							FixCommand: fmt.Sprintf("Remove %s from securityContext.capabilities.add", cap),
						})
					}
				}
			}

			// No resource limits
			if c.Resources.Limits == nil || len(c.Resources.Limits) == 0 {
				result.Findings = append(result.Findings, RemediationFinding{
					ID:         nextID(),
					Category:   "pod-security",
					Title:      "Container without resource limits",
					Severity:   "medium",
					RiskScore:  40,
					Effort:     "quick",
					Resource:   fmt.Sprintf("%s/%s (%s)", pod.Namespace, pod.Name, c.Name),
					Namespace:  pod.Namespace,
					Detail:     "No resource limits set — vulnerable to resource exhaustion DoS",
					FixCommand: fmt.Sprintf("Add resources.limits to container %s", c.Name),
				})
			}
		}

		// Host namespaces
		if pod.Spec.HostNetwork || pod.Spec.HostPID || pod.Spec.HostIPC {
			nsList := []string{}
			if pod.Spec.HostNetwork {
				nsList = append(nsList, "hostNetwork")
			}
			if pod.Spec.HostPID {
				nsList = append(nsList, "hostPID")
			}
			if pod.Spec.HostIPC {
				nsList = append(nsList, "hostIPC")
			}
			result.Findings = append(result.Findings, RemediationFinding{
				ID:         nextID(),
				Category:   "pod-security",
				Title:      fmt.Sprintf("Pod uses host namespaces: %s", strings.Join(nsList, ", ")),
				Severity:   "high",
				RiskScore:  72,
				Effort:     "moderate",
				Resource:   fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
				Namespace:  pod.Namespace,
				Detail:     "Pod shares host network/PID/IPC namespace, increasing attack surface",
				FixCommand: fmt.Sprintf("Disable %s in pod spec for %s", strings.Join(nsList, ", "), pod.Name),
			})
		}
	}

	// === Network Security Findings ===
	nsWithoutNetPol := make(map[string]bool)
	for _, ns := range namespaces.Items {
		if !systemNS[ns.Name] {
			nsWithoutNetPol[ns.Name] = true
		}
	}
	for _, np := range npList.Items {
		delete(nsWithoutNetPol, np.Namespace)
	}
	for ns := range nsWithoutNetPol {
		result.Findings = append(result.Findings, RemediationFinding{
			ID:         nextID(),
			Category:   "network",
			Title:      "Namespace without NetworkPolicy (no traffic isolation)",
			Severity:   "high",
			RiskScore:  65,
			Effort:     "moderate",
			Resource:   fmt.Sprintf("namespace/%s", ns),
			Namespace:  ns,
			Detail:     "No NetworkPolicy exists — all pods can communicate freely, lateral movement risk",
			FixCommand: fmt.Sprintf("kubectl apply -f default-deny.yaml -n %s  # Create default deny NetworkPolicy", ns),
		})
	}

	// Exposed LoadBalancer services
	for _, svc := range svcList.Items {
		if systemNS[svc.Namespace] {
			continue
		}
		if svc.Spec.Type == corev1.ServiceTypeLoadBalancer || svc.Spec.Type == corev1.ServiceTypeNodePort {
			result.Findings = append(result.Findings, RemediationFinding{
				ID:         nextID(),
				Category:   "network",
				Title:      fmt.Sprintf("Externally exposed service (%s)", svc.Spec.Type),
				Severity:   "medium",
				RiskScore:  50,
				Effort:     "moderate",
				Resource:   fmt.Sprintf("%s/%s", svc.Namespace, svc.Name),
				Namespace:  svc.Namespace,
				Detail:     fmt.Sprintf("Service type %s exposes workload to external traffic without network policy protection", svc.Spec.Type),
				FixCommand: fmt.Sprintf("Consider Ingress with TLS or add NetworkPolicy for service %s", svc.Name),
			})
		}
	}

	// === RBAC Findings ===
	// Service accounts with cluster-admin or privileged bindings
	for _, sa := range saList.Items {
		if systemNS[sa.Namespace] {
			continue
		}
		// Check for automounted token
		if sa.AutomountServiceAccountToken == nil || *sa.AutomountServiceAccountToken {
			// Only flag if the SA has no associated pods (unused SA with token)
			hasPods := false
			for _, pod := range pods.Items {
				if pod.Namespace == sa.Namespace && pod.Spec.ServiceAccountName == sa.Name {
					hasPods = true
					break
				}
			}
			if !hasPods && sa.Name != "default" {
				result.Findings = append(result.Findings, RemediationFinding{
					ID:         nextID(),
					Category:   "rbac",
					Title:      "Unused ServiceAccount with automounted token",
					Severity:   "medium",
					RiskScore:  45,
					Effort:     "quick",
					Resource:   fmt.Sprintf("%s/%s", sa.Namespace, sa.Name),
					Namespace:  sa.Namespace,
					Detail:     "ServiceAccount has no associated pods but automounts API tokens, creating unused credentials",
					FixCommand: fmt.Sprintf("kubectl delete serviceaccount %s -n %s  # Or set automountServiceAccountToken: false", sa.Name, sa.Namespace),
				})
			}
		}
	}

	// === Image Security Findings ===
	imageSet := make(map[string]bool)
	for _, pod := range pods.Items {
		if systemNS[pod.Namespace] {
			continue
		}
		for _, c := range pod.Spec.Containers {
			img := c.Image
			// Check for latest tag
			if strings.HasSuffix(img, ":latest") || !strings.Contains(lastTag(img), ":") {
				if !imageSet[img] {
					imageSet[img] = true
					result.Findings = append(result.Findings, RemediationFinding{
						ID:         nextID(),
						Category:   "image",
						Title:      "Image uses mutable tag (latest or no tag)",
						Severity:   "medium",
						RiskScore:  42,
						Effort:     "quick",
						Resource:   img,
						Namespace:  pod.Namespace,
						Detail:     "Using mutable tags like 'latest' makes deployments non-reproducible and vulnerable to supply chain attacks",
						FixCommand: fmt.Sprintf("Pin image to specific digest: %s@sha256:...", img),
					})
				}
			}
		}
	}

	// === Namespace PSA Findings ===
	for _, ns := range namespaces.Items {
		if systemNS[ns.Name] {
			continue
		}
		labels := ns.GetLabels()
		hasPSA := false
		if labels != nil {
			for k := range labels {
				if strings.Contains(k, "pod-security.kubernetes.io") {
					hasPSA = true
					break
				}
			}
		}
		if !hasPSA {
			result.Findings = append(result.Findings, RemediationFinding{
				ID:         nextID(),
				Category:   "admission",
				Title:      "Namespace without Pod Security Admission labels",
				Severity:   "medium",
				RiskScore:  38,
				Effort:     "quick",
				Resource:   fmt.Sprintf("namespace/%s", ns.Name),
				Namespace:  ns.Name,
				Detail:     "No Pod Security Admission labels — no policy enforcement for pod security standards",
				FixCommand: fmt.Sprintf("kubectl label namespace %s pod-security.kubernetes.io/enforce=restricted", ns.Name),
			})
		}
	}

	// === Compute Summary ===
	result.Summary.TotalFindings = len(result.Findings)
	totalRisk := 0
	totalEffortHours := 0.0
	for _, f := range result.Findings {
		totalRisk += f.RiskScore
		switch f.Severity {
		case "critical":
			result.Summary.CriticalCount++
		case "high":
			result.Summary.HighCount++
		case "medium":
			result.Summary.MediumCount++
		case "low":
			result.Summary.LowCount++
		}
		if f.Effort == "quick" {
			result.Summary.QuickWinCount++
			totalEffortHours += 0.5
		} else if f.Effort == "moderate" {
			totalEffortHours += 2
		} else {
			totalEffortHours += 6
		}
	}
	result.Summary.TotalRiskScore = totalRisk
	if len(result.Findings) > 0 {
		result.Summary.AvgRiskScore = float64(totalRisk) / float64(len(result.Findings))
	}
	result.Summary.EstimatedFixTimeHours = totalEffortHours

	// === Categorize: Quick Wins vs Strategic ===
	for _, f := range result.Findings {
		if f.RiskScore >= 60 && f.Effort == "quick" {
			result.QuickWins = append(result.QuickWins, f)
		}
		if f.RiskScore >= 60 && f.Effort != "quick" {
			result.StrategicFixes = append(result.StrategicFixes, f)
		}
	}

	// Sort by risk score descending
	sort.Slice(result.QuickWins, func(i, j int) bool {
		return result.QuickWins[i].RiskScore > result.QuickWins[j].RiskScore
	})
	sort.Slice(result.StrategicFixes, func(i, j int) bool {
		return result.StrategicFixes[i].RiskScore > result.StrategicFixes[j].RiskScore
	})

	// === Category Risk Aggregation ===
	catMap := make(map[string]*CategoryRisk)
	for _, f := range result.Findings {
		cat, ok := catMap[f.Category]
		if !ok {
			cat = &CategoryRisk{Category: f.Category}
			catMap[f.Category] = cat
		}
		cat.FindingCount++
		cat.TotalRisk += f.RiskScore
		// Track highest severity
		if f.Severity == "critical" || (f.Severity == "high" && cat.TopSeverity != "critical") ||
			(f.Severity == "medium" && cat.TopSeverity == "") {
			cat.TopSeverity = f.Severity
		}
	}
	for _, cat := range catMap {
		if cat.FindingCount > 0 {
			cat.AvgRisk = cat.TotalRisk / cat.FindingCount
		}
		result.ByCategory = append(result.ByCategory, *cat)
	}
	sort.Slice(result.ByCategory, func(i, j int) bool {
		return result.ByCategory[i].TotalRisk > result.ByCategory[j].TotalRisk
	})

	// === Remediation Plan ===
	result.RemediationPlan = buildRemediationPlan(result.Findings)

	// === Recommendations ===
	result.Recommendations = generateRemediationRecs(result)

	writeJSON(w, result)
}

// buildRemediationPlan creates an ordered action plan.
func buildRemediationPlan(findings []RemediationFinding) []RemediationStep {
	// Sort all findings by risk score, then prefer quick wins
	sorted := make([]RemediationFinding, len(findings))
	copy(sorted, findings)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].RiskScore == sorted[j].RiskScore {
			// Prefer quick wins first
			return effortRank(sorted[i].Effort) < effortRank(sorted[j].Effort)
		}
		return sorted[i].RiskScore > sorted[j].RiskScore
	})

	steps := make([]RemediationStep, 0)
	for i, f := range sorted {
		if i >= 15 {
			break // Top 15 actions
		}
		impact := fmt.Sprintf("Reduces cluster risk score by %d points", f.RiskScore)
		if f.Severity == "critical" {
			impact = fmt.Sprintf("Eliminates critical risk: %s", f.Title)
		}
		steps = append(steps, RemediationStep{
			Priority:  i + 1,
			FindingID: f.ID,
			Action:    f.FixCommand,
			Impact:    impact,
			Effort:    f.Effort,
		})
	}
	return steps
}

// effortRank returns a sortable rank for effort levels.
func effortRank(effort string) int {
	switch effort {
	case "quick":
		return 0
	case "moderate":
		return 1
	case "strategic":
		return 2
	default:
		return 3
	}
}

// generateRemediationRecs produces actionable recommendations.
func generateRemediationRecs(result RemediationMatrixResult) []string {
	var recs []string

	if result.Summary.CriticalCount > 0 {
		recs = append(recs, fmt.Sprintf("%d critical findings detected — fix immediately before any other work", result.Summary.CriticalCount))
	}

	if len(result.QuickWins) > 0 {
		recs = append(recs, fmt.Sprintf("%d quick wins available (high risk, fixable in <1 hour) — tackle these first for maximum risk reduction per effort", len(result.QuickWins)))
	}

	if result.Summary.TotalFindings > 0 {
		recs = append(recs, fmt.Sprintf("Estimated total remediation effort: %.0f hours across %d findings", result.Summary.EstimatedFixTimeHours, result.Summary.TotalFindings))
	}

	// Category-specific
	if len(result.ByCategory) > 0 {
		top := result.ByCategory[0]
		recs = append(recs, fmt.Sprintf("Highest-risk category: %s (%d findings, total risk %d) — prioritize this category", top.Category, top.FindingCount, top.TotalRisk))
	}

	if result.Summary.AvgRiskScore > 50 {
		recs = append(recs, fmt.Sprintf("Average risk score is %.1f — consider implementing Pod Security Admission 'restricted' policy cluster-wide", result.Summary.AvgRiskScore))
	}

	return recs
}

// lastTag returns the tag portion of an image reference.
func lastTag(image string) string {
	// Strip registry/repo, look for the last :tag
	parts := strings.Split(image, "/")
	last := parts[len(parts)-1]
	if idx := strings.LastIndex(last, ":"); idx >= 0 {
		return last[idx:]
	}
	return ""
}
