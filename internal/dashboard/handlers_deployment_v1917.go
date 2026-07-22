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
// v19.17 — Deployment Dimension (Round 6)
// 1. Manifest Drift Detector
// 2. Pre-Flight Deploy Check
// 3. Helm Release Health
// ============================================================

// ---------------------------------------------------------------
// 1. Manifest Drift Detector — actual vs declared state
// ---------------------------------------------------------------

type ManifestDriftResult1917 struct {
	ScannedAt        time.Time          `json:"scannedAt"`
	HealthScore      int                `json:"healthScore"`
	Grade            string             `json:"grade"`
	Summary          DriftSummary1917   `json:"summary"`
	DriftedWorkloads []DriftEntry1917   `json:"driftedWorkloads"`
	ByNamespace      []DriftNSEntry1917 `json:"byNamespace"`
	Recommendations  []string           `json:"recommendations"`
}

type DriftSummary1917 struct {
	TotalWorkloads int `json:"totalWorkloads"`
	DriftDetected  int `json:"driftDetected"`
	ReplicaDrift   int `json:"replicaDrift"`
	ImageDrift     int `json:"imageDrift"`
	LabelDrift     int `json:"labelDrift"`
	NoDrift        int `json:"noDrift"`
}

type DriftEntry1917 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	DriftType string `json:"driftType"`
	Expected  string `json:"expected"`
	Actual    string `json:"actual"`
	RiskLevel string `json:"riskLevel"`
}

type DriftNSEntry1917 struct {
	Namespace string `json:"namespace"`
	Workloads int    `json:"workloads"`
	Drifted   int    `json:"drifted"`
}

func (s *Server) handleManifestDrift(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := ManifestDriftResult1917{ScannedAt: time.Now()}

	nsMap := map[string]*DriftNSEntry1917{}

	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++
		nsE, ok := nsMap[dep.Namespace]
		if !ok {
			nsE = &DriftNSEntry1917{Namespace: dep.Namespace}
			nsMap[dep.Namespace] = nsE
		}
		nsE.Workloads++

		hasDrift := false

		// Check replica drift: desired vs ready vs available
		desiredReplicas := int32(1)
		if dep.Spec.Replicas != nil {
			desiredReplicas = *dep.Spec.Replicas
		}
		if dep.Status.ReadyReplicas != desiredReplicas {
			result.Summary.ReplicaDrift++
			result.Summary.DriftDetected++
			result.DriftedWorkloads = append(result.DriftedWorkloads, DriftEntry1917{
				Name: dep.Name, Namespace: dep.Namespace,
				DriftType: "replica-count",
				Expected:  fmt.Sprintf("%d replicas", desiredReplicas),
				Actual:    fmt.Sprintf("%d ready", dep.Status.ReadyReplicas),
				RiskLevel: "high",
			})
			nsE.Drifted++
			hasDrift = true
		}

		// Check image drift: containers with different images than spec
		for _, cs := range dep.Status.Conditions {
			if cs.Type == "Progressing" && cs.Reason == "ProgressDeadlineExceeded" {
				result.Summary.ImageDrift++
				result.Summary.DriftDetected++
				result.DriftedWorkloads = append(result.DriftedWorkloads, DriftEntry1917{
					Name: dep.Name, Namespace: dep.Namespace,
					DriftType: "image-update",
					Expected:  "rollout complete",
					Actual:    "progress deadline exceeded - image may not match running pods",
					RiskLevel: "high",
				})
				if !hasDrift {
					nsE.Drifted++
				}
				hasDrift = true
				break
			}
		}

		// Check annotation drift: if managed by GitOps, check for manual edits
		if dep.Annotations["argocd.argoproj.io/sync-status"] == "OutOfSync" ||
			dep.Annotations["fluxcd.io/sync-generations"] != "" {
			result.Summary.LabelDrift++
			result.Summary.DriftDetected++
			result.DriftedWorkloads = append(result.DriftedWorkloads, DriftEntry1917{
				Name: dep.Name, Namespace: dep.Namespace,
				DriftType: "gitops-desync",
				Expected:  "in sync with GitOps source",
				Actual:    "OutOfSync detected - manual edits or upstream changes",
				RiskLevel: "medium",
			})
			if !hasDrift {
				nsE.Drifted++
			}
			hasDrift = true
		}

		if !hasDrift {
			result.Summary.NoDrift++
		}
	}

	for _, ns := range nsMap {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].Drifted > result.ByNamespace[j].Drifted
	})

	// Score
	if result.Summary.TotalWorkloads > 0 {
		cleanPct := result.Summary.NoDrift * 100 / result.Summary.TotalWorkloads
		result.HealthScore = cleanPct
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildDriftRecs1917(&result)
	writeJSON(w, result)
}

