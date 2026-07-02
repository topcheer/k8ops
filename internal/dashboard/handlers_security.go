package dashboard

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SecurityFinding represents a single security issue found during audit.
type SecurityFinding struct {
	Severity  string `json:"severity"`            // critical, high, medium, low, info
	Category  string `json:"category"`             // e.g. "Pod Security", "RBAC", "Network"
	Resource  string `json:"resource"`             // "namespace/kind/name"
	Namespace string `json:"namespace"`
	Detail    string `json:"detail"`
	Fix       string `json:"fix"`
}

// handleSecurityAudit scans the cluster for common security issues.
// GET /api/security/audit
func (s *Server) handleSecurityAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	findings := []SecurityFinding{}

	// 1. Pod Security audit
	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err == nil {
		findings = append(findings, auditPods(pods.Items)...)
	}

	// 2. ServiceAccount audit (check for default SA usage)
	sas, err := rc.clientset.CoreV1().ServiceAccounts("").List(ctx, metav1.ListOptions{})
	if err == nil {
		findings = append(findings, auditServiceAccounts(sas.Items, pods.Items)...)
	}

	// 3. NetworkPolicy coverage audit
	nps, err := rc.clientset.NetworkingV1().NetworkPolicies("").List(ctx, metav1.ListOptions{})
	if err == nil {
		findings = append(findings, auditNetworkPolicies(nps.Items, sas)...)
	}

	// 4. RBAC audit (cluster-admin bindings)
	roleBindings, _ := rc.clientset.RbacV1().RoleBindings("").List(ctx, metav1.ListOptions{})
	clusterRoleBindings, _ := rc.clientset.RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{})
	findings = append(findings, auditRBAC(roleBindings.Items, clusterRoleBindings.Items)...)

	// 5. Secret audit (count + image pull secrets)
	secrets, err := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	if err == nil {
		findings = append(findings, auditSecrets(secrets.Items)...)
	}

	// Sort by severity (critical first)
	sevOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3, "info": 4}
	sort.Slice(findings, func(i, j int) bool {
		if sevOrder[findings[i].Severity] != sevOrder[findings[j].Severity] {
			return sevOrder[findings[i].Severity] < sevOrder[findings[j].Severity]
		}
		return findings[i].Resource < findings[j].Resource
	})

	// Summary
	summary := map[string]int{
		"critical": 0, "high": 0, "medium": 0, "low": 0, "info": 0, "total": 0,
	}
	for _, f := range findings {
		summary[f.Severity]++
		summary["total"]++
	}

	writeJSON(w, map[string]any{
		"summary":  summary,
		"findings": findings,
		"scannedAt": time.Now().Format(time.RFC3339),
	})
}

