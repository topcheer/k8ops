package dashboard

import (
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.41 — Scalability & HA Dimension (Round 59)
// 1. Namespace Ephemeral Storage Usage
// 2. Node CPU Request vs Limit Spread
// 3. Cluster Pod Affinity Anti-Affinity Count
// ============================================================

type NSEphStorageResult2241 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
	} `json:"summary"`
	TopNS []struct {
		Namespace string  `json:"namespace"`
		EphReqGB  float64 `json:"ephemeralReqGB"`
	} `json:"topNamespaces"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleNSEphStorage2241(w http.ResponseWriter, r *http.Request) {
	result := NSEphStorageResult2241{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	nsEph := make(map[string]float64)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			nsEph[pod.Namespace] += c.Resources.Requests.StorageEphemeral().AsApproximateFloat64() / 1e9
		}
	}
	result.Summary.TotalNS = len(nsEph)
	for ns, eph := range nsEph {
		result.TopNS = append(result.TopNS, struct {
			Namespace string  `json:"namespace"`
			EphReqGB  float64 `json:"ephemeralReqGB"`
		}{ns, eph})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].EphReqGB > result.TopNS[j].EphReqGB })
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Node CPU Req vs Limit Spread
type CPUReqLimitSpreadResult2241 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes  int     `json:"totalNodes"`
		TotalReqCPU float64 `json:"totalRequestedCPU"`
		TotalLimCPU float64 `json:"totalLimitedCPU"`
		SpreadPct   int     `json:"spreadPct"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleCPUReqLimitSpread2241(w http.ResponseWriter, r *http.Request) {
	result := CPUReqLimitSpreadResult2241{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	result.Summary.TotalNodes = len(nodeList.Items)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalReqCPU += c.Resources.Requests.Cpu().AsApproximateFloat64()
			result.Summary.TotalLimCPU += c.Resources.Limits.Cpu().AsApproximateFloat64()
		}
	}
	if result.Summary.TotalReqCPU > 0 {
		result.Summary.SpreadPct = int((result.Summary.TotalLimCPU - result.Summary.TotalReqCPU) / result.Summary.TotalReqCPU * 100)
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. Pod Affinity Anti-Affinity Count
type AffAntiAffCountResult2241 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalDeploys int `json:"totalDeployments"`
		WithAffinity int `json:"withAffinity"`
		WithAntiAff  int `json:"withAntiAffinity"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleAffAntiAffCount2241(w http.ResponseWriter, r *http.Request) {
	result := AffAntiAffCountResult2241{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		aff := dep.Spec.Template.Spec.Affinity
		if aff == nil {
			continue
		}
		if aff.NodeAffinity != nil || aff.PodAffinity != nil {
			result.Summary.WithAffinity++
		}
		if aff.PodAntiAffinity != nil {
			result.Summary.WithAntiAff++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
