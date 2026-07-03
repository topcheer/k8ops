package dashboard

import (
	"encoding/json"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestAnalyzeSAHealth_Unused(t *testing.T) {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: "old-sa", Namespace: "production",
			CreationTimestamp: metav1.Time{Time: time.Now().AddDate(0, 0, -30)},
		},
	}

	h := analyzeSAHealth(sa, map[string][]string{}, map[string][]SABinding{}, map[string][]SABinding{}, map[string][]SABinding{})

	if h.PodCount != 0 {
		t.Errorf("expected 0 pods, got %d", h.PodCount)
	}
	found := false
	for _, issue := range h.Issues {
		if issue == "Unused ServiceAccount (30 days old, no pods reference it)" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected unused SA issue, got: %v", h.Issues)
	}
}

func TestAnalyzeSAHealth_Healthy(t *testing.T) {
	tokenFalse := false
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: "app-sa", Namespace: "default",
			CreationTimestamp: metav1.Time{Time: time.Now().AddDate(0, 0, -5)},
		},
	}
	sa.AutomountServiceAccountToken = &tokenFalse

	usage := map[string][]string{
		"default/app-sa": {"default/app-pod-1", "default/app-pod-2"},
	}
	bindings := map[string][]SABinding{
		"default/app-sa": {{Name: "app-rb", Kind: "RoleBinding", RoleName: "app-role", RoleKind: "Role"}},
	}

	h := analyzeSAHealth(sa, usage, bindings, map[string][]SABinding{}, map[string][]SABinding{})

	if h.PodCount != 2 {
		t.Errorf("expected 2 pods, got %d", h.PodCount)
	}
	if h.AutomountToken {
		t.Error("expected automountToken=false")
	}
	if len(h.RoleBindings) != 1 {
		t.Errorf("expected 1 role binding, got %d", len(h.RoleBindings))
	}
}

func TestAnalyzeSAHealth_ClusterAdmin(t *testing.T) {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: "powerful-sa", Namespace: "production",
			CreationTimestamp: metav1.Time{Time: time.Now().AddDate(0, 0, -100)},
		},
	}

	usage := map[string][]string{
		"production/powerful-sa": {"production/app-pod"},
	}
	crbBySA := map[string][]SABinding{
		"production/powerful-sa": {{Name: "admin-binding", Kind: "ClusterRoleBinding", RoleName: "cluster-admin", RoleKind: "ClusterRole", ClusterWide: true}},
	}
	clusterAdminBindings := map[string][]SABinding{
		"production/powerful-sa": {{Name: "admin-binding", Kind: "ClusterRoleBinding", RoleName: "cluster-admin"}},
	}

	h := analyzeSAHealth(sa, usage, map[string][]SABinding{}, crbBySA, clusterAdminBindings)

	if h.MaxSeverity != SASevCritical {
		t.Errorf("expected critical severity, got %s", h.MaxSeverity)
	}
	if h.RiskScore < 80 {
		t.Errorf("expected risk score >= 80, got %d", h.RiskScore)
	}
}

func TestAnalyzeSAHealth_DefaultSAUsedByPods(t *testing.T) {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: "default", Namespace: "my-app",
			CreationTimestamp: metav1.Time{Time: time.Now().AddDate(0, 0, -10)},
		},
	}

	usage := map[string][]string{
		"my-app/default": {"my-app/web-1"},
	}

	h := analyzeSAHealth(sa, usage, map[string][]SABinding{}, map[string][]SABinding{}, map[string][]SABinding{})

	if !h.IsDefault {
		t.Error("expected isDefault=true")
	}
	if h.PodCount != 1 {
		t.Errorf("expected 1 pod, got %d", h.PodCount)
	}
	found := false
	for _, issue := range h.Issues {
		if issue == "Default SA used by 1 pod(s) — should use dedicated SA with least privilege" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected default SA usage issue, got: %v", h.Issues)
	}
}

func TestAnalyzeSAHealth_TokenAutoMountNoBindings(t *testing.T) {
	tokenTrue := true
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: "web-sa", Namespace: "frontend",
			CreationTimestamp: metav1.Time{Time: time.Now().AddDate(0, 0, -5)},
		},
	}
	sa.AutomountServiceAccountToken = &tokenTrue

	usage := map[string][]string{
		"frontend/web-sa": {"frontend/web-1"},
	}

	h := analyzeSAHealth(sa, usage, map[string][]SABinding{}, map[string][]SABinding{}, map[string][]SABinding{})

	found := false
	for _, issue := range h.Issues {
		if issue == "automountServiceAccountToken=true but SA has no RoleBindings (unnecessary token exposure)" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected token exposure issue, got: %v", h.Issues)
	}
}