// auditPods checks each pod for Pod Security Standards violations.
func auditPods(pods []corev1.Pod) []SecurityFinding {
	var findings []SecurityFinding

	skipSystem := func(ns string) bool {
		systemNs := map[string]bool{
			"kube-system": true, "kube-public": true, "kube-node-lease": true,
		}
		return systemNs[ns]
	}

	for _, pod := range pods {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		ns := pod.Namespace
		podName := pod.Name
		resource := fmt.Sprintf("%s/pod/%s", ns, podName)
		isSystem := skipSystem(ns)

		for _, c := range pod.Spec.Containers {
			cName := c.Name
			sc := c.SecurityContext

			// Privileged container
			if sc != nil && sc.Privileged != nil && *sc.Privileged {
				sev := "critical"
				if isSystem {
					sev = "info"
				}
				findings = append(findings, SecurityFinding{
					Severity: sev, Category: "Pod Security",
					Resource:  fmt.Sprintf("%s/container/%s", resource, cName),
					Namespace: ns,
					Detail:    fmt.Sprintf("Container %q in pod %q runs privileged", cName, podName),
					Fix:       "Remove privileged: true or set to false unless absolutely required",
				})
			}

			// Privilege escalation
			if sc != nil && sc.AllowPrivilegeEscalation != nil && *sc.AllowPrivilegeEscalation {
				sev := "high"
				if isSystem {
					sev = "info"
				}
				findings = append(findings, SecurityFinding{
					Severity: sev, Category: "Pod Security",
					Resource:  fmt.Sprintf("%s/container/%s", resource, cName),
					Namespace: ns,
					Detail:    fmt.Sprintf("Container %q allows privilege escalation", cName),
					Fix:       "Set allowPrivilegeEscalation: false in securityContext",
				})
			}

			// Running as root (UID 0 or no non-root setting)
			runsAsRoot := false
			if sc != nil && sc.RunAsUser != nil && *sc.RunAsUser == 0 {
				runsAsRoot = true
			}
			// Check pod-level too
			podSC := pod.Spec.SecurityContext
			if podSC != nil && podSC.RunAsUser != nil && *podSC.RunAsUser == 0 {
				runsAsRoot = true
			}
			// If neither pod nor container sets RunAsNonRoot or RunAsUser, flag as potential root
			if sc == nil || (sc.RunAsUser == nil && sc.RunAsNonRoot == nil) {
				if podSC == nil || (podSC.RunAsUser == nil && podSC.RunAsNonRoot == nil) {
					runsAsRoot = true // ambiguous: defaults to root
				}
			}
			if runsAsRoot && !isSystem {
				findings = append(findings, SecurityFinding{
					Severity: "medium", Category: "Pod Security",
					Resource:  fmt.Sprintf("%s/container/%s", resource, cName),
					Namespace: ns,
					Detail:    fmt.Sprintf("Container %q may run as root (UID 0 or unset)", cName),
					Fix:       "Set runAsNonRoot: true and specify a non-zero runAsUser in securityContext",
				})
			}

			// No resource limits
			if c.Resources.Limits.Cpu().IsZero() && c.Resources.Limits.Memory().IsZero() {
				findings = append(findings, SecurityFinding{
					Severity: "low", Category: "Resource Governance",
					Resource:  fmt.Sprintf("%s/container/%s", resource, cName),
					Namespace: ns,
					Detail:    fmt.Sprintf("Container %q has no resource limits set", cName),
					Fix:       "Set resource.limits for CPU and memory to prevent resource exhaustion",
				})
			}

			// Capabilities: NET_RAW or ALL
			if sc != nil && sc.Capabilities != nil {
				for _, cap := range sc.Capabilities.Add {
					if cap == "ALL" || cap == "NET_RAW" || cap == "SYS_ADMIN" {
						findings = append(findings, SecurityFinding{
							Severity: "high", Category: "Pod Security",
							Resource:  fmt.Sprintf("%s/container/%s", resource, cName),
							Namespace: ns,
							Detail:    fmt.Sprintf("Container %q adds dangerous capability %q", cName, cap),
							Fix:       "Drop unnecessary capabilities; use capabilities.drop to remove defaults",
						})
						break
					}
				}
			}

			// Host network/pid/ipc
			if pod.Spec.HostNetwork || pod.Spec.HostPID || pod.Spec.HostIPC {
				if !isSystem {
					findings = append(findings, SecurityFinding{
						Severity: "high", Category: "Pod Security",
						Resource:  resource,
						Namespace: ns,
						Detail:    fmt.Sprintf("Pod %q uses host network/PID/IPC namespace", podName),
						Fix:       "Avoid hostNetwork, hostPID, and hostIPC unless required (e.g., CNI plugins)",
					})
				}
			}

			// HostPath volumes
			for _, vol := range pod.Spec.Volumes {
				if vol.HostPath != nil && !isSystem {
					findings = append(findings, SecurityFinding{
						Severity: "high", Category: "Pod Security",
						Resource:  resource,
						Namespace: ns,
						Detail:    fmt.Sprintf("Pod %q mounts hostPath %q -> %q", podName, vol.HostPath.Path, vol.Name),
						Fix:       "Avoid hostPath volumes; use PersistentVolumeClaims or projected volumes",
					})
					break
				}
			}

			// readOnlyRootFilesystem not set
			if sc == nil || sc.ReadOnlyRootFilesystem == nil || !*sc.ReadOnlyRootFilesystem {
				if !isSystem {
					findings = append(findings, SecurityFinding{
						Severity: "low", Category: "Pod Security",
						Resource:  fmt.Sprintf("%s/container/%s", resource, cName),
						Namespace: ns,
						Detail:    fmt.Sprintf("Container %q has writable root filesystem", cName),
						Fix:       "Set readOnlyRootFilesystem: true for defense in depth",
					})
				}
			}
		}
	}

	return findings
}

