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
// v20.55 — Product Dimension (Round 29)
// 1. Image Registry Diversity — registry source distribution audit
// 2. Ingress Backend Health — ingress backend service availability
// 3. Volume Claim Lifecycle — PVC age and binding mode analysis
// ============================================================

// ---------------------------------------------------------------
// 1. Image Registry Diversity
// ---------------------------------------------------------------

type RegDivResult2055 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         RegDivSummary2055 `json:"summary"`
	Registries      []RegDivEntry2055 `json:"registries"`
	Recommendations []string          `json:"recommendations"`
}

type RegDivSummary2055 struct {
	TotalImages       int `json:"totalImages"`
	UniqueRegistries  int `json:"uniqueRegistries"`
	UnknownRegistries int `json:"unknownRegistries"`
}

type RegDivEntry2055 struct {
	Registry string `json:"registry"`
	Count    int    `json:"imageCount"`
}

func (s *Server) handleRegDiversity2055(w http.ResponseWriter, r *http.Request) {
	result := RegDivResult2055{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	regCount := make(map[string]int)
	totalImages := make(map[string]bool)

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			totalImages[c.Image] = true
			registry := extractRegistry2055(c.Image)
			regCount[registry]++
		}
	}

	result.Summary.TotalImages = len(totalImages)
	result.Summary.UniqueRegistries = len(regCount)

	for reg, count := range regCount {
		if reg == "docker.io" || reg == "unknown" {
			result.Summary.UnknownRegistries++
		}
		result.Registries = append(result.Registries, RegDivEntry2055{
			Registry: reg, Count: count,
		})
	}

	if result.Summary.UnknownRegistries > 0 {
		score -= 5
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.Registries, func(i, j int) bool {
		return result.Registries[i].Count > result.Registries[j].Count
	})

	if result.Summary.UniqueRegistries > 5 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d unique registries — consider consolidating for security policies", result.Summary.UniqueRegistries))
	}

	writeJSON(w, result)
}

func extractRegistry2055(image string) string {
	if strings.Contains(image, "/") {
		parts := strings.SplitN(image, "/", 2)
		first := parts[0]
		if strings.Contains(first, ".") || strings.Contains(first, ":") {
			return first
		}
		return "docker.io"
	}
	return "docker.io"
}

// ---------------------------------------------------------------
// 2. Ingress Backend Health
// ---------------------------------------------------------------

type IngBackendResult2055 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         IngBackendSummary2055 `json:"summary"`
	DeadBackends    []IngBackendEntry2055 `json:"deadBackends"`
	Recommendations []string              `json:"recommendations"`
}

type IngBackendSummary2055 struct {
	TotalIngresses  int `json:"totalIngresses"`
	HealthyBackends int `json:"healthyBackends"`
	DeadBackends    int `json:"deadBackends"`
}

type IngBackendEntry2055 struct {
	Ingress   string `json:"ingress"`
	Namespace string `json:"namespace"`
	Backend   string `json:"backend"`
}

func (s *Server) handleIngBackendHealth(w http.ResponseWriter, r *http.Request) {
	result := IngBackendResult2055{ScannedAt: time.Now()}
	score := 100

	ingList, _ := s.clientset.NetworkingV1().Ingresses("").List(r.Context(), metav1.ListOptions{})
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})

	svcSet := make(map[string]bool)
	for _, svc := range svcList.Items {
		svcSet[svc.Namespace+"/"+svc.Name] = true
	}

	for _, ing := range ingList.Items {
		result.Summary.TotalIngresses++

		for _, rule := range ing.Spec.Rules {
			if rule.HTTP == nil {
				continue
			}
			for _, path := range rule.HTTP.Paths {
				if path.Backend.Service == nil {
					continue
				}
				backendName := path.Backend.Service.Name
				key := ing.Namespace + "/" + backendName

				if svcSet[key] {
					result.Summary.HealthyBackends++
				} else {
					result.Summary.DeadBackends++
					result.DeadBackends = append(result.DeadBackends, IngBackendEntry2055{
						Ingress: ing.Name, Namespace: ing.Namespace, Backend: backendName,
					})
					score -= 5
				}
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.DeadBackends, func(i, j int) bool {
		return result.DeadBackends[i].Namespace < result.DeadBackends[j].Namespace
	})

	if result.Summary.DeadBackends > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d ingress backends reference missing services — fix routing", result.Summary.DeadBackends))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Volume Claim Lifecycle
// ---------------------------------------------------------------

type PVCLifecycleResult2055 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         PVCLifecycleSummary2055 `json:"summary"`
	OldPVCs         []PVCLifecycleEntry2055 `json:"oldPVCs"`
	Recommendations []string                `json:"recommendations"`
}

type PVCLifecycleSummary2055 struct {
	TotalPVCs   int `json:"totalPVCs"`
	BoundPVCs   int `json:"boundPVCs"`
	PendingPVCs int `json:"pendingPVCs"`
	OldPVCs     int `json:"oldPVCs"`
}

type PVCLifecycleEntry2055 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	AgeDays   int    `json:"ageDays"`
	Phase     string `json:"phase"`
}

func (s *Server) handlePVCLifecycle2055(w http.ResponseWriter, r *http.Request) {
	result := PVCLifecycleResult2055{ScannedAt: time.Now()}
	score := 100

	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})

	now := time.Now()
	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++

		phase := string(pvc.Status.Phase)
		if phase == "Bound" {
			result.Summary.BoundPVCs++
		} else if phase == "Pending" {
			result.Summary.PendingPVCs++
			score -= 3
		}

		ageDays := int(now.Sub(pvc.CreationTimestamp.Time).Hours() / 24)
		if ageDays > 180 {
			result.Summary.OldPVCs++
			result.OldPVCs = append(result.OldPVCs, PVCLifecycleEntry2055{
				Name: pvc.Name, Namespace: pvc.Namespace,
				AgeDays: ageDays, Phase: phase,
			})
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.OldPVCs, func(i, j int) bool {
		return result.OldPVCs[i].AgeDays > result.OldPVCs[j].AgeDays
	})

	if result.Summary.PendingPVCs > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d PVCs pending — check storage provisioner", result.Summary.PendingPVCs))
	}

	writeJSON(w, result)
}
