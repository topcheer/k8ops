package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v19.22 — Scalability & HA Dimension (Round 6)
// 1. Rollback Window Analyzer — deploy rollback readiness
// 2. DNS Resolution Scalability — CoreDNS capacity analysis
// 3. Connection Pool Exhaustion Risk — endpoint connection pressure
// ============================================================

// ---------------------------------------------------------------
// 1. Rollback Window Analyzer — deployment rollback readiness
// ---------------------------------------------------------------

type RollbackWindowResult1922 struct {
	ScannedAt       time.Time                 `json:"scannedAt"`
	HealthScore     int                       `json:"healthScore"`
	Grade           string                    `json:"grade"`
	Summary         RollbackWindowSummary1922 `json:"summary"`
	Deployments     []RollbackDeployEntry1922 `json:"deployments"`
	RiskyRollbacks  []RollbackRiskEntry1922   `json:"riskyRollbacks"`
	Recommendations []string                  `json:"recommendations"`
}

type RollbackWindowSummary1922 struct {
	TotalDeployments int `json:"totalDeployments"`
	WithHistory      int `json:"withHistory"`
	WithOldRev       int `json:"withOldRevision"`
	RollbackReady    int `json:"rollbackReady"`
	NotReady         int `json:"notReady"`
	MaxHistoryKept   int `json:"maxHistoryKept"`
}

type RollbackDeployEntry1922 struct {
	Name            string `json:"name"`
	Namespace       string `json:"namespace"`
	RevisionHistory int    `json:"revisionHistory"`
	Replicas        int    `json:"replicas"`
	CanRollback     bool   `json:"canRollback"`
}

type RollbackRiskEntry1922 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	RiskType  string `json:"riskType"`
	Detail    string `json:"detail"`
}

