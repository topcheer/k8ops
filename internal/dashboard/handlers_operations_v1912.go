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
// v19.12 — Operations Dimension (Round 5)
// 1. Control Plane Health
// 2. CSI Driver Health
// 3. Cert Renewal Timeline
// ============================================================

// ---------------------------------------------------------------
// 1. Control Plane Health — scheduler/controller-manager/api-server
// ---------------------------------------------------------------

type ControlPlaneResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         ControlPlaneSummary `json:"summary"`
	Components      []ControlPlaneComp  `json:"components"`
	Issues          []ControlPlaneComp  `json:"issues"`
	Recommendations []string            `json:"recommendations"`
}

type ControlPlaneSummary struct {
	TotalComponents int `json:"totalComponents"`
	Healthy         int `json:"healthy"`
	Degraded        int `json:"degraded"`
	Unhealthy       int `json:"unhealthy"`
	TotalReplicas   int `json:"totalReplicas"`
	ReadyReplicas   int `json:"readyReplicas"`
}

type ControlPlaneComp struct {
	Name          string `json:"name"`
	Namespace     string `json:"namespace"`
	Kind          string `json:"kind"`
	Ready         string `json:"ready"`
	Replicas      int32  `json:"replicas"`
	ReadyReplicas int32  `json:"readyReplicas"`
	Status        string `json:"status"`
	RiskLevel     string `json:"riskLevel"`
	Issue         string `json:"issue"`
}

