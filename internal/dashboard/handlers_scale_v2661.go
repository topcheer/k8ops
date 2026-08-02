package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// v26.61 Scalability: HPAMinReplicas, TopNSByPods, StorageClassDefault

type HPAMinReplicas2661Result struct {
	ScannedAt   time.Time                 `json:"scannedAt"`
	Summary     HPAMinReplicas2661Summary `json:"summary"`
	Items       []HPAMinReplicas2661Item  `json:"items"`
	HealthScore int                       `json:"healthScore"`
	Grade       string                    `json:"grade"`
}

type HPAMinReplicas2661Summary struct {
	TotalHPA    int `json:"totalHPA"`
	MinRep1     int `json:"minRep1"`
	MinRep2Plus int `json:"minRep2Plus"`
}

type HPAMinReplicas2661Item struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	MinReplicas int32  `json:"minReplicas"`
}

func (s *Server) handleHPAMinReplicas2661(w http.ResponseWriter, r *http.Request) {
	result := HPAMinReplicas2661Result{ScannedAt: time.Now()}
	hpas, err := s.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, hpa := range hpas.Items {
			result.Summary.TotalHPA++
			minRep := int32(1)
			if hpa.Spec.MinReplicas != nil {
				minRep = *hpa.Spec.MinReplicas
			}
			if minRep <= 1 {
				result.Summary.MinRep1++
			} else {
				result.Summary.MinRep2Plus++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, HPAMinReplicas2661Item{
					Name: hpa.Name, Namespace: hpa.Namespace, MinReplicas: minRep,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type TopNSByPods2661Result struct {
	ScannedAt   time.Time              `json:"scannedAt"`
	Summary     TopNSByPods2661Summary `json:"summary"`
	Items       []TopNSByPods2661Item  `json:"items"`
	HealthScore int                    `json:"healthScore"`
	Grade       string                 `json:"grade"`
}

type TopNSByPods2661Summary struct {
	TotalNamespaces int `json:"totalNamespaces"`
	TotalPods       int `json:"totalPods"`
}

type TopNSByPods2661Item struct {
	Namespace string `json:"namespace"`
	PodCount  int    `json:"podCount"`
}

func (s *Server) handleTopNSByPods2661(w http.ResponseWriter, r *http.Request) {
	result := TopNSByPods2661Result{ScannedAt: time.Now()}
	pods, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		nsCount := map[string]int{}
		for _, pod := range pods.Items {
			nsCount[pod.Namespace]++
			result.Summary.TotalPods++
		}
		result.Summary.TotalNamespaces = len(nsCount)
		for ns, cnt := range nsCount {
			if len(result.Items) < 50 {
				result.Items = append(result.Items, TopNSByPods2661Item{
					Namespace: ns, PodCount: cnt,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type StorageClassDefault2661Result struct {
	ScannedAt   time.Time                      `json:"scannedAt"`
	Summary     StorageClassDefault2661Summary `json:"summary"`
	Items       []StorageClassDefault2661Item  `json:"items"`
	HealthScore int                            `json:"healthScore"`
	Grade       string                         `json:"grade"`
}

type StorageClassDefault2661Summary struct {
	TotalSCs   int `json:"totalSCs"`
	HasDefault int `json:"hasDefault"`
	NoDefault  int `json:"noDefault"`
}

type StorageClassDefault2661Item struct {
	Name        string `json:"name"`
	Provisioner string `json:"provisioner"`
	IsDefault   bool   `json:"isDefault"`
}

func (s *Server) handleStorageClassDefault2661(w http.ResponseWriter, r *http.Request) {
	result := StorageClassDefault2661Result{ScannedAt: time.Now()}
	scs, err := s.clientset.StorageV1().StorageClasses().List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, sc := range scs.Items {
			result.Summary.TotalSCs++
			isDefault := false
			if sc.Annotations != nil {
				if v, ok := sc.Annotations["storageclass.kubernetes.io/is-default-class"]; ok && v == "true" {
					isDefault = true
				}
			}
			if isDefault {
				result.Summary.HasDefault++
			} else {
				result.Summary.NoDefault++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, StorageClassDefault2661Item{
					Name: sc.Name, Provisioner: sc.Provisioner, IsDefault: isDefault,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
