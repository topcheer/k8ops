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

// ComplianceCheck represents a single CIS Benchmark control check result.
type ComplianceCheck struct {
	ID          string `json:"id"`
	Category    string `json:"category"`
	Title       string `json:"title"`
	Status      string `json:"status"` // pass, fail, warn, skip
	Description string `json:"description"`
	Remediation string `json:"remediation"`
}

// runComplianceChecks executes all CIS-style checks against the cluster and returns results.
func runComplianceChecks(ctx context.Context, rc *requestClients) []ComplianceCheck {
	var checks []ComplianceCheck

	// === CIS 5.1: RBAC ===
	crb, err := rc.clientset.RbacV1().ClusterRoleBindings().List(ctx, metav1.ListOptions{})
	if err == nil {
		adminCount := 0
		for _, binding := range crb.Items {
			if binding.RoleRef.Name == "cluster-admin" {
				for _, subj := range binding.Subjects {
					if !strings.HasPrefix(subj.Name, "system:") {
						adminCount++
					}
				}
			}
		}
		status := "pass"
		remediation := "Cluster-admin role assignment is appropriate."
		if adminCount > 3 {
			status = "fail"
			remediation = fmt.Sprintf("Found %d non-system cluster-admin bindings. Reduce to minimum necessary (ideally <= 3).", adminCount)
		} else if adminCount > 1 {
			status = "warn"
			remediation = fmt.Sprintf("%d non-system cluster-admin bindings found. Review if all are necessary.", adminCount)
		}
		checks = append(checks, ComplianceCheck{
			ID: "5.1.1", Category: "RBAC", Status: status,
			Title:       "Cluster-admin role binding scope",
			Description: fmt.Sprintf("Ensure cluster-admin is not broadly assigned (%d non-system bindings)", adminCount),
			Remediation: remediation,
		})
	}

	// 5.1.3 — Minimize wildcard ClusterRoles
	crList, err := rc.clientset.RbacV1().ClusterRoles().List(ctx, metav1.ListOptions{})
	if err == nil {
		wildcardCount := 0
		for _, role := range crList.Items {
			if strings.HasPrefix(role.Name, "system:") {
				continue
			}
			for _, rule := range role.Rules {
				for _, verb := range rule.Verbs {
					if verb == "*" {
						wildcardCount++
					}
				}
			}
		}
		status := "pass"
		remediation := "No unnecessary wildcard ClusterRoles."
		if wildcardCount > 5 {
			status = "warn"
			remediation = fmt.Sprintf("%d wildcard verbs in ClusterRoles. Prefer least-privilege with specific verbs.", wildcardCount)
		}
		checks = append(checks, ComplianceCheck{
			ID: "5.1.3", Category: "RBAC", Status: status,
			Title:       "Minimize wildcard ClusterRoles",
			Description: fmt.Sprintf("Ensure ClusterRoles with wildcard (*) verbs are minimized (%d found)", wildcardCount),
			Remediation: remediation,
		})
	}

	// === CIS 5.2: Pod Security ===
	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	var podItems []corev1.Pod
	if err == nil {
		podItems = pods.Items
		privilegedCount := 0
		rootCount := 0
		noLimitCount := 0
		hostNetCount := 0
		hostPathCount := 0
		for _, pod := range podItems {
			if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
				continue
			}
			isSystem := isSystemNamespace(pod.Namespace)
			for _, c := range pod.Spec.Containers {
				if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
					if !isSystem {
						privilegedCount++
					}
				}
				if !isSystem {
					runsRoot := true
					if c.SecurityContext != nil && c.SecurityContext.RunAsUser != nil && *c.SecurityContext.RunAsUser != 0 {
						runsRoot = false
					}
					if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.RunAsUser != nil && *pod.Spec.SecurityContext.RunAsUser != 0 {
						runsRoot = false
					}
					if runsRoot {
						rootCount++
					}
					if c.Resources.Limits.Cpu().IsZero() && c.Resources.Limits.Memory().IsZero() {
						noLimitCount++
					}
				}
			}
			if !isSystem {
				if pod.Spec.HostNetwork || pod.Spec.HostPID || pod.Spec.HostIPC {
					hostNetCount++
				}
				for _, vol := range pod.Spec.Volumes {
					if vol.HostPath != nil {
						hostPathCount++
						break
					}
				}
			}
		}

		// 5.2.1 — Privileged containers
		status := "pass"
		remediation := "No privileged containers found in non-system namespaces."
		if privilegedCount > 0 {
			status = "fail"
			remediation = fmt.Sprintf("%d privileged containers found. Set privileged: false in all non-system pods.", privilegedCount)
		}
		checks = append(checks, ComplianceCheck{
			ID: "5.2.1", Category: "Pod Security", Status: status,
			Title:       "Minimize privileged containers",
			Description: fmt.Sprintf("Ensure privileged containers are not used (%d found)", privilegedCount),
			Remediation: remediation,
		})

		// 5.2.2 — Host namespaces
		status = "pass"
		remediation = "No pods using host network/PID/IPC namespaces."
		if hostNetCount > 0 {
			status = "warn"
			remediation = fmt.Sprintf("%d pods use host network/PID/IPC. Restrict to system components only.", hostNetCount)
		}
		checks = append(checks, ComplianceCheck{
			ID: "5.2.2", Category: "Pod Security", Status: status,
			Title:       "Minimize hostNetwork/hostPID/hostIPC",
			Description: fmt.Sprintf("Ensure pods do not use host namespaces (%d found)", hostNetCount),
			Remediation: remediation,
		})

		// 5.2.3 — HostPath volumes
		status = "pass"
		remediation = "No hostPath volume mounts in non-system namespaces."
		if hostPathCount > 0 {
			status = "warn"
			remediation = fmt.Sprintf("%d pods mount hostPath volumes. Use PersistentVolumeClaims instead.", hostPathCount)
		}
		checks = append(checks, ComplianceCheck{
			ID: "5.2.3", Category: "Pod Security", Status: status,
			Title:       "Minimize hostPath volumes",
			Description: fmt.Sprintf("Ensure hostPath volumes are not used (%d found)", hostPathCount),
			Remediation: remediation,
		})

		// 5.2.4 — Non-root containers
		status = "pass"
		remediation = "All containers specify non-root user."
		if rootCount > 0 {
			status = "warn"
			remediation = fmt.Sprintf("%d containers may run as root. Set runAsNonRoot: true.", rootCount)
		}
		checks = append(checks, ComplianceCheck{
			ID: "5.2.4", Category: "Pod Security", Status: status,
			Title:       "Minimize containers running as root",
			Description: fmt.Sprintf("Ensure containers run as non-root user (%d potential)", rootCount),
			Remediation: remediation,
		})

		// 5.2.5 — Resource limits
		status = "pass"
		remediation = "All containers have resource limits set."
		if noLimitCount > 10 {
			status = "warn"
			remediation = fmt.Sprintf("%d containers without resource limits. Set limits to prevent resource exhaustion.", noLimitCount)
		}
		checks = append(checks, ComplianceCheck{
			ID: "5.2.5", Category: "Resource Governance", Status: status,
			Title:       "Ensure resource limits are set",
			Description: fmt.Sprintf("Ensure all containers have resource limits (%d missing)", noLimitCount),
			Remediation: remediation,
		})
	}

	// === CIS 5.3: Network Policies ===
	nss, err := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err == nil {
		nps, _ := rc.clientset.NetworkingV1().NetworkPolicies("").List(ctx, metav1.ListOptions{})
		nsWithPolicy := map[string]bool{}
		if nps != nil {
			for _, np := range nps.Items {
				nsWithPolicy[np.Namespace] = true
			}
		}
		nsWithoutPolicy := 0
		for _, ns := range nss.Items {
			if !isSystemNamespace(ns.Name) && !nsWithPolicy[ns.Name] {
				nsWithoutPolicy++
			}
		}
		status := "pass"
		remediation := "All non-system namespaces have NetworkPolicies."
		if nsWithoutPolicy > 0 {
			status = "warn"
			remediation = fmt.Sprintf("%d namespaces lack NetworkPolicies. Add default-deny policies.", nsWithoutPolicy)
		}
		checks = append(checks, ComplianceCheck{
			ID: "5.3.1", Category: "Network", Status: status,
			Title:       "Ensure NetworkPolicies exist",
			Description: fmt.Sprintf("Ensure namespaces have NetworkPolicies (%d without)", nsWithoutPolicy),
			Remediation: remediation,
		})
	}

	// === CIS 5.4: Secrets ===
	secrets, err := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	if err == nil {
		saTokenCount := 0
		opaqueCount := 0
		for _, sec := range secrets.Items {
			if sec.Type == corev1.SecretTypeServiceAccountToken {
				saTokenCount++
			}
			if sec.Type == corev1.SecretTypeOpaque {
				opaqueCount++
			}
		}
		status := "pass"
		remediation := "Secret management looks healthy."
		if opaqueCount > 50 {
			status = "warn"
			remediation = fmt.Sprintf("%d opaque secrets — consider externalizing to a secrets manager (Vault, Sealed Secrets).", opaqueCount)
		}
		checks = append(checks, ComplianceCheck{
			ID: "5.4.1", Category: "Secrets", Status: status,
			Title:       "Secret management hygiene",
			Description: fmt.Sprintf("Review secret types (%d SA tokens, %d opaque)", saTokenCount, opaqueCount),
			Remediation: remediation,
		})
	}

	// === CIS 5.7: Default Service Accounts ===
	if len(podItems) > 0 {
		defaultSACount := 0
		for _, pod := range podItems {
			if pod.Status.Phase == corev1.PodSucceeded || pod.Status.Phase == corev1.PodFailed {
				continue
			}
			saName := pod.Spec.ServiceAccountName
			if saName == "" || saName == "default" {
				if !isSystemNamespace(pod.Namespace) {
					defaultSACount++
				}
			}
		}
		status := "pass"
		remediation := "No pods using default service account."
		if defaultSACount > 5 {
			status = "warn"
			remediation = fmt.Sprintf("%d pods use the default service account. Create dedicated SAs.", defaultSACount)
		}
		checks = append(checks, ComplianceCheck{
			ID: "5.7.1", Category: "RBAC", Status: status,
			Title:       "Minimize default SA usage",
			Description: fmt.Sprintf("Ensure pods use dedicated service accounts (%d using default)", defaultSACount),
			Remediation: remediation,
		})
	}

	// Sort by CIS ID
	sort.Slice(checks, func(i, j int) bool {
		return checks[i].ID < checks[j].ID
	})

	return checks
}

