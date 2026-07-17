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

// StoragePerfResult analyzes storage performance classification and workload-storage alignment.
// It classifies StorageClasses by performance tier, identifies workloads with mismatched
// storage performance (e.g., databases on slow storage), and recommends optimal assignments.
type StoragePerfResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         StoragePerfSummary  `json:"summary"`
	StorageClasses  []SCPerfInfo        `json:"storageClasses"`
	Mismatches      []StorageMismatch   `json:"mismatches"`
	ByNamespace     []StoragePerfNSStat `json:"byNamespace"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

// StoragePerfSummary aggregates storage performance statistics.
type StoragePerfSummary struct {
	TotalPVCs        int     `json:"totalPVCs"`
	TotalSizeGB      float64 `json:"totalSizeGB"`
	FastTierPVCs     int     `json:"fastTierPVCs"`
	StandardTierPVCs int     `json:"standardTierPVCs"`
	SlowTierPVCs     int     `json:"slowTierPVCs"`
	UnknownTierPVCs  int     `json:"unknownTierPVCs"`
	MismatchedPVCs   int     `json:"mismatchedPVCs"`
	AvgPVCSizeGB     float64 `json:"avgPVCSizeGB"`
	UnboundPVCs      int     `json:"unboundPVCs"`
}

// SCPerfInfo describes a StorageClass performance classification.
type SCPerfInfo struct {
	Name        string  `json:"name"`
	Provisioner string  `json:"provisioner"`
	PerfTier    string  `json:"perfTier"` // fast, standard, slow, unknown
	PVCCount    int     `json:"pvcCount"`
	TotalGB     float64 `json:"totalGB"`
	BindingMode string  `json:"bindingMode"`
	ReclaimP    string  `json:"reclaimPolicy"`
	IsDefault   bool    `json:"isDefault"`
}

// StorageMismatch describes a workload-storage performance mismatch.
type StorageMismatch struct {
	Workload     string `json:"workload"`
	Namespace    string `json:"namespace"`
	PVCName      string `json:"pvcName"`
	SCName       string `json:"scName"`
	CurrentTier  string `json:"currentTier"`
	ExpectedTier string `json:"expectedTier"`
	WorkloadType string `json:"workloadType"`
	Severity     string `json:"severity"`
	Reason       string `json:"reason"`
}

// StoragePerfNSStat per-namespace storage stats.
type StoragePerfNSStat struct {
	Namespace     string  `json:"namespace"`
	PVCCount      int     `json:"pvcCount"`
	TotalGB       float64 `json:"totalGB"`
	MismatchCount int     `json:"mismatchCount"`
}

// classifyStorageTier determines the performance tier of a StorageClass.
func classifyStorageTier(provisioner, scName string) string {
	name := strings.ToLower(scName)
	prov := strings.ToLower(provisioner)

	// Fast tier indicators: NVMe, SSD, premium, ultra, fast
	fastPatterns := []string{"nvme", "ssd", "premium", "ultra", "fast", "io1", "io2", "gp3", "pd-ssd", "pd-extreme", "ultrassd"}
	for _, p := range fastPatterns {
		if strings.Contains(name, p) || strings.Contains(prov, p) {
			return "fast"
		}
	}

	// Slow tier indicators: hdd, cold, archive, standard-disk, st1, sc1
	slowPatterns := []string{"hdd", "cold", "archive", "st1", "sc1", "pd-standard", "standard-lrs"}
	for _, p := range slowPatterns {
		if strings.Contains(name, p) {
			return "slow"
		}
	}

	// Standard tier: gp2, default, standard
	standardPatterns := []string{"gp2", "default", "standard", "managed-csi", "managed-standard", "pd-standard"}
	for _, p := range standardPatterns {
		if name == p || strings.Contains(name, p) {
			return "standard"
		}
	}

	// Generic CSI / local-path / network storage → assume standard
	genericPatterns := []string{"local-path", "hostpath", "nfs", "ceph", "longhorn", "openebs", "rbd", "csi"}
	for _, p := range genericPatterns {
		if strings.Contains(prov, p) {
			return "standard"
		}
	}

	_ = prov
	return "unknown"
}

// inferWorkloadType guesses the workload's storage needs from pod name/labels.
func inferWorkloadType(name, namespace string, labels map[string]string) string {
	combined := strings.ToLower(name + " " + namespace)
	for k, v := range labels {
		combined += " " + strings.ToLower(k) + " " + strings.ToLower(v)
	}

	// Database workloads need fast storage
	dbPatterns := []string{"postgres", "mysql", "mongo", "redis", "elastic", "zookeeper", "etcd", "mariadb", "cockroach", "influx", "prometheus", "timescale", "database", "db-", "-db", "sql"}
	for _, p := range dbPatterns {
		if strings.Contains(combined, p) {
			return "database"
		}
	}

	// Message queue workloads need fast storage
	mqPatterns := []string{"rabbitmq", "pulsar", "nats", "queue", "broker", "kafka"}
	for _, p := range mqPatterns {
		if strings.Contains(combined, p) {
			return "message-queue"
		}
	}

	// Logging/analytics workloads
	logPatterns := []string{"fluentd", "fluent-bit", "loki", "graylog", "filebeat", "log-"}
	for _, p := range logPatterns {
		if strings.Contains(combined, p) {
			return "logging"
		}
	}

	return "general"
}

// expectedTierForWorkloadType returns the ideal storage tier for a workload type.
func expectedTierForWorkloadType(wt string) string {
	switch wt {
	case "database", "message-queue":
		return "fast"
	case "logging":
		return "standard"
	default:
		return "standard"
	}
}

// handleStoragePerf handles GET /api/scalability/storage-performance
func (s *Server) handleStoragePerf(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := StoragePerfResult{ScannedAt: time.Now()}

	pvcs, _ := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	scs, _ := rc.clientset.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})

	// Build SC tier map
	scTierMap := map[string]string{} // scName -> tier
	scDefaultName := ""
	scInfoMap := map[string]*SCPerfInfo{}
	for _, sc := range scs.Items {
		tier := classifyStorageTier(sc.Provisioner, sc.Name)
		scTierMap[sc.Name] = tier
		info := &SCPerfInfo{
			Name:        sc.Name,
			Provisioner: sc.Provisioner,
			PerfTier:    tier,
			BindingMode: "",
			ReclaimP:    "",
		}
		if sc.VolumeBindingMode != nil {
			info.BindingMode = string(*sc.VolumeBindingMode)
		}
		if sc.ReclaimPolicy != nil {
			info.ReclaimP = string(*sc.ReclaimPolicy)
		}
		if sc.Annotations != nil && sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			info.IsDefault = true
			scDefaultName = sc.Name
		}
		scInfoMap[sc.Name] = info
	}

	// Build PVC -> pod mapping for workload type inference
	pvcOwnerMap := map[string]string{}    // ns/pvc -> workload type
	pvcWorkloadMap := map[string]string{} // ns/pvc -> workload name
	for _, pod := range pods.Items {
		for _, vol := range pod.Spec.Volumes {
			if vol.PersistentVolumeClaim != nil {
				key := pod.Namespace + "/" + vol.PersistentVolumeClaim.ClaimName
				wt := inferWorkloadType(pod.Name, pod.Namespace, pod.Labels)
				pvcOwnerMap[key] = wt
				pvcWorkloadMap[key] = pod.Name
			}
		}
	}

	// Analyze PVCs
	nsStats := map[string]*StoragePerfNSStat{}
	for _, pvc := range pvcs.Items {
		if strings.HasPrefix(pvc.Namespace, "kube-") {
			continue
		}
		result.Summary.TotalPVCs++

		sizeGB := 0.0
		if q, ok := pvc.Spec.Resources.Requests[corev1.ResourceStorage]; ok {
			sizeGB = float64(q.Value()) / (1024 * 1024 * 1024)
		}
		result.Summary.TotalSizeGB += sizeGB

		// Determine SC and tier
		scName := ""
		if pvc.Spec.StorageClassName != nil {
			scName = *pvc.Spec.StorageClassName
		} else {
			scName = scDefaultName
		}

		tier := scTierMap[scName]
		if tier == "" {
			tier = "unknown"
		}

		switch tier {
		case "fast":
			result.Summary.FastTierPVCs++
		case "standard":
			result.Summary.StandardTierPVCs++
		case "slow":
			result.Summary.SlowTierPVCs++
		default:
			result.Summary.UnknownTierPVCs++
		}

		// Update SC info
		if info, ok := scInfoMap[scName]; ok {
			info.PVCCount++
			info.TotalGB += sizeGB
		}

		// Check for unbound PVCs
		if pvc.Status.Phase != corev1.ClaimBound {
			result.Summary.UnboundPVCs++
		}

		// Check for performance mismatch
		key := pvc.Namespace + "/" + pvc.Name
		wt := pvcOwnerMap[key]
		if wt != "" {
			expected := expectedTierForWorkloadType(wt)
			severity := assessStorageMismatch(tier, expected)
			if severity != "" {
				result.Summary.MismatchedPVCs++
				result.Mismatches = append(result.Mismatches, StorageMismatch{
					Workload:     pvcWorkloadMap[key],
					Namespace:    pvc.Namespace,
					PVCName:      pvc.Name,
					SCName:       scName,
					CurrentTier:  tier,
					ExpectedTier: expected,
					WorkloadType: wt,
					Severity:     severity,
					Reason:       fmt.Sprintf("%s workload on %s-tier storage — expected %s-tier", wt, tier, expected),
				})
			}
		}

		// Track namespace stats
		ns := pvc.Namespace
		if _, ok := nsStats[ns]; !ok {
			nsStats[ns] = &StoragePerfNSStat{Namespace: ns}
		}
		nsStats[ns].PVCCount++
		nsStats[ns].TotalGB += sizeGB
	}

	// Update mismatch counts in namespace stats
	for _, m := range result.Mismatches {
		if ns, ok := nsStats[m.Namespace]; ok {
			ns.MismatchCount++
		}
	}
	for _, ns := range nsStats {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].MismatchCount > result.ByNamespace[j].MismatchCount
	})

	// Build SC info list
	for _, sc := range scs.Items {
		if info, ok := scInfoMap[sc.Name]; ok {
			result.StorageClasses = append(result.StorageClasses, *info)
		}
	}
	sort.Slice(result.StorageClasses, func(i, j int) bool {
		return result.StorageClasses[i].PVCCount > result.StorageClasses[j].PVCCount
	})

	// Calculate averages
	if result.Summary.TotalPVCs > 0 {
		result.Summary.AvgPVCSizeGB = result.Summary.TotalSizeGB / float64(result.Summary.TotalPVCs)
	}

	// Compute health score
	result.HealthScore = computeStoragePerfScore(result.Summary)
	result.Grade = scoreToGrade(result.HealthScore)

	// Generate recommendations
	result.Recommendations = generateStoragePerfRecs(result.Summary, result.Mismatches, result.StorageClasses)

	writeJSON(w, result)
}

// assessStorageMismatch returns severity if there's a performance mismatch, empty otherwise.
func assessStorageMismatch(currentTier, expectedTier string) string {
	if currentTier == "unknown" || expectedTier == "" {
		return ""
	}
	tierRank := map[string]int{"slow": 1, "standard": 2, "fast": 3, "unknown": 0}
	cur := tierRank[currentTier]
	exp := tierRank[expectedTier]
	if cur < exp {
		if exp-cur >= 2 {
			return "critical"
		}
		return "warning"
	}
	return ""
}

// computeStoragePerfScore computes a 0-100 score.
func computeStoragePerfScore(s StoragePerfSummary) int {
	score := 100
	if s.TotalPVCs == 0 {
		return score
	}
	// Penalize mismatches heavily
	mismatchRatio := float64(s.MismatchedPVCs) / float64(s.TotalPVCs)
	score -= int(mismatchRatio * 40)
	// Penalize unknown tiers
	unknownRatio := float64(s.UnknownTierPVCs) / float64(s.TotalPVCs)
	score -= int(unknownRatio * 20)
	// Penalize unbound PVCs
	unboundRatio := float64(s.UnboundPVCs) / float64(s.TotalPVCs)
	score -= int(unboundRatio * 20)
	// Penalize slow-tier usage
	if s.SlowTierPVCs > 0 {
		slowRatio := float64(s.SlowTierPVCs) / float64(s.TotalPVCs)
		score -= int(slowRatio * 10)
	}
	// Penalize no fast-tier at all (performance blind spot)
	if s.FastTierPVCs == 0 && s.TotalPVCs > 3 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	return score
}

// generateStoragePerfRecs produces human-readable recommendations.
func generateStoragePerfRecs(s StoragePerfSummary, mismatches []StorageMismatch, scs []SCPerfInfo) []string {
	var recs []string

	if s.TotalPVCs == 0 {
		recs = append(recs, "No PVCs detected — storage performance analysis not applicable")
		return recs
	}

	recs = append(recs, fmt.Sprintf("Storage performance score: %d/100 (grade %s) across %d PVCs (%.1f GB total)",
		computeStoragePerfScore(s), scoreToGrade(computeStoragePerfScore(s)), s.TotalPVCs, s.TotalSizeGB))

	if s.MismatchedPVCs > 0 {
		recs = append(recs, fmt.Sprintf("%d PVC(s) have storage performance mismatches — databases on slow storage cause I/O bottlenecks", s.MismatchedPVCs))
		for _, m := range mismatches {
			if m.Severity == "critical" {
				recs = append(recs, fmt.Sprintf("  CRITICAL: %s/%s (%s) on %s-tier — upgrade to fast-tier storage", m.Namespace, m.PVCName, m.WorkloadType, m.CurrentTier))
			}
		}
	}

	if s.UnknownTierPVCs > 0 {
		recs = append(recs, fmt.Sprintf("%d PVC(s) have unknown storage tier — classify StorageClasses with performance labels", s.UnknownTierPVCs))
	}

	if s.UnboundPVCs > 0 {
		recs = append(recs, fmt.Sprintf("%d PVC(s) are unbound — check storage provisioner health and capacity", s.UnboundPVCs))
	}

	if s.SlowTierPVCs > 0 {
		recs = append(recs, fmt.Sprintf("%d PVC(s) on slow-tier storage — suitable for cold/archival data only", s.SlowTierPVCs))
	}

	fastSCs := 0
	for _, sc := range scs {
		if sc.PerfTier == "fast" {
			fastSCs++
		}
	}
	if fastSCs == 0 && s.TotalPVCs > 3 {
		recs = append(recs, "No fast-tier StorageClass available — create premium SSD/NVMe-backed class for database workloads")
	}

	return recs
}
