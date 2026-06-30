package auth

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToAPIConfig_MasksSecrets(t *testing.T) {
	// Build a provider with both LDAP and OIDC config containing real secrets
	p := &AuthProvider{
		Name: "test-provider",
		Type: ProviderTypeOIDC,
	}
	p.Config = `{"ldap":{"server":"ldap://host:389","bind_dn":"cn=admin","bind_pw":"super-secret-password","search_base":"dc=example,dc=com"},"oidc":{"issuer":"https://idp.example.com","client_id":"my-client","client_secret":"my-secret-client-secret","redirect_url":"https://k8ops.example.com/callback","scopes":["openid","profile","email"]}}`

	api := p.ToAPIConfig()

	// LDAP BindPW must be masked
	if api.LDAP == nil {
		t.Fatal("expected LDAP config to be non-nil")
	}
	if api.LDAP.BindPW != "••••••••" {
		t.Errorf("LDAP BindPW = %q, want masked '••••••••'", api.LDAP.BindPW)
	}
	// Non-secret LDAP fields should be preserved
	if api.LDAP.Server != "ldap://host:389" {
		t.Errorf("LDAP Server = %q, want 'ldap://host:389'", api.LDAP.Server)
	}
	if api.LDAP.BindDN != "cn=admin" {
		t.Errorf("LDAP BindDN = %q, want 'cn=admin'", api.LDAP.BindDN)
	}

	// OIDC ClientSecret must be masked
	if api.OIDC == nil {
		t.Fatal("expected OIDC config to be non-nil")
	}
	if api.OIDC.ClientSecret != "••••••••" {
		t.Errorf("OIDC ClientSecret = %q, want masked '••••••••'", api.OIDC.ClientSecret)
	}
	// Non-secret OIDC fields should be preserved
	if api.OIDC.ClientID != "my-client" {
		t.Errorf("OIDC ClientID = %q, want 'my-client'", api.OIDC.ClientID)
	}
	if api.OIDC.Issuer != "https://idp.example.com" {
		t.Errorf("OIDC Issuer = %q, want 'https://idp.example.com'", api.OIDC.Issuer)
	}
}

func TestToAPIConfig_EmptyConfig(t *testing.T) {
	p := &AuthProvider{Name: "empty"}
	api := p.ToAPIConfig()
	if api.LDAP != nil {
		t.Error("expected nil LDAP for empty config")
	}
	if api.OIDC != nil {
		t.Error("expected nil OIDC for empty config")
	}
}

func TestToAPIConfig_NoSecretsToMask(t *testing.T) {
	// Config with empty secrets — masking should be a no-op (stays empty)
	p := &AuthProvider{Name: "no-secrets"}
	p.Config = `{"ldap":{"server":"ldap://host:389","bind_dn":"cn=admin","bind_pw":""},"oidc":{"issuer":"https://idp.example.com","client_id":"cid","client_secret":""}}`

	api := p.ToAPIConfig()
	if api.LDAP.BindPW != "" {
		t.Errorf("empty BindPW should stay empty, got %q", api.LDAP.BindPW)
	}
	if api.OIDC.ClientSecret != "" {
		t.Errorf("empty ClientSecret should stay empty, got %q", api.OIDC.ClientSecret)
	}
}

