package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

// ExternalSecretHealthResult is the external secrets & secret store CSI health analysis.
type ExternalSecretHealthResult struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	Summary         ExtSecretSummary       `json:"summary"`
	Secrets         []ExtSecretEntry       `json:"secrets"`
	ProviderClasses []ESProviderClassEntry `json:"providerClasses"`
	PodHealth       []ESPodHealth          `json:"podHealth"`
	Issues          []ExtSecretIssue       `json:"issues"`
	Recommendations []string               `json:"recommendations"`
	HealthScore     int                    `json:"healthScore"`
}

// ExtSecretSummary aggregates external secrets statistics.
type ExtSecretSummary struct {
	ESODetected     bool   `json:"esoDetected"`
	CSIDetected     bool   `json:"csiDetected"`
	ESOVersion      string `json:"esoVersion,omitempty"`
	TotalSecrets    int    `json:"totalSecrets"`
	SyncedSecrets   int    `json:"syncedSecrets"`
	FailedSecrets   int    `json:"failedSecrets"`
	ProviderClasses int    `json:"providerClasses"`
	PodCount        int    `json:"podCount"`
	ReadyPods       int    `json:"readyPods"`
}

// ExtSecretEntry describes one ExternalSecret.
type ExtSecretEntry struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	StoreRef     string `json:"storeRef"`
	Status       string `json:"status"`
	TargetSecret string `json:"targetSecret"`
	RiskLevel    string `json:"riskLevel"`
}

// ESProviderClassEntry describes one SecretProviderClass.
type ESProviderClassEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Provider  string `json:"provider"`
	RiskLevel string `json:"riskLevel"`
}

// ESPodHealth describes an external secrets pod's health.
type ESPodHealth struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Ready     bool   `json:"ready"`
	Restarts  int    `json:"restarts"`
	Image     string `json:"image"`
}