func (s *Server) handleRollbackWindow(w http.ResponseWriter, r *http.Request) {
	result := RollbackWindowResult1922{
		ScannedAt: time.Now(),
	}
	score := 100

	depList, err := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	for _, dep := range depList.Items {
		if isSystemNamespace(dep.Namespace) {
			continue
		}
		history := 10
		if dep.Spec.RevisionHistoryLimit != nil {
			history = int(*dep.Spec.RevisionHistoryLimit)
		}
		replicas := 1
		if dep.Spec.Replicas != nil {
			replicas = int(*dep.Spec.Replicas)
		}
		canRollback := history > 0 && dep.Status.UpdatedReplicas == int32(replicas)

		entry := RollbackDeployEntry1922{
			Name:            dep.Name,
			Namespace:       dep.Namespace,
			RevisionHistory: history,
			Replicas:        replicas,
			CanRollback:     canRollback,
		}
		result.Deployments = append(result.Deployments, entry)
		result.Summary.TotalDeployments++

		if history > 0 {
			result.Summary.WithHistory++
		}
		if history > result.Summary.MaxHistoryKept {
			result.Summary.MaxHistoryKept = history
		}
		if canRollback {
			result.Summary.RollbackReady++
		} else {
			result.Summary.NotReady++
			if history == 0 {
				result.RiskyRollbacks = append(result.RiskyRollbacks, RollbackRiskEntry1922{
					Name: dep.Name, Namespace: dep.Namespace,
					RiskType: "no-history",
					Detail:   "RevisionHistoryLimit is 0 — cannot rollback",
				})
				score -= 5
			}
		}

		// Check for single-replica deployments (can't do rolling update)
		if replicas == 1 {
			result.RiskyRollbacks = append(result.RiskyRollbacks, RollbackRiskEntry1922{
				Name: dep.Name, Namespace: dep.Namespace,
				RiskType: "single-replica",
				Detail:   "Single replica — rollback will cause downtime",
			})
			score -= 2
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.NotReady > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d deployments cannot rollback — check revisionHistoryLimit", result.Summary.NotReady))
	}
	riskyCount := len(result.RiskyRollbacks)
	if riskyCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d rollback risk items detected — increase replicas or history limit", riskyCount))
	}
	sort.Strings(result.Recommendations)

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. DNS Resolution Scalability — CoreDNS capacity analysis
// ---------------------------------------------------------------

type DNSScalabilityResult1922 struct {
	ScannedAt       time.Time                 `json:"scannedAt"`
	HealthScore     int                       `json:"healthScore"`
	Grade           string                    `json:"grade"`
	Summary         DNSScalabilitySummary1922 `json:"summary"`
	CoreDNSPods     []DNSPodEntry1922         `json:"coreDNSPods"`
	Config          DNSConfigEntry1922        `json:"config"`
	Bottlenecks     []DNSBottleneck1922       `json:"bottlenecks"`
	Recommendations []string                  `json:"recommendations"`
}

type DNSScalabilitySummary1922 struct {
	CoreDNSReplicas int     `json:"coreDNSReplicas"`
	TotalPods       int     `json:"totalPods"`
	PodsPerCoreDNS  float64 `json:"podsPerCoreDNS"`
	CoreDNSCPUReq   string  `json:"coreDNSCPUReq"`
	CoreDNSMemReq   string  `json:"coreDNSMemReq"`
	ClusterDomain   string  `json:"clusterDomain"`
	NDotsSetting    int     `json:"ndotsSetting"`
	BottleneckCount int     `json:"bottleneckCount"`
}

type DNSPodEntry1922 struct {
	Name   string `json:"name"`
	Node   string `json:"node"`
	Status string `json:"status"`
	Age    string `json:"age"`
	CPUReq string `json:"cpuReq"`
	MemReq string `json:"memReq"`
}

type DNSConfigEntry1922 struct {
	CoreDNSImage  string `json:"coreDNSImage"`
	HasAutoscaler bool   `json:"hasAutoscaler"`
	ForwardPolicy string `json:"forwardPolicy"`
	CacheTTL      int    `json:"cacheTTL"`
}

type DNSBottleneck1922 struct {
	Type     string `json:"type"`
	Severity string `json:"severity"`
	Detail   string `json:"detail"`
}

func (s *Server) handleDNSScalability(w http.ResponseWriter, r *http.Request) {
	result := DNSScalabilityResult1922{
		ScannedAt: time.Now(),
	}
	score := 100

	// Find CoreDNS pods
	podList, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	var dnsReplicas int
	var totalPods int
	var dnsImage string
	var cpuReq, memReq string

	for _, pod := range podList.Items {
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		totalPods++
		if strings.Contains(pod.Name, "coredns") || strings.Contains(pod.Name, "dns") {
			dnsReplicas++
			age := time.Since(pod.CreationTimestamp.Time).Hours() / 24
			for _, c := range pod.Spec.Containers {
				if !c.Resources.Requests.Cpu().IsZero() {
					cpuReq = c.Resources.Requests.Cpu().String()
				}
				if !c.Resources.Requests.Memory().IsZero() {
					memReq = c.Resources.Requests.Memory().String()
				}
				if dnsImage == "" {
					dnsImage = c.Image
				}
			}
			result.CoreDNSPods = append(result.CoreDNSPods, DNSPodEntry1922{
				Name:   pod.Name,
				Node:   pod.Spec.NodeName,
				Status: string(pod.Status.Phase),
				Age:    fmt.Sprintf("%.0fd", age),
				CPUReq: cpuReq,
				MemReq: memReq,
			})
		}
	}

	result.Summary.CoreDNSReplicas = dnsReplicas
	result.Summary.TotalPods = totalPods
	result.Summary.CoreDNSCPUReq = cpuReq
	result.Summary.CoreDNSMemReq = memReq
	result.Summary.ClusterDomain = "cluster.local"

	// ndots default
	result.Summary.NDotsSetting = 5 // Kubernetes default

	// Check autoscaler
	hasAutoscaler := false
	deployList, err := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{LabelSelector: "k8s-app=coredns-autoscaler"})
	if err == nil && len(deployList.Items) > 0 {
		hasAutoscaler = true
	}

	result.Config = DNSConfigEntry1922{
		CoreDNSImage:  dnsImage,
		HasAutoscaler: hasAutoscaler,
		ForwardPolicy: "proxy",
		CacheTTL:      30,
	}

	// Bottlenecks
	if dnsReplicas == 0 {
		result.Bottlenecks = append(result.Bottlenecks, DNSBottleneck1922{
			Type: "no-coredns", Severity: "critical",
			Detail: "No CoreDNS pods found running",
		})
		score -= 50
	}
	if dnsReplicas > 0 && totalPods > 0 {
		podsPerDNS := float64(totalPods) / float64(dnsReplicas)
		result.Summary.PodsPerCoreDNS = podsPerDNS
		if podsPerDNS > 500 {
			result.Bottlenecks = append(result.Bottlenecks, DNSBottleneck1922{
				Type: "dns-overload", Severity: "high",
				Detail: fmt.Sprintf("%.0f pods per CoreDNS replica (recommended <500)", podsPerDNS),
			})
			score -= 15
		} else if podsPerDNS > 250 {
			result.Bottlenecks = append(result.Bottlenecks, DNSBottleneck1922{
				Type: "dns-pressure", Severity: "medium",
				Detail: fmt.Sprintf("%.0f pods per CoreDNS replica — monitor query latency", podsPerDNS),
			})
			score -= 5
		}
	}
	if !hasAutoscaler {
		result.Bottlenecks = append(result.Bottlenecks, DNSBottleneck1922{
			Type: "no-autoscaler", Severity: "medium",
			Detail: "No CoreDNS autoscaler — DNS won't scale with cluster growth",
		})
		score -= 10
	}
	if result.Summary.NDotsSetting == 5 {
		result.Bottlenecks = append(result.Bottlenecks, DNSBottleneck1922{
			Type: "ndots-high", Severity: "low",
			Detail: "ndots:5 causes unnecessary DNS lookups for short names — consider dnsConfig overrides",
		})
	}
	if dnsReplicas == 1 {
		result.Bottlenecks = append(result.Bottlenecks, DNSBottleneck1922{
			Type: "single-replica", Severity: "high",
			Detail: "Single CoreDNS replica — SPOF for DNS resolution",
		})
		score -= 10
	}

	result.Summary.BottleneckCount = len(result.Bottlenecks)
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if !hasAutoscaler {
		result.Recommendations = append(result.Recommendations, "Deploy CoreDNS autoscaler (cluster-proportional-autoscaler) for automatic DNS scaling")
	}
	if dnsReplicas == 1 {
		result.Recommendations = append(result.Recommendations, "Increase CoreDNS to at least 2 replicas for HA")
	}
	if result.Summary.PodsPerCoreDNS > 250 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("Scale CoreDNS to handle %.0f pods/replica ratio", result.Summary.PodsPerCoreDNS))
	}
	sort.Strings(result.Recommendations)

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Connection Pool Exhaustion Risk — endpoint connection pressure
// ---------------------------------------------------------------

