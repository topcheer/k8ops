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
// v20.02 — Operations Dimension (Round 20)
// 1. Pod Init Time — container startup duration analyzer
// 2. Kubelet Cert Expiry — node serving certificate staleness
// 3. Namespace Event Noise — event volume & spam per namespace
// ============================================================

// ---------------------------------------------------------------
// 1. Pod Init Time
// ---------------------------------------------------------------

type PodInitResult2002 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         PodInitSummary2002 `json:"summary"`
	SlowPods        []PodInitEntry2002 `json:"slowPods"`
	Recommendations []string           `json:"recommendations"`
}

type PodInitSummary2002 struct {
	TotalPods  int     `json:"totalPods"`
	AvgInitSec float64 `json:"avgInitSeconds"`
	MaxInitSec float64 `json:"maxInitSeconds"`
	SlowPods   int     `json:"slowPodsOver60s"`
	FastPods   int     `json:"fastPodsUnder5s"`
}

type PodInitEntry2002 struct {
	Name      string  `json:"name"`
	Namespace string  `json:"namespace"`
	InitSec   float64 `json:"initSeconds"`
}

func (s *Server) handlePodInitTime(w http.ResponseWriter, r *http.Request) {
	result := PodInitResult2002{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	var totalInit float64
	var maxInit float64
	var count int

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		var createdTime, readyTime time.Time
		if !pod.CreationTimestamp.IsZero() {
			createdTime = pod.CreationTimestamp.Time
		}
		for _, cs := range pod.Status.ContainerStatuses {
			if cs.Ready && cs.State.Running != nil && !cs.State.Running.StartedAt.IsZero() {
				if readyTime.IsZero() || cs.State.Running.StartedAt.Time.After(readyTime) {
					readyTime = cs.State.Running.StartedAt.Time
				}
			}
		}

		if createdTime.IsZero() || readyTime.IsZero() {
			continue
		}

		initSec := readyTime.Sub(createdTime).Seconds()
		if initSec < 0 {
			initSec = 0
		}

		result.Summary.TotalPods++
		count++
		totalInit += initSec
		if initSec > maxInit {
			maxInit = initSec
		}

		if initSec > 60 {
			result.Summary.SlowPods++
			result.SlowPods = append(result.SlowPods, PodInitEntry2002{
				Name: pod.Name, Namespace: pod.Namespace, InitSec: initSec,
			})
			score -= 2
		} else if initSec < 5 {
			result.Summary.FastPods++
		}
	}

	if count > 0 {
		result.Summary.AvgInitSec = totalInit / float64(count)
	}
	result.Summary.MaxInitSec = maxInit

	sort.Slice(result.SlowPods, func(i, j int) bool {
		return result.SlowPods[i].InitSec > result.SlowPods[j].InitSec
	})
	if len(result.SlowPods) > 20 {
		result.SlowPods = result.SlowPods[:20]
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pods, avg init %.1fs, max %.1fs, %d slow (>60s)", result.Summary.TotalPods, result.Summary.AvgInitSec, result.Summary.MaxInitSec, result.Summary.SlowPods))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Kubelet Cert Expiry
// ---------------------------------------------------------------

type KubeletCertResult2002 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         KubeletCertSummary2002 `json:"summary"`
	PerNode         []KubeletCertEntry2002 `json:"perNode"`
	Recommendations []string               `json:"recommendations"`
}

type KubeletCertSummary2002 struct {
	TotalNodes   int `json:"totalNodes"`
	WithCertInfo int `json:"nodesWithCertInfo"`
	ExpiringSoon int `json:"expiringWithin30d"`
	Expired      int `json:"expiredCerts"`
}

type KubeletCertEntry2002 struct {
	Name    string `json:"name"`
	Ready   bool   `json:"nodeReady"`
	CertAge string `json:"estimatedCertAge"`
}

func (s *Server) handleKubeletCertExpiry(w http.ResponseWriter, r *http.Request) {
	result := KubeletCertResult2002{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++

		isReady := false
		for _, cond := range node.Status.Conditions {
			if cond.Type == corev1.NodeReady && cond.Status == corev1.ConditionTrue {
				isReady = true
			}
		}

		// Estimate cert age from node creation time (kubelet certs rotate ~1 year)
		certAge := "unknown"
		expiring := false
		expired := false
		if !node.CreationTimestamp.IsZero() {
			ageDays := time.Since(node.CreationTimestamp.Time).Hours() / 24
			if ageDays > 365 {
				certAge = fmt.Sprintf("%.0fd (expired)", ageDays)
				expired = true
			} else if ageDays > 335 {
				certAge = fmt.Sprintf("%.0fd (expiring soon)", ageDays)
				expiring = true
			} else {
				certAge = fmt.Sprintf("%.0fd", ageDays)
			}
		}

		entry := KubeletCertEntry2002{
			Name: node.Name, Ready: isReady, CertAge: certAge,
		}
		result.PerNode = append(result.PerNode, entry)

		if expiring {
			result.Summary.ExpiringSoon++
			score -= 3
		}
		if expired {
			result.Summary.Expired++
			score -= 5
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d nodes: %d expiring soon, %d expired", result.Summary.TotalNodes, result.Summary.ExpiringSoon, result.Summary.Expired))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Namespace Event Noise
// ---------------------------------------------------------------

type NSEventNoiseResult2002 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         NSEventNoiseSummary2002 `json:"summary"`
	NoisyNS         []NSEventNoiseEntry2002 `json:"noisyNamespaces"`
	Recommendations []string                `json:"recommendations"`
}

type NSEventNoiseSummary2002 struct {
	TotalEvents int     `json:"totalEvents"`
	TotalNS     int     `json:"totalNamespaces"`
	AvgPerNS    float64 `json:"avgEventsPerNS"`
	NoisyNS     int     `json:"noisyNamespaces"`
}

type NSEventNoiseEntry2002 struct {
	Namespace string `json:"namespace"`
	Events    int    `json:"eventCount"`
	Warnings  int    `json:"warningCount"`
}

func (s *Server) handleNSEventNoise(w http.ResponseWriter, r *http.Request) {
	result := NSEventNoiseResult2002{ScannedAt: time.Now()}
	score := 100

	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})

	nsStats := make(map[string]*NSEventNoiseEntry2002)
	for _, evt := range eventList.Items {
		result.Summary.TotalEvents++

		entry, ok := nsStats[evt.Namespace]
		if !ok {
			entry = &NSEventNoiseEntry2002{Namespace: evt.Namespace}
			nsStats[evt.Namespace] = entry
		}
		entry.Events++
		if evt.Type == "Warning" {
			entry.Warnings++
		}
	}

	result.Summary.TotalNS = len(nsStats)
	if result.Summary.TotalNS > 0 {
		result.Summary.AvgPerNS = float64(result.Summary.TotalEvents) / float64(result.Summary.TotalNS)
	}

	for _, entry := range nsStats {
		if entry.Events > int(result.Summary.AvgPerNS)*3 && entry.Events > 20 {
			result.Summary.NoisyNS++
			result.NoisyNS = append(result.NoisyNS, *entry)
			score -= 2
		}
	}

	sort.Slice(result.NoisyNS, func(i, j int) bool {
		return result.NoisyNS[i].Events > result.NoisyNS[j].Events
	})
	if len(result.NoisyNS) > 10 {
		result.NoisyNS = result.NoisyNS[:10]
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d events across %d NS, avg %.0f/NS, %d noisy", result.Summary.TotalEvents, result.Summary.TotalNS, result.Summary.AvgPerNS, result.Summary.NoisyNS))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
