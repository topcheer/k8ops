package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v19.74 — Documentation Dimension (Round 15)
// 1. ConfigMap Catalog — all ConfigMaps with key count & mount status
// 2. HPA Catalog — autoscaling policy inventory
// 3. PDB Catalog — PodDisruptionBudget coverage inventory
// ============================================================

// ---------------------------------------------------------------
// 1. ConfigMap Catalog
// ---------------------------------------------------------------

type ConfigMapCatResult1974 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         ConfigMapCatSummary1974 `json:"summary"`
	ConfigMaps      []ConfigMapCatEntry1974 `json:"configMaps"`
	Unused          []ConfigMapCatEntry1974 `json:"unusedConfigMaps"`
	Recommendations []string                `json:"recommendations"`
}

type ConfigMapCatSummary1974 struct {
	TotalConfigMaps int `json:"totalConfigMaps"`
	TotalKeys       int `json:"totalKeys"`
	BinaryData      int `json:"configsWithBinaryData"`
	LargeConfigs    int `json:"largeConfigMaps"`
	UnusedConfigs   int `json:"unusedConfigMaps"`
}

type ConfigMapCatEntry1974 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	KeyCount  int    `json:"keyCount"`
	DataSize  int    `json:"dataSizeKB"`
	HasBinary bool   `json:"hasBinaryData"`
	Age       string `json:"age"`
}

func (s *Server) handleConfigMapCatalog(w http.ResponseWriter, r *http.Request) {
	result := ConfigMapCatResult1974{ScannedAt: time.Now()}
	score := 100

	cmList, _ := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	// Build used CM set from pod volumes
	usedCM := make(map[string]bool)
	for _, pod := range podList.Items {
		for _, vol := range pod.Spec.Volumes {
			if vol.ConfigMap != nil {
				usedCM[pod.Namespace+"/"+vol.ConfigMap.Name] = true
			}
			for _, env := range podAllEnvRefs(pod) {
				if env.ConfigMapRef != nil {
					usedCM[pod.Namespace+"/"+env.ConfigMapRef.Name] = true
				}
			}
		}
	}

	for _, cm := range cmList.Items {
		result.Summary.TotalConfigMaps++

		keyCount := len(cm.Data) + len(cm.BinaryData)
		dataSize := 0
		for _, v := range cm.Data {
			dataSize += len(v)
		}
		for _, v := range cm.BinaryData {
			dataSize += len(v)
		}

		entry := ConfigMapCatEntry1974{
			Name: cm.Name, Namespace: cm.Namespace,
			KeyCount: keyCount, DataSize: dataSize / 1024,
			HasBinary: len(cm.BinaryData) > 0,
			Age:       fmt.Sprintf("%.0fd", time.Since(cm.CreationTimestamp.Time).Hours()/24),
		}

		result.Summary.TotalKeys += keyCount
		if entry.HasBinary {
			result.Summary.BinaryData++
		}
		if entry.DataSize > 512 {
			result.Summary.LargeConfigs++
		}

		key := cm.Namespace + "/" + cm.Name
		if !usedCM[key] {
			result.Summary.UnusedConfigs++
			result.Unused = append(result.Unused, entry)
		}

		result.ConfigMaps = append(result.ConfigMaps, entry)
	}

	if result.Summary.UnusedConfigs > 10 {
		score -= 5
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d ConfigMaps, %d keys, %d unused, %d large", result.Summary.TotalConfigMaps, result.Summary.TotalKeys, result.Summary.UnusedConfigs, result.Summary.LargeConfigs))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

func podAllEnvRefs(pod corev1.Pod) []corev1.EnvFromSource {
	var refs []corev1.EnvFromSource
	for _, c := range pod.Spec.Containers {
		refs = append(refs, c.EnvFrom...)
	}
	return refs
}

// ---------------------------------------------------------------
// 2. HPA Catalog
// ---------------------------------------------------------------

type HPACatResult1974 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         HPACatSummary1974 `json:"summary"`
	HPAs            []HPACatEntry1974 `json:"hpas"`
	Recommendations []string          `json:"recommendations"`
}

type HPACatSummary1974 struct {
	TotalHPAs       int `json:"totalHPAs"`
	WithCPUUtil     int `json:"withCPUUtilization"`
	WithMemUtil     int `json:"withMemoryUtilization"`
	WithMinReplicas int `json:"allHaveMinReplicas"`
	WithScaleDown   int `json:"withScaleDownPolicy"`
}

type HPACatEntry1974 struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	Target      string `json:"targetRef"`
	MinReplicas int32  `json:"minReplicas"`
	MaxReplicas int32  `json:"maxReplicas"`
	CPUTarget   int32  `json:"cpuTargetUtilization"`
	MemTarget   int32  `json:"memTargetUtilization,omitempty"`
}

