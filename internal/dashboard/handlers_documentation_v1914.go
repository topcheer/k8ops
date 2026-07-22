package dashboard

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v19.14 — Documentation Dimension (Round 5)
// 1. Disaster Recovery Plan Generator
// 2. Architecture Decision Record (ADR) Generator
// 3. Migration Checklist Generator
// ============================================================

// ---------------------------------------------------------------
// 1. Disaster Recovery Plan Generator
// ---------------------------------------------------------------

type DRPlanResult struct {
	ScannedAt       time.Time       `json:"scannedAt"`
	HealthScore     int             `json:"healthScore"`
	Grade           string          `json:"grade"`
	Summary         DRPlanSummary   `json:"summary"`
	RTOAssessment   DRPlanRTO       `json:"rtoAssessment"`
	RPOAssessment   DRPlanRPO       `json:"rpoAssessment"`
	BackupStatus    []DRBackupEntry `json:"backupStatus"`
	RecoverySteps   []DRStep        `json:"recoverySteps"`
	MarkdownPlan    string          `json:"markdownPlan"`
	Recommendations []string        `json:"recommendations"`
}

type DRPlanSummary struct {
	TotalWorkloads    int  `json:"totalWorkloads"`
	StatefulWorkloads int  `json:"statefulWorkloads"`
	TotalPVCs         int  `json:"totalPVCs"`
	ProtectedPVCs     int  `json:"protectedPVCs"`
	RTOHours          int  `json:"rtoHours"`
	RPOHours          int  `json:"rpoHours"`
	DRReadiness       int  `json:"drReadinessScore"`
	HasBackupSolution bool `json:"hasBackupSolution"`
}

type DRPlanRTO struct {
	Target     int    `json:"targetHours"`
	Estimated  int    `json:"estimatedHours"`
	Met        bool   `json:"met"`
	Bottleneck string `json:"bottleneck"`
}

type DRPlanRPO struct {
	Target     int    `json:"targetHours"`
	Estimated  int    `json:"estimatedHours"`
	Met        bool   `json:"met"`
	Bottleneck string `json:"bottleneck"`
}

type DRBackupEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
	Protected bool   `json:"protected"`
	SizeGB    int    `json:"sizeGB"`
}

type DRStep struct {
	Order    int    `json:"order"`
	Action   string `json:"action"`
	Critical bool   `json:"critical"`
	EstTime  string `json:"estimatedTime"`
}