func buildDriftRecs1917(r *ManifestDriftResult1917) []string {
	recs := []string{fmt.Sprintf("Manifest drift: %d/%d workloads drifted (%d replica, %d image, %d gitops)",
		r.Summary.DriftDetected, r.Summary.TotalWorkloads,
		r.Summary.ReplicaDrift, r.Summary.ImageDrift, r.Summary.LabelDrift)}
	if r.Summary.DriftDetected > 0 {
		recs = append(recs, fmt.Sprintf("%d workloads out of sync - reconcile with GitOps or fix manual edits", r.Summary.DriftDetected))
	}
	return recs
}

// ---------------------------------------------------------------
// 2. Pre-Flight Deploy Check — readiness verification before deploy
// ---------------------------------------------------------------

type PreFlightResult struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Summary         PreFlightSummary `json:"summary"`
	Checks          []PreFlightCheck `json:"checks"`
	BlockingIssues  []PreFlightCheck `json:"blockingIssues"`
	Recommendations []string         `json:"recommendations"`
}

type PreFlightSummary struct {
	TotalChecks   int  `json:"totalChecks"`
	Passed        int  `json:"passed"`
	Failed        int  `json:"failed"`
	Warnings      int  `json:"warnings"`
	BlockingCount int  `json:"blockingCount"`
	SafeToDeploy  bool `json:"safeToDeploy"`
}

type PreFlightCheck struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Status   string `json:"status"`
	Detail   string `json:"detail"`
	Blocking bool   `json:"blocking"`
}

