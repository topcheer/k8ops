package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// RHResult is the deployment rollout strategy & health analysis.
type RHResult struct {
	ScannedAt       time.Time `json:"scannedAt"`
	Summary         RHSummary `json:"summary"`
	Deployments     []RHEntry `json:"deployments"`
	StuckRollouts   []RHEntry `json:"stuckRollouts"`
	PoorStrategy    []RHEntry `json:"poorStrategy"`
	Issues          []RHIssue `json:"issues"`
	Recommendations []string  `json:"recommendations"`
}

// RHSummary aggregates rollout health statistics.
type RHSummary struct {
	TotalDeployments    int `json:"totalDeployments"`
	Healthy             int `json:"healthy"`
	Stuck               int `json:"stuck"`
	Paused              int `json:"paused"`
	Progressing         int `json:"progressing"`
	FailedReplicas      int `json:"failedReplicas"`
	DefaultStrategy     int `json:"defaultStrategy"`
	CustomStrategy      int `json:"customStrategy"`
	RecreateStrategy    int `json:"recreateStrategy"`
	NoRevisionHistory   int `json:"noRevisionHistory"`
	LowProgressDeadline int `json:"lowProgressDeadline"`
	NoMinReadySeconds   int `json:"noMinReadySeconds"`
	HealthScore         int `json:"healthScore"`
}

// RHEntry describes one deployment's rollout health.
type RHEntry struct {
	Name                 string        `json:"name"`
	Namespace            string        `json:"namespace"`
	Strategy             string        `json:"strategy"`
	MaxSurge             string        `json:"maxSurge"`
	MaxUnavailable       string        `json:"maxUnavailable"`
	RevisionHistoryLimit *int32        `json:"revisionHistoryLimit"`
	ProgressDeadline     int32         `json:"progressDeadlineSeconds"`
	MinReadySeconds      int32         `json:"minReadySeconds"`
	Replicas             int32         `json:"replicas"`
	UpdatedReplicas      int32         `json:"updatedReplicas"`
	ReadyReplicas        int32         `json:"readyReplicas"`
	AvailableReplicas    int32         `json:"availableReplicas"`
	UnavailableReplicas  int32         `json:"unavailableReplicas"`
	Conditions           []RHCondition `json:"conditions"`
	Status               string        `json:"status"`
	RollbackReady        bool          `json:"rollbackReady"`
	RiskLevel            string        `json:"riskLevel"`
	Age                  string        `json:"age"`
}

// RHCondition describes a deployment condition.
type RHCondition struct {
	Type    string `json:"type"`
	Status  string `json:"status"`
	Reason  string `json:"reason"`
	Message string `json:"message"`
}

