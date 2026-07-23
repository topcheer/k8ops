package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v19.35 — Deployment Dimension (Round 9)
// 1. Deployment Revision Timeline — ReplicaSet revision history
// 2. Pod QoS Class Distribution — Guaranteed/Burstable/BestEffort
// 3. DaemonSet Health Monitor — DS rollout & node coverage
// ============================================================

// ---------------------------------------------------------------
// 1. Deployment Revision Timeline
// ---------------------------------------------------------------

type RevisionTimelineResult1935 struct {
	ScannedAt       time.Time                   `json:"scannedAt"`
	HealthScore     int                         `json:"healthScore"`
	Grade           string                      `json:"grade"`
	Summary         RevisionTimelineSummary1935 `json:"summary"`
	Deployments     []RevisionEntry1935         `json:"deployments"`
	OldRevisions    []OldRevisionEntry1935      `json:"oldRevisions"`
	Recommendations []string                    `json:"recommendations"`
}

type RevisionTimelineSummary1935 struct {
	TotalDeployments int `json:"totalDeployments"`
	TotalRevisions   int `json:"totalRevisions"`
	MaxRevisions     int `json:"maxRevisions"`
	OldReplicaSets   int `json:"oldReplicaSets"`
	StaleRevisions   int `json:"staleRevisions"`
	AvgRevisions     int `json:"avgRevisions"`
}

type RevisionEntry1935 struct {
	Name            string `json:"name"`
	Namespace       string `json:"namespace"`
	CurrentRev      string `json:"currentRevision"`
	OldRevCount     int    `json:"oldRevisionCount"`
	TotalReplicas   int32  `json:"totalReplicaSets"`
	UpdatedReplicas int32  `json:"updatedReplicas"`
}

type OldRevisionEntry1935 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Revision  string `json:"revision"`
	Replicas  int32  `json:"replicas"`
	Age       string `json:"age"`
}

func (s *Server) handleRevisionTimeline(w http.ResponseWriter, r *http.Request) {
	result := RevisionTimelineResult1935{ScannedAt: time.Now()}
	score := 100

	depList, err := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	// Get ReplicaSets to count revisions
	rsList, err := s.clientset.AppsV1().ReplicaSets("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	type rsInfo struct {
		namespace  string
		revision   string
		replicas   int32
		age        time.Duration
		ownedByDep string
	}
	allRS := make(map[string][]rsInfo) // "ns/dep" -> revisions

	for _, rs := range rsList.Items {
		if isSystemNamespace(rs.Namespace) {
			continue
		}
		rev := rs.Annotations["deployment.kubernetes.io/revision"]
		if rev == "" {
			continue
		}
		ownerName := ""
		for _, owner := range rs.OwnerReferences {
			if owner.Kind == "Deployment" {
				ownerName = owner.Name
				break
			}
		}
		if ownerName == "" {
			continue
		}
		key := fmt.Sprintf("%s/%s", rs.Namespace, ownerName)
		allRS[key] = append(allRS[key], rsInfo{
			namespace: rs.Namespace, revision: rev,
			replicas: rs.Status.Replicas,
			age:      time.Since(rs.CreationTimestamp.Time),
		})
	}

	var totalRevs int
	for _, dep := range depList.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalDeployments++

		key := fmt.Sprintf("%s/%s", dep.Namespace, dep.Name)
		revisions := allRS[key]
		revCount := len(revisions)

		currentRev := ""
		for _, ri := range revisions {
			if ri.replicas > 0 {
				currentRev = ri.revision
				break
			}
		}

		entry := RevisionEntry1935{
			Name: dep.Name, Namespace: dep.Namespace,
			CurrentRev: currentRev, OldRevCount: revCount - 1,
			TotalReplicas: int32(revCount), UpdatedReplicas: dep.Status.UpdatedReplicas,
		}
		result.Deployments = append(result.Deployments, entry)
		totalRevs += revCount
		if revCount > result.Summary.MaxRevisions {
			result.Summary.MaxRevisions = revCount
		}

		// Flag stale old ReplicaSets
		for _, ri := range revisions {
			if ri.replicas == 0 && ri.age.Hours() > 168 {
				result.OldRevisions = append(result.OldRevisions, OldRevisionEntry1935{
					Name: dep.Name, Namespace: dep.Namespace,
					Revision: ri.revision, Replicas: ri.replicas,
					Age: fmt.Sprintf("%.0fd", ri.age.Hours()/24),
				})
				result.Summary.StaleRevisions++
				result.Summary.OldReplicaSets++
			}
		}

		// Score: too many revisions = clutter
		if revCount > 10 {
			score -= 1
		}
	}

	if result.Summary.TotalDeployments > 0 {
		result.Summary.AvgRevisions = totalRevs / result.Summary.TotalDeployments
	}
	result.Summary.TotalRevisions = totalRevs

	if result.Summary.StaleRevisions > 10 {
		score -= 5
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.StaleRevisions > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d stale ReplicaSets older than 7 days — clean up revision history", result.Summary.StaleRevisions))
	}
	if result.Summary.MaxRevisions > 10 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("Max %d revisions per deployment — consider lowering revisionHistoryLimit", result.Summary.MaxRevisions))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Pod QoS Class Distribution
