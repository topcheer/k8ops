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
// v20.00 — Scalability & HA Dimension (Round 19 Final, v20 Milestone)
// 1. Control Plane HA — API server replica & etcd health estimator
// 2. Pod Anti-Affinity Coverage — spread guarantee for HA workloads
// 3. Resource Request Headroom — cluster-wide schedulable capacity
// ============================================================

// ---------------------------------------------------------------
// 1. Control Plane HA
// ---------------------------------------------------------------

type CPHAAResult2000 struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	HealthScore     int              `json:"healthScore"`
	Grade           string           `json:"grade"`
	Summary         CPHAASummary2000 `json:"summary"`
	Components      []CPHAAEntry2000 `json:"components"`
	Recommendations []string         `json:"recommendations"`
}

type CPHAASummary2000 struct {
	APIServerCount int    `json:"estAPIServerReplicas"`
	EtcdCount      int    `json:"estEtcdReplicas"`
	TotalNodes     int    `json:"totalNodes"`
	HALevel        string `json:"haLevel"`
	FaultTolerance int    `json:"estFaultTolerance"`
}

type CPHAAEntry2000 struct {
	Component string `json:"component"`
	Count     int    `json:"estimatedReplicas"`
	Status    string `json:"status"`
}

func (s *Server) handleControlPlaneHA(w http.ResponseWriter, r *http.Request) {
	result := CPHAAResult2000{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	result.Summary.TotalNodes = len(nodeList.Items)

	// Detect control plane components
	apiServerCount := 0
	etcdCount := 0
	controllerMgrCount := 0
	schedulerCount := 0

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		ns := pod.Namespace
		if ns != "kube-system" {
			continue
		}
		for _, c := range pod.Spec.Containers {
			// Match by container name or image
			name := c.Name
			switch {
			case name == "kube-apiserver" || containsStr2000(c.Image, "kube-apiserver"):
				apiServerCount++
			case name == "etcd" || containsStr2000(c.Image, "etcd"):
				etcdCount++
			case name == "kube-controller-manager" || containsStr2000(c.Image, "kube-controller-manager"):
				controllerMgrCount++
			case name == "kube-scheduler" || containsStr2000(c.Image, "kube-scheduler"):
				schedulerCount++
			}
		}
	}

	// If no explicit pods found, estimate from node count (single-node clusters)
	if apiServerCount == 0 {
		apiServerCount = 1 // at least static pod or embedded
	}
	if etcdCount == 0 {
		etcdCount = 1
	}

	result.Summary.APIServerCount = apiServerCount
	result.Summary.EtcdCount = etcdCount
	result.Summary.FaultTolerance = (etcdCount - 1) / 2

	if apiServerCount >= 3 && etcdCount >= 3 {
		result.Summary.HALevel = "high"
	} else if apiServerCount >= 2 || etcdCount >= 2 {
		result.Summary.HALevel = "medium"
		score -= 10
	} else {
		result.Summary.HALevel = "none"
		score -= 20
	}

	result.Components = append(result.Components, CPHAAEntry2000{
		Component: "kube-apiserver", Count: apiServerCount, Status: "running",
	})
	result.Components = append(result.Components, CPHAAEntry2000{
		Component: "etcd", Count: etcdCount, Status: "running",
	})
	result.Components = append(result.Components, CPHAAEntry2000{
		Component: "kube-controller-manager", Count: controllerMgrCount, Status: "running",
	})
	result.Components = append(result.Components, CPHAAEntry2000{
		Component: "kube-scheduler", Count: schedulerCount, Status: "running",
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("API server: %d, etcd: %d, HA level: %s, fault tolerance: %d node(s)", apiServerCount, etcdCount, result.Summary.HALevel, result.Summary.FaultTolerance))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Pod Anti-Affinity Coverage
// ---------------------------------------------------------------

type AntiAffResult2000 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         AntiAffSummary2000 `json:"summary"`
	Coverage        []AntiAffEntry2000 `json:"coverage"`
	Recommendations []string           `json:"recommendations"`
}

type AntiAffSummary2000 struct {
	TotalDeployments    int `json:"totalDeployments"`
	WithAntiAffinity    int `json:"withPodAntiAffinity"`
	WithReqAntiAffinity int `json:"withRequiredAntiAffinity"`
	WithoutAny          int `json:"withoutAnySpread"`
}

type AntiAffEntry2000 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Replicas  int    `json:"replicas"`
	Type      string `json:"antiAffinityType"`
}

