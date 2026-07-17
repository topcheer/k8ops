package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// PodDisruptionToleranceResult analyzes how well the cluster tolerates
// both voluntary (drains, maintenance) and involuntary (node failure)
// disruptions, computing recovery time and data loss risk.
type PodDisruptionToleranceResult struct {
	ScannedAt       time.Time                  `json:"scannedAt"`
	Summary         DisruptionToleranceSummary `json:"summary"`
	ByWorkload      []ToleranceEntry           `json:"byWorkload"`
	VoluntaryRisk   []ToleranceEntry           `json:"voluntaryRisk"`
	InvoluntaryRisk []ToleranceEntry           `json:"involuntaryRisk"`
	ToleranceScore  int                        `json:"toleranceScore"`
	Grade           string                     `json:"grade"`
	Recommendations []string                   `json:"recommendations"`
}

type DisruptionToleranceSummary struct {
	TotalWorkloads   int `json:"totalWorkloads"`
	MultiReplica     int `json:"multiReplica"`
	SingleReplica    int `json:"singleReplica"`
	WithPDB          int `json:"withPDB"`
	WithAntiAffinity int `json:"withAntiAffinity"`
	AcrossNodes      int `json:"spreadAcrossNodes"`
	AvgRecoveryTime  int `json:"avgRecoveryTimeSec"`
	DataLossRiskWL   int `json:"dataLossRiskWorkloads"`
}

type ToleranceEntry struct {
	Workload         string `json:"workload"`
	Namespace        string `json:"namespace"`
	Kind             string `json:"kind"`
	Replicas         int    `json:"replicas"`
	HasPDB           bool   `json:"hasPDB"`
	HasAntiAffinity  bool   `json:"hasAntiAffinity"`
	NodeSpread       int    `json:"nodeSpread"`
	VoluntaryScore   int    `json:"voluntaryScore"`
	InvoluntaryScore int    `json:"involuntaryScore"`
	RecoveryTime     int    `json:"recoveryTimeSec"`
	DataLossRisk     bool   `json:"dataLossRisk"`
	ToleranceLevel   string `json:"toleranceLevel"`
}

