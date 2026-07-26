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
// v19.91 — Security Dimension (Round 18)
// 1. Pod Privilege Escalation — allowPrivilegeEscalation config audit
// 2. Seccomp Profile Audit — seccomp profile compliance
// 3. Container CapDrop Audit — dropped capabilities tracking
// ============================================================

// ---------------------------------------------------------------
// 1. Pod Privilege Escalation
// ---------------------------------------------------------------

type PrivEscResult1991 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         PrivEscSummary1991 `json:"summary"`
	Issues          []PrivEscEntry1991 `json:"issues"`
	Recommendations []string           `json:"recommendations"`
}

type PrivEscSummary1991 struct {
	TotalContainers int `json:"totalContainers"`
	ExplicitFalse   int `json:"explicitlyFalse"`
	ExplicitTrue    int `json:"explicitlyTrue"`
	NotSet          int `json:"notSet"`
}

type PrivEscEntry1991 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
	Value     bool   `json:"allowPrivilegeEscalation"`
	Issue     string `json:"issue"`
}

func (s *Server) handlePrivEscAudit(w http.ResponseWriter, r *http.Request) {
	result := PrivEscResult1991{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			if c.SecurityContext == nil {
				result.Summary.NotSet++
				continue
			}

			if c.SecurityContext.AllowPrivilegeEscalation == nil {
				result.Summary.NotSet++
			} else if *c.SecurityContext.AllowPrivilegeEscalation {
				result.Summary.ExplicitTrue++
				result.Issues = append(result.Issues, PrivEscEntry1991{
					Pod: pod.Name, Namespace: pod.Namespace, Container: c.Name,
					Value: true, Issue: "allowPrivilegeEscalation: true",
				})
				score -= 5
			} else {
				result.Summary.ExplicitFalse++
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers: %d explicit false, %d explicit true, %d not set", result.Summary.TotalContainers, result.Summary.ExplicitFalse, result.Summary.ExplicitTrue, result.Summary.NotSet))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Seccomp Profile Audit
// ---------------------------------------------------------------

type SeccompResult1991 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         SeccompSummary1991 `json:"summary"`
	Issues          []SeccompEntry1991 `json:"issues"`
	Recommendations []string           `json:"recommendations"`
}

type SeccompSummary1991 struct {
	TotalContainers int `json:"totalContainers"`
	WithSeccomp     int `json:"withSeccompProfile"`
	RuntimeDefault  int `json:"runtimeDefault"`
	Unconfined      int `json:"unconfined"`
	NotSet          int `json:"notSet"`
}

type SeccompEntry1991 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
	Profile   string `json:"profile"`
}

func (s *Server) handleSeccompProfileV2(w http.ResponseWriter, r *http.Request) {
	result := SeccompResult1991{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		// Pod-level seccomp
		podProfile := ""
		if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.SeccompProfile != nil {
			podProfile = string(pod.Spec.SecurityContext.SeccompProfile.Type)
		}

		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			profile := podProfile
			if c.SecurityContext != nil && c.SecurityContext.SeccompProfile != nil {
				profile = string(c.SecurityContext.SeccompProfile.Type)
			}

			if profile == "" {
				result.Summary.NotSet++
			} else if profile == "RuntimeDefault" {
				result.Summary.RuntimeDefault++
				result.Summary.WithSeccomp++
			} else if profile == "Unconfined" {
				result.Summary.Unconfined++
				result.Summary.WithSeccomp++
				result.Issues = append(result.Issues, SeccompEntry1991{
					Pod: pod.Name, Namespace: pod.Namespace, Container: c.Name,
					Profile: "Unconfined",
				})
				score -= 3
			} else if profile == "Localhost" {
				result.Summary.WithSeccomp++
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers: %d RuntimeDefault, %d Unconfined, %d not set", result.Summary.TotalContainers, result.Summary.RuntimeDefault, result.Summary.Unconfined, result.Summary.NotSet))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Container CapDrop Audit
// ---------------------------------------------------------------

type CapDropResult1991 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         CapDropSummary1991 `json:"summary"`
	Containers      []CapDropEntry1991 `json:"containers"`
	Recommendations []string           `json:"recommendations"`
}

type CapDropSummary1991 struct {
	TotalContainers int `json:"totalContainers"`
	WithCapDrop     int `json:"withCapDrop"`
	WithCapAdd      int `json:"withCapAdd"`
	DroppedAll      int `json:"droppedAll"`
	HighRiskCapAdd  int `json:"highRiskCapAdd"`
}

type CapDropEntry1991 struct {
	Pod       string   `json:"pod"`
	Namespace string   `json:"namespace"`
	Container string   `json:"container"`
	CapDrop   []string `json:"capDrop"`
	CapAdd    []string `json:"capAdd"`
}

var highRiskCaps1991 = map[string]bool{
	"SYS_ADMIN": true, "SYS_MODULE": true, "SYS_PTRACE": true,
	"SYS_RAWIO": true, "NET_ADMIN": true, "NET_RAW": true,
	"DAC_READ_SEARCH": true, "DAC_OVERRIDE": true,
	"SETUID": true, "SETGID": true,
}

func (s *Server) handleCapDropAudit(w http.ResponseWriter, r *http.Request) {
	result := CapDropResult1991{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			hasDrop := false
			hasAdd := false
			if c.SecurityContext != nil && c.SecurityContext.Capabilities != nil {
				hasDrop = len(c.SecurityContext.Capabilities.Drop) > 0
				hasAdd = len(c.SecurityContext.Capabilities.Add) > 0
			}

			entry := CapDropEntry1991{
				Pod: pod.Name, Namespace: pod.Namespace, Container: c.Name,
			}

			if hasDrop && c.SecurityContext.Capabilities != nil {
				result.Summary.WithCapDrop++
				drops := c.SecurityContext.Capabilities.Drop
				entry.CapDrop = make([]string, len(drops))
				for i, d := range drops {
					entry.CapDrop[i] = string(d)
				}
				// Check if ALL dropped
				for _, d := range entry.CapDrop {
					if d == "ALL" {
						result.Summary.DroppedAll++
						break
					}
				}
			}

			if hasAdd && c.SecurityContext.Capabilities != nil {
				result.Summary.WithCapAdd++
				entry.CapAdd = make([]string, len(c.SecurityContext.Capabilities.Add))
				for i, a := range c.SecurityContext.Capabilities.Add {
					entry.CapAdd[i] = string(a)
					if highRiskCaps1991[string(a)] {
						result.Summary.HighRiskCapAdd++
						score -= 3
					}
				}
			}

			if hasDrop || hasAdd {
				result.Containers = append(result.Containers, entry)
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers: %d with capDrop (%d ALL), %d with capAdd (%d high-risk)", result.Summary.TotalContainers, result.Summary.WithCapDrop, result.Summary.DroppedAll, result.Summary.WithCapAdd, result.Summary.HighRiskCapAdd))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
