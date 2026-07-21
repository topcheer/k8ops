package dashboard

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v18.96 — Security Dimension
// 1. Pod Escape Risk Audit
// 2. Egress Policy Gap Analyzer
// 3. CIS Benchmark Lite Check
// ============================================================

// ---------------------------------------------------------------
// 1. Pod Escape Risk — identifies containers that could escape isolation
// ---------------------------------------------------------------

type PodEscapeResult struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Summary         PodEscapeSummary `json:"summary"`
	HighRisk        []PodEscapeEntry `json:"highRisk"`
	MediumRisk      []PodEscapeEntry `json:"mediumRisk"`
	RiskFactors     []RiskFactorStat `json:"riskFactors"`
	Recommendations []string         `json:"recommendations"`
}

type PodEscapeSummary struct {
	TotalWorkloads  int `json:"totalWorkloads"`
	SafeWorkloads   int `json:"safeWorkloads"`
	HighRiskCount   int `json:"highRiskCount"`
	MediumRiskCount int `json:"mediumRiskCount"`
	Privileged      int `json:"privileged"`
	HostPID         int `json:"hostPid"`
	HostIPC         int `json:"hostIpc"`
	HostNetwork     int `json:"hostNetwork"`
	HostPathMounts  int `json:"hostPathMounts"`
	DangerousCaps   int `json:"dangerousCaps"`
	RunAsRoot       int `json:"runAsRoot"`
	NoSecurityCtx   int `json:"noSecurityContext"`
}

type PodEscapeEntry struct {
	Name      string   `json:"name"`
	Namespace string   `json:"namespace"`
	Kind      string   `json:"kind"`
	RiskScore int      `json:"riskScore"`
	RiskLevel string   `json:"riskLevel"`
	Factors   []string `json:"factors"`
}

type RiskFactorStat struct {
	Factor string `json:"factor"`
	Count  int    `json:"count"`
}

func (s *Server) handlePodEscapeRisk(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := PodEscapeResult{ScannedAt: time.Now()}

	analyze := func(name, ns, kind string, podSpec *corev1.PodSpec) {
		result.Summary.TotalWorkloads++
		entry := PodEscapeEntry{Name: name, Namespace: ns, Kind: kind}
		var factors []string

		// Check pod-level security
		if podSpec.HostPID {
			entry.RiskScore += 20
			factors = append(factors, "hostPID enabled")
			result.Summary.HostPID++
		}
		if podSpec.HostIPC {
			entry.RiskScore += 20
			factors = append(factors, "hostIPC enabled")
			result.Summary.HostIPC++
		}
		if podSpec.HostNetwork {
			entry.RiskScore += 15
			factors = append(factors, "hostNetwork enabled")
			result.Summary.HostNetwork++
		}

		// Check each container
		for _, c := range podSpec.Containers {
			sc := c.SecurityContext
			if sc == nil {
				result.Summary.NoSecurityCtx++
				entry.RiskScore += 5
				factors = append(factors, "container "+c.Name+": no securityContext")
				continue
			}

			if sc.Privileged != nil && *sc.Privileged {
				entry.RiskScore += 30
				factors = append(factors, "container "+c.Name+": privileged")
				result.Summary.Privileged++
			}

			if sc.RunAsUser != nil && *sc.RunAsUser == 0 {
				entry.RiskScore += 10
				factors = append(factors, "container "+c.Name+": runs as root (UID 0)")
				result.Summary.RunAsRoot++
			}

			if sc.Capabilities != nil {
				for _, cap := range sc.Capabilities.Add {
					if isDangerousCap1896(string(cap)) {
						entry.RiskScore += 10
						factors = append(factors, "container "+c.Name+": dangerous capability "+string(cap))
						result.Summary.DangerousCaps++
					}
				}
			}
		}

		// Check host path mounts
		for _, vol := range podSpec.Volumes {
			if vol.HostPath != nil {
				entry.RiskScore += 15
				path := vol.HostPath.Path
				if isSensitiveHostPath1896(path) {
					entry.RiskScore += 10
					factors = append(factors, "sensitive hostPath mount: "+path)
				} else {
					factors = append(factors, "hostPath mount: "+path)
				}
				result.Summary.HostPathMounts++
			}
		}

		entry.Factors = factors

		switch {
		case entry.RiskScore >= 30:
			entry.RiskLevel = "high"
			result.Summary.HighRiskCount++
			result.HighRisk = append(result.HighRisk, entry)
		case entry.RiskScore >= 10:
			entry.RiskLevel = "medium"
			result.Summary.MediumRiskCount++
			result.MediumRisk = append(result.MediumRisk, entry)
		default:
			entry.RiskLevel = "low"
			result.Summary.SafeWorkloads++
		}
	}

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		analyze(dep.Name, dep.Namespace, "Deployment", &dep.Spec.Template.Spec)
	}
	sts, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	for _, ss := range sts.Items {
		if isSystemNamespace(ss.Namespace) {
			continue
		}
		analyze(ss.Name, ss.Namespace, "StatefulSet", &ss.Spec.Template.Spec)
	}

	// Build risk factor stats
	result.RiskFactors = buildRiskFactorStats1896(&result.Summary)

	// Score
	if result.Summary.TotalWorkloads > 0 {
		safePct := result.Summary.SafeWorkloads * 100 / result.Summary.TotalWorkloads
		result.HealthScore = safePct
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = buildPodEscapeRecs1896(&result)
	writeJSON(w, result)
}

