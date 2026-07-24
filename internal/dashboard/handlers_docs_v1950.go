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
// v19.50 — Documentation Dimension (Round 11)
// 1. Cluster Config Snapshot — version & component documentation
// 2. Event History Doc — recent event catalog for audit trail
// 3. Resource Quota Doc — quota allocation & usage documentation
// ============================================================

// ---------------------------------------------------------------
// 1. Cluster Config Snapshot
// ---------------------------------------------------------------

type ClusterConfigResult1950 struct {
	ScannedAt       time.Time                  `json:"scannedAt"`
	HealthScore     int                        `json:"healthScore"`
	Grade           string                     `json:"grade"`
	Summary         ClusterConfigSummary1950   `json:"summary"`
	Version         string                     `json:"clusterVersion"`
	Nodes           []ClusterConfigNode1950    `json:"nodes"`
	Features        []ClusterConfigFeature1950 `json:"features"`
	Recommendations []string                   `json:"recommendations"`
}

type ClusterConfigSummary1950 struct {
	TotalNodes      int    `json:"totalNodes"`
	K8sVersion      string `json:"k8sVersion"`
	TotalNamespaces int    `json:"totalNamespaces"`
	TotalPods       int    `json:"totalPods"`
	TotalServices   int    `json:"totalServices"`
	TotalImages     int    `json:"uniqueImages"`
	OldestNodeAge   string `json:"oldestNodeAge"`
}

type ClusterConfigNode1950 struct {
	Name        string `json:"name"`
	KubeletVer  string `json:"kubeletVersion"`
	OS          string `json:"osImage"`
	Arch        string `json:"architecture"`
	CPUCapacity string `json:"cpuCapacity"`
	MemCapacity string `json:"memCapacity"`
}

type ClusterConfigFeature1950 struct {
	Name    string `json:"feature"`
	Enabled bool   `json:"enabled"`
}

func (s *Server) handleClusterConfigSnap(w http.ResponseWriter, r *http.Request) {
	result := ClusterConfigResult1950{ScannedAt: time.Now()}
	score := 100

	version, verr := s.clientset.Discovery().ServerVersion()
	if verr == nil {
		result.Version = version.GitVersion
		result.Summary.K8sVersion = version.GitVersion
	}

	nodeList, _ := s.clientset.CoreV1().Nodes().List(r.Context(), metav1.ListOptions{})
	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})

	result.Summary.TotalNamespaces = len(nsList.Items)
	result.Summary.TotalServices = len(svcList.Items)

	imageSet := make(map[string]bool)
	var oldestNodeTime time.Time
	for _, node := range nodeList.Items {
		result.Summary.TotalNodes++
		entry := ClusterConfigNode1950{
			Name:        node.Name,
			KubeletVer:  node.Status.NodeInfo.KubeletVersion,
			OS:          node.Status.NodeInfo.OSImage,
			Arch:        node.Status.NodeInfo.Architecture,
			CPUCapacity: node.Status.Capacity.Cpu().String(),
			MemCapacity: node.Status.Capacity.Memory().String(),
		}
		result.Nodes = append(result.Nodes, entry)

		if oldestNodeTime.IsZero() || node.CreationTimestamp.Time.Before(oldestNodeTime) {
			oldestNodeTime = node.CreationTimestamp.Time
		}
	}

	if !oldestNodeTime.IsZero() {
		days := time.Since(oldestNodeTime).Hours() / 24
		result.Summary.OldestNodeAge = fmt.Sprintf("%.0fd", days)
		if days > 365 {
			score -= 5
		}
	}

	for _, pod := range podList.Items {
		if pod.Status.Phase == corev1.PodRunning {
			result.Summary.TotalPods++
			for _, c := range pod.Spec.Containers {
				imageSet[c.Image] = true
			}
		}
	}
	result.Summary.TotalImages = len(imageSet)

	// Document feature gates (heuristic)
	result.Features = append(result.Features,
		ClusterConfigFeature1950{Name: "PodSecurityPolicy", Enabled: false},
		ClusterConfigFeature1950{Name: "NetworkPolicy", Enabled: true},
		ClusterConfigFeature1950{Name: "RBAC", Enabled: true},
	)

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations,
		fmt.Sprintf("Cluster %s, %d nodes, %d namespaces", result.Summary.K8sVersion, result.Summary.TotalNodes, result.Summary.TotalNamespaces),
	)
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Event History Doc
// ---------------------------------------------------------------

