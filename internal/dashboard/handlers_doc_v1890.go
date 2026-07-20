package dashboard

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes"
)

// ============================================================
// 1. Backup Compliance Deep
// ============================================================

// BackupComplianceResult provides a deep audit of backup compliance posture.
type BackupComplianceResult struct {
	ScannedAt        time.Time               `json:"scannedAt"`
	HealthScore      int                     `json:"healthScore"`
	Grade            string                  `json:"grade"`
	Summary          BackupComplianceSummary `json:"summary"`
	NamespacePolicy  []BackupNamespacePolicy `json:"namespacePolicy"`
	PVCBackupStatus  []BackupPVCEntry        `json:"pvcBackupStatus"`
	SecretBackupRisk []BackupSecretEntry     `json:"secretBackupRisk"`
	Checklist        []BackupChecklistItem   `json:"checklist"`
	Recommendations  []string                `json:"recommendations"`
}

type BackupComplianceSummary struct {
	TotalNamespaces     int `json:"totalNamespaces"`
	WithBackupPolicy    int `json:"namespacesWithBackupPolicy"`
	WithoutPolicy       int `json:"namespacesWithoutPolicy"`
	TotalPVCs           int `json:"totalPVCs"`
	PVCsBackedUp        int `json:"pvcsBackedUp"`
	PVCsAtRisk          int `json:"pvcsAtRisk"`
	TotalSecrets        int `json:"totalSecrets"`
	SecretsWithBackup   int `json:"secretsWithBackup"`
	TotalChecklistItems int `json:"totalChecklistItems"`
	ChecklistPassed     int `json:"checklistPassed"`
}

type BackupNamespacePolicy struct {
	Namespace  string `json:"namespace"`
	HasPolicy  bool   `json:"hasPolicy"`
	PolicyTool string `json:"policyTool"`
	Schedule   string `json:"schedule"`
	Retention  string `json:"retention"`
	RiskLevel  string `json:"riskLevel"`
}

type BackupPVCEntry struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Size        string `json:"size"`
	HasSnapshot bool   `json:"hasSnapshot"`
	RiskLevel   string `json:"riskLevel"`
}

type BackupSecretEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
	HasBackup bool   `json:"hasBackup"`
	RiskLevel string `json:"riskLevel"`
}

type BackupChecklistItem struct {
	Item   string `json:"item"`
	Status string `json:"status"`
	Detail string `json:"detail"`
}