// handleComplianceScan runs CIS-style compliance checks against the cluster.
// GET /api/security/compliance
func (s *Server) handleComplianceScan(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil || s.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	checks := runComplianceChecks(ctx, rc)

	// Build summary
	summary := map[string]int{"pass": 0, "fail": 0, "warn": 0, "skip": 0, "total": len(checks)}
	for _, c := range checks {
		summary[c.Status]++
	}

	score := 100
	if summary["total"] > 0 {
		score = summary["pass"] * 100 / summary["total"]
	}

	writeJSON(w, map[string]any{
		"summary":   summary,
		"score":     score,
		"scannedAt": time.Now().Format(time.RFC3339),
		"benchmark": "CIS Kubernetes Benchmark (simplified)",
		"checks":    checks,
		"reportUrl": "/api/security/compliance/report",
	})
}

// handleComplianceReport generates a downloadable text compliance report.
// GET /api/security/compliance/report
func (s *Server) handleComplianceReport(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil || s.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	checks := runComplianceChecks(ctx, rc)
	summary := map[string]int{"pass": 0, "fail": 0, "warn": 0, "skip": 0, "total": len(checks)}
	for _, c := range checks {
		summary[c.Status]++
	}
	score := 100
	if summary["total"] > 0 {
		score = summary["pass"] * 100 / summary["total"]
	}

	w.Header().Set("Content-Type", "text/plain")
	w.Header().Set("Content-Disposition", `attachment; filename="k8ops-compliance-report.txt"`)

	fmt.Fprintf(w, "========================================================\n")
	fmt.Fprintf(w, "  k8ops Compliance Report\n")
	fmt.Fprintf(w, "  CIS Kubernetes Benchmark (simplified)\n")
	fmt.Fprintf(w, "  Generated: %s\n", time.Now().Format(time.RFC3339))
	fmt.Fprintf(w, "========================================================\n\n")
	fmt.Fprintf(w, "Compliance Score: %d%%\n", score)
	fmt.Fprintf(w, "Total Checks: %d  |  Pass: %d  |  Warn: %d  |  Fail: %d\n\n", summary["total"], summary["pass"], summary["warn"], summary["fail"])

	currentCategory := ""
	for _, c := range checks {
		if c.Category != currentCategory {
			currentCategory = c.Category
			fmt.Fprintf(w, "\n--- %s ---\n\n", c.Category)
		}
		icon := "[PASS]"
		switch c.Status {
		case "fail":
			icon = "[FAIL]"
		case "warn":
			icon = "[WARN]"
		}
		fmt.Fprintf(w, "%s CIS %s: %s\n", icon, c.ID, c.Title)
		fmt.Fprintf(w, "     %s\n", c.Description)
		if c.Status != "pass" {
			fmt.Fprintf(w, "     Remediation: %s\n", c.Remediation)
		}
		fmt.Fprintf(w, "\n")
	}

	fmt.Fprintf(w, "\n========================================================\n")
	fmt.Fprintf(w, "  End of Report\n")
	fmt.Fprintf(w, "========================================================\n")
}