func (s *Server) handleAntiAffinityCoverageV2(w http.ResponseWriter, r *http.Request) {
	result := AntiAffResult2000{ScannedAt: time.Now()}
	score := 100

	depList, _ := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})

	for _, dep := range depList.Items {
		result.Summary.TotalDeployments++

		replicas := 1
		if dep.Spec.Replicas != nil {
			replicas = int(*dep.Spec.Replicas)
		}

		// Skip single-replica deployments
		if replicas < 2 {
			continue
		}

		hasAntiAff := false
		affType := "none"
		if dep.Spec.Template.Spec.Affinity != nil && dep.Spec.Template.Spec.Affinity.PodAntiAffinity != nil {
			hasAntiAff = true
			aff := dep.Spec.Template.Spec.Affinity.PodAntiAffinity
			if len(aff.RequiredDuringSchedulingIgnoredDuringExecution) > 0 {
				affType = "required"
				result.Summary.WithReqAntiAffinity++
			} else if len(aff.PreferredDuringSchedulingIgnoredDuringExecution) > 0 {
				affType = "preferred"
			}
		}

		entry := AntiAffEntry2000{
			Name: dep.Name, Namespace: dep.Namespace,
			Replicas: replicas, Type: affType,
		}

		if hasAntiAff {
			result.Summary.WithAntiAffinity++
		} else {
			result.Summary.WithoutAny++
			score -= 1
		}

		result.Coverage = append(result.Coverage, entry)
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d multi-replica deps: %d with anti-affinity (%d required), %d without spread", result.Summary.TotalDeployments, result.Summary.WithAntiAffinity, result.Summary.WithReqAntiAffinity, result.Summary.WithoutAny))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Resource Request Headroom
// ---------------------------------------------------------------

type HeadroomResult2000 struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Summary         HeadroomSummary2000 `json:"summary"`
	PerNode         []HeadroomEntry2000 `json:"perNode"`
	Recommendations []string            `json:"recommendations"`
}

type HeadroomSummary2000 struct {
	TotalNodes     int     `json:"totalNodes"`
	TotalAllocCPU  float64 `json:"totalAllocatableCPU"`
	TotalAllocMem  float64 `json:"totalAllocatableMemGB"`
	TotalReqCPU    float64 `json:"totalRequestedCPU"`
	TotalReqMem    float64 `json:"totalRequestedMemGB"`
	CPUHeadroomPct float64 `json:"cpuHeadroomPct"`
	MemHeadroomPct float64 `json:"memHeadroomPct"`
}

type HeadroomEntry2000 struct {
	Node        string  `json:"node"`
	AllocCPU    float64 `json:"allocatableCPU"`
	ReqCPU      float64 `json:"requestedCPU"`
	HeadroomPct float64 `json:"headroomPct"`
}

func (s *Server) handleRequestHeadroom(w http.ResponseWriter, r *http.Request) {
	result := HeadroomResult2000{ScannedAt: time.Now()}
	score := 100

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	nodeReq := make(map[string]float64)
	for _, pod := range podList.Items {
		if (pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodPending) || pod.Spec.NodeName == "" {
			continue
		}
		for _, c := range pod.Spec.Containers {
			nodeReq[pod.Spec.NodeName] += c.Resources.Requests.Cpu().AsApproximateFloat64()
		}
	}

	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++

		allocCPU := node.Status.Allocatable.Cpu().AsApproximateFloat64()
		reqCPU := nodeReq[node.Name]

		result.Summary.TotalAllocCPU += allocCPU
		result.Summary.TotalReqCPU += reqCPU

		headroom := 0.0
		if allocCPU > 0 {
			headroom = (1 - reqCPU/allocCPU) * 100
		}

		result.PerNode = append(result.PerNode, HeadroomEntry2000{
			Node: node.Name, AllocCPU: allocCPU, ReqCPU: reqCPU, HeadroomPct: headroom,
		})
	}

	if result.Summary.TotalAllocCPU > 0 {
		result.Summary.CPUHeadroomPct = (1 - result.Summary.TotalReqCPU/result.Summary.TotalAllocCPU) * 100
	}

	// Memory headroom
	var totalAllocMem, totalReqMem float64
	for _, node := range nodeList.Items {
		allocMem := float64(node.Status.Allocatable.Memory().Value()) / (1024 * 1024 * 1024)
		totalAllocMem += allocMem
	}
	for _, pod := range podList.Items {
		if (pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodPending) || pod.Spec.NodeName == "" {
			continue
		}
		for _, c := range pod.Spec.Containers {
			totalReqMem += float64(c.Resources.Requests.Memory().Value()) / (1024 * 1024 * 1024)
		}
	}
	result.Summary.TotalAllocMem = totalAllocMem
	result.Summary.TotalReqMem = totalReqMem
	if totalAllocMem > 0 {
		result.Summary.MemHeadroomPct = (1 - totalReqMem/totalAllocMem) * 100
	}

	if result.Summary.CPUHeadroomPct < 10 {
		score -= 10
	} else if result.Summary.CPUHeadroomPct < 25 {
		score -= 5
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d nodes, CPU headroom %.0f%%, Mem headroom %.0f%%", result.Summary.TotalNodes, result.Summary.CPUHeadroomPct, result.Summary.MemHeadroomPct))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

func containsStr2000(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
