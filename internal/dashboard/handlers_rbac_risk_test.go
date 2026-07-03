package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func rbacTestReq(objects ...runtime.Object) (*Server, *http.Request) {
	clientset := k8sfake.NewSimpleClientset(objects...)
	req := newReqWithClients(http.MethodGet, "/api/security/rbac-risk", clientset)
	return &Server{}, req
}

// --- Cluster admin tests ---

func TestRBAC_ClusterAdminCritical(t *testing.T) {
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "admin-binding"},
		Subjects:   []rbacv1.Subject{{Kind: "User", Name: "admin-user"}},
		RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "cluster-admin"},
	}
	srv, req := rbacTestReq(crb)
	rr := httptest.NewRecorder()
	srv.handleRBACRiskScan(rr, req)

	var result RBACRiskResult
	if err := json.Unmarshal(rr.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(result.Subjects) != 1 {
		t.Fatalf("expected 1 subject, got %d", len(result.Subjects))
	}
	s := result.Subjects[0]
	if s.RiskLevel != RBACRiskCritical {
		t.Errorf("expected critical, got %s", s.RiskLevel)
	}
	if s.RiskScore != 100 {
		t.Errorf("expected score 100, got %d", s.RiskScore)
	}
}

func TestRBAC_ClusterAdminSA(t *testing.T) {
	sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "powerful-sa", Namespace: "app"}}
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "sa-admin"},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: "powerful-sa", Namespace: "app"}},
		RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "cluster-admin"},
	}
	srv, req := rbacTestReq(sa, crb)
	rr := httptest.NewRecorder()
	srv.handleRBACRiskScan(rr, req)

	var result RBACRiskResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	s := result.Subjects[0]
	if s.RiskLevel != RBACRiskCritical {
		t.Errorf("expected critical for SA with cluster-admin, got %s", s.RiskLevel)
	}
	if s.DisplayName != "app/powerful-sa" {
		t.Errorf("expected display name app/powerful-sa, got %s", s.DisplayName)
	}
}

// --- Privilege escalation tests ---

func TestRBAC_PrivilegeEscalation(t *testing.T) {
	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "rbac-manager"},
		Rules: []rbacv1.PolicyRule{
			{
				Verbs:     []string{"create", "update"},
				Resources: []string{"rolebindings", "clusterrolebindings"},
				APIGroups: []string{"rbac.authorization.k8s.io"},
			},
		},
	}
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "rbac-mgr-binding"},
		Subjects:   []rbacv1.Subject{{Kind: "User", Name: "rbac-user"}},
		RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "rbac-manager"},
	}
	srv, req := rbacTestReq(cr, crb)
	rr := httptest.NewRecorder()
	srv.handleRBACRiskScan(rr, req)

	var result RBACRiskResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	s := result.Subjects[0]
	if !s.PrivilegeEscalation {
		t.Error("expected privilege escalation flag")
	}
	if s.RiskLevel != RBACRiskHigh {
		t.Errorf("expected high risk for privilege escalation, got %s", s.RiskLevel)
	}
}

// --- Wildcard access tests ---

func TestRBAC_WildcardVerbs(t *testing.T) {
	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "wildcard-role"},
		Rules: []rbacv1.PolicyRule{
			{Verbs: []string{"*"}, Resources: []string{"pods"}, APIGroups: []string{""}},
		},
	}
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "wildcard-binding"},
		Subjects:   []rbacv1.Subject{{Kind: "User", Name: "wildcard-user"}},
		RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "wildcard-role"},
	}
	srv, req := rbacTestReq(cr, crb)
	rr := httptest.NewRecorder()
	srv.handleRBACRiskScan(rr, req)

	var result RBACRiskResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	s := result.Subjects[0]
	if !s.WildcardAccess {
		t.Error("expected wildcard access flag")
	}
}

func TestRBAC_WildcardResources(t *testing.T) {
	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "all-resources"},
		Rules: []rbacv1.PolicyRule{
			{Verbs: []string{"get", "list"}, Resources: []string{"*"}, APIGroups: []string{"*"}},
		},
	}
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "reader-binding"},
		Subjects:   []rbacv1.Subject{{Kind: "User", Name: "reader"}},
		RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "all-resources"},
	}
	srv, req := rbacTestReq(cr, crb)
	rr := httptest.NewRecorder()
	srv.handleRBACRiskScan(rr, req)

	var result RBACRiskResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	s := result.Subjects[0]
	if !s.WildcardAccess {
		t.Error("expected wildcard access for * resources")
	}
}

// --- Read-only low risk ---