func (s *Server) handleControlPlaneHealth1912(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := ControlPlaneResult{ScannedAt: time.Now()}

	// Known control plane component patterns
	cpComponents := map[string]bool{
		"kube-apiserver":          true,
		"kube-controller-manager": true,
		"kube-scheduler":          true,
		"etcd":                    true,
		"coredns":                 true,
	}

	// Check kube-system pods
	pods, _ := rc.clientset.CoreV1().Pods("kube-system").List(ctx, metav1.ListOptions{})
	compReady := map[string]int{}
	compTotal := map[string]int{}
	compIssues := map[string][]string{}

	for _, pod := range pods.Items {
		// Match by name prefix
		for cpName := range cpComponents {
			if strings.HasPrefix(pod.Name, cpName) {
				compTotal[cpName]++
				isReady := true
				var issues []string
				for _, cs := range pod.Status.ContainerStatuses {
					if !cs.Ready {
						isReady = false
						issues = append(issues, fmt.Sprintf("container %s not ready", cs.Name))
					}
					if cs.RestartCount > 5 {
						issues = append(issues, fmt.Sprintf("container %s restarted %d times", cs.Name, cs.RestartCount))
					}
				}
				for _, cond := range pod.Status.Conditions {
					if cond.Type == corev1.PodScheduled && cond.Status != corev1.ConditionTrue {
						isReady = false
						issues = append(issues, "pod not scheduled")
					}
				}
				if isReady {
					compReady[cpName]++
				}
				if len(issues) > 0 {
					compIssues[cpName] = append(compIssues[cpName], issues...)
				}
				break
			}
		}
	}

	for name := range cpComponents {
		total := compTotal[name]
		ready := compReady[name]
		result.Summary.TotalComponents++
		result.Summary.TotalReplicas += total
		result.Summary.ReadyReplicas += ready

		entry := ControlPlaneComp{
			Name: name, Namespace: "kube-system",
			Kind: "Pod", Replicas: int32(total), ReadyReplicas: int32(ready),
			Ready: fmt.Sprintf("%d/%d", ready, total),
		}

		switch {
		case total == 0:
			entry.Status = "missing"
			entry.RiskLevel = "critical"
			entry.Issue = "component not found in kube-system"
			result.Summary.Unhealthy++
		case ready == 0:
			entry.Status = "unhealthy"
			entry.RiskLevel = "critical"
			entry.Issue = "no ready replicas"
			result.Summary.Unhealthy++
		case ready < total:
			entry.Status = "degraded"
			entry.RiskLevel = "high"
			entry.Issue = fmt.Sprintf("only %d/%d replicas ready", ready, total)
			result.Summary.Degraded++
		default:
			entry.Status = "healthy"
			entry.RiskLevel = "low"
			result.Summary.Healthy++
		}

		if len(compIssues[name]) > 0 && entry.RiskLevel != "low" {
			entry.Issue += " (" + strings.Join(compIssues[name][:minInt1912(3, len(compIssues[name]))], "; ") + ")"
		}

		result.Components = append(result.Components, entry)
		if entry.RiskLevel != "low" {
			result.Issues = append(result.Issues, entry)
		}
	}

	sort.Slice(result.Components, func(i, j int) bool {
		return result.Components[i].RiskLevel == "critical"
	})

	// Score
	if result.Summary.TotalComponents > 0 {
		healthyPct := result.Summary.Healthy * 100 / result.Summary.TotalComponents
		result.HealthScore = healthyPct
	} else {
		result.HealthScore = 0
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildControlPlaneRecs1912(&result)
	writeJSON(w, result)
}

func minInt1912(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func buildControlPlaneRecs1912(r *ControlPlaneResult) []string {
	recs := []string{fmt.Sprintf("Control plane: %d components (%d healthy, %d degraded, %d unhealthy), %d/%d replicas ready",
		r.Summary.TotalComponents, r.Summary.Healthy, r.Summary.Degraded, r.Summary.Unhealthy,
		r.Summary.ReadyReplicas, r.Summary.TotalReplicas)}
	if r.Summary.Unhealthy > 0 {
		recs = append(recs, fmt.Sprintf("%d unhealthy control plane components - immediate investigation required", r.Summary.Unhealthy))
	}
	return recs
}

// ---------------------------------------------------------------
// 2. CSI Driver Health — storage driver status
// ---------------------------------------------------------------

type CSIDriverResult struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         CSIDriverSummary     `json:"summary"`
	Drivers         []CSIDriverEntry1912 `json:"drivers"`
	NodePlugins     []CSINodePluginEntry `json:"nodePlugins"`
	Issues          []CSIDriverEntry1912 `json:"issues"`
	Recommendations []string             `json:"recommendations"`
}

type CSIDriverSummary struct {
	TotalDrivers     int `json:"totalDrivers"`
	HealthyDrivers   int `json:"healthyDrivers"`
	NodePluginsFound int `json:"nodePluginsFound"`
	PluginPodsReady  int `json:"pluginPodsReady"`
	PluginPodsTotal  int `json:"pluginPodsTotal"`
	StorageClasses   int `json:"storageClasses"`
}

type CSIDriverEntry1912 struct {
	Name        string `json:"name"`
	Provisioner string `json:"provisioner"`
	Status      string `json:"status"`
	PodReady    string `json:"podReady"`
	RiskLevel   string `json:"riskLevel"`
	Issue       string `json:"issue"`
}

type CSINodePluginEntry struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	PodCount   int    `json:"podCount"`
	ReadyCount int    `json:"readyCount"`
	DaemonSet  bool   `json:"isDaemonSet"`
}

