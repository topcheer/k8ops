package auth

import (
	"testing"
)

func TestRandomString(t *testing.T) {
	t.Run("correct length for various input sizes", func(t *testing.T) {
		// base64.URLEncoding encodes 3 bytes → 4 chars, so n bytes → ceil(n*4/3) chars (with padding)
		// For n=32: 32*4/3 = 42.67 → 44 chars (with = padding)
		tests := []struct {
			inputBytes int
		}{
			{16},
			{32},
			{48},
			{64},
		}
		for _, tt := range tests {
			s, err := randomString(tt.inputBytes)
			if err != nil {
				t.Fatalf("randomString(%d) returned error: %v", tt.inputBytes, err)
			}
			// base64.URLEncoding always produces output of length ceil(n/3)*4
			expectedLen := ((tt.inputBytes + 2) / 3) * 4
			if len(s) != expectedLen {
				t.Errorf("randomString(%d) len = %d, want %d", tt.inputBytes, len(s), expectedLen)
			}
		}
	})

	t.Run("non-empty output", func(t *testing.T) {
		s, err := randomString(32)
		if err != nil {
			t.Fatalf("randomString error: %v", err)
		}
		if s == "" {
			t.Error("randomString returned empty string")
		}
	})

	t.Run("uniqueness over 100 consecutive calls", func(t *testing.T) {
		seen := make(map[string]bool, 100)
		for i := 0; i < 100; i++ {
			s, err := randomString(32)
			if err != nil {
				t.Fatalf("randomString error on call %d: %v", i, err)
			}
			if seen[s] {
				t.Fatalf("duplicate random string at call %d: %q", i, s)
			}
			seen[s] = true
		}
		if len(seen) != 100 {
			t.Errorf("expected 100 unique strings, got %d", len(seen))
		}
	})
}

// Note: TestVerifyState, TestIsHTTPS, TestStateCookieName, TestSetAndClearStateCookie,
// and TestSetStateCookieSecureFlagWithTLS already exist in oidc_state_test.go.
// They are not duplicated here to avoid compile errors from duplicate function names.