// handlePodDisruptionTolerance handles GET /api/scalability/pod-disruption-tolerance
func (s *Server) handlePodDisruptionTolerance(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := PodDisruptionToleranceResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	sts, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})

	// Build PDB lookup
	pdbMap := make(map[string]bool)
	for _, pdb := range pdbs.Items {
		pdbMap[pdb.Namespace+"/"+pdb.Name] = true
	}

	// Build workload -> nodes map
	wlNodes := make(map[string]map[string]bool)
	wlReplicas := make(map[string]int)
	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		wlName := ""
		for _, ref := range pod.OwnerReferences {
			if ref.Controller != nil && *ref.Controller {
				wlName = ref.Name
				break
			}
		}
		if wlName == "" {
			continue
		}
		key := pod.Namespace + "/" + wlName
		if wlNodes[key] == nil {
			wlNodes[key] = make(map[string]bool)
		}
		if pod.Spec.NodeName != "" {
			wlNodes[key][pod.Spec.NodeName] = true
		}
		wlReplicas[key]++
	}

	// Analyze Deployments
	analyzeTolerance := func(name, namespace, kind string, replicas int, hasPDBFlag bool, hasAntiAff bool) ToleranceEntry {
		key := namespace + "/" + name
		entry := ToleranceEntry{
			Workload:        name,
			Namespace:       namespace,
			Kind:            kind,
			Replicas:        replicas,
			HasPDB:          hasPDBFlag,
			HasAntiAffinity: hasAntiAff,
		}

		nodeSet := wlNodes[key]
		entry.NodeSpread = len(nodeSet)

		// Voluntary disruption score (0-100, higher = more tolerant)
		volScore := 0
		if replicas >= 3 {
			volScore += 30
		} else if replicas >= 2 {
			volScore += 15
		}
		if hasPDBFlag {
			volScore += 30
		}
		if entry.NodeSpread >= 2 {
			volScore += 20
		}
		if hasAntiAff {
			volScore += 20
		}
		entry.VoluntaryScore = volScore

		// Involuntary disruption score
		involScore := 0
		if replicas >= 3 {
			involScore += 35
		} else if replicas >= 2 {
			involScore += 20
		}
		if entry.NodeSpread >= 3 {
			involScore += 35
		} else if entry.NodeSpread >= 2 {
			involScore += 20
		}
		if hasAntiAff {
			involScore += 15
		}
		if hasPDBFlag {
			involScore += 15
		}
		entry.InvoluntaryScore = involScore

		// Recovery time estimate
		switch {
		case replicas >= 3 && entry.NodeSpread >= 2:
			entry.RecoveryTime = 5
		case replicas >= 2:
			entry.RecoveryTime = 30
		case replicas == 1:
			entry.RecoveryTime = 120
		default:
			entry.RecoveryTime = 300
		}

		// Data loss risk for StatefulSets with single replica
		if kind == "StatefulSet" && replicas <= 1 {
			entry.DataLossRisk = true
		}

		// Tolerance level
		avgScore := (volScore + involScore) / 2
		switch {
		case avgScore >= 70:
			entry.ToleranceLevel = "high"
		case avgScore >= 40:
			entry.ToleranceLevel = "medium"
		case avgScore >= 20:
			entry.ToleranceLevel = "low"
		default:
			entry.ToleranceLevel = "none"
		}

		return entry
	}

	var entries []ToleranceEntry

	for _, d := range deployments.Items {
		if isSystemNamespace(d.Namespace) {
			continue
		}
		replicas := 1
		if d.Spec.Replicas != nil {
			replicas = int(*d.Spec.Replicas)
		}
		result.Summary.TotalWorkloads++
		if replicas > 1 {
			result.Summary.MultiReplica++
		} else {
			result.Summary.SingleReplica++
		}

		// Check anti-affinity
		hasAntiAff := false
		if d.Spec.Template.Spec.Affinity != nil && d.Spec.Template.Spec.Affinity.PodAntiAffinity != nil {
			hasAntiAff = true
			result.Summary.WithAntiAffinity++
		}

		hasPDB := pdbMap[d.Namespace+"/"+d.Name]
		if hasPDB {
			result.Summary.WithPDB++
		}

		entry := analyzeTolerance(d.Name, d.Namespace, "Deployment", replicas, hasPDB, hasAntiAff)
		if entry.NodeSpread > 1 {
			result.Summary.AcrossNodes++
		}
		entries = append(entries, entry)
	}

	for _, ss := range sts.Items {
		if isSystemNamespace(ss.Namespace) {
			continue
		}
		replicas := 1
		if ss.Spec.Replicas != nil {
			replicas = int(*ss.Spec.Replicas)
		}
		result.Summary.TotalWorkloads++
		if replicas > 1 {
			result.Summary.MultiReplica++
		} else {
			result.Summary.SingleReplica++
		}

		hasAntiAff := false
		if ss.Spec.Template.Spec.Affinity != nil && ss.Spec.Template.Spec.Affinity.PodAntiAffinity != nil {
			hasAntiAff = true
			result.Summary.WithAntiAffinity++
		}

		hasPDB := pdbMap[ss.Namespace+"/"+ss.Name]
		if hasPDB {
			result.Summary.WithPDB++
		}

		entry := analyzeTolerance(ss.Name, ss.Namespace, "StatefulSet", replicas, hasPDB, hasAntiAff)
		if entry.DataLossRisk {
			result.Summary.DataLossRiskWL++
		}
		entries = append(entries, entry)
	}

	// Sort by tolerance level ascending (worst first)
	tolRank := map[string]int{"none": 0, "low": 1, "medium": 2, "high": 3}
	sort.Slice(entries, func(i, j int) bool {
		return tolRank[entries[i].ToleranceLevel] < tolRank[entries[j].ToleranceLevel]
	})
	result.ByWorkload = entries

	// Collect at-risk workloads
	for _, e := range entries {
		if e.VoluntaryScore < 40 {
			result.VoluntaryRisk = append(result.VoluntaryRisk, e)
		}
		if e.InvoluntaryScore < 40 {
			result.InvoluntaryRisk = append(result.InvoluntaryRisk, e)
		}
	}

	// Average recovery time
	if len(entries) > 0 {
		totalRT := 0
		for _, e := range entries {
			totalRT += e.RecoveryTime
		}
		result.Summary.AvgRecoveryTime = totalRT / len(entries)
	}

	// Tolerance score
	if result.Summary.TotalWorkloads > 0 {
		highTol := 0
		for _, e := range entries {
			if e.ToleranceLevel == "high" || e.ToleranceLevel == "medium" {
				highTol++
			}
		}
		result.ToleranceScore = highTol * 100 / result.Summary.TotalWorkloads
	}

	switch {
	case result.ToleranceScore >= 80:
		result.Grade = "A"
	case result.ToleranceScore >= 60:
		result.Grade = "B"
	case result.ToleranceScore >= 40:
		result.Grade = "C"
	case result.ToleranceScore >= 20:
		result.Grade = "D"
	default:
		result.Grade = "F"
	}

	result.Recommendations = buildDisruptionToleranceRecs(&result)
	writeJSON(w, result)
}

func buildDisruptionToleranceRecs(r *PodDisruptionToleranceResult) []string {
	recs := []string{
		fmt.Sprintf("中断容忍度: %d 工作负载, %d 多副本, %d 单副本, %d 有 PDB", r.Summary.TotalWorkloads, r.Summary.MultiReplica, r.Summary.SingleReplica, r.Summary.WithPDB),
	}
	if r.Summary.SingleReplica > 0 {
		recs = append(recs, fmt.Sprintf("警告: %d 个单副本工作负载在节点故障时完全不可用", r.Summary.SingleReplica))
	}
	if r.Summary.DataLossRiskWL > 0 {
		recs = append(recs, fmt.Sprintf("紧急: %d 个单副本 StatefulSet 存在数据丢失风险", r.Summary.DataLossRiskWL))
	}
	if len(r.VoluntaryRisk) > 0 {
		recs = append(recs, fmt.Sprintf("%d 个工作负载自愿中断容忍度低 (<40分)", len(r.VoluntaryRisk)))
	}
	if len(r.InvoluntaryRisk) > 0 {
		recs = append(recs, fmt.Sprintf("%d 个工作负载非自愿中断容忍度低 (<40分)", len(r.InvoluntaryRisk)))
	}
	if r.ToleranceScore < 60 {
		recs = append(recs, "建议: 增加副本数>=3, 添加 podAntiAffinity, 创建 PDB")
	}
	return recs
}
