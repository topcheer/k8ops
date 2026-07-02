// Package audit provides structured audit logging for all AI-initiated actions.
// Every tool execution, LLM call, and remediation action is recorded as an audit event
// for compliance, debugging, and post-incident review.
package audit

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// EventType categorizes audit events.
type EventType string

const (
	EventTypeToolCall    EventType = "tool_call"
	EventTypeLLMCall     EventType = "llm_call"
	EventTypeRemediation EventType = "remediation"
	EventTypeSafetyBlock EventType = "safety_block"
	EventTypeDiagnostic  EventType = "diagnostic"
	EventTypeRollback    EventType = "rollback"
	EventTypeUserAction  EventType = "user_action" // user-initiated operations via dashboard
)

// Severity levels for audit events.
type Severity string

const (
	SeverityInfo     Severity = "info"
	SeverityWarning  Severity = "warning"
	SeverityCritical Severity = "critical"
)

// Event represents a single audit event.
type Event struct {
	ID        string         `json:"id"`
	Timestamp string         `json:"timestamp"`
	Type      EventType      `json:"type"`
	Severity  Severity       `json:"severity"`
	Actor     string         `json:"actor"`  // "ai-agent" or "controller"
	Action    string         `json:"action"` // tool name or operation type
	Target    string         `json:"target"` // resource being acted on
	Namespace string         `json:"namespace,omitempty"`
	Success   bool           `json:"success"`
	Detail    map[string]any `json:"detail,omitempty"`
	Duration  string         `json:"duration,omitempty"`
	Error     string         `json:"error,omitempty"`
	Source    string         `json:"source,omitempty"` // which controller/trigger
}

// Logger is the audit logger. It writes structured events to both a JSON file
// and the standard logger, and keeps an in-memory ring buffer for the dashboard.
type Logger struct {
	mu         sync.Mutex
	log        *slog.Logger
	file       *os.File
	filePath   string
	encoder    *json.Encoder
	ring       []*Event
	ringSize   int
	rptr       int
	maxSize    int64 // max file size in bytes before rotation
	maxAgeDays int   // max age in days for rotated files
}

// NewLogger creates a new audit logger.
// If logPath is empty, file logging is skipped (memory + slog only).
func NewLogger(logPath string, log *slog.Logger) (*Logger, error) {
	l := &Logger{
		log:        log,
		filePath:   logPath,
		ringSize:   500,
		ring:       make([]*Event, 500),
		maxSize:    100 * 1024 * 1024, // 100MB default
		maxAgeDays: 30,                // 30 days default
	}

	if logPath != "" {
		if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
			return nil, fmt.Errorf("failed to create audit log dir: %w", err)
		}
		if err := l.openFile(); err != nil {
			return nil, err
		}
	}

	return l, nil
}

// openFile opens (or reopens) the audit log file for appending.
func (l *Logger) openFile() error {
	f, err := os.OpenFile(l.filePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return fmt.Errorf("failed to open audit log file: %w", err)
	}
	l.file = f
	l.encoder = json.NewEncoder(f)
	return nil
}

// SetMaxSize sets the maximum file size in bytes before rotation.
func (l *Logger) SetMaxSize(bytes int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.maxSize = bytes
}

// SetMaxAge sets the maximum age in days for rotated log files.
func (l *Logger) SetMaxAge(days int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.maxAgeDays = days
}

// Rotate performs manual log rotation: closes current file, renames it
// with a timestamp suffix, and opens a fresh file.
func (l *Logger) Rotate() error {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		return nil
	}

	// Close current file
	oldPath := l.filePath
	_ = l.file.Close()
	l.file = nil
	l.encoder = nil

	// Rename to rotated file
	rotatedPath := oldPath + "." + time.Now().UTC().Format("20060102-150405")
	if err := os.Rename(oldPath, rotatedPath); err != nil {
		// Reopen original even if rename failed
		_ = l.openFile()
		return fmt.Errorf("failed to rotate audit log: %w", err)
	}

	// Open fresh file
	if err := l.openFile(); err != nil {
		return err
	}

	l.log.Info("audit log rotated", "old", rotatedPath, "new", l.filePath)
	return nil
}