// ---------------------------------------------------------------

type QoSDistResult1935 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         QoSDistSummary1935 `json:"summary"`
	ByNamespace     []QoSNSStat1935    `json:"byNamespace"`
	BestEffortPods  []QoSPodEntry1935  `json:"bestEffortPods"`
	Recommendations []string           `json:"recommendations"`
}

type QoSDistSummary1935 struct {
	TotalPods     int     `json:"totalPods"`
	Guaranteed    int     `json:"guaranteed"`
	Burstable     int     `json:"burstable"`
	BestEffort    int     `json:"bestEffort"`
	GuaranteedPct float64 `json:"guaranteedPct"`
	NoRequests    int     `json:"noRequests"`
	NoLimits      int     `json:"noLimits"`
}

type QoSNSStat1935 struct {
	Namespace  string `json:"namespace"`
	Guaranteed int    `json:"guaranteed"`
	Burstable  int    `json:"burstable"`
	BestEffort int    `json:"bestEffort"`
	Total      int    `json:"total"`
}

type QoSPodEntry1935 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	QoSClass  string `json:"qosClass"`
}

func (s *Server) handleQoSDistribution(w http.ResponseWriter, r *http.Request) {
	result := QoSDistResult1935{ScannedAt: time.Now()}
	score := 100
	nsStats := make(map[string]*QoSNSStat1935)

	podList, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	for _, pod := range podList.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		qos := string(pod.Status.QOSClass)
		if qos == "" {
			// Determine manually
			allHaveReq := true
			allHaveLim := true
			for _, c := range pod.Spec.Containers {
				if c.Resources.Requests.Cpu().IsZero() || c.Resources.Requests.Memory().IsZero() {
					allHaveReq = false
				}
				if c.Resources.Limits.Cpu().IsZero() || c.Resources.Limits.Memory().IsZero() {
					allHaveLim = false
				}
			}
			if allHaveReq && allHaveLim {
				qos = "Guaranteed"
			} else if allHaveReq {
				qos = "Burstable"
			} else {
				qos = "BestEffort"
			}
		}

		// Count missing requests/limits
		for _, c := range pod.Spec.Containers {
			if c.Resources.Requests.Cpu().IsZero() {
				result.Summary.NoRequests++
			}
			if c.Resources.Limits.Cpu().IsZero() {
				result.Summary.NoLimits++
			}
		}

		switch qos {
		case "Guaranteed":
			result.Summary.Guaranteed++
		case "Burstable":
			result.Summary.Burstable++
		case "BestEffort":
			result.Summary.BestEffort++
			result.BestEffortPods = append(result.BestEffortPods, QoSPodEntry1935{
				Name: pod.Name, Namespace: pod.Namespace, QoSClass: qos,
			})
			score -= 1
		}

		ns, exists := nsStats[pod.Namespace]
		if !exists {
			ns = &QoSNSStat1935{Namespace: pod.Namespace}
			nsStats[pod.Namespace] = ns
		}
		ns.Total++
		switch qos {
		case "Guaranteed":
			ns.Guaranteed++
		case "Burstable":
			ns.Burstable++
		case "BestEffort":
			ns.BestEffort++
		}
	}

	for _, ns := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}

	if result.Summary.TotalPods > 0 {
		result.Summary.GuaranteedPct = float64(result.Summary.Guaranteed) * 100 / float64(result.Summary.TotalPods)
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.BestEffort > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d BestEffort pods — add resource requests for QoS guarantees", result.Summary.BestEffort))
	}
	if result.Summary.NoRequests > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers without CPU requests — unpredictable scheduling", result.Summary.NoRequests))
	}
	if result.Summary.GuaranteedPct < 30 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("Only %.0f%% Guaranteed QoS — aim for >50%% in production", result.Summary.GuaranteedPct))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. DaemonSet Health Monitor
