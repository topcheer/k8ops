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
// v19.71 — Deployment Dimension (Round 15)
// 1. Resource Request Gap — containers without requests/limits
// 2. Container Port Map — port exposure within pods
// 3. Termination Message Audit — termination message config compliance
// ============================================================

// ---------------------------------------------------------------
// 1. Resource Request Gap
// ---------------------------------------------------------------

type ResReqGapResult1971 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         ResReqGapSummary1971 `json:"summary"`
	Gaps            []ResReqGapEntry1971 `json:"gaps"`
	Recommendations []string             `json:"recommendations"`
}

type ResReqGapSummary1971 struct {
	TotalContainers int `json:"totalContainers"`
	WithRequests    int `json:"withRequests"`
	WithoutRequests int `json:"withoutRequests"`
	WithLimits      int `json:"withLimits"`
	WithoutLimits   int `json:"withoutLimits"`
	WithoutBoth     int `json:"withoutBoth"`
}

type ResReqGapEntry1971 struct {
	Pod        string `json:"pod"`
	Namespace  string `json:"namespace"`
	Container  string `json:"container"`
	HasRequest bool   `json:"hasRequest"`
	HasLimit   bool   `json:"hasLimit"`
}

func (s *Server) handleResourceRequestGap(w http.ResponseWriter, r *http.Request) {
	result := ResReqGapResult1971{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			hasReq := !c.Resources.Requests.Cpu().IsZero() || !c.Resources.Requests.Memory().IsZero()
			hasLim := !c.Resources.Limits.Cpu().IsZero() || !c.Resources.Limits.Memory().IsZero()

			if hasReq {
				result.Summary.WithRequests++
			} else {
				result.Summary.WithoutRequests++
			}
			if hasLim {
				result.Summary.WithLimits++
			} else {
				result.Summary.WithoutLimits++
			}
			if !hasReq && !hasLim {
				result.Summary.WithoutBoth++
				result.Gaps = append(result.Gaps, ResReqGapEntry1971{
					Pod: pod.Name, Namespace: pod.Namespace, Container: c.Name,
					HasRequest: false, HasLimit: false,
				})
				score -= 2
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers: %d without requests, %d without limits, %d without both", result.Summary.TotalContainers, result.Summary.WithoutRequests, result.Summary.WithoutLimits, result.Summary.WithoutBoth))
	if result.Summary.WithoutBoth > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers with no resource constraints — add requests and limits", result.Summary.WithoutBoth))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Container Port Map
// ---------------------------------------------------------------

type ContainerPortResult1971 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         ContainerPortSummary1971 `json:"summary"`
	PortMappings    []ContainerPortEntry1971 `json:"portMappings"`
	Duplicates      []ContainerPortDup1971   `json:"duplicatePorts"`
	Recommendations []string                 `json:"recommendations"`
}

type ContainerPortSummary1971 struct {
	TotalContainers int `json:"totalContainers"`
	WithPorts       int `json:"containersWithPorts"`
	TotalPorts      int `json:"totalPorts"`
	NamedPorts      int `json:"namedPorts"`
	HostPorts       int `json:"hostPorts"`
}

type ContainerPortEntry1971 struct {
	Pod         string `json:"pod"`
	Namespace   string `json:"namespace"`
	Container   string `json:"container"`
	Port        int32  `json:"port"`
	Name        string `json:"name"`
	Protocol    string `json:"protocol"`
	HasHostPort bool   `json:"hasHostPort"`
}

type ContainerPortDup1971 struct {
	Port       int32    `json:"port"`
	Containers []string `json:"containers"`
}

func (s *Server) handleContainerPortMap(w http.ResponseWriter, r *http.Request) {
	result := ContainerPortResult1971{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	// Track host port usage
	hostPortMap := make(map[int32][]string)

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			if len(c.Ports) == 0 {
				continue
			}
			result.Summary.WithPorts++

			for _, p := range c.Ports {
				result.Summary.TotalPorts++
				entry := ContainerPortEntry1971{
					Pod: pod.Name, Namespace: pod.Namespace, Container: c.Name,
					Port: p.ContainerPort, Name: p.Name,
					Protocol: string(p.Protocol),
				}
				if p.Name != "" {
					result.Summary.NamedPorts++
				}
				if p.HostPort > 0 {
					entry.HasHostPort = true
					result.Summary.HostPorts++
					hostPortMap[p.HostPort] = append(hostPortMap[p.HostPort], fmt.Sprintf("%s/%s/%s", pod.Namespace, pod.Name, c.Name))
					score -= 3
				}
				result.PortMappings = append(result.PortMappings, entry)
			}
		}
	}

	// Detect duplicate host ports
	for hp, containers := range hostPortMap {
		if len(containers) > 1 {
			result.Duplicates = append(result.Duplicates, ContainerPortDup1971{
				Port: hp, Containers: containers,
			})
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers, %d ports (%d named, %d host ports)", result.Summary.TotalContainers, result.Summary.TotalPorts, result.Summary.NamedPorts, result.Summary.HostPorts))
	if result.Summary.HostPorts > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d host ports bound — restrict for security and portability", result.Summary.HostPorts))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Termination Message Audit
// ---------------------------------------------------------------

type TermMsgResult1971 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         TermMsgSummary1971 `json:"summary"`
	Issues          []TermMsgEntry1971 `json:"issues"`
	Recommendations []string           `json:"recommendations"`
}

type TermMsgSummary1971 struct {
	TotalContainers   int `json:"totalContainers"`
	WithTermMsgPath   int `json:"withTerminationMessagePath"`
	WithTermMsgPolicy int `json:"withTerminationMessagePolicy"`
	CustomPolicy      int `json:"customPolicyCount"`
	FallbackPolicy    int `json:"fallbackPolicyCount"`
}

type TermMsgEntry1971 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
	Issue     string `json:"issue"`
}

func (s *Server) handleTerminationMsgAudit(w http.ResponseWriter, r *http.Request) {
	result := TermMsgResult1971{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			hasPath := c.TerminationMessagePath != ""
			policy := string(c.TerminationMessagePolicy)
			if policy == "" {
				policy = "File" // default
			}

			if hasPath {
				result.Summary.WithTermMsgPath++
			}
			if policy != "" {
				result.Summary.WithTermMsgPolicy++
			}

			if policy == "File" && !hasPath {
				// Default behavior — no explicit path set
				result.Summary.FallbackPolicy++
			} else if policy == "FallbackToLogsOnError" {
				result.Summary.CustomPolicy++
			}

			// Check for issues
			if policy == "File" && hasPath {
				// Verify path doesn't point to sensitive location
				if strings.Contains(c.TerminationMessagePath, "/etc/") || strings.Contains(c.TerminationMessagePath, "/proc/") {
					result.Issues = append(result.Issues, TermMsgEntry1971{
						Pod: pod.Name, Namespace: pod.Namespace, Container: c.Name,
						Issue: fmt.Sprintf("Termination message path in sensitive location: %s", c.TerminationMessagePath),
					})
					score -= 3
				}
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d containers: %d with explicit term path, %d FallbackToLogsOnError policy", result.Summary.TotalContainers, result.Summary.WithTermMsgPath, result.Summary.CustomPolicy))
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("Use FallbackToLogsOnError for better crash diagnostics (%d containers using default File policy)", result.Summary.FallbackPolicy))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