// Cleanup removes rotated log files older than maxAgeDays.
func (l *Logger) Cleanup() (int, error) {
	l.mu.Lock()
	maxAge := l.maxAgeDays
	dir := filepath.Dir(l.filePath)
	base := filepath.Base(l.filePath)
	l.mu.Unlock()

	if dir == "" || base == "" {
		return 0, nil
	}

	cutoff := time.Now().AddDate(0, 0, -maxAge)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("failed to read audit log dir: %w", err)
	}

	removed := 0
	for _, entry := range entries {
		name := entry.Name()
		// Only match rotated files: base.timestamp
		if len(name) <= len(base) || name[:len(base)] != base {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(filepath.Join(dir, name)); err == nil {
				removed++
			}
		}
	}

	if removed > 0 {
		l.log.Info("audit log cleanup", "removed", removed, "older_than_days", maxAge)
	}
	return removed, nil
}

// FileSize returns the current audit log file size in bytes.
func (l *Logger) FileSize() (int64, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.filePath == "" {
		return 0, nil
	}
	info, err := os.Stat(l.filePath)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// maybeRotate checks if the file exceeds maxSize and rotates if needed.
// Must be called with mu held.
func (l *Logger) maybeRotate() {
	if l.file == nil || l.maxSize <= 0 {
		return
	}

	info, err := l.file.Stat()
	if err != nil {
		return
	}

	if info.Size() >= l.maxSize {
		oldPath := l.filePath
		_ = l.file.Close()
		l.file = nil
		l.encoder = nil

		rotatedPath := oldPath + "." + time.Now().UTC().Format("20060102-150405")
		if err := os.Rename(oldPath, rotatedPath); err == nil {
			_ = l.openFile()
		} else {
			_ = l.openFile()
		}
	}
}

// Close flushes and closes the audit log file.
func (l *Logger) Close() error {
	if l.file != nil {
		return l.file.Close()
	}
	return nil
}

// Log records an audit event.
func (l *Logger) Log(ctx context.Context, event Event) {
	if event.ID == "" {
		event.ID = fmt.Sprintf("%d", time.Now().UnixNano())
	}
	if event.Timestamp == "" {
		event.Timestamp = time.Now().UTC().Format(time.RFC3339Nano)
	}

	// Write to file
	l.mu.Lock()
	l.maybeRotate()
	if l.encoder != nil {
		_ = l.encoder.Encode(event)
	}

	// Write to ring buffer
	l.ring[l.rptr] = &event
	l.rptr = (l.rptr + 1) % l.ringSize
	l.mu.Unlock()

	// Write to structured logger
	attrs := []any{
		"audit_id", event.ID,
		"type", string(event.Type),
		"severity", string(event.Severity),
		"actor", event.Actor,
		"action", event.Action,
		"target", event.Target,
		"success", event.Success,
	}
	if event.Namespace != "" {
		attrs = append(attrs, "namespace", event.Namespace)
	}
	if event.Error != "" {
		attrs = append(attrs, "error", event.Error)
	}

	switch event.Severity {
	case SeverityCritical:
		l.log.Error("AUDIT", attrs...)
	case SeverityWarning:
		l.log.Warn("AUDIT", attrs...)
	default:
		l.log.Info("AUDIT", attrs...)
	}
}

// Recent returns up to n most recent audit events (newest first).
func (l *Logger) Recent(n int) []*Event {
	l.mu.Lock()
	defer l.mu.Unlock()

	var events []*Event
	// Read ring buffer backwards from rptr-1
	for i := 0; i < l.ringSize && i < n; i++ {
		idx := (l.rptr - 1 - i + l.ringSize) % l.ringSize
		if l.ring[idx] != nil {
			events = append(events, l.ring[idx])
		}
	}
	return events
}

// Filter returns events matching the given criteria.
func (l *Logger) Filter(eventType EventType, namespace string, limit int) []*Event {
	all := l.Recent(l.ringSize)
	var filtered []*Event
	for _, e := range all {
		if eventType != "" && e.Type != eventType {
			continue
		}
		if namespace != "" && e.Namespace != namespace {
			continue
		}
		filtered = append(filtered, e)
		if len(filtered) >= limit {
			break
		}
	}
	return filtered
}

// Stats returns summary statistics of recent audit events.
func (l *Logger) Stats() map[string]any {
	all := l.Recent(l.ringSize)

	stats := map[string]any{
		"total":        len(all),
		"byType":       map[string]int{},
		"bySeverity":   map[string]int{},
		"successCount": 0,
		"failureCount": 0,
	}

	for _, e := range all {
		stats["byType"].(map[string]int)[string(e.Type)]++
		stats["bySeverity"].(map[string]int)[string(e.Severity)]++
		if e.Success {
			stats["successCount"] = stats["successCount"].(int) + 1
		} else {
			stats["failureCount"] = stats["failureCount"].(int) + 1
		}
	}

	return stats
}

// QueryFile reads audit events from the JSON lines file with optional filters.
// Parameters support pagination (page/limit), time range (from/to as RFC3339),
// and severity filter. Returns events newest first.
func (l *Logger) QueryFile(page, limit int, severity, from, to string) ([]*Event, int, error) {
	if l.filePath == "" {
		// Fall back to in-memory ring buffer
		all := l.Recent(limit * page)
		return paginate(all, page, limit, severity, from, to)
	}

	f, err := os.Open(l.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return []*Event{}, 0, nil
		}
		return nil, 0, fmt.Errorf("failed to open audit file: %w", err)
	}
	defer f.Close()

	var all []*Event
	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			continue // skip malformed lines
		}
		all = append(all, &ev)
	}
	if err := scanner.Err(); err != nil {
		return nil, 0, fmt.Errorf("error reading audit file: %w", err)
	}

	return paginate(all, page, limit, severity, from, to)
}

