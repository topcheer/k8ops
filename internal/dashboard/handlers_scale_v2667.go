package dashboard

import (
	"net/http"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// v26.67 Scalability: HPAScaleTargetRef, TopNSByCM, CSIWorkerNodes

type HPAScaleTargetRef2667Result struct {
	ScannedAt   time.Time                    `json:"scannedAt"`
	Summary     HPAScaleTargetRef2667Summary `json:"summary"`
	Items       []HPAScaleTargetRef2667Item  `json:"items"`
	HealthScore int                          `json:"healthScore"`
	Grade       string                       `json:"grade"`
}

type HPAScaleTargetRef2667Summary struct {
	TotalHPA    int `json:"totalHPA"`
	TargetDep   int `json:"targetDeployment"`
	TargetSTS   int `json:"targetSTS"`
	TargetOther int `json:"targetOther"`
}

type HPAScaleTargetRef2667Item struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	TargetKind string `json:"targetKind"`
	TargetName string `json:"targetName"`
}

func (s *Server) handleHPAScaleTargetRef2667(w http.ResponseWriter, r *http.Request) {
	result := HPAScaleTargetRef2667Result{ScannedAt: time.Now()}
	hpas, err := s.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, hpa := range hpas.Items {
			result.Summary.TotalHPA++
			kind := hpa.Spec.ScaleTargetRef.Kind
			name := hpa.Spec.ScaleTargetRef.Name
			switch kind {
			case "Deployment":
				result.Summary.TargetDep++
			case "StatefulSet":
				result.Summary.TargetSTS++
			default:
				result.Summary.TargetOther++
			}
			if len(result.Items) < 50 {
				result.Items = append(result.Items, HPAScaleTargetRef2667Item{
					Name: hpa.Name, Namespace: hpa.Namespace, TargetKind: kind, TargetName: name,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type TopNSByCM2667Result struct {
	ScannedAt   time.Time            `json:"scannedAt"`
	Summary     TopNSByCM2667Summary `json:"summary"`
	Items       []TopNSByCM2667Item  `json:"items"`
	HealthScore int                  `json:"healthScore"`
	Grade       string               `json:"grade"`
}

type TopNSByCM2667Summary struct {
	TotalNamespaces int `json:"totalNamespaces"`
	TotalCMs        int `json:"totalCMs"`
}

type TopNSByCM2667Item struct {
	Namespace string `json:"namespace"`
	CMCount   int    `json:"cmCount"`
}

func (s *Server) handleTopNSByCM2667(w http.ResponseWriter, r *http.Request) {
	result := TopNSByCM2667Result{ScannedAt: time.Now()}
	cms, err := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		nsCount := map[string]int{}
		for _, cm := range cms.Items {
			nsCount[cm.Namespace]++
			result.Summary.TotalCMs++
		}
		result.Summary.TotalNamespaces = len(nsCount)
		for ns, cnt := range nsCount {
			if len(result.Items) < 50 {
				result.Items = append(result.Items, TopNSByCM2667Item{
					Namespace: ns, CMCount: cnt,
				})
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type CSIWorkerNodes2667Result struct {
	ScannedAt   time.Time                 `json:"scannedAt"`
	Summary     CSIWorkerNodes2667Summary `json:"summary"`
	Items       []CSIWorkerNodes2667Item  `json:"items"`
	HealthScore int                       `json:"healthScore"`
	Grade       string                    `json:"grade"`
}

type CSIWorkerNodes2667Summary struct {
	TotalNodes     int `json:"totalNodes"`
	CSIDriverNodes int `json:"csiDriverNodes"`
	NoCSINodes     int `json:"noCsiNodes"`
}

type CSIWorkerNodes2667Item struct {
	Name      string `json:"name"`
	CSIDriver string `json:"csiDriver"`
}

func (s *Server) handleCSIWorkerNodes2667(w http.ResponseWriter, r *http.Request) {
	result := CSIWorkerNodes2667Result{ScannedAt: time.Now()}
	nodes, err := s.clientset.StorageV1().CSINodes().List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, csiNode := range nodes.Items {
			result.Summary.TotalNodes++
			if len(csiNode.Spec.Drivers) > 0 {
				result.Summary.CSIDriverNodes++
				driverNames := make([]string, 0, len(csiNode.Spec.Drivers))
				for _, d := range csiNode.Spec.Drivers {
					driverNames = append(driverNames, d.Name)
				}
				if len(result.Items) < 50 {
					result.Items = append(result.Items, CSIWorkerNodes2667Item{
						Name: csiNode.Name, CSIDriver: driverNames[0],
					})
				}
			} else {
				result.Summary.NoCSINodes++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
