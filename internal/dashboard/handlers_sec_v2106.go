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
// v21.06 — Security Dimension (Round 37)
// 1. Pod SA AutoMount Token Audit
// 2. Volume Projection Audit — projected volume usage
// 3. ClusterRoleBinding Subject Type Audit
// ============================================================

type SATokenMountResult2106 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         SATokenMountSummary2106 `json:"summary"`
	AtRisk          []SATokenMountEntry2106 `json:"atRiskPods"`
	Recommendations []string                `json:"recommendations"`
}

type SATokenMountSummary2106 struct {
	TotalPods int `json:"totalPods"`
	AutoMount int `json:"autoMountTrue"`
}

type SATokenMountEntry2106 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleSATokenMount2106(w http.ResponseWriter, r *http.Request) {
	result := SATokenMountResult2106{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		autoMount := true
		if pod.Spec.AutomountServiceAccountToken != nil {
			autoMount = *pod.Spec.AutomountServiceAccountToken
		}
		if autoMount {
			result.Summary.AutoMount++
			if pod.Spec.ServiceAccountName == "" || pod.Spec.ServiceAccountName == "default" {
				result.AtRisk = append(result.AtRisk, SATokenMountEntry2106{Pod: pod.Name, Namespace: pod.Namespace})
				score -= 1
			}
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.AtRisk, func(i, j int) bool { return result.AtRisk[i].Namespace < result.AtRisk[j].Namespace })
	writeJSON(w, result)
}

// 2. Volume Projection Audit
type VolProjResult2106 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         VolProjSummary2106 `json:"summary"`
	Recommendations []string           `json:"recommendations"`
}

type VolProjSummary2106 struct {
	TotalPods   int `json:"totalPods"`
	WithProjVol int `json:"withProjectedVolume"`
}

func (s *Server) handleVolProj2106(w http.ResponseWriter, r *http.Request) {
	result := VolProjResult2106{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for _, vol := range pod.Spec.Volumes {
			if vol.Projected != nil {
				result.Summary.WithProjVol++
				break
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. CRB Subject Type Audit
type CRBSubjResult2106 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         CRBSubjSummary2106 `json:"summary"`
	Recommendations []string           `json:"recommendations"`
}

type CRBSubjSummary2106 struct {
	TotalCRBs int            `json:"totalClusterRoleBindings"`
	ByKind    map[string]int `json:"subjectsByKind"`
}

func (s *Server) handleCRBSubj2106(w http.ResponseWriter, r *http.Request) {
	result := CRBSubjResult2106{ScannedAt: time.Now()}
	score := 100
	crbList, _ := s.clientset.RbacV1().ClusterRoleBindings().List(r.Context(), metav1.ListOptions{})

	byKind := make(map[string]int)
	for _, crb := range crbList.Items {
		result.Summary.TotalCRBs++
		for _, subj := range crb.Subjects {
			byKind[subj.Kind]++
		}
	}
	result.Summary.ByKind = byKind
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if byKind["User"] > 10 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d User subjects in CRBs — prefer ServiceAccount", byKind["User"]))
	}
	writeJSON(w, result)
}