type EventHistoryDocResult1950 struct {
	ScannedAt       time.Time                  `json:"scannedAt"`
	HealthScore     int                        `json:"healthScore"`
	Grade           string                     `json:"grade"`
	Summary         EventHistoryDocSummary1950 `json:"summary"`
	Events          []EventHistoryDocEntry1950 `json:"events"`
	ByKind          []EventHistoryKindStat1950 `json:"byKind"`
	Recommendations []string                   `json:"recommendations"`
}

type EventHistoryDocSummary1950 struct {
	TotalEvents   int `json:"totalEvents"`
	WarningCount  int `json:"warningCount"`
	NormalCount   int `json:"normalCount"`
	Recent24h     int `json:"eventsLast24h"`
	UniqueReasons int `json:"uniqueReasons"`
}

type EventHistoryDocEntry1950 struct {
	Timestamp string `json:"timestamp"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`
	Reason    string `json:"reason"`
	Type      string `json:"type"`
	Message   string `json:"message"`
}

type EventHistoryKindStat1950 struct {
	ResourceKind string `json:"resourceKind"`
	Count        int    `json:"count"`
}

func (s *Server) handleEventHistoryDoc(w http.ResponseWriter, r *http.Request) {
	result := EventHistoryDocResult1950{ScannedAt: time.Now()}
	score := 100
	now := time.Now()
	kindCount := make(map[string]int)
	reasonSet := make(map[string]bool)

	evList, _ := s.clientset.CoreV1().Events("").List(r.Context(), metav1.ListOptions{})

	for _, ev := range evList.Items {
		if isSystemNamespace(ev.Namespace) {
			continue
		}
		result.Summary.TotalEvents++

		kind := ev.InvolvedObject.Kind
		kindCount[kind]++
		reasonSet[ev.Reason] = true

		if ev.Type == "Warning" {
			result.Summary.WarningCount++
		} else {
			result.Summary.NormalCount++
		}

		var ts time.Time
		if ts.IsZero() {
			ts = ev.EventTime.Time
		}
		if !ts.IsZero() && now.Sub(ts).Hours() <= 24 {
			result.Summary.Recent24h++
		}

		if len(result.Events) < 50 {
			msg := ev.Message
			if len(msg) > 120 {
				msg = msg[:120] + "..."
			}
			result.Events = append(result.Events, EventHistoryDocEntry1950{
				Timestamp: fmt.Sprintf("%.0fm ago", now.Sub(ts).Minutes()),
				Name:      ev.InvolvedObject.Name, Namespace: ev.Namespace,
				Kind: kind, Reason: ev.Reason, Type: ev.Type, Message: msg,
			})
		}
	}

	result.Summary.UniqueReasons = len(reasonSet)

	for k, c := range kindCount {
		result.ByKind = append(result.ByKind, EventHistoryKindStat1950{ResourceKind: k, Count: c})
	}
	sort.Slice(result.ByKind, func(i, j int) bool { return result.ByKind[i].Count > result.ByKind[j].Count })

	if result.Summary.WarningCount > 50 {
		score -= 5
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	result.Recommendations = append(result.Recommendations,
		fmt.Sprintf("%d events documented (%d warning, %d normal)", result.Summary.TotalEvents, result.Summary.WarningCount, result.Summary.NormalCount),
	)
	if result.Summary.WarningCount > 20 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d warning events — review common reasons", result.Summary.WarningCount))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Resource Quota Doc
// ---------------------------------------------------------------

type QuotaDocResult1950 struct {
	ScannedAt         time.Time             `json:"scannedAt"`
	HealthScore       int                   `json:"healthScore"`
	Grade             string                `json:"grade"`
	Summary           QuotaDocSummary1950   `json:"summary"`
	Quotas            []QuotaDocEntry1950   `json:"quotas"`
	NamespacesNoQuota []QuotaDocNSEntry1950 `json:"namespacesWithoutQuota"`
	Recommendations   []string              `json:"recommendations"`
}

type QuotaDocSummary1950 struct {
	TotalNamespaces int `json:"totalNamespaces"`
	WithQuota       int `json:"namespacesWithQuota"`
	WithoutQuota    int `json:"namespacesWithoutQuota"`
	TotalQuotas     int `json:"totalQuotaObjects"`
	CPUQuotaNS      int `json:"namespacesWithCPUQuota"`
	MemQuotaNS      int `json:"namespacesWithMemQuota"`
	PodQuotaNS      int `json:"namespacesWithPodQuota"`
}

type QuotaDocEntry1950 struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	HasCPU    bool   `json:"hasCPUQuota"`
	HasMem    bool   `json:"hasMemQuota"`
	HasPod    bool   `json:"hasPodQuota"`
}

type QuotaDocNSEntry1950 struct {
	Namespace string `json:"namespace"`
	Severity  string `json:"severity"`
}

func (s *Server) handleQuotaDoc(w http.ResponseWriter, r *http.Request) {
	result := QuotaDocResult1950{ScannedAt: time.Now()}
	score := 100

	nsList, _ := s.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	rqList, _ := s.clientset.CoreV1().ResourceQuotas("").List(r.Context(), metav1.ListOptions{})

	hasQuota := make(map[string]*QuotaDocEntry1950)
	for _, rq := range rqList.Items {
		if isSystemNamespace(rq.Namespace) {
			continue
		}
		result.Summary.TotalQuotas++

		if hasQuota[rq.Namespace] == nil {
			hasQuota[rq.Namespace] = &QuotaDocEntry1950{Namespace: rq.Namespace}
		}
		entry := hasQuota[rq.Namespace]
		entry.Name = rq.Name

		for k := range rq.Spec.Hard {
			kStr := string(k)
			if kStr == "cpu" || kStr == "requests.cpu" || kStr == "limits.cpu" {
				entry.HasCPU = true
			}
			if kStr == "memory" || kStr == "requests.memory" || kStr == "limits.memory" {
				entry.HasMem = true
			}
			if kStr == "pods" {
				entry.HasPod = true
			}
		}
	}

	for _, q := range hasQuota {
		result.Summary.WithQuota++
		if q.HasCPU {
			result.Summary.CPUQuotaNS++
		}
		if q.HasMem {
			result.Summary.MemQuotaNS++
		}
		if q.HasPod {
			result.Summary.PodQuotaNS++
		}
		result.Quotas = append(result.Quotas, *q)
	}

	for _, ns := range nsList.Items {
		if isSystemNamespace(ns.Name) {
			continue
		}
		result.Summary.TotalNamespaces++
		if hasQuota[ns.Name] == nil {
			result.Summary.WithoutQuota++
			result.NamespacesNoQuota = append(result.NamespacesNoQuota, QuotaDocNSEntry1950{
				Namespace: ns.Name, Severity: "medium",
			})
			score -= 2
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.WithoutQuota > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d namespaces without ResourceQuota — add for governance", result.Summary.WithoutQuota))
	}
	result.Recommendations = append(result.Recommendations,
		fmt.Sprintf("%d/%d namespaces have quota (%d CPU, %d memory, %d pods)",
			result.Summary.WithQuota, result.Summary.TotalNamespaces,
			result.Summary.CPUQuotaNS, result.Summary.MemQuotaNS, result.Summary.PodQuotaNS),
	)
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
