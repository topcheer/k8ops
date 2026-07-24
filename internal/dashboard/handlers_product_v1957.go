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
// v19.57 — Product Dimension (Round 12)
// 1. Horizontal Scale Limit Analysis — HPA max replica gap
// 2. ConfigMap Key Exposure — sensitive key detection in CMs
// 3. PVC Access Pattern — mount mode & R/W distribution
// ============================================================

// ---------------------------------------------------------------
// 1. Horizontal Scale Limit Analysis
// ---------------------------------------------------------------

type ScaleLimitResult1957 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         ScaleLimitSummary1957    `json:"summary"`
	Gaps            []ScaleLimitEntry1957    `json:"gaps"`
	HPAs            []ScaleLimitHPAEntry1957 `json:"hpas"`
	Recommendations []string                 `json:"recommendations"`
}

type ScaleLimitSummary1957 struct {
	TotalDeployments int     `json:"totalDeployments"`
	WithHPA          int     `json:"withHPA"`
	WithoutHPA       int     `json:"withoutHPA"`
	MaxReplicaGap    int     `json:"maxReplicaGapCount"`
	MinReplicas1     int     `json:"minReplicasOne"`
	AvgMaxReplicas   float64 `json:"avgMaxReplicas"`
}

type ScaleLimitEntry1957 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Replicas  int32  `json:"replicas"`
	Reason    string `json:"reason"`
}

type ScaleLimitHPAEntry1957 struct {
	Name            string `json:"name"`
	Namespace       string `json:"namespace"`
	MinReplicas     int32  `json:"minReplicas"`
	MaxReplicas     int32  `json:"maxReplicas"`
	CurrentReplicas int32  `json:"currentReplicas"`
}

func (s *Server) handleScaleLimitAnalysis(w http.ResponseWriter, r *http.Request) {
	result := ScaleLimitResult1957{ScannedAt: time.Now()}
	score := 100

	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	hpaList, _ := s.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(r.Context(), metav1.ListOptions{})

	hpaByDep := make(map[string]*ScaleLimitHPAEntry1957)
	var totalMaxReplicas float64
	hpaCount := 0
	for _, hpa := range hpaList.Items {
		if isSystemNamespace(hpa.Namespace) {
			continue
		}
		key := fmt.Sprintf("%s/%s", hpa.Namespace, hpa.Spec.ScaleTargetRef.Name)

		minRep := int32(1)
		maxRep := int32(10)
		curRep := hpa.Status.CurrentReplicas

		if hpa.Spec.MinReplicas != nil {
			minRep = *hpa.Spec.MinReplicas
		}
		if hpa.Spec.MaxReplicas != 0 {
			maxRep = hpa.Spec.MaxReplicas
		}

		entry := &ScaleLimitHPAEntry1957{
			Name: hpa.Name, Namespace: hpa.Namespace,
			MinReplicas: minRep, MaxReplicas: maxRep, CurrentReplicas: curRep,
		}
		hpaByDep[key] = entry
		totalMaxReplicas += float64(maxRep)
		hpaCount++

		if minRep == 1 {
			result.Summary.MinReplicas1++
		}
		if maxRep-minRep < 2 {
			result.Summary.MaxReplicaGap++
		}
	}

	for _, dep := range depList.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		result.Summary.TotalDeployments++

		key := fmt.Sprintf("%s/%s", dep.Namespace, dep.Name)
		replicas := int32(1)
		if dep.Spec.Replicas != nil {
			replicas = *dep.Spec.Replicas
		}

		if hpa, ok := hpaByDep[key]; ok {
			result.Summary.WithHPA++
			result.HPAs = append(result.HPAs, *hpa)
		} else {
			result.Summary.WithoutHPA++
			if replicas >= 2 {
				result.Gaps = append(result.Gaps, ScaleLimitEntry1957{
					Name: dep.Name, Namespace: dep.Namespace,
					Replicas: replicas,
					Reason:   "Multi-replica without HPA — no auto-scaling",
				})
				score -= 1
			}
		}
	}

	if hpaCount > 0 {
		result.Summary.AvgMaxReplicas = totalMaxReplicas / float64(hpaCount)
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.WithoutHPA > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d deployments without HPA — add auto-scaling", result.Summary.WithoutHPA))
	}
	if result.Summary.MaxReplicaGap > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d HPAs with <2 replica gap — increase maxReplicas headroom", result.Summary.MaxReplicaGap))
	}
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d/%d with HPA, avg max %.0f replicas",
		result.Summary.WithHPA, result.Summary.TotalDeployments, result.Summary.AvgMaxReplicas))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. ConfigMap Key Exposure
// ---------------------------------------------------------------

type CMKeyExposureResult1957 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         CMKeyExposureSummary1957 `json:"summary"`
	Sensitive       []CMKeyExposureEntry1957 `json:"sensitiveKeys"`
	ByNS            []CMKeyExposureNS1957    `json:"byNamespace"`
	Recommendations []string                 `json:"recommendations"`
}

type CMKeyExposureSummary1957 struct {
	TotalCMs       int `json:"totalConfigMaps"`
	WithSensitive  int `json:"cmWithSensitiveKeys"`
	TotalKeys      int `json:"totalDataKeys"`
	SensitiveKeys  int `json:"sensitiveKeyCount"`
	ShouldBeSecret int `json:"shouldbeSecretCount"`
}

