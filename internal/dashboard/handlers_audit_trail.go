package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AuditTrailResult analyzes Kubernetes audit logging coverage and compliance trail.
type AuditTrailResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	Summary         AuditTrailSummary `json:"summary"`
	Gaps            []AuditGapItem    `json:"gaps"`
	ComplianceScore int               `json:"complianceScore"`
	Grade           string            `json:"grade"`
	Recommendations []string          `json:"recommendations"`
}

type AuditTrailSummary struct {
	AuditLogEnabled    bool   `json:"auditLogEnabled"`
	LogBackend         string `json:"logBackend"`
	MaxRetention       string `json:"maxRetention"`
	NamespacesTracked  int    `json:"namespacesTracked"`
	EventsWithoutTrail int    `json:"eventsWithoutTrail"`
	SensitiveAccess    int    `json:"sensitiveAccess"`
}

type AuditGapItem struct {
	Category string `json:"category"`
	Gap      string `json:"gap"`
	Severity string `json:"severity"`
	Impact   string `json:"impact"`
}

// handleAuditTrail analyzes Kubernetes audit logging coverage.
// GET /api/security/audit-trail
func (s *Server) handleAuditTrail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := AuditTrailResult{ScannedAt: time.Now()}
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

	nsList, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	configmaps, _ := rc.clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{})

	// Detect audit logging backend
	auditKeywords := map[string]string{
		"fluent-bit": "Fluent Bit", "fluentd": "Fluentd", "vector": "Vector",
		"elasticsearch": "Elasticsearch", "loki": "Loki", "splunk": "Splunk",
		"datadog": "Datadog", "cloudwatch": "CloudWatch",
	}
	detectedBackend := ""
	_ = detectedBackend
	for _, pod := range pods.Items {
		for _, c := range pod.Spec.Containers {
			imgLower := strings.ToLower(c.Image)
			for kw, backend := range auditKeywords {
				if strings.Contains(imgLower, kw) {
					detectedBackend = backend
					result.Summary.AuditLogEnabled = true
					result.Summary.LogBackend = backend
				}
			}
		}
	}

	// Count tracked namespaces
	for _, ns := range nsList.Items {
		if systemNS[ns.Name] {
			continue
		}
		result.Summary.NamespacesTracked++
	}

	// Detect sensitive access patterns
	sensitiveAccess := 0
	for _, dep := range deployments.Items {
		if systemNS[dep.Namespace] {
			continue
		}
		for _, c := range dep.Spec.Template.Spec.Containers {
			if c.SecurityContext != nil && c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
				sensitiveAccess++
			}
		}
		// Check for secrets access
		for _, vol := range dep.Spec.Template.Spec.Volumes {
			if vol.Secret != nil {
				sensitiveAccess++
			}
		}
	}
	result.Summary.SensitiveAccess = sensitiveAccess

	// Gaps
	if !result.Summary.AuditLogEnabled {
		result.Gaps = append(result.Gaps, AuditGapItem{
			Category: "log-collection", Gap: "No audit log collector detected",
			Severity: "critical", Impact: "API server audit events not being captured or forwarded",
		})
	}

	// Check for audit policy configmap
	auditPolicyFound := false
	for _, cm := range configmaps.Items {
		if strings.Contains(strings.ToLower(cm.Name), "audit") {
			auditPolicyFound = true
			break
		}
	}
	if !auditPolicyFound {
		result.Gaps = append(result.Gaps, AuditGapItem{
			Category: "audit-policy", Gap: "No audit policy ConfigMap found",
			Severity: "medium", Impact: "Default audit policy may miss sensitive operations",
		})
	}

	if sensitiveAccess > 5 {
		result.Gaps = append(result.Gaps, AuditGapItem{
			Category: "sensitive-access", Gap: fmt.Sprintf("%d sensitive access points without dedicated audit trail", sensitiveAccess),
			Severity: "high", Impact: "Privileged pods and secret access should trigger audit alerts",
		})
	}

	result.Summary.MaxRetention = "unknown"

	// Score
	score := 0
	if result.Summary.AuditLogEnabled {
		score += 40
	}
	if auditPolicyFound {
		score += 20
	}
	if score == 0 {
		score = 10
	}
	score += 20 // base for K8s default audit log
	if sensitiveAccess > 10 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.ComplianceScore = min(100, score)
	result.Grade = goldenScoreToGrade(result.ComplianceScore)

	sort.Slice(result.Gaps, func(i, j int) bool {
		return result.Gaps[i].Severity > result.Gaps[j].Severity
	})

	var recs []string
	recs = append(recs, fmt.Sprintf("Audit trail compliance: %d/100 (grade %s) — backend: %s", result.ComplianceScore, result.Grade, result.Summary.LogBackend))
	if !result.Summary.AuditLogEnabled {
		recs = append(recs, "No audit log collector — install Fluent Bit/Vector to forward K8s audit logs")
	}
	if !auditPolicyFound {
		recs = append(recs, "Configure audit policy to capture RequestResponse for sensitive resources (secrets, rbac)")
	}
	if sensitiveAccess > 5 {
		recs = append(recs, fmt.Sprintf("%d sensitive access points — enable dedicated audit alerts", sensitiveAccess))
	}
	if len(recs) == 1 {
		recs = append(recs, "Audit trail coverage is comprehensive")
	}
	result.Recommendations = recs

	writeJSON(w, result)
}
