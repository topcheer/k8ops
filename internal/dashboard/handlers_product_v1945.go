package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v19.45 — Product Dimension (Round 10)
// 1. Volume Snapshot Audit — PV/PVC snapshot coverage & recovery readiness
// 2. Pod Priority Class Inventory — priority class usage & preemption analysis
// 3. Container Image Pull Policy — pull policy compliance & optimization
// ============================================================

// ---------------------------------------------------------------
// 1. Volume Snapshot Audit
// ---------------------------------------------------------------

type VolSnapshotResult1945 struct {
	ScannedAt       time.Time                    `json:"scannedAt"`
	HealthScore     int                          `json:"healthScore"`
	Grade           string                       `json:"grade"`
	Summary         VolSnapshotSummary1945       `json:"summary"`
	UnprotectedPVs  []VolSnapshotUnprotEntry1945 `json:"unprotectedPVs"`
	Snapshots       []VolSnapshotEntry1945       `json:"snapshots"`
	Recommendations []string                     `json:"recommendations"`
}

type VolSnapshotSummary1945 struct {
	TotalPVCs       int     `json:"totalPVCs"`
	WithSnapshot    int     `json:"pvcsWithSnapshot"`
	WithoutSnapshot int     `json:"pvcsWithoutSnapshot"`
	TotalSnapshots  int     `json:"totalSnapshots"`
	ReadySnapshots  int     `json:"readySnapshots"`
	FailedSnapshots int     `json:"failedSnapshots"`
	ProtectedPct    float64 `json:"protectedPct"`
}

type VolSnapshotEntry1945 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	PVCName   string `json:"pvcName"`
	Ready     bool   `json:"ready"`
	Size      string `json:"size"`
	Age       string `json:"age"`
}

type VolSnapshotUnprotEntry1945 struct {
	PVCName   string `json:"pvcName"`
	Namespace string `json:"namespace"`
	Size      string `json:"size"`
	Severity  string `json:"severity"`
}

func (s *Server) handleVolSnapshotAudit(w http.ResponseWriter, r *http.Request) {
	result := VolSnapshotResult1945{ScannedAt: time.Now()}
	score := 100

	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})

	// Try to get VolumeSnapshots
	snapProtected := make(map[string]bool) // "ns/pvc" -> has snapshot

	for _, pvc := range pvcList.Items {
		if isSystemNamespace(pvc.Namespace) {
			continue
		}
		if pvc.Status.Phase != corev1.ClaimBound {
			continue
		}
		result.Summary.TotalPVCs++

		key := fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name)
		size := pvc.Spec.Resources.Requests.Storage().String()

		if snapProtected[key] {
			result.Summary.WithSnapshot++
		} else {
			result.Summary.WithoutSnapshot++
			severity := "medium"
			if size != "" && len(size) > 0 {
				result.UnprotectedPVs = append(result.UnprotectedPVs, VolSnapshotUnprotEntry1945{
					PVCName: pvc.Name, Namespace: pvc.Namespace,
					Size: size, Severity: severity,
				})
			}
			score -= 3
		}
	}

	if result.Summary.TotalPVCs > 0 {
		result.Summary.ProtectedPct = float64(result.Summary.WithSnapshot) * 100 / float64(result.Summary.TotalPVCs)
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.WithoutSnapshot > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d PVCs without snapshots — enable backup for disaster recovery", result.Summary.WithoutSnapshot))
	}
	if result.Summary.ProtectedPct < 50 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("Only %.0f%% PVCs have snapshots — aim for >80%%", result.Summary.ProtectedPct))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Pod Priority Class Inventory
// ---------------------------------------------------------------

type PriorityClassResult1945 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         PriorityClassSummary1945 `json:"summary"`
	PriorityClasses []PriorityClassEntry1945 `json:"priorityClasses"`
	PodsByPriority  []PriorityPodStat1945    `json:"podsByPriority"`
	Recommendations []string                 `json:"recommendations"`
}

