package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v20.22 — Documentation Dimension (Round 23)
// 1. Storage Class Binding Mode — volume binding mode catalog
// 2. CRD Version Catalog — CRD available versions inventory
// 3. PriorityLevel Config Catalog — API priority and fairness catalog
// ============================================================

// ---------------------------------------------------------------
// 1. Storage Class Binding Mode
// ---------------------------------------------------------------

type SCBindModeResult2022 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         SCBindModeSummary2022 `json:"summary"`
	Classes         []SCBindModeEntry2022 `json:"classes"`
	Recommendations []string              `json:"recommendations"`
}

type SCBindModeSummary2022 struct {
	TotalClasses     int  `json:"totalStorageClasses"`
	ImmediateBinding int  `json:"immediateBinding"`
	WaitForConsumer  int  `json:"waitForConsumer"`
	HasDefault       bool `json:"hasDefaultClass"`
}

type SCBindModeEntry2022 struct {
	Name        string `json:"name"`
	Provisioner string `json:"provisioner"`
	BindingMode string `json:"volumeBindingMode"`
	IsDefault   bool   `json:"isDefault"`
}

func (s *Server) handleSCBindMode(w http.ResponseWriter, r *http.Request) {
	result := SCBindModeResult2022{ScannedAt: time.Now()}
	score := 100

	scList, _ := s.clientset.StorageV1().StorageClasses().List(r.Context(), metav1.ListOptions{})

	for _, sc := range scList.Items {
		result.Summary.TotalClasses++

		entry := SCBindModeEntry2022{
			Name: sc.Name, Provisioner: sc.Provisioner,
		}

		if sc.VolumeBindingMode != nil {
			entry.BindingMode = string(*sc.VolumeBindingMode)
			if entry.BindingMode == "WaitForFirstConsumer" {
				result.Summary.WaitForConsumer++
			} else {
				result.Summary.ImmediateBinding++
			}
		} else {
			entry.BindingMode = "Immediate"
			result.Summary.ImmediateBinding++
		}

		if sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true" {
			entry.IsDefault = true
			result.Summary.HasDefault = true
		}

		result.Classes = append(result.Classes, entry)
	}

	if !result.Summary.HasDefault && result.Summary.TotalClasses > 0 {
		score -= 5
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d storage classes: %d immediate, %d wait-consumer, default: %v", result.Summary.TotalClasses, result.Summary.ImmediateBinding, result.Summary.WaitForConsumer, result.Summary.HasDefault))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. CRD Version Catalog
// ---------------------------------------------------------------

type CRDVerResult2022 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         CRDVerSummary2022 `json:"summary"`
	CRDs            []CRDVerEntry2022 `json:"crds"`
	Recommendations []string          `json:"recommendations"`
}

type CRDVerSummary2022 struct {
	TotalCRDs     int `json:"totalCRDs"`
	WithMultiple  int `json:"withMultipleVersions"`
	TotalVersions int `json:"totalVersions"`
}

type CRDVerEntry2022 struct {
	Name     string   `json:"name"`
	Group    string   `json:"group"`
	Versions []string `json:"versions"`
}

func (s *Server) handleCRDVerCat(w http.ResponseWriter, r *http.Request) {
	result := CRDVerResult2022{ScannedAt: time.Now()}
	score := 100

	// Use discovery to list API groups (proxy for CRDs)
	groups, err := s.clientset.Discovery().ServerGroups()
	if err != nil {
		writeJSON(w, result)
		return
	}

	for _, group := range groups.Groups {
		if group.Name == "" {
			continue // skip core API
		}
		result.Summary.TotalCRDs++
		var versions []string
		for _, v := range group.Versions {
			versions = append(versions, v.Version)
		}
		if len(versions) > 1 {
			result.Summary.WithMultiple++
		}
		result.Summary.TotalVersions += len(versions)

		result.CRDs = append(result.CRDs, CRDVerEntry2022{
			Name: group.Name, Group: group.Name, Versions: versions,
		})
	}

	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d CRDs, %d with multiple versions, %d total versions", result.Summary.TotalCRDs, result.Summary.WithMultiple, result.Summary.TotalVersions))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. PriorityLevel Config Catalog
// ---------------------------------------------------------------

type PLConfigResult2022 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         PLConfigSummary2022 `json:"summary"`
	Levels          []PLConfigEntry2022 `json:"priorityLevels"`
	Recommendations []string            `json:"recommendations"`
}

type PLConfigSummary2022 struct {
	TotalLevels  int `json:"totalPriorityLevels"`
	Exempt       int `json:"exemptLevels"`
	LimitedQueue int `json:"limitedLevels"`
}

type PLConfigEntry2022 struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

func (s *Server) handlePLConfigCat(w http.ResponseWriter, r *http.Request) {
	result := PLConfigResult2022{ScannedAt: time.Now()}
	score := 100

	plList, err := s.clientset.FlowcontrolV1().PriorityLevelConfigurations().List(r.Context(), metav1.ListOptions{})
	if err != nil {
		result.Recommendations = append(result.Recommendations, "PriorityLevel API not available")
		writeJSON(w, result)
		return
	}

	for _, pl := range plList.Items {
		result.Summary.TotalLevels++

		plType := string(pl.Spec.Type)
		entry := PLConfigEntry2022{Name: pl.Name, Type: plType}

		if plType == "Exempt" {
			result.Summary.Exempt++
		} else {
			result.Summary.LimitedQueue++
		}

		result.Levels = append(result.Levels, entry)
	}

	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d priority levels: %d exempt, %d limited", result.Summary.TotalLevels, result.Summary.Exempt, result.Summary.LimitedQueue))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
