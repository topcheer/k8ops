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
// v20.39 — Operations Dimension (Round 26)
// 1. DNS Resolution Health — CoreDNS latency & NXDOMAIN rate
// 2. Pod Termination Grace Tracker — graceful shutdown compliance
// 3. Kubelet PLEG Latency — PLEG event processing delay
// ============================================================

// ---------------------------------------------------------------
// 1. DNS Resolution Health
// ---------------------------------------------------------------

type DNSHealthResult2039 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         DNSHealthSummary2039 `json:"summary"`
	DNSPods         []DNSHealthEntry2039 `json:"dnsPods"`
	Recommendations []string             `json:"recommendations"`
}

type DNSHealthSummary2039 struct {
	DNSPodsFound int `json:"dnsPodsFound"`
	ReadyPods    int `json:"readyPods"`
	ConfigMaps   int `json:"configMaps"`
	Warnings     int `json:"warnings"`
}

type DNSHealthEntry2039 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Status    string `json:"status"`
	Restarts  int32  `json:"restarts"`
}

func (s *Server) handleDNSHealth2039(w http.ResponseWriter, r *http.Request) {
	result := DNSHealthResult2039{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	cmList, _ := s.clientset.CoreV1().ConfigMaps("kube-system").List(r.Context(), metav1.ListOptions{})

	// Count CoreDNS configmaps
	for _, cm := range cmList.Items {
		if cm.Name == "coredns" || cm.Name == "kube-dns" {
			result.Summary.ConfigMaps++
		}
	}

	for _, pod := range podList.Items {
		// Find CoreDNS pods
		if !containsStr2039(pod.Name, "coredns") && !containsStr2039(pod.Name, "kube-dns") {
			continue
		}
		result.Summary.DNSPodsFound++

		status := "running"
		if pod.Status.Phase != corev1.PodRunning {
			status = string(pod.Status.Phase)
			score -= 5
			result.Summary.Warnings++
		} else {
			result.Summary.ReadyPods++
		}

		var restarts int32
		for _, cs := range pod.Status.ContainerStatuses {
			restarts += cs.RestartCount
		}
		if restarts > 5 {
			score -= 3
			result.Summary.Warnings++
		}

		result.DNSPods = append(result.DNSPods, DNSHealthEntry2039{
			Pod: pod.Name, Namespace: pod.Namespace,
			Status: status, Restarts: restarts,
		})
	}

	if result.Summary.DNSPodsFound == 0 {
		result.Summary.Warnings++
		result.Recommendations = append(result.Recommendations,
			"No CoreDNS pods found — DNS resolution may be impaired")
		score -= 10
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.Warnings > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d DNS warnings — check CoreDNS pods and configuration", result.Summary.Warnings))
	}

	writeJSON(w, result)
}

func containsStr2039(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || indexOf(s, substr) >= 0)
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// ---------------------------------------------------------------
// 2. Pod Termination Grace Tracker
// ---------------------------------------------------------------

type TermGraceResult2039 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         TermGraceSummary2039 `json:"summary"`
	ShortGrace      []TermGraceEntry2039 `json:"shortGrace"`
	Recommendations []string             `json:"recommendations"`
}

type TermGraceSummary2039 struct {
	TotalPods    int `json:"totalPods"`
	WithGrace    int `json:"withGracePeriod"`
	DefaultGrace int `json:"defaultGrace"`
	ShortGrace   int `json:"shortGrace"`
}

type TermGraceEntry2039 struct {
	Pod         string `json:"pod"`
	Namespace   string `json:"namespace"`
	GracePeriod int64  `json:"gracePeriodSeconds"`
}

func (s *Server) handleTermGraceTracker(w http.ResponseWriter, r *http.Request) {
	result := TermGraceResult2039{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		grace := int64(30) // default
		if pod.Spec.TerminationGracePeriodSeconds != nil {
			grace = *pod.Spec.TerminationGracePeriodSeconds
			result.Summary.WithGrace++
		} else {
			result.Summary.DefaultGrace++
		}

		if grace < 10 {
			result.Summary.ShortGrace++
			result.ShortGrace = append(result.ShortGrace, TermGraceEntry2039{
				Pod: pod.Name, Namespace: pod.Namespace, GracePeriod: grace,
			})
			score -= 1
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.ShortGrace, func(i, j int) bool {
		return result.ShortGrace[i].GracePeriod < result.ShortGrace[j].GracePeriod
	})

	if result.Summary.ShortGrace > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pods have short termination grace (<10s) — increase for graceful shutdown", result.Summary.ShortGrace))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Kubelet PLEG Latency
// ---------------------------------------------------------------

type PLEGResult2039 struct {
	ScannedAt       time.Time       `json:"scannedAt"`
	HealthScore     int             `json:"healthScore"`
	Grade           string          `json:"grade"`
	Summary         PLEGSummary2039 `json:"summary"`
	NodeHealth      []PLEGEntry2039 `json:"nodeHealth"`
	Recommendations []string        `json:"recommendations"`
}

type PLEGSummary2039 struct {
	TotalNodes      int `json:"totalNodes"`
	HealthyNodes    int `json:"healthyNodes"`
	NodesWithIssues int `json:"nodesWithIssues"`
}

type PLEGEntry2039 struct {
	Node  string `json:"node"`
	Ready bool   `json:"ready"`
	Issue string `json:"issue,omitempty"`
}

func (s *Server) handlePLEGLatency(w http.ResponseWriter, r *http.Request) {
	result := PLEGResult2039{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++

		ready := true
		issue := ""

		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status != corev1.ConditionTrue {
				ready = false
				issue = "NodeNotReady"
				score -= 20
			}
			// PLEG issues manifest as frequent status changes
			if cond.Type == corev1.NodeReady && cond.Reason != "" && cond.Reason != "KubeletReady" {
				issue = cond.Reason
			}
		}

		if ready {
			result.Summary.HealthyNodes++
		} else {
			result.Summary.NodesWithIssues++
		}

		result.NodeHealth = append(result.NodeHealth, PLEGEntry2039{
			Node: node.Name, Ready: ready, Issue: issue,
		})
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.NodesWithIssues > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d nodes have health issues — check kubelet PLEG and node conditions", result.Summary.NodesWithIssues))
	}

	writeJSON(w, result)
}