// ---------------------------------------------------------------

type DSHealthResult1935 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         DSHealthSummary1935 `json:"summary"`
	DaemonSets      []DSEntry1935       `json:"daemonSets"`
	Issues          []DSIssue1935       `json:"issues"`
	Recommendations []string            `json:"recommendations"`
}

type DSHealthSummary1935 struct {
	TotalDS            int `json:"totalDaemonSets"`
	HealthyDS          int `json:"healthyDaemonSets"`
	DesiredScheduled   int `json:"desiredScheduled"`
	CurrentScheduled   int `json:"currentScheduled"`
	NumberReady        int `json:"numberReady"`
	NumberMisscheduled int `json:"numberMisscheduled"`
	UpdatedDS          int `json:"updatedDaemonSets"`
	IssueCount         int `json:"issueCount"`
}

type DSEntry1935 struct {
	Name             string `json:"name"`
	Namespace        string `json:"namespace"`
	DesiredScheduled int32  `json:"desiredScheduled"`
	CurrentScheduled int32  `json:"currentScheduled"`
	NumberReady      int32  `json:"numberReady"`
	NumberAvailable  int32  `json:"numberAvailable"`
	Misscheduled     int32  `json:"numberMisscheduled"`
	Updated          int32  `json:"updatedNumberScheduled"`
	UpdateStrategy   string `json:"updateStrategy"`
	Age              string `json:"age"`
}

type DSIssue1935 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	IssueType string `json:"issueType"`
	Severity  string `json:"severity"`
	Detail    string `json:"detail"`
}

func (s *Server) handleDSHealth(w http.ResponseWriter, r *http.Request) {
	result := DSHealthResult1935{ScannedAt: time.Now()}
	score := 100

	dsList, err := s.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	for _, ds := range dsList.Items {
		if isSystemNamespace(ds.Namespace) {
			continue
		}
		result.Summary.TotalDS++

		desired := ds.Status.DesiredNumberScheduled
		current := ds.Status.CurrentNumberScheduled
		ready := ds.Status.NumberReady
		available := ds.Status.NumberAvailable
		missched := ds.Status.NumberMisscheduled
		updated := ds.Status.UpdatedNumberScheduled
		strategy := string(ds.Spec.UpdateStrategy.Type)

		entry := DSEntry1935{
			Name: ds.Name, Namespace: ds.Namespace,
			DesiredScheduled: desired, CurrentScheduled: current,
			NumberReady: ready, NumberAvailable: available,
			Misscheduled: missched, Updated: updated,
			UpdateStrategy: strategy,
			Age:            fmt.Sprintf("%.0fd", time.Since(ds.CreationTimestamp.Time).Hours()/24),
		}
		result.DaemonSets = append(result.DaemonSets, entry)

		result.Summary.DesiredScheduled += int(desired)
		result.Summary.CurrentScheduled += int(current)
		result.Summary.NumberReady += int(ready)
		result.Summary.NumberMisscheduled += int(missched)

		isHealthy := true

		if ready < desired && desired > 0 {
			result.Issues = append(result.Issues, DSIssue1935{
				Name: ds.Name, Namespace: ds.Namespace,
				IssueType: "not-all-ready", Severity: "high",
				Detail: fmt.Sprintf("%d/%d pods ready — some DaemonSet pods unhealthy", ready, desired),
			})
			score -= 5
			isHealthy = false
		}

		if missched > 0 {
			result.Issues = append(result.Issues, DSIssue1935{
				Name: ds.Name, Namespace: ds.Namespace,
				IssueType: "misscheduled", Severity: "medium",
				Detail: fmt.Sprintf("%d pods misscheduled — running on wrong nodes", missched),
			})
			score -= 3
			isHealthy = false
		}

		if updated < desired && desired > 0 {
			result.Issues = append(result.Issues, DSIssue1935{
				Name: ds.Name, Namespace: ds.Namespace,
				IssueType: "update-pending", Severity: "medium",
				Detail: fmt.Sprintf("%d/%d pods updated — rollout incomplete", updated, desired),
			})
			result.Summary.UpdatedDS--
			isHealthy = false
		}

		if isHealthy {
			result.Summary.HealthyDS++
		}
		result.Summary.UpdatedDS++
	}

	result.Summary.IssueCount = len(result.Issues)
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if len(result.Issues) > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d DaemonSet issues — investigate pod scheduling and health", len(result.Issues)))
	}
	if result.Summary.NumberMisscheduled > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d misscheduled pods — check node selectors and taints", result.Summary.NumberMisscheduled))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
