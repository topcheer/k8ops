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
// v20.79 — Product Dimension (Round 33)
// 1. Pod Topology Pin Analysis — node selector & nodeName usage
// 2. Ingress Rule Complexity — rules per ingress ratio
// 3. Storage Access Mode Coverage — PVC access mode distribution
// ============================================================

type TopoPinResult2079 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         TopoPinSummary2079 `json:"summary"`
	PinnedPods      []TopoPinEntry2079 `json:"pinnedPods"`
	Recommendations []string           `json:"recommendations"`
}

type TopoPinSummary2079 struct {
	TotalPods    int `json:"totalPods"`
	WithNodeSel  int `json:"withNodeSelector"`
	WithNodeName int `json:"withNodeName"`
}

type TopoPinEntry2079 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	PinType   string `json:"pinType"`
}

func (s *Server) handleTopoPin2079(w http.ResponseWriter, r *http.Request) {
	result := TopoPinResult2079{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		if len(pod.Spec.NodeSelector) > 0 {
			result.Summary.WithNodeSel++
			result.PinnedPods = append(result.PinnedPods, TopoPinEntry2079{
				Pod: pod.Name, Namespace: pod.Namespace, PinType: "nodeSelector",
			})
		}
		if pod.Spec.NodeName != "" && len(pod.OwnerReferences) == 0 {
			result.Summary.WithNodeName++
			result.PinnedPods = append(result.PinnedPods, TopoPinEntry2079{
				Pod: pod.Name, Namespace: pod.Namespace, PinType: "nodeName",
			})
			score -= 3
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.PinnedPods, func(i, j int) bool { return result.PinnedPods[i].Namespace < result.PinnedPods[j].Namespace })

	if result.Summary.WithNodeName > 5 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pods pinned via nodeName — use nodeSelector instead", result.Summary.WithNodeName))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Ingress Rule Complexity
// ---------------------------------------------------------------

type IngRuleResult2079 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         IngRuleSummary2079 `json:"summary"`
	ComplexIngress  []IngRuleEntry2079 `json:"complexIngresses"`
	Recommendations []string           `json:"recommendations"`
}

type IngRuleSummary2079 struct {
	TotalIngresses int `json:"totalIngresses"`
	TotalRules     int `json:"totalRules"`
	AvgRules       int `json:"avgRules"`
}

type IngRuleEntry2079 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	RuleCount int    `json:"ruleCount"`
}

func (s *Server) handleIngRuleComplex(w http.ResponseWriter, r *http.Request) {
	result := IngRuleResult2079{ScannedAt: time.Now()}
	score := 100
	ingList, _ := s.clientset.NetworkingV1().Ingresses("").List(r.Context(), metav1.ListOptions{})

	for _, ing := range ingList.Items {
		result.Summary.TotalIngresses++
		ruleCount := len(ing.Spec.Rules)
		result.Summary.TotalRules += ruleCount

		if ruleCount > 10 {
			result.ComplexIngress = append(result.ComplexIngress, IngRuleEntry2079{
				Name: ing.Name, Namespace: ing.Namespace, RuleCount: ruleCount,
			})
			score -= 2
		}
	}
	if result.Summary.TotalIngresses > 0 {
		result.Summary.AvgRules = result.Summary.TotalRules / result.Summary.TotalIngresses
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if len(result.ComplexIngress) > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d ingresses with >10 rules — consider splitting", len(result.ComplexIngress)))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Storage Access Mode Coverage
// ---------------------------------------------------------------

type AccessModeResult2079 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         AccessModeSummary2079 `json:"summary"`
	Recommendations []string              `json:"recommendations"`
}

type AccessModeSummary2079 struct {
	TotalPVCs int            `json:"totalPVCs"`
	Modes     map[string]int `json:"accessModes"`
}

func (s *Server) handleAccessMode2079(w http.ResponseWriter, r *http.Request) {
	result := AccessModeResult2079{ScannedAt: time.Now()}
	score := 100
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})

	modes := make(map[string]int)
	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		for _, am := range pvc.Spec.AccessModes {
			modes[string(am)]++
		}
	}
	result.Summary.Modes = modes
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
