package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// STSResult is the StatefulSet health & ordered rollout analysis.
type STSResult struct {
	ScannedAt       time.Time  `json:"scannedAt"`
	Summary         STSSummary `json:"summary"`
	ByWorkload      []STSEntry `json:"byWorkload"`
	StuckRollouts   []STSEntry `json:"stuckRollouts"`   // replicas != readyReplicas
	NoHeadlessSvc   []STSEntry `json:"noHeadlessSvc"`   // missing headless service
	BadPVCRetention []STSEntry `json:"badPVCRetention"` // Delete policy = data loss
	NoParallel      []STSEntry `json:"noParallel"`      // OrderedReady = slow scale
	Issues          []STSIssue `json:"issues"`
	Recommendations []string   `json:"recommendations"`
}

// STSSummary aggregates StatefulSet statistics.
type STSSummary struct {
	TotalSTS      int `json:"totalStatefulSets"`
	Healthy       int `json:"healthy"`
	StuckRollout  int `json:"stuckRollout"` // replicas != readyReplicas
	NoHeadlessSvc int `json:"noHeadlessSvc"`
	PVCRetain     int `json:"pvcRetain"`
	PVCDelete     int `json:"pvcDelete"`    // data loss risk
	OrderedReady  int `json:"orderedReady"` // slow scaling
	Parallel      int `json:"parallel"`
	HasPartition  int `json:"hasPartition"` // partition > 0 = canary paused
	NoPVC         int `json:"noPVC"`        // stateless STS (should be Deployment)
	HealthScore   int `json:"healthScore"`  // 0-100
}

// STSEntry describes one StatefulSet's health.
type STSEntry struct {
	Name                string   `json:"name"`
	Namespace           string   `json:"namespace"`
	Replicas            int32    `json:"replicas"`
	ReadyReplicas       int32    `json:"readyReplicas"`
	UpdatedReplicas     int32    `json:"updatedReplicas"`
	CurrentRevision     string   `json:"currentRevision,omitempty"`
	UpdateRevision      string   `json:"updateRevision,omitempty"`
	PodManagementPolicy string   `json:"podManagementPolicy"`
	PVCRetentionPolicy  string   `json:"pvcRetentionPolicy"`
	HasHeadlessSvc      bool     `json:"hasHeadlessSvc"`
	HasVolumeClaims     bool     `json:"hasVolumeClaims"`
	Partition           *int32   `json:"partition,omitempty"`
	Violations          []string `json:"violations,omitempty"`
	RiskLevel           string   `json:"riskLevel"`
}

