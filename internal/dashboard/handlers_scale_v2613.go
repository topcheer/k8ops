package dashboard

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"net/http"
	"sort"
	"time"
)

// v26.13 Scalability: Top Namespace by PDB, Node Memory Request vs Limit, Cluster EndpointSlice Port Count
type TopNSByPDB2613Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNS int `json:"totalNamespaces"`
	}
	TopNS []struct {
		Namespace string `json:"namespace"`
		PDBCount  int    `json:"pdbCount"`
	} `json:"topNamespaces"`
}

func (s *Server) handleTopNSByPDB2613(w http.ResponseWriter, r *http.Request) {
	result := TopNSByPDB2613Result{ScannedAt: time.Now()}
	pdbList, _ := s.clientset.PolicyV1().PodDisruptionBudgets("").List(r.Context(), metav1.ListOptions{})
	nsPDBs := make(map[string]int)
	for _, pdb := range pdbList.Items {
		nsPDBs[pdb.Namespace]++
	}
	result.Summary.TotalNS = len(nsPDBs)
	for ns, count := range nsPDBs {
		result.TopNS = append(result.TopNS, struct {
			Namespace string `json:"namespace"`
			PDBCount  int    `json:"pdbCount"`
		}{ns, count})
	}
	sort.Slice(result.TopNS, func(i, j int) bool { return result.TopNS[i].PDBCount > result.TopNS[j].PDBCount })
	if len(result.TopNS) > 10 {
		result.TopNS = result.TopNS[:10]
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type NodeMemReqVsLim2613Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods int     `json:"totalPods"`
		TotalReq  float64 `json:"totalMemReqGB"`
		TotalLim  float64 `json:"totalMemLimGB"`
	} `json:"summary"`
}

func (s *Server) handleNodeMemReqVsLim2613(w http.ResponseWriter, r *http.Request) {
	result := NodeMemReqVsLim2613Result{ScannedAt: time.Now()}
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		for _, c := range pod.Spec.Containers {
			result.Summary.TotalReq += c.Resources.Requests.Memory().AsApproximateFloat64() / (1024 * 1024 * 1024)
			result.Summary.TotalLim += c.Resources.Limits.Memory().AsApproximateFloat64() / (1024 * 1024 * 1024)
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}

type EPSlicePort2613Result struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSlices int            `json:"totalSlices"`
		ByPortName  map[string]int `json:"byPortName"`
	} `json:"summary"`
}

func (s *Server) handleEPSlicePort2613(w http.ResponseWriter, r *http.Request) {
	result := EPSlicePort2613Result{ScannedAt: time.Now()}
	result.Summary.ByPortName = make(map[string]int)
	sliceList, _ := s.clientset.DiscoveryV1().EndpointSlices("").List(r.Context(), metav1.ListOptions{})
	for _, slice := range sliceList.Items {
		result.Summary.TotalSlices++
		for _, port := range slice.Ports {
			if port.Name != nil {
				result.Summary.ByPortName[*port.Name]++
			}
		}
	}
	result.HealthScore = 100
	gradeFromScore(&result.Grade, 100)
	writeJSON(w, result)
}