// handleChangeImpactBrief handles GET /api/docs/change-impact-brief
// handleLabelTaxonomyStandard handles GET /api/docs/label-taxonomy-standard
// handleBackupComplianceDeep handles GET /api/docs/backup-compliance-deep
func (s *Server) handleBackupComplianceDeep(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := BackupComplianceResult{ScannedAt: time.Now()}

	// Scan namespaces for backup annotations
	namespaces, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	for _, ns := range namespaces.Items {
		if isSystemNamespace(ns.Name) {
			continue
		}
		result.Summary.TotalNamespaces++

		policy := BackupNamespacePolicy{Namespace: ns.Name}
		annotations := ns.Annotations
		if annotations == nil {
			annotations = map[string]string{}
		}

		// Check for common backup tool annotations
		backupKeys := []string{
			"backup.velero.io/backup-volumes",
			"velero.io/backup-volumes",
			"k8up.io/backup",
			"k8up.syncthing.io/schedule",
			"backup.appuio.ch/schedule",
			"stork.libopenstorage.org/snapshot-name",
		}
		for _, key := range backupKeys {
			if val, ok := annotations[key]; ok {
				policy.HasPolicy = true
				policy.PolicyTool = classifyBackupTool(key)
				policy.Schedule = val
				break
			}
		}

		// Check for backup labels
		if val, ok := ns.Labels["backup"]; ok && val != "disabled" && val != "false" {
			policy.HasPolicy = true
			if policy.PolicyTool == "" {
				policy.PolicyTool = "label-based"
			}
		}

		if policy.HasPolicy {
			result.Summary.WithBackupPolicy++
			policy.RiskLevel = "low"
		} else {
			result.Summary.WithoutPolicy++
			policy.RiskLevel = "high"
		}
		result.NamespacePolicy = append(result.NamespacePolicy, policy)
	}

	// Scan PVCs for snapshot coverage
	pvcs, _ := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	for _, pvc := range pvcListFilter(pvcs.Items) {
		if isSystemNamespace(pvc.Namespace) {
			continue
		}
		result.Summary.TotalPVCs++

		entry := BackupPVCEntry{
			Name:      pvc.Name,
			Namespace: pvc.Namespace,
			Size:      pvcSizeStr(pvc),
		}

		// Check PVC annotations for backup
		if pvc.Annotations != nil {
			if _, ok := pvc.Annotations["backup.velero.io/backup-volumes"]; ok {
				entry.HasSnapshot = true
			}
			if _, ok := pvc.Annotations["snapshot.storage.kubernetes.io/volume-snapshot"]; ok {
				entry.HasSnapshot = true
			}
		}

		// Check for VolumeSnapshot in DataSource
		if pvc.Spec.DataSource != nil && pvc.Spec.DataSource.Kind == "VolumeSnapshot" {
			entry.HasSnapshot = true
		}

		if entry.HasSnapshot {
			result.Summary.PVCsBackedUp++
			entry.RiskLevel = "low"
		} else {
			result.Summary.PVCsAtRisk++
			entry.RiskLevel = "high"
		}
		result.PVCBackupStatus = append(result.PVCBackupStatus, entry)
	}

	// Scan secrets for backup risk (non-system secrets with sensitive data)
	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	for _, secret := range secrets.Items {
		if isSystemNamespace(secret.Namespace) {
			continue
		}
		// Only check important secret types
		if secret.Type != corev1.SecretTypeOpaque &&
			secret.Type != corev1.SecretTypeDockerConfigJson &&
			!strings.HasPrefix(string(secret.Type), "kubernetes.io/") {
			continue
		}
		result.Summary.TotalSecrets++

		entry := BackupSecretEntry{
			Name:      secret.Name,
			Namespace: secret.Namespace,
			Type:      string(secret.Type),
		}

		// Heuristic: if the namespace has backup policy, assume secret is covered
		nsHasPolicy := false
		for _, np := range result.NamespacePolicy {
			if np.Namespace == secret.Namespace && np.HasPolicy {
				nsHasPolicy = true
				break
			}
		}
		entry.HasBackup = nsHasPolicy
		if nsHasPolicy {
			result.Summary.SecretsWithBackup++
			entry.RiskLevel = "low"
		} else {
			entry.RiskLevel = "high"
		}
		result.SecretBackupRisk = append(result.SecretBackupRisk, entry)
	}

	// Build checklist
	result.Checklist = buildBackupChecklist(&result.Summary)
	result.Summary.TotalChecklistItems = len(result.Checklist)
	for _, c := range result.Checklist {
		if c.Status == "pass" {
			result.Summary.ChecklistPassed++
		}
	}

	// Score
	if result.Summary.TotalChecklistItems > 0 {
		result.HealthScore = result.Summary.ChecklistPassed * 100 / result.Summary.TotalChecklistItems
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = buildBackupRecommendations(&result)
	writeJSON(w, result)
}

func classifyBackupTool(annotationKey string) string {
	if strings.Contains(annotationKey, "velero") {
		return "Velero"
	}
	if strings.Contains(annotationKey, "k8up") {
		return "K8up"
	}
	if strings.Contains(annotationKey, "appuio") {
		return "Appuio"
	}
	if strings.Contains(annotationKey, "stork") {
		return "Stork"
	}
	return "unknown"
}

func pvcListFilter(items []corev1.PersistentVolumeClaim) []corev1.PersistentVolumeClaim {
	return items
}

func pvcSizeStr(pvc corev1.PersistentVolumeClaim) string {
	if qty, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
		return qty.String()
	}
	return "unknown"
}

