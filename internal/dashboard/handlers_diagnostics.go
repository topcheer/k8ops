package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	aiv1alpha1 "github.com/ggai/k8ops/api/v1alpha1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// --- Diagnostics ---

func (s *Server) handleDiagnostics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc.ctrlClient == nil {
		writeError(w, http.StatusServiceUnavailable, "controller runtime client not initialized")
		return
	}
	list := &aiv1alpha1.DiagnosticReportList{}

	namespace := r.URL.Query().Get("namespace")
	if namespace != "" {
		if err := rc.ctrlClient.List(ctx, list, client.InNamespace(namespace)); err != nil {
			writeK8sError(w, err)
			return
		}
	} else {
		if err := rc.ctrlClient.List(ctx, list); err != nil {
			writeK8sError(w, err)
			return
		}
	}

	type diagSummary struct {
		Name       string  `json:"name"`
		Namespace  string  `json:"namespace"`
		Phase      string  `json:"phase"`
		Trigger    string  `json:"trigger"`
		Summary    string  `json:"summary"`
		Confidence float64 `json:"confidence"`
		Findings   int     `json:"findings"`
		Created    string  `json:"created"`
	}

	results := make([]diagSummary, 0, len(list.Items))
	for _, d := range list.Items {
		phase := d.Status.Phase
		if phase == "" {
			phase = "Pending"
		}
		results = append(results, diagSummary{
			Name:       d.Name,
			Namespace:  d.Namespace,
			Phase:      phase,
			Trigger:    d.Spec.Trigger.Reason,
			Summary:    truncate(d.Status.Summary, 200),
			Confidence: d.Status.Confidence,
			Findings:   len(d.Status.Findings),
			Created:    d.CreationTimestamp.Format(time.RFC3339),
		})
	}

	// Sort by creation time, newest first
	sort.Slice(results, func(i, j int) bool {
		return results[i].Created > results[j].Created
	})

	writeJSON(w, map[string]any{"count": len(results), "items": results})
}

// handleDiagnosticsHistory returns a filtered history of DiagnosticReports.
// GET /api/diagnostics/history?status=
// Returns: {count, items: [{id, namespace, summary, status, createdAt}]}
func (s *Server) handleDiagnosticsHistory(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc.ctrlClient == nil {
		writeError(w, http.StatusServiceUnavailable, "controller runtime client not initialized")
		return
	}
	list := &aiv1alpha1.DiagnosticReportList{}

	namespace := r.URL.Query().Get("namespace")
	if namespace != "" {
		if err := rc.ctrlClient.List(ctx, list, client.InNamespace(namespace)); err != nil {
			writeK8sError(w, err)
			return
		}
	} else {
		if err := rc.ctrlClient.List(ctx, list); err != nil {
			writeK8sError(w, err)
			return
		}
	}

	filterStatus := r.URL.Query().Get("status")

	type historyItem struct {
		ID        string `json:"id"`
		Namespace string `json:"namespace"`
		Summary   string `json:"summary"`
		Status    string `json:"status"`
		CreatedAt string `json:"createdAt"`
	}

	results := make([]historyItem, 0, len(list.Items))
	for _, d := range list.Items {
		status := d.Status.Phase
		if status == "" {
			status = "Pending"
		}

		// Apply status filter
		if filterStatus != "" && !strings.EqualFold(status, filterStatus) {
			continue
		}

		results = append(results, historyItem{
			ID:        d.Name,
			Namespace: d.Namespace,
			Summary:   truncate(d.Status.Summary, 200),
			Status:    status,
			CreatedAt: d.CreationTimestamp.Format(time.RFC3339),
		})
	}

	// Sort by creation time, newest first
	sort.Slice(results, func(i, j int) bool {
		return results[i].CreatedAt > results[j].CreatedAt
	})

	writeJSON(w, map[string]any{"count": len(results), "items": results})
}

