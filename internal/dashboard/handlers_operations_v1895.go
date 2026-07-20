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
// v18.95 — Operations Dimension
// 1. Pod Phase Timeline Analysis
// 2. Image GC Pressure Monitor
// 3. Controller Reconcile Health
// ============================================================

// ---------------------------------------------------------------
// 1. Pod Phase Timeline — analyzes pod lifecycle phase distribution
// ---------------------------------------------------------------

type PodPhaseTimelineResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         PodPhaseSummary       `json:"summary"`
	PhaseBreakdown  map[string]int        `json:"phaseBreakdown"`
	StalePods       []PodPhaseEntry       `json:"stalePods"`
	PendingPods     []PodPhaseEntry       `json:"pendingPods"`
	OldRunningPods  []PodPhaseEntry       `json:"oldRunningPods"`
	NamespacePhase  []NamespacePhaseEntry `json:"namespacePhase"`
	Recommendations []string              `json:"recommendations"`
}

type PodPhaseSummary struct {
	TotalPods      int `json:"totalPods"`
	Running        int `json:"running"`
	Pending        int `json:"pending"`
	Failed         int `json:"failed"`
	Succeeded      int `json:"succeeded"`
	Unknown        int `json:"unknown"`
	StalePods      int `json:"stalePods"`
	LongPending    int `json:"longPending"`
	VeryOldRunning int `json:"veryOldRunning"`
	AvgPodAgeHours int `json:"avgPodAgeHours"`
}

type PodPhaseEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Phase     string `json:"phase"`
	AgeHours  int    `json:"ageHours"`
	NodeName  string `json:"nodeName"`
	Ready     string `json:"ready"`
	RiskLevel string `json:"riskLevel"`
	Issue     string `json:"issue"`
}

type NamespacePhaseEntry struct {
	Namespace string         `json:"namespace"`
	Phases    map[string]int `json:"phases"`
	TotalPods int            `json:"totalPods"`
}

func (s *Server) handlePodPhaseTimeline(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := PodPhaseTimelineResult{
		ScannedAt:      time.Now(),
		PhaseBreakdown: map[string]int{},
	}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	now := time.Now()
	totalAge := 0
	nsPhaseMap := map[string]map[string]int{}

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		result.Summary.TotalPods++
		phase := string(pod.Status.Phase)
		result.PhaseBreakdown[phase]++

		// Track namespace
		if nsPhaseMap[pod.Namespace] == nil {
			nsPhaseMap[pod.Namespace] = map[string]int{}
		}
		nsPhaseMap[pod.Namespace][phase]++

		// Calculate age
		ageHours := 0
		if !pod.CreationTimestamp.IsZero() {
			ageHours = int(now.Sub(pod.CreationTimestamp.Time).Hours())
			totalAge += ageHours
		}

		switch phase {
		case "Running":
			result.Summary.Running++
			// Check for very old pods (possible orphans)
			if ageHours > 720 { // 30 days
				result.Summary.VeryOldRunning++
				result.OldRunningPods = append(result.OldRunningPods, PodPhaseEntry{
					Name: pod.Name, Namespace: pod.Namespace, Phase: phase,
					AgeHours: ageHours, NodeName: pod.Spec.NodeName,
					Ready:     podReadyStr1895(&pod),
					RiskLevel: "medium",
					Issue:     fmt.Sprintf("running for %d days without restart - verify this is expected", ageHours/24),
				})
			}
		case "Pending":
			result.Summary.Pending++
			// Check for long pending
			if ageHours > 1 {
				result.Summary.LongPending++
				result.PendingPods = append(result.PendingPods, PodPhaseEntry{
					Name: pod.Name, Namespace: pod.Namespace, Phase: phase,
					AgeHours:  ageHours,
					RiskLevel: "high",
					Issue:     fmt.Sprintf("pending for %d hours - may be stuck scheduling", ageHours),
				})
			}
		case "Failed":
			result.Summary.Failed++
		case "Succeeded":
			result.Summary.Succeeded++
		case "Unknown":
			result.Summary.Unknown++
		}

		// Stale pods: terminated but not cleaned up
		if phase == "Failed" || phase == "Succeeded" {
			if ageHours > 24 {
				result.Summary.StalePods++
				result.StalePods = append(result.StalePods, PodPhaseEntry{
					Name: pod.Name, Namespace: pod.Namespace, Phase: phase,
					AgeHours:  ageHours,
					RiskLevel: "low",
					Issue:     fmt.Sprintf("completed %d hours ago but not cleaned up", ageHours),
				})
			}
		}
	}

	// Average pod age
	if result.Summary.TotalPods > 0 {
		result.Summary.AvgPodAgeHours = totalAge / result.Summary.TotalPods
	}

	// Namespace phase map
	for ns, phases := range nsPhaseMap {
		total := 0
		for _, c := range phases {
			total += c
		}
		result.NamespacePhase = append(result.NamespacePhase, NamespacePhaseEntry{
			Namespace: ns, Phases: phases, TotalPods: total,
		})
	}
	sort.Slice(result.NamespacePhase, func(i, j int) bool {
		return result.NamespacePhase[i].TotalPods > result.NamespacePhase[j].TotalPods
	})

	// Score: based on healthy pod ratio
	if result.Summary.TotalPods > 0 {
		healthy := result.Summary.Running + result.Summary.Succeeded
		result.HealthScore = healthy * 100 / result.Summary.TotalPods
		if result.Summary.LongPending > 0 {
			result.HealthScore -= result.Summary.LongPending * 5
		}
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = buildPodPhaseRecs1895(&result)
	writeJSON(w, result)
}