// RHIssue is a detected problem.
type RHIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleRolloutHealth analyzes deployment rollout strategies and health.
// GET /api/deployment/rollout-health
func (s *Server) handleRolloutHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")
	deployments, err := rc.clientset.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	result := RHResult{ScannedAt: time.Now()}
	now := time.Now()

	for _, dep := range deployments.Items {
		entry := RHEntry{
			Name:      dep.Name,
			Namespace: dep.Namespace,
			Age:       now.Sub(dep.CreationTimestamp.Time).Round(time.Hour).String(),
		}

		entry.Strategy = string(dep.Spec.Strategy.Type)

		if dep.Spec.Strategy.RollingUpdate != nil {
			ru := dep.Spec.Strategy.RollingUpdate
			if ru.MaxSurge != nil {
				entry.MaxSurge = intStrToString(ru.MaxSurge)
			}
			if ru.MaxUnavailable != nil {
				entry.MaxUnavailable = intStrToString(ru.MaxUnavailable)
			}
		}

		entry.RevisionHistoryLimit = dep.Spec.RevisionHistoryLimit
		if dep.Spec.ProgressDeadlineSeconds != nil {
			entry.ProgressDeadline = *dep.Spec.ProgressDeadlineSeconds
		}
		entry.MinReadySeconds = dep.Spec.MinReadySeconds

		if dep.Spec.Replicas != nil {
			entry.Replicas = *dep.Spec.Replicas
		}
		entry.UpdatedReplicas = dep.Status.UpdatedReplicas
		entry.ReadyReplicas = dep.Status.ReadyReplicas
		entry.AvailableReplicas = dep.Status.AvailableReplicas
		entry.UnavailableReplicas = dep.Status.UnavailableReplicas

		for _, c := range dep.Status.Conditions {
			entry.Conditions = append(entry.Conditions, RHCondition{
				Type:    string(c.Type),
				Status:  string(c.Status),
				Reason:  c.Reason,
				Message: c.Message,
			})
		}

		entry.Status, entry.RiskLevel = rhClassify(dep, entry)
		entry.RollbackReady = rhAssessRollback(dep)

		result.Summary.TotalDeployments++

		switch entry.Strategy {
		case "RollingUpdate":
			if entry.MaxSurge == "" && entry.MaxUnavailable == "" {
				result.Summary.DefaultStrategy++
			} else {
				result.Summary.CustomStrategy++
			}
		case "Recreate":
			result.Summary.RecreateStrategy++
		}

		if dep.Spec.RevisionHistoryLimit != nil && *dep.Spec.RevisionHistoryLimit == 0 {
			result.Summary.NoRevisionHistory++
			result.Issues = append(result.Issues, RHIssue{
				Severity: "warning", Type: "no-rollback",
				Resource: fmt.Sprintf("%s/%s", dep.Namespace, dep.Name),
				Message:  fmt.Sprintf("revisionHistoryLimit=0 — rollback impossible for %s/%s", dep.Namespace, dep.Name),
			})
		}

		if entry.ProgressDeadline > 0 && entry.ProgressDeadline < 300 {
			result.Summary.LowProgressDeadline++
			result.Issues = append(result.Issues, RHIssue{
				Severity: "info", Type: "aggressive-deadline",
				Resource: fmt.Sprintf("%s/%s", dep.Namespace, dep.Name),
				Message:  fmt.Sprintf("progressDeadlineSeconds=%d (<300s) — may cause false rollout failures", entry.ProgressDeadline),
			})
		}

		if entry.MinReadySeconds == 0 {
			result.Summary.NoMinReadySeconds++
		}

		if entry.Status == "stuck" {
			result.Summary.Stuck++
			result.StuckRollouts = append(result.StuckRollouts, entry)
			result.Issues = append(result.Issues, RHIssue{
				Severity: "critical", Type: "stuck-rollout",
				Resource: fmt.Sprintf("%s/%s", dep.Namespace, dep.Name),
				Message:  fmt.Sprintf("Deployment %s/%s rollout is STUCK — %d/%d ready", dep.Namespace, dep.Name, entry.ReadyReplicas, entry.Replicas),
			})
			result.Summary.FailedReplicas += int(entry.UnavailableReplicas)
		} else if entry.Status == "progressing" {
			result.Summary.Progressing++
		} else if entry.Status == "paused" {
			result.Summary.Paused++
		} else if entry.Status == "healthy" {
			result.Summary.Healthy++
		}

		if entry.Strategy == "Recreate" && entry.Replicas > 1 {
			result.PoorStrategy = append(result.PoorStrategy, entry)
			result.Issues = append(result.Issues, RHIssue{
				Severity: "warning", Type: "downtime-strategy",
				Resource: fmt.Sprintf("%s/%s", dep.Namespace, dep.Name),
				Message:  fmt.Sprintf("Deployment %s/%s uses Recreate — causes downtime", dep.Namespace, dep.Name),
			})
		}

		result.Deployments = append(result.Deployments, entry)
	}

	sort.Slice(result.Deployments, func(i, j int) bool {
		return rhRiskRank(result.Deployments[i].RiskLevel) < rhRiskRank(result.Deployments[j].RiskLevel)
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return rhIssueRank(result.Issues[i].Severity) < rhIssueRank(result.Issues[j].Severity)
	})

	result.Summary.HealthScore = rhScore(result.Summary)
	result.Recommendations = rhRecs(result.Summary, result.StuckRollouts, result.PoorStrategy)

	writeJSON(w, result)
}

