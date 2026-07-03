package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalidCredentials = errors.New("invalid credentials")
	ErrUserNotFound       = errors.New("user not found")
	ErrUserDisabled       = errors.New("user disabled")
)

// Config holds all auth configuration.
type Config struct {
	JWTSecret string
	JWTExpiry time.Duration // default: 24h

	// Database
	DBDriver string // "sqlite" (default), "mysql", "postgres"
	DBDSN    string // connection string; for sqlite this is a file path
	DBPath   string // legacy: sqlite file path (used when DBDriver is empty/sqlite and DBDSN is empty)

	// Default role for auto-provisioned LDAP/OIDC users
	DefaultRole              string // default: "viewer"
	DefaultAllowedNamespaces string // default: "" (cluster-wide for the role)
}

// Authenticator manages all authentication operations.
type Authenticator struct {
	store        *Store
	cfg          *Config
	rbacSyncer   *RBACSyncer
	loginLimiter *ipRateLimiter
}

// SetRBACSyncer sets the RBAC syncer for namespace-scoped role management.
func (a *Authenticator) SetRBACSyncer(s *RBACSyncer) {
	a.rbacSyncer = s
}

// New creates a new Authenticator with the given config.
func New(cfg *Config) (*Authenticator, error) {
	driver := cfg.DBDriver
	dsn := cfg.DBDSN

	// Fallback to legacy DBPath for sqlite
	if driver == "" && dsn == "" && cfg.DBPath != "" {
		driver = "sqlite"
		dsn = cfg.DBPath
	}

	store, err := NewStore(driver, dsn)
	if err != nil {
		return nil, err
	}

	if cfg.JWTExpiry == 0 {
		cfg.JWTExpiry = 24 * time.Hour
	}

	a := &Authenticator{
		store:        store,
		cfg:          cfg,
		loginLimiter: newIPRateLimiter(5, 1), // 5 burst, 1/min refill
	}

	// Bootstrap: create default admin if no users exist
	if err := a.bootstrapAdmin(); err != nil {
		return nil, fmt.Errorf("failed to bootstrap admin: %w", err)
	}

	// Migrate and seed role definitions
	if err := store.AutoMigrateRoles(); err != nil {
		return nil, fmt.Errorf("failed to migrate role defs: %w", err)
	}
	if err := store.SeedBuiltinRoles(); err != nil {
		return nil, fmt.Errorf("failed to seed builtin roles: %w", err)
	}

	// Migrate auth providers table
	if err := store.AutoMigrateAuthProviders(); err != nil {
		return nil, fmt.Errorf("failed to migrate auth providers: %w", err)
	}

	// Migrate API keys table
	if err := store.AutoMigrateAPIKeys(); err != nil {
		return nil, fmt.Errorf("failed to migrate API keys: %w", err)
	}

	return a, nil
}

// Close closes the underlying store and stops background goroutines.
func (a *Authenticator) Close() error {
	if a.loginLimiter != nil {
		a.loginLimiter.Stop()
	}
	return a.store.Close()
}

// Store returns the underlying store (for admin handlers).
func (a *Authenticator) Store() *Store { return a.store }

// Config returns the auth config.
func (a *Authenticator) Config() *Config { return a.cfg }

// defaultRole returns the configured default role for auto-provisioned users.
func (a *Authenticator) defaultRole() string {
	if a.cfg.DefaultRole != "" {
		return a.cfg.DefaultRole
	}
	return "viewer"
}

// uniqueUsername generates a unique username when there's a collision.
// e.g. if "admin" exists and an LDAP user named "admin" logs in,
// it becomes "admin_ldap" or "admin_ldap_2", etc.
func (a *Authenticator) uniqueUsername(base, provider string) string {
	candidate := base
	for i := 0; i < 100; i++ {
		existing, err := a.store.GetUserByUsername(candidate)
		if err != nil || existing == nil {
			return candidate // available
		}
		// Check if this is the same provider user (by ProviderID)
		if existing.Provider == provider && existing.Username == base {
			return candidate // this is an update, not a collision
		}
		if i == 0 {
			candidate = base + "_" + provider
		} else {
			candidate = fmt.Sprintf("%s_%s_%d", base, provider, i+1)
		}
	}
	return base + "_" + provider + "_x"
}