func TestParseConfig(t *testing.T) {
	t.Run("round-trip with real secrets preserved", func(t *testing.T) {
		original := ProviderConfig{
			LDAP: &LDAPConfig{
				Server:       "ldaps://dc01.corp.local:636",
				BindDN:       "CN=svc-ldap,OU=Service,DC=corp,DC=local",
				BindPW:       "real-password-123",
				SearchBase:   "DC=corp,DC=local",
				SearchFilter: "(sAMAccountName={username})",
				StartTLS:     true,
			},
			OIDC: &OIDCConfig{
				Issuer:       "https://accounts.google.com",
				ClientID:     "g-client-id",
				ClientSecret: "g-client-secret",
				RedirectURL:  "https://k8ops.corp.local/api/auth/oidc/google/callback",
				Scopes:       []string{"openid", "profile", "email"},
			},
		}

		p := &AuthProvider{Name: "test"}
		if err := p.SetConfig(original); err != nil {
			t.Fatalf("SetConfig failed: %v", err)
		}

		parsed := p.ParseConfig()

		// LDAP fields — real secrets must be preserved (not masked)
		if parsed.LDAP.BindPW != "real-password-123" {
			t.Errorf("ParseConfig LDAP BindPW = %q, want 'real-password-123'", parsed.LDAP.BindPW)
		}
		if parsed.LDAP.Server != original.LDAP.Server {
			t.Errorf("ParseConfig LDAP Server mismatch")
		}
		if parsed.LDAP.StartTLS != true {
			t.Errorf("ParseConfig LDAP StartTLS = %v, want true", parsed.LDAP.StartTLS)
		}

		// OIDC fields — real secrets must be preserved (not masked)
		if parsed.OIDC.ClientSecret != "g-client-secret" {
			t.Errorf("ParseConfig OIDC ClientSecret = %q, want 'g-client-secret'", parsed.OIDC.ClientSecret)
		}
		if parsed.OIDC.ClientID != original.OIDC.ClientID {
			t.Errorf("ParseConfig OIDC ClientID mismatch")
		}
		if len(parsed.OIDC.Scopes) != 3 {
			t.Errorf("ParseConfig OIDC Scopes len = %d, want 3", len(parsed.OIDC.Scopes))
		}
	})

	t.Run("empty config returns zero value", func(t *testing.T) {
		p := &AuthProvider{Name: "empty"}
		pc := p.ParseConfig()
		if pc.LDAP != nil || pc.OIDC != nil {
			t.Error("expected nil LDAP and OIDC for empty config")
		}
	})

	t.Run("invalid JSON returns zero value", func(t *testing.T) {
		p := &AuthProvider{Name: "bad", Config: "{not valid json"}
		pc := p.ParseConfig()
		if pc.LDAP != nil || pc.OIDC != nil {
			t.Error("expected nil LDAP and OIDC for invalid JSON")
		}
	})
}

func TestSetConfig(t *testing.T) {
	t.Run("serializes LDAP and OIDC to JSON", func(t *testing.T) {
		pc := ProviderConfig{
			LDAP: &LDAPConfig{
				Server:     "ldap://test:389",
				BindDN:     "cn=admin,dc=test",
				BindPW:     "secret",
				SearchBase: "dc=test",
			},
			OIDC: &OIDCConfig{
				Issuer:       "https://idp.test.com",
				ClientID:     "cid",
				ClientSecret: "csec",
				RedirectURL:  "https://app.test.com/cb",
				Scopes:       []string{"openid", "email"},
			},
		}

		p := &AuthProvider{Name: "test"}
		if err := p.SetConfig(pc); err != nil {
			t.Fatalf("SetConfig failed: %v", err)
		}

		if p.Config == "" {
			t.Fatal("Config should not be empty after SetConfig")
		}

		// Verify it's valid JSON and has expected fields
		var raw map[string]json.RawMessage
		if err := json.Unmarshal([]byte(p.Config), &raw); err != nil {
			t.Fatalf("Config is not valid JSON: %v", err)
		}

		// Both ldap and oidc keys should be present
		if _, ok := raw["ldap"]; !ok {
			t.Error("Config JSON missing 'ldap' key")
		}
		if _, ok := raw["oidc"]; !ok {
			t.Error("Config JSON missing 'oidc' key")
		}

		// Verify it can be parsed back
		parsed := p.ParseConfig()
		if parsed.LDAP.Server != "ldap://test:389" {
			t.Errorf("round-trip LDAP Server = %q", parsed.LDAP.Server)
		}
		if parsed.OIDC.ClientID != "cid" {
			t.Errorf("round-trip OIDC ClientID = %q", parsed.OIDC.ClientID)
		}
	})

	t.Run("empty ProviderConfig produces valid JSON", func(t *testing.T) {
		p := &AuthProvider{Name: "empty"}
		if err := p.SetConfig(ProviderConfig{}); err != nil {
			t.Fatalf("SetConfig failed: %v", err)
		}
		if p.Config != "{}" && p.Config != `{"ldap":null,"oidc":null}` {
			// json.Marshal of empty struct with omitempty fields → "{}"
			// Just verify it's valid JSON
			var v any
			if err := json.Unmarshal([]byte(p.Config), &v); err != nil {
				t.Fatalf("empty config is not valid JSON: %v (raw: %s)", err, p.Config)
			}
		}
	})
}

