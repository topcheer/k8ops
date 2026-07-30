package dashboard

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"time"

	k8saudit "github.com/ggai/k8ops/internal/tools/k8s"
)

// ============================================================
// Audit cache pre-warming and batch summary
//
// On server startup, a background goroutine pre-computes all audit
// endpoints and stores results in the response cache. This ensures
// the audit dashboard shows real data on first load instead of
// "Loading..." forever.
//
// The batch endpoint reads from the pre-warmed cache, returning
// all 1300+ results in a single HTTP response.
// ============================================================

type AuditSummaryResponse struct {
	Timestamp time.Time                    `json:"timestamp"`
	Cached    bool                         `json:"cached"`
	CacheAge  int                          `json:"cacheAgeSeconds"`
	Count     int                          `json:"count"`
	OK        int                          `json:"ok"`
	Pending   int                          `json:"pending"`
	Results   map[string]AuditSummaryEntry `json:"results"`
}

type AuditSummaryEntry struct {
	Score   int                    `json:"score"`
	Grade   string                 `json:"grade"`
	Summary map[string]interface{} `json:"summary,omitempty"`
	Status  string                 `json:"status"`
}

// batchAuditCache holds the pre-computed summary
var batchAuditCache struct {
	sync.RWMutex
	data      *AuditSummaryResponse
	refreshAt time.Time
}

// StartAuditWarmer launches a background goroutine that pre-warms
// the audit endpoint cache. Called once on server startup.
func (s *Server) StartAuditWarmer() {
	go func() {
		// Wait for server to be ready
		time.Sleep(10 * time.Second)
		s.log.Info("starting audit cache warmer", "endpoints", len(k8saudit.GetAuditEndpointPaths()))
		s.warmAuditCache()

		// Refresh every 2 minutes
		ticker := time.NewTicker(2 * time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			s.warmAuditCache()
		}
	}()
}

// warmAuditCache fetches all audit endpoints and populates the
// per-endpoint response cache so the batch endpoint can aggregate them.
func (s *Server) warmAuditCache() {
	paths := k8saudit.GetAuditEndpointPaths()
	const maxWorkers = 6
	jobs := make(chan string, len(paths))

	var wg sync.WaitGroup
	for i := 0; i < maxWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range jobs {
				cacheKey := "anon:" + path + "?"
				// Skip if already cached (fresh)
				if _, ok := s.cache.get(cacheKey); ok {
					continue
				}
				// Execute handler via internal HTTP call
				rec := httptest.NewRecorder()
				httpReq := &http.Request{
					Method: "GET",
					URL:    &url.URL{Path: path},
					Header: make(http.Header),
				}
				s.mux.ServeHTTP(rec, httpReq)
				body := rec.Body.Bytes()
				if len(body) > 0 && len(body) < 5*1024*1024 {
					s.cache.set(cacheKey, body)
				}
			}
		}()
	}

	for _, p := range paths {
		jobs <- p
	}
	close(jobs)
	wg.Wait()

	// Also rebuild the batch summary
	s.rebuildBatchSummary()

	s.log.Info("audit cache warmed", "endpoints", len(paths))
}

// rebuildBatchSummary builds the aggregated summary from cached results
func (s *Server) rebuildBatchSummary() {
	paths := k8saudit.GetAuditEndpointPaths()
	summary := &AuditSummaryResponse{
		Timestamp: time.Now(),
		Results:   make(map[string]AuditSummaryEntry, len(paths)),
	}

	for _, path := range paths {
		key := "anon:" + path + "?"
		data, ok := s.cache.get(key)
		if !ok {
			summary.Results[path] = AuditSummaryEntry{Status: "pending"}
			summary.Pending++
			continue
		}

		var result map[string]interface{}
		if err := json.Unmarshal(data, &result); err != nil {
			summary.Results[path] = AuditSummaryEntry{Status: "error"}
			continue
		}

		e := AuditSummaryEntry{Status: "ok"}
		if score, ok := result["healthScore"].(float64); ok {
			e.Score = int(score)
		}
		if grade, ok := result["grade"].(string); ok {
			e.Grade = grade
		}
		if sm, ok := result["summary"].(map[string]interface{}); ok {
			e.Summary = sm
		}
		summary.Results[path] = e
		summary.OK++
	}
	summary.Count = len(summary.Results)

	batchAuditCache.Lock()
	batchAuditCache.data = summary
	batchAuditCache.refreshAt = time.Now()
	batchAuditCache.Unlock()
}

// handleAuditSummary returns ALL audit results in a single HTTP response.
// Results come from the pre-warmed cache (refreshed every 2 min by background goroutine).
func (s *Server) handleAuditSummary(w http.ResponseWriter, r *http.Request) {
	batchAuditCache.RLock()
	cached := batchAuditCache.data
	cacheAge := time.Since(batchAuditCache.refreshAt)
	batchAuditCache.RUnlock()

	if cached != nil && cacheAge < 5*time.Minute {
		cached.Cached = true
		cached.CacheAge = int(cacheAge.Seconds())
		writeJSON(w, cached)
		return
	}

	// Fallback: build on-demand if cache is stale or missing
	s.rebuildBatchSummary()

	batchAuditCache.RLock()
	cached = batchAuditCache.data
	batchAuditCache.RUnlock()

	if cached != nil {
		cached.Cached = false
		cached.CacheAge = 0
		writeJSON(w, cached)
		return
	}

	// Last resort: empty response
	writeJSON(w, &AuditSummaryResponse{
		Timestamp: time.Now(),
		Results:   make(map[string]AuditSummaryEntry),
	})
}
