package auth

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestVerifyState(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		actual   string
		want     bool
	}{
		{"matching", "abc123", "abc123", true},
		{"mismatch", "abc123", "xyz789", false},
		{"empty expected", "", "abc123", false},
		{"empty actual", "abc123", "", false},
		{"both empty", "", "", false},
		{"long match", "a-very-long-state-value-for-testing-1234567890", "a-very-long-state-value-for-testing-1234567890", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := VerifyState(tt.expected, tt.actual); got != tt.want {
				t.Errorf("VerifyState() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSetAndClearStateCookie(t *testing.T) {
	providerName := "google"
	state := "test-state-value-123"

	// Test SetStateCookie
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/auth/oidc/google/login", nil)

	SetStateCookie(rec, req, providerName, state)

	resp := rec.Result()
	t.Cleanup(func() { resp.Body.Close() })

	var cookie *http.Cookie
	for _, c := range resp.Cookies() {
		if c.Name == stateCookieName(providerName) {
			cookie = c
			break
		}
	}
	if cookie == nil {
		t.Fatal("state cookie not set")
	}
	if cookie.Value != state {
		t.Errorf("cookie value = %q, want %q", cookie.Value, state)
	}
	if !cookie.HttpOnly {
		t.Error("cookie should be HttpOnly")
	}
	if cookie.SameSite != http.SameSiteLaxMode {
		t.Error("cookie should have SameSite=Lax")
	}
	if cookie.MaxAge != 300 {
		t.Errorf("cookie MaxAge = %d, want 300", cookie.MaxAge)
	}

	// Test ClearStateCookie
	rec2 := httptest.NewRecorder()
	ClearStateCookie(rec2, providerName)
	resp2 := rec2.Result()
	t.Cleanup(func() { resp2.Body.Close() })

	var cleared *http.Cookie
	for _, c := range resp2.Cookies() {
		if c.Name == stateCookieName(providerName) {
			cleared = c
			break
		}
	}
	if cleared == nil {
		t.Fatal("clear cookie not found in response")
	}
	if cleared.MaxAge != -1 {
		t.Errorf("cleared cookie MaxAge = %d, want -1", cleared.MaxAge)
	}
}

func TestIsHTTPS(t *testing.T) {
	tests := []struct {
		name string
		req  *http.Request
		want bool
	}{
		{
			name: "plain HTTP",
			req:  httptest.NewRequest(http.MethodGet, "http://example.com/cb", nil),
			want: false,
		},
		{
			name: "X-Forwarded-Proto https",
			req: func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "http://internal.example.com/cb", nil)
				r.Header.Set("X-Forwarded-Proto", "https")
				return r
			}(),
			want: true,
		},
		{
			name: "X-Forwarded-Proto http",
			req: func() *http.Request {
				r := httptest.NewRequest(http.MethodGet, "http://internal.example.com/cb", nil)
				r.Header.Set("X-Forwarded-Proto", "http")
				return r
			}(),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isHTTPS(tt.req); got != tt.want {
				t.Errorf("isHTTPS() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSetStateCookieSecureFlagWithTLS(t *testing.T) {
	rec := httptest.NewRecorder()
	// httptest.NewRequest always has TLS=nil; simulate a TLS request manually
	req := &http.Request{
		Method: http.MethodGet,
		URL:    mustParse(t, "/api/auth/oidc/keycloak/login"),
		Header: make(http.Header),
		TLS:    &tls.ConnectionState{}, // non-nil TLS state indicates HTTPS
	}

	SetStateCookie(rec, req, "keycloak", "state-abc")
	resp := rec.Result()
	t.Cleanup(func() { resp.Body.Close() })

	for _, c := range resp.Cookies() {
		if c.Name == stateCookieName("keycloak") {
			if !c.Secure {
				t.Error("cookie should have Secure=true when request is over TLS")
			}
			return
		}
	}
	t.Fatal("cookie not found")
}

func mustParse(t *testing.T, raw string) *url.URL {
	t.Helper()
	u, err := url.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	return u
}

// ---------------------------------------------------------------------------
// Table-driven VerifyState tests
// ---------------------------------------------------------------------------

func TestVerifyState_TableDriven(t *testing.T) {
	tests := []struct {
		name     string
		expected string
		actual   string
		want     bool
	}{
		{"matching", "abc123", "abc123", true},
		{"empty expected", "", "abc123", false},
		{"empty actual", "abc123", "", false},
		{"both empty", "", "", false},
		{"mismatch", "abc123", "xyz789", false},
		{"partial match", "abc123", "abc12", false},
		{"different case", "ABC123", "abc123", false},
		{"whitespace difference", "abc123", "abc123 ", false},
		{"long match", "a-very-long-state-string-12345", "a-very-long-state-string-12345", true},
		{"unicode match", "状态码123", "状态码123", true},
		{"unicode mismatch", "状态码123", "状态码456", false},
		{"single char match", "x", "x", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := VerifyState(tt.expected, tt.actual)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestStateCookieName(t *testing.T) {
	tests := []struct {
		provider string
		want     string
	}{
		{"github", "oidc_state_github"},
		{"google", "oidc_state_google"},
		{"custom-oidc", "oidc_state_custom-oidc"},
		{"", "oidc_state_"},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			got := stateCookieName(tt.provider)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestProviderCookieName(t *testing.T) {
	tests := []struct {
		provider string
		want     string
	}{
		{"github", "oidc_provider_github"},
		{"", "oidc_provider_"},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			got := providerCookieName(tt.provider)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestSetStateCookie(t *testing.T) {
	tests := []struct {
		name       string
		tls        *tls.ConnectionState
		fwdProto   string
		wantSecure bool
	}{
		{"plain HTTP", nil, "", false},
		{"direct TLS", &tls.ConnectionState{}, "", true},
		{"reverse proxy HTTPS", nil, "https", true},
		{"reverse proxy HTTP", nil, "http", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			r := httptest.NewRequest("GET", "/api/auth/oidc/github/login", nil)
			if tt.tls != nil {
				r.TLS = tt.tls
			}
			if tt.fwdProto != "" {
				r.Header.Set("X-Forwarded-Proto", tt.fwdProto)
			}

			SetStateCookie(w, r, "github", "test-state")

			resp := w.Result()
			cookies := resp.Cookies()
			require.Len(t, cookies, 1)
			assert.Equal(t, "oidc_state_github", cookies[0].Name)
			assert.Equal(t, "test-state", cookies[0].Value)
			assert.Equal(t, 300, cookies[0].MaxAge)
			assert.True(t, cookies[0].HttpOnly)
			assert.Equal(t, http.SameSiteLaxMode, cookies[0].SameSite)
			assert.Equal(t, tt.wantSecure, cookies[0].Secure)
		})
	}
}

func TestClearStateCookie(t *testing.T) {
	w := httptest.NewRecorder()
	ClearStateCookie(w, "google")

	resp := w.Result()
	cookies := resp.Cookies()
	require.Len(t, cookies, 1)
	assert.Equal(t, "oidc_state_google", cookies[0].Name)
	assert.Equal(t, "", cookies[0].Value)
	assert.Equal(t, -1, cookies[0].MaxAge, "MaxAge=-1 deletes cookie")
}

func TestIsHTTPS_TableDriven(t *testing.T) {
	tests := []struct {
		name     string
		tls      *tls.ConnectionState
		fwdProto string
		want     bool
	}{
		{"plain HTTP", nil, "", false},
		{"direct TLS", &tls.ConnectionState{}, "", true},
		{"X-Forwarded-Proto https", nil, "https", true},
		{"X-Forwarded-Proto http", nil, "http", false},
		{"X-Forwarded-Proto empty", nil, "", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			if tt.tls != nil {
				r.TLS = tt.tls
			}
			if tt.fwdProto != "" {
				r.Header.Set("X-Forwarded-Proto", tt.fwdProto)
			}

			got := isHTTPS(r)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestRandomString_TableDriven(t *testing.T) {
	// Test that randomString generates unique values and correct lengths
	tests := []struct {
		inputBytes int
		expectLen  int
	}{
		{16, 24}, // ceil(16/3)*4 = 24
		{32, 44}, // ceil(32/3)*4 = 44
		{48, 64}, // ceil(48/3)*4 = 64
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			s1, err := randomString(tt.inputBytes)
			require.NoError(t, err)
			assert.Equal(t, tt.expectLen, len(s1))

			// Generate a second one — should be different (random)
			s2, err := randomString(tt.inputBytes)
			require.NoError(t, err)
			assert.NotEqual(t, s1, s2, "two random strings should differ")
		})
	}
}