func TestProviderPresets(t *testing.T) {
	if len(ProviderPresets) == 0 {
		t.Fatal("ProviderPresets should not be empty")
	}

	seenKeys := make(map[string]bool)
	for i, p := range ProviderPresets {
		// Every preset must have a non-empty Key
		if p.Key == "" {
			t.Errorf("preset[%d]: Key is empty", i)
		}
		// Keys must be unique
		if seenKeys[p.Key] {
			t.Errorf("preset[%d]: duplicate Key %q", i, p.Key)
		}
		seenKeys[p.Key] = true

		// Every preset must have a Type (ldap or oidc)
		if p.Type == "" {
			t.Errorf("preset[%d] (%s): Type is empty", i, p.Key)
		}
		if p.Type != "ldap" && p.Type != "oidc" {
			t.Errorf("preset[%d] (%s): Type = %q, want 'ldap' or 'oidc'", i, p.Key, p.Type)
		}

		// Every preset must have a DisplayName
		if p.DisplayName == "" {
			t.Errorf("preset[%d] (%s): DisplayName is empty", i, p.Key)
		}
	}

	// Sanity check: known presets must be present
	requiredKeys := []string{"github", "google", "microsoft", "gitlab", "keycloak", "okta", "auth0", "ldap"}
	for _, key := range requiredKeys {
		if !seenKeys[key] {
			t.Errorf("required preset %q not found in ProviderPresets", key)
		}
	}

	// OIDC presets should have OIDC config with issuer (except custom-oidc which is manual)
	for _, p := range ProviderPresets {
		if p.Type == "oidc" && p.Key != "custom-oidc" {
			if p.OIDC == nil {
				t.Errorf("preset %q is OIDC but has no OIDC config", p.Key)
			} else {
				if p.OIDC.Issuer == "" {
					t.Errorf("preset %q OIDC config missing Issuer", p.Key)
				}
				if len(p.OIDC.Scopes) == 0 {
					t.Errorf("preset %q OIDC config has no Scopes", p.Key)
				}
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Table-driven ProviderConfig serialization tests
// ---------------------------------------------------------------------------

func TestProviderConfig_SetAndParse(t *testing.T) {
	tests := []struct {
		name string
		pc   ProviderConfig
	}{
		{
			"LDAP only",
			ProviderConfig{
				LDAP: &LDAPConfig{
					Server:       "ldap://host:389",
					BindDN:       "cn=admin,dc=example,dc=com",
					BindPW:       "secret",
					SearchBase:   "ou=users,dc=example,dc=com",
					SearchFilter: "(uid={username})",
					StartTLS:     true,
				},
			},
		},
		{
			"OIDC only",
			ProviderConfig{
				OIDC: &OIDCConfig{
					Issuer:       "https://accounts.google.com",
					ClientID:     "client-123",
					ClientSecret: "secret-456",
					RedirectURL:  "https://k8ops.example.com/api/auth/oidc/google/callback",
					Scopes:       []string{"openid", "profile", "email"},
				},
			},
		},
		{
			"both LDAP and OIDC",
			ProviderConfig{
				LDAP: &LDAPConfig{
					Server:   "ldap://host:389",
					BindDN:   "cn=admin",
					BindPW:   "ldap-secret",
				},
				OIDC: &OIDCConfig{
					Issuer:       "https://issuer.example.com",
					ClientID:     "id",
					ClientSecret: "oidc-secret",
					RedirectURL:  "https://redirect.example.com",
					Scopes:       []string{"openid", "profile"},
				},
			},
		},
		{
			"empty config",
			ProviderConfig{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &AuthProvider{Name: "test-provider"}
			require.NoError(t, p.SetConfig(tt.pc))

			// ParseConfig should return the original (with real secrets)
			parsed := p.ParseConfig()

			if tt.pc.LDAP != nil {
				require.NotNil(t, parsed.LDAP)
				assert.Equal(t, tt.pc.LDAP.Server, parsed.LDAP.Server)
				assert.Equal(t, tt.pc.LDAP.BindPW, parsed.LDAP.BindPW, "ParseConfig returns real secrets")
			}
			if tt.pc.OIDC != nil {
				require.NotNil(t, parsed.OIDC)
				assert.Equal(t, tt.pc.OIDC.Issuer, parsed.OIDC.Issuer)
				assert.Equal(t, tt.pc.OIDC.ClientSecret, parsed.OIDC.ClientSecret, "ParseConfig returns real secrets")
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ToAPIConfig masks secrets tests
// ---------------------------------------------------------------------------

func TestToAPIConfig_MasksSecrets_TableDriven(t *testing.T) {
	tests := []struct {
		name          string
		pc            ProviderConfig
		wantLDAPBindPW string
		wantOIDCSecret string
	}{
		{
			"LDAP with secret",
			ProviderConfig{LDAP: &LDAPConfig{BindPW: "super-secret"}},
			"\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022",
			"",
		},
		{
			"OIDC with secret",
			ProviderConfig{OIDC: &OIDCConfig{ClientSecret: "oidc-secret"}},
			"",
			"\u2022\u2022\u2022\u2022\u2022\u2022\u2022\u2022",
		},
		{
			"LDAP empty password (no masking)",
			ProviderConfig{LDAP: &LDAPConfig{BindPW: ""}},
			"",
			"",
		},
		{
			"OIDC empty secret (no masking)",
			ProviderConfig{OIDC: &OIDCConfig{ClientSecret: ""}},
			"",
			"",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p := &AuthProvider{Name: "mask-test"}
			require.NoError(t, p.SetConfig(tt.pc))

			api := p.ToAPIConfig()

			if tt.pc.LDAP != nil {
				assert.Equal(t, tt.wantLDAPBindPW, api.LDAP.BindPW)
			}
			if tt.pc.OIDC != nil {
				assert.Equal(t, tt.wantOIDCSecret, api.OIDC.ClientSecret)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// ParseConfig with invalid JSON
// ---------------------------------------------------------------------------

func TestParseConfig_InvalidJSON(t *testing.T) {
	p := &AuthProvider{
		Name:   "bad-json",
		Config: `{invalid json`,
	}

	// Should not panic, just log a warning and return empty
	pc := p.ParseConfig()
	assert.Nil(t, pc.LDAP)
	assert.Nil(t, pc.OIDC)
}

func TestToAPIConfig_InvalidJSON(t *testing.T) {
	p := &AuthProvider{
		Name:   "bad-json-api",
		Config: `{invalid json`,
	}

	pc := p.ToAPIConfig()
	assert.Nil(t, pc.LDAP)
	assert.Nil(t, pc.OIDC)
}

func TestParseConfig_EmptyConfig(t *testing.T) {
	p := &AuthProvider{Name: "empty", Config: ""}

	pc := p.ParseConfig()
	assert.Nil(t, pc.LDAP)
	assert.Nil(t, pc.OIDC)
}

// ---------------------------------------------------------------------------
// ProviderPresets tests
// ---------------------------------------------------------------------------

func TestProviderPresets_TableDriven(t *testing.T) {
	wantKeys := map[string]bool{
		"github": true, "google": true, "microsoft": true,
		"gitlab": true, "keycloak": true, "okta": true,
		"auth0": true, "custom-oidc": true, "ldap": true,
	}

	for _, p := range ProviderPresets {
		assert.True(t, wantKeys[p.Key], "preset key %s should be in expected set", p.Key)
		assert.NotEmpty(t, p.DisplayName)
		assert.NotEmpty(t, p.Type)
		assert.NotEmpty(t, p.Description)

		if p.Type == "oidc" && p.Key != "custom-oidc" {
			require.NotNil(t, p.OIDC, "OIDC preset %s should have OIDC config", p.Key)
			assert.NotEmpty(t, p.OIDC.Issuer)
		}
	}
}

func TestProviderPresets_FindByKey(t *testing.T) {
	tests := []struct {
		key      string
		wantName string
		wantType string
	}{
		{"github", "GitHub", "oidc"},
		{"google", "Google", "oidc"},
		{"microsoft", "Microsoft", "oidc"},
		{"gitlab", "GitLab", "oidc"},
		{"keycloak", "Keycloak", "oidc"},
		{"okta", "Okta", "oidc"},
		{"auth0", "Auth0", "oidc"},
		{"custom-oidc", "Custom OIDC", "oidc"},
		{"ldap", "LDAP / Active Directory", "ldap"},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			var found *ProviderPreset
			for i := range ProviderPresets {
				if ProviderPresets[i].Key == tt.key {
					found = &ProviderPresets[i]
					break
				}
			}
			require.NotNil(t, found, "preset %s not found", tt.key)
			assert.Equal(t, tt.wantName, found.DisplayName)
			assert.Equal(t, tt.wantType, found.Type)
		})
	}
}
