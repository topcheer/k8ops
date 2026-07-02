package dashboard

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const backupDir = "/data/backups"

// BackupInfo represents a backup file on disk.
type BackupInfo struct {
	Name      string    `json:"name"`
	Size      int64     `json:"sizeBytes"`
	SizeMB    float64   `json:"sizeMB"`
	CreatedAt time.Time `json:"createdAt"`
	Age       string    `json:"age"`
	Type      string    `json:"type"` // "full" or "audit"
}

// handleBackupDispatch routes by HTTP method to the appropriate backup handler.
func (s *Server) handleBackupDispatch(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleBackupList(w, r)
	case http.MethodPost:
		s.handleBackupCreate(w, r)
	case http.MethodDelete:
		s.handleBackupDelete(w, r)
	default:
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
	}
}

// handleBackupList returns available backup files.
// GET /api/system/backup
func (s *Server) handleBackupList(w http.ResponseWriter, r *http.Request) {
	backups, err := listBackups()
	if err != nil {
		writeJSON(w, map[string]any{
			"backups": []BackupInfo{},
			"summary": map[string]any{
				"count":       0,
				"totalSizeMB": 0,
				"backupDir":   backupDir,
				"available":   false,
				"error":       err.Error(),
			},
		})
		return
	}

	var totalSize int64
	for _, b := range backups {
		totalSize += b.Size
	}

	writeJSON(w, map[string]any{
		"backups": backups,
		"summary": map[string]any{
			"count":       len(backups),
			"totalSizeMB": float64(totalSize) / 1024 / 1024,
			"backupDir":   backupDir,
			"available":   true,
		},
	})
}

// handleBackupCreate triggers a database backup.
// POST /api/system/backup
func (s *Server) handleBackupCreate(w http.ResponseWriter, r *http.Request) {
	if err := os.MkdirAll(backupDir, 0755); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create backup dir: "+err.Error())
		return
	}

	timestamp := time.Now().UTC().Format("20060102-150405")
	backupPath := filepath.Join(backupDir, fmt.Sprintf("k8ops-db-%s.db", timestamp))

	// Copy the SQLite database file
	dbPath := "/data/k8ops.db"
	src, err := os.Open(dbPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to open database: "+err.Error())
		return
	}
	defer src.Close()

	dst, err := os.Create(backupPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create backup file: "+err.Error())
		return
	}
	defer dst.Close()

	written, err := io.Copy(dst, src)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "backup copy failed: "+err.Error())
		return
	}

	if s.log != nil {
		s.log.Info("database backup created",
			"path", backupPath, "size", written)
	}

	writeJSON(w, map[string]any{
		"success": true,
		"name":    filepath.Base(backupPath),
		"sizeMB":  float64(written) / 1024 / 1024,
		"path":    backupPath,
		"message": "backup created successfully",
	})
}

// handleBackupDelete removes a specific backup file.
// DELETE /api/system/backup/{name}
func (s *Server) handleBackupDelete(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		// Try to extract from path
		parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
		if len(parts) >= 4 {
			name = parts[len(parts)-1]
		}
	}

	if name == "" {
		writeError(w, http.StatusBadRequest, "backup name required")
		return
	}

	// Security: prevent path traversal
	cleanName := filepath.Base(name)
	if cleanName != name {
		writeError(w, http.StatusBadRequest, "invalid backup name")
		return
	}

	backupPath := filepath.Join(backupDir, cleanName)
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, "backup not found: "+cleanName)
		return
	}

	if err := os.Remove(backupPath); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete: "+err.Error())
		return
	}

	if s.log != nil {
		s.log.Info("backup deleted", "name", cleanName)
	}

	writeJSON(w, map[string]any{
		"success": true,
		"message": "backup deleted: " + cleanName,
	})
}

// listBackups scans the backup directory for backup files.
func listBackups() ([]BackupInfo, error) {
	entries, err := os.ReadDir(backupDir)
	if err != nil {
		return nil, err
	}

	backups := make([]BackupInfo, 0)
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}

		name := entry.Name()
		backupType := "full"
		if strings.Contains(name, "audit") {
			backupType = "audit"
		}

		backups = append(backups, BackupInfo{
			Name:      name,
			Size:      info.Size(),
			SizeMB:    float64(info.Size()) / 1024 / 1024,
			CreatedAt: info.ModTime(),
			Age:       ageTime(info.ModTime()),
			Type:      backupType,
		})
	}

	// Sort by creation time descending (newest first)
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt.After(backups[j].CreatedAt)
	})

	return backups, nil
}

// handleBackupRestore restores from a backup file.
// POST /api/system/backup/restore?name=<filename>
func (s *Server) handleBackupRestore(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if name == "" {
		writeError(w, http.StatusBadRequest, "backup name required (?name=)")
		return
	}

	cleanName := filepath.Base(name)
	if cleanName != name {
		writeError(w, http.StatusBadRequest, "invalid backup name")
		return
	}

	backupPath := filepath.Join(backupDir, cleanName)
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		writeError(w, http.StatusNotFound, "backup not found: "+cleanName)
		return
	}

	dbPath := "/data/k8ops.db"

	// Copy backup to database location
	src, err := os.Open(backupPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to open backup: "+err.Error())
		return
	}
	defer src.Close()

	dst, err := CreateFile(dbPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to open database for write: "+err.Error())
		return
	}
	defer dst.Close()

	written, err := io.Copy(dst, src)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "restore copy failed: "+err.Error())
		return
	}

	if s.log != nil {
		s.log.Info("database restored from backup",
			"source", backupPath, "size", written)
	}

	writeJSON(w, map[string]any{
		"success":      true,
		"restoredFrom": cleanName,
		"sizeMB":       float64(written) / 1024 / 1024,
		"message":      "restore completed — restart pod to apply changes",
		"note":         "The restored data will be active after the pod restarts",
	})
}

// CreateFile is os.Create wrapper for testability.
var CreateFile = os.Create
