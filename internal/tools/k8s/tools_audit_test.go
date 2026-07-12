package k8s

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAuditToolName(t *testing.T) {
	tool := &AuditTool{}
	if tool.Name() != "k8s_run_audit" {
		t.Errorf("expected k8s_run_audit, got %s", tool.Name())
	}
}

func TestAuditToolParameters(t *testing.T) {
	tool := &AuditTool{}
	params := tool.Parameters()
	if params == nil {
		t.Fatal("expected non-nil parameters")
	}
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatal("expected properties map")
	}
	if _, ok := props["audit_name"]; !ok {
		t.Error("expected audit_name property")
	}
}

func TestAuditToolExecuteUnknownAudit(t *testing.T) {
	tool := &AuditTool{}
	result, err := tool.Execute(context.Background(), map[string]any{
		"audit_name": "nonexistent-audit",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure for unknown audit")
	}
}

func TestAuditToolExecuteMissingParam(t *testing.T) {
	tool := &AuditTool{}
	result, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure for missing audit_name")
	}
}

func TestAuditToolExecuteSuccess(t *testing.T) {
	// Create a test server that returns audit data
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/operations/etcd-health" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{"summary":{"healthScore":85,"totalMembers":3},"status":"healthy"}`))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	// Extract host:port from test server URL
	addr := ts.URL[len("http://"):]

	tool := &AuditTool{DashboardAddr: addr}
	result, err := tool.Execute(context.Background(), map[string]any{
		"audit_name": "operations:etcd-health",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}

	// Verify output contains the audit data
	var data map[string]any
	lines := []byte(result.Output)
	// Find the JSON part (after the header line)
	jsonStart := 0
	for i := 0; i < len(lines); i++ {
		if lines[i] == '\n' {
			jsonStart = i + 1
			break
		}
	}
	// Skip second header line
	for i := jsonStart; i < len(lines); i++ {
		if lines[i] == '\n' {
			jsonStart = i + 1
			break
		}
	}
	if err := json.Unmarshal(lines[jsonStart:], &data); err != nil {
		t.Fatalf("failed to parse output JSON: %v", err)
	}

	summary, ok := data["summary"].(map[string]any)
	if !ok {
		t.Fatal("expected summary in output")
	}
	if score, ok := summary["healthScore"].(float64); !ok || int(score) != 85 {
		t.Errorf("expected healthScore=85, got %v", summary["healthScore"])
	}
}

func TestAuditToolExecuteHTTPError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("internal error"))
	}))
	defer ts.Close()

	addr := ts.URL[len("http://"):]
	tool := &AuditTool{DashboardAddr: addr}
	result, err := tool.Execute(context.Background(), map[string]any{
		"audit_name": "operations:etcd-health",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Success {
		t.Error("expected failure on HTTP 500")
	}
}

func TestListAuditsTool(t *testing.T) {
	tool := &ListAuditsTool{}
	if tool.Name() != "k8s_list_audits" {
		t.Errorf("expected k8s_list_audits, got %s", tool.Name())
	}

	result, err := tool.Execute(context.Background(), map[string]any{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Success {
		t.Fatalf("expected success, got error: %s", result.Error)
	}

	var data map[string]any
	if err := json.Unmarshal([]byte(result.Output), &data); err != nil {
		t.Fatalf("failed to parse output: %v", err)
	}

	count, ok := data["count"].(float64)
	if !ok {
		t.Fatal("expected count in output")
	}
	if int(count) == 0 {
		t.Error("expected non-zero audit count")
	}
	if int(count) < 100 {
		t.Errorf("expected at least 100 audits, got %d", int(count))
	}

	audits, ok := data["audits"].([]any)
	if !ok {
		t.Fatal("expected audits array")
	}
	if len(audits) < 100 {
		t.Errorf("expected at least 100 audit entries, got %d", len(audits))
	}
}

func TestAuditRegistryCoverage(t *testing.T) {
	// Verify we have audits from all 6 dimensions
	categories := make(map[string]int)
	for _, a := range auditRegistry {
		cat := "other"
		if idx := indexOf(a.name, ':'); idx > 0 {
			cat = a.name[:idx]
		}
		categories[cat]++
	}

	expected := []string{"product", "deployment", "operations", "security", "scalability", "cluster", "infra"}
	for _, cat := range expected {
		if categories[cat] == 0 {
			t.Errorf("expected audits for category %s, got 0", cat)
		}
	}
}

func indexOf(s string, c byte) int {
	for i := 0; i < len(s); i++ {
		if s[i] == c {
			return i
		}
	}
	return -1
}