func buildBackupChecklist(summary *BackupComplianceSummary) []BackupChecklistItem {
	var items []BackupChecklistItem

	// Namespace backup policy
	if summary.TotalNamespaces > 0 {
		coverage := summary.WithBackupPolicy * 100 / summary.TotalNamespaces
		if coverage >= 80 {
			items = append(items, BackupChecklistItem{Item: "Namespace backup policy coverage", Status: "pass", Detail: fmt.Sprintf("%d/%d namespaces have backup policy (%d%%)", summary.WithBackupPolicy, summary.TotalNamespaces, coverage)})
		} else if coverage >= 50 {
			items = append(items, BackupChecklistItem{Item: "Namespace backup policy coverage", Status: "warn", Detail: fmt.Sprintf("%d/%d namespaces have backup policy (%d%%) - need >= 80%%", summary.WithBackupPolicy, summary.TotalNamespaces, coverage)})
		} else {
			items = append(items, BackupChecklistItem{Item: "Namespace backup policy coverage", Status: "fail", Detail: fmt.Sprintf("%d/%d namespaces have backup policy (%d%%) - critical gap", summary.WithBackupPolicy, summary.TotalNamespaces, coverage)})
		}
	} else {
		items = append(items, BackupChecklistItem{Item: "Namespace backup policy coverage", Status: "pass", Detail: "no user namespaces found"})
	}

	// PVC backup coverage
	if summary.TotalPVCs > 0 {
		coverage := summary.PVCsBackedUp * 100 / summary.TotalPVCs
		if coverage >= 80 {
			items = append(items, BackupChecklistItem{Item: "PVC snapshot/backup coverage", Status: "pass", Detail: fmt.Sprintf("%d/%d PVCs backed up (%d%%)", summary.PVCsBackedUp, summary.TotalPVCs, coverage)})
		} else if coverage >= 50 {
			items = append(items, BackupChecklistItem{Item: "PVC snapshot/backup coverage", Status: "warn", Detail: fmt.Sprintf("%d/%d PVCs backed up (%d%%)", summary.PVCsBackedUp, summary.TotalPVCs, coverage)})
		} else {
			items = append(items, BackupChecklistItem{Item: "PVC snapshot/backup coverage", Status: "fail", Detail: fmt.Sprintf("%d/%d PVCs backed up (%d%%) - data loss risk", summary.PVCsBackedUp, summary.TotalPVCs, coverage)})
		}
	} else {
		items = append(items, BackupChecklistItem{Item: "PVC snapshot/backup coverage", Status: "pass", Detail: "no PVCs found"})
	}

	// Secret backup coverage
	if summary.TotalSecrets > 0 {
		coverage := summary.SecretsWithBackup * 100 / summary.TotalSecrets
		if coverage >= 80 {
			items = append(items, BackupChecklistItem{Item: "Secret backup coverage", Status: "pass", Detail: fmt.Sprintf("%d/%d secrets covered (%d%%)", summary.SecretsWithBackup, summary.TotalSecrets, coverage)})
		} else {
			items = append(items, BackupChecklistItem{Item: "Secret backup coverage", Status: "warn", Detail: fmt.Sprintf("%d/%d secrets covered (%d%%)", summary.SecretsWithBackup, summary.TotalSecrets, coverage)})
		}
	} else {
		items = append(items, BackupChecklistItem{Item: "Secret backup coverage", Status: "pass", Detail: "no secrets found"})
	}

	// Standard checklist items (always check)
	items = append(items, BackupChecklistItem{Item: "Etcd backup configured", Status: "warn", Detail: "Verify etcd backup is running on control plane nodes"})
	items = append(items, BackupChecklistItem{Item: "Backup encryption enabled", Status: "warn", Detail: "Verify backup storage encryption is enabled"})
	items = append(items, BackupChecklistItem{Item: "Cross-region replication", Status: "warn", Detail: "Verify backup replication to secondary region"})
	items = append(items, BackupChecklistItem{Item: "Periodic restore test", Status: "warn", Detail: "Schedule periodic restore verification tests"})
	items = append(items, BackupChecklistItem{Item: "RTO/RPO documented", Status: "warn", Detail: "Document Recovery Time/Point Objectives for critical services"})

	return items
}

func buildBackupRecommendations(result *BackupComplianceResult) []string {
	recs := []string{
		fmt.Sprintf("Backup compliance: score %d/100 (%s), %d/%d checks passed",
			result.HealthScore, result.Grade, result.Summary.ChecklistPassed, result.Summary.TotalChecklistItems),
	}
	if result.Summary.WithoutPolicy > 0 {
		recs = append(recs, fmt.Sprintf("%d namespaces without backup policy - install Velero or equivalent", result.Summary.WithoutPolicy))
	}
	if result.Summary.PVCsAtRisk > 0 {
		recs = append(recs, fmt.Sprintf("%d PVCs without backup/snapshot - risk of data loss", result.Summary.PVCsAtRisk))
	}
	return recs
}

// ============================================================
// 2. Label Taxonomy Standard
// ============================================================

// LabelTaxonomyResult analyzes label usage across the cluster and proposes standardization.
type LabelTaxonomyResult struct {
	ScannedAt         time.Time            `json:"scannedAt"`
	HealthScore       int                  `json:"healthScore"`
	Grade             string               `json:"grade"`
	Summary           LabelTaxonomySummary `json:"summary"`
	RecommendedLabels []LabelStandardEntry `json:"recommendedLabels"`
	Inconsistencies   []LabelInconsistency `json:"inconsistencies"`
	TopLabels         []LabelUsageEntry    `json:"topLabels"`
	ResourceCoverage  map[string]int       `json:"resourceCoverage"`
	Recommendations   []string             `json:"recommendations"`
}

