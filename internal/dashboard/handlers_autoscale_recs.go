package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// AutoscaleRecommendationResult is the full autoscaling right-sizing analysis.
type AutoscaleRecommendationResult struct {
	ScannedAt         time.Time            `json:"scannedAt"`
	Summary           AutoscaleSummary     `json:"summary"`
	Recommendations   []AutoscaleRec       `json:"recommendations"`
	UnscaledWorkloads []UnscaledWorkload   `json:"unscaledWorkloads"`
	HPAEfficiency     []HPAEfficiencyEntry `json:"hpaEfficiency"`
	TopSavings        []SavingsEntry       `json:"topSavings"`
}

// AutoscaleSummary aggregates cluster-wide autoscaling metrics.
type AutoscaleSummary struct {
	TotalWorkloads    int     `json:"totalWorkloads"`
	WithHPA           int     `json:"withHPA"`
	WithoutHPA        int     `json:"withoutHPA"`
	HPAAtMax          int     `json:"hpaAtMax"`         // HPA pegged at maxReplicas
	HPAAtMin          int     `json:"hpaAtMin"`         // HPA pegged at minReplicas
	OverProvisioned   int     `json:"overProvisioned"`  // requests >> usage
	UnderProvisioned  int     `json:"underProvisioned"` // requests << usage
	NoLimits          int     `json:"noLimits"`
	PotentialCPUcores float64 `json:"potentialCPUSavingsCores"`
	PotentialMemGB    float64 `json:"potentialMemSavingsGB"`
	AutoscaleScore    int     `json:"autoscaleScore"` // 0-100
}

// AutoscaleRec is a right-sizing recommendation for one workload.
type AutoscaleRec struct {
	Name             string  `json:"name"`
	Namespace        string  `json:"namespace"`
	Kind             string  `json:"kind"`
	CurrentCPUm      int64   `json:"currentCPUM"` // current request in millicores
	RecommendedCPUm  int64   `json:"recommendedCPUM"`
	CPUChangePct     float64 `json:"cpuChangePct"`
	CurrentMemMB     int64   `json:"currentMemMB"`
	RecommendedMemMB int64   `json:"recommendedMemMB"`
	MemChangePct     float64 `json:"memChangePct"`
	Replicas         int32   `json:"replicas"`
	Reason           string  `json:"reason"`
	Confidence       string  `json:"confidence"` // high / medium / low
}

// UnscaledWorkload is a multi-replica workload without HPA.
type UnscaledWorkload struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Replicas  int32  `json:"replicas"`
	CPUm      int64  `json:"cpuM"` // total request
	MemMB     int64  `json:"memMB"`
	Reason    string `json:"reason"`
}

// HPAEfficiencyEntry describes how well an HPA is configured.
type HPAEfficiencyEntry struct {
	Name            string `json:"name"`
	Namespace       string `json:"namespace"`
	TargetName      string `json:"targetName"`
	MinReplicas     int32  `json:"minReplicas"`
	MaxReplicas     int32  `json:"maxReplicas"`
	CurrentReplicas int32  `json:"currentReplicas"`
	UtilizationPct  int    `json:"utilizationPct"` // current/max * 100
	Status          string `json:"status"`         // optimal / at-max / at-min / idle
	Issue           string `json:"issue,omitempty"`
}

// SavingsEntry summarizes potential resource savings.
type SavingsEntry struct {
	Workload   string `json:"workload"`
	Namespace  string `json:"namespace"`
	CPUSavedm  int64  `json:"cpuSavedM"`
	MemSavedMB int64  `json:"memSavedMB"`
}

