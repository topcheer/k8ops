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

// EtcdHealthResult is the etcd health & database size analysis.
type EtcdHealthResult struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	Summary         EtcdHealthSummary  `json:"summary"`
	EtcdPods        []EtcdPodEntry     `json:"etcdPods"`
	Issues          []EtcdIssue        `json:"issues"`
	ConfigMaps      []LargeObjectEntry `json:"largeConfigMaps"`
	Secrets         []LargeObjectEntry `json:"largeSecrets"`
	Recommendations []string           `json:"recommendations"`
}

// EtcdHealthSummary aggregates etcd health statistics.
type EtcdHealthSummary struct {
	EtcdFound         int    `json:"etcdFound"`
	EtcdReady         int    `json:"etcdReady"`
	EtcdNotReady      int    `json:"etcdNotReady"`
	EtcdVersion       string `json:"etcdVersion,omitempty"`
	EtcdNamespace     string `json:"etcdNamespace,omitempty"`
	TotalConfigMaps   int    `json:"totalConfigMaps"`
	TotalSecrets      int    `json:"totalSecrets"`
	LargeObjects      int    `json:"largeObjects"`      // objects > 100KB
	EtcdPressureScore int    `json:"etcdPressureScore"` // 0-100 (100 = low pressure)
	HealthScore       int    `json:"healthScore"`
}

// EtcdPodEntry describes an etcd pod.
type EtcdPodEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Node      string `json:"nodeName"`
	Status    string `json:"status"`
	Ready     bool   `json:"ready"`
	Restarts  int    `json:"restarts"`
}

// EtcdIssue is a detected etcd-related issue.
type EtcdIssue struct {
	Severity string `json:"severity"`
	Category string `json:"category"`
	Message  string `json:"message"`
}

// LargeObjectEntry describes a large ConfigMap or Secret.
type LargeObjectEntry struct {
	Name      string  `json:"name"`
	Namespace string  `json:"namespace"`
	Type      string  `json:"type"` // ConfigMap or Secret
	SizeKB    float64 `json:"sizeKB"`
	Severity  string  `json:"severity"`
}

// handleEtcdHealth monitors etcd health and database pressure.
// GET /api/operations/etcd-health
func (s *Server) handleEtcdHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	cms, _ := rc.clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{})
	secrets, _ := rc.clientset.CoreV1().Secrets("").List(ctx, metav1.ListOptions{})

	now := time.Now()
	result := EtcdHealthResult{ScannedAt: now}
	result.Summary.TotalConfigMaps = len(cms.Items)
	result.Summary.TotalSecrets = len(secrets.Items)

	// Find etcd pods
	var etcdNamespace string
	for _, pod := range pods.Items {
		if !isEtcdPod(&pod) {
			continue
		}
		etcdNamespace = pod.Namespace
		result.Summary.EtcdFound++

		ready := isPodReadyGeneral(&pod)
		entry := EtcdPodEntry{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Node:      pod.Spec.NodeName,
			Status:    string(pod.Status.Phase),
			Ready:     ready,
			Restarts:  getTotalRestarts(&pod),
		}
		if ready {
			result.Summary.EtcdReady++
		} else {
			result.Summary.EtcdNotReady++
			result.Issues = append(result.Issues, EtcdIssue{
				Severity: "critical",
				Category: "etcd_unhealthy",
				Message:  fmt.Sprintf("etcd pod %s is not ready", pod.Name),
			})
		}

		// Extract version
		if result.Summary.EtcdVersion == "" {
			for _, c := range pod.Spec.Containers {
				image := c.Image
				parts := strings.Split(image, ":")
				if len(parts) > 1 {
					result.Summary.EtcdVersion = parts[len(parts)-1]
				}
			}
		}
		result.EtcdPods = append(result.EtcdPods, entry)
	}

	result.Summary.EtcdNamespace = etcdNamespace

	if result.Summary.EtcdFound == 0 {
		result.Issues = append(result.Issues, EtcdIssue{
			Severity: "medium",
			Category: "etcd_not_found",
			Message:  "No etcd pods found — may be running externally or using managed Kubernetes",
		})
	}

	if result.Summary.EtcdFound == 1 {
		result.Issues = append(result.Issues, EtcdIssue{
			Severity: "high",
			Category: "single_etcd",
			Message:  "Only 1 etcd instance — no quorum redundancy, deploy at least 3 for HA",
		})
	}

	// Check for large ConfigMaps (etcd pressure)
	for _, cm := range cms.Items {
		size := configMapSizeKB(&cm)
		if size > 100 {
			result.Summary.LargeObjects++
			severity := "medium"
			if size > 500 {
				severity = "high"
			}
			if size > 1000 {
				severity = "critical"
			}
			result.ConfigMaps = append(result.ConfigMaps, LargeObjectEntry{
				Name:      cm.Name,
				Namespace: cm.Namespace,
				Type:      "ConfigMap",
				SizeKB:    float64(int(size*100)) / 100,
				Severity:  severity,
			})
		}
	}

	// Check for large Secrets
	for _, sec := range secrets.Items {
		size := secretSizeKB(&sec)
		if size > 100 {
			result.Summary.LargeObjects++
			severity := "medium"
			if size > 500 {
				severity = "high"
			}
			if size > 1000 {
				severity = "critical"
			}
			result.Secrets = append(result.Secrets, LargeObjectEntry{
				Name:      sec.Name,
				Namespace: sec.Namespace,
				Type:      "Secret",
				SizeKB:    float64(int(size*100)) / 100,
				Severity:  severity,
			})
		}
	}

	// Sort large objects by size
	sort.Slice(result.ConfigMaps, func(i, j int) bool {
		return result.ConfigMaps[i].SizeKB > result.ConfigMaps[j].SizeKB
	})
	sort.Slice(result.Secrets, func(i, j int) bool {
		return result.Secrets[i].SizeKB > result.Secrets[j].SizeKB
	})
	if len(result.ConfigMaps) > 20 {
		result.ConfigMaps = result.ConfigMaps[:20]
	}
	if len(result.Secrets) > 20 {
		result.Secrets = result.Secrets[:20]
	}

	// Sort issues by severity
	sort.Slice(result.Issues, func(i, j int) bool {
		sevOrder := map[string]int{"critical": 0, "high": 1, "medium": 2, "low": 3}
		return sevOrder[result.Issues[i].Severity] < sevOrder[result.Issues[j].Severity]
	})

	result.Summary.EtcdPressureScore = etcdPressureScore(result.Summary)
	result.Summary.HealthScore = etcdHealthScore(result.Summary)
	result.Recommendations = etcdHealthRecommendations(&result)

	writeJSON(w, result)
}