func TestRBAC_ReadOnlyLowRisk(t *testing.T) {
	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "viewer"},
		Rules: []rbacv1.PolicyRule{
			{Verbs: []string{"get", "list", "watch"}, Resources: []string{"pods", "services"}, APIGroups: []string{""}},
		},
	}
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "viewer-binding"},
		Subjects:   []rbacv1.Subject{{Kind: "User", Name: "viewer-user"}},
		RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "viewer"},
	}
	srv, req := rbacTestReq(cr, crb)
	rr := httptest.NewRecorder()
	srv.handleRBACRiskScan(rr, req)

	var result RBACRiskResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	s := result.Subjects[0]
	if s.RiskLevel != RBACRiskLow {
		t.Errorf("expected low risk for read-only, got %s (score=%d)", s.RiskLevel, s.RiskScore)
	}
}

// --- Sensitive resource access ---

func TestRBAC_SensitiveResources(t *testing.T) {
	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "secret-reader"},
		Rules: []rbacv1.PolicyRule{
			{Verbs: []string{"get"}, Resources: []string{"secrets"}, APIGroups: []string{""}},
		},
	}
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "secret-reader-binding"},
		Subjects:   []rbacv1.Subject{{Kind: "User", Name: "secret-user"}},
		RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "secret-reader"},
	}
	srv, req := rbacTestReq(cr, crb)
	rr := httptest.NewRecorder()
	srv.handleRBACRiskScan(rr, req)

	var result RBACRiskResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	s := result.Subjects[0]
	found := false
	for _, issue := range s.Issues {
		if containsLower(issue, "sensitive") {
			found = true
		}
	}
	if !found {
		t.Error("expected issue about sensitive resources")
	}
}

// --- Namespace-scoped RoleBinding ---

func TestRBAC_NamespaceScopedRole(t *testing.T) {
	role := &rbacv1.Role{
		ObjectMeta: metav1.ObjectMeta{Name: "app-admin", Namespace: "app"},
		Rules: []rbacv1.PolicyRule{
			{Verbs: []string{"*"}, Resources: []string{"*"}, APIGroups: []string{""}},
		},
	}
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "app-admin-binding", Namespace: "app"},
		Subjects:   []rbacv1.Subject{{Kind: "User", Name: "dev-user"}},
		RoleRef:    rbacv1.RoleRef{Kind: "Role", Name: "app-admin"},
	}
	srv, req := rbacTestReq(role, rb)
	rr := httptest.NewRecorder()
	srv.handleRBACRiskScan(rr, req)

	var result RBACRiskResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	s := result.Subjects[0]
	if s.IsClusterScoped {
		t.Error("namespace-scoped binding should not be cluster-scoped")
	}
	if s.WildcardAccess != true {
		t.Error("expected wildcard access for * verbs/resources")
	}
}

// --- Unused binding detection ---

func TestRBAC_UnusedBindingNonExistentSA(t *testing.T) {
	// RoleBinding references a SA that doesn't exist
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "stale-binding", Namespace: "app"},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: "deleted-sa", Namespace: "app"}},
		RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "view"},
	}
	srv, req := rbacTestReq(rb)
	rr := httptest.NewRecorder()
	srv.handleRBACRiskScan(rr, req)

	var result RBACRiskResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	if result.Summary.UnusedBindings < 1 {
		t.Error("expected at least 1 unused binding for non-existent SA")
	}
}

// --- Default SA with write permissions ---

func TestRBAC_DefaultSAWithWrite(t *testing.T) {
	sa := &corev1.ServiceAccount{ObjectMeta: metav1.ObjectMeta{Name: "default", Namespace: "app"}}
	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "default-write", Namespace: "app"},
		Subjects:   []rbacv1.Subject{{Kind: "ServiceAccount", Name: "default", Namespace: "app"}},
		RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "edit"},
	}
	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "edit"},
		Rules: []rbacv1.PolicyRule{
			{Verbs: []string{"*"}, Resources: []string{"pods"}, APIGroups: []string{""}},
		},
	}
	srv, req := rbacTestReq(sa, rb, cr)
	rr := httptest.NewRecorder()
	srv.handleRBACRiskScan(rr, req)

	var result RBACRiskResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	s := result.Subjects[0]
	found := false
	for _, issue := range s.Issues {
		if containsLower(issue, "default service account") {
			found = true
		}
	}
	if !found {
		t.Error("expected issue about default SA with write permissions")
	}
}

// --- Summary tests ---