// handleAutoscaleRecommendations analyzes HPA coverage and resource right-sizing.
// GET /api/scalability/autoscale-recommendations
func (s *Server) handleAutoscaleRecommendations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")

	deployments, _ := rc.clientset.AppsV1().Deployments(ns).List(ctx, metav1.ListOptions{})
	statefulsets, _ := rc.clientset.AppsV1().StatefulSets(ns).List(ctx, metav1.ListOptions{})
	hpas, _ := rc.clientset.AutoscalingV2().HorizontalPodAutoscalers(ns).List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})

	result := AutoscaleRecommendationResult{ScannedAt: time.Now()}

	// Build HPA lookup: ns/name → HPA
	hpaMap := make(map[string]*autoscalingv2.HorizontalPodAutoscaler)
	for i := range hpas.Items {
		key := fmt.Sprintf("%s/%s", hpas.Items[i].Namespace, hpas.Items[i].Spec.ScaleTargetRef.Name)
		hpaMap[key] = &hpas.Items[i]
	}

	// Build pod resource usage map (from actual pod specs)
	podUsage := buildPodUsageMap(pods)

	totalPotentialCPUSaved := float64(0)
	totalPotentialMemSaved := float64(0)

	// Process deployments
	for i := range deployments.Items {
		d := &deployments.Items[i]
		rec, unscaled := analyzeWorkloadAutoscale("Deployment", d.Name, d.Namespace,
			d.Spec.Replicas,
			d.Spec.Template.Spec.Containers, d.Spec.Selector, hpaMap, podUsage)
		result.Summary.TotalWorkloads++

		key := fmt.Sprintf("%s/%s", d.Namespace, d.Name)
		if _, hasHPA := hpaMap[key]; hasHPA {
			result.Summary.WithHPA++
		} else {
			result.Summary.WithoutHPA++
		}

		if rec != nil {
			result.Recommendations = append(result.Recommendations, *rec)
			totalPotentialCPUSaved += float64(rec.CurrentCPUm-rec.RecommendedCPUm) * float64(rec.Replicas) / 1000
			totalPotentialMemSaved += float64(rec.CurrentMemMB-rec.RecommendedMemMB) * float64(rec.Replicas) / 1024
		}
		if unscaled != nil {
			result.UnscaledWorkloads = append(result.UnscaledWorkloads, *unscaled)
		}
	}

	// Process statefulsets
	for i := range statefulsets.Items {
		sts := &statefulsets.Items[i]
		rec, unscaled := analyzeWorkloadAutoscale("StatefulSet", sts.Name, sts.Namespace,
			sts.Spec.Replicas,
			sts.Spec.Template.Spec.Containers, sts.Spec.Selector, hpaMap, podUsage)
		result.Summary.TotalWorkloads++

		key := fmt.Sprintf("%s/%s", sts.Namespace, sts.Name)
		if _, hasHPA := hpaMap[key]; hasHPA {
			result.Summary.WithHPA++
		} else {
			result.Summary.WithoutHPA++
		}

		if rec != nil {
			result.Recommendations = append(result.Recommendations, *rec)
		}
		if unscaled != nil {
			result.UnscaledWorkloads = append(result.UnscaledWorkloads, *unscaled)
		}
	}

	// Analyze HPA efficiency
	for i := range hpas.Items {
		hpa := &hpas.Items[i]
		entry := analyzeHPAEfficiency(hpa)
		result.HPAEfficiency = append(result.HPAEfficiency, entry)

		if entry.Status == "at-max" {
			result.Summary.HPAAtMax++
		}
		if entry.Status == "at-min" {
			result.Summary.HPAAtMin++
		}
	}

	// Count over/under provisioned
	for _, rec := range result.Recommendations {
		if rec.CPUChangePct < -20 || rec.MemChangePct < -20 {
			result.Summary.OverProvisioned++
		}
		if rec.CPUChangePct > 20 || rec.MemChangePct > 20 {
			result.Summary.UnderProvisioned++
		}
	}

	result.Summary.PotentialCPUcores = totalPotentialCPUSaved
	result.Summary.PotentialMemGB = totalPotentialMemSaved
	result.Summary.AutoscaleScore = calculateAutoscaleScore(result.Summary)

	// Sort recommendations by potential savings
	sort.Slice(result.Recommendations, func(i, j int) bool {
		return absFloat64(result.Recommendations[i].CPUChangePct) > absFloat64(result.Recommendations[j].CPUChangePct)
	})

	// Sort unscaled by replicas (largest first)
	sort.Slice(result.UnscaledWorkloads, func(i, j int) bool {
		return result.UnscaledWorkloads[i].Replicas > result.UnscaledWorkloads[j].Replicas
	})

	// Build top savings
	for _, rec := range result.Recommendations {
		if rec.CurrentCPUm > rec.RecommendedCPUm || rec.CurrentMemMB > rec.RecommendedMemMB {
			result.TopSavings = append(result.TopSavings, SavingsEntry{
				Workload:   fmt.Sprintf("%s/%s", rec.Namespace, rec.Name),
				Namespace:  rec.Namespace,
				CPUSavedm:  (rec.CurrentCPUm - rec.RecommendedCPUm) * int64(rec.Replicas),
				MemSavedMB: (rec.CurrentMemMB - rec.RecommendedMemMB) * int64(rec.Replicas),
			})
		}
	}
	sort.Slice(result.TopSavings, func(i, j int) bool {
		return result.TopSavings[i].CPUSavedm > result.TopSavings[j].CPUSavedm
	})
	if len(result.TopSavings) > 20 {
		result.TopSavings = result.TopSavings[:20]
	}

	writeJSON(w, result)
}

