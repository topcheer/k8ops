package auth

import (
	"encoding/json"
	"log/slog"
	"time"
)

// AuthProviderType is the type of authentication provider.
type AuthProviderType string

const (
	ProviderTypeLDAP AuthProviderType = "ldap"
	ProviderTypeOIDC AuthProviderType = "oidc"
)

// AuthProvider represents a configurable authentication source.
// Multiple providers of the same type can coexist and be simultaneously enabled.
type AuthProvider struct {
	ID          uint             `gorm:"primaryKey" json:"id"`
	Name        string           `gorm:"uniqueIndex;size:64;not null" json:"name"`          // unique slug: "company-ldap", "github", "google"
	Type        AuthProviderType `gorm:"size:16;not null" json:"type"`                       // "ldap" or "oidc"
	DisplayName string           `gorm:"size:255" json:"display_name"`                       // "Company LDAP"
	Icon        string           `gorm:"size:64" json:"icon"`                                // preset key: "github", "google", "microsoft", "gitlab", "okta", "keycloak", "auth0", "ldap", or emoji
	Enabled     bool             `gorm:"default:true" json:"enabled"`
	Priority    int              `gorm:"default:0" json:"priority"`                          // lower = tried first in login
	Config      string           `gorm:"type:text" json:"-"`                                 // JSON config (secrets masked in API)
	CreatedAt   time.Time        `json:"created_at"`
	UpdatedAt   time.Time        `json:"updated_at"`
}

// TableName overrides the table name.
func (AuthProvider) TableName() string { return "auth_providers" }

// LDAPConfig holds LDAP-specific provider configuration.
type LDAPConfig struct {
	Server        string `json:"server"`          // ldap://host:389 or ldaps://host:636
	BindDN        string `json:"bind_dn"`         // service account DN
	BindPW        string `json:"bind_pw"`         // service account password (secret)
	SearchBase    string `json:"search_base"`     // base DN for user search
	SearchFilter  string `json:"search_filter"`   // (uid={username})
	StartTLS      bool   `json:"start_tls"`
	SkipTLSVerify bool   `json:"skip_tls_verify"` // skip TLS certificate verification (default: false)
}

// OIDCConfig holds OIDC-specific provider configuration.
type OIDCConfig struct {
	Issuer       string   `json:"issuer"`        // https://accounts.google.com
	ClientID     string   `json:"client_id"`
	ClientSecret string   `json:"client_secret"` // secret
	RedirectURL  string   `json:"redirect_url"`  // https://k8ops.example.com/api/auth/oidc/{name}/callback
	Scopes       []string `json:"scopes"`        // default: ["openid", "profile", "email"]
}

// ProviderConfig is the API-facing config with secrets masked.
type ProviderConfig struct {
	LDAP *LDAPConfig `json:"ldap,omitempty"`
	OIDC *OIDCConfig `json:"oidc,omitempty"`
}

// ToAPIConfig converts stored Config string to API-safe struct (secrets masked).
func (p *AuthProvider) ToAPIConfig() ProviderConfig {
	var pc ProviderConfig
	if p.Config == "" {
		return pc
	}
	if err := json.Unmarshal([]byte(p.Config), &pc); err != nil {
		slog.Warn("failed to unmarshal provider config", "provider", p.Name, "error", err)
	}

	// Mask secrets
	if pc.LDAP != nil && pc.LDAP.BindPW != "" {
		pc.LDAP.BindPW = "••••••••"
	}
	if pc.OIDC != nil && pc.OIDC.ClientSecret != "" {
		pc.OIDC.ClientSecret = "••••••••"
	}
	return pc
}

// ParseConfig returns the raw config struct (with real secrets).
func (p *AuthProvider) ParseConfig() ProviderConfig {
	var pc ProviderConfig
	if p.Config == "" {
		return pc
	}
	if err := json.Unmarshal([]byte(p.Config), &pc); err != nil {
		slog.Warn("failed to unmarshal provider config", "provider", p.Name, "error", err)
	}
	return pc
}

// SetConfig serializes config to JSON.
func (p *AuthProvider) SetConfig(pc ProviderConfig) error {
	b, err := json.Marshal(pc)
	if err != nil {
		return err
	}
	p.Config = string(b)
	return nil
}

// --- Built-in OIDC presets ---

type ProviderPreset struct {
	Key         string   `json:"key"`
	Type        string   `json:"type"`
	DisplayName string   `json:"display_name"`
	Icon        string   `json:"icon"`
	Description string   `json:"description"`
	OIDC        *struct {
		Issuer  string   `json:"issuer"`
		Scopes  []string `json:"scopes"`
	} `json:"oidc,omitempty"`
	Help string `json:"help"` // configuration guidance
}