// STSIssue is a detected StatefulSet problem.
type STSIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleStatefulSetAudit audits StatefulSet health and rollout status.
// GET /api/product/statefulset-audit
func (s *Server) handleStatefulSetAudit(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")
	stsList, err := rc.clientset.AppsV1().StatefulSets(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	svcs, err := rc.clientset.CoreV1().Services(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Build headless service lookup: ns/name → true
	headlessSvcs := make(map[string]bool)
	for _, svc := range svcs.Items {
		if svc.Spec.ClusterIP == "None" {
			headlessSvcs[svc.Namespace+"/"+svc.Name] = true
		}
	}

	result := STSResult{ScannedAt: time.Now()}

	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++

		entry := STSEntry{
			Name:      sts.Name,
			Namespace: sts.Namespace,
		}

		replicas := int32(1)
		if sts.Spec.Replicas != nil {
			replicas = *sts.Spec.Replicas
		}
		entry.Replicas = replicas
		entry.ReadyReplicas = sts.Status.ReadyReplicas
		entry.UpdatedReplicas = sts.Status.UpdatedReplicas
		entry.CurrentRevision = sts.Status.CurrentRevision
		entry.UpdateRevision = sts.Status.UpdateRevision

		// Pod management policy
		entry.PodManagementPolicy = string(sts.Spec.PodManagementPolicy)
		if sts.Spec.PodManagementPolicy == appsv1.OrderedReadyPodManagement {
			result.Summary.OrderedReady++
			if replicas > 3 {
				entry.Violations = append(entry.Violations, "OrderedReady pod management — slow scaling for large replica counts, consider Parallel")
				result.NoParallel = append(result.NoParallel, entry)
			}
		} else {
			result.Summary.Parallel++
		}

		// PVC retention policy
		entry.PVCRetentionPolicy = "Retain" // default
		if sts.Spec.PersistentVolumeClaimRetentionPolicy != nil {
			policy := sts.Spec.PersistentVolumeClaimRetentionPolicy
			if policy.WhenDeleted == appsv1.DeletePersistentVolumeClaimRetentionPolicyType {
				entry.PVCRetentionPolicy = "Delete"
				result.Summary.PVCDelete++
				if replicas > 1 {
					entry.Violations = append(entry.Violations, "PVC retention WhenDeleted=Delete — deleting StatefulSet will destroy all PVC data")
					result.BadPVCRetention = append(result.BadPVCRetention, entry)
					result.Issues = append(result.Issues, STSIssue{
						Severity: "warning", Type: "pvc-delete-policy",
						Resource: fmt.Sprintf("%s/%s", sts.Namespace, sts.Name),
						Message:  fmt.Sprintf("StatefulSet %s/%s has PVC retention WhenDeleted=Delete — deleting the STS will destroy all PVC data", sts.Namespace, sts.Name),
					})
				}
			} else {
				result.Summary.PVCRetain++
			}
		} else {
			result.Summary.PVCRetain++
		}

		// Volume claim templates
		entry.HasVolumeClaims = len(sts.Spec.VolumeClaimTemplates) > 0
		if !entry.HasVolumeClaims {
			result.Summary.NoPVC++
			entry.Violations = append(entry.Violations, "No volumeClaimTemplates — consider using Deployment instead of StatefulSet")
		}

		// Headless service check
		entry.HasHeadlessSvc = headlessSvcs[sts.Namespace+"/"+sts.Spec.ServiceName]
		if !entry.HasHeadlessSvc {
			result.Summary.NoHeadlessSvc++
			entry.Violations = append(entry.Violations, fmt.Sprintf("No headless service '%s' — pod DNS resolution will fail", sts.Spec.ServiceName))
			result.NoHeadlessSvc = append(result.NoHeadlessSvc, entry)
			result.Issues = append(result.Issues, STSIssue{
				Severity: "critical", Type: "no-headless-service",
				Resource: fmt.Sprintf("%s/%s", sts.Namespace, sts.Name),
				Message:  fmt.Sprintf("StatefulSet %s/%s references headless service '%s' which doesn't exist or isn't headless", sts.Namespace, sts.Name, sts.Spec.ServiceName),
			})
		}

		// Partition (canary update)
		if sts.Spec.UpdateStrategy.RollingUpdate != nil &&
			sts.Spec.UpdateStrategy.RollingUpdate.Partition != nil {
			partition := *sts.Spec.UpdateStrategy.RollingUpdate.Partition
			if partition > 0 {
				entry.Partition = &partition
				result.Summary.HasPartition++
				entry.Violations = append(entry.Violations, fmt.Sprintf("RollingUpdate partition=%d — canary update paused, %d pods not updated", partition, partition))
				result.Issues = append(result.Issues, STSIssue{
					Severity: "warning", Type: "partition-paused",
					Resource: fmt.Sprintf("%s/%s", sts.Namespace, sts.Name),
					Message:  fmt.Sprintf("StatefulSet %s/%s update partition=%d — %d pods still running old revision", sts.Namespace, sts.Name, partition, partition),
				})
			}
		}

		// Stuck rollout (replicas != readyReplicas or updated != replicas)
		if entry.ReadyReplicas < replicas || entry.UpdatedReplicas < replicas {
			result.Summary.StuckRollout++
			entry.Violations = append(entry.Violations, fmt.Sprintf("Rollout incomplete: %d/%d ready, %d/%d updated", entry.ReadyReplicas, replicas, entry.UpdatedReplicas, replicas))
			result.StuckRollouts = append(result.StuckRollouts, entry)
			result.Issues = append(result.Issues, STSIssue{
				Severity: "warning", Type: "stuck-rollout",
				Resource: fmt.Sprintf("%s/%s", sts.Namespace, sts.Name),
				Message:  fmt.Sprintf("StatefulSet %s/%s rollout stuck: %d/%d ready, %d/%d updated — check pod health and PVC binding", sts.Namespace, sts.Name, entry.ReadyReplicas, replicas, entry.UpdatedReplicas, replicas),
			})
		} else if replicas > 0 {
			result.Summary.Healthy++
		}

		entry.RiskLevel = stsAssessRisk(entry)
		result.ByWorkload = append(result.ByWorkload, entry)
	}

	// Sort
	sort.Slice(result.ByWorkload, func(i, j int) bool {
		return stsRiskRank(result.ByWorkload[i].RiskLevel) < stsRiskRank(result.ByWorkload[j].RiskLevel)
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return stsIssueRank(result.Issues[i].Severity) < stsIssueRank(result.Issues[j].Severity)
	})

	result.Summary.HealthScore = stsScore(result.Summary)
	result.Recommendations = stsGenRecs(result.Summary, result.StuckRollouts, result.BadPVCRetention, result.NoHeadlessSvc)

	writeJSON(w, result)
}

// stsAssessRisk determines risk level.
func stsAssessRisk(entry STSEntry) string {
	if !entry.HasHeadlessSvc {
		return "critical"
	}
	if entry.ReadyReplicas < entry.Replicas {
		return "high"
	}
	if entry.PVCRetentionPolicy == "Delete" && entry.Replicas > 1 {
		return "high"
	}
	if len(entry.Violations) > 0 {
		return "medium"
	}
	return "low"
}

// stsScore computes 0-100.
func stsScore(s STSSummary) int {
	if s.TotalSTS == 0 {
		return 100
	}
	score := 100
	score -= s.NoHeadlessSvc * 15
	score -= s.StuckRollout * 8
	score -= s.PVCDelete * 5
	score -= s.HasPartition * 4
	score -= s.NoPVC * 3
	if score < 0 {
		score = 0
	}
	return score
}

// stsGenRecs produces actionable advice.
func stsGenRecs(s STSSummary, stuck []STSEntry, badPVC []STSEntry, noHeadless []STSEntry) []string {
	var recs []string

	if s.NoHeadlessSvc > 0 {
		recs = append(recs, fmt.Sprintf("%d StatefulSet(s) missing headless service — pod DNS (.pod.ns.svc.cluster.local) will fail", s.NoHeadlessSvc))
	}
	if s.StuckRollout > 0 {
		top := ""
		if len(stuck) > 0 {
			top = fmt.Sprintf(" (e.g. %s/%s: %d/%d ready)", stuck[0].Namespace, stuck[0].Name, stuck[0].ReadyReplicas, stuck[0].Replicas)
		}
		recs = append(recs, fmt.Sprintf("%d StatefulSet(s) have stuck rollouts%s — check pod health and PVC binding", s.StuckRollout, top))
	}
	if s.PVCDelete > 0 {
		recs = append(recs, fmt.Sprintf("%d StatefulSet(s) use PVC Delete retention — deleting STS destroys data, switch to Retain", s.PVCDelete))
	}
	if s.HasPartition > 0 {
		recs = append(recs, fmt.Sprintf("%d StatefulSet(s) have paused canary updates (partition > 0) — complete or rollback the update", s.HasPartition))
	}
	if s.NoPVC > 0 {
		recs = append(recs, fmt.Sprintf("%d StatefulSet(s) have no volumeClaimTemplates — consider using Deployment instead", s.NoPVC))
	}
	if s.OrderedReady > 0 {
		recs = append(recs, fmt.Sprintf("%d StatefulSet(s) use OrderedReady pod management — consider Parallel for faster scaling", s.OrderedReady))
	}
	if s.HealthScore < 70 {
		recs = append(recs, fmt.Sprintf("StatefulSet health score is %d/100 — multiple StatefulSet issues detected", s.HealthScore))
	}
	if s.StuckRollout == 0 && s.NoHeadlessSvc == 0 && s.PVCDelete == 0 {
		recs = append(recs, "All StatefulSets are healthy — good stateful workload posture")
	}

	return recs
}

func stsRiskRank(level string) int {
	switch level {
	case "critical":
		return 0
	case "high":
		return 1
	case "medium":
		return 2
	default:
		return 3
	}
}

func stsIssueRank(s string) int {
	switch s {
	case "critical":
		return 0
	case "warning":
		return 1
	case "info":
		return 2
	default:
		return 3
	}
}

var _ = corev1.ServiceSpec{}