func TestRBAC_SummaryCounts(t *testing.T) {
	cr1 := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: "admin"}, Rules: nil}
	crb1 := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "b1"},
		Subjects:   []rbacv1.Subject{{Kind: "User", Name: "admin-user"}},
		RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "cluster-admin"},
	}
	cr2 := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "viewer"},
		Rules:      []rbacv1.PolicyRule{{Verbs: []string{"get"}, Resources: []string{"pods"}, APIGroups: []string{""}}},
	}
	crb2 := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "b2"},
		Subjects:   []rbacv1.Subject{{Kind: "User", Name: "viewer-user"}},
		RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "viewer"},
	}
	srv, req := rbacTestReq(cr1, crb1, cr2, crb2)
	rr := httptest.NewRecorder()
	srv.handleRBACRiskScan(rr, req)

	var result RBACRiskResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	if result.Summary.TotalSubjects != 2 {
		t.Errorf("expected 2 subjects, got %d", result.Summary.TotalSubjects)
	}
	if result.Summary.ByRiskLevel[string(RBACRiskCritical)] != 1 {
		t.Errorf("expected 1 critical, got %d", result.Summary.ByRiskLevel[string(RBACRiskCritical)])
	}
	if result.Summary.TotalBindings != 2 {
		t.Errorf("expected 2 total bindings, got %d", result.Summary.TotalBindings)
	}
}

func TestRBAC_SortingByRiskScore(t *testing.T) {
	crb1 := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "admin-b"},
		Subjects:   []rbacv1.Subject{{Kind: "User", Name: "zzz-admin"}},
		RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "cluster-admin"},
	}
	cr := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "viewer"},
		Rules:      []rbacv1.PolicyRule{{Verbs: []string{"get"}, Resources: []string{"pods"}, APIGroups: []string{""}}},
	}
	crb2 := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "viewer-b"},
		Subjects:   []rbacv1.Subject{{Kind: "User", Name: "aaa-viewer"}},
		RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "viewer"},
	}
	srv, req := rbacTestReq(crb1, cr, crb2)
	rr := httptest.NewRecorder()
	srv.handleRBACRiskScan(rr, req)

	var result RBACRiskResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	if len(result.Subjects) < 2 {
		t.Fatalf("expected at least 2 subjects, got %d", len(result.Subjects))
	}
	// Higher risk should be first
	if result.Subjects[0].RiskScore < result.Subjects[1].RiskScore {
		t.Error("expected subjects sorted by risk score descending")
	}
}

// --- Group subject ---

func TestRBAC_GroupSubject(t *testing.T) {
	crb := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "group-binding"},
		Subjects:   []rbacv1.Subject{{Kind: "Group", Name: "dev-team"}},
		RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "cluster-admin"},
	}
	srv, req := rbacTestReq(crb)
	rr := httptest.NewRecorder()
	srv.handleRBACRiskScan(rr, req)

	var result RBACRiskResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	s := result.Subjects[0]
	if s.Kind != SubjectKindGroup {
		t.Errorf("expected Group kind, got %s", s.Kind)
	}
	if s.RiskLevel != RBACRiskCritical {
		t.Errorf("expected critical, got %s", s.RiskLevel)
	}
}

// --- Empty cluster ---

func TestRBAC_EmptyCluster(t *testing.T) {
	srv, req := rbacTestReq()
	rr := httptest.NewRecorder()
	srv.handleRBACRiskScan(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", rr.Code)
	}
	var result RBACRiskResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	if result.Summary.TotalSubjects != 0 {
		t.Errorf("expected 0 subjects, got %d", result.Summary.TotalSubjects)
	}
}

// --- Multiple bindings to same subject ---

func TestRBAC_MultipleBindingsSameSubject(t *testing.T) {
	cr1 := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "writer"},
		Rules:      []rbacv1.PolicyRule{{Verbs: []string{"create"}, Resources: []string{"pods"}, APIGroups: []string{""}}},
	}
	cr2 := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{Name: "secret-access"},
		Rules:      []rbacv1.PolicyRule{{Verbs: []string{"get"}, Resources: []string{"secrets"}, APIGroups: []string{""}}},
	}
	crb1 := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "writer-b"},
		Subjects:   []rbacv1.Subject{{Kind: "User", Name: "multi-user"}},
		RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "writer"},
	}
	crb2 := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{Name: "secret-b"},
		Subjects:   []rbacv1.Subject{{Kind: "User", Name: "multi-user"}},
		RoleRef:    rbacv1.RoleRef{Kind: "ClusterRole", Name: "secret-access"},
	}
	srv, req := rbacTestReq(cr1, cr2, crb1, crb2)
	rr := httptest.NewRecorder()
	srv.handleRBACRiskScan(rr, req)

	var result RBACRiskResult
	json.Unmarshal(rr.Body.Bytes(), &result)
	if len(result.Subjects) != 1 {
		t.Fatalf("expected 1 subject with merged bindings, got %d", len(result.Subjects))
	}
	s := result.Subjects[0]
	if len(s.Bindings) != 2 {
		t.Errorf("expected 2 bindings, got %d", len(s.Bindings))
	}
}

// --- Helper ---

func containsLower(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
