package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleOpenAPISpec(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/openapi.json", nil)
	rr := httptest.NewRecorder()

	s.handleOpenAPISpec(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var spec OpenAPISpec
	if err := json.NewDecoder(rr.Body).Decode(&spec); err != nil {
		t.Fatalf("failed to decode spec: %v", err)
	}

	if spec.OpenAPI != "3.0.3" {
		t.Errorf("expected openapi 3.0.3, got %s", spec.OpenAPI)
	}
	if spec.Info.Title != "k8ops API" {
		t.Errorf("unexpected title: %s", spec.Info.Title)
	}
	if len(spec.Paths) == 0 {
		t.Error("expected non-empty paths")
	}

	// Verify key endpoints exist
	expectedPaths := []string{
		"/api/health", "/api/version", "/api/cluster/overview",
		"/api/nodes", "/api/pods", "/api/resources",
		"/api/security/audit", "/api/yaml/apply", "/api/scale",
	}
	for _, p := range expectedPaths {
		if _, ok := spec.Paths[p]; !ok {
			t.Errorf("expected path %q in spec", p)
		}
	}
}

func TestHandleAPIDocs(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/docs", nil)
	rr := httptest.NewRecorder()

	s.handleAPIDocs(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var result map[string]interface{}
	if err := json.NewDecoder(rr.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result["endpointCount"] == nil {
		t.Error("expected endpointCount field")
	}
	if result["tagGroups"] == nil {
		t.Error("expected tagGroups field")
	}
	if result["tagOrder"] == nil {
		t.Error("expected tagOrder field")
	}
}

func TestOpenAPISpecWriteOpsHaveBody(t *testing.T) {
	spec := buildOpenAPISpec()

	writePaths := []string{"/api/scale", "/api/yaml/apply", "/api/pod/delete", "/api/rollout/restart"}
	for _, path := range writePaths {
		ops, ok := spec.Paths[path]
		if !ok {
			t.Errorf("expected path %q", path)
			continue
		}
		postOp, ok := ops["post"]
		if !ok {
			t.Errorf("expected POST method for %s", path)
			continue
		}
		if postOp.RequestBody == nil {
			t.Errorf("expected request body for POST %s", path)
		}
	}
}

func TestOpenAPISpecAllMethods(t *testing.T) {
	spec := buildOpenAPISpec()

	// Ensure every operation has a summary and operationId
	for path, methods := range spec.Paths {
		for method, op := range methods {
			if op.Summary == "" {
				t.Errorf("path %s %s: missing summary", method, path)
			}
			if op.OperationID == "" {
				t.Errorf("path %s %s: missing operationId", method, path)
			}
			if len(op.Tags) == 0 {
				t.Errorf("path %s %s: missing tags", method, path)
			}
			if len(op.Responses) == 0 {
				t.Errorf("path %s %s: missing responses", method, path)
			}
		}
	}
}
