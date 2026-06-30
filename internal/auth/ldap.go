package auth

import (
	"crypto/tls"
	"fmt"
	"strings"

	"github.com/go-ldap/ldap/v3"
)

// ldapAuthenticateWithConfig authenticates using a specific LDAP config (from DB provider).
func (a *Authenticator) ldapAuthenticateWithConfig(cfg *LDAPConfig, username, password string) (string, map[string][]string, error) {
	if cfg == nil || cfg.Server == "" {
		return "", nil, fmt.Errorf("LDAP server not configured")
	}

	l, err := ldapConnectConfig(cfg)
	if err != nil {
		return "", nil, fmt.Errorf("failed to connect to LDAP: %w", err)
	}
	defer l.Close()

	// Step 1: Bind with service account to search for the user
	if err := l.Bind(cfg.BindDN, cfg.BindPW); err != nil {
		return "", nil, fmt.Errorf("LDAP service bind failed: %w", err)
	}

	// Step 2: Search for the user DN
	filter := strings.ReplaceAll(cfg.SearchFilter, "{username}", ldap.EscapeFilter(username))
	if filter == "" {
		filter = fmt.Sprintf("(uid=%s)", ldap.EscapeFilter(username))
	}

	searchReq := ldap.NewSearchRequest(
		cfg.SearchBase,
		ldap.ScopeWholeSubtree, ldap.NeverDerefAliases, 0, 0, false,
		filter,
		[]string{"dn", "cn", "mail", "uid", "memberOf", "displayName"},
		nil,
	)

	sr, err := l.Search(searchReq)
	if err != nil {
		return "", nil, fmt.Errorf("LDAP search failed: %w", err)
	}
	if len(sr.Entries) == 0 {
		return "", nil, ErrInvalidCredentials
	}
	if len(sr.Entries) > 1 {
		return "", nil, fmt.Errorf("LDAP search returned multiple results for %s", username)
	}

	entry := sr.Entries[0]
	userDN := entry.DN

	// Step 3: Rebind with the user's DN and password to verify credentials
	if err := l.Bind(userDN, password); err != nil {
		return "", nil, ErrInvalidCredentials
	}

	// Convert entry attributes to map
	attrs := make(map[string][]string)
	for _, attr := range entry.Attributes {
		attrs[attr.Name] = attr.Values
	}

	return userDN, attrs, nil
}

// ldapConnectConfig creates a new LDAP connection from a provider config.
func ldapConnectConfig(cfg *LDAPConfig) (*ldap.Conn, error) {
	server := cfg.Server
	if server == "" {
		return nil, fmt.Errorf("LDAP server not configured")
	}

	var l *ldap.Conn
	var err error

	if strings.HasPrefix(server, "ldaps://") {
		l, err = ldap.DialURL(server, ldap.DialWithTLSConfig(&tls.Config{
			InsecureSkipVerify: cfg.SkipTLSVerify,
		}))
	} else {
		l, err = ldap.DialURL(server)
	}
	if err != nil {
		return nil, err
	}

	if cfg.StartTLS && !strings.HasPrefix(server, "ldaps://") {
		if err := l.StartTLS(&tls.Config{InsecureSkipVerify: cfg.SkipTLSVerify}); err != nil {
			l.Close()
			return nil, fmt.Errorf("LDAP StartTLS failed: %w", err)
		}
	}

	l.SetTimeout(0)
	return l, nil
}

// ldapAttr safely extracts a single attribute value.
func ldapAttr(attrs map[string][]string, key string, fallback ...string) string {
	if vals, ok := attrs[key]; ok && len(vals) > 0 {
		return vals[0]
	}
	if len(fallback) > 0 {
		return fallback[0]
	}
	return ""
}