func podReadyStr1895(pod *corev1.Pod) string {
	ready, total := 0, 0
	for _, cs := range pod.Status.ContainerStatuses {
		total++
		if cs.Ready {
			ready++
		}
	}
	return fmt.Sprintf("%d/%d", ready, total)
}

func buildPodPhaseRecs1895(result *PodPhaseTimelineResult) []string {
	recs := []string{
		fmt.Sprintf("Pod phase health: %d pods (%d running, %d pending, %d failed), avg age %dh",
			result.Summary.TotalPods, result.Summary.Running,
			result.Summary.Pending, result.Summary.Failed,
			result.Summary.AvgPodAgeHours),
	}
	if result.Summary.LongPending > 0 {
		recs = append(recs, fmt.Sprintf("%d pods pending for >1 hour - check resource availability and scheduling constraints", result.Summary.LongPending))
	}
	if result.Summary.StalePods > 0 {
		recs = append(recs, fmt.Sprintf("%d stale completed pods not cleaned up - configure TTL or cleanup job", result.Summary.StalePods))
	}
	if result.Summary.Failed > 0 {
		recs = append(recs, fmt.Sprintf("%d failed pods - investigate and clean up", result.Summary.Failed))
	}
	return recs
}

// ---------------------------------------------------------------
// 2. Image GC Pressure — disk usage and image garbage collection
// ---------------------------------------------------------------

type ImageGCResult struct {
	ScannedAt        time.Time       `json:"scannedAt"`
	HealthScore      int             `json:"healthScore"`
	Grade            string          `json:"grade"`
	Summary          ImageGCSummary  `json:"summary"`
	ImagesByRegistry map[string]int  `json:"imagesByRegistry"`
	DuplicateImages  []ImageGCEntity `json:"duplicateImages"`
	UnusedImages     []ImageGCEntity `json:"unusedImages"`
	LargeImages      []ImageGCEntity `json:"largeImages"`
	NodeDiskUsage    []NodeDiskEntry `json:"nodeDiskUsage"`
	Recommendations  []string        `json:"recommendations"`
}

type ImageGCSummary struct {
	TotalImages        int `json:"totalImages"`
	UniqueImages       int `json:"uniqueImages"`
	DuplicateCount     int `json:"duplicateCount"`
	UnusedCount        int `json:"unusedCount"`
	TotalRegistries    int `json:"totalRegistries"`
	EstimatedSavingsMB int `json:"estimatedSavingsMB"`
}