// auditServiceAccounts checks for default SA usage in pods.
func auditServiceAccounts(sas []corev1.ServiceAccount, pods []corev1.Pod) []SecurityFinding {
	var findings []SecurityFinding

	// Find pods using default service account
	for _, pod := range pods {
		if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		saName := pod.Spec.ServiceAccountName
		if saName == "" {
			saName = "default"
		}
		if saName == "default" && !isSystemNamespace(pod.Namespace) {
			findings = append(findings, SecurityFinding{
				Severity: "low", Category: "RBAC",
				Resource:  fmt.Sprintf("%s/pod/%s", pod.Namespace, pod.Name),
				Namespace: pod.Namespace,
				Detail:    fmt.Sprintf("Pod %q uses the default service account", pod.Name),
				Fix:       "Create a dedicated service account with least-privilege permissions",
			})
		}
	}

	// Check for SA with auto-mounted token where it shouldn't be
	for _, sa := range sas {
		if sa.AutomountServiceAccountToken == nil || *sa.AutomountServiceAccountToken {
			// Default is true, just flag as info
			if !isSystemNamespace(sa.Namespace) && sa.Name != "default" {
				findings = append(findings, SecurityFinding{
					Severity: "info", Category: "RBAC",
					Resource:  fmt.Sprintf("%s/serviceaccount/%s", sa.Namespace, sa.Name),
					Namespace: sa.Namespace,
					Detail:    fmt.Sprintf("ServiceAccount %q has automountServiceAccountToken enabled", sa.Name),
					Fix:       "Set automountServiceAccountToken: false if the pod doesn't need API access",
				})
			}
		}
	}

	return findings
}

// auditNetworkPolicies checks for namespaces without network policies.
func auditNetworkPolicies(nps []networkingv1.NetworkPolicy, sas *corev1.ServiceAccountList) []SecurityFinding {
	var findings []SecurityFinding

	// Count policies per namespace
	nsPolicies := map[string]int{}
	for _, np := range nps {
		nsPolicies[np.Namespace]++
	}

	// Get all namespaces from SAs (rough approximation) - better to list namespaces
	// Use the SA list to discover namespaces
	nsSet := map[string]bool{}
	if sas != nil {
		for _, sa := range sas.Items {
			nsSet[sa.Namespace] = true
		}
	}

	for ns := range nsSet {
		if isSystemNamespace(ns) {
			continue
		}
		if nsPolicies[ns] == 0 {
			findings = append(findings, SecurityFinding{
				Severity: "medium", Category: "Network",
				Resource:  fmt.Sprintf("%s/networkpolicy", ns),
				Namespace: ns,
				Detail:    fmt.Sprintf("Namespace %q has no NetworkPolicies — all pods can communicate freely", ns),
				Fix:       "Add a default-deny NetworkPolicy and selectively allow required traffic",
			})
		}
	}

	return findings
}