type LabelTaxonomySummary struct {
	TotalResources      int `json:"totalResources"`
	ResourcesWithLabels int `json:"resourcesWithLabels"`
	UniqueLabels        int `json:"uniqueLabels"`
	StandardLabels      int `json:"standardLabelsUsed"`
	CustomLabels        int `json:"customLabelsUsed"`
	InconsistentCount   int `json:"inconsistentLabels"`
	CoveragePercent     int `json:"labelCoveragePercent"`
}

type LabelStandardEntry struct {
	Label           string `json:"label"`
	RecommendedBy   string `json:"recommendedBy"`
	Description     string `json:"description"`
	UsageCount      int    `json:"usageCount"`
	AdoptionPercent int    `json:"adoptionPercent"`
}

type LabelInconsistency struct {
	Label      string   `json:"label"`
	Variants   []string `json:"variants"`
	Suggestion string   `json:"suggestion"`
	Count      int      `json:"count"`
}

type LabelUsageEntry struct {
	Label      string `json:"label"`
	Count      int    `json:"count"`
	IsStandard bool   `json:"isStandard"`
}

func (s *Server) handleLabelTaxonomyStandard(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := LabelTaxonomyResult{
		ScannedAt:        time.Now(),
		ResourceCoverage: map[string]int{},
	}

	// K8s recommended labels (from kubernetes labels standard)
	k8sRecommended := map[string]string{
		"app.kubernetes.io/name":       "K8s recommended label",
		"app.kubernetes.io/instance":   "K8s recommended label",
		"app.kubernetes.io/version":    "K8s recommended label",
		"app.kubernetes.io/component":  "K8s recommended label",
		"app.kubernetes.io/part-of":    "K8s recommended label",
		"app.kubernetes.io/managed-by": "K8s recommended label",
		"app.kubernetes.io/created-by": "K8s recommended label",
	}

	// Collect labels from all resources
	labelCounts := map[string]int{}                 // label -> count
	labelValuesMap := map[string]sets.Set[string]{} // label -> set of distinct values
	totalResources := 0
	resourcesWithLabels := 0

	// Helper to extract labels
	extractLabels := func(labels map[string]string, resourceType string) {
		totalResources++
		result.ResourceCoverage[resourceType]++
		if len(labels) == 0 {
			return
		}
		resourcesWithLabels++
		for k, v := range labels {
			labelCounts[k]++
			if labelValuesMap[k] == nil {
				labelValuesMap[k] = sets.New[string]()
			}
			labelValuesMap[k].Insert(v)
		}
	}

	// Deployments
	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, item := range deps.Items {
		if isSystemNamespace(item.Namespace) {
			continue
		}
		extractLabels(item.Labels, "Deployment")
	}

	// Services
	svcs, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	for _, item := range svcs.Items {
		if isSystemNamespace(item.Namespace) {
			continue
		}
		extractLabels(item.Labels, "Service")
	}

	// Pods
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	for _, item := range pods.Items {
		if isSystemNamespace(item.Namespace) {
			continue
		}
		extractLabels(item.Labels, "Pod")
	}

	// Namespaces
	nss, _ := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	for _, item := range nss.Items {
		if isSystemNamespace(item.Name) {
			continue
		}
		extractLabels(item.Labels, "Namespace")
	}

	// ConfigMaps
	cms, _ := rc.clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{})
	for _, item := range cms.Items {
		if isSystemNamespace(item.Namespace) {
			continue
		}
		extractLabels(item.Labels, "ConfigMap")
	}

	// Calculate statistics
	standardLabelsUsed := 0
	customLabelsUsed := 0
	for label := range labelCounts {
		if _, ok := k8sRecommended[label]; ok {
			standardLabelsUsed++
		} else if strings.HasPrefix(label, "app.kubernetes.io/") {
			standardLabelsUsed++
		} else {
			customLabelsUsed++
		}
	}

	result.Summary = LabelTaxonomySummary{
		TotalResources:      totalResources,
		ResourcesWithLabels: resourcesWithLabels,
		UniqueLabels:        len(labelCounts),
		StandardLabels:      standardLabelsUsed,
		CustomLabels:        customLabelsUsed,
	}
	if totalResources > 0 {
		result.Summary.CoveragePercent = resourcesWithLabels * 100 / totalResources
	}

	// Top labels by usage
	for label, count := range labelCounts {
		_, isStandard := k8sRecommended[label]
		if !isStandard && strings.HasPrefix(label, "app.kubernetes.io/") {
			isStandard = true
		}
		result.TopLabels = append(result.TopLabels, LabelUsageEntry{
			Label:      label,
			Count:      count,
			IsStandard: isStandard,
		})
	}
	sort.Slice(result.TopLabels, func(i, j int) bool {
		return result.TopLabels[i].Count > result.TopLabels[j].Count
	})
	if len(result.TopLabels) > 30 {
		result.TopLabels = result.TopLabels[:30]
	}

	// Recommended labels adoption
	for label, desc := range k8sRecommended {
		count := labelCounts[label]
		var pct int
		if totalResources > 0 {
			pct = count * 100 / totalResources
		}
		result.RecommendedLabels = append(result.RecommendedLabels, LabelStandardEntry{
			Label:           label,
			RecommendedBy:   desc,
			Description:     getLabelDescription(label),
			UsageCount:      count,
			AdoptionPercent: pct,
		})
	}
	sort.Slice(result.RecommendedLabels, func(i, j int) bool {
		return result.RecommendedLabels[i].UsageCount > result.RecommendedLabels[j].UsageCount
	})

	// Detect inconsistencies (case sensitivity, similar names, deprecated prefixes)
	result.Inconsistencies = detectLabelInconsistencies(labelCounts)
	result.Summary.InconsistentCount = len(result.Inconsistencies)

	// Score: based on coverage + standard adoption
	coverageScore := result.Summary.CoveragePercent
	standardScore := 0
	if (standardLabelsUsed + customLabelsUsed) > 0 {
		standardScore = standardLabelsUsed * 100 / (standardLabelsUsed + customLabelsUsed)
	}
	result.HealthScore = (coverageScore + standardScore) / 2
	// Penalty for inconsistencies
	if result.Summary.InconsistentCount > 5 {
		result.HealthScore -= 10
	}
	if result.HealthScore < 0 {
		result.HealthScore = 0
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = buildLabelTaxonomyRecommendations(&result)
	writeJSON(w, result)
}

