package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// v26.49 Scalability: HPATargetUtil, TopNSByDeployment, PVCStorageClass

type HPATargetUtil2649Result struct {
	ScannedAt   time.Time                `json:"scannedAt"`
	Summary     HPATargetUtil2649Summary `json:"summary"`
	Items       []HPATargetUtil2649Item  `json:"items"`
	HealthScore int                      `json:"healthScore"`
	Grade       string                   `json:"grade"`
}

type HPATargetUtil2649Summary struct {
	TotalHPA   int `json:"totalHPA"`
	WithTarget int `json:"withTarget"`
	NoTarget   int `json:"noTarget"`
}

type HPATargetUtil2649Item struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	TargetUtil int32  `json:"targetUtil"`
}

func (s *Server) handleHPATargetUtil2649(w http.ResponseWriter, r *http.Request) {
	result := HPATargetUtil2649Result{ScannedAt: time.Now()}
	hpas, err := s.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, hpa := range hpas.Items {
			result.Summary.TotalHPA++
			utilVal := int32(0)
			hasTarget := false
			if hpa.Spec.Metrics != nil {
				for _, m := range hpa.Spec.Metrics {
					if m.Resource != nil && m.Resource.Target.AverageUtilization != nil {
						utilVal = *m.Resource.Target.AverageUtilization
						hasTarget = true
						break
					}
				}
			}
			if hasTarget {
				result.Summary.WithTarget++
			} else {
				result.Summary.NoTarget++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, HPATargetUtil2649Item{
					Name: hpa.Name, Namespace: hpa.Namespace, TargetUtil: utilVal,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type TopNSByDeploy2649Result struct {
	ScannedAt   time.Time                `json:"scannedAt"`
	Summary     TopNSByDeploy2649Summary `json:"summary"`
	Items       []TopNSByDeploy2649Item  `json:"items"`
	HealthScore int                      `json:"healthScore"`
	Grade       string                   `json:"grade"`
}

type TopNSByDeploy2649Summary struct {
	TotalNamespaces  int `json:"totalNamespaces"`
	TotalDeployments int `json:"totalDeployments"`
}

type TopNSByDeploy2649Item struct {
	Namespace   string `json:"namespace"`
	DeployCount int    `json:"deployCount"`
}

func (s *Server) handleTopNSByDeploy2649(w http.ResponseWriter, r *http.Request) {
	result := TopNSByDeploy2649Result{ScannedAt: time.Now()}
	deps, err := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		nsCount := map[string]int{}
		for _, dep := range deps.Items {
			nsCount[dep.Namespace]++
			result.Summary.TotalDeployments++
		}
		result.Summary.TotalNamespaces = len(nsCount)
		for ns, cnt := range nsCount {
			if len(result.Items) < 50 {
				result.Items = append(result.Items, TopNSByDeploy2649Item{
					Namespace: ns, DeployCount: cnt,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PVCStorageClass2649Result struct {
	ScannedAt   time.Time                  `json:"scannedAt"`
	Summary     PVCStorageClass2649Summary `json:"summary"`
	Items       []PVCStorageClass2649Item  `json:"items"`
	HealthScore int                        `json:"healthScore"`
	Grade       string                     `json:"grade"`
}

type PVCStorageClass2649Summary struct {
	TotalPVCs  int `json:"totalPVCs"`
	ClassCount int `json:"classCount"`
}

type PVCStorageClass2649Item struct {
	Name         string `json:"name"`
	Namespace    string `json:"namespace"`
	StorageClass string `json:"storageClass"`
}

func (s *Server) handlePVCStorageClass2649(w http.ResponseWriter, r *http.Request) {
	result := PVCStorageClass2649Result{ScannedAt: time.Now()}
	pvcs, err := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		classSet := map[string]bool{}
		for _, pvc := range pvcs.Items {
			result.Summary.TotalPVCs++
			sc := *pvc.Spec.StorageClassName
			if sc != "" {
				classSet[sc] = true
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, PVCStorageClass2649Item{
					Name: pvc.Name, Namespace: pvc.Namespace, StorageClass: sc,
				})
			}
		}
		result.Summary.ClassCount = len(classSet)
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
