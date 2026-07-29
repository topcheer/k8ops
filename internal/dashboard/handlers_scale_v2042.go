package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v20.42 — Scalability & HA Dimension (Round 26)
// 1. Control Plane Pressure — API server request rate estimate
// 2. Etcd Size Forecast — etcd database size growth tracking
// 3. Scheduling Latency Estimator — pending pod scheduling time
// ============================================================

// ---------------------------------------------------------------
// 1. Control Plane Pressure
// ---------------------------------------------------------------

type CtrlPlaneResult2042 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         CtrlPlaneSummary2042 `json:"summary"`
	PressureFactors []CtrlPlaneEntry2042 `json:"pressureFactors"`
	Recommendations []string             `json:"recommendations"`
}

type CtrlPlaneSummary2042 struct {
	TotalObjects int `json:"totalObjects"`
	CRDCount     int `json:"crdCount"`
	WebhookCount int `json:"webhookCount"`
}

type CtrlPlaneEntry2042 struct {
	Factor string `json:"factor"`
	Count  int    `json:"count"`
	Impact string `json:"impact"`
}

func (s *Server) handleCtrlPlanePressure(w http.ResponseWriter, r *http.Request) {
	result := CtrlPlaneResult2042{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	serverGroups, _ := s.clientset.Discovery().ServerGroups()
	mwcList, _ := s.clientset.AdmissionregistrationV1().MutatingWebhookConfigurations().List(r.Context(), metav1.ListOptions{})
	vwcList, _ := s.clientset.AdmissionregistrationV1().ValidatingWebhookConfigurations().List(r.Context(), metav1.ListOptions{})

	result.Summary.TotalObjects = len(podList.Items)
	crdCount := 0
	if serverGroups != nil {
		for _, grp := range serverGroups.Groups {
			if !startsWithStr(grp.Name, "k8s.io") &&
				!startsWithStr(grp.Name, "kubernetes.io") &&
				!startsWithStr(grp.Name, "apiextensions") &&
				grp.Name != "" {
				crdCount++
			}
		}
	}
	result.Summary.CRDCount = crdCount
	result.Summary.WebhookCount = len(mwcList.Items) + len(vwcList.Items)

	// CRD pressure
	if crdCount > 50 {
		result.PressureFactors = append(result.PressureFactors, CtrlPlaneEntry2042{
			Factor: "CRDs", Count: crdCount, Impact: "high",
		})
		score -= 5
	} else if crdCount > 20 {
		result.PressureFactors = append(result.PressureFactors, CtrlPlaneEntry2042{
			Factor: "CRDs", Count: crdCount, Impact: "medium",
		})
		score -= 2
	}

	// Webhook pressure
	if result.Summary.WebhookCount > 10 {
		result.PressureFactors = append(result.PressureFactors, CtrlPlaneEntry2042{
			Factor: "Webhooks", Count: result.Summary.WebhookCount, Impact: "high",
		})
		score -= 5
	} else if result.Summary.WebhookCount > 5 {
		result.PressureFactors = append(result.PressureFactors, CtrlPlaneEntry2042{
			Factor: "Webhooks", Count: result.Summary.WebhookCount, Impact: "medium",
		})
		score -= 2
	}

	// Total objects pressure
	if len(podList.Items) > 500 {
		result.PressureFactors = append(result.PressureFactors, CtrlPlaneEntry2042{
			Factor: "Pods", Count: len(podList.Items), Impact: "high",
		})
		score -= 3
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.PressureFactors, func(i, j int) bool {
		return result.PressureFactors[i].Count > result.PressureFactors[j].Count
	})

	if len(result.PressureFactors) > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d control plane pressure factors — monitor API server latency", len(result.PressureFactors)))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Etcd Size Forecast
// ---------------------------------------------------------------

type EtcdForecastResult2042 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         EtcdForecastSummary2042 `json:"summary"`
	Recommendations []string                `json:"recommendations"`
}

type EtcdForecastSummary2042 struct {
	ConfigMaps int `json:"configMaps"`
	Secrets    int `json:"secrets"`
	Events     int `json:"events"`
	LargeCMs   int `json:"largeConfigMaps"`
}

func (s *Server) handleEtcdForecast(w http.ResponseWriter, r *http.Request) {
	result := EtcdForecastResult2042{ScannedAt: time.Now()}
	score := 100

	cmList, _ := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	eventList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})

	result.Summary.ConfigMaps = len(cmList.Items)
	result.Summary.Secrets = len(secretList.Items)
	result.Summary.Events = len(eventList.Items)

	for _, cm := range cmList.Items {
		totalSize := 0
		for _, v := range cm.Data {
			totalSize += len(v)
		}
		for _, v := range cm.BinaryData {
			totalSize += len(v)
		}
		if totalSize > 500000 { // >500KB
			result.Summary.LargeCMs++
			score -= 2
		}
	}

	// Events are major etcd consumer
	if result.Summary.Events > 10000 {
		score -= 5
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.LargeCMs > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d ConfigMaps >500KB — these increase etcd size significantly", result.Summary.LargeCMs))
	}
	if result.Summary.Events > 10000 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d events — consider event TTL reduction to save etcd space", result.Summary.Events))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Scheduling Latency Estimator
// ---------------------------------------------------------------

type SchedLatencyResult2042 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         SchedLatencySummary2042 `json:"summary"`
	PendingPods     []SchedLatencyEntry2042 `json:"pendingPods"`
	Recommendations []string                `json:"recommendations"`
}

type SchedLatencySummary2042 struct {
	TotalPods   int `json:"totalPods"`
	PendingPods int `json:"pendingPods"`
	FailedSched int `json:"failedScheduling"`
}

type SchedLatencyEntry2042 struct {
	Pod         string `json:"pod"`
	Namespace   string `json:"namespace"`
	PendingTime int    `json:"pendingSeconds"`
	Reason      string `json:"reason"`
}

func (s *Server) handleSchedLatency(w http.ResponseWriter, r *http.Request) {
	result := SchedLatencyResult2042{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	now := time.Now()
	for _, pod := range podList.Items {
		result.Summary.TotalPods++

		if pod.Status.Phase == corev1.PodPending {
			result.Summary.PendingPods++
			pendingSecs := 0
			if !pod.CreationTimestamp.IsZero() {
				pendingSecs = int(now.Sub(pod.CreationTimestamp.Time).Seconds())
			}

			reason := "unknown"
			for _, cond := range pod.Status.Conditions {
				if cond.Reason != "" {
					reason = cond.Reason
				}
				if cond.Message != "" && reason == "" {
					reason = cond.Message
				}
			}
			if containsStr2039(reason, "FailedScheduling") {
				result.Summary.FailedSched++
				score -= 5
			}

			result.PendingPods = append(result.PendingPods, SchedLatencyEntry2042{
				Pod: pod.Name, Namespace: pod.Namespace,
				PendingTime: pendingSecs, Reason: reason,
			})
			score -= 2
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.PendingPods, func(i, j int) bool {
		return result.PendingPods[i].PendingTime > result.PendingPods[j].PendingTime
	})

	if result.Summary.PendingPods > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pods pending — check scheduler and node capacity", result.Summary.PendingPods))
	}

	writeJSON(w, result)
}

// keep import
var _ = autoscalingv2.HorizontalPodAutoscaler{}

func startsWithStr(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