func (s *Server) handleCSIDriverHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := CSIDriverResult{ScannedAt: time.Now()}

	// Get StorageClasses to identify provisioners
	scList, _ := rc.clientset.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	provisionerSet := map[string]bool{}
	for _, sc := range scList.Items {
		provisionerSet[sc.Provisioner] = true
		result.Summary.StorageClasses++
	}

	// Check for CSI driver pods (typically in kube-system)
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	pluginPods := map[string]*CSINodePluginEntry{}

	for _, pod := range pods.Items {
		name := pod.Name
		// Match CSI driver pod patterns
		isCSI := false
		if strings.Contains(name, "csi") || strings.Contains(name, "provisioner") {
			isCSI = true
		}
		if !isCSI {
			continue
		}

		// Track as plugin entry
		nsKey := pod.Namespace + "/" + extractCSIBaseName1912(name)
		entry, ok := pluginPods[nsKey]
		if !ok {
			entry = &CSINodePluginEntry{
				Name: extractCSIBaseName1912(name), Namespace: pod.Namespace,
			}
			pluginPods[nsKey] = entry
		}
		entry.PodCount++
		result.Summary.PluginPodsTotal++

		isReady := true
		for _, cs := range pod.Status.ContainerStatuses {
			if !cs.Ready {
				isReady = false
			}
		}
		if isReady {
			entry.ReadyCount++
			result.Summary.PluginPodsReady++
		}
	}

	for _, plugin := range pluginPods {
		if plugin.PodCount > 1 {
			plugin.DaemonSet = true
		}
		result.NodePlugins = append(result.NodePlugins, *plugin)
	}
	result.Summary.NodePluginsFound = len(pluginPods)

	// Build driver entries from provisioners
	for provisioner := range provisionerSet {
		result.Summary.TotalDrivers++
		entry := CSIDriverEntry1912{
			Name:        provisioner,
			Provisioner: provisioner,
		}

		// Check if plugin pods exist
		found := false
		for _, plugin := range result.NodePlugins {
			if strings.Contains(strings.ToLower(plugin.Name), strings.ToLower(strings.ReplaceAll(provisioner, ".csi.io", ""))) {
				found = true
				if plugin.ReadyCount == plugin.PodCount {
					entry.Status = "healthy"
					entry.PodReady = fmt.Sprintf("%d/%d", plugin.ReadyCount, plugin.PodCount)
					entry.RiskLevel = "low"
					result.Summary.HealthyDrivers++
				} else {
					entry.Status = "degraded"
					entry.PodReady = fmt.Sprintf("%d/%d", plugin.ReadyCount, plugin.PodCount)
					entry.RiskLevel = "high"
					entry.Issue = fmt.Sprintf("only %d/%d plugin pods ready", plugin.ReadyCount, plugin.PodCount)
				}
				break
			}
		}
		if !found {
			entry.Status = "no-pods"
			entry.PodReady = "0/0"
			entry.RiskLevel = "medium"
			entry.Issue = "no CSI plugin pods detected"
		}

		result.Drivers = append(result.Drivers, entry)
		if entry.RiskLevel != "low" {
			result.Issues = append(result.Issues, entry)
		}
	}

	// Score
	if result.Summary.PluginPodsTotal > 0 {
		readyPct := result.Summary.PluginPodsReady * 100 / result.Summary.PluginPodsTotal
		result.HealthScore = readyPct
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildCSIDriverRecs1912(&result)
	writeJSON(w, result)
}

func extractCSIBaseName1912(name string) string {
	// Extract base name from pod name like "csi-node-driver-abcde"
	parts := strings.Split(name, "-")
	if len(parts) > 1 {
		// Take first 2-3 parts as base name
		maxParts := 3
		if len(parts) < maxParts {
			maxParts = len(parts)
		}
		return strings.Join(parts[:maxParts], "-")
	}
	return name
}

func buildCSIDriverRecs1912(r *CSIDriverResult) []string {
	recs := []string{fmt.Sprintf("CSI driver health: %d drivers, %d healthy, %d node plugins (%d/%d pods ready)",
		r.Summary.TotalDrivers, r.Summary.HealthyDrivers,
		r.Summary.NodePluginsFound, r.Summary.PluginPodsReady, r.Summary.PluginPodsTotal)}
	if len(r.Issues) > 0 {
		recs = append(recs, fmt.Sprintf("%d CSI driver issues detected - check storage provisioner logs", len(r.Issues)))
	}
	return recs
}

// ---------------------------------------------------------------
// 3. Cert Renewal Timeline — track expiring certs
// ---------------------------------------------------------------

type CertRenewalResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         CertRenewalSummary  `json:"summary"`
	Certificates    []CertRenewalEntry  `json:"certificates"`
	Timeline        []CertTimelineEntry `json:"timeline"`
	Recommendations []string            `json:"recommendations"`
}

type CertRenewalSummary struct {
	TotalSecrets  int `json:"totalSecrets"`
	TLSSecrets    int `json:"tlsSecrets"`
	Expiring30d   int `json:"expiring30Days"`
	Expiring7d    int `json:"expiring7Days"`
	Expired       int `json:"expired"`
	RenewedRecent int `json:"reewedRecent"`
}

type CertRenewalEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Type      string `json:"type"`
	AgeDays   int    `json:"ageDays"`
	ExpiresIn string `json:"expiresIn"`
	RiskLevel string `json:"riskLevel"`
}

type CertTimelineEntry struct {
	Date   string `json:"date"`
	Event  string `json:"event"`
	Count  int    `json:"count"`
	Action string `json:"action"`
}

func (s *Server) handleCertRenewalTimeline(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := CertRenewalResult{ScannedAt: time.Now()}

	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	now := time.Now()

	for _, secret := range secrets.Items {
		if isSystemNamespace(secret.Namespace) {
			continue
		}
		if secret.Type != corev1.SecretTypeTLS && !strings.Contains(strings.ToLower(secret.Name), "cert") &&
			!strings.Contains(strings.ToLower(secret.Name), "tls") {
			continue
		}

		result.Summary.TotalSecrets++
		if secret.Type == corev1.SecretTypeTLS {
			result.Summary.TLSSecrets++
		}

		ageDays := int(now.Sub(secret.CreationTimestamp.Time).Hours() / 24)
		entry := CertRenewalEntry{
			Name: secret.Name, Namespace: secret.Namespace,
			Type:    string(secret.Type),
			AgeDays: ageDays,
		}

		// Estimate renewal timeline (certs typically last 365 days)
		certLifetime := 365
		expiresInDays := certLifetime - ageDays
		entry.ExpiresIn = fmt.Sprintf("%dd", expiresInDays)

		switch {
		case expiresInDays <= 0:
			entry.RiskLevel = "critical"
			result.Summary.Expired++
			result.Timeline = append(result.Timeline, CertTimelineEntry{
				Date:  now.Format("2006-01-02"),
				Event: "expired", Count: 1,
				Action: fmt.Sprintf("Renew %s/%s immediately", secret.Namespace, secret.Name),
			})
		case expiresInDays <= 7:
			entry.RiskLevel = "critical"
			result.Summary.Expiring7d++
			result.Summary.Expiring30d++
			result.Timeline = append(result.Timeline, CertTimelineEntry{
				Date:  now.Add(time.Duration(expiresInDays) * 24 * time.Hour).Format("2006-01-02"),
				Event: "expiring", Count: 1,
				Action: fmt.Sprintf("Renew %s/%s within %d days", secret.Namespace, secret.Name, expiresInDays),
			})
		case expiresInDays <= 30:
			entry.RiskLevel = "high"
			result.Summary.Expiring30d++
		case expiresInDays <= 90:
			entry.RiskLevel = "medium"
		default:
			entry.RiskLevel = "low"
			if ageDays < 30 {
				result.Summary.RenewedRecent++
			}
		}

		result.Certificates = append(result.Certificates, entry)
	}

	// Sort by risk
	sort.Slice(result.Certificates, func(i, j int) bool {
		riskOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		return riskOrder[result.Certificates[i].RiskLevel] < riskOrder[result.Certificates[j].RiskLevel]
	})
	sort.Slice(result.Timeline, func(i, j int) bool {
		return result.Timeline[i].Date < result.Timeline[j].Date
	})

	// Score
	if result.Summary.TotalSecrets > 0 {
		expiredRisk := result.Summary.Expired + result.Summary.Expiring7d
		result.HealthScore = (result.Summary.TotalSecrets - expiredRisk) * 100 / result.Summary.TotalSecrets
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildCertRenewalRecs1912(&result)
	writeJSON(w, result)
}

func buildCertRenewalRecs1912(r *CertRenewalResult) []string {
	recs := []string{fmt.Sprintf("Cert renewal: %d TLS/cert secrets, %d expiring in 30d, %d in 7d, %d expired",
		r.Summary.TotalSecrets, r.Summary.Expiring30d, r.Summary.Expiring7d, r.Summary.Expired)}
	if r.Summary.Expiring30d > 0 {
		recs = append(recs, fmt.Sprintf("%d certificates expiring within 30 days - set up automated renewal", r.Summary.Expiring30d))
	}
	if r.Summary.Expired > 0 {
		recs = append(recs, fmt.Sprintf("%d expired certificates - renew immediately to avoid service disruption", r.Summary.Expired))
	}
	return recs
}