type CMKeyExposureEntry1957 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Key       string `json:"key"`
	Reason    string `json:"reason"`
}

type CMKeyExposureNS1957 struct {
	Namespace      string `json:"namespace"`
	CMCount        int    `json:"cmCount"`
	SensitiveCount int    `json:"sensitiveCount"`
}

var sensitiveKeyPatterns = []string{"password", "passwd", "secret", "token", "apikey", "api_key", "credential", "private_key", "access_key", "auth"}

func (s *Server) handleCMKeyExposure(w http.ResponseWriter, r *http.Request) {
	result := CMKeyExposureResult1957{ScannedAt: time.Now()}
	score := 100
	nsStats := make(map[string]*CMKeyExposureNS1957)

	cmList, _ := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})

	for _, cm := range cmList.Items {
		if isSystemNamespace(cm.Namespace) {
			continue
		}
		result.Summary.TotalCMs++

		if nsStats[cm.Namespace] == nil {
			nsStats[cm.Namespace] = &CMKeyExposureNS1957{Namespace: cm.Namespace}
		}
		nsStats[cm.Namespace].CMCount++

		hasSensitive := false
		for key := range cm.Data {
			result.Summary.TotalKeys++
			kl := strings.ToLower(key)
			for _, pattern := range sensitiveKeyPatterns {
				if strings.Contains(kl, pattern) {
					result.Summary.SensitiveKeys++
					result.Summary.ShouldBeSecret++
					hasSensitive = true
					if len(result.Sensitive) < 100 {
						result.Sensitive = append(result.Sensitive, CMKeyExposureEntry1957{
							Name: cm.Name, Namespace: cm.Namespace,
							Key: key, Reason: fmt.Sprintf("Key '%s' matches sensitive pattern — should be Secret", key),
						})
					}
					break
				}
			}
		}
		if hasSensitive {
			result.Summary.WithSensitive++
			nsStats[cm.Namespace].SensitiveCount++
			score -= 2
		}
	}

	for _, ns := range nsStats {
		result.ByNS = append(result.ByNS, *ns)
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.SensitiveKeys > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d sensitive keys in ConfigMaps — move to Secrets", result.Summary.SensitiveKeys))
	}
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d ConfigMaps, %d total keys, %d sensitive",
		result.Summary.TotalCMs, result.Summary.TotalKeys, result.Summary.SensitiveKeys))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. PVC Access Pattern
// ---------------------------------------------------------------

type PVCAccessResult1957 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         PVCAccessSummary1957 `json:"summary"`
	PVCs            []PVCAccessEntry1957 `json:"pvcs"`
	ByNS            []PVCAccessNS1957    `json:"byNamespace"`
	Recommendations []string             `json:"recommendations"`
}

type PVCAccessSummary1957 struct {
	TotalPVCs  int `json:"totalPVCs"`
	ReadWrite  int `json:"readWrite"`
	ReadOnly   int `json:"readOnly"`
	RWOPVCs    int `json:"rwoPVCs"`
	RWXPVCs    int `json:"rwxPVCs"`
	MultiMount int `json:"multiMountPVCs"`
}

type PVCAccessEntry1957 struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	AccessMode string `json:"accessMode"`
	MountCount int    `json:"mountCount"`
	Size       string `json:"size"`
}

type PVCAccessNS1957 struct {
	Namespace string `json:"namespace"`
	PVCCount  int    `json:"pvcCount"`
}

func (s *Server) handlePVCAccessPattern(w http.ResponseWriter, r *http.Request) {
	result := PVCAccessResult1957{ScannedAt: time.Now()}
	score := 100
	nsStats := make(map[string]int)

	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	// Count mounts per PVC
	mountCount := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, vol := range pod.Spec.Volumes {
			if vol.PersistentVolumeClaim != nil {
				key := fmt.Sprintf("%s/%s", pod.Namespace, vol.PersistentVolumeClaim.ClaimName)
				mountCount[key]++
			}
		}
	}

	for _, pvc := range pvcList.Items {
		if isSystemNamespace(pvc.Namespace) {
			continue
		}
		result.Summary.TotalPVCs++
		nsStats[pvc.Namespace]++

		accessMode := ""
		if len(pvc.Spec.AccessModes) > 0 {
			accessMode = string(pvc.Spec.AccessModes[0])
		}

		key := fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name)
		mc := mountCount[key]

		switch accessMode {
		case "ReadWriteOnce":
			result.Summary.RWOPVCs++
		case "ReadWriteMany":
			result.Summary.RWXPVCs++
		}

		if mc > 1 {
			result.Summary.MultiMount++
			if accessMode == "ReadWriteOnce" {
				score -= 3
			}
		}

		result.PVCs = append(result.PVCs, PVCAccessEntry1957{
			Name: pvc.Name, Namespace: pvc.Namespace,
			AccessMode: accessMode, MountCount: mc,
			Size: pvc.Spec.Resources.Requests.Storage().String(),
		})
	}

	for ns, c := range nsStats {
		result.ByNS = append(result.ByNS, PVCAccessNS1957{Namespace: ns, PVCCount: c})
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.MultiMount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d PVCs mounted by multiple pods — verify access safety", result.Summary.MultiMount))
	}
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d PVCs (%d RWO, %d RWX, %d multi-mount)",
		result.Summary.TotalPVCs, result.Summary.RWOPVCs, result.Summary.RWXPVCs, result.Summary.MultiMount))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
