package dashboard

import "testing"

func TestIsCredentialEnvVar(t *testing.T) {
	tests := []struct {
		name string
		want bool
	}{
		{"PASSWORD", true},
		{"DB_PASSWORD", true},
		{"api_key", true},
		{"TOKEN", true},
		{"SECRET_KEY", true},
		{"HOSTNAME", false},
		{"PORT", false},
		{"", false},
	}
	for _, tt := range tests {
		if got := isCredentialEnvVar(tt.name); got != tt.want {
			t.Errorf("isCredentialEnvVar(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestClassifyCredential(t *testing.T) {
	tests := []struct {
		name string
		want string
	}{
		{"DB_PASSWORD", "password"},
		{"API_TOKEN", "token"},
		{"X_API_KEY", "api_key"},
		{"PRIVATE_KEY", "private_key"},
		{"ACCESS_KEY_ID", "access_key"},
		{"MY_CREDENTIAL", "credential"},
		{"FOO_SECRET", "secret"},
	}
	for _, tt := range tests {
		if got := classifyCredential(tt.name); got != tt.want {
			t.Errorf("classifyCredential(%q) = %q, want %q", tt.name, got, tt.want)
		}
	}
}

func TestIsSensitiveKey(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"password", true},
		{"tls.crt", false},
		{"username", false},
		{"config.yaml", false},
		{"private.key", true},
		{"api_token", true},
	}
	for _, tt := range tests {
		if got := isSensitiveKey(tt.key); got != tt.want {
			t.Errorf("isSensitiveKey(%q) = %v, want %v", tt.key, got, tt.want)
		}
	}
}

func TestSecretScanScore(t *testing.T) {
	tests := []struct {
		name     string
		summary  SecretExpSummary
		minScore int
		maxScore int
	}{
		{"no secrets", SecretExpSummary{}, 95, 100},
		{"clean", SecretExpSummary{TotalSecrets: 10, SecretsMounted: 10}, 90, 100},
		{"exposed", SecretExpSummary{TotalSecrets: 10, ExposedPlainSecrets: 5}, 80, 90},
		{"stale+unreferenced", SecretExpSummary{TotalSecrets: 20, StaleSecrets: 10, UnreferencedSecrets: 10}, 60, 85},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			score := secretScanScore(tt.summary)
			if score < tt.minScore || score > tt.maxScore {
				t.Errorf("score = %d, want [%d, %d]", score, tt.minScore, tt.maxScore)
			}
		})
	}
}

func TestSecretScanRecommendations(t *testing.T) {
	t.Run("healthy", func(t *testing.T) {
		r := &SecretExpResult{Summary: SecretExpSummary{TotalSecrets: 10, SecretsMounted: 10}}
		recs := secretScanRecommendations(r)
		if len(recs) == 0 {
			t.Error("expected at least one recommendation")
		}
	})
	t.Run("with issues", func(t *testing.T) {
		r := &SecretExpResult{
			Summary: SecretExpSummary{
				ExposedPlainSecrets: 3, StaleSecrets: 5, UnreferencedSecrets: 4,
			},
			EnvVarLeaks: []EnvLeakEntry{{HasValue: true}},
		}
		recs := secretScanRecommendations(r)
		if len(recs) < 3 {
			t.Errorf("expected at least 3 recommendations, got %d", len(recs))
		}
	})
}
