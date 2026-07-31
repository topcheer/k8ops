package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"strings"
	"time"
)

// v23.24 Documentation: Service Account Token Age, Node Feature Label Census, Pod Annotation Key Count
type SATokenAgeResult2324 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSAs    int            `json:"totalServiceAccounts"`
		ByAgeBucket map[string]int `json:"byAgeBucket"`
	} `json:"summary"`
}

func (s *Server) handleSATokenAge2324(w http.ResponseWriter, r *http.Request) {
	result := SATokenAgeResult2324{ScannedAt: time.Now()}
	result.Summary.ByAgeBucket = make(map[string]int)
	saList, _ := s.clientset.CoreV1().ServiceAccounts("").List(r.Context(), metav1.ListOptions{})
	now := time.Now()
	for _, sa := range saList.Items {
		result.Summary.TotalSAs++
		age := now.Sub(sa.CreationTimestamp.Time)
		var bucket string
		switch {
		case age < 7*24*time.Hour:
			bucket = "<7d"
		case age < 30*24*time.Hour:
			bucket = "7-30d"
		case age < 90*24*time.Hour:
			bucket = "30-90d"
		default:
			bucket = "90d+"
		}
		result.Summary.ByAgeBucket[bucket]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeFeatureLabelResult2324 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes        int `json:"totalNodes"`
		WithFeatureLabels int `json:"withFeatureLabels"`
		TotalFeatureKeys  int `json:"totalFeatureLabelKeys"`
	} `json:"summary"`
}

func (s *Server) handleNodeFeatureLabel2324(w http.ResponseWriter, r *http.Request) {
	result := NodeFeatureLabelResult2324{ScannedAt: time.Now()}
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	featureKeys := make(map[string]bool)
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		hasFeature := false
		for k := range node.Labels {
			if strings.HasPrefix(k, "feature.node.kubernetes.io/") {
				hasFeature = true
				featureKeys[k] = true
			}
		}
		if hasFeature {
			result.Summary.WithFeatureLabels++
		}
	}
	result.Summary.TotalFeatureKeys = len(featureKeys)
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodAnnotationResult2324 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods           int `json:"totalPods"`
		WithAnnotations     int `json:"withAnnotations"`
		TotalAnnotationKeys int `json:"totalAnnotationKeys"`
	} `json:"summary"`
}

func (s *Server) handlePodAnnotation2324(w http.ResponseWriter, r *http.Request) {
	result := PodAnnotationResult2324{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		if len(pod.Annotations) > 0 {
			result.Summary.WithAnnotations++
			result.Summary.TotalAnnotationKeys += len(pod.Annotations)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
