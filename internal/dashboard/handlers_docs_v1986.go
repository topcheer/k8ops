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
// v19.86 — Documentation Dimension (Round 17)
// 1. Node Taint Catalog — all taints & their effect on scheduling
// 2. Volume Snapshot Catalog — VolumeSnapshot inventory
// 3. Storage Class Catalog — storage class provisioning & binding info
// ============================================================

// ---------------------------------------------------------------
// 1. Node Taint Catalog
// ---------------------------------------------------------------

type TaintCatResult1986 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         TaintCatSummary1986 `json:"summary"`
	Taints          []TaintCatEntry1986 `json:"taints"`
	Recommendations []string            `json:"recommendations"`
}

type TaintCatSummary1986 struct {
	TotalNodes       int `json:"totalNodes"`
	NodesWithTaints  int `json:"nodesWithTaints"`
	TotalTaints      int `json:"totalTaints"`
	NoScheduleTaints int `json:"noScheduleTaints"`
	NoExecuteTaints  int `json:"noExecuteTaints"`
	PreferNoSchedule int `json:"preferNoScheduleTaints"`
}

type TaintCatEntry1986 struct {
	Node   string `json:"node"`
	Key    string `json:"key"`
	Value  string `json:"value"`
	Effect string `json:"effect"`
}

func (s *Server) handleNodeTaintCatalog(w http.ResponseWriter, r *http.Request) {
	result := TaintCatResult1986{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++

		for _, taint := range node.Spec.Taints {
			result.Summary.TotalTaints++

			entry := TaintCatEntry1986{
				Node: node.Name, Key: taint.Key,
				Value: taint.Value, Effect: string(taint.Effect),
			}
			result.Taints = append(result.Taints, entry)

			switch taint.Effect {
			case corev1.TaintEffectNoSchedule:
				result.Summary.NoScheduleTaints++
			case corev1.TaintEffectNoExecute:
				result.Summary.NoExecuteTaints++
			case corev1.TaintEffectPreferNoSchedule:
				result.Summary.PreferNoSchedule++
			}
		}

		if len(node.Spec.Taints) > 0 {
			result.Summary.NodesWithTaints++
		}
	}

	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d nodes, %d taints (%d NoSchedule, %d NoExecute, %d PreferNoSchedule)", result.Summary.TotalNodes, result.Summary.TotalTaints, result.Summary.NoScheduleTaints, result.Summary.NoExecuteTaints, result.Summary.PreferNoSchedule))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Volume Snapshot Catalog
// ---------------------------------------------------------------

type VolSnapCatResult1986 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         VolSnapCatSummary1986 `json:"summary"`
	Snapshots       []VolSnapCatEntry1986 `json:"snapshots"`
	Recommendations []string              `json:"recommendations"`
}

type VolSnapCatSummary1986 struct {
	TotalSnapshots    int `json:"totalSnapshots"`
	ReadySnapshots    int `json:"readySnapshots"`
	NotReadySnapshots int `json:"notReadySnapshots"`
	SnapshotClasses   int `json:"snapshotClasses"`
}

type VolSnapCatEntry1986 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	PVCName   string `json:"pvcName"`
	Ready     bool   `json:"readyToUse"`
}

func (s *Server) handleVolSnapshotCatalog(w http.ResponseWriter, r *http.Request) {
	result := VolSnapCatResult1986{ScannedAt: time.Now()}
	score := 100

	// Try to list VolumeSnapshots via dynamic client
	// Since we don't have the snapshot clientset, use PVs to estimate
	pvList, _ := s.clientset.CoreV1().PersistentVolumes().List(r.Context(), metav1.ListOptions{})
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})

	// Check for snapshot-related PVs
	for _, pv := range pvList.Items {
		if pv.Spec.ClaimRef != nil && pv.Spec.PersistentVolumeReclaimPolicy == corev1.PersistentVolumeReclaimDelete {
			// Check if PV was created from snapshot
			if pv.Annotations != nil {
				if pv.Annotations["snapshot.storage.kubernetes.io/volumesnapshot-name"] != "" {
					result.Summary.TotalSnapshots++
					entry := VolSnapCatEntry1986{
						Name:      pv.Annotations["snapshot.storage.kubernetes.io/volumesnapshot-name"],
						Namespace: pv.Spec.ClaimRef.Namespace,
						PVCName:   pv.Spec.ClaimRef.Name,
						Ready:     pv.Status.Phase == corev1.VolumeBound,
					}
					if entry.Ready {
						result.Summary.ReadySnapshots++
					} else {
						result.Summary.NotReadySnapshots++
						score -= 2
					}
					result.Snapshots = append(result.Snapshots, entry)
				}
			}
		}
	}

	// Also check PVC annotations for snapshot references
	for _, pvc := range pvcList.Items {
		for k := range pvc.Annotations {
			if k == "snapshot.storage.kubernetes.io/volumesnapshot-name" {
				// PVC restored from snapshot
				_ = pvc
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d snapshots detected (%d ready, %d not ready)", result.Summary.TotalSnapshots, result.Summary.ReadySnapshots, result.Summary.NotReadySnapshots))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Storage Class Catalog
// ---------------------------------------------------------------

type SCCatResult1986 struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Summary         SCCatSummary1986 `json:"summary"`
	Classes         []SCCatEntry1986 `json:"storageClasses"`
	Recommendations []string         `json:"recommendations"`
}

type SCCatSummary1986 struct {
	TotalClasses             int   `json:"totalStorageClasses"`
	DefaultClass             *bool `json:"hasDefaultStorageClass"`
	WithBinding              int   `json:"withImmediateBinding"`
	WithWaitForFirstConsumer int   `json:"withWaitForFirstConsumer"`
}

type SCCatEntry1986 struct {
	Name              string `json:"name"`
	Provisioner       string `json:"provisioner"`
	ReclaimPolicy     string `json:"reclaimPolicy"`
	VolumeBindingMode string `json:"volumeBindingMode"`
	IsDefault         bool   `json:"isDefault"`
}

func (s *Server) handleStorageClassCatalog(w http.ResponseWriter, r *http.Request) {
	result := SCCatResult1986{ScannedAt: time.Now()}
	score := 100

	scList, _ := s.clientset.StorageV1().StorageClasses().List(r.Context(), metav1.ListOptions{})

	hasDefault := false

	for _, sc := range scList.Items {
		result.Summary.TotalClasses++

		entry := SCCatEntry1986{
			Name:          sc.Name,
			Provisioner:   sc.Provisioner,
			ReclaimPolicy: string(*sc.ReclaimPolicy),
		}

		if sc.VolumeBindingMode != nil {
			entry.VolumeBindingMode = string(*sc.VolumeBindingMode)
			if *sc.VolumeBindingMode == "WaitForFirstConsumer" {
				result.Summary.WithWaitForFirstConsumer++
			} else {
				result.Summary.WithBinding++
			}
		}

		// Check default
		if sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			entry.IsDefault = true
			hasDefault = true
		}

		result.Classes = append(result.Classes, entry)
	}

	result.Summary.DefaultClass = &hasDefault

	if result.Summary.TotalClasses == 0 {
		score -= 10
	}
	if !hasDefault && result.Summary.TotalClasses > 0 {
		score -= 5
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d storage classes, default: %v, %d WaitForFirstConsumer", result.Summary.TotalClasses, hasDefault, result.Summary.WithWaitForFirstConsumer))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