func (s *Server) handleHPACatalog(w http.ResponseWriter, r *http.Request) {
	result := HPACatResult1974{ScannedAt: time.Now()}
	score := 100

	hpaList, _ := s.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(r.Context(), metav1.ListOptions{})

	for _, hpa := range hpaList.Items {
		result.Summary.TotalHPAs++

		entry := HPACatEntry1974{
			Name: hpa.Name, Namespace: hpa.Namespace,
			MinReplicas: 1, MaxReplicas: hpa.Spec.MaxReplicas,
		}
		if hpa.Spec.MinReplicas != nil {
			entry.MinReplicas = *hpa.Spec.MinReplicas
		}
		if hpa.Spec.ScaleTargetRef.Name != "" {
			entry.Target = hpa.Spec.ScaleTargetRef.Kind + "/" + hpa.Spec.ScaleTargetRef.Name
		}

		for _, metric := range hpa.Spec.Metrics {
			if metric.Type == "Resource" && metric.Resource != nil {
				if metric.Resource.Name == "cpu" && metric.Resource.Target.AverageUtilization != nil {
					entry.CPUTarget = *metric.Resource.Target.AverageUtilization
					result.Summary.WithCPUUtil++
				}
				if metric.Resource.Name == "memory" && metric.Resource.Target.AverageUtilization != nil {
					entry.MemTarget = *metric.Resource.Target.AverageUtilization
					result.Summary.WithMemUtil++
				}
			}
		}

		if entry.CPUTarget == 0 && entry.MemTarget == 0 {
			score -= 2
		}

		result.HPAs = append(result.HPAs, entry)
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d HPAs: %d with CPU metric, %d with memory metric", result.Summary.TotalHPAs, result.Summary.WithCPUUtil, result.Summary.WithMemUtil))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. PDB Catalog
// ---------------------------------------------------------------

type PDBCatResult1974 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         PDBCatSummary1974 `json:"summary"`
	PDBs            []PDBCatEntry1974 `json:"pdbs"`
	Unprotected     []string          `json:"unprotectedDeployments"`
	Recommendations []string          `json:"recommendations"`
}

type PDBCatSummary1974 struct {
	TotalPDBs      int `json:"totalPDBs"`
	HealthyPDBs    int `json:"healthyPDBs"`
	UnhealthyPDBs  int `json:"unhealthyPDBs"`
	MinAvailable   int `json:"minAvailablePDBs"`
	MaxUnavailable int `json:"maxUnavailablePDBs"`
}

type PDBCatEntry1974 struct {
	Name           string `json:"name"`
	Namespace      string `json:"namespace"`
	Selector       string `json:"selector"`
	MinAvailable   int    `json:"minAvailable"`
	MaxUnavailable int    `json:"maxUnavailable"`
	CurrentHealthy int    `json:"currentHealthy"`
	DesiredHealthy int    `json:"desiredHealthy"`
	IsHealthy      bool   `json:"isHealthy"`
}

func (s *Server) handlePDBCatalog(w http.ResponseWriter, r *http.Request) {
	result := PDBCatResult1974{ScannedAt: time.Now()}
	score := 100

	pdbList, _ := s.clientset.PolicyV1().PodDisruptionBudgets("").List(r.Context(), metav1.ListOptions{})
	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	// Build PDB namespace+selector map
	pdbCovered := make(map[string]bool)

	for _, pdb := range pdbList.Items {
		result.Summary.TotalPDBs++

		entry := PDBCatEntry1974{
			Name: pdb.Name, Namespace: pdb.Namespace,
			CurrentHealthy: int(pdb.Status.CurrentHealthy),
			DesiredHealthy: int(pdb.Status.DesiredHealthy),
		}

		// Selector
		selParts := []string{}
		for k, v := range pdb.Spec.Selector.MatchLabels {
			selParts = append(selParts, k+"="+v)
		}
		entry.Selector = strings.Join(selParts, ",")

		if pdb.Spec.MinAvailable != nil {
			entry.MinAvailable = int(pdb.Spec.MinAvailable.IntVal)
			result.Summary.MinAvailable++
		}
		if pdb.Spec.MaxUnavailable != nil {
			entry.MaxUnavailable = int(pdb.Spec.MaxUnavailable.IntVal)
			result.Summary.MaxUnavailable++
		}

		entry.IsHealthy = pdb.Status.CurrentHealthy >= pdb.Status.DesiredHealthy
		if entry.IsHealthy {
			result.Summary.HealthyPDBs++
		} else {
			result.Summary.UnhealthyPDBs++
			score -= 3
		}

		// Mark covered deployments
		for _, dep := range depList.Items {
			if dep.Namespace == pdb.Namespace {
				match := true
				for k, v := range pdb.Spec.Selector.MatchLabels {
					if dep.Spec.Selector.MatchLabels[k] != v {
						match = false
						break
					}
				}
				if match {
					pdbCovered[dep.Namespace+"/"+dep.Name] = true
				}
			}
		}

		result.PDBs = append(result.PDBs, entry)
	}

	// Find unprotected deployments
	unprotectedCount := 0
	for _, dep := range depList.Items {
		if dep.Spec.Replicas != nil && *dep.Spec.Replicas >= 2 {
			if !pdbCovered[dep.Namespace+"/"+dep.Name] {
				result.Unprotected = append(result.Unprotected, dep.Namespace+"/"+dep.Name)
				unprotectedCount++
			}
		}
	}

	if unprotectedCount > 5 {
		score -= 5
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d PDBs (%d healthy), %d unprotected multi-replica deployments", result.Summary.TotalPDBs, result.Summary.HealthyPDBs, unprotectedCount))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// suppress unused import
var _ policyv1.PodDisruptionBudget
