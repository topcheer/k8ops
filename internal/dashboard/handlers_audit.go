package dashboard

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/ggai/k8ops/internal/audit"
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

// handleAuditEvents returns paginated audit events with filtering.
// GET /api/audit/events?page=&limit=&severity=&actor=&action=&q=&from=&to=
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

	// Apply additional client-side filters (actor, action, text search)
	actor := strings.ToLower(q.Get("actor"))
	action := strings.ToLower(q.Get("action"))
	search := strings.ToLower(q.Get("q"))

	if actor != "" || action != "" || search != "" {
		var filtered []*audit.Event
		for _, ev := range events {
			b, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			s := strings.ToLower(string(b))
			if actor != "" && !strings.Contains(s, "\"actor\":\""+actor) {
				continue
			}
			if action != "" && !strings.Contains(s, "\"action\":\""+action) {
				continue
			}
			if search != "" && !strings.Contains(s, search) {
				continue
			}
			filtered = append(filtered, ev)
		}
		events = filtered
		total = len(events)
	}

	writeJSON(w, map[string]any{
		"items": events,
		"total": total,
		"page":  page,
		"limit": limit,
	})
}

// handleAuditExport exports audit events as CSV.
// GET /api/audit/export?severity=&from=&to=
func (s *Server) handleAuditExport(w http.ResponseWriter, r *http.Request) {
	if s.auditLog == nil {
		writeError(w, http.StatusServiceUnavailable, "audit log not available")
		return
	}

	q := r.URL.Query()
	events, _, err := s.auditLog.QueryFile(1, 10000, q.Get("severity"), q.Get("from"), q.Get("to"))
	if err != nil {
		writeError(w, 500, err.Error())
		return
	}

	w.Header().Set("Content-Type", "text/csv")
	w.Header().Set("Content-Disposition", `attachment; filename="k8ops-audit-export.csv"`)

	cw := csv.NewWriter(w)
	_ = cw.Write([]string{"ID", "Timestamp", "Severity", "Actor", "Action", "Target", "Success", "Detail"})

	for _, ev := range events {
		b, err := json.Marshal(ev)
		if err != nil {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal(b, &m); err != nil {
			continue
		}

		detailStr := ""
		if d, ok := m["detail"]; ok {
			if db, err := json.Marshal(d); err == nil {
				detailStr = string(db)
			}
		}

		_ = cw.Write([]string{
			getStringField(m, "id"),
			getStringField(m, "timestamp"),
			getStringField(m, "severity"),
			getStringField(m, "actor"),
			getStringField(m, "action"),
			getStringField(m, "target"),
			fmt.Sprintf("%v", m["success"]),
			detailStr,
		})
	}
	cw.Flush()
}

// getStringField safely extracts a string field from a map.
func getStringField(m map[string]any, key string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return ""
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