var ProviderPresets = []ProviderPreset{
	{
		Key: "github", Type: "oidc", DisplayName: "GitHub", Icon: "github",
		Description: "GitHub OAuth / OIDC",
		OIDC: &struct {
			Issuer  string   `json:"issuer"`
			Scopes  []string `json:"scopes"`
		}{Issuer: "https://token.actions.githubusercontent.com", Scopes: []string{"openid", "profile", "email", "read:user"}},
		Help: "GitHub Settings → Developer settings → OAuth Apps → New OAuth App. Authorization callback URL = your redirect_url.",
	},
	{
		Key: "google", Type: "oidc", DisplayName: "Google", Icon: "google",
		Description: "Google Workspace / Gmail",
		OIDC: &struct {
			Issuer  string   `json:"issuer"`
			Scopes  []string `json:"scopes"`
		}{Issuer: "https://accounts.google.com", Scopes: []string{"openid", "profile", "email"}},
		Help: "Google Cloud Console → APIs & Services → Credentials → Create OAuth 2.0 Client ID. Authorized redirect URI = your redirect_url.",
	},
	{
		Key: "microsoft", Type: "oidc", DisplayName: "Microsoft", Icon: "microsoft",
		Description: "Microsoft Entra ID (Azure AD)",
		OIDC: &struct {
			Issuer  string   `json:"issuer"`
			Scopes  []string `json:"scopes"`
		}{Issuer: "https://login.microsoftonline.com/{tenant}/v2.0", Scopes: []string{"openid", "profile", "email"}},
		Help: "Azure Portal → App registrations → New registration. Replace {tenant} in issuer with your Directory (tenant) ID or 'common'.",
	},
	{
		Key: "gitlab", Type: "oidc", DisplayName: "GitLab", Icon: "gitlab",
		Description: "GitLab.com / self-hosted GitLab",
		OIDC: &struct {
			Issuer  string   `json:"issuer"`
			Scopes  []string `json:"scopes"`
		}{Issuer: "https://gitlab.com", Scopes: []string{"openid", "profile", "email"}},
		Help: "GitLab Admin → Applications → New application. Scopes: openid, profile, email. Redirect URI = your redirect_url.",
	},
	{
		Key: "keycloak", Type: "oidc", DisplayName: "Keycloak", Icon: "keycloak",
		Description: "Keycloak IAM",
		OIDC: &struct {
			Issuer  string   `json:"issuer"`
			Scopes  []string `json:"scopes"`
		}{Issuer: "https://keycloak.example.com/realms/{realm}", Scopes: []string{"openid", "profile", "email"}},
		Help: "Keycloak Admin → Clients → Create client. Valid Redirect URIs = your redirect_url. Replace {realm} with your realm name.",
	},
	{
		Key: "okta", Type: "oidc", DisplayName: "Okta", Icon: "okta",
		Description: "Okta Workforce Identity",
		OIDC: &struct {
			Issuer  string   `json:"issuer"`
			Scopes  []string `json:"scopes"`
		}{Issuer: "https://{your-okta-domain}.okta.com", Scopes: []string{"openid", "profile", "email"}},
		Help: "Okta Admin → Applications → Create App Integration → OIDC. Sign-in redirect URIs = your redirect_url.",
	},
	{
		Key: "auth0", Type: "oidc", DisplayName: "Auth0", Icon: "auth0",
		Description: "Auth0 by Okta",
		OIDC: &struct {
			Issuer  string   `json:"issuer"`
			Scopes  []string `json:"scopes"`
		}{Issuer: "https://{your-domain}.auth0.com", Scopes: []string{"openid", "profile", "email"}},
		Help: "Auth0 Dashboard → Applications → Create Application → Regular Web App. Allowed Callback URLs = your redirect_url.",
	},
	{
		Key: "custom-oidc", Type: "oidc", DisplayName: "Custom OIDC", Icon: "oidc",
		Description: "Any OIDC-compatible provider",
		Help: "Enter the issuer URL, client ID, and client secret manually.",
	},
	{
		Key: "ldap", Type: "ldap", DisplayName: "LDAP / Active Directory", Icon: "ldap",
		Description: "LDAP or AD server",
		Help: "Enter LDAP server URL (ldap://host:389 or ldaps://host:636), bind DN, search base, and filter.",
	},
}

// --- CRUD ---

func (s *Store) AutoMigrateAuthProviders() error {
	return s.db.AutoMigrate(&AuthProvider{})
}

func (s *Store) ListAuthProviders() ([]AuthProvider, error) {
	var providers []AuthProvider
	if err := s.db.Order("priority ASC, id ASC").Find(&providers).Error; err != nil {
		return nil, err
	}
	return providers, nil
}

func (s *Store) GetAuthProvider(name string) (*AuthProvider, error) {
	var p AuthProvider
	if err := s.db.Where("name = ?", name).First(&p).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) GetAuthProviderByID(id uint) (*AuthProvider, error) {
	var p AuthProvider
	if err := s.db.First(&p, id).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

func (s *Store) GetEnabledProvidersByType(t AuthProviderType) ([]AuthProvider, error) {
	var providers []AuthProvider
	if err := s.db.Where("type = ? AND enabled = ?", t, true).Order("priority ASC, id ASC").Find(&providers).Error; err != nil {
		return nil, err
	}
	return providers, nil
}

func (s *Store) CreateAuthProvider(p *AuthProvider) error {
	return s.db.Create(p).Error
}

func (s *Store) UpdateAuthProvider(p *AuthProvider) error {
	return s.db.Save(p).Error
}

func (s *Store) DeleteAuthProvider(id uint) error {
	return s.db.Delete(&AuthProvider{}, id).Error
}
