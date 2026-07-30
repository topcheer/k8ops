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
// v21.48 — Security Dimension (Round 44)
// 1. Pod ReadOnlyRootFilesystem Audit
// 2. ClusterRoleBinding Aggregation Rule Count
// 3. ServiceAccount Annotated Secret Validator
// ============================================================

type ROFSResult2148 struct {
	ScannedAt       time.Time       `json:"scannedAt"`
	HealthScore     int             `json:"healthScore"`
	Grade           string          `json:"grade"`
	Summary         ROFSSummary2148 `json:"summary"`
	WritableFS      []ROFSEntry2148 `json:"writableRootFS"`
	Recommendations []string        `json:"recommendations"`
}

type ROFSSummary2148 struct {
	TotalContainers int `json:"totalContainers"`
	ReadOnlyRoot    int `json:"readOnlyRootFilesystem"`
}

type ROFSEntry2148 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleROFS2148(w http.ResponseWriter, r *http.Request) {
	result := ROFSResult2148{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.SecurityContext != nil && c.SecurityContext.ReadOnlyRootFilesystem != nil && *c.SecurityContext.ReadOnlyRootFilesystem {
				result.Summary.ReadOnlyRoot++
			} else {
				result.WritableFS = append(result.WritableFS, ROFSEntry2148{Pod: pod.Name, Namespace: pod.Namespace})
			}
		}
	}
	sort.Slice(result.WritableFS, func(i, j int) bool { return result.WritableFS[i].Namespace < result.WritableFS[j].Namespace })
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.ReadOnlyRoot < result.Summary.TotalContainers/2 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d/%d containers with writable root filesystem", len(result.WritableFS), result.Summary.TotalContainers))
	}
	writeJSON(w, result)
}

// 2. CRB Aggregation Count
type CRBAggCountResult2148 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         CRBAggCountSummary2148 `json:"summary"`
	Recommendations []string               `json:"recommendations"`
}

type CRBAggCountSummary2148 struct {
	TotalCRBs  int `json:"totalClusterRoleBindings"`
	Aggregated int `json:"aggregated"`
}

func (s *Server) handleCRBAggCount2148(w http.ResponseWriter, r *http.Request) {
	result := CRBAggCountResult2148{ScannedAt: time.Now()}
	score := 100
	crList, _ := s.clientset.RbacV1().ClusterRoles().List(r.Context(), metav1.ListOptions{})

	for _, cr := range crList.Items {
		result.Summary.TotalCRBs++
		if cr.AggregationRule != nil {
			result.Summary.Aggregated++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. SA Annotated Secret
type SAAnnotResult2148 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         SAAnnotSummary2148 `json:"summary"`
	Recommendations []string           `json:"recommendations"`
}

type SAAnnotSummary2148 struct {
	TotalSAs        int `json:"totalServiceAccounts"`
	WithAnnotSecret int `json:"withAnnotatedSecret"`
}

func (s *Server) handleSAAnnot2148(w http.ResponseWriter, r *http.Request) {
	result := SAAnnotResult2148{ScannedAt: time.Now()}
	score := 100
	saList, _ := s.clientset.CoreV1().ServiceAccounts("").List(r.Context(), metav1.ListOptions{})

	for _, sa := range saList.Items {
		result.Summary.TotalSAs++
		if sa.Annotations != nil && sa.Annotations["kubernetes.io/service-account.name"] != "" {
			result.Summary.WithAnnotSecret++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