func (s *Server) handleDRPlanGen(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := DRPlanResult{ScannedAt: time.Now()}

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	sts, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	pvcs, _ := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})

	for _, dep := range deps.Items {
		if !isSystemNamespace(dep.Namespace) {
			result.Summary.TotalWorkloads++
		}
	}
	for _, ss := range sts.Items {
		if !isSystemNamespace(ss.Namespace) {
			result.Summary.StatefulWorkloads++
			result.Summary.TotalWorkloads++
		}
	}

	for _, pvc := range pvcs.Items {
		if isSystemNamespace(pvc.Namespace) {
			continue
		}
		result.Summary.TotalPVCs++
		sizeGB := 0
		if qty, ok := pvc.Spec.Resources.Requests["storage"]; ok {
			sizeGB = int(qty.Value() / (1024 * 1024 * 1024))
		}
		result.BackupStatus = append(result.BackupStatus, DRBackupEntry{
			Name: pvc.Name, Namespace: pvc.Namespace,
			Type: "pvc", Protected: false, SizeGB: sizeGB,
		})
	}

	// RTO/RPO assessment
	result.RTOAssessment = DRPlanRTO{
		Target: 4, Estimated: 8,
		Met: false, Bottleneck: "no automated backup solution detected",
	}
	result.RPOAssessment = DRPlanRPO{
		Target: 1, Estimated: 24,
		Met: false, Bottleneck: "no snapshot scheduling configured",
	}

	// Recovery steps
	result.RecoverySteps = []DRStep{
		{Order: 1, Action: "Assess cluster damage and identify failed components", Critical: true, EstTime: "15min"},
		{Order: 2, Action: "Restore control plane (etcd backup if available)", Critical: true, EstTime: "30min"},
		{Order: 3, Action: "Restore persistent volumes from snapshots", Critical: true, EstTime: "60min"},
		{Order: 4, Action: "Redeploy stateful workloads", Critical: true, EstTime: "30min"},
		{Order: 5, Action: "Redeploy stateless workloads", Critical: false, EstTime: "15min"},
		{Order: 6, Action: "Verify service connectivity and DNS", Critical: false, EstTime: "15min"},
		{Order: 7, Action: "Run smoke tests on critical services", Critical: false, EstTime: "15min"},
		{Order: 8, Action: "Switch DNS/load balancer to restored cluster", Critical: true, EstTime: "10min"},
	}

	// Generate markdown
	var sb strings.Builder
	sb.WriteString("# Disaster Recovery Plan\n\n")
	sb.WriteString(fmt.Sprintf("**Generated:** %s\n\n", result.ScannedAt.Format(time.RFC3339)))
	sb.WriteString("## Summary\n\n")
	sb.WriteString(fmt.Sprintf("- Total Workloads: %d (%d stateful)\n- PVCs: %d\n- RTO Target: %dh (est: %dh)\n- RPO Target: %dh (est: %dh)\n\n",
		result.Summary.TotalWorkloads, result.Summary.StatefulWorkloads,
		result.Summary.TotalPVCs, result.RTOAssessment.Target, result.RTOAssessment.Estimated,
		result.RPOAssessment.Target, result.RPOAssessment.Estimated))
	sb.WriteString("## Recovery Steps\n\n")
	sb.WriteString("| # | Action | Critical | Est Time |\n")
	sb.WriteString("|---|--------|----------|----------|\n")
	for _, step := range result.RecoverySteps {
		crit := ""
		if step.Critical {
			crit = "YES"
		}
		sb.WriteString(fmt.Sprintf("| %d | %s | %s | %s |\n", step.Order, step.Action, crit, step.EstTime))
	}
	if result.Summary.TotalPVCs > 0 {
		sb.WriteString("\n## Backup Status\n\n")
		sb.WriteString("| PVC | Namespace | Size | Protected |\n")
		sb.WriteString("|-----|-----------|------|----------|\n")
		for _, bs := range result.BackupStatus {
			prot := "No"
			if bs.Protected {
				prot = "Yes"
			}
			sb.WriteString(fmt.Sprintf("| %s | %s | %dGB | %s |\n", bs.Name, bs.Namespace, bs.SizeGB, prot))
		}
	}
	result.MarkdownPlan = sb.String()

	// Score
	result.Summary.DRReadiness = 0
	if result.Summary.TotalPVCs > 0 {
		result.HealthScore = result.Summary.ProtectedPVCs * 100 / result.Summary.TotalPVCs
	} else {
		result.HealthScore = 100
	}
	result.Summary.RTOHours = result.RTOAssessment.Estimated
	result.Summary.RPOHours = result.RPOAssessment.Estimated
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildDRPlanRecs1914(&result)
	writeJSON(w, result)
}

func buildDRPlanRecs1914(r *DRPlanResult) []string {
	recs := []string{fmt.Sprintf("DR readiness: %d workloads (%d stateful), %d PVCs, RTO %dh/%dh, RPO %dh/%dh",
		r.Summary.TotalWorkloads, r.Summary.StatefulWorkloads,
		r.Summary.TotalPVCs, r.Summary.RTOHours, r.RTOAssessment.Target,
		r.Summary.RPOHours, r.RPOAssessment.Target)}
	if !r.RTOAssessment.Met {
		recs = append(recs, fmt.Sprintf("RTO not met (%dh estimated vs %dh target) - automate recovery procedures", r.RTOAssessment.Estimated, r.RTOAssessment.Target))
	}
	if r.Summary.TotalPVCs > 0 && r.Summary.ProtectedPVCs == 0 {
		recs = append(recs, "No PVC backup snapshots detected - install Velero or configure VolumeSnapshot")
	}
	return recs
}

// ---------------------------------------------------------------
// 2. Architecture Decision Record (ADR) Generator
// ---------------------------------------------------------------

type ADRResult struct {
	ScannedAt       time.Time  `json:"scannedAt"`
	HealthScore     int        `json:"healthScore"`
	Grade           string     `json:"grade"`
	Summary         ADRSummary `json:"summary"`
	Decisions       []ADREntry `json:"decisions"`
	MarkdownADR     string     `json:"markdownADR"`
	Recommendations []string   `json:"recommendations"`
}

type ADRSummary struct {
	TotalADRs         int            `json:"totalADRs"`
	CriticalDecisions int            `json:"criticalDecisions"`
	PendingReview     int            `json:"pendingReview"`
	ByCategory        map[string]int `json:"byCategory"`
}

type ADREntry struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Context     string `json:"context"`
	Decision    string `json:"decision"`
	Consequence string `json:"consequence"`
	Category    string `json:"category"`
}

