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
// v19.01 — Operations Dimension (Round 3)
// 1. Node Maintenance Window
// 2. Resource Leak Detector
// 3. Log Aggregation Health
// ============================================================

// ---------------------------------------------------------------
// 1. Node Maintenance Window — impact analysis for cordon/drain
// ---------------------------------------------------------------

type NodeMaintWindowResult struct {
	ScannedAt         time.Time        `json:"scannedAt"`
	HealthScore       int              `json:"healthScore"`
	Grade             string           `json:"grade"`
	Summary           NodeMaintSummary `json:"summary"`
	NodeImpact        []NodeMaintEntry `json:"nodeImpact"`
	AffectedWorkloads []AffectedWL     `json:"affectedWorkloads"`
	SafeWindows       []MaintWindow    `json:"safeWindows"`
	Recommendations   []string         `json:"recommendations"`
}

type NodeMaintSummary struct {
	TotalNodes       int `json:"totalNodes"`
	CordonedNodes    int `json:"cordonedNodes"`
	ReadyNodes       int `json:"readyNodes"`
	TotalPodsOnNodes int `json:"totalPodsOnNodes"`
	DisruptablePods  int `json:"disruptablePods"`
	StuckPods        int `json:"stuckPods"`
	PDBProtected     int `json:"pdbProtected"`
}

type NodeMaintEntry struct {
	Node           string `json:"node"`
	Ready          bool   `json:"ready"`
	Cordoned       bool   `json:"cordoned"`
	PodCount       int    `json:"podCount"`
	NamespaceCount int    `json:"namespaceCount"`
	ImpactLevel    string `json:"impactLevel"`
}

type AffectedWL struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Replicas   int32  `json:"replicas"`
	PodsOnNode int    `json:"podsOnNode"`
	RiskLevel  string `json:"riskLevel"`
}

type MaintWindow struct {
	TimeWindow  string `json:"timeWindow"`
	Description string `json:"description"`
	SafetyLevel string `json:"safetyLevel"`
}

func (s *Server) handleNodeMaintWindow(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := NodeMaintWindowResult{ScannedAt: time.Now()}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})
	hasPDB := len(pdbs.Items) > 0

	// Map pods to nodes
	nodePods := map[string][]corev1.Pod{}
	nodeNS := map[string]map[string]bool{}
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Spec.NodeName == "" {
			continue
		}
		nodePods[pod.Spec.NodeName] = append(nodePods[pod.Spec.NodeName], pod)
		if nodeNS[pod.Spec.NodeName] == nil {
			nodeNS[pod.Spec.NodeName] = map[string]bool{}
		}
		nodeNS[pod.Spec.NodeName][pod.Namespace] = true
	}

	for _, node := range nodes.Items {
		result.Summary.TotalNodes++
		entry := NodeMaintEntry{Node: node.Name}

		// Check ready
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady {
				entry.Ready = cond.Status == corev1.ConditionTrue
			}
		}
		entry.Cordoned = node.Spec.Unschedulable
		if entry.Cordoned {
			result.Summary.CordonedNodes++
		}
		if entry.Ready && !entry.Cordoned {
			result.Summary.ReadyNodes++
		}

		entry.PodCount = len(nodePods[node.Name])
		entry.NamespaceCount = len(nodeNS[node.Name])
		result.Summary.TotalPodsOnNodes += entry.PodCount

		switch {
		case entry.PodCount == 0:
			entry.ImpactLevel = "none"
		case entry.PodCount > 50:
			entry.ImpactLevel = "critical"
		case entry.PodCount > 20:
			entry.ImpactLevel = "high"
		default:
			entry.ImpactLevel = "medium"
		}

		// Track affected workloads
		wlMap := map[string]*AffectedWL{}
		for _, pod := range nodePods[node.Name] {
			owner := getOwnerName(pod.OwnerReferences)
			if owner == "" {
				owner = pod.Name
			}
			key := pod.Namespace + "/" + owner
			if wlMap[key] == nil {
				wlMap[key] = &AffectedWL{
					Name: owner, Namespace: pod.Namespace, PodsOnNode: 0,
				}
			}
			wlMap[key].PodsOnNode++
			result.Summary.DisruptablePods++
		}
		for _, wl := range wlMap {
			if wl.PodsOnNode >= 5 {
				wl.RiskLevel = "high"
			} else if hasPDB {
				wl.RiskLevel = "low"
				result.Summary.PDBProtected++
			} else {
				wl.RiskLevel = "medium"
			}
			result.AffectedWorkloads = append(result.AffectedWorkloads, *wl)
		}

		result.NodeImpact = append(result.NodeImpact, entry)
	}

	// Safe maintenance windows
	result.SafeWindows = []MaintWindow{
		{TimeWindow: "night-02-06", Description: "02:00-06:00 - lowest traffic, safest for drain", SafetyLevel: "high"},
		{TimeWindow: "weekend", Description: "Saturday/Sunday - reduced workload impact", SafetyLevel: "medium"},
		{TimeWindow: "business-hours", Description: "09:00-18:00 weekdays - avoid unless emergency", SafetyLevel: "low"},
	}

	// Score
	if result.Summary.TotalNodes > 0 {
		readyPct := result.Summary.ReadyNodes * 100 / result.Summary.TotalNodes
		result.HealthScore = readyPct
	}
	if result.Summary.CordonedNodes > 0 && result.Summary.TotalNodes <= 1 {
		result.HealthScore = 0
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildNodeMaintRecs1901(&result)
	writeJSON(w, result)
}

