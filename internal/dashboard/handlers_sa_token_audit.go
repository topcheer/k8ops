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

// SATokenAuditResult is the ServiceAccount token & rotation risk audit.
type SATokenAuditResult struct {
	ScannedAt       time.Time       `json:"scannedAt"`
	Summary         SATokenSummary  `json:"summary"`
	ByNamespace     []SATokenNSStat `json:"byNamespace"`
	ServiceAccounts []SATokenEntry  `json:"serviceAccounts"`
	Issues          []SATokenIssue  `json:"issues"`
	Recommendations []string        `json:"recommendations"`
	HealthScore     int             `json:"healthScore"`
}

// SATokenSummary aggregates SA token audit statistics.
type SATokenSummary struct {
	TotalSAs          int `json:"totalServiceAccounts"`
	AutoMountEnabled  int `json:"autoMountEnabled"`
	AutoMountDisabled int `json:"autoMountDisabled"`
	WithTokens        int `json:"withTokens"`
	LongLivedTokens   int `json:"longLivedTokens"`
	NoSecretRef       int `json:"noSecretRef"`
	DefaultSAUsed     int `json:"defaultSAUsed"`
}

// SATokenNSStat per-namespace SA stats.
type SATokenNSStat struct {
	Namespace   string `json:"namespace"`
	SACount     int    `json:"saCount"`
	AutoMountOn int    `json:"autoMountOn"`
	RiskCount   int    `json:"riskCount"`
}

// SATokenEntry describes one ServiceAccount's token configuration.
type SATokenEntry struct {
	Name           string `json:"name"`
	Namespace      string `json:"namespace"`
	AutoMountToken bool   `json:"autoMountToken"`
	HasSecretRef   bool   `json:"hasSecretRef"`
	SecretName     string `json:"secretName,omitempty"`
	SecretAge      string `json:"secretAge,omitempty"`
	IsDefault      bool   `json:"isDefault"`
	HasClusterRole bool   `json:"hasClusterRoleBinding"`
	PodUsageCount  int    `json:"podUsageCount"`
	RiskLevel      string `json:"riskLevel"`
}