type ImageGCEntity struct {
	Image      string `json:"image"`
	Registry   string `json:"registry"`
	PodCount   int    `json:"podCount"`
	Namespaces int    `json:"namespaces"`
	IsUsed     bool   `json:"isUsed"`
	RiskLevel  string `json:"riskLevel"`
}

type NodeDiskEntry struct {
	Node            string `json:"node"`
	DiskPressure    bool   `json:"diskPressure"`
	ImageGCPressure bool   `json:"imageGCPressure"`
	Condition       string `json:"condition"`
}

func (s *Server) handleImageGCPressure(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := ImageGCResult{ScannedAt: time.Now()}

	// Collect images used by running pods
	usedImages := map[string]*ImageGCEntity{} // image -> entity
	imageRegistryMap := map[string]int{}
	imageNamespaces := map[string]map[string]bool{}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		if pod.Status.Phase != "Running" && pod.Status.Phase != "Pending" {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalImages++
			img := c.Image

			if usedImages[img] == nil {
				usedImages[img] = &ImageGCEntity{Image: img, IsUsed: true}
				imageNamespaces[img] = map[string]bool{}
				registry := extractRegistry1892(img)
				usedImages[img].Registry = registry
				imageRegistryMap[registry]++
			}
			usedImages[img].PodCount++
			imageNamespaces[img][pod.Namespace] = true
		}
	}

	// Finalize namespace counts and detect duplicates (same base image, different tags)
	imageList := make([]*ImageGCEntity, 0, len(usedImages))
	for img, ent := range usedImages {
		ent.Namespaces = len(imageNamespaces[img])
		imageList = append(imageList, ent)
	}

	// Detect potential duplicates (same image name different tags)
	baseImageMap := map[string][]*ImageGCEntity{}
	for _, ent := range imageList {
		base := ent.Image
		if idx := strings.LastIndex(base, ":"); idx > 0 {
			base = base[:idx]
		}
		baseImageMap[base] = append(baseImageMap[base], ent)
	}
	for _, variants := range baseImageMap {
		if len(variants) > 1 {
			result.Summary.DuplicateCount += len(variants)
			for _, v := range variants {
				v.RiskLevel = "medium"
				result.DuplicateImages = append(result.DuplicateImages, *v)
			}
		}
	}

	// Estimate unused images (from nodes - images not referenced by any pod)
	// We check for image IDs in container statuses that differ from spec
	nodeImages := map[string]map[string]bool{} // node -> set of images
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Spec.NodeName == "" {
			continue
		}
		if nodeImages[pod.Spec.NodeName] == nil {
			nodeImages[pod.Spec.NodeName] = map[string]bool{}
		}
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.ImageID != "" {
				nodeImages[pod.Spec.NodeName][cs.ImageID] = true
			}
		}
	}

	// Analyze node disk conditions
	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	for _, node := range nodes.Items {
		entry := NodeDiskEntry{Node: node.Name}
		for _, cond := range node.Status.Conditions {
			switch cond.Type {
			case corev1.NodeDiskPressure:
				entry.DiskPressure = cond.Status == corev1.ConditionTrue
			}
		}
		imgCount := len(nodeImages[node.Name])
		entry.ImageGCPressure = imgCount > 50
		if entry.DiskPressure {
			entry.Condition = "DiskPressure - critical disk usage"
		} else if entry.ImageGCPressure {
			entry.Condition = fmt.Sprintf("High image count (%d) - GC pressure building", imgCount)
		} else {
			entry.Condition = fmt.Sprintf("Healthy - %d images cached", imgCount)
		}
		result.NodeDiskUsage = append(result.NodeDiskUsage, entry)
	}

	result.Summary.UniqueImages = len(usedImages)
	result.Summary.TotalRegistries = len(imageRegistryMap)
	result.ImagesByRegistry = imageRegistryMap
	result.Summary.EstimatedSavingsMB = result.Summary.DuplicateCount * 200 // ~200MB per duplicate image

	// Score
	if result.Summary.TotalImages > 0 {
		dupPct := 100
		if result.Summary.UniqueImages > 0 {
			dupPct = (result.Summary.UniqueImages) * 100 / result.Summary.TotalImages
		}
		result.HealthScore = dupPct
		// Penalty for disk pressure nodes
		diskPressureCount := 0
		for _, nd := range result.NodeDiskUsage {
			if nd.DiskPressure {
				diskPressureCount++
			}
		}
		result.HealthScore -= diskPressureCount * 20
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = buildImageGCRecs1895(&result)
	writeJSON(w, result)
}

