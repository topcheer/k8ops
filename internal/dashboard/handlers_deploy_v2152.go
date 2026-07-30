package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v21.52 — Deployment Dimension (Round 45)
// 1. Pod Node Selector Required Validator
// 2. Container Image Volume Mount Audit
// 3. Deployment Condition Status Tracker
// ============================================================

type NodeSelReqResult2152 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         NodeSelReqSummary2152 `json:"summary"`
	WithRequired    []NodeSelReqEntry2152 `json:"withRequiredSelector"`
	Recommendations []string              `json:"recommendations"`
}

type NodeSelReqSummary2152 struct {
	TotalDeploys int `json:"totalDeployments"`
	WithRequired int `json:"withRequiredNodeSelector"`
}

type NodeSelReqEntry2152 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleNodeSelReq2152(w http.ResponseWriter, r *http.Request) {
	result := NodeSelReqResult2152{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		if len(dep.Spec.Template.Spec.NodeSelector) > 0 {
			result.Summary.WithRequired++
			result.WithRequired = append(result.WithRequired, NodeSelReqEntry2152{Name: dep.Name, Namespace: dep.Namespace})
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.WithRequired, func(i, j int) bool { return result.WithRequired[i].Namespace < result.WithRequired[j].Namespace })
	writeJSON(w, result)
}

// 2. Image Volume Mount
type ImgVolMountResult2152 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         ImgVolMountSummary2152 `json:"summary"`
	WithImgVol      []ImgVolMountEntry2152 `json:"withImageVolume"`
	Recommendations []string               `json:"recommendations"`
}

type ImgVolMountSummary2152 struct {
	TotalPods  int `json:"totalPods"`
	WithImgVol int `json:"withImageVolume"`
}

type ImgVolMountEntry2152 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleImgVolMount2152(w http.ResponseWriter, r *http.Request) {
	result := ImgVolMountResult2152{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for _, vol := range pod.Spec.Volumes {
			if vol.Image != nil {
				result.Summary.WithImgVol++
				result.WithImgVol = append(result.WithImgVol, ImgVolMountEntry2152{Pod: pod.Name, Namespace: pod.Namespace})
				break
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.WithImgVol > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pods using image volumes — alpha feature", result.Summary.WithImgVol))
	}
	writeJSON(w, result)
}

// 3. Deployment Condition Status
type DepCondResult2152 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         DepCondSummary2152 `json:"summary"`
	Recommendations []string           `json:"recommendations"`
}

type DepCondSummary2152 struct {
	TotalDeploys int `json:"totalDeployments"`
	Available    int `json:"available"`
	Progressing  int `json:"progressing"`
}

func (s *Server) handleDepCond2152(w http.ResponseWriter, r *http.Request) {
	result := DepCondResult2152{ScannedAt: time.Now()}
	score := 100
	deployList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range deployList.Items {
		result.Summary.TotalDeploys++
		for _, cond := range dep.Status.Conditions {
			if cond.Type == appsv1.DeploymentAvailable && cond.Status == corev1.ConditionTrue {
				result.Summary.Available++
			}
			if cond.Type == appsv1.DeploymentProgressing && cond.Status == corev1.ConditionTrue {
				result.Summary.Progressing++
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