// SATokenIssue is a detected SA token problem.
type SATokenIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleSATokenAudit audits ServiceAccount token rotation and access risk.
// GET /api/security/sa-token-audit
func (s *Server) handleSATokenAudit(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	ctx := r.Context()

	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := &SATokenAuditResult{
		ScannedAt: time.Now(),
	}

	// 1. List all ServiceAccounts
	sas, err := rc.clientset.CoreV1().ServiceAccounts("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// 2. List all Secrets to find SA tokens
	secrets, err := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Build secret map: namespace/name -> secret creation time
	secretAgeMap := make(map[string]time.Time)
	for i := range secrets.Items {
		sec := &secrets.Items[i]
		if sec.Type == corev1.SecretTypeServiceAccountToken {
			key := fmt.Sprintf("%s/%s", sec.Namespace, sec.Name)
			secretAgeMap[key] = sec.CreationTimestamp.Time
		}
	}

	// 3. List all pods to count SA usage
	pods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err == nil {
		// Build SA usage map: namespace/saName -> pod count
	}

	saUsageMap := make(map[string]int)
	if pods != nil {
		for i := range pods.Items {
			pod := &pods.Items[i]
			saRef := pod.Spec.ServiceAccountName
			if saRef == "" {
				saRef = "default"
			}
			key := fmt.Sprintf("%s/%s", pod.Namespace, saRef)
			saUsageMap[key]++
		}
	}

	var entries []SATokenEntry
	var issues []SATokenIssue
	nsStats := make(map[string]*SATokenNSStat)

	autoMountOn := 0
	autoMountOff := 0
	withTokens := 0
	longLived := 0
	noSecretRef := 0
	defaultUsed := 0

	for i := range sas.Items {
		sa := &sas.Items[i]
		if isSystemNamespace(sa.Namespace) {
			continue
		}

		entry := SATokenEntry{
			Name:      sa.Name,
			Namespace: sa.Namespace,
		}

		// Check automountServiceAccountToken
		entry.AutoMountToken = true // default is true
		if sa.AutomountServiceAccountToken != nil {
			entry.AutoMountToken = *sa.AutomountServiceAccountToken
		}
		if entry.AutoMountToken {
			autoMountOn++
		} else {
			autoMountOff++
		}

		// Check secret references
		if len(sa.Secrets) > 0 {
			entry.HasSecretRef = true
			entry.SecretName = sa.Secrets[0].Name
			withTokens++

			// Check token age
			key := fmt.Sprintf("%s/%s", sa.Namespace, entry.SecretName)
			if createTime, ok := secretAgeMap[key]; ok {
				age := time.Since(createTime)
				entry.SecretAge = age.Round(time.Hour * 24).String()

				// Long-lived tokens (>90 days)
				if age > 90*24*time.Hour {
					longLived++
					issues = append(issues, SATokenIssue{
						Severity: "warning",
						Type:     "long-lived-token",
						Resource: fmt.Sprintf("%s/%s", sa.Namespace, sa.Name),
						Message:  fmt.Sprintf("SA token is %s old — consider rotating or using projected token volumes", entry.SecretAge),
					})
				}
			}
		} else {
			noSecretRef++
		}

		// Check if default SA
		if sa.Name == "default" {
			entry.IsDefault = true
		}

		// Check pod usage
		key := fmt.Sprintf("%s/%s", sa.Namespace, sa.Name)
		entry.PodUsageCount = saUsageMap[key]

		// Default SA used by pods is a risk
		if entry.IsDefault && entry.PodUsageCount > 0 && entry.AutoMountToken {
			defaultUsed++
			issues = append(issues, SATokenIssue{
				Severity: "warning",
				Type:     "default-sa-used",
				Resource: fmt.Sprintf("%s/default", sa.Namespace),
				Message:  fmt.Sprintf("Default ServiceAccount is used by %d pod(s) with auto-mounted token — create dedicated SAs for workloads", entry.PodUsageCount),
			})
		}

		// Auto-mount enabled but no pods using it
		if entry.AutoMountToken && !entry.IsDefault && entry.PodUsageCount == 0 {
			issues = append(issues, SATokenIssue{
				Severity: "info",
				Type:     "unused-sa-with-automount",
				Resource: fmt.Sprintf("%s/%s", sa.Namespace, sa.Name),
				Message:  "SA has automount enabled but no pods are using it — consider disabling automount",
			})
		}

		entry.RiskLevel = assessSATokenRisk(entry)
		entries = append(entries, entry)

		// Namespace stats
		if _, ok := nsStats[sa.Namespace]; !ok {
			nsStats[sa.Namespace] = &SATokenNSStat{Namespace: sa.Namespace}
		}
		nsStats[sa.Namespace].SACount++
		if entry.AutoMountToken {
			nsStats[sa.Namespace].AutoMountOn++
		}
		if entry.RiskLevel == "warning" || entry.RiskLevel == "critical" {
			nsStats[sa.Namespace].RiskCount++
		}
	}

	// Convert namespace stats
	for _, ns := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].RiskCount > result.ByNamespace[j].RiskCount
	})

	sort.Slice(entries, func(i, j int) bool {
		riskOrder := map[string]int{"critical": 0, "warning": 1, "info": 2, "healthy": 3}
		return riskOrder[entries[i].RiskLevel] < riskOrder[entries[j].RiskLevel]
	})

	// Recommendations
	var recommendations []string
	if defaultUsed > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d namespace(s) use default ServiceAccount with auto-mounted tokens — create dedicated SAs for each workload", defaultUsed))
	}
	if longLived > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d SA token(s) are older than 90 days — rotate tokens or switch to projected token volumes with expiration", longLived))
	}
	if autoMountOn > 0 && autoMountOff == 0 {
		recommendations = append(recommendations, "All SAs have automount enabled — disable automount for SAs that don't need API access")
	}
	if noSecretRef > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d SA(s) have no secret reference — verify token provisioning is working", noSecretRef))
	}
	if len(recommendations) == 0 {
		recommendations = append(recommendations, "ServiceAccount token configuration is healthy — proper SA isolation and token management")
	}

	result.ServiceAccounts = entries
	result.Issues = issues
	result.Recommendations = recommendations
	result.Summary = SATokenSummary{
		TotalSAs:          len(entries),
		AutoMountEnabled:  autoMountOn,
		AutoMountDisabled: autoMountOff,
		WithTokens:        withTokens,
		LongLivedTokens:   longLived,
		NoSecretRef:       noSecretRef,
		DefaultSAUsed:     defaultUsed,
	}
	result.HealthScore = computeSATokenScore(result.Summary, len(issues))

	writeJSON(w, result)
}

// assessSATokenRisk determines risk level.
func assessSATokenRisk(entry SATokenEntry) string {
	risk := 0
	if entry.IsDefault && entry.PodUsageCount > 0 && entry.AutoMountToken {
		risk += 2
	}
	if entry.SecretAge != "" {
		// Parse approximate age
		if strings.Contains(entry.SecretAge, "d") {
			risk += 1 // has age, potentially long-lived
		}
	}
	if entry.AutoMountToken && entry.PodUsageCount == 0 && !entry.IsDefault {
		risk += 1
	}
	switch {
	case risk >= 3:
		return "critical"
	case risk >= 2:
		return "warning"
	case risk >= 1:
		return "info"
	default:
		return "healthy"
	}
}

// computeSATokenScore computes a 0-100 health score.
func computeSATokenScore(s SATokenSummary, issueCount int) int {
	if s.TotalSAs == 0 {
		return 100
	}
	score := 100
	score -= s.DefaultSAUsed * 5
	score -= s.LongLivedTokens * 3
	score -= issueCount * 1
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}