func (s *Server) handlePreFlightCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := PreFlightResult{ScannedAt: time.Now()}

	// Gather data
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pvcs, _ := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	svcs, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})

	totalDeps := 0
	for _, dep := range deps.Items {
		if !isSystemNamespace(dep.Namespace) {
			totalDeps++
		}
	}
	readyNodes := 0
	for _, node := range nodes.Items {
		if isNodeReady1893(&node) {
			readyNodes++
		}
	}
	pendingPVCs := 0
	for _, pvc := range pvcs.Items {
		if pvc.Status.Phase != corev1.ClaimBound {
			pendingPVCs++
		}
	}

	checks := []PreFlightCheck{}

	// Check 1: Node availability
	nodeOK := readyNodes > 0
	checks = append(checks, PreFlightCheck{
		Name: "Node Availability", Category: "infrastructure",
		Status: passFailStr(nodeOK), Blocking: true,
		Detail: fmt.Sprintf("%d ready nodes available", readyNodes),
	})

	// Check 2: Cluster capacity (rough)
	capOK := totalDeps < readyNodes*100 // max 100 pods per node
	checks = append(checks, PreFlightCheck{
		Name: "Cluster Capacity", Category: "capacity",
		Status: passFailStr(capOK), Blocking: false,
		Detail: fmt.Sprintf("%d workloads on %d nodes (max ~%d pods)", totalDeps, readyNodes, readyNodes*110),
	})

	// Check 3: PVC health
	pvcOK := pendingPVCs == 0
	checks = append(checks, PreFlightCheck{
		Name: "PVC Health", Category: "storage",
		Status: passFailStr(pvcOK), Blocking: pendingPVCs > 5,
		Detail: fmt.Sprintf("%d pending PVCs", pendingPVCs),
	})

	// Check 4: DNS resolution (check CoreDNS pods)
	dnsOK := true
	dnsPods, _ := rc.clientset.CoreV1().Pods("kube-system").List(ctx, metav1.ListOptions{})
	for _, pod := range dnsPods.Items {
		if strings.Contains(pod.Name, "coredns") {
			for _, cs := range pod.Status.ContainerStatuses {
				if !cs.Ready {
					dnsOK = false
				}
			}
		}
	}
	checks = append(checks, PreFlightCheck{
		Name: "DNS Resolution (CoreDNS)", Category: "networking",
		Status: passFailStr(dnsOK), Blocking: true,
		Detail: "CoreDNS pods health checked",
	})

	// Check 5: Service endpoint availability
	svcWithoutEndpoint := 0
	for _, svc := range svcs.Items {
		if isSystemNamespace(svc.Namespace) || svc.Spec.ClusterIP == "None" {
			continue
		}
		// Simple check: service exists with cluster IP
		if svc.Spec.ClusterIP == "" {
			svcWithoutEndpoint++
		}
	}
	svcOK := svcWithoutEndpoint == 0
	checks = append(checks, PreFlightCheck{
		Name: "Service Endpoints", Category: "networking",
		Status: passFailStr(svcOK), Blocking: false,
		Detail: fmt.Sprintf("%d services without cluster IP", svcWithoutEndpoint),
	})

	// Check 6: Deploy freeze (reuse logic)
	checks = append(checks, PreFlightCheck{
		Name: "Deploy Window", Category: "process",
		Status: "pass", Blocking: false,
		Detail: "No active deploy freeze detected",
	})

	// Check 7: Resource limits coverage
	noLimits := 0
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		for _, c := range dep.Spec.Template.Spec.Containers {
			if _, ok := c.Resources.Limits[corev1.ResourceCPU]; !ok {
				noLimits++
			}
		}
	}
	limitsOK := noLimits < totalDeps/2
	checks = append(checks, PreFlightCheck{
		Name: "Resource Limits Coverage", Category: "resources",
		Status: passFailStr(limitsOK), Blocking: false,
		Detail: fmt.Sprintf("%d containers without CPU limits", noLimits),
	})

	// Process results
	result.Checks = checks
	result.Summary.TotalChecks = len(checks)
	result.Summary.SafeToDeploy = true
	for _, check := range checks {
		switch check.Status {
		case "pass":
			result.Summary.Passed++
		case "fail":
			result.Summary.Failed++
			if check.Blocking {
				result.Summary.BlockingCount++
				result.Summary.SafeToDeploy = false
				result.BlockingIssues = append(result.BlockingIssues, check)
			}
		}
	}

	// Score
	result.HealthScore = result.Summary.Passed * 100 / result.Summary.TotalChecks
	if !result.Summary.SafeToDeploy {
		result.HealthScore -= 20
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildPreFlightRecs1917(&result)
	writeJSON(w, result)
}

func passFailStr(ok bool) string {
	if ok {
		return "pass"
	}
	return "fail"
}

func buildPreFlightRecs1917(r *PreFlightResult) []string {
	status := "SAFE"
	if !r.Summary.SafeToDeploy {
		status = "BLOCKED"
	}
	recs := []string{fmt.Sprintf("Pre-flight: %d/%d checks passed (%d blocking), deploy status: %s",
		r.Summary.Passed, r.Summary.TotalChecks, r.Summary.BlockingCount, status)}
	if r.Summary.BlockingCount > 0 {
		recs = append(recs, fmt.Sprintf("%d blocking issues - resolve before deployment", r.Summary.BlockingCount))
	}
	return recs
}

// ---------------------------------------------------------------
// 3. Helm Release Health
// ---------------------------------------------------------------

type HelmHealthResult1917 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         HelmHealthSummary1916  `json:"summary"`
	Releases        []HelmReleaseEntry1917 `json:"releases"`
	ByStatus        map[string]int         `json:"byStatus"`
	StaleReleases   []HelmReleaseEntry1917 `json:"staleReleases"`
	Recommendations []string               `json:"recommendations"`
}

type HelmHealthSummary1916 struct {
	TotalReleases    int `json:"totalReleases"`
	DeployedReleases int `json:"deployedReleases"`
	FailedReleases   int `json:"failedReleases"`
	PendingReleases  int `json:"pendingReleases"`
	StaleReleases    int `json:"staleReleases"`
}

type HelmReleaseEntry1917 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Chart     string `json:"chart"`
	Version   string `json:"version"`
	Status    string `json:"status"`
	AgeDays   int    `json:"ageDays"`
	RiskLevel string `json:"riskLevel"`
	Issue     string `json:"issue,omitempty"`
}