func buildNodeMaintRecs1901(r *NodeMaintWindowResult) []string {
	recs := []string{fmt.Sprintf("Node maintenance: %d nodes (%d ready, %d cordoned), %d pods affected",
		r.Summary.TotalNodes, r.Summary.ReadyNodes, r.Summary.CordonedNodes, r.Summary.DisruptablePods)}
	if r.Summary.CordonedNodes > 0 {
		recs = append(recs, fmt.Sprintf("%d cordoned nodes - drain and patch, then uncordon", r.Summary.CordonedNodes))
	}
	return recs
}

// ---------------------------------------------------------------
// 2. Resource Leak Detector — orphaned ConfigMaps, Secrets, PVCs
// ---------------------------------------------------------------

type ResourceLeakResult struct {
	ScannedAt       time.Time   `json:"scannedAt"`
	HealthScore     int         `json:"healthScore"`
	Grade           string      `json:"grade"`
	Summary         LeakSummary `json:"summary"`
	OrphanedCMs     []LeakEntry `json:"orphanedConfigMaps"`
	OrphanedSecrets []LeakEntry `json:"orphanedSecrets"`
	OrphanedPVCs    []LeakEntry `json:"orphanedPVCs"`
	LargeCMs        []LeakEntry `json:"largeConfigMaps"`
	Recommendations []string    `json:"recommendations"`
}

type LeakSummary struct {
	TotalCMs         int `json:"totalConfigMaps"`
	OrphanedCMs      int `json:"orphanedConfigMaps"`
	TotalSecrets     int `json:"totalSecrets"`
	OrphanedSecrets  int `json:"orphanedSecrets"`
	TotalPVCs        int `json:"totalPVCs"`
	OrphanedPVCs     int `json:"orphanedPVCs"`
	LargeCMs         int `json:"largeConfigMaps"`
	EstimatedWasteKB int `json:"estimatedWasteKB"`
}

type LeakEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	SizeKB    int    `json:"sizeKB,omitempty"`
	Age       string `json:"age"`
	RiskLevel string `json:"riskLevel"`
	Reason    string `json:"reason"`
}

