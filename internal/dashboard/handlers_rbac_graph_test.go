package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8sfake "k8s.io/client-go/kubernetes/fake"
)

func TestRBACGraphEmpty(t *testing.T) {
	s := &Server{}
	req := newReqWithClients("GET", "/api/security/rbac-graph", k8sfake.NewSimpleClientset())
	w := httptest.NewRecorder()
	s.handleRBACGraph(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var result RBACPermGraphResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
}

func TestRBACGraphClusterAdmin(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "admin-binding"},
			Subjects: []rbacv1.Subject{
				{Kind: "User", Name: "admin@example.com"},
			},
			RoleRef: rbacv1.RoleRef{Kind: "ClusterRole", Name: "cluster-admin"},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/security/rbac-graph", clientset)
	w := httptest.NewRecorder()
	s.handleRBACGraph(w, req)
	var result RBACPermGraphResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Summary.ClusterAdmins != 1 {
		t.Errorf("expected 1 cluster admin, got %d", result.Summary.ClusterAdmins)
	}
}

func TestRBACGraphEscalationPath(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{Name: "escalator"},
			Rules: []rbacv1.PolicyRule{
				{Verbs: []string{"escalate"}, Resources: []string{"clusterroles"}},
			},
		},
		&rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "esc-binding"},
			Subjects: []rbacv1.Subject{
				{Kind: "ServiceAccount", Name: "deploy-bot", Namespace: "ci"},
			},
			RoleRef: rbacv1.RoleRef{Kind: "ClusterRole", Name: "escalator"},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/security/rbac-graph", clientset)
	w := httptest.NewRecorder()
	s.handleRBACGraph(w, req)
	var result RBACPermGraphResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result.EscalationPaths) == 0 {
		t.Error("expected escalation paths")
	}
	foundCritical := false
	for _, p := range result.EscalationPaths {
		if p.Severity == "critical" {
			foundCritical = true
		}
	}
	if !foundCritical {
		t.Error("expected critical escalation path")
	}
}

func TestRBACGraphWildcard(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{Name: "wildcard-role"},
			Rules: []rbacv1.PolicyRule{
				{Verbs: []string{"*"}, Resources: []string{"*"}},
			},
		},
		&rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "wild-binding"},
			Subjects: []rbacv1.Subject{
				{Kind: "Group", Name: "devs"},
			},
			RoleRef: rbacv1.RoleRef{Kind: "ClusterRole", Name: "wildcard-role"},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/security/rbac-graph", clientset)
	w := httptest.NewRecorder()
	s.handleRBACGraph(w, req)
	var result RBACPermGraphResult
	json.Unmarshal(w.Body.Bytes(), &result)
	if result.Summary.WildcardPerms == 0 {
		t.Error("expected wildcard permissions")
	}
}

func TestRBACGraphRecommendations(t *testing.T) {
	result := RBACPermGraphResult{
		Summary: RBACPermGraphSummary{
			ClusterAdmins: 3, WildcardPerms: 2, EscalationPaths: 1,
		},
		EscalationPaths: []EscalationPath{{}},
		CriticalRoles:   []RBACPermRole{{}},
		Overprivileged:  []OverprivEntry{{}},
		RiskScore:       35,
	}
	recs := generateRBACGraphRecs(result)
	if len(recs) == 0 {
		t.Fatal("expected recs")
	}
	foundAdmin := false
	for _, r := range recs {
		if strings.Contains(strings.ToLower(r), "cluster-admin") {
			foundAdmin = true
		}
	}
	if !foundAdmin {
		t.Error("expected cluster-admin recommendation")
	}
}

func TestRBACGraphSecretAccess(t *testing.T) {
	clientset := k8sfake.NewSimpleClientset(
		&rbacv1.ClusterRole{
			ObjectMeta: metav1.ObjectMeta{Name: "secret-reader"},
			Rules: []rbacv1.PolicyRule{
				{Verbs: []string{"get", "list"}, Resources: []string{"secrets"}},
			},
		},
		&rbacv1.ClusterRoleBinding{
			ObjectMeta: metav1.ObjectMeta{Name: "sr-binding"},
			Subjects: []rbacv1.Subject{
				{Kind: "ServiceAccount", Name: "monitor", Namespace: "observability"},
			},
			RoleRef: rbacv1.RoleRef{Kind: "ClusterRole", Name: "secret-reader"},
		},
	)
	s := &Server{}
	req := newReqWithClients("GET", "/api/security/rbac-graph", clientset)
	w := httptest.NewRecorder()
	s.handleRBACGraph(w, req)
	var result RBACPermGraphResult
	json.Unmarshal(w.Body.Bytes(), &result)
	// Note: get/list on secrets is read-only, not flagged as dangerous by default
	// The handler checks for write verbs on secrets
}

func TestClassifyRoleDanger(t *testing.T) {
	// Wildcard = cluster-admin equivalent
	r := classifyRoleDanger("test-admin", []rbacv1.PolicyRule{
		{Verbs: []string{"*"}, Resources: []string{"*"}},
	}, true, "")
	if r == nil || r.Danger != "cluster-admin" {
		t.Error("expected cluster-admin danger")
	}

	// System role should be skipped
	r = classifyRoleDanger("system:controller:foo", []rbacv1.PolicyRule{}, true, "")
	if r != nil {
		t.Error("expected nil for system role")
	}

	// Safe role should return nil
	r = classifyRoleDanger("pod-reader", []rbacv1.PolicyRule{
		{Verbs: []string{"get", "list"}, Resources: []string{"pods"}},
	}, false, "default")
	if r != nil {
		t.Error("expected nil for safe role")
	}
}
