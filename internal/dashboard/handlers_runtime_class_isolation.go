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

// RuntimeClassIsolationResult audits RuntimeClass adoption for container sandbox isolation.
type RuntimeClassIsolationResult struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	Summary         RCIsolationSummary    `json:"summary"`
	RuntimeClasses  []RCIsolationClass    `json:"runtimeClasses"`
	ByNamespace     []RCIsolationNSEntry  `json:"byNamespace"`
	UnprotectedPods []RCIsolationPodEntry `json:"unprotectedPods"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Recommendations []string              `json:"recommendations"`
}

type RCIsolationSummary struct {
	TotalPods        int `json:"totalPods"`
	WithRuntimeClass int `json:"withRuntimeClass"`
	AvailableClasses int `json:"availableRuntimeClasses"`
	NoRuntimeClass   int `json:"withoutRuntimeClass"`
	GVisorPods       int `json:"gVisorPods"`
	KataPods         int `json:"kataPods"`
}

type RCIsolationClass struct {
	Name    string `json:"name"`
	Handler string `json:"handler"`
	InUse   bool   `json:"inUse"`
}

type RCIsolationNSEntry struct {
	Namespace string `json:"namespace"`
	PodCount  int    `json:"podCount"`
	Protected int    `json:"protectedPods"`
}

type RCIsolationPodEntry struct {
	PodName   string `json:"podName"`
	Namespace string `json:"namespace"`
	Severity  string `json:"severity"`
	Reason    string `json:"reason"`
}

// handleRuntimeClassIsolation handles GET /api/security/runtime-class-audit
func (s *Server) handleRuntimeClassIsolation(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}
	result := RuntimeClassIsolationResult{ScannedAt: time.Now()}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})

	rcList, err := rc.clientset.NodeV1().RuntimeClasses().List(ctx, metav1.ListOptions{})
	availableRCs := make(map[string]bool)
	if err == nil {
		for _, class := range rcList.Items {
			availableRCs[class.Name] = true
			result.RuntimeClasses = append(result.RuntimeClasses, RCIsolationClass{
				Name: class.Name, Handler: class.Handler,
			})
			result.Summary.AvailableClasses++
		}
	}

	nsMap := make(map[string]*RCIsolationNSEntry)

	for _, pod := range pods.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase == corev1.PodFailed {
			continue
		}
		result.Summary.TotalPods++

		if nsMap[pod.Namespace] == nil {
			nsMap[pod.Namespace] = &RCIsolationNSEntry{Namespace: pod.Namespace}
		}
		nsMap[pod.Namespace].PodCount++

		if pod.Spec.RuntimeClassName != nil {
			rcName := *pod.Spec.RuntimeClassName
			result.Summary.WithRuntimeClass++
			nsMap[pod.Namespace].Protected++

			if strings.Contains(strings.ToLower(rcName), "gvisor") {
				result.Summary.GVisorPods++
			}
			if strings.Contains(strings.ToLower(rcName), "kata") {
				result.Summary.KataPods++
			}
			for i := range result.RuntimeClasses {
				if result.RuntimeClasses[i].Name == rcName {
					result.RuntimeClasses[i].InUse = true
				}
			}
		} else {
			result.Summary.NoRuntimeClass++
			result.UnprotectedPods = append(result.UnprotectedPods, RCIsolationPodEntry{
				PodName: pod.Name, Namespace: pod.Namespace,
				Severity: "medium", Reason: "no RuntimeClass - uses default runc",
			})
		}
	}

	for _, e := range nsMap {
		result.ByNamespace = append(result.ByNamespace, *e)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].Protected < result.ByNamespace[j].Protected
	})

	if result.Summary.TotalPods > 0 {
		result.HealthScore = result.Summary.WithRuntimeClass * 100 / result.Summary.TotalPods
	} else {
		result.HealthScore = 100
	}
	gradeFromScore(&result.Grade, result.HealthScore)

	result.Recommendations = []string{
		fmt.Sprintf("RuntimeClass 隔离审计: %d Pod, %d 有 RC, %d 无 RC, %d 可用类, %d gVisor, %d Kata",
			result.Summary.TotalPods, result.Summary.WithRuntimeClass,
			result.Summary.NoRuntimeClass, result.Summary.AvailableClasses,
			result.Summary.GVisorPods, result.Summary.KataPods),
	}
	if result.Summary.NoRuntimeClass > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d 个 Pod 使用默认 runc, 建议对不可信工作负载使用 gVisor/Kata", result.Summary.NoRuntimeClass))
	}
	writeJSON(w, result)
}
