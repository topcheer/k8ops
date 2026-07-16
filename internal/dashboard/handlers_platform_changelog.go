package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ChangeLogResult generates a platform changelog from recent resource changes.
type ChangeLogResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         ChangeLogSummary    `json:"summary"`
	RecentChanges   []ChangeEntry       `json:"recentChanges"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

type ChangeLogSummary struct {
	TotalChanges24h int `json:"totalChanges24h"`
	NewWorkloads    int `json:"newWorkloads"`
	UpdatedWorkloads int `json:"updatedWorkloads"`
	NewServices     int `json:"newServices"`
	NewConfigMaps   int `json:"newConfigMaps"`
	DeletedResources int `json:"deletedResources"`
}

type ChangeEntry struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Action    string `json:"action"`
	Age       string `json:"age"`
}

// handleChangeLog generates a platform changelog from recent resource changes.
// GET /api/docs/platform-changelog
func (s *Server) handleChangeLog(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := ChangeLogResult{ScannedAt: time.Now()}
	now := time.Now()
	twentyFourHoursAgo := now.AddDate(0, 0, -1)
	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true}

	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	configmaps, _ := rc.clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{})

	collectChanges := func(kind string, items interface{ Len() int }) {
		_ = items
	}
	_ = collectChanges

	for _, dep := range deployments.Items {
		if systemNS[dep.Namespace] { continue }
		created := dep.CreationTimestamp.Time
		ageStr := fmt.Sprintf("%dh", int(now.Sub(created).Hours()))

		if created.After(twentyFourHoursAgo) {
			result.Summary.TotalChanges24h++
			result.Summary.NewWorkloads++
			result.RecentChanges = append(result.RecentChanges, ChangeEntry{
				Kind: "Deployment", Name: dep.Name, Namespace: dep.Namespace,
				Action: "created", Age: ageStr,
			})
		} else {
			// Check for updates via conditions
			for _, cond := range dep.Status.Conditions {
				if cond.Type == "Progressing" && cond.LastUpdateTime.Time.After(twentyFourHoursAgo) {
					result.Summary.TotalChanges24h++
					result.Summary.UpdatedWorkloads++
					result.RecentChanges = append(result.RecentChanges, ChangeEntry{
						Kind: "Deployment", Name: dep.Name, Namespace: dep.Namespace,
						Action: "updated", Age: fmt.Sprintf("%dm", int(now.Sub(cond.LastUpdateTime.Time).Minutes())),
					})
					break
				}
			}
		}
	}

	for _, svc := range services.Items {
		if systemNS[svc.Namespace] { continue }
		if svc.CreationTimestamp.Time.After(twentyFourHoursAgo) {
			result.Summary.TotalChanges24h++
			result.Summary.NewServices++
			result.RecentChanges = append(result.RecentChanges, ChangeEntry{
				Kind: "Service", Name: svc.Name, Namespace: svc.Namespace,
				Action: "created", Age: fmt.Sprintf("%dh", int(now.Sub(svc.CreationTimestamp.Time).Hours())),
			})
		}
	}

	for _, cm := range configmaps.Items {
		if systemNS[cm.Namespace] { continue }
		if strings.Contains(cm.Name, "leader") || strings.Contains(cm.Name, "kube-root-ca") { continue }
		if cm.CreationTimestamp.Time.After(twentyFourHoursAgo) {
			result.Summary.TotalChanges24h++
			result.Summary.NewConfigMaps++
			result.RecentChanges = append(result.RecentChanges, ChangeEntry{
				Kind: "ConfigMap", Name: cm.Name, Namespace: cm.Namespace,
				Action: "created", Age: fmt.Sprintf("%dh", int(now.Sub(cm.CreationTimestamp.Time).Hours())),
			})
		}
	}

	// Score: more recent changes = higher activity score
	score := 50
	if result.Summary.TotalChanges24h > 0 { score += 20 }
	if result.Summary.NewWorkloads > 0 { score += 15 }
	if result.Summary.UpdatedWorkloads > 0 { score += 15 }
	result.HealthScore = min(100, score)
	result.Grade = goldenScoreToGrade(result.HealthScore)

	sort.Slice(result.RecentChanges, func(i, j int) bool {
		return result.RecentChanges[i].Age < result.RecentChanges[j].Age
	})

	var recs []string
	recs = append(recs, fmt.Sprintf("Platform activity: %d changes in 24h (%d new, %d updated)", result.Summary.TotalChanges24h, result.Summary.NewWorkloads, result.Summary.UpdatedWorkloads))
	if result.Summary.NewWorkloads > 5 { recs = append(recs, fmt.Sprintf("%d new workloads deployed — review for resource limits and probes", result.Summary.NewWorkloads)) }
	if len(recs) == 1 { recs = append(recs, "Platform change rate is stable") }
	result.Recommendations = recs

	writeJSON(w, result)
}
