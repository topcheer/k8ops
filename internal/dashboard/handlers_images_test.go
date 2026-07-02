package dashboard

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseImageRef_Simple(t *testing.T) {
	info := parseImageRef("nginx")
	if info.Registry != "docker.io" {
		t.Errorf("registry = %q, want docker.io", info.Registry)
	}
	if info.Repo != "library/nginx" {
		t.Errorf("repo = %q, want library/nginx", info.Repo)
	}
	if info.Tag != "latest" {
		t.Errorf("tag = %q, want latest", info.Tag)
	}
}

func TestParseImageRef_WithTag(t *testing.T) {
	info := parseImageRef("nginx:1.21")
	if info.Tag != "1.21" {
		t.Errorf("tag = %q, want 1.21", info.Tag)
	}
	if info.Repo != "library/nginx" {
		t.Errorf("repo = %q, want library/nginx", info.Repo)
	}
}

func TestParseImageRef_WithRegistry(t *testing.T) {
	info := parseImageRef("registry.iot2.win/k8ops:v14.40")
	if info.Registry != "registry.iot2.win" {
		t.Errorf("registry = %q, want registry.iot2.win", info.Registry)
	}
	if info.Repo != "k8ops" {
		t.Errorf("repo = %q, want k8ops", info.Repo)
	}
	if info.Tag != "v14.40" {
		t.Errorf("tag = %q, want v14.40", info.Tag)
	}
}

func TestParseImageRef_WithPort(t *testing.T) {
	info := parseImageRef("registry.io:5000/app:v2")
	if info.Registry != "registry.io:5000" {
		t.Errorf("registry = %q, want registry.io:5000", info.Registry)
	}
	if info.Repo != "app" {
		t.Errorf("repo = %q, want app", info.Repo)
	}
}

func TestParseImageRef_WithDigest(t *testing.T) {
	info := parseImageRef("nginx@sha256:abc123")
	if info.Repo != "library/nginx" {
		t.Errorf("repo = %q, want library/nginx", info.Repo)
	}
	if info.Tag != "@sha256:abc123" {
		t.Errorf("tag = %q, want @sha256:abc123", info.Tag)
	}
}

func TestParseImageRef_CustomRepo(t *testing.T) {
	info := parseImageRef("myrepo/myapp:v2")
	if info.Registry != "docker.io" {
		t.Errorf("registry = %q, want docker.io", info.Registry)
	}
	if info.Repo != "myrepo/myapp" {
		t.Errorf("repo = %q, want myrepo/myapp", info.Repo)
	}
}

func TestHandleImageInventory_NoClient(t *testing.T) {
	s := &Server{}
	req := httptest.NewRequest(http.MethodGet, "/api/images", nil)
	rr := httptest.NewRecorder()

	s.handleImageInventory(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}
}