func getLabelDescription(label string) string {
	descs := map[string]string{
		"app.kubernetes.io/name":       "The name of the application",
		"app.kubernetes.io/instance":   "A unique name identifying the instance of an application",
		"app.kubernetes.io/version":    "The current version of the application",
		"app.kubernetes.io/component":  "The component within the architecture",
		"app.kubernetes.io/part-of":    "The name of a higher level application this one is part of",
		"app.kubernetes.io/managed-by": "The tool being used to manage the operation of an application",
		"app.kubernetes.io/created-by": "The controller/user who created this resource",
	}
	if d, ok := descs[label]; ok {
		return d
	}
	return ""
}

func detectLabelInconsistencies(labelCounts map[string]int) []LabelInconsistency {
	var inconsistencies []LabelInconsistency
	allLabels := make([]string, 0, len(labelCounts))
	for k := range labelCounts {
		allLabels = append(allLabels, k)
	}

	// Group by normalized form
	groups := map[string][]string{}
	for _, label := range allLabels {
		// Normalize: lowercase, remove separators
		normalized := strings.ToLower(label)
		normalized = strings.ReplaceAll(normalized, "_", "-")
		normalized = strings.ReplaceAll(normalized, "/", "-")
		normalized = strings.ReplaceAll(normalized, ".", "-")
		// Also check without prefix
		if idx := strings.LastIndex(normalized, "-"); idx >= 0 {
			normalized = normalized[idx+1:]
		}
		groups[normalized] = append(groups[normalized], label)
	}

	for normalized, variants := range groups {
		if len(variants) > 1 {
			// Sort variants for deterministic output
			sort.Strings(variants)
			// Pick the recommended canonical form
			canonical := variants[0]
			for _, v := range variants {
				if strings.HasPrefix(v, "app.kubernetes.io/") {
					canonical = v
					break
				}
			}
			totalCount := 0
			for _, v := range variants {
				totalCount += labelCounts[v]
			}
			inconsistencies = append(inconsistencies, LabelInconsistency{
				Label:      normalized,
				Variants:   variants,
				Suggestion: fmt.Sprintf("Use consistent label: '%s'", canonical),
				Count:      totalCount,
			})
		}
	}

	sort.Slice(inconsistencies, func(i, j int) bool {
		return inconsistencies[i].Count > inconsistencies[j].Count
	})

	if len(inconsistencies) > 15 {
		inconsistencies = inconsistencies[:15]
	}
	return inconsistencies
}

