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
// v20.40 — Security Dimension (Round 26)
// 1. Service Account Token Mount Risk — automountServiceAccountToken audit
// 2. ClusterRole Binding Explosion — excessive subject bindings
// 3. Container Port Exposure — unnecessary port exposure audit
// ============================================================

// ---------------------------------------------------------------
// 1. Service Account Token Mount Risk
// ---------------------------------------------------------------

type SATokenMountResult2040 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         SATokenMountSummary2040 `json:"summary"`
	AtRiskPods      []SATokenMountEntry2040 `json:"atRiskPods"`
	Recommendations []string                `json:"recommendations"`
}

type SATokenMountSummary2040 struct {
	TotalPods      int `json:"totalPods"`
	AutoMountTrue  int `json:"autoMountTokenTrue"`
	AutoMountFalse int `json:"autoMountTokenFalse"`
	WithDefaultSA  int `json:"withDefaultSA"`
}

type SATokenMountEntry2040 struct {
	Pod            string `json:"pod"`
	Namespace      string `json:"namespace"`
	ServiceAccount string `json:"serviceAccount"`
}

func (s *Server) handleSATokenMountRisk(w http.ResponseWriter, r *http.Request) {
	result := SATokenMountResult2040{ScannedAt: time.Now()}
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

		sa := pod.Spec.ServiceAccountName
		if sa == "" || sa == "default" {
			result.Summary.WithDefaultSA++
			if autoMount {
				score -= 2
				result.AtRiskPods = append(result.AtRiskPods, SATokenMountEntry2040{
					Pod: pod.Name, Namespace: pod.Namespace,
					ServiceAccount: sa,
				})
			}
		}

		if autoMount {
			result.Summary.AutoMountTrue++
		} else {
			result.Summary.AutoMountFalse++
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.AtRiskPods, func(i, j int) bool {
		return result.AtRiskPods[i].Namespace < result.AtRiskPods[j].Namespace
	})

	if result.Summary.WithDefaultSA > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pods use default SA with token mount — set automountServiceAccountToken: false", result.Summary.WithDefaultSA))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. ClusterRole Binding Explosion
// ---------------------------------------------------------------

type CRBindingResult2040 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         CRBindingSummary2040 `json:"summary"`
	BloatedRoles    []CRBindingEntry2040 `json:"bloatedRoles"`
	Recommendations []string             `json:"recommendations"`
}

type CRBindingSummary2040 struct {
	TotalCRBs       int `json:"totalClusterRoleBindings"`
	TotalSubjects   int `json:"totalSubjects"`
	BloatedBindings int `json:"bloatedBindings"`
}

type CRBindingEntry2040 struct {
	Name     string `json:"name"`
	Subjects int    `json:"subjects"`
	RoleRef  string `json:"roleRef"`
}

func (s *Server) handleCRBindingExplosion(w http.ResponseWriter, r *http.Request) {
	result := CRBindingResult2040{ScannedAt: time.Now()}
	score := 100

	crbList, _ := s.clientset.RbacV1().ClusterRoleBindings().List(r.Context(), metav1.ListOptions{})

	for _, crb := range crbList.Items {
		result.Summary.TotalCRBs++
		subjectCount := len(crb.Subjects)
		result.Summary.TotalSubjects += subjectCount

		if subjectCount > 10 {
			result.Summary.BloatedBindings++
			result.BloatedRoles = append(result.BloatedRoles, CRBindingEntry2040{
				Name:     crb.Name,
				Subjects: subjectCount,
				RoleRef:  crb.RoleRef.Name,
			})
			score -= 3
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.BloatedRoles, func(i, j int) bool {
		return result.BloatedRoles[i].Subjects > result.BloatedRoles[j].Subjects
	})

	if result.Summary.BloatedBindings > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d ClusterRoleBindings have >10 subjects — consider namespace-scoped RoleBindings", result.Summary.BloatedBindings))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Container Port Exposure
// ---------------------------------------------------------------

type PortExposureResult2040 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         PortExposureSummary2040 `json:"summary"`
	ExposedPorts    []PortExposureEntry2040 `json:"exposedPorts"`
	Recommendations []string                `json:"recommendations"`
}

type PortExposureSummary2040 struct {
	TotalContainers int `json:"totalContainers"`
	WithPorts       int `json:"withPorts"`
	PrivilegedPorts int `json:"privilegedPorts"`
	HostPorts       int `json:"hostPorts"`
}

type PortExposureEntry2040 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Port      int32  `json:"port"`
	HostPort  int32  `json:"hostPort,omitempty"`
}

func (s *Server) handlePortExposure2040(w http.ResponseWriter, r *http.Request) {
	result := PortExposureResult2040{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			for _, port := range c.Ports {
				result.Summary.WithPorts++

				// Privileged port (<1024)
				if port.ContainerPort < 1024 {
					result.Summary.PrivilegedPorts++
					score -= 1
				}

				// HostPort binding
				if port.HostPort != 0 {
					result.Summary.HostPorts++
					score -= 3
					result.ExposedPorts = append(result.ExposedPorts, PortExposureEntry2040{
						Pod: pod.Name, Namespace: pod.Namespace,
						Port: port.ContainerPort, HostPort: port.HostPort,
					})
				}
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.ExposedPorts, func(i, j int) bool {
		return result.ExposedPorts[i].Namespace < result.ExposedPorts[j].Namespace
	})

	if result.Summary.HostPorts > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d containers use HostPort — use Services instead for port exposure", result.Summary.HostPorts))
	}

	writeJSON(w, result)
}
