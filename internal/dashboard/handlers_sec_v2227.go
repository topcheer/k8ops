package dashboard

import (
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v22.27 — Security Dimension (Round 57)
// 1. Pod ProcMount Type Audit
// 2. Secret DockerConfigJSON Tracker
// 3. NetworkPolicy Port Named Distribution
// ============================================================

type ProcMountResult2227 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalPods   int            `json:"totalPods"`
		ByProcMount map[string]int `json:"byProcMountType"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleProcMount2227(w http.ResponseWriter, r *http.Request) {
	result := ProcMountResult2227{ScannedAt: time.Now()}
	score := 100
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByProcMount = make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		result.Summary.TotalPods++
		procMount := "Default"
		for _, c := range pod.Spec.Containers {
			if c.SecurityContext != nil && c.SecurityContext.ProcMount != nil {
				procMount = string(*c.SecurityContext.ProcMount)
				break
			}
		}
		result.Summary.ByProcMount[procMount]++
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 2. DockerConfigJSON Tracker
type DockerCfgResult2227 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalSecrets  int `json:"totalSecrets"`
		DockerCfgJSON int `json:"dockerconfigjsonCount"`
		OtherRegistry int `json:"otherRegistrySecrets"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleDockerCfg2227(w http.ResponseWriter, r *http.Request) {
	result := DockerCfgResult2227{ScannedAt: time.Now()}
	score := 100
	secretList, _ := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	for _, secret := range secretList.Items {
		result.Summary.TotalSecrets++
		if string(secret.Type) == "kubernetes.io/dockerconfigjson" {
			result.Summary.DockerCfgJSON++
		} else if containsStr2039(string(secret.Type), "registry") {
			result.Summary.OtherRegistry++
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}

// 3. NP Port Named Distribution
type NPPortNamedResult2227 struct {
	ScannedAt   time.Time `json:"scannedAt"`
	HealthScore int       `json:"healthScore"`
	Grade       string    `json:"grade"`
	Summary     struct {
		TotalNP       int            `json:"totalNetworkPolicies"`
		WithNamedPort int            `json:"withNamedPort"`
		ByPortName    map[string]int `json:"byPortName"`
	} `json:"summary"`
	Recommendations []string `json:"recommendations"`
}

func (s *Server) handleNPPortNamed2227(w http.ResponseWriter, r *http.Request) {
	result := NPPortNamedResult2227{ScannedAt: time.Now()}
	score := 100
	npList, _ := s.clientset.NetworkingV1().NetworkPolicies("").List(r.Context(), metav1.ListOptions{})
	result.Summary.ByPortName = make(map[string]int)
	for _, np := range npList.Items {
		result.Summary.TotalNP++
		checkPort := func(ports []interface{}) bool { return false }
		_ = checkPort
		for _, rule := range np.Spec.Ingress {
			for _, p := range rule.Ports {
				if p.Port != nil && p.Port.StrVal != "" {
					result.Summary.WithNamedPort++
					result.Summary.ByPortName[p.Port.StrVal]++
				}
			}
		}
	}
	result.HealthScore = score
	gradeFromScore(&result.Grade, score)
	writeJSON(w, result)
}