func buildLabelTaxonomyRecommendations(result *LabelTaxonomyResult) []string {
	recs := []string{
		fmt.Sprintf("Label taxonomy health: score %d/100 (%s), %d/%d resources have labels",
			result.HealthScore, result.Grade, result.Summary.ResourcesWithLabels, result.Summary.TotalResources),
	}
	lowAdoption := 0
	for _, rl := range result.RecommendedLabels {
		if rl.AdoptionPercent < 50 {
			lowAdoption++
		}
	}
	if lowAdoption > 0 {
		recs = append(recs, fmt.Sprintf("%d recommended K8s labels have < 50%% adoption - apply app.kubernetes.io/* labels for better tooling compatibility", lowAdoption))
	}
	if result.Summary.InconsistentCount > 0 {
		recs = append(recs, fmt.Sprintf("%d label inconsistencies detected - standardize naming for better filtering and querying", result.Summary.InconsistentCount))
	}
	if result.Summary.CoveragePercent < 50 {
		recs = append(recs, fmt.Sprintf("Only %d%% of resources have labels - add labels for better resource management", result.Summary.CoveragePercent))
	}
	return recs
}

// ============================================================
// 3. Change Impact Brief
// ============================================================

// ChangeImpactResult provides a structured change impact assessment.
type ChangeImpactResult struct {
	ScannedAt         time.Time                `json:"scannedAt"`
	HealthScore       int                      `json:"healthScore"`
	Grade             string                   `json:"grade"`
	Summary           ChangeImpactSummary      `json:"summary"`
	RecentChanges     []ChangeLogEntry         `json:"recentChanges"`
	BlastRadius       []ChangeBlastRadiusEntry `json:"blastRadius"`
	RollbackReadiness []RollbackReadinessEntry `json:"rollbackReadiness"`
	RiskAreas         []RiskAreaEntry          `json:"riskAreas"`
	Recommendations   []string                 `json:"recommendations"`
}

type ChangeImpactSummary struct {
	TotalRecentChanges int `json:"totalRecentChanges"`
	CriticalChanges    int `json:"criticalChanges"`
	HighRiskChanges    int `json:"highRiskChanges"`
	ResourcesAtRisk    int `json:"resourcesAtRisk"`
	NamespacesAffected int `json:"namespacesAffected"`
	WithRollbackPlan   int `json:"withRollbackPlan"`
	WithoutRollback    int `json:"withoutRollback"`
}

type ChangeLogEntry struct {
	Kind             string   `json:"resourceType"`
	Name             string   `json:"name"`
	Namespace        string   `json:"namespace"`
	ChangeType       string   `json:"changeType"`
	Reason           string   `json:"reason"`
	Timestamp        string   `json:"timestamp"`
	AffectedServices []string `json:"affectedServices,omitempty"`
	RiskLevel        string   `json:"riskLevel"`
}

type ChangeBlastRadiusEntry struct {
	Namespace    string   `json:"namespace"`
	Kind         string   `json:"kind"`
	Resource     string   `json:"resource"`
	Impact       string   `json:"impact"`
	PodsAffected int      `json:"podsAffected"`
	Services     []string `json:"services"`
}

type RollbackReadinessEntry struct {
	ResourceType    string `json:"resourceType"`
	Name            string `json:"name"`
	Namespace       string `json:"namespace"`
	RevisionHistory int    `json:"revisionHistory"`
	HasPDB          bool   `json:"hasPDB"`
	RollbackReady   bool   `json:"rollbackReady"`
	Notes           string `json:"notes"`
}

type RiskAreaEntry struct {
	Area     string `json:"area"`
	Severity string `json:"severity"`
	Detail   string `json:"detail"`
	Count    int    `json:"count"`
}

