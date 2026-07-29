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
// v20.67 — Product Dimension (Round 31)
// 1. Workload Density Profile — pods-per-namespace ratio analysis
// 2. Image Vintage Tracker — image age and freshness
// 3. Service Protocol Distribution — TCP/UDP/SCTP usage catalog
// ============================================================

type WkldDensResult2067 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         WkldDensSummary2067 `json:"summary"`
	DenseNS         []WkldDensEntry2067 `json:"denseNamespaces"`
	Recommendations []string            `json:"recommendations"`
}

type WkldDensSummary2067 struct {
	TotalNS   int `json:"totalNamespaces"`
	DenseNS   int `json:"denseNamespaces"`
	TotalPods int `json:"totalPods"`
}

type WkldDensEntry2067 struct {
	Namespace string `json:"namespace"`
	PodCount  int    `json:"podCount"`
}

func (s *Server) handleWkldDensProfile(w http.ResponseWriter, r *http.Request) {
	result := WkldDensResult2067{ScannedAt: time.Now()}
	score := 100

	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	podCountNS := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			podCountNS[pod.Namespace]++
		}
	}

	result.Summary.TotalNS = len(nsList.Items)
	for _, count := range podCountNS {
		result.Summary.TotalPods += count
	}

	systemNS := map[string]bool{"kube-system": true, "kube-public": true, "kube-node-lease": true, "k8ops-system": true}
	for _, ns := range nsList.Items {
		if systemNS[ns.Name] {
			continue
		}
		pods := podCountNS[ns.Name]
		if pods > 20 {
			result.Summary.DenseNS++
			result.DenseNS = append(result.DenseNS, WkldDensEntry2067{
				Namespace: ns.Name, PodCount: pods,
			})
			score -= 1
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.DenseNS, func(i, j int) bool { return result.DenseNS[i].PodCount > result.DenseNS[j].PodCount })

	if result.Summary.DenseNS > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d namespaces with >20 pods — review resource allocation", result.Summary.DenseNS))
	}
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Image Vintage Tracker
// ---------------------------------------------------------------

type ImgVintageResult2067 struct {
	ScannedAt       time.Time             `json:"scannedAt"`
	HealthScore     int                   `json:"healthScore"`
	Grade           string                `json:"grade"`
	Summary         ImgVintageSummary2067 `json:"summary"`
	OldImages       []ImgVintageEntry2067 `json:"oldImages"`
	Recommendations []string              `json:"recommendations"`
}

type ImgVintageSummary2067 struct {
	TotalImages int `json:"totalImages"`
	StaleImages int `json:"staleImages"`
}

type ImgVintageEntry2067 struct {
	Image string `json:"image"`
	Tag   string `json:"tag"`
}

func (s *Server) handleImgVintage(w http.ResponseWriter, r *http.Request) {
	result := ImgVintageResult2067{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	seenImg := make(map[string]bool)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			img := c.Image
			if seenImg[img] {
				continue
			}
			seenImg[img] = true
			result.Summary.TotalImages++

			// Check for version-pinned vs latest
			tag := extractTag2067(img)
			if tag == "latest" || tag == "main" || tag == "master" || tag == "" {
				result.Summary.StaleImages++
				result.OldImages = append(result.OldImages, ImgVintageEntry2067{Image: img, Tag: tag})
				score -= 2
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.StaleImages > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d images use mutable tags — pin to specific versions", result.Summary.StaleImages))
	}
	writeJSON(w, result)
}

func extractTag2067(image string) string {
	for i := len(image) - 1; i >= 0; i-- {
		if image[i] == '/' {
			break
		}
		if image[i] == ':' {
			return image[i+1:]
		}
	}
	return ""
}

// ---------------------------------------------------------------
// 3. Service Protocol Distribution
// ---------------------------------------------------------------

type SvcProtoResult2067 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         SvcProtoSummary2067 `json:"summary"`
	Protocols       []SvcProtoEntry2067 `json:"protocols"`
	Recommendations []string            `json:"recommendations"`
}

type SvcProtoSummary2067 struct {
	TotalServices int `json:"totalServices"`
	TCPCount      int `json:"tcpCount"`
	UDPCount      int `json:"udpCount"`
	SCTPCount     int `json:"sctpCount"`
}

type SvcProtoEntry2067 struct {
	Protocol string `json:"protocol"`
	Count    int    `json:"count"`
}

func (s *Server) handleSvcProtoDist(w http.ResponseWriter, r *http.Request) {
	result := SvcProtoResult2067{ScannedAt: time.Now()}
	score := 100

	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})

	protoCount := make(map[string]int)
	for _, svc := range svcList.Items {
		result.Summary.TotalServices++
		for _, p := range svc.Spec.Ports {
			proto := string(p.Protocol)
			protoCount[proto]++
		}
	}

	result.Summary.TCPCount = protoCount["TCP"]
	result.Summary.UDPCount = protoCount["UDP"]
	result.Summary.SCTPCount = protoCount["SCTP"]

	for proto, count := range protoCount {
		result.Protocols = append(result.Protocols, SvcProtoEntry2067{Protocol: proto, Count: count})
	}

	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	sort.Slice(result.Protocols, func(i, j int) bool { return result.Protocols[i].Count > result.Protocols[j].Count })

	writeJSON(w, result)
}
