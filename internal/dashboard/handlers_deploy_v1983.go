package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v19.83 — Deployment Dimension (Round 17)
// 1. Node Selector Audit — nodeSelector & nodeAffinity coverage
// 2. Pod OS Selector — OS/arch constraint compliance
// 3. Container Working Dir — working directory config compliance
// ============================================================

// ---------------------------------------------------------------
// 1. Node Selector Audit
// ---------------------------------------------------------------

type NodeSelResult1983 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         NodeSelSummary1983 `json:"summary"`
	Pods            []NodeSelEntry1983 `json:"pods"`
	Recommendations []string           `json:"recommendations"`
}

type NodeSelSummary1983 struct {
	TotalPods        int `json:"totalPods"`
	WithNodeSelector int `json:"withNodeSelector"`
	WithNodeAffinity int `json:"withNodeAffinity"`
	WithToleration   int `json:"withToleration"`
	WithoutSelector  int `json:"withoutSelector"`
}

type NodeSelEntry1983 struct {
	Pod             string            `json:"pod"`
	Namespace       string            `json:"namespace"`
	Selector        map[string]string `json:"nodeSelector"`
	HasAffinity     bool              `json:"hasNodeAffinity"`
	TolerationCount int               `json:"tolerationCount"`
}

func (s *Server) handleNodeSelectorAudit(w http.ResponseWriter, r *http.Request) {
	result := NodeSelResult1983{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		entry := NodeSelEntry1983{
			Pod: pod.Name, Namespace: pod.Namespace,
			Selector:        pod.Spec.NodeSelector,
			HasAffinity:     pod.Spec.Affinity != nil && pod.Spec.Affinity.NodeAffinity != nil,
			TolerationCount: len(pod.Spec.Tolerations),
		}

		hasSelector := len(pod.Spec.NodeSelector) > 0
		if hasSelector {
			result.Summary.WithNodeSelector++
		}
		if entry.HasAffinity {
			result.Summary.WithNodeAffinity++
		}
		if entry.TolerationCount > 0 {
			result.Summary.WithToleration++
		}
		if !hasSelector && !entry.HasAffinity {
			result.Summary.WithoutSelector++
		}

		result.Pods = append(result.Pods, entry)
	}

	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods: %d with nodeSelector, %d with affinity, %d with tolerations, %d unconstrained", result.Summary.TotalPods, result.Summary.WithNodeSelector, result.Summary.WithNodeAffinity, result.Summary.WithToleration, result.Summary.WithoutSelector))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Pod OS Selector
// ---------------------------------------------------------------

type PodOSResult1983 struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Summary         PodOSSummary1983 `json:"summary"`
	Pods            []PodOSEntry1983 `json:"pods"`
	Recommendations []string         `json:"recommendations"`
}

type PodOSSummary1983 struct {
	TotalPods      int `json:"totalPods"`
	WithOSSelector int `json:"withOSSelector"`
	LinuxPods      int `json:"linuxPods"`
	WindowsPods    int `json:"windowsPods"`
	NoOSSpecified  int `json:"noOSSpecified"`
}

type PodOSEntry1983 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	OS        string `json:"os"`
}

func (s *Server) handlePodOSSelector(w http.ResponseWriter, r *http.Request) {
	result := PodOSResult1983{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		osName := ""
		if pod.Spec.OS != nil {
			osName = string(pod.Spec.OS.Name)
		}

		entry := PodOSEntry1983{
			Pod: pod.Name, Namespace: pod.Namespace, OS: osName,
		}

		if osName != "" {
			result.Summary.WithOSSelector++
			if osName == "linux" {
				result.Summary.LinuxPods++
			} else if osName == "windows" {
				result.Summary.WindowsPods++
			}
		} else {
			result.Summary.NoOSSpecified++
		}

		result.Pods = append(result.Pods, entry)
	}

	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods: %d with OS selector (%d Linux, %d Windows), %d without OS spec", result.Summary.TotalPods, result.Summary.WithOSSelector, result.Summary.LinuxPods, result.Summary.WindowsPods, result.Summary.NoOSSpecified))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Container Working Dir
// ---------------------------------------------------------------

type WorkDirResult1983 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         WorkDirSummary1983 `json:"summary"`
	Containers      []WorkDirEntry1983 `json:"containers"`
	Recommendations []string           `json:"recommendations"`
}

type WorkDirSummary1983 struct {
	TotalContainers int `json:"totalContainers"`
	WithWorkDir     int `json:"withWorkingDir"`
	UsingRoot       int `json:"usingRootDir"`
	NonStandard     int `json:"nonStandardDir"`
}

type WorkDirEntry1983 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
	WorkDir   string `json:"workingDir"`
}

func (s *Server) handleContainerWorkingDir(w http.ResponseWriter, r *http.Request) {
	result := WorkDirResult1983{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			wd := c.WorkingDir
			entry := WorkDirEntry1983{
				Pod: pod.Name, Namespace: pod.Namespace,
				Container: c.Name, WorkDir: wd,
			}

			if wd != "" {
				result.Summary.WithWorkDir++
				if wd == "/" {
					result.Summary.UsingRoot++
				} else if !strings.HasPrefix(wd, "/app") && !strings.HasPrefix(wd, "/home") &&
					!strings.HasPrefix(wd, "/opt") && !strings.HasPrefix(wd, "/srv") {
					result.Summary.NonStandard++
				}
			}

			result.Containers = append(result.Containers, entry)
		}
	}

	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers: %d with explicit workingDir, %d using root, %d non-standard", result.Summary.TotalContainers, result.Summary.WithWorkDir, result.Summary.UsingRoot, result.Summary.NonStandard))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