var dangerousCaps1896 = map[string]bool{
	"SYS_ADMIN":       true,
	"SYS_MODULE":      true,
	"SYS_PTRACE":      true,
	"SYS_RAWIO":       true,
	"NET_ADMIN":       true,
	"NET_RAW":         true,
	"DAC_READ_SEARCH": true,
	"DAC_OVERRIDE":    true,
	"SETUID":          true,
	"SETGID":          true,
	"CHOWN":           true,
	"FOWNER":          true,
}

func isDangerousCap1896(cap string) bool {
	return dangerousCaps1896[strings.ToUpper(cap)]
}

func isSensitiveHostPath1896(path string) bool {
	sensitive := []string{"/etc", "/var/run", "/run", "/proc", "/sys", "/dev", "/root", "/var/lib/docker", "/var/lib/kubelet", "/boot"}
	for _, s := range sensitive {
		if strings.HasPrefix(path, s) {
			return true
		}
	}
	return false
}

func buildRiskFactorStats1896(summary *PodEscapeSummary) []RiskFactorStat {
	return []RiskFactorStat{
		{Factor: "Privileged containers", Count: summary.Privileged},
		{Factor: "HostPID enabled", Count: summary.HostPID},
		{Factor: "HostIPC enabled", Count: summary.HostIPC},
		{Factor: "HostNetwork enabled", Count: summary.HostNetwork},
		{Factor: "HostPath mounts", Count: summary.HostPathMounts},
		{Factor: "Dangerous capabilities", Count: summary.DangerousCaps},
		{Factor: "Running as root (UID 0)", Count: summary.RunAsRoot},
		{Factor: "No securityContext", Count: summary.NoSecurityCtx},
	}
}

func buildPodEscapeRecs1896(result *PodEscapeResult) []string {
	recs := []string{
		fmt.Sprintf("Pod escape risk: %d workloads (%d high risk, %d medium risk, %d safe)",
			result.Summary.TotalWorkloads, result.Summary.HighRiskCount,
			result.Summary.MediumRiskCount, result.Summary.SafeWorkloads),
	}
	if result.Summary.Privileged > 0 {
		recs = append(recs, fmt.Sprintf("%d privileged containers - remove privileged flag and use granular capabilities", result.Summary.Privileged))
	}
	if result.Summary.HostPathMounts > 0 {
		recs = append(recs, fmt.Sprintf("%d hostPath mounts - use PVCs or projected volumes instead", result.Summary.HostPathMounts))
	}
	if result.Summary.RunAsRoot > 0 {
		recs = append(recs, fmt.Sprintf("%d containers run as root - set runAsUser to non-zero UID", result.Summary.RunAsRoot))
	}
	return recs
}