func buildImageGCRecs1895(result *ImageGCResult) []string {
	recs := []string{
		fmt.Sprintf("Image GC: %d total images, %d unique, %d duplicates across %d registries",
			result.Summary.TotalImages, result.Summary.UniqueImages,
			result.Summary.DuplicateCount, result.Summary.TotalRegistries),
	}
	if result.Summary.DuplicateCount > 0 {
		recs = append(recs, fmt.Sprintf("%d duplicate image variants - standardize on single tag to reduce disk usage (~%dMB savings)",
			result.Summary.DuplicateCount, result.Summary.EstimatedSavingsMB))
	}
	for _, nd := range result.NodeDiskUsage {
		if nd.DiskPressure {
			recs = append(recs, fmt.Sprintf("Node %s has DiskPressure - run 'crictl rmi --prune' to clean unused images", nd.Node))
		}
	}
	return recs
}

// ---------------------------------------------------------------
// 3. Controller Reconcile Health — analyzes controller patterns
// ---------------------------------------------------------------

type ControllerReconcileResult struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         ReconcileSummary  `json:"summary"`
	ByController    []ReconcileEntry  `json:"byController"`
	Unhealthy       []ReconcileEntry  `json:"unhealthy"`
	OwnerChain      []OwnerChainEntry `json:"ownerChain"`
	Orphaned        []OrphanedEntry   `json:"orphaned"`
	Recommendations []string          `json:"recommendations"`
}

type ReconcileSummary struct {
	TotalWorkloads       int `json:"totalWorkloads"`
	WithOwner            int `json:"withOwner"`
	WithoutOwner         int `json:"withoutOwner"`
	HealthyControllers   int `json:"healthyControllers"`
	UnhealthyControllers int `json:"unhealthyControllers"`
	OrphanedWorkloads    int `json:"orphanedWorkloads"`
	DesiredReplicas      int `json:"desiredReplicas"`
	ActualReplicas       int `json:"actualReplicas"`
	Mismatch             int `json:"replicaMismatch"`
}

type ReconcileEntry struct {
	Name              string   `json:"name"`
	Namespace         string   `json:"namespace"`
	Kind              string   `json:"kind"`
	OwnerKind         string   `json:"ownerKind"`
	OwnerName         string   `json:"ownerName"`
	DesiredReplicas   int32    `json:"desiredReplicas"`
	ActualReplicas    int32    `json:"actualReplicas"`
	AvailableReplicas int32    `json:"availableReplicas"`
	Conditions        []string `json:"conditions"`
	RiskLevel         string   `json:"riskLevel"`
	Issue             string   `json:"issue"`
}

type OwnerChainEntry struct {
	Resource   string `json:"resource"`
	Namespace  string `json:"namespace"`
	Kind       string `json:"kind"`
	OwnerChain string `json:"ownerChain"`
}

type OrphanedEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	RiskLevel string `json:"riskLevel"`
}

