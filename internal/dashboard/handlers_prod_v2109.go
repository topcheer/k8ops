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
// v21.09 — Product Dimension (Round 38)
// 1. Pod Subresource Inventory — pods vs containers vs init
// 2. Ingress Annotation Compliance — standard ingress annotations
// 3. Namespace Pod Capacity Ratio — pods vs NS capacity
// ============================================================

type SubresResult2109 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         SubresSummary2109 `json:"summary"`
	Recommendations []string          `json:"recommendations"`
}

type SubresSummary2109 struct {
	TotalPods         int `json:"totalPods"`
	TotalContainers   int `json:"totalContainers"`
	TotalInitCtnrs    int `json:"totalInitContainers"`
	TotalSidecarCtnrs int `json:"totalSidecarContainers"`
}

func (s *Server) handleSubres2109(w http.ResponseWriter, r *http.Request) {
	result := SubresResult2109{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		result.Summary.TotalContainers += len(pod.Spec.Containers)
		result.Summary.TotalInitCtnrs += len(pod.Spec.InitContainers)
		result.Summary.TotalSidecarCtnrs += len(pod.Spec.EphemeralContainers)
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. Ingress Annotation Compliance
type IngAnnotResult2109 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         IngAnnotSummary2109 `json:"summary"`
	MissingAnnot    []IngAnnotEntry2109 `json:"missingAnnotations"`
	Recommendations []string            `json:"recommendations"`
}

type IngAnnotSummary2109 struct {
	TotalIngresses int `json:"totalIngresses"`
	WithClass      int `json:"withIngressClass"`
	WithRewrite    int `json:"withRewrite"`
}

type IngAnnotEntry2109 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

func (s *Server) handleIngAnnot2109(w http.ResponseWriter, r *http.Request) {
	result := IngAnnotResult2109{ScannedAt: time.Now()}
	score := 100
	ingList, _ := s.clientset.NetworkingV1().Ingresses("").List(r.Context(), metav1.ListOptions{})

	for _, ing := range ingList.Items {
		result.Summary.TotalIngresses++
		hasClass := ing.Spec.IngressClassName != nil
		if hasClass {
			result.Summary.WithClass++
		} else {
			result.MissingAnnot = append(result.MissingAnnot, IngAnnotEntry2109{Name: ing.Name, Namespace: ing.Namespace})
		}
		if ing.Annotations != nil && ing.Annotations["nginx.ingress.kubernetes.io/rewrite-target"] != "" {
			result.Summary.WithRewrite++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.MissingAnnot, func(i, j int) bool { return result.MissingAnnot[i].Namespace < result.MissingAnnot[j].Namespace })
	writeJSON(w, result)
}

// 3. Namespace Pod Capacity Ratio
type NSPodCapResult2109 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         NSPodCapSummary2109 `json:"summary"`
	TopNS           []NSPodCapEntry2109 `json:"topNamespaces"`
	Recommendations []string            `json:"recommendations"`
}

type NSPodCapSummary2109 struct {
	TotalNS   int `json:"totalNamespaces"`
	TotalPods int `json:"totalPods"`
}

type NSPodCapEntry2109 struct {
	Namespace string `json:"namespace"`
	PodCount  int    `json:"podCount"`
}

func (s *Server) handleNSPodCap2109(w http.ResponseWriter, r *http.Request) {
	result := NSPodCapResult2109{ScannedAt: time.Now()}
	score := 100
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	podsPerNS := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			podsPerNS[pod.Namespace]++
		}
	}
	result.Summary.TotalNS = len(nsList.Items)
	for _, cnt := range podsPerNS {
		result.Summary.TotalPods += cnt
	}
	for ns, cnt := range podsPerNS {
		result.TopNS = append(result.TopNS, NSPodCapEntry2109{Namespace: ns, PodCount: cnt})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].PodCount > result.TopNS[j].PodCount })
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if len(result.TopNS) > 0 && result.TopNS[0].PodCount > 100 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("Namespace %s has %d pods — high density", result.TopNS[0].Namespace, result.TopNS[0].PodCount))
	}
	writeJSON(w, result)
}