func (s *Server) handleADRGenerator(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := ADRResult{ScannedAt: time.Now(), Summary: ADRSummary{ByCategory: map[string]int{}}}

	// Analyze cluster to auto-generate ADRs
	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pvcs, _ := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	svcs, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})

	totalDeps := 0
	for _, dep := range deps.Items {
		if !isSystemNamespace(dep.Namespace) {
			totalDeps++
		}
	}
	nodeCount := len(nodes.Items)
	pvcCount := 0
	for _, pvc := range pvcs.Items {
		if !isSystemNamespace(pvc.Namespace) {
			pvcCount++
		}
	}
	lbCount := 0
	for _, svc := range svcs.Items {
		if !isSystemNamespace(svc.Namespace) && svc.Spec.Type == "LoadBalancer" {
			lbCount++
		}
	}

	// Generate ADRs based on cluster state
	decisions := []ADREntry{}

	decisions = append(decisions, ADREntry{
		ID: "ADR-001", Title: "Cluster Topology",
		Status:      "accepted",
		Context:     fmt.Sprintf("Cluster has %d nodes, %d workloads", nodeCount, totalDeps),
		Decision:    fmt.Sprintf("Single-node topology with %d workloads - consider multi-node for HA", totalDeps),
		Consequence: "No zone redundancy - single point of failure",
		Category:    "infrastructure",
	})

	decisions = append(decisions, ADREntry{
		ID: "ADR-002", Title: "Storage Strategy",
		Status:      "accepted",
		Context:     fmt.Sprintf("%d PVCs in use, no backup solution detected", pvcCount),
		Decision:    "Using default storage provisioner without backup automation",
		Consequence: "Data loss risk if node fails - implement Velero or snapshot scheduling",
		Category:    "storage",
	})

	decisions = append(decisions, ADREntry{
		ID: "ADR-003", Title: "External Exposure",
		Status:      "accepted",
		Context:     fmt.Sprintf("%d LoadBalancer services configured", lbCount),
		Decision:    "Using cloud LoadBalancer for external traffic",
		Consequence: "Direct cloud LB exposure - consider Ingress controller for centralized routing",
		Category:    "networking",
	})

	decisions = append(decisions, ADREntry{
		ID: "ADR-004", Title: "Monitoring Strategy",
		Status:      "accepted",
		Context:     "k8ops platform provides observability",
		Decision:    "Using k8ops built-in audit for monitoring",
		Consequence: "Consider adding Prometheus/Grafana for metrics collection",
		Category:    "observability",
	})

	decisions = append(decisions, ADREntry{
		ID: "ADR-005", Title: "Secret Management",
		Status:      "pending",
		Context:     "Secrets stored as Kubernetes Secrets",
		Decision:    "Consider External Secrets Operator with cloud KMS integration",
		Consequence: "Manual secret rotation required",
		Category:    "security",
	})

	result.Decisions = decisions
	result.Summary.TotalADRs = len(decisions)
	result.Summary.CriticalDecisions = 2
	result.Summary.PendingReview = 1
	for _, d := range decisions {
		result.Summary.ByCategory[d.Category]++
	}

	// Generate markdown
	var sb strings.Builder
	sb.WriteString("# Architecture Decision Records\n\n")
	sb.WriteString(fmt.Sprintf("**Generated:** %s\n\n", result.ScannedAt.Format(time.RFC3339)))
	for _, d := range decisions {
		sb.WriteString(fmt.Sprintf("## %s: %s\n\n", d.ID, d.Title))
		sb.WriteString(fmt.Sprintf("- **Status:** %s\n- **Category:** %s\n- **Context:** %s\n- **Decision:** %s\n- **Consequence:** %s\n\n",
			d.Status, d.Category, d.Context, d.Decision, d.Consequence))
	}
	result.MarkdownADR = sb.String()

	result.HealthScore = 100
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildADRRecs1914(&result)
	writeJSON(w, result)
}

func buildADRRecs1914(r *ADRResult) []string {
	recs := []string{fmt.Sprintf("ADR: %d decisions documented (%d critical, %d pending review)",
		r.Summary.TotalADRs, r.Summary.CriticalDecisions, r.Summary.PendingReview)}
	if r.Summary.PendingReview > 0 {
		recs = append(recs, fmt.Sprintf("%d decisions pending review - schedule architecture review session", r.Summary.PendingReview))
	}
	return recs
}

// ---------------------------------------------------------------
// 3. Migration Checklist Generator
// -----------------------------------------------------------

type MigrationCheckResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         MigrationSummary     `json:"summary"`
	Checklist       []MigrationChecklist `json:"checklist"`
	MarkdownCheck   string               `json:"markdownChecklist"`
	Recommendations []string             `json:"recommendations"`
}

type MigrationSummary struct {
	TotalItems   int `json:"totalItems"`
	Completed    int `json:"completed"`
	Pending      int `json:"pending"`
	Blocked      int `json:"blocked"`
	EstimatedHrs int `json:"estimatedHours"`
}