// handleDiagnosticDetail returns full details of a DiagnosticReport as markdown.
// GET /api/diagnostics/{namespace}/{name}
func (s *Server) handleDiagnosticDetail(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/diagnostics/"), "/")
	if len(parts) < 2 {
		writeError(w, 400, "expected /api/diagnostics/{namespace}/{name}")
		return
	}
	ns, name := parts[0], parts[1]

	dr := &aiv1alpha1.DiagnosticReport{}
	if err := rc.ctrlClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, dr); err != nil {
		writeError(w, 404, err.Error())
		return
	}

	// Build markdown content
	var md strings.Builder
	md.WriteString("# Diagnostic Report: " + dr.Name + "\n\n")
	md.WriteString("| Field | Value |\n|---|---|\n")
	md.WriteString(fmt.Sprintf("| Namespace | `%s` |\n", dr.Namespace))
	if dr.Status.Phase != "" {
		md.WriteString(fmt.Sprintf("| Phase | **%s** |\n", dr.Status.Phase))
	}
	if dr.Spec.Trigger.Reason != "" {
		md.WriteString(fmt.Sprintf("| Trigger | %s |\n", dr.Spec.Trigger.Reason))
	}
	if dr.Status.Confidence > 0 {
		md.WriteString(fmt.Sprintf("| Confidence | %.0f%% |\n", dr.Status.Confidence*100))
	}
	if dr.Status.AIModel != "" {
		md.WriteString(fmt.Sprintf("| AI Model | `%s` |\n", dr.Status.AIModel))
	}
	if !dr.Status.AnalyzedAt.IsZero() {
		md.WriteString(fmt.Sprintf("| Analyzed At | %s |\n", dr.Status.AnalyzedAt.Format(time.RFC3339)))
	}
	if dr.Status.Error != "" {
		md.WriteString(fmt.Sprintf("| Error | %s |\n", dr.Status.Error))
	}

	// Summary
	if dr.Status.Summary != "" {
		md.WriteString("\n## Summary\n\n")
		md.WriteString(dr.Status.Summary + "\n")
	}

	// Findings
	if len(dr.Status.Findings) > 0 {
		md.WriteString(fmt.Sprintf("\n## Findings (%d)\n\n", len(dr.Status.Findings)))
		for i, f := range dr.Status.Findings {
			md.WriteString(fmt.Sprintf("### %d. [%s] %s\n\n", i+1, f.Severity, f.Category))
			md.WriteString(f.Description + "\n\n")
			if f.RootCause != "" {
				md.WriteString("**Root Cause:** " + f.RootCause + "\n\n")
			}
			if len(f.Evidence) > 0 {
				md.WriteString("**Evidence:**\n\n")
				for _, e := range f.Evidence {
					md.WriteString(fmt.Sprintf("- *%s* (%s): `%s`\n", e.Type, e.Source, truncate(e.Message, 200)))
				}
				md.WriteString("\n")
			}
			if len(f.SuggestedActions) > 0 {
				md.WriteString("**Suggested Actions:**\n\n")
				for _, a := range f.SuggestedActions {
					md.WriteString(fmt.Sprintf("- [%s/%s risk] %s\n", a.Type, a.Risk, a.Description))
				}
				md.WriteString("\n")
			}
		}
	}

	writeJSON(w, map[string]any{
		"name":      dr.Name,
		"namespace": dr.Namespace,
		"phase":     dr.Status.Phase,
		"markdown":  md.String(),
	})
}

// --- Remediations ---

func (s *Server) handleRemediations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc.ctrlClient == nil {
		writeError(w, http.StatusServiceUnavailable, "controller runtime client not initialized")
		return
	}
	list := &aiv1alpha1.RemediationPlanList{}

	if err := rc.ctrlClient.List(ctx, list); err != nil {
		writeK8sError(w, err)
		return
	}

	type remSummary struct {
		Name       string `json:"name"`
		Namespace  string `json:"namespace"`
		Phase      string `json:"phase"`
		Mode       string `json:"mode"`
		Actions    int    `json:"actions"`
		Summary    string `json:"summary"`
		DiagRef    string `json:"diagnosticRef"`
		Created    string `json:"created"`
		ApprovedBy string `json:"approvedBy"`
	}

	results := make([]remSummary, 0, len(list.Items))
	for _, r := range list.Items {
		phase := r.Status.Phase
		if phase == "" {
			phase = "Pending"
		}
		results = append(results, remSummary{
			Name:       r.Name,
			Namespace:  r.Namespace,
			Phase:      phase,
			Mode:       r.Spec.Mode,
			Actions:    len(r.Spec.Actions),
			Summary:    truncate(r.Status.Summary, 200),
			DiagRef:    r.Spec.DiagnosticRef,
			Created:    r.CreationTimestamp.Format(time.RFC3339),
			ApprovedBy: r.Status.ApprovedBy,
		})
	}

	// Sort by creation time, newest first
	sort.Slice(results, func(i, j int) bool {
		return results[i].Created > results[j].Created
	})

	writeJSON(w, map[string]any{"count": len(results), "items": results})
}