// ---------------------------------------------------------------
// 2. Egress Policy Gap — analyzes missing egress network policies
// ---------------------------------------------------------------

type EgressPolicyResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         EgressPolicySummary `json:"summary"`
	ExposedNS       []EgressNSEntry     `json:"exposedNamespaces"`
	PolicyCoverage  []EgressPolicyEntry `json:"policyCoverage"`
	Recommendations []string            `json:"recommendations"`
}

type EgressPolicySummary struct {
	TotalNamespaces   int `json:"totalNamespaces"`
	WithEgressPolicy  int `json:"withEgressPolicy"`
	WithoutEgress     int `json:"withoutEgressPolicy"`
	TotalNetPols      int `json:"totalNetPols"`
	EgressNetPols     int `json:"egressNetPols"`
	IngressNetPols    int `json:"ingressNetPols"`
	DefaultDenyEgress int `json:"defaultDenyEgress"`
	WideOpenEgress    int `json:"wideOpenEgress"`
}

type EgressNSEntry struct {
	Namespace   string `json:"namespace"`
	HasPolicy   bool   `json:"hasPolicy"`
	DefaultDeny bool   `json:"defaultDeny"`
	PodCount    int    `json:"podCount"`
	RiskLevel   string `json:"riskLevel"`
}

type EgressPolicyEntry struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	HasEgress     bool   `json:"hasEgress"`
	HasIngress    bool   `json:"hasIngress"`
	IsDefaultDeny bool   `json:"isDefaultDeny"`
	PodSelector   string `json:"podSelector"`
}