// ExtSecretIssue is a detected external secrets problem.
type ExtSecretIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleExternalSecretHealth audits external secrets & secret store CSI health.
// GET /api/product/external-secret-health
func (s *Server) handleExternalSecretHealth(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	ctx := r.Context()

	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := &ExternalSecretHealthResult{
		ScannedAt: time.Now(),
	}

	// 1. Detect ESO and CSI pods
	allPods, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	var esoPods []corev1.Pod
	var csiPods []corev1.Pod
	var esoImage string

	for i := range allPods.Items {
		pod := &allPods.Items[i]
		podName := strings.ToLower(pod.Name)
		for _, c := range pod.Spec.Containers {
			img := strings.ToLower(c.Image)
			if strings.Contains(podName, "external-secrets") || strings.Contains(img, "external-secrets") {
				esoPods = append(esoPods, *pod)
				if esoImage == "" {
					esoImage = c.Image
				}
			}
			if strings.Contains(podName, "secrets-store-csi") || strings.Contains(img, "secrets-store-csi") {
				csiPods = append(csiPods, *pod)
			}
		}
	}

	esoDetected := len(esoPods) > 0
	csiDetected := len(csiPods) > 0

	// 2. Check pod health
	allManagedPods := append(esoPods, csiPods...)
	readyPods := 0
	for _, pod := range allManagedPods {
		ready := false
		for _, cond := range pod.Status.Conditions {
			if cond.Type == corev1.PodReady && cond.Status == corev1.ConditionTrue {
				ready = true
				break
			}
		}
		if ready {
			readyPods++
		}
		restarts := 0
		for _, cs := range pod.Status.ContainerStatuses {
			restarts += int(cs.RestartCount)
		}
		result.PodHealth = append(result.PodHealth, ESPodHealth{
			Name:      pod.Name,
			Namespace: pod.Namespace,
			Ready:     ready,
			Restarts:  restarts,
			Image:     getFirstImage(pod),
		})
		if !ready {
			result.Issues = append(result.Issues, ExtSecretIssue{
				Severity: "critical",
				Type:     "pod-not-ready",
				Resource: fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
				Message:  "External Secrets pod is not ready — secret synchronization may be impaired",
			})
		}
		if restarts > 3 {
			result.Issues = append(result.Issues, ExtSecretIssue{
				Severity: "warning",
				Type:     "high-restarts",
				Resource: fmt.Sprintf("%s/%s", pod.Namespace, pod.Name),
				Message:  fmt.Sprintf("Pod has %d restarts — may indicate instability", restarts),
			})
		}
	}

	// 3. Try dynamic client for ExternalSecret CRDs
	var secrets []ExtSecretEntry
	var providerClasses []ESProviderClassEntry

	if rc.restConfig != nil {
		dynClient, err := dynamic.NewForConfig(rc.restConfig)
		if err == nil {
			// ExternalSecrets: external-secrets.io/v1beta1
			esGVR := schema.GroupVersionResource{
				Group:    "external-secrets.io",
				Version:  "v1beta1",
				Resource: "externalsecrets",
			}
			esList, err := dynClient.Resource(esGVR).List(ctx, metav1.ListOptions{})
			if err == nil && esList != nil {
				for _, item := range esList.Items {
					entry := ExtSecretEntry{
						Name:      item.GetName(),
						Namespace: item.GetNamespace(),
					}
					if store, ok, _ := unstructured.NestedString(item.Object, "spec", "secretStoreRef", "name"); ok {
						entry.StoreRef = store
					}
					if target, ok, _ := unstructured.NestedString(item.Object, "spec", "target", "name"); ok {
						entry.TargetSecret = target
					} else {
						entry.TargetSecret = item.GetName()
					}
					// Check status conditions
					condType := ""
					if conditions, ok, _ := unstructured.NestedSlice(item.Object, "status", "conditions"); ok && len(conditions) > 0 {
						if firstCond, ok := conditions[0].(map[string]interface{}); ok {
							if ct, ok := firstCond["type"].(string); ok {
								condType = ct
								entry.Status = ct
							}
							if condStatus, ok := firstCond["status"].(string); ok {
								if condType == "Ready" && condStatus == "True" {
									entry.Status = "Ready"
								}
							}
						}
					}
					if entry.Status == "" {
						entry.Status = "Unknown"
					}
					entry.RiskLevel = assessESRisk(entry)
					secrets = append(secrets, entry)
				}
			}

			// SecretProviderClasses: secrets-store.csi.x-k8s.io/v1
			spcGVR := schema.GroupVersionResource{
				Group:    "secrets-store.csi.x-k8s.io",
				Version:  "v1",
				Resource: "secretproviderclasses",
			}
			spcList, err := dynClient.Resource(spcGVR).List(ctx, metav1.ListOptions{})
			if err == nil && spcList != nil {
				for _, item := range spcList.Items {
					entry := ESProviderClassEntry{
						Name:      item.GetName(),
						Namespace: item.GetNamespace(),
						RiskLevel: "healthy",
					}
					if provider, ok, _ := unstructured.NestedString(item.Object, "spec", "provider"); ok {
						entry.Provider = provider
					}
					providerClasses = append(providerClasses, entry)
				}
			}
		}
	}

	// 4. Count sync stats
	syncedCount := 0
	failedCount := 0
	for _, es := range secrets {
		if es.Status == "Ready" || es.Status == "Synced" {
			syncedCount++
		} else if strings.Contains(strings.ToLower(es.Status), "fail") || strings.Contains(strings.ToLower(es.Status), "error") {
			failedCount++
			result.Issues = append(result.Issues, ExtSecretIssue{
				Severity: "warning",
				Type:     "secret-sync-failed",
				Resource: fmt.Sprintf("%s/%s", es.Namespace, es.Name),
				Message:  fmt.Sprintf("ExternalSecret '%s' sync status: %s", es.Name, es.Status),
			})
		}
	}

	sort.Slice(secrets, func(i, j int) bool {
		if secrets[i].Namespace != secrets[j].Namespace {
			return secrets[i].Namespace < secrets[j].Namespace
		}
		return secrets[i].Name < secrets[j].Name
	})

	// 5. Recommendations
	var recommendations []string
	if !esoDetected && !csiDetected {
		recommendations = append(recommendations, "No External Secrets Operator or Secret Store CSI Driver detected — consider installing for automated secret management")
	}
	if esoDetected && len(secrets) == 0 {
		recommendations = append(recommendations, "ESO is installed but no ExternalSecrets found — define ExternalSecrets to sync from external secret stores")
	}
	if failedCount > 0 {
		recommendations = append(recommendations, fmt.Sprintf("%d ExternalSecret(s) failed to sync — check store credentials and connectivity", failedCount))
	}
	if len(allManagedPods) > 0 && readyPods < len(allManagedPods) {
		recommendations = append(recommendations, fmt.Sprintf("%d/%d pod(s) are not ready — check pod logs", len(allManagedPods)-readyPods, len(allManagedPods)))
	}
	if len(recommendations) == 0 {
		recommendations = append(recommendations, "External secrets management is healthy — all secrets are syncing properly")
	}

	result.Secrets = secrets
	result.ProviderClasses = providerClasses
	result.Recommendations = recommendations
	result.Summary = ExtSecretSummary{
		ESODetected:     esoDetected,
		CSIDetected:     csiDetected,
		ESOVersion:      extractESVersion(esoImage),
		TotalSecrets:    len(secrets),
		SyncedSecrets:   syncedCount,
		FailedSecrets:   failedCount,
		ProviderClasses: len(providerClasses),
		PodCount:        len(allManagedPods),
		ReadyPods:       readyPods,
	}
	result.HealthScore = computeESHealthScore(result.Summary, len(result.Issues))

	writeJSON(w, result)
}

// getFirstImage returns the first container image.
func getFirstImage(pod corev1.Pod) string {
	if len(pod.Spec.Containers) > 0 {
		return pod.Spec.Containers[0].Image
	}
	return ""
}

// extractESVersion extracts version from image string.
func extractESVersion(image string) string {
	if image == "" {
		return ""
	}
	parts := strings.Split(image, ":")
	if len(parts) < 2 {
		return ""
	}
	ver := parts[len(parts)-1]
	if idx := strings.Index(ver, "-"); idx != -1 {
		ver = ver[:idx]
	}
	return ver
}

// assessESRisk determines risk level of an ExternalSecret.
func assessESRisk(entry ExtSecretEntry) string {
	status := strings.ToLower(entry.Status)
	if strings.Contains(status, "fail") || strings.Contains(status, "error") {
		return "critical"
	}
	if status == "unknown" || status == "" {
		return "warning"
	}
	if strings.Contains(status, "pending") || strings.Contains(status, "syncing") {
		return "info"
	}
	return "healthy"
}

// computeESHealthScore computes a 0-100 health score.
func computeESHealthScore(s ExtSecretSummary, issueCount int) int {
	if !s.ESODetected && !s.CSIDetected {
		return 50
	}
	score := 100
	score -= (s.PodCount - s.ReadyPods) * 15
	score -= s.FailedSecrets * 5
	score -= issueCount * 1
	if s.ESODetected && s.TotalSecrets == 0 {
		score -= 5
	}
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}
	return score
}
