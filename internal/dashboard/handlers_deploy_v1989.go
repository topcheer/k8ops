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
// v19.89 — Deployment Dimension (Round 18)
// 1. Container Stdin/TTY — interactive mode misconfiguration
// 2. Pod DNS Config — custom DNS resolver compliance
// 3. Host Alias Audit — /etc/hosts override tracking
// ============================================================

// ---------------------------------------------------------------
// 1. Container Stdin/TTY
// ---------------------------------------------------------------

type StdinTTYResult1989 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         StdinTTYSummary1989 `json:"summary"`
	Issues          []StdinTTYEntry1989 `json:"issues"`
	Recommendations []string            `json:"recommendations"`
}

type StdinTTYSummary1989 struct {
	TotalContainers int `json:"totalContainers"`
	WithStdin       int `json:"withStdin"`
	WithTTY         int `json:"withTTY"`
	BothStdinTTY    int `json:"withBothStdinTTY"`
}

type StdinTTYEntry1989 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
	HasStdin  bool   `json:"hasStdin"`
	HasTTY    bool   `json:"hasTTY"`
}

func (s *Server) handleStdinTTY(w http.ResponseWriter, r *http.Request) {
	result := StdinTTYResult1989{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			hasStdin := c.Stdin
			hasTTY := c.TTY

			if hasStdin {
				result.Summary.WithStdin++
			}
			if hasTTY {
				result.Summary.WithTTY++
			}
			if hasStdin && hasTTY {
				result.Summary.BothStdinTTY++
				result.Issues = append(result.Issues, StdinTTYEntry1989{
					Pod: pod.Name, Namespace: pod.Namespace, Container: c.Name,
					HasStdin: true, HasTTY: true,
				})
				score -= 1
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers: %d with stdin, %d with TTY, %d with both", result.Summary.TotalContainers, result.Summary.WithStdin, result.Summary.WithTTY, result.Summary.BothStdinTTY))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Pod DNS Config
// ---------------------------------------------------------------

type PodDNSResult1989 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         PodDNSSummary1989 `json:"summary"`
	Pods            []PodDNSEntry1989 `json:"pods"`
	Recommendations []string          `json:"recommendations"`
}

type PodDNSSummary1989 struct {
	TotalPods       int `json:"totalPods"`
	WithDNSConfig   int `json:"withCustomDNSConfig"`
	WithNameservers int `json:"withCustomNameservers"`
	WithSearches    int `json:"withCustomSearchDomains"`
	DNSNonePolicy   int `json:"dnsNonePolicy"`
}

type PodDNSEntry1989 struct {
	Pod         string   `json:"pod"`
	Namespace   string   `json:"namespace"`
	Policy      string   `json:"dnsPolicy"`
	Nameservers []string `json:"nameservers"`
}

func (s *Server) handlePodDNSConfig(w http.ResponseWriter, r *http.Request) {
	result := PodDNSResult1989{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		policy := string(pod.Spec.DNSPolicy)
		hasCustom := pod.Spec.DNSConfig != nil

		entry := PodDNSEntry1989{
			Pod: pod.Name, Namespace: pod.Namespace, Policy: policy,
		}

		if hasCustom {
			result.Summary.WithDNSConfig++
			entry.Nameservers = pod.Spec.DNSConfig.Nameservers
			if len(entry.Nameservers) > 0 {
				result.Summary.WithNameservers++
			}
			if len(pod.Spec.DNSConfig.Searches) > 0 {
				result.Summary.WithSearches++
			}
		}

		if pod.Spec.DNSPolicy == corev1.DNSNone {
			result.Summary.DNSNonePolicy++
			score -= 2
		}

		if hasCustom {
			result.Pods = append(result.Pods, entry)
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods: %d with custom DNS, %d DNSNone policy", result.Summary.TotalPods, result.Summary.WithDNSConfig, result.Summary.DNSNonePolicy))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Host Alias Audit
// ---------------------------------------------------------------

type HostAliasResult1989 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         HostAliasSummary1989 `json:"summary"`
	Pods            []HostAliasEntry1989 `json:"pods"`
	Recommendations []string             `json:"recommendations"`
}

type HostAliasSummary1989 struct {
	TotalPods     int `json:"totalPods"`
	WithHostAlias int `json:"withHostAlias"`
	TotalAliases  int `json:"totalAliases"`
}

type HostAliasEntry1989 struct {
	Pod        string   `json:"pod"`
	Namespace  string   `json:"namespace"`
	AliasCount int      `json:"aliasCount"`
	Aliases    []string `json:"aliases"`
}

func (s *Server) handleHostAliasAudit(w http.ResponseWriter, r *http.Request) {
	result := HostAliasResult1989{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		aliases := pod.Spec.HostAliases
		if len(aliases) > 0 {
			result.Summary.WithHostAlias++
			result.Summary.TotalAliases += len(aliases)

			entry := HostAliasEntry1989{
				Pod: pod.Name, Namespace: pod.Namespace,
				AliasCount: len(aliases),
			}
			for _, a := range aliases {
				entry.Aliases = append(entry.Aliases, a.IP+" -> "+joinStrings1989(a.Hostnames))
			}
			result.Pods = append(result.Pods, entry)
			score -= 1
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods: %d with host aliases, %d total aliases", result.Summary.TotalPods, result.Summary.WithHostAlias, result.Summary.TotalAliases))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

func joinStrings1989(ss []string) string {
	result := ""
	for i, s := range ss {
		if i > 0 {
			result += ", "
		}
		result += s
	}
	return result
}