func (s *Server) handleResourceLeakDetector(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := ResourceLeakResult{ScannedAt: time.Now()}

	// Collect referenced CMs and Secrets from pods
	referencedCMs := map[string]bool{}
	referencedSecrets := map[string]bool{}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		// Volumes
		for _, vol := range pod.Spec.Volumes {
			if vol.ConfigMap != nil {
				referencedCMs[pod.Namespace+"/"+vol.ConfigMap.Name] = true
			}
			if vol.Secret != nil {
				referencedSecrets[pod.Namespace+"/"+vol.Secret.SecretName] = true
			}
			if vol.Projected != nil {
				for _, src := range vol.Projected.Sources {
					if src.ConfigMap != nil {
						referencedCMs[pod.Namespace+"/"+src.ConfigMap.Name] = true
					}
					if src.Secret != nil {
						referencedSecrets[pod.Namespace+"/"+src.Secret.Name] = true
					}
				}
			}
		}
		// Env vars
		for _, c := range pod.Spec.Containers {
			for _, env := range c.Env {
				if env.ValueFrom != nil {
					if env.ValueFrom.ConfigMapKeyRef != nil {
						referencedCMs[pod.Namespace+"/"+env.ValueFrom.ConfigMapKeyRef.Name] = true
					}
					if env.ValueFrom.SecretKeyRef != nil {
						referencedSecrets[pod.Namespace+"/"+env.ValueFrom.SecretKeyRef.Name] = true
					}
				}
			}
		}
	}

	// Check ConfigMaps
	cms, _ := rc.clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{})
	for _, cm := range cms.Items {
		if isSystemNamespace(cm.Namespace) {
			continue
		}
		result.Summary.TotalCMs++
		key := cm.Namespace + "/" + cm.Name
		sizeKB := 0
		for _, v := range cm.Data {
			sizeKB += len(v) / 1024
		}
		for _, v := range cm.BinaryData {
			sizeKB += len(v) / 1024
		}

		// Large CMs
		if sizeKB > 512 {
			result.Summary.LargeCMs++
			result.LargeCMs = append(result.LargeCMs, LeakEntry{
				Name: cm.Name, Namespace: cm.Namespace, SizeKB: sizeKB,
				Age:       time.Since(cm.CreationTimestamp.Time).Round(time.Hour).String(),
				RiskLevel: "medium", Reason: fmt.Sprintf("large configmap: %dKB", sizeKB),
			})
		}

		// Orphaned CM (not referenced by any pod, not labeled as retained)
		if !referencedCMs[key] && cm.Labels["app.kubernetes.io/managed-by"] == "" {
			result.Summary.OrphanedCMs++
			result.OrphanedCMs = append(result.OrphanedCMs, LeakEntry{
				Name: cm.Name, Namespace: cm.Namespace,
				Age:       time.Since(cm.CreationTimestamp.Time).Round(time.Hour).String(),
				RiskLevel: "low", Reason: "not referenced by any pod",
			})
			result.Summary.EstimatedWasteKB += sizeKB
		}
	}

	// Check Secrets
	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})
	for _, secret := range secrets.Items {
		if isSystemNamespace(secret.Namespace) {
			continue
		}
		// Skip standard service account tokens
		if strings.HasPrefix(secret.Name, "default-token-") {
			continue
		}
		result.Summary.TotalSecrets++
		key := secret.Namespace + "/" + secret.Name
		if !referencedSecrets[key] {
			result.Summary.OrphanedSecrets++
			result.OrphanedSecrets = append(result.OrphanedSecrets, LeakEntry{
				Name: secret.Name, Namespace: secret.Namespace,
				Age:       time.Since(secret.CreationTimestamp.Time).Round(time.Hour).String(),
				RiskLevel: "medium", Reason: "secret not referenced by any pod",
			})
		}
	}

	// Check PVCs
	pvcs, _ := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	for _, pvc := range pvcs.Items {
		if isSystemNamespace(pvc.Namespace) {
			continue
		}
		result.Summary.TotalPVCs++
		// PVC is orphaned if no pod mounts it
		mounted := false
		for _, pod := range pods.Items {
			if pod.Namespace != pvc.Namespace || pod.Spec.NodeName == "" {
				continue
			}
			for _, vol := range pod.Spec.Volumes {
				if vol.PersistentVolumeClaim != nil && vol.PersistentVolumeClaim.ClaimName == pvc.Name {
					mounted = true
					break
				}
			}
		}
		if !mounted {
			result.Summary.OrphanedPVCs++
			result.OrphanedPVCs = append(result.OrphanedPVCs, LeakEntry{
				Name: pvc.Name, Namespace: pvc.Namespace,
				Age:       time.Since(pvc.CreationTimestamp.Time).Round(time.Hour).String(),
				RiskLevel: "medium", Reason: "PVC not mounted by any pod",
			})
		}
	}

	// Score: higher orphaned ratio = lower score
	total := result.Summary.TotalCMs + result.Summary.TotalSecrets + result.Summary.TotalPVCs
	orphaned := result.Summary.OrphanedCMs + result.Summary.OrphanedSecrets + result.Summary.OrphanedPVCs
	if total > 0 {
		result.HealthScore = (total - orphaned) * 100 / total
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildLeakRecs1901(&result)
	writeJSON(w, result)
}

func buildLeakRecs1901(r *ResourceLeakResult) []string {
	recs := []string{fmt.Sprintf("Resource leaks: %d orphaned CMs, %d orphaned secrets, %d orphaned PVCs (~%dKB waste)",
		r.Summary.OrphanedCMs, r.Summary.OrphanedSecrets, r.Summary.OrphanedPVCs, r.Summary.EstimatedWasteKB)}
	if r.Summary.OrphanedSecrets > 0 {
		recs = append(recs, fmt.Sprintf("%d orphaned secrets - clean up to reduce security surface", r.Summary.OrphanedSecrets))
	}
	if r.Summary.OrphanedPVCs > 0 {
		recs = append(recs, fmt.Sprintf("%d orphaned PVCs - delete to reclaim storage costs", r.Summary.OrphanedPVCs))
	}
	return recs
}