func (s *Server) handleEgressPolicyGap(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := EgressPolicyResult{ScannedAt: time.Now()}

	// Get all namespaces
	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	nsPodCount := map[string]int{}
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	for _, pod := range pods.Items {
		if !isSystemNamespace(pod.Namespace) {
			nsPodCount[pod.Namespace]++
		}
	}

	// Get all NetworkPolicies
	netpols, _ := rc.clientset.NetworkingV1().NetworkPolicies("").List(ctx, metav1.ListOptions{})
	nsHasEgressPolicy := map[string]bool{}
	nsHasDefaultDeny := map[string]bool{}

	for _, np := range netpols.Items {
		if isSystemNamespace(np.Namespace) {
			continue
		}
		result.Summary.TotalNetPols++

		hasEgress := false
		hasIngress := false

		// Check policy types
		for _, pt := range np.Spec.PolicyTypes {
			if pt == "Egress" {
				hasEgress = true
			}
			if pt == "Ingress" {
				hasIngress = true
			}
		}

		if hasEgress {
			result.Summary.EgressNetPols++
			nsHasEgressPolicy[np.Namespace] = true
			// Check if it's a default deny (no egress rules = deny all)
			if len(np.Spec.Egress) == 0 {
				nsHasDefaultDeny[np.Namespace] = true
				result.Summary.DefaultDenyEgress++
			}
		}
		if hasIngress {
			result.Summary.IngressNetPols++
		}

		result.PolicyCoverage = append(result.PolicyCoverage, EgressPolicyEntry{
			Name:          np.Name,
			Namespace:     np.Namespace,
			HasEgress:     hasEgress,
			HasIngress:    hasIngress,
			IsDefaultDeny: len(np.Spec.Egress) == 0,
			PodSelector:   metav1.FormatLabelSelector(&metav1.LabelSelector{MatchLabels: np.Spec.PodSelector.MatchLabels}),
		})
	}

	// Build namespace coverage
	for _, ns := range namespaces.Items {
		if isSystemNamespace(ns.Name) {
			continue
		}
		result.Summary.TotalNamespaces++

		entry := EgressNSEntry{
			Namespace: ns.Name,
			PodCount:  nsPodCount[ns.Name],
		}

		if nsHasEgressPolicy[ns.Name] {
			entry.HasPolicy = true
			entry.DefaultDeny = nsHasDefaultDeny[ns.Name]
			result.Summary.WithEgressPolicy++
			entry.RiskLevel = "low"
		} else {
			result.Summary.WithoutEgress++
			entry.RiskLevel = "high"
			result.Summary.WideOpenEgress++
		}

		result.ExposedNS = append(result.ExposedNS, entry)
	}
	sort.Slice(result.ExposedNS, func(i, j int) bool {
		return result.ExposedNS[i].RiskLevel == "high" && result.ExposedNS[j].RiskLevel != "high"
	})

	// Score
	if result.Summary.TotalNamespaces > 0 {
		result.HealthScore = result.Summary.WithEgressPolicy * 100 / result.Summary.TotalNamespaces
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = buildEgressRecs1896(&result)
	writeJSON(w, result)
}

func buildEgressRecs1896(result *EgressPolicyResult) []string {
	recs := []string{
		fmt.Sprintf("Egress policy: %d namespaces (%d with policy, %d without), %d total NetPols (%d egress)",
			result.Summary.TotalNamespaces, result.Summary.WithEgressPolicy,
			result.Summary.WithoutEgress, result.Summary.TotalNetPols, result.Summary.EgressNetPols),
	}
	if result.Summary.WithoutEgress > 0 {
		recs = append(recs, fmt.Sprintf("%d namespaces have no egress network policy - pods can connect to any destination", result.Summary.WithoutEgress))
	}
	if result.Summary.DefaultDenyEgress == 0 && result.Summary.WithEgressPolicy > 0 {
		recs = append(recs, "No default-deny egress policy found - apply default-deny + specific allow rules for zero-trust")
	}
	return recs
}

// ---------------------------------------------------------------
// 3. CIS Benchmark Lite — basic CIS Kubernetes Benchmark checks
// ---------------------------------------------------------------

type CISBenchmarkResult struct {
	ScannedAt       time.Time       `json:"scannedAt"`
	HealthScore     int             `json:"healthScore"`
	Grade           string          `json:"grade"`
	Summary         CISMBSummary    `json:"summary"`
	Checks          []CISCheckEntry `json:"checks"`
	Failed          []CISCheckEntry `json:"failed"`
	Warnings        []CISCheckEntry `json:"warnings"`
	ByCategory      map[string]int  `json:"passByCategory"`
	Recommendations []string        `json:"recommendations"`
}

type CISMBSummary struct {
	TotalChecks int `json:"totalChecks"`
	Passed      int `json:"passed"`
	Failed      int `json:"failed"`
	Warn        int `json:"warnings"`
	Score       int `json:"cisScore"`
}

type CISCheckEntry struct {
	ID          string `json:"id"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Status      string `json:"status"` // pass, fail, warn
	Detail      string `json:"detail"`
	Remediation string `json:"remediation"`
}

func (s *Server) handleCISBenchmarkLite(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := CISBenchmarkResult{
		ScannedAt:  time.Now(),
		ByCategory: map[string]int{},
	}

	// CIS 5.1.1: Ensure cluster-admin role only granted to specific users
	crbList, _ := rc.clientset.RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{})
	clusterAdminCount := 0
	for _, crb := range crbList.Items {
		if crb.RoleRef.Name == "cluster-admin" {
			clusterAdminCount += len(crb.Subjects)
		}
	}
	check := func(id, category, desc string, pass bool, detail, remediation string) {
		entry := CISCheckEntry{
			ID:          id,
			Category:    category,
			Description: desc,
			Detail:      detail,
			Remediation: remediation,
		}
		if pass {
			entry.Status = "pass"
			result.Summary.Passed++
			result.ByCategory[category]++
		} else {
			entry.Status = "fail"
			result.Summary.Failed++
			result.Failed = append(result.Failed, entry)
		}
		result.Checks = append(result.Checks, entry)
		result.Summary.TotalChecks++
	}

	warn := func(id, category, desc, detail, remediation string) {
		entry := CISCheckEntry{
			ID:          id,
			Category:    category,
			Description: desc,
			Status:      "warn",
			Detail:      detail,
			Remediation: remediation,
		}
		result.Summary.Warn++
		result.Warnings = append(result.Warnings, entry)
		result.Checks = append(result.Checks, entry)
		result.Summary.TotalChecks++
	}

	// CIS 5.1.1: Cluster-admin restricted
	check("5.1.1", "RBAC", "Ensure cluster-admin role only granted when necessary",
		clusterAdminCount <= 3,
		fmt.Sprintf("%d subjects with cluster-admin", clusterAdminCount),
		"Remove unnecessary cluster-admin bindings")

	// CIS 5.1.5: Ensure default ServiceAccount has minimal privileges
	saList, _ := rc.clientset.CoreV1().ServiceAccounts("").List(ctx, metav1.ListOptions{})
	defaultSACount := 0
	for _, sa := range saList.Items {
		if sa.Name == "default" && !isSystemNamespace(sa.Namespace) {
			if sa.AutomountServiceAccountToken == nil || *sa.AutomountServiceAccountToken {
				defaultSACount++
			}
		}
	}
	check("5.1.5", "RBAC", "Ensure default ServiceAccount auto-mount is disabled",
		defaultSACount == 0,
		fmt.Sprintf("%d default SAs with auto-mount enabled", defaultSACount),
		"Set automountServiceAccountToken: false on default SAs")

	// CIS 5.3.1: Ensure all namespaces have NetworkPolicy
	nsList, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	netpolList, _ := rc.clientset.NetworkingV1().NetworkPolicies("").List(ctx, metav1.ListOptions{})
	nsWithNetPol := map[string]bool{}
	for _, np := range netpolList.Items {
		nsWithNetPol[np.Namespace] = true
	}
	nsWithoutNetPol := 0
	totalUserNS := 0
	for _, ns := range nsList.Items {
		if isSystemNamespace(ns.Name) {
			continue
		}
		totalUserNS++
		if !nsWithNetPol[ns.Name] {
			nsWithoutNetPol++
		}
	}
	check("5.3.1", "Network", "Ensure namespaces have NetworkPolicy",
		nsWithoutNetPol == 0,
		fmt.Sprintf("%d/%d namespaces without NetworkPolicy", nsWithoutNetPol, totalUserNS),
		"Apply default-deny NetworkPolicy to all namespaces")

	// CIS 5.4.1: Prefer using namespaces for isolation
	check("5.4.1", "Network", "Ensure namespaces are used for resource isolation",
		totalUserNS > 1,
		fmt.Sprintf("%d user namespaces", totalUserNS),
		"Organize workloads into separate namespaces")

	// CIS 5.7.1: Ensure privileged containers are not allowed (PSS)
	podEscResult := s.collectPodSecurityCheck1896(ctx, rc)
	check("5.7.1", "Pod Security", "Ensure privileged containers are not deployed",
		podEscResult.privileged == 0,
		fmt.Sprintf("%d privileged containers found", podEscResult.privileged),
		"Remove privileged flag from all containers")

	// CIS 5.7.2: Ensure containers use read-only root filesystem
	check("5.7.2", "Pod Security", "Ensure read-only root filesystem is used",
		podEscResult.readOnlyRoot < podEscResult.totalContainers/2,
		fmt.Sprintf("%d/%d containers without read-only rootFS", podEscResult.readOnlyRoot, podEscResult.totalContainers),
		"Set readOnlyRootFilesystem: true in securityContext")

	// CIS 5.7.3: Ensure containers run as non-root
	check("5.7.3", "Pod Security", "Ensure containers run as non-root user",
		podEscResult.runAsRoot < podEscResult.totalContainers/2,
		fmt.Sprintf("%d/%d containers running as root", podEscResult.runAsRoot, podEscResult.totalContainers),
		"Set runAsNonRoot: true and runAsUser to non-zero UID")

	// CIS 5.7.4: Ensure hostPath volumes are not used
	check("5.7.4", "Pod Security", "Ensure hostPath volumes are not used",
		podEscResult.hostPath == 0,
		fmt.Sprintf("%d hostPath volume mounts", podEscResult.hostPath),
		"Use PersistentVolumeClaims instead of hostPath")

	// CIS 5.1.3: Minimize wildcard RBAC permissions
	wildcardRoles := 0
	roleList, _ := rc.clientset.RbacV1().Roles("").List(ctx, metav1.ListOptions{})
	for _, role := range roleList.Items {
		for _, rule := range role.Rules {
			for _, res := range rule.Resources {
				if res == "*" {
					wildcardRoles++
				}
			}
		}
	}
	check("5.1.3", "RBAC", "Minimize wildcard RBAC permissions",
		wildcardRoles == 0,
		fmt.Sprintf("%d Roles with wildcard (*) resource access", wildcardRoles),
		"Replace wildcard permissions with specific resource grants")

	// Warnings for things we can't directly check
	warn("1.2.1", "API Server", "Ensure --anonymous-auth is set to false",
		"Cannot verify API server flags from within pod",
		"Check kube-apiserver --anonymous-auth=false")

	warn("4.1.1", "Worker Node", "Ensure kubelet uses anonymous-auth=false",
		"Cannot verify kubelet config from within pod",
		"Check kubelet --anonymous-auth=false on each node")

	warn("2.1", "etcd", "Ensure etcd has TLS enabled",
		"Cannot verify etcd configuration from within pod",
		"Verify etcd --cert-file and --key-file are set")

	// Score
	if result.Summary.TotalChecks > 0 {
		result.Summary.Score = result.Summary.Passed * 100 / result.Summary.TotalChecks
	}
	result.HealthScore = result.Summary.Score
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = buildCISRecs1896(&result)
	writeJSON(w, result)
}

type podSecurityCheckData1896 struct {
	totalContainers int
	privileged      int
	readOnlyRoot    int
	runAsRoot       int
	hostPath        int
}

func (s *Server) collectPodSecurityCheck1896(ctx context.Context, rc *requestClients) podSecurityCheckData1896 {
	data := podSecurityCheckData1896{}

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		spec := &dep.Spec.Template.Spec
		for _, vol := range spec.Volumes {
			if vol.HostPath != nil {
				data.hostPath++
			}
		}
		for _, c := range spec.Containers {
			data.totalContainers++
			if c.SecurityContext != nil {
				if c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
					data.privileged++
				}
				if c.SecurityContext.ReadOnlyRootFilesystem == nil || !*c.SecurityContext.ReadOnlyRootFilesystem {
					data.readOnlyRoot++
				}
				if c.SecurityContext.RunAsUser != nil && *c.SecurityContext.RunAsUser == 0 {
					data.runAsRoot++
				}
			} else {
				data.readOnlyRoot++
				data.runAsRoot++
			}
		}
	}
	return data
}

func buildCISRecs1896(result *CISBenchmarkResult) []string {
	recs := []string{
		fmt.Sprintf("CIS Benchmark: %d checks (%d pass, %d fail, %d warn), score %d/100",
			result.Summary.TotalChecks, result.Summary.Passed,
			result.Summary.Failed, result.Summary.Warn, result.Summary.Score),
	}
	if result.Summary.Failed > 0 {
		recs = append(recs, fmt.Sprintf("%d CIS checks failed - review remediation steps for each control", result.Summary.Failed))
	}
	if result.Summary.Warn > 0 {
		recs = append(recs, fmt.Sprintf("%d CIS warnings require manual verification on control plane nodes", result.Summary.Warn))
	}
	return recs
}