func rhClassify(dep appsv1.Deployment, entry RHEntry) (status, risk string) {
	if dep.Spec.Paused {
		return "paused", "medium"
	}

	for _, c := range dep.Status.Conditions {
		if c.Type == appsv1.DeploymentProgressing {
			if c.Status == "False" {
				return "stuck", "critical"
			}
			if c.Reason == "ProgressDeadlineExceeded" {
				return "stuck", "critical"
			}
			if c.Reason == "NewReplicaSetCreated" || c.Reason == "ReplicaSetUpdated" {
				return "progressing", "medium"
			}
		}
		if c.Type == appsv1.DeploymentReplicaFailure && c.Status == "True" {
			return "stuck", "high"
		}
	}

	if entry.Replicas > 0 && entry.ReadyReplicas == entry.Replicas && entry.UpdatedReplicas == entry.Replicas {
		return "healthy", "low"
	}

	if entry.Replicas > 0 && entry.ReadyReplicas < entry.Replicas {
		return "progressing", "medium"
	}

	return "healthy", "low"
}

func rhAssessRollback(dep appsv1.Deployment) bool {
	if dep.Spec.RevisionHistoryLimit != nil && *dep.Spec.RevisionHistoryLimit == 0 {
		return false
	}
	return true
}

func rhScore(s RHSummary) int {
	if s.TotalDeployments == 0 {
		return 100
	}
	score := 100
	score -= s.Stuck * 15
	score -= s.Paused * 5
	score -= s.NoRevisionHistory * 5
	score -= s.RecreateStrategy * 5
	if score < 0 {
		score = 0
	}
	return score
}

func rhRecs(s RHSummary, stuck []RHEntry, poor []RHEntry) []string {
	var recs []string

	if s.Stuck > 0 {
		top := ""
		if len(stuck) > 0 {
			top = fmt.Sprintf(" (e.g. %s/%s: %d/%d ready)", stuck[0].Namespace, stuck[0].Name, stuck[0].ReadyReplicas, stuck[0].Replicas)
		}
		recs = append(recs, fmt.Sprintf("%d deployment(s) have STUCK rollouts%s — check pod events and logs", s.Stuck, top))
	}
	if s.Paused > 0 {
		recs = append(recs, fmt.Sprintf("%d deployment(s) are paused — resume with kubectl rollout resume", s.Paused))
	}
	if s.NoRevisionHistory > 0 {
		recs = append(recs, fmt.Sprintf("%d deployment(s) have revisionHistoryLimit=0 — rollback impossible, set to >=2", s.NoRevisionHistory))
	}
	if s.RecreateStrategy > 0 {
		top := ""
		if len(poor) > 0 {
			top = fmt.Sprintf(" (e.g. %s/%s)", poor[0].Namespace, poor[0].Name)
		}
		recs = append(recs, fmt.Sprintf("%d deployment(s) use Recreate strategy%s — switch to RollingUpdate to avoid downtime", s.RecreateStrategy, top))
	}
	if s.LowProgressDeadline > 0 {
		recs = append(recs, fmt.Sprintf("%d deployment(s) have aggressive progressDeadline (<300s) — increase to 600s", s.LowProgressDeadline))
	}
	if s.NoMinReadySeconds > 0 {
		recs = append(recs, fmt.Sprintf("%d deployment(s) have minReadySeconds=0 — set >=5 to ensure pod stability", s.NoMinReadySeconds))
	}
	if s.HealthScore < 60 {
		recs = append(recs, fmt.Sprintf("Rollout health score is %d/100 — review deployment strategies", s.HealthScore))
	}
	if s.Stuck == 0 && s.Paused == 0 && s.NoRevisionHistory == 0 && s.RecreateStrategy == 0 {
		recs = append(recs, "All deployments have healthy rollouts with proper strategy and rollback support")
	}

	return recs
}

func rhRiskRank(level string) int {
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

func rhIssueRank(s string) int {
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