func (s *Server) handleChangeImpactBrief(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := ChangeImpactResult{ScannedAt: time.Now()}

	now := time.Now()
	cutoff := now.Add(-72 * time.Hour) // Last 72 hours
	affectedNs := sets.New[string]()

	// Analyze recent events for changes
	events, _ := rc.clientset.CoreV1().Events("").List(ctx, metav1.ListOptions{})
	for _, evt := range events.Items {
		if evt.LastTimestamp.IsZero() || evt.LastTimestamp.Time.Before(cutoff) {
			continue
		}
		if isSystemNamespace(evt.Namespace) {
			continue
		}

		// Filter for meaningful change events
		reason := evt.Reason
		if !isChangeEvent(reason, evt.Message) {
			continue
		}

		result.Summary.TotalRecentChanges++
		affectedNs.Insert(evt.Namespace)

		entry := ChangeLogEntry{
			Kind:       evt.InvolvedObject.Kind,
			Name:       evt.InvolvedObject.Name,
			Namespace:  evt.Namespace,
			ChangeType: classifyChangeEvent(reason, evt.Message),
			Reason:     reason,
			Timestamp:  evt.LastTimestamp.Format(time.RFC3339),
		}

		entry.RiskLevel = classifyChangeRisk(reason, evt.Message)
		switch entry.RiskLevel {
		case "critical":
			result.Summary.CriticalChanges++
		case "high":
			result.Summary.HighRiskChanges++
		}

		result.RecentChanges = append(result.RecentChanges, entry)
	}
	result.Summary.NamespacesAffected = len(affectedNs)

	// Limit to top 50 changes by time
	sort.Slice(result.RecentChanges, func(i, j int) bool {
		return result.RecentChanges[i].Timestamp > result.RecentChanges[j].Timestamp
	})
	if len(result.RecentChanges) > 50 {
		result.RecentChanges = result.RecentChanges[:50]
	}

	// Analyze blast radius for deployments
	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})
	pdbMap := map[string]bool{}
	for _, pdb := range pdbs.Items {
		key := pdb.Namespace + "/" + pdb.Name
		pdbMap[key] = true
		// Also check by selector match
	}

	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}

		// Blast radius
		pods, _ := rc.clientset.CoreV1().Pods(dep.Namespace).List(ctx, metav1.ListOptions{
			LabelSelector: metav1.FormatLabelSelector(dep.Spec.Selector),
		})
		if len(pods.Items) > 0 {
			br := ChangeBlastRadiusEntry{
				Namespace:    dep.Namespace,
				Kind:         "Deployment",
				Resource:     dep.Name,
				PodsAffected: len(pods.Items),
				Services:     findDependentServicesForChange(ctx, rc.clientset, dep.Namespace, dep.Labels),
			}
			replicas := int32(1)
			if dep.Spec.Replicas != nil {
				replicas = *dep.Spec.Replicas
			}
			if replicas >= 10 {
				br.Impact = "high - large deployment, potential widespread outage"
				result.Summary.ResourcesAtRisk++
			} else if replicas >= 3 {
				br.Impact = "medium - moderate blast radius"
			} else {
				br.Impact = "low - small deployment"
			}
			result.BlastRadius = append(result.BlastRadius, br)
		}

		// Rollback readiness
		historyLimit := int32(10)
		if dep.Spec.RevisionHistoryLimit != nil {
			historyLimit = *dep.Spec.RevisionHistoryLimit
		}
		hasPDB := false
		for _, pdb := range pdbs.Items {
			if pdb.Namespace == dep.Namespace && matchLabelsSelector(dep.Spec.Selector, pdb.Spec.Selector) {
				hasPDB = true
				break
			}
		}

		rollbackReady := historyLimit > 0 && hasPDB
		notes := ""
		if historyLimit == 0 {
			notes = "no revision history - rollback impossible"
		} else if !hasPDB {
			notes = "no PDB - rolling back may cause disruption"
		} else {
			notes = "rollback ready - has history and PDB"
		}

		if rollbackReady {
			result.Summary.WithRollbackPlan++
		} else {
			result.Summary.WithoutRollback++
		}

		result.RollbackReadiness = append(result.RollbackReadiness, RollbackReadinessEntry{
			ResourceType:    "Deployment",
			Name:            dep.Name,
			Namespace:       dep.Namespace,
			RevisionHistory: int(historyLimit),
			HasPDB:          hasPDB,
			RollbackReady:   rollbackReady,
			Notes:           notes,
		})
	}

	// Identify risk areas
	result.RiskAreas = identifyChangeRiskAreas(&result.Summary)

	// Score based on rollback readiness and critical changes
	if result.Summary.WithRollbackPlan+result.Summary.WithoutRollback > 0 {
		result.HealthScore = result.Summary.WithRollbackPlan * 100 / (result.Summary.WithRollbackPlan + result.Summary.WithoutRollback)
	} else {
		result.HealthScore = 100
	}
	// Penalty for critical changes
	if result.Summary.CriticalChanges > 0 {
		result.HealthScore -= 10
	}
	if result.HealthScore < 0 {
		result.HealthScore = 0
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = buildChangeImpactRecommendations(&result)
	writeJSON(w, result)
}

func isChangeEvent(reason, msg string) bool {
	changeReasons := []string{
		"Scaled", "ScalingReplicaSet", "SuccessfulCreate", "SuccessfulDelete",
		"Updated", "Created", "Deleted", "UpdatedLoadBalancer",
		"ImageUpdated", "RollingUpdate", "BackOff", "Unhealthy",
	}
	changeMsgKeywords := []string{
		"scaled", "updated", "created", "deleted", "image updated",
		"rolling update", "rollout", "configmap", "secret",
	}
	reasonLower := strings.ToLower(reason)
	for _, cr := range changeReasons {
		if strings.Contains(reasonLower, strings.ToLower(cr)) {
			return true
		}
	}
	msgLower := strings.ToLower(msg)
	for _, kw := range changeMsgKeywords {
		if strings.Contains(msgLower, kw) {
			return true
		}
	}
	return false
}