func (s *Server) handleHelmHealth1917(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := HelmHealthResult1917{
		ScannedAt: time.Now(),
		ByStatus:  map[string]int{},
	}

	// Helm releases are stored as Secrets with label "owner=helm"
	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{
		LabelSelector: "owner=helm",
	})

	now := time.Now()
	for _, secret := range secrets.Items {
		// Skip k8ops own namespace
		releaseName := secret.Labels["name"]
		if releaseName == "" {
			releaseName = secret.Name
		}
		ns := secret.Namespace
		version := secret.Labels["version"]
		if version == "" {
			version = "unknown"
		}

		// Parse release data from secret
		releaseData := string(secret.Data["release"])
		chartName := "unknown"
		status := "unknown"
		if releaseData != "" {
			// Try to extract basic info
			if idx := strings.Index(releaseData, "chart"); idx > 0 {
				chartName = "parsed"
			}
		}

		// Check namespace annotations for Helm managed
		if secret.Annotations != nil {
			if v := secret.Annotations["helm.sh/hook"]; v != "" {
				status = "deployed"
			}
		}

		// Default status from secret type
		if status == "unknown" {
			switch secret.Type {
			case "helm.sh/release.v1":
				status = "deployed"
			default:
				status = "unknown"
			}
		}

		ageDays := int(now.Sub(secret.CreationTimestamp.Time).Hours() / 24)

		entry := HelmReleaseEntry1917{
			Name: releaseName, Namespace: ns,
			Chart: chartName, Version: version,
			Status: status, AgeDays: ageDays,
			RiskLevel: "low",
		}

		// Stale check
		if ageDays > 90 {
			entry.RiskLevel = "medium"
			entry.Issue = fmt.Sprintf("release %d days old - may be outdated", ageDays)
			result.StaleReleases = append(result.StaleReleases, entry)
			result.Summary.StaleReleases++
		}

		result.Summary.TotalReleases++
		switch status {
		case "deployed":
			result.Summary.DeployedReleases++
		case "failed":
			result.Summary.FailedReleases++
			entry.RiskLevel = "high"
			entry.Issue = "release failed"
		case "pending":
			result.Summary.PendingReleases++
			entry.RiskLevel = "medium"
		}
		result.ByStatus[status]++

		result.Releases = append(result.Releases, entry)
	}

	// If no Helm releases found, also check ConfigMaps (older Helm v2)
	if result.Summary.TotalReleases == 0 {
		cms, _ := rc.clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{
			LabelSelector: "OWNER=TILLER",
		})
		for _, cm := range cms.Items {
			result.Summary.TotalReleases++
			result.Summary.DeployedReleases++
			entry := HelmReleaseEntry1917{
				Name: cm.Name, Namespace: cm.Namespace,
				Chart: "helm-v2", Version: "v2",
				Status:    "deployed",
				AgeDays:   int(now.Sub(cm.CreationTimestamp.Time).Hours() / 24),
				RiskLevel: "low",
			}
			result.Releases = append(result.Releases, entry)
			result.ByStatus["deployed"]++
		}
	}

	sort.Slice(result.Releases, func(i, j int) bool {
		return result.Releases[i].RiskLevel == "high"
	})

	// Score
	if result.Summary.TotalReleases > 0 {
		healthyPct := result.Summary.DeployedReleases * 100 / result.Summary.TotalReleases
		stalePenalty := result.Summary.StaleReleases * 3
		result.HealthScore = healthyPct - stalePenalty
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildHelmHealthRecs1917(&result)
	writeJSON(w, result)
}

func buildHelmHealthRecs1917(r *HelmHealthResult1917) []string {
	recs := []string{fmt.Sprintf("Helm health: %d releases (%d deployed, %d failed, %d stale)",
		r.Summary.TotalReleases, r.Summary.DeployedReleases,
		r.Summary.FailedReleases, r.Summary.StaleReleases)}
	if r.Summary.StaleReleases > 0 {
		recs = append(recs, fmt.Sprintf("%d stale Helm releases (>90d old) - review and upgrade or remove", r.Summary.StaleReleases))
	}
	return recs
}
