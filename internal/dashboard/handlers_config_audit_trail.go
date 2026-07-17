package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ConfigAuditTrailResult tracks configuration changes across deployments
// by comparing ReplicaSet annotations and revision history. It identifies
// who changed what, when, and helps build a complete audit trail.
type ConfigAuditTrailResult struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	Summary         ConfigAuditTrailSummary `json:"summary"`
	RecentChanges   []ConfigChangeEntry     `json:"recentChanges"`
	ByNamespace     []ConfigAuditTrailNS    `json:"byNamespace"`
	ChangeFrequency []ConfigChangeFreqStat  `json:"changeFrequency"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Recommendations []string                `json:"recommendations"`
}

type ConfigAuditTrailSummary struct {
	TotalWorkloads   int `json:"totalWorkloads"`
	ChangedWorkloads int `json:"changedWorkloads"`
	TotalChanges     int `json:"totalChanges"`
	RecentChanges    int `json:"recentChanges24h"`
	StaleWorkloads   int `json:"staleWorkloads"`
}

type ConfigChangeEntry struct {
	Workload   string `json:"workload"`
	Namespace  string `json:"namespace"`
	Revision   string `json:"revision"`
	ChangedAt  string `json:"changedAt"`
	Age        string `json:"age"`
	ChangeType string `json:"changeType"`
}

type ConfigAuditTrailNS struct {
	Namespace string `json:"namespace"`
	Changes   int    `json:"changes"`
	Workloads int    `json:"workloads"`
}

type ConfigChangeFreqStat struct {
	Period string `json:"period"`
	Count  int    `json:"count"`
}

// handleConfigAuditTrail handles GET /api/security/config-audit-trail
func (s *Server) handleConfigAuditTrail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ConfigAuditTrailResult{ScannedAt: time.Now()}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	replicaSets, _ := rc.clientset.AppsV1().ReplicaSets("").List(ctx, metav1.ListOptions{})

	now := time.Now()
	rsByOwner := make(map[string][]string) // ns/name -> []revision strings
	rsTimeByOwner := make(map[string][]time.Time)
	for _, rs := range replicaSets.Items {
		if isSystemNamespace(rs.Namespace) {
			continue
		}
		for _, ref := range rs.OwnerReferences {
			if ref.Kind == "Deployment" {
				key := rs.Namespace + "/" + ref.Name
				rev := rs.Annotations["deployment.kubernetes.io/revision"]
				if rev == "" {
					rev = "unknown"
				}
				rsByOwner[key] = append(rsByOwner[key], rev)
				rsTimeByOwner[key] = append(rsTimeByOwner[key], rs.CreationTimestamp.Time)
			}
		}
	}

	nsMap := make(map[string]*ConfigAuditTrailNS)
	var changes []ConfigChangeEntry
	freqMap := map[string]int{"24h": 0, "7d": 0, "30d": 0, "90d": 0}

	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++
		key := d.Namespace + "/" + d.Name

		if _, ok := nsMap[d.Namespace]; !ok {
			nsMap[d.Namespace] = &ConfigAuditTrailNS{Namespace: d.Namespace}
		}
		nsMap[d.Namespace].Workloads++

		revs := rsByOwner[key]
		times := rsTimeByOwner[key]
		changeCount := len(revs)
		if changeCount > 1 {
			result.Summary.ChangedWorkloads++
			result.Summary.TotalChanges += changeCount
			nsMap[d.Namespace].Changes += changeCount
		}

		// Find most recent change
		var lastChange time.Time
		for _, t := range times {
			if t.After(lastChange) {
				lastChange = t
			}
		}

		if !lastChange.IsZero() {
			age := now.Sub(lastChange)
			entry := ConfigChangeEntry{
				Workload: d.Name, Namespace: d.Namespace,
				Revision:  fmt.Sprintf("%d", changeCount),
				ChangedAt: lastChange.Format("2006-01-02 15:04"),
				Age:       fmt.Sprintf("%.0fd", age.Hours()/24),
			}
			if age < 24*time.Hour {
				entry.ChangeType = "recent"
				result.Summary.RecentChanges++
				freqMap["24h"]++
			} else if age < 7*24*time.Hour {
				entry.ChangeType = "week"
				freqMap["7d"]++
			} else if age < 30*24*time.Hour {
				entry.ChangeType = "month"
				freqMap["30d"]++
			} else {
				entry.ChangeType = "stale"
				result.Summary.StaleWorkloads++
				freqMap["90d"]++
			}
			changes = append(changes, entry)
		}
	}

	sort.Slice(changes, func(i, j int) bool {
		return changes[i].Age < changes[j].Age
	})
	result.RecentChanges = changes

	for _, ns := range nsMap {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].Changes > result.ByNamespace[j].Changes
	})

	for _, p := range []string{"24h", "7d", "30d", "90d"} {
		result.ChangeFrequency = append(result.ChangeFrequency, ConfigChangeFreqStat{Period: p, Count: freqMap[p]})
	}

	// Score: active changes = healthy, stale = poor
	if result.Summary.TotalWorkloads > 0 {
		active := result.Summary.TotalWorkloads - result.Summary.StaleWorkloads
		result.HealthScore = active * 100 / result.Summary.TotalWorkloads
	} else {
		result.HealthScore = 100
	}

	switch {
	case result.HealthScore >= 80:
		result.Grade = "A"
	case result.HealthScore >= 60:
		result.Grade = "B"
	case result.HealthScore >= 40:
		result.Grade = "C"
	default:
		result.Grade = "D"
	}

	result.Recommendations = buildConfigAuditRecs(&result)
	writeJSON(w, result)
}

func buildConfigAuditRecs(r *ConfigAuditTrailResult) []string {
	recs := []string{
		fmt.Sprintf("%d 个工作负载, %d 次配置变更, %d 个近期变更", r.Summary.TotalWorkloads, r.Summary.TotalChanges, r.Summary.RecentChanges),
	}
	if r.Summary.StaleWorkloads > 0 {
		recs = append(recs, fmt.Sprintf("%d 个工作负载超过 90 天未更新", r.Summary.StaleWorkloads))
	}
	if r.Summary.RecentChanges > 10 {
		recs = append(recs, fmt.Sprintf("24h 内 %d 次变更，注意变更窗口管理", r.Summary.RecentChanges))
	}
	return recs
}
