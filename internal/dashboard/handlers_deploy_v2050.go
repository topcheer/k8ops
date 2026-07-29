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
// v20.50 — Deployment Dimension (Round 28)
// 1. StatefulSet Update Compliance — OnDelete vs RollingUpdate
// 2. Pod DNS Policy Audit — dnsPolicy and dnsConfig settings
// 3. Container Args Standardization — command/args consistency
// ============================================================

// ---------------------------------------------------------------
// 1. StatefulSet Update Compliance
// ---------------------------------------------------------------

type STSUpdateResult2050 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         STSUpdateSummary2050 `json:"summary"`
	OnDeleteSTS     []STSUpdateEntry2050 `json:"onDeleteStatefulSets"`
	Recommendations []string             `json:"recommendations"`
}

type STSUpdateSummary2050 struct {
	TotalSTS      int `json:"totalStatefulSets"`
	RollingUpdate int `json:"rollingUpdate"`
	OnDelete      int `json:"onDelete"`
}

type STSUpdateEntry2050 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Replicas  int32  `json:"replicas"`
}

func (s *Server) handleSTSUpdateCompliance(w http.ResponseWriter, r *http.Request) {
	result := STSUpdateResult2050{ScannedAt: time.Now()}
	score := 100

	stsList, _ := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})

	for _, sts := range stsList.Items {
		result.Summary.TotalSTS++

		strategy := sts.Spec.UpdateStrategy.Type
		replicas := int32(1)
		if sts.Spec.Replicas != nil {
			replicas = *sts.Spec.Replicas
		}

		if strategy == appsv1.RollingUpdateStatefulSetStrategyType || strategy == "" {
			result.Summary.RollingUpdate++
		} else if strategy == appsv1.OnDeleteStatefulSetStrategyType {
			result.Summary.OnDelete++
			result.OnDeleteSTS = append(result.OnDeleteSTS, STSUpdateEntry2050{
				Name: sts.Name, Namespace: sts.Namespace, Replicas: replicas,
			})
			if replicas > 1 {
				score -= 5
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.OnDeleteSTS, func(i, j int) bool {
		return result.OnDeleteSTS[i].Replicas > result.OnDeleteSTS[j].Replicas
	})

	if result.Summary.OnDelete > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d StatefulSets use OnDelete — switch to RollingUpdate for automatic updates", result.Summary.OnDelete))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Pod DNS Policy Audit
// ---------------------------------------------------------------

type DNSPolicyResult2050 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         DNSPolicySummary2050 `json:"summary"`
	CustomDNS       []DNSPolicyEntry2050 `json:"customDNSPods"`
	Recommendations []string             `json:"recommendations"`
}

type DNSPolicySummary2050 struct {
	TotalPods    int `json:"totalPods"`
	DefaultDNS   int `json:"defaultDNS"`
	ClusterFirst int `json:"clusterFirst"`
	CustomDNS    int `json:"customDNS"`
	None         int `json:"noneDNS"`
}

type DNSPolicyEntry2050 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Policy    string `json:"policy"`
}

func (s *Server) handleDNSPolicyAudit2050(w http.ResponseWriter, r *http.Request) {
	result := DNSPolicyResult2050{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		policy := string(pod.Spec.DNSPolicy)
		if policy == "" {
			policy = "ClusterFirst"
		}

		switch policy {
		case "ClusterFirst":
			result.Summary.ClusterFirst++
		case "Default":
			result.Summary.DefaultDNS++
		case "None":
			result.Summary.None++
			result.CustomDNS = append(result.CustomDNS, DNSPolicyEntry2050{
				Pod: pod.Name, Namespace: pod.Namespace, Policy: "None",
			})
			score -= 3
		}

		if pod.Spec.DNSConfig != nil {
			result.Summary.CustomDNS++
			result.CustomDNS = append(result.CustomDNS, DNSPolicyEntry2050{
				Pod: pod.Name, Namespace: pod.Namespace, Policy: policy + "+customConfig",
			})
			score -= 1
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.CustomDNS, func(i, j int) bool {
		return result.CustomDNS[i].Namespace < result.CustomDNS[j].Namespace
	})

	if result.Summary.None > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pods have dnsPolicy=None — ensure custom DNS config is correct", result.Summary.None))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Container Args Standardization
// ---------------------------------------------------------------

type CmdArgsResult2050 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         CmdArgsSummary2050 `json:"summary"`
	OverriddenCmd   []CmdArgsEntry2050 `json:"overriddenCommands"`
	Recommendations []string           `json:"recommendations"`
}

type CmdArgsSummary2050 struct {
	TotalContainers int `json:"totalContainers"`
	WithCommand     int `json:"withCommand"`
	WithArgs        int `json:"withArgs"`
	NoCmd           int `json:"noCommandOrArgs"`
}

type CmdArgsEntry2050 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Container string `json:"container"`
}

func (s *Server) handleCmdArgsStandard(w http.ResponseWriter, r *http.Request) {
	result := CmdArgsResult2050{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++

			hasCmd := len(c.Command) > 0
			hasArgs := len(c.Args) > 0

			if hasCmd {
				result.Summary.WithCommand++
				result.OverriddenCmd = append(result.OverriddenCmd, CmdArgsEntry2050{
					Pod: pod.Name, Namespace: pod.Namespace, Container: c.Name,
				})
			} else if hasArgs {
				result.Summary.WithArgs++
			} else {
				result.Summary.NoCmd++
			}
		}
	}

	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.OverriddenCmd, func(i, j int) bool {
		return result.OverriddenCmd[i].Namespace < result.OverriddenCmd[j].Namespace
	})

	if result.Summary.WithCommand > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d containers override the image command — verify this is intentional", result.Summary.WithCommand))
	}

	writeJSON(w, result)
}
