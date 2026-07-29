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
// v20.57 — Operations Dimension (Round 29)
// 1. Pod OOM Risk Predictor — memory limit vs usage prediction
// 2. API Server QPS Estimate — API server request rate
// 3. Node Pressure Score — combined node pressure metrics
// ============================================================

type OOMRiskResult2057 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         OOMRiskSummary2057 `json:"summary"`
	AtRiskPods      []OOMRiskEntry2057 `json:"atRiskPods"`
	Recommendations []string           `json:"recommendations"`
}

type OOMRiskSummary2057 struct {
	TotalPods    int `json:"totalPods"`
	WithMemLimit int `json:"withMemLimit"`
	NoMemLimit   int `json:"noMemLimit"`
	AtRisk       int `json:"atRisk"`
}

type OOMRiskEntry2057 struct {
	Pod       string  `json:"pod"`
	Namespace string  `json:"namespace"`
	MemLimit  float64 `json:"memLimitMB"`
}

func (s *Server) handleOOMRiskPredictor(w http.ResponseWriter, r *http.Request) {
	result := OOMRiskResult2057{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		for _, c := range pod.Spec.Containers {
			if c.Resources.Limits.Memory().IsZero() {
				result.Summary.NoMemLimit++
				score -= 1
			} else {
				result.Summary.WithMemLimit++
				memLimitMB := c.Resources.Limits.Memory().AsApproximateFloat64() / 1e6
				if memLimitMB < 128 {
					result.Summary.AtRisk++
					result.AtRiskPods = append(result.AtRiskPods, OOMRiskEntry2057{
						Pod: pod.Name, Namespace: pod.Namespace, MemLimit: memLimitMB,
					})
					score -= 2
				}
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.AtRiskPods, func(i, j int) bool {
		return result.AtRiskPods[i].MemLimit < result.AtRiskPods[j].MemLimit
	})

	if result.Summary.AtRisk > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d containers with <128MB memory limit — high OOM risk", result.Summary.AtRisk))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. API Server QPS Estimate
// ---------------------------------------------------------------

type APIServerQPSResult2057 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         APIServerQPSSummary2057 `json:"summary"`
	Recommendations []string                `json:"recommendations"`
}

type APIServerQPSSummary2057 struct {
	WatchCount   int `json:"estimatedWatchCount"`
	ListCount    int `json:"estimatedListCount"`
	TotalObjects int `json:"totalObjects"`
}

func (s *Server) handleAPIServerQPS(w http.ResponseWriter, r *http.Request) {
	result := APIServerQPSResult2057{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})

	// Estimate watch connections (each controller = 1 watch, ~50 per cluster)
	controllers := len(nsList.Items) * 2         // rough estimate
	result.Summary.WatchCount = controllers + 20 // base system controllers
	result.Summary.ListCount = len(nsList.Items)
	result.Summary.TotalObjects = len(podList.Items) + len(svcList.Items)

	if result.Summary.TotalObjects > 10000 {
		score -= 20
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d total objects — high API server load expected", result.Summary.TotalObjects))
	} else if result.Summary.TotalObjects > 5000 {
		score -= 10
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Node Pressure Score
// ---------------------------------------------------------------

type NodePressureResult2057 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         NodePressureSummary2057 `json:"summary"`
	PressuredNodes  []NodePressureEntry2057 `json:"pressuredNodes"`
	Recommendations []string                `json:"recommendations"`
}

type NodePressureSummary2057 struct {
	TotalNodes     int `json:"totalNodes"`
	HealthyNodes   int `json:"healthyNodes"`
	PressuredNodes int `json:"pressuredNodes"`
}

type NodePressureEntry2057 struct {
	Node      string   `json:"node"`
	Pressures []string `json:"pressures"`
}

func (s *Server) handleNodePressureScore(w http.ResponseWriter, r *http.Request) {
	result := NodePressureResult2057{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		pressures := []string{}
		healthy := true

		for _, cond := range node.Status.Conditions {
			if cond.Status != corev1.ConditionTrue {
				continue
			}
			switch cond.Type {
			case corev1.NodeMemoryPressure:
				pressures = append(pressures, "MemoryPressure")
				healthy = false
			case corev1.NodeDiskPressure:
				pressures = append(pressures, "DiskPressure")
				healthy = false
			case corev1.NodePIDPressure:
				pressures = append(pressures, "PIDPressure")
				healthy = false
			case corev1.NodeNetworkUnavailable:
				pressures = append(pressures, "NetworkUnavailable")
				healthy = false
			}
		}

		if healthy {
			result.Summary.HealthyNodes++
		} else {
			result.Summary.PressuredNodes++
			result.PressuredNodes = append(result.PressuredNodes, NodePressureEntry2057{
				Node: node.Name, Pressures: pressures,
			})
			score -= 15
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.PressuredNodes > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d nodes under pressure — check resources", result.Summary.PressuredNodes))
	}

	writeJSON(w, result)
}
