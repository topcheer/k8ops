// Package auth provides authentication and authorization for the dashboard.
// It supports local users, LDAP/AD, and OIDC providers, with SQLite as
// the default persistent store (extensible to MySQL/PostgreSQL via GORM).
package auth

import (
	"fmt"
	"strings"
	"time"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// User represents an authenticated dashboard user.
type User struct {
	ID                uint      `gorm:"primaryKey" json:"id"`
	Username          string    `gorm:"uniqueIndex;size:255;not null" json:"username"`
	Email             string    `gorm:"index;size:255" json:"email"`
	DisplayName       string    `gorm:"size:255" json:"display_name"`
	PasswordHash      string    `gorm:"size:255" json:"-"`                       // bcrypt hash, never serialized
	Role              string    `gorm:"size:32;default:'viewer'" json:"role"`    // admin, operator, viewer, ns-admin, ns-viewer
	Provider          string    `gorm:"size:64;default:'local'" json:"provider"` // local, ldap, oidc
	ProviderID        string    `gorm:"size:255" json:"provider_id"`
	AllowedNamespaces string    `gorm:"size:1024" json:"allowed_namespaces"` // comma-separated, for ns-* roles
	MustChangePwd     bool      `gorm:"default:false" json:"must_change_pwd"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// TableName overrides the table name.
func (User) TableName() string { return "users" }

// Store wraps the GORM DB connection for auth persistence.
type Store struct {
	db *gorm.DB
}

// NewStore opens (or creates) a database using the provided DSN and driver.
// driver: "sqlite" (default), "mysql", "postgres"
// dsn: connection string appropriate for the driver
//
// Examples:
//
//	sqlite:   /data/k8ops.db
//	mysql:    user:password@tcp(host:3306)/k8ops?charset=utf8mb4&parseTime=True
//	postgres: host=localhost user=postgres password=secret dbname=k8ops port=5432 sslmode=disable
func NewStore(driver, dsn string) (*Store, error) {
	var dialector gorm.Dialector

	switch strings.ToLower(driver) {
	case "", "sqlite", "sqlite3":
		dsn := dsn
		if dsn == "" {
			dsn = "/data/k8ops.db"
		}
		// For in-memory SQLite, don't append pragma query params (breaks :memory: DSN)
		if dsn == ":memory:" {
			dialector = sqlite.Open(dsn)
		} else {
			dialector = sqlite.Open(dsn + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)")
		}
	case "mysql":
		dialector = mysql.Open(dsn)
	case "postgres", "postgresql", "pg":
		dialector = postgres.Open(dsn)
	default:
		return nil, fmt.Errorf("unsupported database driver: %s (use sqlite, mysql, or postgres)", driver)
	}

	db, err := gorm.Open(dialector, &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to open auth database (%s): %w", driver, err)
	}

	// SQLite-specific optimizations
	// Note: PRAGMA settings also appear in the DSN query params, but
	// gorm/sqlite does not reliably apply them there. Explicit Exec
	// guarantees WAL mode and busy timeout are active.
	if strings.ToLower(driver) == "" || strings.ToLower(driver) == "sqlite" || strings.ToLower(driver) == "sqlite3" {
		db.Exec("PRAGMA journal_mode=WAL;")
		db.Exec("PRAGMA busy_timeout=5000;")
	}

	if err := db.AutoMigrate(&User{}); err != nil {
		return nil, fmt.Errorf("failed to migrate auth database: %w", err)
	}

	return &Store{db: db}, nil
}

// CreateUser creates a new user record.
func (s *Store) CreateUser(user *User) error {
	return s.db.Create(user).Error
}

// GetUserByUsername looks up a user by username.
func (s *Store) GetUserByUsername(username string) (*User, error) {
	var u User
	if err := s.db.Where("username = ?", username).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// GetUserByID looks up a user by ID.
func (s *Store) GetUserByID(id uint) (*User, error) {
	var u User
	if err := s.db.First(&u, id).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// GetUserByProvider looks up a user by provider + providerID (for LDAP/OIDC).
func (s *Store) GetUserByProvider(provider, providerID string) (*User, error) {
	var u User
	if err := s.db.Where("provider = ? AND provider_id = ?", provider, providerID).First(&u).Error; err != nil {
		return nil, err
	}
	return &u, nil
}

// UpdateUser updates an existing user record.
func (s *Store) UpdateUser(user *User) error {
	return s.db.Save(user).Error
}

// DeleteUser deletes a user by ID.
func (s *Store) DeleteUser(id uint) error {
	result := s.db.Delete(&User{}, id)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return ErrUserNotFound
	}
	return nil
}

// ListUsers returns all users.
func (s *Store) ListUsers() ([]User, error) {
	var users []User
	if err := s.db.Order("created_at DESC").Find(&users).Error; err != nil {
		return nil, err
	}
	return users, nil
}

// CountUsers returns the total number of users.
func (s *Store) CountUsers() (int64, error) {
	var count int64
	if err := s.db.Model(&User{}).Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

// Close closes the underlying database connection.
func (s *Store) Close() error {
	sqlDB, err := s.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}
