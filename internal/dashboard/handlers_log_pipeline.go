package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LogPipelineResult is the log aggregation & forwarding pipeline health audit.
type LogPipelineResult struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	Summary         LogPipelineSummary     `json:"summary"`
	Collectors      []LogPipelineCollector `json:"collectors"`
	Forwarders      []LogPipelineForwarder `json:"forwarders"`
	StorageBackends []LogPipelineStorage   `json:"storageBackends"`
	Gaps            []LogPipelineGap       `json:"gaps"`
	Recommendations []string               `json:"recommendations"`
	HealthScore     int                    `json:"healthScore"`
}

// LogPipelineSummary aggregates log pipeline statistics.
type LogPipelineSummary struct {
	TotalNamespaces     int  `json:"totalNamespaces"`
	WithLogAgent        int  `json:"withLogAgent"`    // namespaces with a log collector DS/DS-Set
	TotalCollectors     int  `json:"totalCollectors"` // DaemonSet/Deployment log collectors
	TotalForwarders     int  `json:"totalForwarders"` // ConfigMaps referencing log forwarding
	ReadyCollectors     int  `json:"readyCollectors"` // collectors with all pods ready
	UnhealthyCollectors int  `json:"unhealthyCollectors"`
	MissingNamespaces   int  `json:"missingNamespaces"` // namespaces without log collection
	HasFluentBit        bool `json:"hasFluentBit"`
	HasFluentd          bool `json:"hasFluentd"`
	HasVector           bool `json:"hasVector"`
	HasPromtail         bool `json:"hasPromtail"`
	HasFilebeat         bool `json:"hasFilebeat"`
}

// LogPipelineCollector describes a log collector daemonset/deployment.
type LogPipelineCollector struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"` // DaemonSet or Deployment
	Image     string `json:"image"`
	Desired   int    `json:"desired"`
	Ready     int    `json:"ready"`
	Available int    `json:"available"`
	Status    string `json:"status"`    // healthy, degraded, critical
	Collector string `json:"collector"` // fluentbit, fluentd, vector, promtail, filebeat, unknown
}

// LogPipelineForwarder describes a log forwarding configuration.
type LogPipelineForwarder struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	ConfigMap  string `json:"configMap"`
	Backend    string `json:"backend"` // elasticsearch, loki, s3, kafka, etc.
	HasOutput  bool   `json:"hasOutput"`
	HasFilters bool   `json:"hasFilters"`
	Status     string `json:"status"`
}

// LogPipelineStorage describes a log storage backend.
type LogPipelineStorage struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"` // StatefulSet, Deployment
	Image     string `json:"image"`
	Type      string `json:"type"` // elasticsearch, loki, kafka, etc.
	Ready     int    `json:"ready"`
	Desired   int    `json:"desired"`
	Status    string `json:"status"`
}

// LogPipelineGap describes a gap in log collection coverage.
type LogPipelineGap struct {
	Namespace string `json:"namespace"`
	PodCount  int    `json:"podCount"`
	Issue     string `json:"issue"`
	Severity  string `json:"severity"`
}

