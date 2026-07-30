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
// v21.40 — Deployment Dimension (Round 43)
// 1. Pod SetHostnameAsFQDN Audit
// 2. Container WorkingDir Overlap Detector
// 3. Deployment OwnerReference Validator
// ============================================================

type HostnameFQDNResult2140 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         HostnameFQDNSummary2140 `json:"summary"`
	Recommendations []string                `json:"recommendations"`
}

type HostnameFQDNSummary2140 struct {
	TotalPods    int `json:"totalPods"`
	WithFQDN     int `json:"withFQDN"`
	WithHostname int `json:"withCustomHostname"`
}

func (s *Server) handleHostnameFQDN2140(w http.ResponseWriter, r *http.Request) {
	result := HostnameFQDNResult2140{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.Hostname != "" {
			result.Summary.WithHostname++
		}
		if pod.Spec.SetHostnameAsFQDN != nil && *pod.Spec.SetHostnameAsFQDN {
			result.Summary.WithFQDN++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. WorkingDir Overlap
type WorkDirOverlapResult2140 struct {
	ScannedAt       time.Time                 `json:"scannedAt"`
	HealthScore     int                       `json:"healthScore"`
	Grade           string                    `json:"grade"`
	Summary         WorkDirOverlapSummary2140 `json:"summary"`
	Overlaps        []WorkDirOverlapEntry2140 `json:"overlaps"`
	Recommendations []string                  `json:"recommendations"`
}

type WorkDirOverlapSummary2140 struct {
	TotalPods int `json:"totalPods"`
	Overlaps  int `json:"overlaps"`
}

type WorkDirOverlapEntry2140 struct {
	Pod     string `json:"pod"`
	WorkDir string `json:"workDir"`
}

func (s *Server) handleWorkDirOverlap2140(w http.ResponseWriter, r *http.Request) {
	result := WorkDirOverlapResult2140{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	dirCount := make(map[string]int)
	podDirs := make(map[string][]string)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for _, c := range pod.Spec.Containers {
			wd := c.WorkingDir
			if wd == "" {
				wd = "/"
			}
			dirCount[wd]++
			podDirs[wd] = append(podDirs[wd], pod.Name)
		}
	}
	for dir, cnt := range dirCount {
		if cnt > 5 {
			result.Summary.Overlaps++
			result.Overlaps = append(result.Overlaps, WorkDirOverlapEntry2140{Pod: fmt.Sprintf("%d pods", cnt), WorkDir: dir})
		}
	}
	sort.Slice(result.Overlaps, func(i, j int) bool { return result.Overlaps[i].WorkDir < result.Overlaps[j].WorkDir })
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. OwnerReference Validator
type OwnerRefResult2140 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         OwnerRefSummary2140 `json:"summary"`
	OrphanPods      []OwnerRefEntry2140 `json:"orphanPods"`
	Recommendations []string            `json:"recommendations"`
}

type OwnerRefSummary2140 struct {
	TotalPods int `json:"totalPods"`
	WithOwner int `json:"withOwner"`
	Orphan    int `json:"orphanPods"`
}

type OwnerRefEntry2140 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleOwnerRef2140(w http.ResponseWriter, r *http.Request) {
	result := OwnerRefResult2140{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.OwnerReferences) > 0 {
			result.Summary.WithOwner++
		} else {
			result.Summary.Orphan++
			result.OrphanPods = append(result.OrphanPods, OwnerRefEntry2140{Pod: pod.Name, Namespace: pod.Namespace})
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.OrphanPods, func(i, j int) bool { return result.OrphanPods[i].Namespace < result.OrphanPods[j].Namespace })
	writeJSON(w, result)
}