// isEtcdPod checks if a pod is an etcd instance.
func isEtcdPod(pod *corev1.Pod) bool {
	name := strings.ToLower(pod.Name)
	for _, c := range pod.Spec.Containers {
		image := strings.ToLower(c.Image)
		if strings.Contains(image, "etcd") || strings.Contains(name, "etcd") {
			return true
		}
	}
	return false
}

// isPodReadyGeneral checks if a pod is ready (avoids collision with other packages).
func isPodReadyGeneral(pod *corev1.Pod) bool {
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

// configMapSizeKB estimates the size of a ConfigMap in KB.
func configMapSizeKB(cm *corev1.ConfigMap) float64 {
	total := 0
	for _, v := range cm.Data {
		total += len(v)
	}
	for _, v := range cm.BinaryData {
		total += len(v)
	}
	return float64(total) / 1024
}

// secretSizeKB estimates the size of a Secret in KB.
func secretSizeKB(sec *corev1.Secret) float64 {
	total := 0
	for _, v := range sec.Data {
		total += len(v)
	}
	for _, v := range sec.StringData {
		total += len(v)
	}
	return float64(total) / 1024
}

// etcdPressureScore computes a 0-100 score (100 = low pressure).
func etcdPressureScore(s EtcdHealthSummary) int {
	score := 100
	if s.LargeObjects > 0 {
		score -= min(30, s.LargeObjects*5)
	}
	return score
}

// etcdHealthScore computes overall etcd health.
func etcdHealthScore(s EtcdHealthSummary) int {
	score := 100

	if s.EtcdFound == 0 {
		return 80 // May be managed, not critical
	}

	if s.EtcdNotReady > 0 {
		notReadyRatio := float64(s.EtcdNotReady) / float64(s.EtcdFound)
		score -= int(notReadyRatio * 50)
	}

	if s.EtcdFound == 1 {
		score -= 20
	}

	if s.LargeObjects > 0 {
		score -= min(20, s.LargeObjects*3)
	}

	if score < 0 {
		score = 0
	}
	return score
}

// etcdHealthRecommendations generates actionable recommendations.
func etcdHealthRecommendations(r *EtcdHealthResult) []string {
	var recs []string

	if r.Summary.EtcdNotReady > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d etcd pod(s) are not ready — investigate immediately as this affects cluster stability",
			r.Summary.EtcdNotReady,
		))
	}

	if r.Summary.EtcdFound == 1 {
		recs = append(recs, "Only 1 etcd instance — deploy at least 3 etcd nodes for HA quorum")
	}

	if len(r.ConfigMaps) > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d large ConfigMap(s) detected (>100KB) — move large data to external storage to reduce etcd pressure",
			len(r.ConfigMaps),
		))
	}

	if len(r.Secrets) > 0 {
		recs = append(recs, fmt.Sprintf(
			"%d large Secret(s) detected (>100KB) — use external secret management for large credentials",
			len(r.Secrets),
		))
	}

	if r.Summary.TotalConfigMaps+r.Summary.TotalSecrets > 10000 {
		recs = append(recs, fmt.Sprintf(
			"%d total ConfigMaps + Secrets — etcd may be under pressure, consider periodic cleanup of unused objects",
			r.Summary.TotalConfigMaps+r.Summary.TotalSecrets,
		))
	}

	if len(recs) == 0 {
		recs = append(recs, "etcd is healthy with no significant database pressure detected")
	}

	return recs
}