// GetByID returns a single audit event by its ID from the file.
func (l *Logger) GetByID(id string) (*Event, error) {
	if l.filePath == "" {
		// Search ring buffer
		all := l.Recent(l.ringSize)
		for _, e := range all {
			if e.ID == id {
				return e, nil
			}
		}
		return nil, nil
	}

	f, err := os.Open(l.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to open audit file: %w", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	buf := make([]byte, 0, 64*1024)
	scanner.Buffer(buf, 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev Event
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		if ev.ID == id {
			return &ev, nil
		}
	}
	return nil, scanner.Err()
}

// paginate filters and paginates an event slice.
func paginate(events []*Event, page, limit int, severity, from, to string) ([]*Event, int, error) {
	var filtered []*Event
	for _, e := range events {
		if severity != "" && string(e.Severity) != severity {
			continue
		}
		if from != "" {
			fromTime, err := time.Parse(time.RFC3339, from)
			if err == nil {
				et, err := time.Parse(time.RFC3339Nano, e.Timestamp)
				if err == nil && et.Before(fromTime) {
					continue
				}
			}
		}
		if to != "" {
			toTime, err := time.Parse(time.RFC3339, to)
			if err == nil {
				et, err := time.Parse(time.RFC3339Nano, e.Timestamp)
				if err == nil && et.After(toTime) {
					continue
				}
			}
		}
		filtered = append(filtered, e)
	}

	// Sort newest first
	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Timestamp > filtered[j].Timestamp
	})

	total := len(filtered)
	if page < 1 {
		page = 1
	}
	if limit < 1 {
		limit = 50
	}
	start := (page - 1) * limit
	if start >= total {
		return []*Event{}, total, nil
	}
	end := start + limit
	if end > total {
		end = total
	}
	return filtered[start:end], total, nil
}

// NoopLogger returns a logger that only writes to slog (no file).
func NoopLogger(log *slog.Logger) *Logger {
	l, _ := NewLogger("", log)
	return l
}
