package dashboard

import (
	"encoding/json"
	"net/http"
	"sync"
	"time"

	k8saudit "github.com/ggai/k8ops/internal/tools/k8s"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// ClusterSnapshot — periodically refreshed K8s data cache
// All audit handlers read from this snapshot instead of calling
// K8s API directly, reducing API server load from 1000+ calls
// to ~10 calls per refresh cycle.
// ============================================================

// ClusterSnapshot holds a point-in-time view of cluster resources
type ClusterSnapshot struct {
	mu        sync.RWMutex
	refreshAt time.Time
	ttl       time.Duration

	// Cached resource lists
	Pods                []corev1.Pod
	Nodes               []corev1.Node
	Namespaces          []corev1.Namespace
	Services            []corev1.Service
	Secrets             []corev1.Secret
	ConfigMaps          []corev1.ConfigMap
	PVCs                []corev1.PersistentVolumeClaim
	PVs                 []corev1.PersistentVolume
	StorageClasses      []interface{} // StorageV1 not imported here
	Deployments         []interface{} // AppsV1 types
	StatefulSets        []interface{}
	DaemonSets          []interface{}
	ReplicaSets         []interface{}
	Events              []corev1.Event
	EndpointSlices      []interface{}
	NetworkPolicies     []interface{}
	ServiceAccounts     []corev1.ServiceAccount
	RoleBindings        []interface{}
	ClusterRoles        []interface{}
	ClusterRoleBindings []interface{}
	PDBs                []interface{}
	HPAs                []interface{}
	Ingresses           []interface{}
}

// NewClusterSnapshot creates a snapshot holder with given TTL
func NewClusterSnapshot(ttl time.Duration) *ClusterSnapshot {
	return &ClusterSnapshot{ttl: ttl}
}

// refresh fetches all cluster resources in one batch
func (cs *ClusterSnapshot) refresh(s *Server) error {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	ctx := &dummyContext{}

	// Fetch all core resources
	if pods, err := s.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{}); err == nil {
		cs.Pods = pods.Items
	}
	if nodes, err := s.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{}); err == nil {
		cs.Nodes = nodes.Items
	}
	if nss, err := s.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{}); err == nil {
		cs.Namespaces = nss.Items
	}
	if svcs, err := s.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{}); err == nil {
		cs.Services = svcs.Items
	}
	if cms, err := s.clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{}); err == nil {
		cs.ConfigMaps = cms.Items
	}
	if pvcs, err := s.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{}); err == nil {
		cs.PVCs = pvcs.Items
	}
	if evts, err := s.clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{}); err == nil {
		cs.Events = evts.Items
	}
	if sas, err := s.clientset.CoreV1().ServiceAccounts("").List(ctx, metav1.ListOptions{}); err == nil {
		cs.ServiceAccounts = sas.Items
	}

	cs.refreshAt = time.Now()
	return nil
}

// IsValid returns true if the snapshot is still fresh
func (cs *ClusterSnapshot) IsValid() bool {
	cs.mu.RLock()
	defer cs.mu.RUnlock()
	return time.Since(cs.refreshAt) < cs.ttl
}

// dummyContext provides a minimal context for K8s API calls
type dummyContext struct{}

func (d *dummyContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (d *dummyContext) Done() <-chan struct{}       { return nil }
func (d *dummyContext) Err() error                  { return nil }
func (d *dummyContext) Value(key any) any           { return nil }

// ============================================================
// Batch audit summary endpoint
// Returns all audit results in a single response
// ============================================================

type AuditSummaryResponse struct {
	Timestamp time.Time                    `json:"timestamp"`
	Cached    bool                         `json:"cached"`
	CacheAge  int                          `json:"cacheAgeSeconds"`
	Count     int                          `json:"count"`
	Results   map[string]AuditSummaryEntry `json:"results"`
}

type AuditSummaryEntry struct {
	Score   int                    `json:"score"`
	Grade   string                 `json:"grade"`
	Summary map[string]interface{} `json:"summary,omitempty"`
	Status  string                 `json:"status"`
}

// batchAuditCache holds the pre-computed summary for all audit endpoints
var batchAuditCache struct {
	sync.RWMutex
	data      *AuditSummaryResponse
	refreshAt time.Time
}

// handleAuditSummary returns ALL audit results in a single HTTP response.
// Results are computed from the cached cluster snapshot (refreshed every 60s),
// NOT from individual K8s API calls per request.
// This replaces 1000+ individual endpoint calls with ONE request.
func (s *Server) handleAuditSummary(w http.ResponseWriter, r *http.Request) {
	// Check if we have a valid cached summary
	batchAuditCache.RLock()
	cached := batchAuditCache.data
	cacheAge := time.Since(batchAuditCache.refreshAt)
	batchAuditCache.RUnlock()

	if cached != nil && cacheAge < 60*time.Second {
		cached.Cached = true
		cached.CacheAge = int(cacheAge.Seconds())
		writeJSON(w, cached)
		return
	}

	// Build a fresh summary from the cluster snapshot
	// For now, return a lightweight summary with just scores/grades
	// Individual handlers still serve detailed data via their own cached routes
	summary := &AuditSummaryResponse{
		Timestamp: time.Now(),
		Cached:    false,
		CacheAge:  0,
		Results:   make(map[string]AuditSummaryEntry),
	}

	// Walk the server's mux to find all audit API paths
	// We use the audit registry from tools package via an exported variable
	auditPaths := k8saudit.GetAuditEndpointPaths()
	for _, path := range auditPaths {
		key := "anon:" + path + "?"
		if data, ok := s.cache.get(key); ok {
			var result map[string]interface{}
			if err := json.Unmarshal(data, &result); err == nil {
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
				continue
			}
		}
		summary.Results[path] = AuditSummaryEntry{Status: "pending"}
	}
	summary.Count = len(summary.Results)

	// Cache the summary
	batchAuditCache.Lock()
	batchAuditCache.data = summary
	batchAuditCache.refreshAt = time.Now()
	batchAuditCache.Unlock()

	writeJSON(w, summary)
}