// handleRemediationAction handles approve/reject actions on remediation plans.
// POST /api/remediation/{namespace}/{name}/approve
// POST /api/remediation/{namespace}/{name}/reject
func (s *Server) handleRemediationAction(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, 405, "method not allowed, use POST")
		return
	}

	ctx := r.Context()
	rc := s.clientsFromReq(r)

	// Parse path: /api/remediation/{namespace}/{name}/{action}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/remediation/"), "/")
	if len(parts) < 3 {
		writeError(w, 400, "expected /api/remediation/{namespace}/{name}/approve or /reject")
		return
	}
	ns, name, action := parts[0], parts[1], parts[2]

	if action != "approve" && action != "reject" {
		writeError(w, 400, `action must be "approve" or "reject"`)
		return
	}

	plan := &aiv1alpha1.RemediationPlan{}
	if err := rc.ctrlClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: name}, plan); err != nil {
		writeK8sError(w, err)
		return
	}

	switch action {
	case "approve":
		if plan.Status.Phase == "Approved" {
			writeJSON(w, map[string]string{"status": "already_approved"})
			return
		}
		if plan.Status.Phase != "Pending" {
			writeError(w, 400, "can only approve plans in Pending phase, current: "+plan.Status.Phase)
			return
		}
		plan.Status.Phase = "Approved"
		plan.Status.ApprovedBy = userName(r)
	case "reject":
		if plan.Status.Phase == "Failed" {
			writeJSON(w, map[string]string{"status": "already_rejected"})
			return
		}
		if plan.Status.Phase != "Pending" {
			writeError(w, 400, "can only reject plans in Pending phase, current: "+plan.Status.Phase)
			return
		}
		plan.Status.Phase = "Failed"
		plan.Status.Summary = "Rejected by " + userName(r)
	}

	if err := rc.ctrlClient.Status().Update(ctx, plan); err != nil {
		writeK8sError(w, err)
		return
	}

	writeJSON(w, map[string]string{
		"status": "ok",
		"phase":  string(plan.Status.Phase),
	})
}

// --- Optimizations ---

func (s *Server) handleOptimizations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc.ctrlClient == nil {
		writeError(w, http.StatusServiceUnavailable, "controller runtime client not initialized")
		return
	}
	list := &aiv1alpha1.OptimizationSuggestionList{}

	if err := rc.ctrlClient.List(ctx, list); err != nil {
		writeK8sError(w, err)
		return
	}

	type optSummary struct {
		Name        string `json:"name"`
		Namespace   string `json:"namespace"`
		Phase       string `json:"phase"`
		Scope       string `json:"scope"`
		Summary     string `json:"summary"`
		Suggestions int    `json:"suggestions"`
		Savings     string `json:"estimatedSavings"`
		Created     string `json:"created"`
	}

	results := make([]optSummary, 0, len(list.Items))
	for _, o := range list.Items {
		phase := o.Status.Phase
		if phase == "" {
			phase = "Pending"
		}
		results = append(results, optSummary{
			Name:        o.Name,
			Namespace:   o.Namespace,
			Phase:       phase,
			Scope:       o.Spec.Scope.Type,
			Summary:     truncate(o.Status.Summary, 200),
			Suggestions: len(o.Status.Suggestions),
			Savings:     o.Status.TotalEstimatedSavings,
			Created:     o.CreationTimestamp.Format(time.RFC3339),
		})
	}

	// Sort by creation time, newest first
	sort.Slice(results, func(i, j int) bool {
		return results[i].Created > results[j].Created
	})

	writeJSON(w, map[string]any{"count": len(results), "items": results})
}