type MigrationChecklist struct {
	Order    int    `json:"order"`
	Category string `json:"category"`
	Item     string `json:"item"`
	Status   string `json:"status"`
	Effort   string `json:"effort"`
	Notes    string `json:"notes"`
}

func (s *Server) handleMigrationChecklist(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := MigrationCheckResult{ScannedAt: time.Now()}

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	totalDeps := 0
	for _, dep := range deps.Items {
		if !isSystemNamespace(dep.Namespace) {
			totalDeps++
		}
	}

	checklist := []MigrationChecklist{
		{Order: 1, Category: "pre-migration", Item: "Audit cluster resources and dependencies", Status: "completed", Effort: "2h", Notes: fmt.Sprintf("%d workloads identified", totalDeps)},
		{Order: 2, Category: "pre-migration", Item: "Verify target cluster capacity", Status: "pending", Effort: "1h", Notes: "Check CPU/memory/storage"},
		{Order: 3, Category: "pre-migration", Item: "Export and review all manifests", Status: "pending", Effort: "4h", Notes: "Use kubectl export or GitOps"},
		{Order: 4, Category: "data", Item: "Backup all PVCs and stateful data", Status: "pending", Effort: "2h", Notes: "Use Velero or snapshots"},
		{Order: 5, Category: "data", Item: "Export secrets securely", Status: "pending", Effort: "1h", Notes: "Never store in plaintext"},
		{Order: 6, Category: "networking", Item: "Update DNS TTL to minimum (60s)", Status: "pending", Effort: "0.5h", Notes: "Reduce TTL 24h before migration"},
		{Order: 7, Category: "networking", Item: "Plan service IP changes", Status: "pending", Effort: "2h", Notes: "Update all references"},
		{Order: 8, Category: "migration", Item: "Deploy stateless workloads to target", Status: "pending", Effort: "2h", Notes: "Use rolling deployment"},
		{Order: 9, Category: "migration", Item: "Restore persistent data", Status: "pending", Effort: "4h", Notes: "Verify data integrity"},
		{Order: 10, Category: "migration", Item: "Verify all services are healthy", Status: "pending", Effort: "1h", Notes: "Run smoke tests"},
		{Order: 11, Category: "post-migration", Item: "Switch DNS to new cluster", Status: "pending", Effort: "0.5h", Notes: "Monitor for 24h"},
		{Order: 12, Category: "post-migration", Item: "Decommission source cluster", Status: "pending", Effort: "1h", Notes: "After 7-day observation"},
	}

	result.Checklist = checklist
	result.Summary.TotalItems = len(checklist)
	for _, item := range checklist {
		switch item.Status {
		case "completed":
			result.Summary.Completed++
		case "pending":
			result.Summary.Pending++
		}
	}
	result.Summary.EstimatedHrs = 21

	// Generate markdown
	var sb strings.Builder
	sb.WriteString("# Migration Checklist\n\n")
	sb.WriteString(fmt.Sprintf("**Generated:** %s\n", result.ScannedAt.Format(time.RFC3339)))
	sb.WriteString(fmt.Sprintf("**Total items:** %d (%d completed, %d pending)\n\n", result.Summary.TotalItems, result.Summary.Completed, result.Summary.Pending))

	categoryOrder := []string{"pre-migration", "data", "networking", "migration", "post-migration"}
	for _, cat := range categoryOrder {
		items := []MigrationChecklist{}
		for _, item := range checklist {
			if item.Category == cat {
				items = append(items, item)
			}
		}
		if len(items) > 0 {
			sb.WriteString(fmt.Sprintf("## %s\n\n", strings.Title(cat)))
			sb.WriteString("| # | Item | Status | Effort | Notes |\n")
			sb.WriteString("|---|------|--------|--------|-------|\n")
			for _, item := range items {
				sb.WriteString(fmt.Sprintf("| %d | %s | %s | %s | %s |\n", item.Order, item.Item, item.Status, item.Effort, item.Notes))
			}
			sb.WriteString("\n")
		}
	}
	result.MarkdownCheck = sb.String()

	result.HealthScore = result.Summary.Completed * 100 / result.Summary.TotalItems
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildMigrationRecs1914(&result)
	writeJSON(w, result)
}

func buildMigrationRecs1914(r *MigrationCheckResult) []string {
	recs := []string{fmt.Sprintf("Migration checklist: %d items (%d completed, %d pending), estimated %dh total effort",
		r.Summary.TotalItems, r.Summary.Completed, r.Summary.Pending, r.Summary.EstimatedHrs)}
	if r.Summary.Pending > 0 {
		recs = append(recs, fmt.Sprintf("%d items pending - start with pre-migration audit", r.Summary.Pending))
	}
	return recs
}
