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
// v20.58 — Security Dimension (Round 29)
// 1. Secret Age Tracker — old secrets lifecycle
// 2. RBAC Role Binding Audit — excessive permissions
// 3. Pod Escalation Surface — containers with privileged flags
// ============================================================

type SecretAgeResult2058 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         SecretAgeSummary2058 `json:"summary"`
	OldSecrets      []SecretAgeEntry2058 `json:"oldSecrets"`
	Recommendations []string             `json:"recommendations"`
}

type SecretAgeSummary2058 struct {
	TotalSecrets int `json:"totalSecrets"`
	OldSecrets   int `json:"oldSecrets"`
}

type SecretAgeEntry2058 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	AgeDays   int    `json:"ageDays"`
	Type      string `json:"type"`
}

func (s *Server) handleSecretAgeTracker(w http.ResponseWriter, r *http.Request) {
	result := SecretAgeResult2058{ScannedAt: time.Now()}
	score := 100

	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})

	now := time.Now()
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		ageDays := int(now.Sub(secret.CreationTimestamp.Time).Hours() / 24)
		if ageDays > 365 {
			result.Summary.OldSecrets++
			result.OldSecrets = append(result.OldSecrets, SecretAgeEntry2058{
				Name: secret.Name, Namespace: secret.Namespace,
				AgeDays: ageDays, Type: string(secret.Type),
			})
			score -= 1
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.OldSecrets, func(i, j int) bool { return result.OldSecrets[i].AgeDays > result.OldSecrets[j].AgeDays })

	if result.Summary.OldSecrets > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d secrets older than 1 year — review and rotate", result.Summary.OldSecrets))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. RBAC Role Binding Audit
// ---------------------------------------------------------------

type RBACBindResult2058 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         RBACBindSummary2058 `json:"summary"`
	Excessive       []RBACBindEntry2058 `json:"excessiveBindings"`
	Recommendations []string            `json:"recommendations"`
}

type RBACBindSummary2058 struct {
	TotalRoleBindings int `json:"totalRoleBindings"`
	ExcessiveBindings int `json:"excessiveBindings"`
}

type RBACBindEntry2058 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	RoleRef   string `json:"roleRef"`
}

func (s *Server) handleRBACBindAudit(w http.ResponseWriter, r *http.Request) {
	result := RBACBindResult2058{ScannedAt: time.Now()}
	score := 100

	rbList, _ := s.clientset.RbacV1().RoleBindings("").List(r.Context(), metav1.ListOptions{})

	for _, rb := range rbList.Items {
		result.Summary.TotalRoleBindings++

		// Flag bindings to cluster-admin or admin roles
		roleName := rb.RoleRef.Name
		if roleName == "cluster-admin" || roleName == "admin" {
			result.Summary.ExcessiveBindings++
			result.Excessive = append(result.Excessive, RBACBindEntry2058{
				Name: rb.Name, Namespace: rb.Namespace, RoleRef: roleName,
			})
			score -= 3
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.Excessive, func(i, j int) bool { return result.Excessive[i].Namespace < result.Excessive[j].Namespace })

	if result.Summary.ExcessiveBindings > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d role bindings to admin-level roles — use least privilege", result.Summary.ExcessiveBindings))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Pod Escalation Surface
// ---------------------------------------------------------------

type EscSurfaceResult2058 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         EscSurfaceSummary2058 `json:"summary"`
	AtRiskPods      []EscSurfaceEntry2058 `json:"atRiskPods"`
	Recommendations []string              `json:"recommendations"`
}

type EscSurfaceSummary2058 struct {
	TotalPods  int `json:"totalPods"`
	AtRiskPods int `json:"atRiskPods"`
}

type EscSurfaceEntry2058 struct {
	Pod       string `json:"pod"`
	Namespace string `json:"namespace"`
	Surface   string `json:"escalationSurface"`
}

func (s *Server) handleEscSurface(w http.ResponseWriter, r *http.Request) {
	result := EscSurfaceResult2058{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++

		surfaces := []string{}
		for _, c := range pod.Spec.Containers {
			if c.SecurityContext != nil {
				if c.SecurityContext.Privileged != nil && *c.SecurityContext.Privileged {
					surfaces = append(surfaces, "privileged:"+c.Name)
				}
				if c.SecurityContext.RunAsUser != nil && *c.SecurityContext.RunAsUser == 0 {
					surfaces = append(surfaces, "runAsRoot:"+c.Name)
				}
			}
		}
		if pod.Spec.HostPID {
			surfaces = append(surfaces, "hostPID")
		}
		if pod.Spec.HostNetwork {
			surfaces = append(surfaces, "hostNetwork")
		}
		if pod.Spec.HostIPC {
			surfaces = append(surfaces, "hostIPC")
		}

		if len(surfaces) > 0 {
			result.Summary.AtRiskPods++
			result.AtRiskPods = append(result.AtRiskPods, EscSurfaceEntry2058{
				Pod: pod.Name, Namespace: pod.Namespace,
				Surface: fmt.Sprintf("%v", surfaces),
			})
			score -= 3
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.AtRiskPods, func(i, j int) bool { return result.AtRiskPods[i].Namespace < result.AtRiskPods[j].Namespace })

	if result.Summary.AtRiskPods > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d pods have escalation surfaces — reduce privileged access", result.Summary.AtRiskPods))
	}
	writeJSON(w, result)
}
