package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// v26.55 Scalability: HPABehaviorPolicy, TopNSBySTS, PVPhaseDist

type HPABehaviorPolicy2655Result struct {
	ScannedAt   time.Time                    `json:"scannedAt"`
	Summary     HPABehaviorPolicy2655Summary `json:"summary"`
	Items       []HPABehaviorPolicy2655Item  `json:"items"`
	HealthScore int                          `json:"healthScore"`
	Grade       string                       `json:"grade"`
}

type HPABehaviorPolicy2655Summary struct {
	TotalHPA     int `json:"totalHPA"`
	WithBehavior int `json:"withBehavior"`
	NoBehavior   int `json:"noBehavior"`
}

type HPABehaviorPolicy2655Item struct {
	Name        string `json:"name"`
	Namespace   string `json:"namespace"`
	HasBehavior bool   `json:"hasBehavior"`
}

func (s *Server) handleHPABehaviorPolicy2655(w http.ResponseWriter, r *http.Request) {
	result := HPABehaviorPolicy2655Result{ScannedAt: time.Now()}
	hpas, err := s.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, hpa := range hpas.Items {
			result.Summary.TotalHPA++
			hasBehavior := hpa.Spec.Behavior != nil
			if hasBehavior {
				result.Summary.WithBehavior++
			} else {
				result.Summary.NoBehavior++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, HPABehaviorPolicy2655Item{
					Name: hpa.Name, Namespace: hpa.Namespace, HasBehavior: hasBehavior,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type TopNSBySTS2655Result struct {
	ScannedAt   time.Time             `json:"scannedAt"`
	Summary     TopNSBySTS2655Summary `json:"summary"`
	Items       []TopNSBySTS2655Item  `json:"items"`
	HealthScore int                   `json:"healthScore"`
	Grade       string                `json:"grade"`
}

type TopNSBySTS2655Summary struct {
	TotalNamespaces int `json:"totalNamespaces"`
	TotalSTS        int `json:"totalSTS"`
}

type TopNSBySTS2655Item struct {
	Namespace string `json:"namespace"`
	STSCount  int    `json:"stsCount"`
}

func (s *Server) handleTopNSBySTS2655(w http.ResponseWriter, r *http.Request) {
	result := TopNSBySTS2655Result{ScannedAt: time.Now()}
	stss, err := s.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		nsCount := map[string]int{}
		for _, sts := range stss.Items {
			nsCount[sts.Namespace]++
			result.Summary.TotalSTS++
		}
		result.Summary.TotalNamespaces = len(nsCount)
		for ns, cnt := range nsCount {
			if len(result.Items) < 50 {
				result.Items = append(result.Items, TopNSBySTS2655Item{
					Namespace: ns, STSCount: cnt,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PVPhaseDist2655Result struct {
	ScannedAt   time.Time              `json:"scannedAt"`
	Summary     PVPhaseDist2655Summary `json:"summary"`
	Items       []PVPhaseDist2655Item  `json:"items"`
	HealthScore int                    `json:"healthScore"`
	Grade       string                 `json:"grade"`
}

type PVPhaseDist2655Summary struct {
	TotalPVs  int `json:"totalPVs"`
	Bound     int `json:"bound"`
	Available int `json:"available"`
	Released  int `json:"released"`
	Failed    int `json:"failed"`
}

type PVPhaseDist2655Item struct {
	Name  string `json:"name"`
	Phase string `json:"phase"`
}

func (s *Server) handlePVPhaseDist2655(w http.ResponseWriter, r *http.Request) {
	result := PVPhaseDist2655Result{ScannedAt: time.Now()}
	pvs, err := s.clientset.CoreV1().PersistentVolumes().List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, pv := range pvs.Items {
			result.Summary.TotalPVs++
			ph := string(pv.Status.Phase)
			switch pv.Status.Phase {
			case "Bound":
				result.Summary.Bound++
			case "Available":
				result.Summary.Available++
			case "Released":
				result.Summary.Released++
			case "Failed":
				result.Summary.Failed++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, PVPhaseDist2655Item{
					Name: pv.Name, Phase: ph,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
