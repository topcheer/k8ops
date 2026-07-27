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
// v20.12 — Scalability & HA Dimension (Round 21 Final)
// 1. ETCD DB Size Estimator — estimate etcd storage from object count
// 2. Scheduler Cache Pressure — scheduler cache pressure from pod/node ratio
// 3. APIServer Request Latency — API server response time estimator
// ============================================================

// ---------------------------------------------------------------
// 1. ETCD DB Size Estimator
// ---------------------------------------------------------------

type ETCDSizeResult2012 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         ETCDSizeSummary2012 `json:"summary"`
	PerType         []ETCDSizeEntry2012 `json:"perType"`
	Recommendations []string            `json:"recommendations"`
}

type ETCDSizeSummary2012 struct {
	TotalObjects int     `json:"totalObjects"`
	EstDBSizeMB  float64 `json:"estDBSizeMB"`
	SizeLevel    string  `json:"sizeLevel"`
}

type ETCDSizeEntry2012 struct {
	Type      string  `json:"type"`
	Count     int     `json:"count"`
	EstSizeMB float64 `json:"estSizeMB"`
}

// Estimated etcd bytes per object type
var etcdSizePerType2012 = map[string]float64{
	"Pods":        25, // KB per pod
	"Services":    10,
	"ConfigMaps":  15,
	"Secrets":     10,
	"Namespaces":  5,
	"PVCs":        15,
	"Deployments": 20,
	"Events":      8,
}

func (s *Server) handleETCDSizeEst(w http.ResponseWriter, r *http.Request) {
	result := ETCDSizeResult2012{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	cmList, _ := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})

	types := []struct {
		name  string
		count int
	}{
		{"Pods", len(podList.Items)},
		{"Services", len(svcList.Items)},
		{"ConfigMaps", len(cmList.Items)},
		{"Secrets", len(secretList.Items)},
		{"Namespaces", len(nsList.Items)},
		{"PVCs", len(pvcList.Items)},
		{"Deployments", len(depList.Items)},
		{"Events", len(eventList.Items)},
	}

	totalObjs := 0
	totalKB := 0.0
	for _, t := range types {
		totalObjs += t.count
		sizeKB := float64(t.count) * etcdSizePerType2012[t.name]
		totalKB += sizeKB
		result.PerType = append(result.PerType, ETCDSizeEntry2012{
			Type: t.name, Count: t.count, EstSizeMB: sizeKB / 1024,
		})
	}

	result.Summary.TotalObjects = totalObjs
	result.Summary.EstDBSizeMB = totalKB / 1024

	if result.Summary.EstDBSizeMB > 2048 {
		result.Summary.SizeLevel = "critical"
		score -= 10
	} else if result.Summary.EstDBSizeMB > 1024 {
		result.Summary.SizeLevel = "high"
		score -= 5
	} else if result.Summary.EstDBSizeMB > 500 {
		result.Summary.SizeLevel = "medium"
	} else {
		result.Summary.SizeLevel = "low"
	}

	sort.Slice(result.PerType, func(i, j int) bool {
		return result.PerType[i].EstSizeMB > result.PerType[j].EstSizeMB
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d objects, est etcd %.0f MB, level: %s", totalObjs, result.Summary.EstDBSizeMB, result.Summary.SizeLevel))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Scheduler Cache Pressure
// ---------------------------------------------------------------

type SchedCacheResult2012 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         SchedCacheSummary2012 `json:"summary"`
	PerNode         []SchedCacheEntry2012 `json:"perNode"`
	Recommendations []string              `json:"recommendations"`
}

type SchedCacheSummary2012 struct {
	TotalNodes    int     `json:"totalNodes"`
	TotalPods     int     `json:"totalPods"`
	PodsPerNode   float64 `json:"podsPerNode"`
	CachePressure string  `json:"cachePressureLevel"`
}

type SchedCacheEntry2012 struct {
	Node     string  `json:"node"`
	PodCount int     `json:"podCount"`
	Pressure float64 `json:"pressureScore"`
}

func (s *Server) handleSchedCachePressure(w http.ResponseWriter, r *http.Request) {
	result := SchedCacheResult2012{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	podsPerNode := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning && pod.Spec.NodeName != "" {
			podsPerNode[pod.Spec.NodeName]++
			result.Summary.TotalPods++
		}
	}

	result.Summary.TotalNodes = len(nodeList.Items)
	if result.Summary.TotalNodes > 0 {
		result.Summary.PodsPerNode = float64(result.Summary.TotalPods) / float64(result.Summary.TotalNodes)
	}

	for _, node := range nodeList.Items {
		podCount := podsPerNode[node.Name]
		// Pressure: 0-100, higher = more pressure
		pressure := float64(podCount) / 110 * 100 // 110 is maxPods default
		if pressure > 100 {
			pressure = 100
		}

		result.PerNode = append(result.PerNode, SchedCacheEntry2012{
			Node: node.Name, PodCount: podCount, Pressure: pressure,
		})
	}

	if result.Summary.PodsPerNode > 100 {
		result.Summary.CachePressure = "critical"
		score -= 10
	} else if result.Summary.PodsPerNode > 80 {
		result.Summary.CachePressure = "high"
		score -= 5
	} else if result.Summary.PodsPerNode > 50 {
		result.Summary.CachePressure = "medium"
	} else {
		result.Summary.CachePressure = "low"
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d nodes, %d pods, %.0f/node, pressure: %s", result.Summary.TotalNodes, result.Summary.TotalPods, result.Summary.PodsPerNode, result.Summary.CachePressure))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. APIServer Request Latency Estimator
// ---------------------------------------------------------------

type APILatResult2012 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         APILatSummary2012 `json:"summary"`
	PerNS           []APILatEntry2012 `json:"perNamespace"`
	Recommendations []string          `json:"recommendations"`
}

type APILatSummary2012 struct {
	TotalPods    int     `json:"totalPods"`
	EstLatencyMs float64 `json:"estAPILatencyMs"`
	LatencyLevel string  `json:"latencyLevel"`
}

type APILatEntry2012 struct {
	Namespace    string  `json:"namespace"`
	PodCount     int     `json:"podCount"`
	EstLatencyMs float64 `json:"estLatencyMs"`
}

func (s *Server) handleAPILatency(w http.ResponseWriter, r *http.Request) {
	result := APILatResult2012{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	nsStats := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		nsStats[pod.Namespace]++
	}

	// Estimate latency: ~1ms base + 0.05ms per pod (watch overhead)
	estLatency := 1.0 + float64(result.Summary.TotalPods)*0.05
	result.Summary.EstLatencyMs = estLatency

	for ns, count := range nsStats {
		result.PerNS = append(result.PerNS, APILatEntry2012{
			Namespace: ns, PodCount: count,
			EstLatencyMs: 1.0 + float64(count)*0.05,
		})
	}
	sort.Slice(result.PerNS, func(i, j int) bool {
		return result.PerNS[i].EstLatencyMs > result.PerNS[j].EstLatencyMs
	})
	if len(result.PerNS) > 10 {
		result.PerNS = result.PerNS[:10]
	}

	if estLatency > 50 {
		result.Summary.LatencyLevel = "critical"
		score -= 10
	} else if estLatency > 20 {
		result.Summary.LatencyLevel = "high"
		score -= 5
	} else if estLatency > 10 {
		result.Summary.LatencyLevel = "medium"
	} else {
		result.Summary.LatencyLevel = "low"
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods, est API latency %.1fms, level: %s", result.Summary.TotalPods, estLatency, result.Summary.LatencyLevel))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
