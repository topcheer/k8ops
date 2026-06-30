package dashboard

import (
	"net/http"
	"strings"
)

func (s *Server) handleAudit(w http.ResponseWriter, r *http.Request) {
	if s.auditLog == nil {
		writeJSON(w, map[string]any{"count": 0, "items": []any{}})
		return
	}
	limit := 100
	events := s.auditLog.Recent(limit)
	writeJSON(w, map[string]any{"count": len(events), "items": events})
}

func (s *Server) handleAuditStats(w http.ResponseWriter, r *http.Request) {
	if s.auditLog == nil {
		writeJSON(w, map[string]any{"total": 0})
		return
	}
	writeJSON(w, s.auditLog.Stats())
}

// handleAuditEvents returns paginated audit events from the file.
func (s *Server) handleAuditEvents(w http.ResponseWriter, r *http.Request) {
	if s.auditLog == nil {
		writeJSON(w, map[string]any{"items": []any{}, "total": 0})
		return
	}

	q := r.URL.Query()
	page := parseInt(q.Get("page"), 1)
	limit := parseInt(q.Get("limit"), 50)
	if limit > 500 {
		limit = 500
	}

	events, total, err := s.auditLog.QueryFile(page, limit, q.Get("severity"), q.Get("from"), q.Get("to"))
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}

	writeJSON(w, map[string]any{
		"items": events,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// handleAuditEventDetail returns a single audit event by ID.
func (s *Server) handleAuditEventDetail(w http.ResponseWriter, r *http.Request) {
	if s.auditLog == nil {
		writeError(w, 404, "audit log not available")
		return
	}

	id := strings.TrimPrefix(r.URL.Path, "/api/audit/events/")
	if id == "" {
		writeError(w, 400, "missing event ID")
		return
	}

	ev, err := s.auditLog.GetByID(id)
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}
	if ev == nil {
		writeError(w, 404, "event not found")
		return
	}

	writeJSON(w, ev)
}
