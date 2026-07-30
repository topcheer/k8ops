package dashboard

import (
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.30 — Security Dimension (Round 41)
// 1. Pod Host PID Namespace Audit
// 2. ServiceAccount Automount Default
// 3. NetworkPolicy Namespace Coverage Ratio
// ============================================================

type HostPIDResult2130 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         HostPIDSummary2130 `json:"summary"`
	AtRisk          []HostPIDEntry2130 `json:"atRiskPods"`
	Recommendations []string           `json:"recommendations"`
}

type HostPIDSummary2130 struct {
	TotalPods int `json:"totalPods"`
	HostPID   int `json:"hostPIDPods"`
	HostIPC   int `json:"hostIPCPods"`
}

type HostPIDEntry2130 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Flag      string `json:"flag"`
}

func (s *Server) handleHostPID2130(w http.ResponseWriter, r *http.Request) {
	result := HostPIDResult2130{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.HostPID {
			result.Summary.HostPID++
			result.AtRisk = append(result.AtRisk, HostPIDEntry2130{Pod: pod.Name, Namespace: pod.Namespace, Flag: "hostPID"})
			score -= 3
		}
		if pod.Spec.HostIPC {
			result.Summary.HostIPC++
			result.AtRisk = append(result.AtRisk, HostPIDEntry2130{Pod: pod.Name, Namespace: pod.Namespace, Flag: "hostIPC"})
			score -= 3
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.HostPID+result.Summary.HostIPC > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pods with hostPID/IPC — security risk", result.Summary.HostPID+result.Summary.HostIPC))
	}
	writeJSON(w, result)
}

// 2. SA Automount Default
type SAAutoResult2130 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         SAAutoSummary2130 `json:"summary"`
	Recommendations []string          `json:"recommendations"`
}

type SAAutoSummary2130 struct {
	TotalSAs     int `json:"totalServiceAccounts"`
	AutoDisabled int `json:"autoMountDisabled"`
}

func (s *Server) handleSAAuto2130(w http.ResponseWriter, r *http.Request) {
	result := SAAutoResult2130{ScannedAt: time.Now()}
	score := 100
	saList, _ := s.clientset.CoreV1().ServiceAccounts("").List(r.Context(), metav1.ListOptions{})

	for _, sa := range saList.Items {
		result.Summary.TotalSAs++
		if sa.AutomountServiceAccountToken != nil && !*sa.AutomountServiceAccountToken {
			result.Summary.AutoDisabled++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. NP Coverage Ratio
type NPCoverRatioResult2130 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         NPCoverRatioSummary2130 `json:"summary"`
	Recommendations []string                `json:"recommendations"`
}

type NPCoverRatioSummary2130 struct {
	TotalNS     int `json:"totalNamespaces"`
	WithNetPol  int `json:"withNetworkPolicy"`
	CoveragePct int `json:"coveragePct"`
}

func (s *Server) handleNPCoverRatio2130(w http.ResponseWriter, r *http.Request) {
	result := NPCoverRatioResult2130{ScannedAt: time.Now()}
	score := 100
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	npList, _ := s.clientset.NetworkingV1().NetworkPolicies("").List(r.Context(), metav1.ListOptions{})

	nsWithNP := make(map[string]bool)
	for _, np := range npList.Items {
		nsWithNP[np.Namespace] = true
	}

	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true, "k8ops-system": true}
	for _, ns := range nsList.Items {
		if systemNS[ns.Name] {
			continue
		}
		result.Summary.TotalNS++
		if nsWithNP[ns.Name] {
			result.Summary.WithNetPol++
		}
	}
	if result.Summary.TotalNS > 0 {
		result.Summary.CoveragePct = result.Summary.WithNetPol * 100 / result.Summary.TotalNS
	}
	if result.Summary.CoveragePct < 50 {
		score -= 10
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
