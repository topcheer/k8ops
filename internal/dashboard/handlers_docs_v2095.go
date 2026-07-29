package dashboard

import (
	"net/http"
	"sort"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v20.95 — Documentation Dimension (Round 35)
// 1. PVC Binding Mode Catalog — Immediate vs WaitForFirstConsumer
// 2. Node Kernel Version Map — kernel version per node
// 3. Service Session Affinity Catalog — session affinity distribution
// ============================================================

type BindModeResult2095 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         BindModeSummary2095 `json:"summary"`
	Recommendations []string            `json:"recommendations"`
}

type BindModeSummary2095 struct {
	TotalSCs     int `json:"totalStorageClasses"`
	Immediate    int `json:"immediateBinding"`
	WaitConsumer int `json:"waitForFirstConsumer"`
}

func (s *Server) handleBindMode2095(w http.ResponseWriter, r *http.Request) {
	result := BindModeResult2095{ScannedAt: time.Now()}
	score := 100
	scList, _ := s.clientset.StorageV1().StorageClasses().List(r.Context(), metav1.ListOptions{})

	for _, sc := range scList.Items {
		result.Summary.TotalSCs++
		mode := "Immediate"
		if sc.VolumeBindingMode != nil {
			mode = string(*sc.VolumeBindingMode)
		}
		if mode == "Immediate" {
			result.Summary.Immediate++
		} else {
			result.Summary.WaitConsumer++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Node Kernel Version Map
type KernelResult2095 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         KernelSummary2095 `json:"summary"`
	Nodes           []KernelEntry2095 `json:"nodes"`
	Recommendations []string          `json:"recommendations"`
}

type KernelSummary2095 struct {
	TotalNodes    int `json:"totalNodes"`
	UniqueKernels int `json:"uniqueKernels"`
}

type KernelEntry2095 struct {
	Node          string `json:"node"`
	KernelVersion string `json:"kernelVersion"`
}

func (s *Server) handleKernel2095(w http.ResponseWriter, r *http.Request) {
	result := KernelResult2095{ScannedAt: time.Now()}
	score := 100
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})

	kernelSet := make(map[string]bool)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		kernel := node.Status.NodeInfo.KernelVersion
		kernelSet[kernel] = true
		result.Nodes = append(result.Nodes, KernelEntry2095{Node: node.Name, KernelVersion: kernel})
	}
	result.Summary.UniqueKernels = len(kernelSet)
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.Nodes, func(i, j int) bool { return result.Nodes[i].KernelVersion < result.Nodes[j].KernelVersion })
	writeJSON(w, result)
}

// 3. Service Session Affinity Catalog
type SessionAffResult2095 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         SessionAffSummary2095 `json:"summary"`
	Recommendations []string              `json:"recommendations"`
}

type SessionAffSummary2095 struct {
	TotalServices int `json:"totalServices"`
	WithAffinity  int `json:"withSessionAffinity"`
	NoneAffinity  int `json:"noneAffinity"`
}

func (s *Server) handleSessionAff2095(w http.ResponseWriter, r *http.Request) {
	result := SessionAffResult2095{ScannedAt: time.Now()}
	score := 100
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})

	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		if svc.Spec.SessionAffinity != "" && svc.Spec.SessionAffinity != "None" {
			result.Summary.WithAffinity++
		} else {
			result.Summary.NoneAffinity++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