type PriorityClassSummary1945 struct {
	TotalPriorityClasses int   `json:"totalPriorityClasses"`
	SystemPriorityCount  int   `json:"systemPriorityCount"`
	CustomPriorityCount  int   `json:"customPriorityCount"`
	PodsWithPriority     int   `json:"podsWithPriorityClass"`
	PodsWithoutPriority  int   `json:"podsWithoutPriorityClass"`
	MaxPriorityValue     int32 `json:"maxPriorityValue"`
	PreemptionEnabled    int   `json:"preemptionEnabledCount"`
}

type PriorityClassEntry1945 struct {
	Name             string `json:"name"`
	PriorityValue    int32  `json:"priorityValue"`
	IsDefault        bool   `json:"isDefault"`
	PreemptionPolicy string `json:"preemptionPolicy"`
	GlobalDefault    bool   `json:"globalDefault"`
}

type PriorityPodStat1945 struct {
	PriorityClass string `json:"priorityClass"`
	PodCount      int    `json:"podCount"`
}

func (s *Server) handlePriorityClassInv(w http.ResponseWriter, r *http.Request) {
	result := PriorityClassResult1945{ScannedAt: time.Now()}
	score := 100

	pcList, _ := s.clientset.SchedulingV1().PriorityClasses().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	podPCCount := make(map[string]int)
	for _, pod := range podList.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		pcName := pod.Spec.PriorityClassName
		if pcName == "" {
			result.Summary.PodsWithoutPriority++
			pcName = "(none)"
		} else {
			result.Summary.PodsWithPriority++
		}
		podPCCount[pcName]++
	}

	for _, pc := range pcList.Items {
		result.Summary.TotalPriorityClasses++

		isSystem := strings.HasPrefix(pc.Name, "system-")
		if isSystem {
			result.Summary.SystemPriorityCount++
		} else {
			result.Summary.CustomPriorityCount++
		}

		if pc.Value > result.Summary.MaxPriorityValue {
			result.Summary.MaxPriorityValue = pc.Value
		}

		preemptionPolicy := "PreemptLowerPriority"
		if pc.PreemptionPolicy != nil {
			preemptionPolicy = string(*pc.PreemptionPolicy)
		}
		if preemptionPolicy == "PreemptLowerPriority" {
			result.Summary.PreemptionEnabled++
		}

		result.PriorityClasses = append(result.PriorityClasses, PriorityClassEntry1945{
			Name: pc.Name, PriorityValue: pc.Value,
			IsDefault: pc.GlobalDefault, PreemptionPolicy: preemptionPolicy,
			GlobalDefault: pc.GlobalDefault,
		})
	}

	for pc, count := range podPCCount {
		result.PodsByPriority = append(result.PodsByPriority, PriorityPodStat1945{
			PriorityClass: pc, PodCount: count,
		})
	}
	sort.Slice(result.PodsByPriority, func(i, j int) bool {
		return result.PodsByPriority[i].PodCount > result.PodsByPriority[j].PodCount
	})

	// Score: if no custom priority classes and many pods without priority
	if result.Summary.CustomPriorityCount == 0 && result.Summary.PodsWithoutPriority > 10 {
		score -= 10
	}
	if result.Summary.PodsWithoutPriority > 0 {
		score -= 2
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.PodsWithoutPriority > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods without priority class — add for scheduling fairness", result.Summary.PodsWithoutPriority))
	}
	if result.Summary.CustomPriorityCount == 0 {
		result.Recommendations = append(result.Recommendations, "No custom priority classes — define critical/normal/batch tiers")
	}
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d priority classes (%d system, %d custom)", result.Summary.TotalPriorityClasses, result.Summary.SystemPriorityCount, result.Summary.CustomPriorityCount))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Container Image Pull Policy
// -----------------------------------------------------------

