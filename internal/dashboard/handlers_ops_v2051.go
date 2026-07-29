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
// v20.51 — Operations Dimension (Round 28)
// 1. Kube Proxy Health — kube-proxy pod health monitoring
// 2. CNI Plugin Audit — CNI plugin pod status
// 3. Storage Operation Latency — PVC attach/detach operation timing
// ============================================================

// ---------------------------------------------------------------
// 1. Kube Proxy Health
// ---------------------------------------------------------------

type KubeProxyResult2051 struct {
	ScannedAt       time.Time            `json:"scannedAt"`
	HealthScore     int                  `json:"healthScore"`
	Grade           string               `json:"grade"`
	Summary         KubeProxySummary2051 `json:"summary"`
	Issues          []KubeProxyEntry2051 `json:"issues"`
	Recommendations []string             `json:"recommendations"`
}

type KubeProxySummary2051 struct {
	ProxyPodsFound int `json:"proxyPodsFound"`
	HealthyPods    int `json:"healthyPods"`
	RestartedPods  int `json:"restartedPods"`
}

type KubeProxyEntry2051 struct {
	Pod      string `json:"pod"`
	Status   string `json:"status"`
	Restarts int32  `json:"restarts"`
}

func (s *Server) handleKubeProxyHealth2051(w http.ResponseWriter, r *http.Request) {
	result := KubeProxyResult2051{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	for _, pod := range podList.Items {
		if !containsStr2039(pod.Name, "kube-proxy") {
			continue
		}
		result.Summary.ProxyPodsFound++

		status := "running"
		if pod.Status.Phase != corev1.PodRunning {
			status = string(pod.Status.Phase)
			score -= 10
		} else {
			result.Summary.HealthyPods++
		}

		var restarts int32
		for _, cs := range pod.Status.ContainerStatuses {
			restarts += cs.RestartCount
		}
		if restarts > 0 {
			result.Summary.RestartedPods++
			score -= 5
		}

		if pod.Status.Phase != corev1.PodRunning || restarts > 0 {
			result.Issues = append(result.Issues, KubeProxyEntry2051{
				Pod: pod.Name, Status: status, Restarts: restarts,
			})
		}
	}

	if result.Summary.ProxyPodsFound == 0 {
		score -= 10
		result.Recommendations = append(result.Recommendations,
			"No kube-proxy pods found — network routing may be impaired")
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.RestartedPods > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d kube-proxy pods restarted — check network configuration", result.Summary.RestartedPods))
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. CNI Plugin Audit
// ---------------------------------------------------------------

type CNIResult2051 struct {
	ScannedAt       time.Time      `json:"scannedAt"`
	HealthScore     int            `json:"healthScore"`
	Grade           string         `json:"grade"`
	Summary         CNISummary2051 `json:"summary"`
	CNIPods         []CNIEntry2051 `json:"cniPods"`
	Recommendations []string       `json:"recommendations"`
}

type CNISummary2051 struct {
	CNIPodsFound int    `json:"cniPodsFound"`
	HealthyPods  int    `json:"healthyPods"`
	CNIDetected  string `json:"cniDetected"`
}

type CNIEntry2051 struct {
	Pod    string `json:"pod"`
	Status string `json:"status"`
}

func (s *Server) handleCNIAudit2051(w http.ResponseWriter, r *http.Request) {
	result := CNIResult2051{ScannedAt: time.Now()}
	score := 100

	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	cniNames := []string{"flannel", "calico", "cilium", "weave", "canal", "kube-router", "traefik"}
	for _, pod := range podList.Items {
		isCNI := false
		for _, cni := range cniNames {
			if containsStr2039(pod.Name, cni) || containsStr2039(pod.Namespace, cni) {
				isCNI = true
				result.Summary.CNIDetected = cni
				break
			}
		}
		if !isCNI {
			continue
		}

		result.Summary.CNIPodsFound++
		status := "running"
		if pod.Status.Phase == corev1.PodRunning {
			result.Summary.HealthyPods++
		} else {
			status = string(pod.Status.Phase)
			score -= 5
		}

		result.CNIPods = append(result.CNIPods, CNIEntry2051{
			Pod: pod.Name, Status: status,
		})
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	if result.Summary.CNIPodsFound == 0 {
		result.Recommendations = append(result.Recommendations,
			"No CNI plugin pods detected — network functionality may be limited")
	}

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Storage Operation Latency
// ---------------------------------------------------------------

type StorOpResult2051 struct {
	ScannedAt       time.Time         `json:"scannedAt"`
	HealthScore     int               `json:"healthScore"`
	Grade           string            `json:"grade"`
	Summary         StorOpSummary2051 `json:"summary"`
	SlowVolumes     []StorOpEntry2051 `json:"slowVolumes"`
	Recommendations []string          `json:"recommendations"`
}

type StorOpSummary2051 struct {
	TotalPVCs   int `json:"totalPVCs"`
	PendingPVCs int `json:"pendingPVCs"`
	BoundPVCs   int `json:"boundPVCs"`
	StuckPVCs   int `json:"stuckPVCs"`
}

type StorOpEntry2051 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Phase     string `json:"phase"`
}

func (s *Server) handleStorOpLatency(w http.ResponseWriter, r *http.Request) {
	result := StorOpResult2051{ScannedAt: time.Now()}
	score := 100

	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})

	now := time.Now()
	for _, pvc := range pvcList.Items {
		result.Summary.TotalPVCs++

		switch pvc.Status.Phase {
		case corev1.ClaimBound:
			result.Summary.BoundPVCs++
		case corev1.ClaimPending:
			result.Summary.PendingPVCs++
			ageHours := now.Sub(pvc.CreationTimestamp.Time).Hours()
			if ageHours > 1 {
				result.Summary.StuckPVCs++
				result.SlowVolumes = append(result.SlowVolumes, StorOpEntry2051{
					Name: pvc.Name, Namespace: pvc.Namespace, Phase: "pending-stuck",
				})
				score -= 5
			}
		case corev1.ClaimLost:
			result.Summary.StuckPVCs++
			result.SlowVolumes = append(result.SlowVolumes, StorOpEntry2051{
				Name: pvc.Name, Namespace: pvc.Namespace, Phase: "lost",
			})
			score -= 10
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)

	sort.Slice(result.SlowVolumes, func(i, j int) bool {
		return result.SlowVolumes[i].Namespace < result.SlowVolumes[j].Namespace
	})

	if result.Summary.StuckPVCs > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d PVCs are stuck — check storage class provisioner and node storage", result.Summary.StuckPVCs))
	}

	writeJSON(w, result)
}