// auditRBAC checks for overly permissive RBAC bindings.
func auditRBAC(roleBindings []rbacv1.RoleBinding, clusterRoleBindings []rbacv1.ClusterRoleBinding) []SecurityFinding {
	var findings []SecurityFinding

	// Check for cluster-admin bindings
	for _, crb := range clusterRoleBindings {
		if crb.RoleRef.Name == "cluster-admin" {
			for _, subj := range crb.Subjects {
				sev := "high"
				if subj.Kind == "Group" && (subj.Name == "system:masters" || strings.HasPrefix(subj.Name, "system:")) {
					sev = "info" // system binding, expected
				}
				findings = append(findings, SecurityFinding{
					Severity: sev, Category: "RBAC",
					Resource:  fmt.Sprintf("clusterrolebinding/%s", crb.Name),
					Namespace: "",
					Detail:    fmt.Sprintf("ClusterRoleBinding %q grants cluster-admin to %s/%s", crb.Name, subj.Kind, subj.Name),
					Fix:       "Review if this binding is necessary; prefer namespace-scoped roles",
				})
			}
		}
	}

	// Check for bindings to wildcard permissions (verbs=* or resources=*)
	for _, crb := range clusterRoleBindings {
		if crb.RoleRef.Name == "cluster-admin" {
			continue // already reported above
		}
		// Flag bindings to roles with * verbs (we'd need to fetch the role, so just flag wildcard role names as info)
	}

	return findings
}

// auditSecrets checks for security-sensitive secret types and practices.
func auditSecrets(secrets []corev1.Secret) []SecurityFinding {
	var findings []SecurityFinding

	// Count by type for info
	typeCounts := map[corev1.SecretType]int{}
	for _, sec := range secrets {
		typeCounts[sec.Type]++
	}

	// Flag any Docker registry secrets in non-system namespaces for info
	for _, sec := range secrets {
		if sec.Type == corev1.SecretTypeDockerConfigJson || sec.Type == corev1.SecretTypeDockercfg {
			if !isSystemNamespace(sec.Namespace) {
				findings = append(findings, SecurityFinding{
					Severity: "info", Category: "Secrets",
					Resource:  fmt.Sprintf("%s/secret/%s", sec.Namespace, sec.Name),
					Namespace: sec.Namespace,
					Detail:    fmt.Sprintf("Docker registry secret %q found — ensure it's rotated regularly", sec.Name),
					Fix:       "Rotate registry credentials periodically and use workload identity where possible",
				})
			}
		}
	}

	return findings
}

func isSystemNamespace(ns string) bool {
	switch ns {
	case "kube-system", "kube-public", "kube-node-lease":
		return true
	default:
		return false
	}
}

// handleSecurityScan performs a deep health check of the k8ops dashboard itself.
// GET /api/security/health
func (s *Server) handleSecurityHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	rc := s.clientsFromReq(r)

	health := map[string]any{
		"timestamp": time.Now().Format(time.RFC3339),
		"checks":    map[string]any{},
	}

	// Check k8s API connectivity
	checks := health["checks"].(map[string]any)
	if rc != nil && rc.clientset != nil {
		_, err := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{Limit: 1})
		if err != nil {
			checks["k8sAPI"] = map[string]any{"status": "unhealthy", "error": err.Error()}
		} else {
			checks["k8sAPI"] = map[string]any{"status": "healthy"}
		}
	} else {
		checks["k8sAPI"] = map[string]any{"status": "not-configured"}
	}

	// Check auth status
	if s.auth != nil {
		checks["auth"] = map[string]any{"status": "enabled"}
	} else {
		checks["auth"] = map[string]any{"status": "disabled", "warning": "Authentication is not configured"}
	}

	// Check TLS
	if s.IsTLS() {
		checks["tls"] = map[string]any{"status": "enabled"}
	} else {
		checks["tls"] = map[string]any{"status": "disabled", "warning": "TLS is not configured; rely on ingress TLS"}
	}

	// Overall status
	allHealthy := true
	for _, v := range checks {
		if m, ok := v.(map[string]any); ok {
			if status, ok := m["status"].(string); ok && status != "healthy" && status != "enabled" {
				allHealthy = false
			}
		}
	}
	if allHealthy {
		health["status"] = "healthy"
	} else {
		health["status"] = "degraded"
	}

	writeJSON(w, health)
}