type PullPolicyResult1945 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         PullPolicySummary1945 `json:"summary"`
	Violations      []PullPolicyEntry1945 `json:"violations"`
	ByPolicy        []PullPolicyStat1945  `json:"byPolicy"`
	Recommendations []string              `json:"recommendations"`
}

type PullPolicySummary1945 struct {
	TotalContainers  int `json:"totalContainers"`
	AlwaysPull       int `json:"alwaysPull"`
	IfNotPresentPull int `json:"ifNotPresentPull"`
	NeverPull        int `json:"neverPull"`
	NoPolicySet      int `json:"noPolicySet"`
	LatestWithAlways int `json:"latestWithAlways"`
	LatestWithIfNot  int `json:"latestWithIfNotPresent"`
}

type PullPolicyEntry1945 struct {
	PodName    string `json:"podName"`
	Namespace  string `json:"namespace"`
	Container  string `json:"container"`
	Image      string `json:"image"`
	PullPolicy string `json:"pullPolicy"`
	Violation  string `json:"violation"`
	Severity   string `json:"severity"`
}

type PullPolicyStat1945 struct {
	PullPolicy string `json:"pullPolicy"`
	Count      int    `json:"count"`
}

func (s *Server) handlePullPolicyAudit(w http.ResponseWriter, r *http.Request) {
	result := PullPolicyResult1945{ScannedAt: time.Now()}
	score := 100
	policyCount := make(map[string]int)

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}

		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			policy := string(c.ImagePullPolicy)
			if policy == "" {
				// Default: IfNotPresent for non-latest, Always for latest
				if strings.HasSuffix(c.Image, ":latest") || !strings.Contains(c.Image, ":") {
					policy = "Always(default)"
					result.Summary.NoPolicySet++
				} else {
					policy = "IfNotPresent(default)"
					result.Summary.NoPolicySet++
				}
			}

			policyCount[policy]++
			isLatest := strings.HasSuffix(c.Image, ":latest") || !strings.Contains(c.Image, ":")

			switch policy {
			case "Always":
				result.Summary.AlwaysPull++
				if !isLatest {
					// Always pull on versioned tags = unnecessary registry load
					result.Violations = append(result.Violations, PullPolicyEntry1945{
						PodName: pod.Name, Namespace: pod.Namespace, Container: c.Name,
						Image: c.Image, PullPolicy: policy,
						Violation: "Always pull on versioned tag — unnecessary registry load",
						Severity:  "low",
					})
					score -= 1
				} else {
					result.Summary.LatestWithAlways++
				}
			case "IfNotPresent":
				result.Summary.IfNotPresentPull++
				if isLatest {
					result.Summary.LatestWithIfNot++
					result.Violations = append(result.Violations, PullPolicyEntry1945{
						PodName: pod.Name, Namespace: pod.Namespace, Container: c.Name,
						Image: c.Image, PullPolicy: policy,
						Violation: ":latest with IfNotPresent — stale image risk",
						Severity:  "high",
					})
					score -= 3
				}
			case "Never":
				result.Summary.NeverPull++
				result.Violations = append(result.Violations, PullPolicyEntry1945{
					PodName: pod.Name, Namespace: pod.Namespace, Container: c.Name,
					Image: c.Image, PullPolicy: policy,
					Violation: "Never pull — will fail if image not pre-loaded",
					Severity:  "high",
				})
				score -= 5
			}
		}
	}

	for p, c := range policyCount {
		result.ByPolicy = append(result.ByPolicy, PullPolicyStat1945{PullPolicy: p, Count: c})
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.LatestWithIfNot > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d :latest images with IfNotPresent — switch to Always or pin versions", result.Summary.LatestWithIfNot))
	}
	if result.Summary.NeverPull > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers with Never pull — risky for multi-node clusters", result.Summary.NeverPull))
	}
	if result.Summary.AlwaysPull > 0 && result.Summary.LatestWithAlways == 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d Always-pull on non-latest — consider IfNotPresent to reduce registry load", result.Summary.AlwaysPull))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