// LoginLocal authenticates a user with username/password (local DB).
func (a *Authenticator) LoginLocal(username, password string) (*User, string, error) {
	user, err := a.store.GetUserByUsername(username)
	if err != nil {
		return nil, "", ErrInvalidCredentials
	}

	if user.Provider != "local" {
		return nil, "", fmt.Errorf("user %s is not a local user, use %s login", username, user.Provider)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, "", ErrInvalidCredentials
	}

	token, err := a.generateToken(user)
	if err != nil {
		return nil, "", err
	}

	return user, token, nil
}

// VerifyToken parses and validates a JWT, returning the claims.
func (a *Authenticator) VerifyToken(tokenStr string) (*UserClaims, error) {
	claims := &UserClaims{}
	token, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(a.cfg.JWTSecret), nil
	})
	if err != nil || !token.Valid {
		return nil, fmt.Errorf("invalid token: %w", err)
	}
	return claims, nil
}

// UserClaims is the JWT payload.
type UserClaims struct {
	UserID   uint   `json:"uid"`
	Username string `json:"usr"`
	Role     string `json:"rol"`
	jwt.RegisteredClaims
}

// generateToken creates a signed JWT for the user.
func (a *Authenticator) generateToken(user *User) (string, error) {
	claims := &UserClaims{
		UserID:   user.ID,
		Username: user.Username,
		Role:     user.Role,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(a.cfg.JWTExpiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "k8ops",
			Subject:   user.Username,
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(a.cfg.JWTSecret))
}

// HashPassword creates a bcrypt hash of the password.
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), 12)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// bootstrapAdmin creates a default admin user if the database is empty.
func (a *Authenticator) bootstrapAdmin() error {
	count, err := a.store.CountUsers()
	if err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	hash, err := HashPassword("admin")
	if err != nil {
		return err
	}

	admin := &User{
		Username:      "admin",
		DisplayName:   "Administrator",
		Role:          "admin",
		Provider:      "local",
		PasswordHash:  hash,
		MustChangePwd: true,
	}
	return a.store.CreateUser(admin)
}

// ChangePassword updates a user's password hash.
func (a *Authenticator) ChangePassword(userID uint, oldPassword, newPassword string) error {
	user, err := a.store.GetUserByID(userID)
	if err != nil {
		return ErrUserNotFound
	}

	if user.Provider != "local" {
		return fmt.Errorf("cannot change password for %s user", user.Provider)
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(oldPassword)); err != nil {
		return ErrInvalidCredentials
	}

	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}

	user.PasswordHash = hash
	user.MustChangePwd = false
	return a.store.UpdateUser(user)
}

// AdminCreateUser creates a new user (admin operation).
func (a *Authenticator) AdminCreateUser(username, password, email, displayName, role, allowedNamespaces string) (*User, error) {
	hash, err := HashPassword(password)
	if err != nil {
		return nil, err
	}
	if role == "" {
		role = "viewer"
	}
	user := &User{
		Username:          username,
		Email:             email,
		DisplayName:       displayName,
		PasswordHash:      hash,
		Role:              role,
		AllowedNamespaces: allowedNamespaces,
		Provider:          "local",
		MustChangePwd:     true,
	}
	if err := a.store.CreateUser(user); err != nil {
		return nil, err
	}
	return user, nil
}

// AdminUpdateUser updates a user's role/display info (admin operation).
func (a *Authenticator) AdminUpdateUser(userID uint, updates map[string]any) error {
	return a.store.db.Model(&User{}).Where("id = ?", userID).Updates(updates).Error
}

// AdminResetPassword resets a user's password (admin operation).
func (a *Authenticator) AdminResetPassword(userID uint, newPassword string) error {
	hash, err := HashPassword(newPassword)
	if err != nil {
		return err
	}
	return a.store.db.Model(&User{}).Where("id = ?", userID).Updates(map[string]any{
		"password_hash":   hash,
		"must_change_pwd": true,
	}).Error
}
