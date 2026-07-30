package dashboard

import (
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.13 — Documentation Dimension (Round 38)
// 1. Node Label Diversity Catalog
// 2. Service Port Name Inventory
// 3. Pod Annotation Key Distribution
// ============================================================

type LabelDivResult2113 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         LabelDivSummary2113 `json:"summary"`
	Recommendations []string            `json:"recommendations"`
}

type LabelDivSummary2113 struct {
	TotalNodes int            `json:"totalNodes"`
	LabelKeys  map[string]int `json:"labelKeyCounts"`
}

func (s *Server) handleLabelDiv2113(w http.ResponseWriter, r *http.Request) {
	result := LabelDivResult2113{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	keyCount := make(map[string]int)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		for k := range node.Labels {
			keyCount[k]++
		}
	}
	result.Summary.LabelKeys = keyCount
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Service Port Name Inventory
type PortNameResult2113 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         PortNameSummary2113 `json:"summary"`
	Recommendations []string            `json:"recommendations"`
}

type PortNameSummary2113 struct {
	TotalServices int `json:"totalServices"`
	NamedPorts    int `json:"namedPorts"`
	UnnamedPorts  int `json:"unnamedPorts"`
}

func (s *Server) handlePortName2113(w http.ResponseWriter, r *http.Request) {
	result := PortNameResult2113{ScannedAt: time.Now()}
	score := 100
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})

	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		for _, p := range svc.Spec.Ports {
			if p.Name != "" {
				result.Summary.NamedPorts++
			} else {
				result.Summary.UnnamedPorts++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Pod Annotation Key Distribution
type AnnotKeyResult2113 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         AnnotKeySummary2113 `json:"summary"`
	TopKeys         []AnnotKeyEntry2113 `json:"topAnnotationKeys"`
	Recommendations []string            `json:"recommendations"`
}

type AnnotKeySummary2113 struct {
	TotalPods int `json:"totalPods"`
	TotalKeys int `json:"uniqueAnnotationKeys"`
}

type AnnotKeyEntry2113 struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

func (s *Server) handleAnnotKey2113(w http.ResponseWriter, r *http.Request) {
	result := AnnotKeyResult2113{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	keyCount := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for k := range pod.Annotations {
			keyCount[k]++
		}
	}
	result.Summary.TotalKeys = len(keyCount)

	type kv struct {
		key   string
		count int
	}
	var sorted []kv
	for k, c := range keyCount {
		sorted = append(sorted, kv{k, c})
	}
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].count > sorted[j].count })
	for i, s2 := range sorted {
		if i >= 15 {
			break
		}
		result.TopKeys = append(result.TopKeys, AnnotKeyEntry2113{Key: s2.key, Count: s2.count})
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
