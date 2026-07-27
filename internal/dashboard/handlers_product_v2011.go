package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v20.11 — Product Dimension (Round 21)
// 1. HPA Target Utilization — autoscaler target config & gap analysis
// 2. Replica Age Distribution — pod age distribution for freshness tracking
// 3. Node Pod Affinity Score — node workload balance scoring
// ============================================================

// ---------------------------------------------------------------
// 1. HPA Target Utilization
// ---------------------------------------------------------------

type HPATargetResult2011 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         HPATargetSummary2011 `json:"summary"`
	HPAs            []HPATargetEntry2011 `json:"hpas"`
	Recommendations []string             `json:"recommendations"`
}

type HPATargetSummary2011 struct {
	TotalHPAs int     `json:"totalHPAs"`
	AvgTarget float64 `json:"avgTargetUtilization"`
	WithCPU   int     `json:"withCPUTarget"`
	WithMem   int     `json:"withMemTarget"`
}

type HPATargetEntry2011 struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	TargetCPU   int32  `json:"targetCPUUtilization"`
	MinReplicas int32  `json:"minReplicas"`
	MaxReplicas int32  `json:"maxReplicas"`
}

func (s *Server) handleHPATargetUtil(w http.ResponseWriter, r *http.Request) {
	result := HPATargetResult2011{ScannedAt: time.Now()}
	score := 100

	hpaList, _ := s.clientset.AutoscalingV1().HorizontalPodAutoscalers("").List(r.Context(), metav1.ListOptions{})

	var totalTarget float64
	for _, hpa := range hpaList.Items {
		result.Summary.TotalHPAs++

		entry := HPATargetEntry2011{
			Name: hpa.Name, Namespace: hpa.Namespace,
			MinReplicas: 1, MaxReplicas: 10,
		}
		if hpa.Spec.MinReplicas != nil {
			entry.MinReplicas = *hpa.Spec.MinReplicas
		}
		if hpa.Spec.MaxReplicas > 0 {
			entry.MaxReplicas = hpa.Spec.MaxReplicas
		}

		if hpa.Spec.TargetCPUUtilizationPercentage != nil {
			entry.TargetCPU = *hpa.Spec.TargetCPUUtilizationPercentage
			totalTarget += float64(entry.TargetCPU)
			result.Summary.WithCPU++
		}

		result.HPAs = append(result.HPAs, entry)
	}

	if result.Summary.TotalHPAs > 0 {
		result.Summary.AvgTarget = totalTarget / float64(result.Summary.TotalHPAs)
	}

	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d HPAs: avg target %.0f%% CPU, %d with CPU target", result.Summary.TotalHPAs, result.Summary.AvgTarget, result.Summary.WithCPU))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Replica Age Distribution
// ---------------------------------------------------------------

type ReplicaAgeResult2011 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         ReplicaAgeSummary2011  `json:"summary"`
	PerBucket       []ReplicaAgeBucket2011 `json:"ageDistribution"`
	Recommendations []string               `json:"recommendations"`
}

type ReplicaAgeSummary2011 struct {
	TotalPods  int     `json:"totalPods"`
	AvgAgeDays float64 `json:"avgAgeDays"`
	NewPods    int     `json:"newPodsUnder1d"`
	OldPods    int     `json:"oldPodsOver30d"`
}

type ReplicaAgeBucket2011 struct {
	Bucket string `json:"bucket"`
	Count  int    `json:"count"`
}

