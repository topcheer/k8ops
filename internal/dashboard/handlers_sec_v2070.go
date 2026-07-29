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
// v20.70 — Security Dimension (Round 31)
// 1. Secret Type Audit — secret type distribution
// 2. Service Account Privilege — SA binding privilege analysis
// 3. Pod RunAsUser Validator — runAsUser=0 detection
// ============================================================

type SecTypeResult2070 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         SecTypeSummary2070 `json:"summary"`
	Distribution    []SecTypeEntry2070 `json:"distribution"`
	Recommendations []string           `json:"recommendations"`
}

type SecTypeSummary2070 struct {
	TotalSecrets int `json:"totalSecrets"`
}

type SecTypeEntry2070 struct {
	Type  string `json:"type"`
	Count int    `json:"count"`
}

func (s *Server) handleSecTypeAudit2070(w http.ResponseWriter, r *http.Request) {
	result := SecTypeResult2070{ScannedAt: time.Now()}
	score := 100

	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})

	typeCount := make(map[string]int)
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		typeCount[string(secret.Type)]++
	}

	for t, c := range typeCount {
		result.Distribution = append(result.Distribution, SecTypeEntry2070{Type: t, Count: c})
	}

	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.Distribution, func(i, j int) bool { return result.Distribution[i].Count > result.Distribution[j].Count })
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Service Account Privilege
// ---------------------------------------------------------------

type SAPrivResult2070 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         SAPrivSummary2070 `json:"summary"`
	PrivilegedSAs   []SAPrivEntry2070 `json:"privilegedSAs"`
	Recommendations []string          `json:"recommendations"`
}

type SAPrivSummary2070 struct {
	TotalSAs   int `json:"totalServiceAccounts"`
	Privileged int `json:"privilegedSAs"`
}

type SAPrivEntry2070 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleSAPrivAnalysis(w http.ResponseWriter, r *http.Request) {
	result := SAPrivResult2070{ScannedAt: time.Now()}
	score := 100

	saList, _ := s.clientset.CoreV1().ServiceAccounts("").List(r.Context(), metav1.ListOptions{})
	crbList, _ := s.clientset.RbacV1().ClusterRoleBindings().List(r.Context(), metav1.ListOptions{})

	privilegedSAs := make(map[string]bool)
	for _, crb := range crbList.Items {
		if crb.RoleRef.Name == "cluster-admin" {
			for _, subj := range crb.Subjects {
				if subj.Kind == "ServiceAccount" {
					privilegedSAs[subj.Namespace+"/"+subj.Name] = true
				}
			}
		}
	}

	for _, sa := range saList.Items {
		result.Summary.TotalSAs++
		key := sa.Namespace + "/" + sa.Name
		if privilegedSAs[key] {
			result.Summary.Privileged++
			result.PrivilegedSAs = append(result.PrivilegedSAs, SAPrivEntry2070{
				Name: sa.Name, Namespace: sa.Namespace,
			})
			score -= 5
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.PrivilegedSAs, func(i, j int) bool { return result.PrivilegedSAs[i].Namespace < result.PrivilegedSAs[j].Namespace })

	if result.Summary.Privileged > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d SAs with cluster-admin — use least privilege", result.Summary.Privileged))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Pod RunAsUser Validator
// ---------------------------------------------------------------

type RunAsUserResult2070 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         RunAsUserSummary2070 `json:"summary"`
	RootPods        []RunAsUserEntry2070 `json:"rootPods"`
	Recommendations []string             `json:"recommendations"`
}

type RunAsUserSummary2070 struct {
	TotalContainers int `json:"totalContainers"`
	RunningAsRoot   int `json:"runningAsRoot"`
}

type RunAsUserEntry2070 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleRunAsUserValidator(w http.ResponseWriter, r *http.Request) {
	result := RunAsUserResult2070{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}

		podRunAsRoot := false
		if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.RunAsUser != nil && *pod.Spec.SecurityContext.RunAsUser == 0 {
			podRunAsRoot = true
		}

		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			isRoot := podRunAsRoot
			if c.SecurityContext != nil && c.SecurityContext.RunAsUser != nil {
				isRoot = *c.SecurityContext.RunAsUser == 0
			}
			if isRoot {
				result.Summary.RunningAsRoot++
				result.RootPods = append(result.RootPods, RunAsUserEntry2070{
					Pod: pod.Name, Namespace: pod.Namespace,
				})
				score -= 1
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.RootPods, func(i, j int) bool { return result.RootPods[i].Namespace < result.RootPods[j].Namespace })

	if result.Summary.RunningAsRoot > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d containers running as root — set runAsUser>0", result.Summary.RunningAsRoot))
	}
	writeJSON(w, result)
}
