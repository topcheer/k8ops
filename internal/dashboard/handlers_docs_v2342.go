package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"time"
)

// v23.42 Documentation: Node Region Label, Pod Controller Owner Kind, Secret Creation Order
type NodeRegionResult2342 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNodes int            `json:"totalNodes"`
		ByRegion   map[string]int `json:"byRegion"`
	} `json:"summary"`
}

func (s *Server) handleNodeRegion2342(w http.ResponseWriter, r *http.Request) {
	result := NodeRegionResult2342{ScannedAt: time.Now()}
	result.Summary.ByRegion = make(map[string]int)
	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		region := node.Labels[corev1.LabelTopologyRegion]
		if region == "" {
			region = "<unknown>"
		}
		result.Summary.ByRegion[region]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PodOwnerKindResult2342 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int            `json:"totalPods"`
		ByKind    map[string]int `json:"byOwnerKind"`
	} `json:"summary"`
}

func (s *Server) handlePodOwnerKind2342(w http.ResponseWriter, r *http.Request) {
	result := PodOwnerKindResult2342{ScannedAt: time.Now()}
	result.Summary.ByKind = make(map[string]int)
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		kind := "standalone"
		for _, ref := range pod.OwnerReferences {
			if ref.Controller != nil && *ref.Controller {
				kind = ref.Kind
				break
			}
		}
		result.Summary.ByKind[kind]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type SecretCreationOrderResult2342 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets int            `json:"totalSecrets"`
		ByAgeBucket  map[string]int `json:"byAgeBucket"`
	} `json:"summary"`
}

func (s *Server) handleSecretCreationOrder2342(w http.ResponseWriter, r *http.Request) {
	result := SecretCreationOrderResult2342{ScannedAt: time.Now()}
	result.Summary.ByAgeBucket = make(map[string]int)
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	now := time.Now()
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		age := now.Sub(secret.CreationTimestamp.Time)
		var b string
		if age < 24*time.Hour {
			b = "<1d"
		} else if age < 30*24*time.Hour {
			b = "1-30d"
		} else if age < 90*24*time.Hour {
			b = "30-90d"
		} else {
			b = "90d+"
		}
		result.Summary.ByAgeBucket[b]++
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
