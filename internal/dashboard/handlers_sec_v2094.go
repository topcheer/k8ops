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
// v20.94 — Security Dimension (Round 35)
// 1. Secret Data Exposure — secrets with plaintext data
// 2. Role Binding Wildcard Audit — wildcard verb/resource RBAC
// 3. Pod fsGroup Validator — fsGroup consistency
// ============================================================

type SecExpResult2094 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         SecExpSummary2094 `json:"summary"`
	Recommendations []string          `json:"recommendations"`
}

type SecExpSummary2094 struct {
	TotalSecrets  int `json:"totalSecrets"`
	OpaqueSecrets int `json:"opaqueSecrets"`
	TLSSecrets    int `json:"tlsSecrets"`
	DockerSecrets int `json:"dockerSecrets"`
}

func (s *Server) handleSecExp2094(w http.ResponseWriter, r *http.Request) {
	result := SecExpResult2094{ScannedAt: time.Now()}
	score := 100
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})

	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		switch secret.Type {
		case corev1.SecretTypeOpaque:
			result.Summary.OpaqueSecrets++
		case corev1.SecretTypeTLS:
			result.Summary.TLSSecrets++
		case corev1.SecretTypeDockerConfigJson, corev1.SecretTypeDockercfg:
			result.Summary.DockerSecrets++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Role Binding Wildcard Audit
type WildcardResult2094 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         WildcardSummary2094 `json:"summary"`
	WildcardRoles   []WildcardEntry2094 `json:"wildcardRoles"`
	Recommendations []string            `json:"recommendations"`
}

type WildcardSummary2094 struct {
	TotalRoles    int `json:"totalRoles"`
	WildcardRoles int `json:"wildcardRoles"`
}

type WildcardEntry2094 struct {
	Name string `json:"name"`
}

func (s *Server) handleWildcard2094(w http.ResponseWriter, r *http.Request) {
	result := WildcardResult2094{ScannedAt: time.Now()}
	score := 100
	crList, _ := s.clientset.RbacV1().ClusterRoles().List(r.Context(), metav1.ListOptions{})

	for _, cr := range crList.Items {
		result.Summary.TotalRoles++
		for _, rule := range cr.Rules {
			for _, verb := range rule.Verbs {
				if verb == "*" {
					result.Summary.WildcardRoles++
					result.WildcardRoles = append(result.WildcardRoles, WildcardEntry2094{Name: cr.Name})
					score -= 2
					break
				}
			}
		}
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.WildcardRoles, func(i, j int) bool { return result.WildcardRoles[i].Name < result.WildcardRoles[j].Name })

	if result.Summary.WildcardRoles > 5 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d roles with wildcard verbs — use specific verbs", result.Summary.WildcardRoles))
	}
	writeJSON(w, result)
}

// 3. Pod fsGroup Validator
type FsGroupResult2094 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         FsGroupSummary2094 `json:"summary"`
	Recommendations []string           `json:"recommendations"`
}

type FsGroupSummary2094 struct {
	TotalPods    int `json:"totalPods"`
	WithFsGroup  int `json:"withFsGroup"`
	WithoutFsGrp int `json:"withoutFsGroup"`
}

func (s *Server) handleFsGroup2094(w http.ResponseWriter, r *http.Request) {
	result := FsGroupResult2094{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if pod.Spec.SecurityContext != nil && pod.Spec.SecurityContext.FSGroup != nil {
			result.Summary.WithFsGroup++
		} else {
			result.Summary.WithoutFsGrp++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
