package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v24.40 Product: Top Pod by Memory Request, Container Stdin Count, PVC Access Modes Summary
type TopPodMemReqResult2440 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int `json:"totalPods"`
	} `json:"summary"`
	TopPods []struct {
		Pod    string  `json:"pod"`
		MemReq float64 `json:"memReqMB"`
	} `json:"topPods"`
}

func (s *Server) handleTopPodMemReq2440(w http.ResponseWriter, r *http.Request) {
	result := TopPodMemReqResult2440{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	podMem := make(map[string]float64)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		var mem float64
		for _, c := range pod.Spec.Containers {
			mem += c.Resources.Requests.Memory().AsApproximateFloat64() / (1024 * 1024)
		}
		podMem[pod.Namespace+"/"+pod.Name] = mem
	}
	for k, v := range podMem {
		result.TopPods = append(result.TopPods, struct {
			Pod    string  `json:"pod"`
			MemReq float64 `json:"memReqMB"`
		}{k, v})
	}
	sort.Slice(result.TopPods, func(i, j int) bool { return result.TopPods[i].MemReq > result.TopPods[j].MemReq })
	if len(result.TopPods) > 10 {
		result.TopPods = result.TopPods[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type StdinCountResult2440 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalContainers int `json:"totalContainers"`
		WithStdin       int `json:"withStdin"`
	} `json:"summary"`
}

func (s *Server) handleStdinCount2440(w http.ResponseWriter, r *http.Request) {
	result := StdinCountResult2440{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalContainers++
			if c.Stdin {
				result.Summary.WithStdin++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type PVCAccessModesResult2440 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPVCs int            `json:"totalPVCs"`
		ByMode    map[string]int `json:"byAccessMode"`
	} `json:"summary"`
}

func (s *Server) handlePVCAccessModes2440(w http.ResponseWriter, r *http.Request) {
	result := PVCAccessModesResult2440{ScannedAt: time.Now()}
	result.Summary.ByMode = make(map[string]int)
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})
	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++
		for _, am := range pvc.Spec.AccessModes {
			result.Summary.ByMode[string(am)]++
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