// buildPodUsageMap builds a map of pod name → resource requests for actual usage estimation.
func buildPodUsageMap(pods *corev1.PodList) map[string]podResourceUsage {
	m := make(map[string]podResourceUsage)
	if pods == nil {
		return m
	}
	for _, pod := range pods.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		usage := podResourceUsage{}
		for _, c := range pod.Spec.Containers {
			if r := c.Resources.Requests.Cpu(); r != nil {
				usage.cpuM += r.MilliValue()
			}
			if r := c.Resources.Requests.Memory(); r != nil {
				usage.memMB += r.Value() / (1024 * 1024)
			}
			if c.Resources.Limits.Cpu().IsZero() {
				usage.noCPULimit = true
			}
			if c.Resources.Limits.Memory().IsZero() {
				usage.noMemLimit = true
			}
		}
		m[pod.Name] = usage
	}
	return m
}

type podResourceUsage struct {
	cpuM       int64
	memMB      int64
	noCPULimit bool
	noMemLimit bool
}

// analyzeWorkloadAutoscale checks if a workload needs right-sizing or HPA.
func analyzeWorkloadAutoscale(kind, name, namespace string, replicas *int32,
	containers []corev1.Container, selector *metav1.LabelSelector,
	hpaMap map[string]*autoscalingv2.HorizontalPodAutoscaler,
	podUsage map[string]podResourceUsage) (*AutoscaleRec, *UnscaledWorkload) {

	key := fmt.Sprintf("%s/%s", namespace, name)
	_, hasHPA := hpaMap[key]

	repCount := int32(1)
	if replicas != nil {
		repCount = *replicas
	}

	// Calculate total container requests
	totalCPUm := int64(0)
	totalMemMB := int64(0)
	hasNoLimit := false
	for _, c := range containers {
		if r := c.Resources.Requests.Cpu(); r != nil && !r.IsZero() {
			totalCPUm += r.MilliValue()
		}
		if r := c.Resources.Requests.Memory(); r != nil && !r.IsZero() {
			totalMemMB += r.Value() / (1024 * 1024)
		}
		if c.Resources.Limits.Cpu().IsZero() || c.Resources.Limits.Memory().IsZero() {
			hasNoLimit = true
		}
	}

	// Check for multi-replica without HPA
	var unscaled *UnscaledWorkload
	if !hasHPA && repCount > 1 {
		unscaled = &UnscaledWorkload{
			Name:      name,
			Namespace: namespace,
			Kind:      kind,
			Replicas:  repCount,
			CPUm:      totalCPUm,
			MemMB:     totalMemMB,
			Reason:    fmt.Sprintf("%s with %d replicas has no HPA — consider autoscaling for demand-based scaling", kind, repCount),
		}
	}

	// Right-sizing recommendation (heuristic: high request relative to typical usage)
	// We recommend reducing if request is >500m CPU (0.5 core) for a single container
	perContainerCPU := int64(0)
	perContainerMem := int64(0)
	if len(containers) > 0 {
		perContainerCPU = totalCPUm / int64(len(containers))
		perContainerMem = totalMemMB / int64(len(containers))
	}

	var rec *AutoscaleRec
	recommendedCPU := perContainerCPU
	recommendedMem := perContainerMem
	confidence := "medium"
	reason := ""

	if perContainerCPU > 1000 {
		// >1 CPU core per container — likely over-provisioned
		recommendedCPU = perContainerCPU / 2
		reason = fmt.Sprintf("CPU request %dm/container is high (>1 core) — typical workloads need less", perContainerCPU)
		confidence = "high"
	}
	if perContainerCPU > 500 && recommendedCPU == perContainerCPU {
		recommendedCPU = perContainerCPU * 70 / 100
		reason = fmt.Sprintf("CPU request %dm/container may be over-provisioned — consider right-sizing", perContainerCPU)
	}

	if perContainerMem > 2048 {
		// >2GB per container
		if reason != "" {
			reason += "; "
		}
		recommendedMem = perContainerMem * 70 / 100
		reason += fmt.Sprintf("Memory request %dMB/container is high (>2GB) — consider right-sizing", perContainerMem)
		confidence = "high"
	}

	if hasNoLimit {
		if reason != "" {
			reason += "; "
		}
		reason += "Some containers have no resource limits — add limits to prevent resource starvation"
		if confidence == "medium" {
			confidence = "low"
		}
	}

	if reason != "" {
		cpuChange := float64(0)
		if perContainerCPU > 0 {
			cpuChange = float64(recommendedCPU-perContainerCPU) / float64(perContainerCPU) * 100
		}
		memChange := float64(0)
		if perContainerMem > 0 {
			memChange = float64(recommendedMem-perContainerMem) / float64(perContainerMem) * 100
		}

		rec = &AutoscaleRec{
			Name:             name,
			Namespace:        namespace,
			Kind:             kind,
			CurrentCPUm:      perContainerCPU,
			RecommendedCPUm:  recommendedCPU,
			CPUChangePct:     cpuChange,
			CurrentMemMB:     perContainerMem,
			RecommendedMemMB: recommendedMem,
			MemChangePct:     memChange,
			Replicas:         repCount,
			Reason:           reason,
			Confidence:       confidence,
		}
	}

	return rec, unscaled
}