type ConnPoolResult1922 struct {
	ScannedAt       time.Time               `json:"scannedAt"`
	HealthScore     int                     `json:"healthScore"`
	Grade           string                  `json:"grade"`
	Summary         ConnPoolSummary1922     `json:"summary"`
	Endpoints       []ConnPoolEntry1922     `json:"endpoints"`
	HighRisk        []ConnPoolRiskEntry1922 `json:"highRisk"`
	Recommendations []string                `json:"recommendations"`
}

type ConnPoolSummary1922 struct {
	TotalEndpoints   int `json:"totalEndpoints"`
	HighFanOutEPs    int `json:"highFanOutEndpoints"`
	TotalConnections int `json:"totalConnections"`
	MaxConnections   int `json:"maxConnections"`
	SinglePodEPs     int `json:"singlePodEndpoints"`
	NoReadyEPs       int `json:"noReadyEndpoints"`
}

type ConnPoolEntry1922 struct {
	Name           string `json:"name"`
	Namespace      string `json:"namespace"`
	ReadyAddresses int    `json:"readyAddresses"`
	Port           int32  `json:"port"`
	EstConnections int    `json:"estConnections"`
	FanOut         int    `json:"fanOut"`
	IsAtRisk       bool   `json:"isAtRisk"`
}

type ConnPoolRiskEntry1922 struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	RiskType  string `json:"riskType"`
	Detail    string `json:"detail"`
}

