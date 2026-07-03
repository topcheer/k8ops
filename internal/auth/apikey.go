package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"
)

// APIKeyPrefix is the prefix used to identify k8ops API keys.
const APIKeyPrefix = "k8ops_"

// APIKey represents an API key for programmatic access (CLI/CI integrations).
// The key itself is never stored — only its SHA-256 hash.
type APIKey struct {
	ID         uint       `gorm:"primaryKey" json:"id"`
	UserID     uint       `gorm:"index;not null" json:"user_id"`
	Name       string     `gorm:"size:128;not null" json:"name"`          // human-readable label
	KeyHash    string     `gorm:"size:255;uniqueIndex;not null" json:"-"` // SHA-256 hash, never serialized
	KeyPrefix  string     `gorm:"size:20" json:"key_prefix"`              // first 12 chars for identification (e.g. "k8ops_ab12cd34")
	LastUsedAt *time.Time `json:"last_used_at"`                           // nil = never used
	ExpiresAt  *time.Time `json:"expires_at"`                             // nil = no expiry
	Revoked    bool       `gorm:"default:false" json:"revoked"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// TableName overrides the table name.
func (APIKey) TableName() string { return "api_keys" }

// GenerateAPIKey creates a new random API key string and its hash.
// Returns the plaintext key (to show to the user once) and the SHA-256 hash (to store).
func GenerateAPIKey() (plaintext, hash, prefix string, err error) {
	// 32 bytes of randomness = 43 base64 chars
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	plaintext = APIKeyPrefix + base64.RawURLEncoding.EncodeToString(raw)
	hash = HashAPIKey(plaintext)
	prefix = plaintext[:12] // e.g. "k8ops_ab12cd34"
	return plaintext, hash, prefix, nil
}

// HashAPIKey computes the SHA-256 hash of an API key.
// We use SHA-256 (not bcrypt) because API keys are high-entropy random strings,
// so brute-force is infeasible and SHA-256 is fast for per-request validation.
func HashAPIKey(key string) string {
	h := sha256.Sum256([]byte(key))
	return base64.StdEncoding.EncodeToString(h[:])
}

// AutoMigrateAPIKeys creates the api_keys table if it doesn't exist.
func (s *Store) AutoMigrateAPIKeys() error {
	return s.db.AutoMigrate(&APIKey{})
}

// CreateAPIKey stores a new API key record.
func (s *Store) CreateAPIKey(apiKey *APIKey) error {
	return s.db.Create(apiKey).Error
}

// GetAPIKeyByHash looks up an API key by its hash. Used during authentication.
func (s *Store) GetAPIKeyByHash(hash string) (*APIKey, error) {
	var k APIKey
	if err := s.db.Where("key_hash = ?", hash).First(&k).Error; err != nil {
		return nil, err
	}
	return &k, nil
}

// GetAPIKeyByID looks up an API key by its numeric ID. Used for revocation.
func (s *Store) GetAPIKeyByID(id uint) (*APIKey, error) {
	var k APIKey
	if err := s.db.First(&k, id).Error; err != nil {
		return nil, err
	}
	return &k, nil
}

// ListAPIKeysByUser returns all non-revoked API keys for a user.
func (s *Store) ListAPIKeysByUser(userID uint) ([]APIKey, error) {
	var keys []APIKey
	if err := s.db.Where("user_id = ? AND revoked = false", userID).Order("created_at DESC").Find(&keys).Error; err != nil {
		return nil, err
	}
	return keys, nil
}

// DeleteAPIKey revokes an API key by setting revoked=true (soft delete).
func (s *Store) DeleteAPIKey(id uint) error {
	return s.db.Model(&APIKey{}).Where("id = ?", id).Update("revoked", true).Error
}

// UpdateAPIKeyLastUsed updates the last_used_at timestamp.
func (s *Store) UpdateAPIKeyLastUsed(id uint) {
	now := time.Now()
	s.db.Model(&APIKey{}).Where("id = ?", id).Update("last_used_at", now)
}

// ValidateAPIKey checks if an API key is valid (not revoked, not expired).
// Returns the associated user on success.
func (a *Authenticator) ValidateAPIKey(plaintext string) (*User, error) {
	hash := HashAPIKey(plaintext)
	apiKey, err := a.store.GetAPIKeyByHash(hash)
	if err != nil {
		return nil, ErrInvalidCredentials
	}

	if apiKey.Revoked {
		return nil, fmt.Errorf("API key has been revoked")
	}

	// Check expiry
	if apiKey.ExpiresAt != nil && time.Now().After(*apiKey.ExpiresAt) {
		return nil, fmt.Errorf("API key has expired")
	}

	// Load the associated user
	user, err := a.store.GetUserByID(apiKey.UserID)
	if err != nil {
		return nil, ErrUserNotFound
	}

	// Update last-used timestamp synchronously (best-effort, ignore errors)
	a.store.UpdateAPIKeyLastUsed(apiKey.ID)

	return user, nil
}