// ---------------------------------------------------------------
// 3. Log Aggregation Health — container log volume & quality
// ---------------------------------------------------------------

type LogAggHealthResult struct {
	ScannedAt        time.Time       `json:"scannedAt"`
	HealthScore      int             `json:"healthScore"`
	Grade            string          `json:"grade"`
	Summary          LogAggSummary   `json:"summary"`
	NoisyLoggers     []LogAggEntry   `json:"noisyLoggers"`
	SilentContainers []LogAggEntry   `json:"silentContainers"`
	ByNamespace      []LogAggNSEntry `json:"byNamespace"`
	Recommendations  []string        `json:"recommendations"`
}

type LogAggSummary struct {
	TotalContainers  int `json:"totalContainers"`
	WithLogVolume    int `json:"withLogVolume"`
	SilentContainers int `json:"silentContainers"`
	NoisyLoggers     int `json:"noisyLoggers"`
	TotalRestarts    int `json:"totalRestarts"`
	HighRestartRate  int `json:"highRestartRate"`
	LogPolicySet     int `json:"logPolicySet"`
}

type LogAggEntry struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	Container    string `json:"container"`
	RestartCount int    `json:"restartCount"`
	RiskLevel    string `json:"riskLevel"`
	Issue        string `json:"issue"`
}

type LogAggNSEntry struct {
	Namespace      string `json:"namespace"`
	ContainerCount int    `json:"containerCount"`
	NoisyCount     int    `json:"noisyCount"`
	SilentCount    int    `json:"silentCount"`
}

func (s *Server) handleLogAggHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := LogAggHealthResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	nsMap := map[string]*LogAggNSEntry{}

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Spec.NodeName == "" {
			continue
		}
		nsE, ok := nsMap[pod.Namespace]
		if !ok {
			nsE = &LogAggNSEntry{Namespace: pod.Namespace}
			nsMap[pod.Namespace] = nsE
		}

		for _, cs := range pod.Status.ContainerStatuses {
			result.Summary.TotalContainers++
			nsE.ContainerCount++

			if cs.RestartCount > 0 {
				result.Summary.WithLogVolume++
				result.Summary.TotalRestarts += int(cs.RestartCount)
				entry := LogAggEntry{
					Name: pod.Name, Namespace: pod.Namespace,
					Container: cs.Name, RestartCount: int(cs.RestartCount),
				}
				switch {
				case cs.RestartCount >= 20:
					entry.RiskLevel = "critical"
					entry.Issue = fmt.Sprintf("extreme restart count (%d) - likely CrashLoopBackOff", cs.RestartCount)
					result.Summary.HighRestartRate++
					result.Summary.NoisyLoggers++
					nsE.NoisyCount++
					result.NoisyLoggers = append(result.NoisyLoggers, entry)
				case cs.RestartCount >= 5:
					entry.RiskLevel = "high"
					entry.Issue = fmt.Sprintf("high restart count (%d) - excessive log generation", cs.RestartCount)
					result.Summary.NoisyLoggers++
					nsE.NoisyCount++
					result.NoisyLoggers = append(result.NoisyLoggers, entry)
				case cs.RestartCount >= 1:
					entry.RiskLevel = "low"
				}
			}
		}
	}

	for _, ns := range nsMap {
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].NoisyCount > result.ByNamespace[j].NoisyCount
	})

	// Score
	if result.Summary.TotalContainers > 0 {
		healthyPct := (result.Summary.TotalContainers - result.Summary.HighRestartRate) * 100 / result.Summary.TotalContainers
		result.HealthScore = healthyPct
	}
	gradeFromScore(&result.Grade, result.HealthScore)
	result.Recommendations = buildLogAggRecs1901(&result)
	writeJSON(w, result)
}

func buildLogAggRecs1901(r *LogAggHealthResult) []string {
	recs := []string{fmt.Sprintf("Log aggregation: %d containers, %d noisy, %d high-restart, %d total restarts",
		r.Summary.TotalContainers, r.Summary.NoisyLoggers, r.Summary.HighRestartRate, r.Summary.TotalRestarts)}
	if r.Summary.HighRestartRate > 0 {
		recs = append(recs, fmt.Sprintf("%d containers with extreme restart rates - investigate CrashLoopBackOff root cause", r.Summary.HighRestartRate))
	}
	return recs
}
