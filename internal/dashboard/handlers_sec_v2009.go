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
// v20.09 — Security Dimension (Round 21)
// 1. SA Image Pull Secret — service account pull secret coverage
// 2. Pod DNS Policy Restrict — DNS policy security compliance
// 3. Container RunAsUser — runAsUser (non-root) compliance
// ============================================================

// ---------------------------------------------------------------
// 1. SA Image Pull Secret
// ---------------------------------------------------------------

type SAPullSecResult2009 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         SAPullSecSummary2009 `json:"summary"`
	Without         []SAPullSecEntry2009 `json:"withoutPullSecret"`
	Recommendations []string             `json:"recommendations"`
}

type SAPullSecSummary2009 struct {
	TotalSAs    int `json:"totalServiceAccounts"`
	WithPullSec int `json:"withImagePullSecret"`
	Without     int `json:"withoutImagePullSecret"`
}

type SAPullSecEntry2009 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleSAPullSec2009(w http.ResponseWriter, r *http.Request) {
	result := SAPullSecResult2009{ScannedAt: time.Now()}
	score := 100

	saList, _ := s.clientset.CoreV1().ServiceAccounts("").List(r.Context(), metav1.ListOptions{})

	for _, sa := range saList.Items {
		result.Summary.TotalSAs++

		if len(sa.ImagePullSecrets) > 0 {
			result.Summary.WithPullSec++
		} else {
			result.Summary.Without++
			// Only flag non-default SAs
			if sa.Name != "default" {
				result.Without = append(result.Without, SAPullSecEntry2009{
					Name: sa.Name, Namespace: sa.Namespace,
				})
			}
		}
	}

	if len(result.Without) > 20 {
		score -= 5
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d SAs: %d with pull secret, %d without", result.Summary.TotalSAs, result.Summary.WithPullSec, result.Summary.Without))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Pod DNS Policy Restrict
// ---------------------------------------------------------------

type DNSPolResult2009 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         DNSPolSummary2009 `json:"summary"`
	Issues          []DNSPolEntry2009 `json:"issues"`
	Recommendations []string          `json:"recommendations"`
}

type DNSPolSummary2009 struct {
	TotalPods    int `json:"totalPods"`
	ClusterFirst int `json:"clusterFirstPolicy"`
	DefaultPol   int `json:"defaultPolicy"`
	DNSNone      int `json:"dnsNonePolicy"`
	NoneSet      int `json:"noExplicitPolicy"`
}

type DNSPolEntry2009 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Policy    string `json:"dnsPolicy"`
	Issue     string `json:"issue"`
}

func (s *Server) handleDNSPolRestrict2009(w http.ResponseWriter, r *http.Request) {
	result := DNSPolResult2009{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		policy := string(pod.Spec.DNSPolicy)

		switch policy {
		case "ClusterFirst":
			result.Summary.ClusterFirst++
		case "Default":
			result.Summary.DefaultPol++
		case "None":
			result.Summary.DNSNone++
			result.Issues = append(result.Issues, DNSPolEntry2009{
				Pod: pod.Name, Namespace: pod.Namespace,
				Policy: policy, Issue: "DNSNone policy — requires explicit DNS config",
			})
			score -= 2
		default:
			result.Summary.NoneSet++
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods: %d ClusterFirst, %d Default, %d DNSNone", result.Summary.TotalPods, result.Summary.ClusterFirst, result.Summary.DefaultPol, result.Summary.DNSNone))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Container RunAsUser
// ---------------------------------------------------------------

type RunAsUserResult2009 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         RunAsUserSummary2009 `json:"summary"`
	Issues          []RunAsUserEntry2009 `json:"issues"`
	Recommendations []string             `json:"recommendations"`
}

type RunAsUserSummary2009 struct {
	TotalContainers int `json:"totalContainers"`
	RunAsRoot       int `json:"runAsRoot"`
	RunAsNonRoot    int `json:"runAsNonRoot"`
	NotSet          int `json:"notSet"`
}

type RunAsUserEntry2009 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
	UID       int64  `json:"uid"`
}

func (s *Server) handleRunAsUser2009(w http.ResponseWriter, r *http.Request) {
	result := RunAsUserResult2009{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		// Check pod-level runAsUser
		podRunAsUser := int64(-1)
		if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.RunAsUser != nil {
			podRunAsUser = *pod.Spec.SecurityContext.RunAsUser
		}

		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			uid := podRunAsUser
			if c.SecurityContext != nil && c.SecurityContext.RunAsUser != nil {
				uid = *c.SecurityContext.RunAsUser
			}

			if uid == 0 {
				result.Summary.RunAsRoot++
				result.Issues = append(result.Issues, RunAsUserEntry2009{
					Pod: pod.Name, Namespace: pod.Namespace,
					Container: c.Name, UID: uid,
				})
				score -= 2
			} else if uid > 0 {
				result.Summary.RunAsNonRoot++
			} else {
				result.Summary.NotSet++
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers: %d root, %d non-root, %d not set", result.Summary.TotalContainers, result.Summary.RunAsRoot, result.Summary.RunAsNonRoot, result.Summary.NotSet))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// Suppress unused import warning
var _ = strings.Contains