// handleLogPipeline audits log aggregation & forwarding pipeline health.
// GET /api/operations/log-pipeline
func (s *Server) handleLogPipeline(w http.ResponseWriter, r *http.Request) {
	result := LogPipelineResult{
		ScannedAt: time.Now(),
	}

	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	knownCollectors := map[string]string{
		"fluent-bit":            "fluentbit",
		"fluentbit":             "fluentbit",
		"fluentd":               "fluentd",
		"fluent":                "fluentd",
		"vector":                "vector",
		"promtail":              "promtail",
		"filebeat":              "filebeat",
		"logstash":              "logstash",
		"fluentd-elasticsearch": "fluentd",
	}

	knownBackends := map[string]string{
		"elasticsearch": "elasticsearch",
		"elastic":       "elasticsearch",
		"loki":          "loki",
		"kafka":         "kafka",
		"s3":            "s3",
		"gcs":           "gcs",
		"azureblob":     "azureblob",
		"splunk":        "splunk",
		"datadog":       "datadog",
		"cloudwatch":    "cloudwatch",
		"stackdriver":   "stackdriver",
	}

	logNamespaces := map[string]bool{
		"logging":         true,
		"log-aggregation": true,
		"logs":            true,
		"observability":   true,
		"monitoring":      true,
		"fluent-bit":      true,
		"fluentd":         true,
		"efk":             true,
		"elastic-system":  true,
		"loki":            true,
		"grafana":         true,
	}

	// 1. Find log collector DaemonSets/Deployments
	daemonsets, err := rc.clientset.AppsV1().DaemonSets("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, ds := range daemonsets.Items {
			collectorType := "unknown"
			imageName := ""
			if len(ds.Spec.Template.Spec.Containers) > 0 {
				imageName = ds.Spec.Template.Spec.Containers[0].Image
				for keyword, ct := range knownCollectors {
					if strings.Contains(strings.ToLower(imageName), keyword) || strings.Contains(strings.ToLower(ds.Name), keyword) {
						collectorType = ct
						break
					}
				}
			}

			if collectorType == "unknown" {
				continue
			}

			status := "healthy"
			if ds.Status.NumberReady < ds.Status.DesiredNumberScheduled {
				status = "degraded"
			}
			if ds.Status.NumberReady == 0 && ds.Status.DesiredNumberScheduled > 0 {
				status = "critical"
			}

			result.Collectors = append(result.Collectors, LogPipelineCollector{
				Name:      ds.Name,
				Namespace: ds.Namespace,
				Kind:      "DaemonSet",
				Image:     imageName,
				Desired:   int(ds.Status.DesiredNumberScheduled),
				Ready:     int(ds.Status.NumberReady),
				Available: int(ds.Status.NumberAvailable),
				Status:    status,
				Collector: collectorType,
			})

			result.Summary.TotalCollectors++
			if status == "healthy" {
				result.Summary.ReadyCollectors++
			} else {
				result.Summary.UnhealthyCollectors++
			}

			switch collectorType {
			case "fluentbit":
				result.Summary.HasFluentBit = true
			case "fluentd":
				result.Summary.HasFluentd = true
			case "vector":
				result.Summary.HasVector = true
			case "promtail":
				result.Summary.HasPromtail = true
			case "filebeat":
				result.Summary.HasFilebeat = true
			}
		}
	}

	deployments, err := rc.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, dep := range deployments.Items {
			collectorType := "unknown"
			imageName := ""
			if len(dep.Spec.Template.Spec.Containers) > 0 {
				imageName = dep.Spec.Template.Spec.Containers[0].Image
				for keyword, ct := range knownCollectors {
					if strings.Contains(strings.ToLower(imageName), keyword) || strings.Contains(strings.ToLower(dep.Name), keyword) {
						collectorType = ct
						break
					}
				}
			}

			if collectorType == "unknown" {
				continue
			}

			status := "healthy"
			if dep.Status.ReadyReplicas < *dep.Spec.Replicas {
				status = "degraded"
			}
			if dep.Status.ReadyReplicas == 0 && dep.Spec.Replicas != nil && *dep.Spec.Replicas > 0 {
				status = "critical"
			}

			desired := 0
			if dep.Spec.Replicas != nil {
				desired = int(*dep.Spec.Replicas)
			}

			result.Collectors = append(result.Collectors, LogPipelineCollector{
				Name:      dep.Name,
				Namespace: dep.Namespace,
				Kind:      "Deployment",
				Image:     imageName,
				Desired:   desired,
				Ready:     int(dep.Status.ReadyReplicas),
				Available: int(dep.Status.AvailableReplicas),
				Status:    status,
				Collector: collectorType,
			})

			result.Summary.TotalCollectors++
			if status == "healthy" {
				result.Summary.ReadyCollectors++
			} else {
				result.Summary.UnhealthyCollectors++
			}

			switch collectorType {
			case "fluentbit":
				result.Summary.HasFluentBit = true
			case "fluentd":
				result.Summary.HasFluentd = true
			case "vector":
				result.Summary.HasVector = true
			case "promtail":
				result.Summary.HasPromtail = true
			case "filebeat":
				result.Summary.HasFilebeat = true
			}
		}
	}

	// 2. Find log storage backends (StatefulSets/Deployments in logging namespaces)
	if err == nil {
		for _, dep := range deployments.Items {
			if !logNamespaces[dep.Namespace] {
				continue
			}
			imageName := ""
			if len(dep.Spec.Template.Spec.Containers) > 0 {
				imageName = dep.Spec.Template.Spec.Containers[0].Image
			}

			backendType := "unknown"
			for keyword, bt := range knownBackends {
				if strings.Contains(strings.ToLower(imageName), keyword) || strings.Contains(strings.ToLower(dep.Name), keyword) {
					backendType = bt
					break
				}
			}

			if backendType == "unknown" {
				continue
			}

			desired := 0
			if dep.Spec.Replicas != nil {
				desired = int(*dep.Spec.Replicas)
			}
			status := "healthy"
			if dep.Status.ReadyReplicas < int32(desired) {
				status = "degraded"
			}

			result.StorageBackends = append(result.StorageBackends, LogPipelineStorage{
				Name:      dep.Name,
				Namespace: dep.Namespace,
				Kind:      "Deployment",
				Image:     imageName,
				Type:      backendType,
				Ready:     int(dep.Status.ReadyReplicas),
				Desired:   desired,
				Status:    status,
			})
		}
	}

	statefulsets, err := rc.clientset.AppsV1().StatefulSets("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, sts := range statefulsets.Items {
			if !logNamespaces[sts.Namespace] {
				continue
			}
			imageName := ""
			if len(sts.Spec.Template.Spec.Containers) > 0 {
				imageName = sts.Spec.Template.Spec.Containers[0].Image
			}

			backendType := "unknown"
			for keyword, bt := range knownBackends {
				if strings.Contains(strings.ToLower(imageName), keyword) || strings.Contains(strings.ToLower(sts.Name), keyword) {
					backendType = bt
					break
				}
			}

			if backendType == "unknown" {
				continue
			}

			desired := 0
			if sts.Spec.Replicas != nil {
				desired = int(*sts.Spec.Replicas)
			}
			status := "healthy"
			if sts.Status.ReadyReplicas < int32(desired) {
				status = "degraded"
			}

			result.StorageBackends = append(result.StorageBackends, LogPipelineStorage{
				Name:      sts.Name,
				Namespace: sts.Namespace,
				Kind:      "StatefulSet",
				Image:     imageName,
				Type:      backendType,
				Ready:     int(sts.Status.ReadyReplicas),
				Desired:   desired,
				Status:    status,
			})
		}
	}

	// 3. Find ConfigMaps that look like log forwarding configs
	configmaps, err := rc.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, cm := range configmaps.Items {
			if !logNamespaces[cm.Namespace] && cm.Namespace != "kube-system" {
				continue
			}

			// Check if configmap has log forwarding config
			hasOutput := false
			hasFilters := false
			backendType := ""

			for _, data := range cm.Data {
				lowerData := strings.ToLower(data)
				for keyword, bt := range knownBackends {
					if strings.Contains(lowerData, keyword) {
						backendType = bt
						hasOutput = true
						break
					}
				}
				if strings.Contains(lowerData, "filter") || strings.Contains(lowerData, "match") || strings.Contains(lowerData, "regex") {
					hasFilters = true
				}
			}

			if !hasOutput && !hasFilters {
				continue
			}

			// Check if it's referenced by a known collector
			isForwarder := false
			for _, c := range result.Collectors {
				if c.Namespace == cm.Namespace && (strings.Contains(strings.ToLower(cm.Name), c.Collector) || strings.Contains(strings.ToLower(c.Name), strings.ToLower(cm.Name))) {
					isForwarder = true
					break
				}
			}

			if !isForwarder && !hasOutput {
				continue
			}

			status := "healthy"
			if !hasOutput {
				status = "warning"
			}

			result.Forwarders = append(result.Forwarders, LogPipelineForwarder{
				Name:       cm.Name,
				Namespace:  cm.Namespace,
				ConfigMap:  cm.Name,
				Backend:    backendType,
				HasOutput:  hasOutput,
				HasFilters: hasFilters,
				Status:     status,
			})
			result.Summary.TotalForwarders++
		}
	}

	// 4. Check namespace coverage
	namespaces, err := rc.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{})
	if err == nil {
		systemNamespaces := map[string]bool{
			"kube-system":     true,
			"kube-public":     true,
			"kube-node-lease": true,
		}

		// Determine which namespaces have log collectors
		nsWithCollector := make(map[string]bool)
		for _, c := range result.Collectors {
			if c.Kind == "DaemonSet" {
				// DaemonSet runs on all nodes, so it covers all namespaces
				nsWithCollector[c.Namespace] = true
			}
		}

		totalNS := 0
		for _, ns := range namespaces.Items {
			if systemNamespaces[ns.Name] {
				continue
			}
			if ns.Status.Phase != "Active" {
				continue
			}
			totalNS++

			// Check if any DaemonSet-based collector covers this namespace
			// DaemonSet collectors run on all nodes, so they cover all namespaces
			hasCoverage := len(result.Collectors) > 0

			if hasCoverage {
				result.Summary.WithLogAgent++
			} else {
				// Count pods in this namespace
				pods, err := rc.clientset.CoreV1().Pods(ns.Name).List(r.Context(), metav1.ListOptions{Limit: 1})
				if err == nil && len(pods.Items) > 0 {
					result.Gaps = append(result.Gaps, LogPipelineGap{
						Namespace: ns.Name,
						PodCount:  len(pods.Items),
						Issue:     "No log collector DaemonSet found",
						Severity:  "high",
					})
					result.Summary.MissingNamespaces++
				}
			}
		}
		result.Summary.TotalNamespaces = totalNS
	}

	// Sort results
	sort.Slice(result.Collectors, func(i, j int) bool {
		return result.Collectors[i].Status > result.Collectors[j].Status
	})
	sort.Slice(result.Gaps, func(i, j int) bool {
		return result.Gaps[i].Severity > result.Gaps[j].Severity
	})

	// Recommendations
	if result.Summary.TotalCollectors == 0 {
		result.Recommendations = append(result.Recommendations,
			"No log collector found. Install Fluent Bit, Vector, Promtail, or Filebeat as a DaemonSet for cluster-wide log collection")
	}
	if result.Summary.UnhealthyCollectors > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("Fix %d unhealthy log collectors (check pod logs and resource limits)", result.Summary.UnhealthyCollectors))
	}
	if result.Summary.MissingNamespaces > 0 {
		result.Recommendations = append(result.Recommendations,
			fmt.Sprintf("%d namespaces have no log collection coverage", result.Summary.MissingNamespaces))
	}
	if len(result.StorageBackends) == 0 && result.Summary.TotalCollectors > 0 {
		result.Recommendations = append(result.Recommendations,
			"Log collectors found but no storage backend detected. Ensure logs are forwarded to Elasticsearch, Loki, or another backend")
	}

	// Health score
	score := 100
	if result.Summary.TotalCollectors == 0 {
		score = 20
	} else {
		score -= result.Summary.UnhealthyCollectors * 10
		score -= result.Summary.MissingNamespaces * 5
		if len(result.StorageBackends) == 0 {
			score -= 20
		}
		if result.Summary.TotalForwarders == 0 {
			score -= 10
		}
		if score < 0 {
			score = 0
		}
	}
	result.HealthScore = score

	writeJSON(w, result)
}
