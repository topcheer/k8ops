package dashboard

import (
	"net/http"
	"runtime"
	"time"
)

// handleSystemInfo returns system-level operational info.
// GET /api/system/info
func (s *Server) handleSystemInfo(w http.ResponseWriter, r *http.Request) {
	info := map[string]any{
		"version":    Version,
		"goVersion":  runtime.Version(),
		"platform":   runtime.GOOS + "/" + runtime.GOARCH,
		"cpus":       runtime.NumCPU(),
		"goroutines": runtime.NumGoroutine(),
		"timestamp":  time.Now().Format(time.RFC3339),
	}

	// Memory stats
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	info["memory"] = map[string]any{
		"allocMB":      m.Alloc / 1024 / 1024,
		"totalAllocMB": m.TotalAlloc / 1024 / 1024,
		"sysMB":        m.Sys / 1024 / 1024,
		"gcCycles":     m.NumGC,
		"heapObjects":  m.HeapObjects,
	}

	// Audit log info
	if s.auditLog != nil {
		logSize, _ := s.auditLog.FileSize()
		info["auditLog"] = map[string]any{
			"sizeBytes": logSize,
			"sizeMB":    logSize / 1024 / 1024,
			"events":    len(s.auditLog.Recent(10000)),
		}
	}

	// Server uptime
	if s.startTime != nil {
		info["uptime"] = time.Since(*s.startTime).String()
	}

	writeJSON(w, info)
}

// handleLogRotate triggers manual rotation of the audit log.
// POST /api/system/log/rotate
func (s *Server) handleLogRotate(w http.ResponseWriter, r *http.Request) {
	if s.auditLog == nil {
		writeError(w, http.StatusServiceUnavailable, "audit log not configured")
		return
	}

	if err := s.auditLog.Rotate(); err != nil {
		writeError(w, http.StatusInternalServerError, "rotation failed: "+err.Error())
		return
	}

	size, _ := s.auditLog.FileSize()
	writeJSON(w, map[string]any{
		"success": true,
		"message": "audit log rotated",
		"newSize": size,
	})
}

// handleLogCleanup removes old rotated audit log files.
// POST /api/system/log/cleanup
func (s *Server) handleLogCleanup(w http.ResponseWriter, r *http.Request) {
	if s.auditLog == nil {
		writeError(w, http.StatusServiceUnavailable, "audit log not configured")
		return
	}

	removed, err := s.auditLog.Cleanup()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "cleanup failed: "+err.Error())
		return
	}

	writeJSON(w, map[string]any{
		"success": true,
		"removed": removed,
		"message": "cleanup completed",
	})
}