func (s *Server) handleConnPoolExhaustion(w http.ResponseWriter, r *http.Request) {
	result := ConnPoolResult1922{
		ScannedAt: time.Now(),
	}
	score := 100

	// List endpoints to count connections per service
	epList, err := s.clientset.CoreV1().Endpoints("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	// Count pods per namespace to estimate fan-out
	podList, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}
	podsPerNS := make(map[string]int)
	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			podsPerNS[pod.Namespace]++
		}
	}

	var totalConns int
	for _, ep := range epList.Items {
		if isSystemNamespace(ep.Namespace) {
			continue
		}
		readyCount := 0
		var port int32
		for _, subset := range ep.Subsets {
			port = 0
			if len(subset.Ports) > 0 {
				port = subset.Ports[0].Port
			}
			readyCount += len(subset.Addresses)
		}

		// Estimate connections: each pod in namespace may connect
		fanOut := podsPerNS[ep.Namespace]
		estConns := fanOut
		if readyCount > 0 && fanOut > 0 {
			estConns = fanOut / readyCount
		}

		atRisk := false
		if estConns > 100 {
			atRisk = true
			result.HighRisk = append(result.HighRisk, ConnPoolRiskEntry1922{
				Name: ep.Name, Namespace: ep.Namespace,
				RiskType: "high-connections",
				Detail:   fmt.Sprintf("~%d connections per endpoint pod — pool exhaustion risk", estConns),
			})
		}
		if readyCount == 0 {
			atRisk = true
			result.HighRisk = append(result.HighRisk, ConnPoolRiskEntry1922{
				Name: ep.Name, Namespace: ep.Namespace,
				RiskType: "no-ready-endpoints",
				Detail:   "Endpoint has 0 ready addresses — service is down",
			})
			result.Summary.NoReadyEPs++
			score -= 5
		}
		if readyCount == 1 {
			result.Summary.SinglePodEPs++
			result.HighRisk = append(result.HighRisk, ConnPoolRiskEntry1922{
				Name: ep.Name, Namespace: ep.Namespace,
				RiskType: "single-pod",
				Detail:   "Endpoint backed by single pod — SPOF for connections",
			})
			score -= 2
		}

		result.Endpoints = append(result.Endpoints, ConnPoolEntry1922{
			Name:           ep.Name,
			Namespace:      ep.Namespace,
			ReadyAddresses: readyCount,
			Port:           port,
			EstConnections: estConns,
			FanOut:         fanOut,
			IsAtRisk:       atRisk,
		})
		totalConns += estConns
		if estConns > result.Summary.MaxConnections {
			result.Summary.MaxConnections = estConns
		}
		result.Summary.TotalEndpoints++
		if estConns > 100 {
			result.Summary.HighFanOutEPs++
		}
	}

	result.Summary.TotalConnections = totalConns

	// Score
	if result.Summary.HighFanOutEPs > 5 {
		score -= 10
	}
	if result.Summary.NoReadyEPs > 0 {
		score -= result.Summary.NoReadyEPs * 3
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.HighFanOutEPs > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d endpoints with high connection fan-out — add connection pooling or scale up", result.Summary.HighFanOutEPs))
	}
	if result.Summary.SinglePodEPs > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d endpoints backed by single pod — increase replicas for HA", result.Summary.SinglePodEPs))
	}
	if result.Summary.NoReadyEPs > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d endpoints with no ready addresses — investigate service health", result.Summary.NoReadyEPs))
	}
	sort.Strings(result.Recommendations)

	writeJSON(w, result)
}