func (s *Server) handleReplicaAgeDist(w http.ResponseWriter, r *http.Request) {
	result := ReplicaAgeResult2011{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	buckets := map[string]int{
		"<1d": 0, "1-7d": 0, "7-30d": 0, "30-90d": 0, ">90d": 0,
	}
	var totalAge float64
	var count int

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		if pod.CreationTimestamp.IsZero() {
			continue
		}

		ageDays := time.Since(pod.CreationTimestamp.Time).Hours() / 24
		totalAge += ageDays
		count++

		switch {
		case ageDays < 1:
			buckets["<1d"]++
		case ageDays < 7:
			buckets["1-7d"]++
		case ageDays < 30:
			buckets["7-30d"]++
		case ageDays < 90:
			buckets["30-90d"]++
		default:
			buckets[">90d"]++
		}
	}

	result.Summary.TotalPods = count
	if count > 0 {
		result.Summary.AvgAgeDays = totalAge / float64(count)
	}

	// Approximate new/old from buckets
	result.Summary.NewPods = buckets["<1d"]
	result.Summary.OldPods = buckets["30-90d"] + buckets[">90d"]

	order := []string{"<1d", "1-7d", "7-30d", "30-90d", ">90d"}
	for _, b := range order {
		result.PerBucket = append(result.PerBucket, ReplicaAgeBucket2011{
			Bucket: b, Count: buckets[b],
		})
	}

	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods, avg age %.0fd (%d new <1d, %d old >30d)", result.Summary.TotalPods, result.Summary.AvgAgeDays, result.Summary.NewPods, result.Summary.OldPods))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Node Pod Affinity Score
// ---------------------------------------------------------------

type NodeScoreResult2011 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         NodeScoreSummary2011 `json:"summary"`
	PerNode         []NodeScoreEntry2011 `json:"perNode"`
	Recommendations []string             `json:"recommendations"`
}

type NodeScoreSummary2011 struct {
	TotalNodes int     `json:"totalNodes"`
	AvgScore   float64 `json:"avgBalanceScore"`
	BestNode   string  `json:"bestBalancedNode"`
	WorstNode  string  `json:"worstBalancedNode"`
}

type NodeScoreEntry2011 struct {
	Name         string  `json:"name"`
	PodCount     int     `json:"podCount"`
	BalanceScore float64 `json:"balanceScore"`
}

func (s *Server) handleNodeAffScore(w http.ResponseWriter, r *http.Request) {
	result := NodeScoreResult2011{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	podsPerNode := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning && pod.Spec.NodeName != "" {
			podsPerNode[pod.Spec.NodeName]++
		}
	}

	totalNodes := len(nodeList.Items)
	result.Summary.TotalNodes = totalNodes

	avgPods := 0.0
	if totalNodes > 0 {
		totalPods := 0
		for _, c := range podsPerNode {
			totalPods += c
		}
		avgPods = float64(totalPods) / float64(totalNodes)
	}

	bestScore := 0.0
	worstScore := 100.0
	bestNode := ""
	worstNode := ""

	for _, node := range nodeList.Items {
		podCount := podsPerNode[node.Name]
		// Balance score: closer to average = higher score
		balanceScore := 100.0
		if avgPods > 0 {
			deviation := abs2011(float64(podCount)-avgPods) / avgPods * 100
			balanceScore = 100 - deviation
			if balanceScore < 0 {
				balanceScore = 0
			}
		}

		result.PerNode = append(result.PerNode, NodeScoreEntry2011{
			Name: node.Name, PodCount: podCount, BalanceScore: balanceScore,
		})

		if balanceScore > bestScore {
			bestScore = balanceScore
			bestNode = node.Name
		}
		if balanceScore < worstScore {
			worstScore = balanceScore
			worstNode = node.Name
		}
	}

	result.Summary.AvgScore = (bestScore + worstScore) / 2
	if totalNodes > 1 {
		result.Summary.AvgScore = bestScore // for single node, use best
	}
	result.Summary.BestNode = bestNode
	result.Summary.WorstNode = worstNode

	sort.Slice(result.PerNode, func(i, j int) bool {
		return result.PerNode[i].BalanceScore > result.PerNode[j].BalanceScore
	})

	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d nodes, avg balance %.0f, best: %s, worst: %s", totalNodes, result.Summary.AvgScore, bestNode, worstNode))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

func abs2011(f float64) float64 {
	if f < 0 {
		return -f
	}
	return f
}
