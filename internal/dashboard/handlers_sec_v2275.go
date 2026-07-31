package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v22.75 Security: readOnlyRootFilesystem audit, allowPrivilegeEscalation audit, runAsUser distribution
type ReadOnlyRootFSResult2275 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		ReadOnlyRoot    int `json:"readOnlyRootFS"`
	} `json:"summary"`
}

func (s *Server) handleReadOnlyRootFS2275(w http.ResponseWriter, r *http.Request) {
	result := ReadOnlyRootFSResult2275{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.SecurityContext != nil && c.SecurityContext.ReadOnlyRootFilesystem != nil && *c.SecurityContext.ReadOnlyRootFilesystem {
				result.Summary.ReadOnlyRoot++
			}
		}
	}
	score := 100
	if result.Summary.TotalContainers > 0 {
		score = result.Summary.ReadOnlyRoot * 100 / result.Summary.TotalContainers
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type PrivEscResult2275 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		AllowEscalation int `json:"allowPrivilegeEscalation"`
	} `json:"summary"`
}

func (s *Server) handlePrivEsc2275(w http.ResponseWriter, r *http.Request) {
	result := PrivEscResult2275{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.SecurityContext == nil || c.SecurityContext.AllowPrivilegeEscalation == nil || *c.SecurityContext.AllowPrivilegeEscalation {
				result.Summary.AllowEscalation++
			}
		}
	}
	score := 100
	if result.Summary.TotalContainers > 0 {
		score = 100 - (result.Summary.AllowEscalation*100)/result.Summary.TotalContainers
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

type RunAsUserResult2275 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		RootUID         int `json:"rootUID"`
		NonRootUID      int `json:"nonRootUID"`
		Unspecified     int `json:"unspecified"`
	} `json:"summary"`
}

func (s *Server) handleRunAsUser2275(w http.ResponseWriter, r *http.Request) {
	result := RunAsUserResult2275{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.SecurityContext != nil && c.SecurityContext.RunAsUser != nil {
				if *c.SecurityContext.RunAsUser == 0 {
					result.Summary.RootUID++
				} else {
					result.Summary.NonRootUID++
				}
			} else {
				result.Summary.Unspecified++
			}
		}
	}
	score := 100
	if result.Summary.TotalContainers > 0 {
		score = 100 - (result.Summary.RootUID*100)/result.Summary.TotalContainers
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