// analyzeHPAEfficiency evaluates HPA configuration quality.
func analyzeHPAEfficiency(hpa *autoscalingv2.HorizontalPodAutoscaler) HPAEfficiencyEntry {
	entry := HPAEfficiencyEntry{
		Name:            hpa.Name,
		Namespace:       hpa.Namespace,
		TargetName:      hpa.Spec.ScaleTargetRef.Name,
		MinReplicas:     1,
		MaxReplicas:     hpa.Spec.MaxReplicas,
		CurrentReplicas: hpa.Status.CurrentReplicas,
	}

	if hpa.Spec.MinReplicas != nil {
		entry.MinReplicas = *hpa.Spec.MinReplicas
	}

	if entry.MaxReplicas > 0 {
		entry.UtilizationPct = int(float64(entry.CurrentReplicas) / float64(entry.MaxReplicas) * 100)
	}

	switch {
	case entry.CurrentReplicas >= entry.MaxReplicas && entry.MaxReplicas > 0:
		entry.Status = "at-max"
		entry.Issue = fmt.Sprintf("HPA pegged at maxReplicas (%d) — increase maxReplicas or optimize workload", entry.MaxReplicas)
	case entry.CurrentReplicas <= entry.MinReplicas:
		entry.Status = "at-min"
		entry.Issue = fmt.Sprintf("HPA at minReplicas (%d) — verify traffic is truly low", entry.MinReplicas)
	case entry.UtilizationPct < 20 && entry.CurrentReplicas > 1:
		entry.Status = "idle"
		entry.Issue = "HPA under-utilized — consider reducing maxReplicas to save resources"
	default:
		entry.Status = "optimal"
	}

	return entry
}

// calculateAutoscaleScore computes 0-100.
func calculateAutoscaleScore(s AutoscaleSummary) int {
	if s.TotalWorkloads == 0 {
		return 100
	}
	score := 100

	// Low HPA coverage
	hpaCoverage := float64(s.WithHPA) / float64(s.TotalWorkloads) * 100
	if hpaCoverage < 20 {
		score -= 20
	} else if hpaCoverage < 50 {
		score -= 10
	}

	// HPAs at max
	score -= s.HPAAtMax * 5

	// Over-provisioned workloads
	score -= s.OverProvisioned * 3

	// No HPA for multi-replica
	if s.WithoutHPA > 0 {
		noHPAPenalty := s.WithoutHPA * 2
		if noHPAPenalty > 30 {
			noHPAPenalty = 30
		}
		score -= noHPAPenalty
	}

	if score < 0 {
		score = 0
	}
	return score
}

func absFloat64(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