func TestAnalyzeSAHealth_StaleAccess(t *testing.T) {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: "stale-sa", Namespace: "legacy",
			CreationTimestamp: metav1.Time{Time: time.Now().AddDate(0, 0, -90)},
		},
	}

	bindings := map[string][]SABinding{
		"legacy/stale-sa": {{Name: "legacy-rb", Kind: "RoleBinding", RoleName: "editor", RoleKind: "ClusterRole"}},
	}

	h := analyzeSAHealth(sa, map[string][]string{}, bindings, map[string][]SABinding{}, map[string][]SABinding{})

	found := false
	for _, issue := range h.Issues {
		if issue == "SA has active permissions but no pod usage in 90 days — revoke stale access" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected stale access issue, got: %v", h.Issues)
	}
	if h.MaxSeverity != SASevHigh {
		t.Errorf("expected high severity, got %s", h.MaxSeverity)
	}
}

func TestAnalyzeSAHealth_LegacySecrets(t *testing.T) {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: "legacy-token-sa", Namespace: "old-app",
			CreationTimestamp: metav1.Time{Time: time.Now().AddDate(0, 0, -200)},
		},
		Secrets: []corev1.ObjectReference{
			{Name: "legacy-token-sa-token-abc"},
			{Name: "legacy-token-sa-dockercfg-xyz"},
		},
	}

	h := analyzeSAHealth(sa, map[string][]string{}, map[string][]SABinding{}, map[string][]SABinding{}, map[string][]SABinding{})

	if !h.HasSecrets {
		t.Error("expected hasSecrets=true")
	}
	if h.SecretCount != 2 {
		t.Errorf("expected 2 secrets, got %d", h.SecretCount)
	}
}

func TestAnalyzeSAHealth_SystemNamespace(t *testing.T) {
	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name: "system-sa", Namespace: "kube-system",
			CreationTimestamp: metav1.Time{Time: time.Now().AddDate(0, 0, -300)},
		},
	}

	h := analyzeSAHealth(sa, map[string][]string{}, map[string][]SABinding{}, map[string][]SABinding{}, map[string][]SABinding{})

	if !h.IsSystem {
		t.Error("expected isSystem=true for kube-system SA")
	}
}

func TestSASeverityRank(t *testing.T) {
	if saSeverityRank(SASevCritical) >= saSeverityRank(SASevHigh) {
		t.Error("critical should rank before high")
	}
	if saSeverityRank(SASevHigh) >= saSeverityRank(SASevMedium) {
		t.Error("high should rank before medium")
	}
	if saSeverityRank(SASevMedium) >= saSeverityRank(SASevLow) {
		t.Error("medium should rank before low")
	}
	if saSeverityRank(SASevLow) >= saSeverityRank(SASevInfo) {
		t.Error("low should rank before info")
	}
}

func TestSAAuditResult_JSON(t *testing.T) {
	result := SAAuditResult{
		Summary: SAAuditSummary{
			TotalSAs:       10,
			UnusedSAs:      3,
			DefaultSAUsed:  2,
			TokenAutoMount: 8,
			HighRiskSAs:    1,
			BySeverity:     map[string]int{"critical": 1, "medium": 3, "low": 2, "info": 4},
		},
		ServiceAccounts: []SAHealth{
			{Name: "app-sa", Namespace: "default", RiskScore: 80, MaxSeverity: SASevCritical},
		},
		Issues: []SAIssue{
			{Severity: SASevLow, Category: "Default SA Usage", Detail: "Pod uses default SA"},
		},
	}

	data, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("JSON marshal failed: %v", err)
	}

	var decoded SAAuditResult
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("JSON unmarshal failed: %v", err)
	}

	if decoded.Summary.TotalSAs != 10 {
		t.Errorf("expected 10 SAs, got %d", decoded.Summary.TotalSAs)
	}
	if len(decoded.ServiceAccounts) != 1 {
		t.Errorf("expected 1 SA, got %d", len(decoded.ServiceAccounts))
	}
	if len(decoded.Issues) != 1 {
		t.Errorf("expected 1 issue, got %d", len(decoded.Issues))
	}
}

func TestSAHealth_RiskScoreOrdering(t *testing.T) {
	// Ensure risk scores make sense: cluster-admin > stale-access > default-sa > unused
	adminSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "admin-sa", Namespace: "prod", CreationTimestamp: metav1.Time{Time: time.Now().AddDate(0, 0, -50)}},
	}
	crbBySA := map[string][]SABinding{
		"prod/admin-sa": {{Name: "admin-crb", Kind: "ClusterRoleBinding", RoleName: "cluster-admin", ClusterWide: true}},
	}
	adminH := analyzeSAHealth(adminSA, map[string][]string{}, map[string][]SABinding{}, crbBySA, crbBySA)

	unusedSA := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{Name: "unused-sa", Namespace: "prod", CreationTimestamp: metav1.Time{Time: time.Now().AddDate(0, 0, -15)}},
	}
	unusedH := analyzeSAHealth(unusedSA, map[string][]string{}, map[string][]SABinding{}, map[string][]SABinding{}, map[string][]SABinding{})

	if adminH.RiskScore <= unusedH.RiskScore {
		t.Errorf("cluster-admin SA should have higher risk (%d) than unused SA (%d)", adminH.RiskScore, unusedH.RiskScore)
	}
}

// Ensure rbacv1 import is used
var _ rbacv1.RoleRef