func classifyChangeEvent(reason, msg string) string {
	r := strings.ToLower(reason)
	m := strings.ToLower(msg)
	switch {
	case strings.Contains(r, "scale") || strings.Contains(m, "scaled"):
		return "scale"
	case strings.Contains(r, "image") || strings.Contains(m, "image updated"):
		return "image-update"
	case strings.Contains(r, "rollout") || strings.Contains(r, "rolling"):
		return "rollout"
	case strings.Contains(r, "create") || strings.Contains(r, "created"):
		return "create"
	case strings.Contains(r, "delete") || strings.Contains(r, "deleted"):
		return "delete"
	case strings.Contains(r, "configmap") || strings.Contains(r, "secret"):
		return "config-change"
	default:
		return "other"
	}
}

func classifyChangeRisk(reason, msg string) string {
	r := strings.ToLower(reason)
	m := strings.ToLower(msg)
	switch {
	case strings.Contains(m, "crashloop") || strings.Contains(m, "backoff"):
		return "critical"
	case strings.Contains(r, "image") && strings.Contains(m, "updated"):
		return "high"
	case strings.Contains(r, "scale") && strings.Contains(m, "down"):
		return "medium"
	case strings.Contains(r, "configmap") || strings.Contains(r, "secret"):
		return "high"
	default:
		return "low"
	}
}

func findDependentServicesForChange(ctx context.Context, clientset kubernetes.Interface, ns string, labels map[string]string) []string {
	// Return services in the same namespace that match the deployment labels
	if len(labels) == 0 || clientset == nil {
		return []string{}
	}
	svcList, err := clientset.CoreV1().Services(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		return []string{}
	}
	var result []string
	for _, svc := range svcList.Items {
		if svc.Spec.Selector == nil {
			continue
		}
		matched := true
		for k, v := range svc.Spec.Selector {
			if labels[k] != v {
				matched = false
				break
			}
		}
		if matched {
			result = append(result, svc.Name)
		}
	}
	return result
}

func matchLabelsSelector(depSel *metav1.LabelSelector, pdbSel *metav1.LabelSelector) bool {
	if depSel == nil || pdbSel == nil {
		return false
	}
	// Simple check: if any match label key exists in both
	for k := range depSel.MatchLabels {
		if _, ok := pdbSel.MatchLabels[k]; ok {
			return true
		}
	}
	return false
}

func identifyChangeRiskAreas(summary *ChangeImpactSummary) []RiskAreaEntry {
	var areas []RiskAreaEntry

	if summary.WithoutRollback > 0 {
		areas = append(areas, RiskAreaEntry{
			Area:     "Rollback Readiness",
			Severity: "high",
			Detail:   "Deployments without revision history or PDB cannot be safely rolled back",
			Count:    summary.WithoutRollback,
		})
	}
	if summary.CriticalChanges > 0 {
		areas = append(areas, RiskAreaEntry{
			Area:     "Critical Changes",
			Severity: "critical",
			Detail:   "Recent changes detected with crash loop or backoff patterns",
			Count:    summary.CriticalChanges,
		})
	}
	if summary.HighRiskChanges > 0 {
		areas = append(areas, RiskAreaEntry{
			Area:     "High Risk Changes",
			Severity: "high",
			Detail:   "Image updates and configuration changes without verification",
			Count:    summary.HighRiskChanges,
		})
	}
	if summary.NamespacesAffected > 3 {
		areas = append(areas, RiskAreaEntry{
			Area:     "Wide Impact",
			Severity: "medium",
			Detail:   "Changes spread across many namespaces - potential uncoordinated deployment",
			Count:    summary.NamespacesAffected,
		})
	}
	return areas
}

func buildChangeImpactRecommendations(result *ChangeImpactResult) []string {
	recs := []string{
		fmt.Sprintf("Change impact readiness: score %d/100 (%s), %d recent changes, %d rollback-ready deployments",
			result.HealthScore, result.Grade, result.Summary.TotalRecentChanges, result.Summary.WithRollbackPlan),
	}
	if result.Summary.WithoutRollback > 0 {
		recs = append(recs, fmt.Sprintf("%d deployments lack rollback readiness (revision history or PDB) - enable revision history and add PDBs", result.Summary.WithoutRollback))
	}
	if result.Summary.CriticalChanges > 0 {
		recs = append(recs, fmt.Sprintf("%d critical changes detected in last 72h - investigate crash loop patterns", result.Summary.CriticalChanges))
	}
	if len(result.RiskAreas) == 0 {
		recs = append(recs, "No significant change risk areas detected")
	}
	return recs
}