func (s *Server) handleControllerReconcile(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := ControllerReconcileResult{ScannedAt: time.Now()}

	// Analyze deployments
	deps, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, dep := range deps.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalWorkloads++

		entry := ReconcileEntry{
			Name:      dep.Name,
			Namespace: dep.Namespace,
			Kind:      "Deployment",
		}

		desired := int32(1)
		if dep.Spec.Replicas != nil {
			desired = *dep.Spec.Replicas
		}
		entry.DesiredReplicas = desired
		entry.ActualReplicas = dep.Status.Replicas
		entry.AvailableReplicas = dep.Status.AvailableReplicas
		result.Summary.DesiredReplicas += int(desired)
		result.Summary.ActualReplicas += int(dep.Status.Replicas)

		// Check owner reference (should have none for top-level Deployment, but may be managed by Helm/ArgoCD)
		if len(dep.OwnerReferences) > 0 {
			entry.OwnerKind = dep.OwnerReferences[0].Kind
			entry.OwnerName = dep.OwnerReferences[0].Name
			result.Summary.WithOwner++
		} else {
			result.Summary.WithoutOwner++
		}

		// Check conditions
		issues := []string{}
		for _, cond := range dep.Status.Conditions {
			if cond.Status == "False" || cond.Type == "Progressing" && cond.Reason != "NewReplicaSetAvailable" {
				if cond.Status == "False" && cond.Type == "Available" {
					issues = append(issues, "Deployment not Available")
				}
				if cond.Status == "True" && cond.Type == "Progressing" && cond.Reason != "NewReplicaSetAvailable" {
					issues = append(issues, "rollout in progress: "+cond.Reason)
				}
			}
		}

		// Replica mismatch
		if desired != dep.Status.Replicas {
			result.Summary.Mismatch++
			issues = append(issues, fmt.Sprintf("replica mismatch: desired=%d actual=%d", desired, dep.Status.Replicas))
		}

		if len(issues) > 0 {
			entry.Issue = strings.Join(issues, "; ")
			entry.RiskLevel = "medium"
			entry.Conditions = issues
			result.Summary.UnhealthyControllers++
			result.Unhealthy = append(result.Unhealthy, entry)
		} else {
			entry.RiskLevel = "low"
			result.Summary.HealthyControllers++
		}
		result.ByController = append(result.ByController, entry)
	}

	// Check ReplicaSet orphan patterns
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	podOwners := map[string]string{} // pod -> owner kind
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) {
			continue
		}
		ownerKind := getOwnerKind(pod.OwnerReferences)
		if ownerKind == "" {
			// Pod without owner = orphaned
			result.Summary.OrphanedWorkloads++
			result.Orphaned = append(result.Orphaned, OrphanedEntry{
				Name: pod.Name, Namespace: pod.Namespace, Kind: "Pod",
				RiskLevel: "medium",
			})
		}
		podOwners[pod.Namespace+"/"+pod.Name] = ownerKind
	}

	// Score
	if result.Summary.TotalWorkloads > 0 {
		healthyPct := result.Summary.HealthyControllers * 100 / result.Summary.TotalWorkloads
		mismatchPenalty := result.Summary.Mismatch * 3
		orphanPenalty := result.Summary.OrphanedWorkloads * 2
		result.HealthScore = healthyPct - mismatchPenalty - orphanPenalty
		if result.HealthScore < 0 {
			result.HealthScore = 0
		}
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = buildReconcileRecs1895(&result)
	writeJSON(w, result)
}

func buildReconcileRecs1895(result *ControllerReconcileResult) []string {
	recs := []string{
		fmt.Sprintf("Controller reconcile: %d workloads (%d healthy, %d unhealthy), %d replica mismatches, %d orphans",
			result.Summary.TotalWorkloads, result.Summary.HealthyControllers,
			result.Summary.UnhealthyControllers, result.Summary.Mismatch,
			result.Summary.OrphanedWorkloads),
	}
	if result.Summary.UnhealthyControllers > 0 {
		recs = append(recs, fmt.Sprintf("%d controllers with reconcile issues - check status conditions and events", result.Summary.UnhealthyControllers))
	}
	if result.Summary.Mismatch > 0 {
		recs = append(recs, fmt.Sprintf("%d controllers with desired != actual replicas - may indicate scheduling or resource constraints", result.Summary.Mismatch))
	}
	if result.Summary.OrphanedWorkloads > 0 {
		recs = append(recs, fmt.Sprintf("%d orphaned pods without owner reference - clean up or wrap in workload controller", result.Summary.OrphanedWorkloads))
	}
	return recs
}
