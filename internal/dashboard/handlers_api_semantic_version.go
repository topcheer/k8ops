package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// APISemanticVersionResult tracks Kubernetes API version semantics across
// all resources, identifying deprecated APIs, breaking changes, and
// providing migration timelines.
type APISemanticVersionResult struct {
	ScannedAt       time.Time        `json:"scannedAt"`
	Summary         APISemVerSummary `json:"summary"`
	ByResource      []APISemVerEntry `json:"byResource"`
	DeprecatedAPIs  []APISemVerEntry `json:"deprecatedAPIs"`
	BreakingChanges []APISemVerEntry `json:"breakingChanges"`
	MaturityScore   int              `json:"maturityScore"`
	Grade           string           `json:"grade"`
	Recommendations []string         `json:"recommendations"`
}

type APISemVerSummary struct {
	TotalResources    int            `json:"totalResources"`
	GAResources       int            `json:"gaResources"`
	BetaResources     int            `json:"betaResources"`
	AlphaResources    int            `json:"alphaResources"`
	DeprecatedCount   int            `json:"deprecatedCount"`
	RemovalInNext     int            `json:"removalInNextVersion"`
	MajorVersionSplit map[string]int `json:"majorVersionSplit"`
}

type APISemVerEntry struct {
	Resource    string   `json:"resource"`
	APIVersion  string   `json:"apiVersion"`
	Group       string   `json:"group"`
	Version     string   `json:"version"`
	Maturity    string   `json:"maturity"`
	Count       int      `json:"count"`
	Deprecated  bool     `json:"deprecated"`
	RemovedIn   string   `json:"removedIn"`
	Replacement string   `json:"replacement"`
	Severity    string   `json:"severity"`
	Namespaces  []string `json:"namespaces"`
}

// Known deprecated APIs in Kubernetes
var knownDeprecatedAPIs = map[string]APISemVerEntry{
	"extensions/v1beta1/Ingress": {
		RemovedIn:   "k8s 1.22",
		Replacement: "networking.k8s.io/v1/Ingress",
		Severity:    "critical",
	},
	"apps/v1beta1/Deployment": {
		RemovedIn:   "k8s 1.16",
		Replacement: "apps/v1/Deployment",
		Severity:    "critical",
	},
	"apps/v1beta2/Deployment": {
		RemovedIn:   "k8s 1.16",
		Replacement: "apps/v1/Deployment",
		Severity:    "critical",
	},
	"apps/v1beta1/StatefulSet": {
		RemovedIn:   "k8s 1.16",
		Replacement: "apps/v1/StatefulSet",
		Severity:    "critical",
	},
	"policy/v1beta1/PodDisruptionBudget": {
		RemovedIn:   "k8s 1.25",
		Replacement: "policy/v1/PodDisruptionBudget",
		Severity:    "high",
	},
	"networking.k8s.io/v1beta1/Ingress": {
		RemovedIn:   "k8s 1.22",
		Replacement: "networking.k8s.io/v1/Ingress",
		Severity:    "critical",
	},
	"autoscaling/v2beta1/HorizontalPodAutoscaler": {
		RemovedIn:   "k8s 1.25",
		Replacement: "autoscaling/v2/HorizontalPodAutoscaler",
		Severity:    "high",
	},
	"autoscaling/v2beta2/HorizontalPodAutoscaler": {
		RemovedIn:   "k8s 1.26",
		Replacement: "autoscaling/v2/HorizontalPodAutoscaler",
		Severity:    "high",
	},
	"batch/v1beta1/CronJob": {
		RemovedIn:   "k8s 1.25",
		Replacement: "batch/v1/CronJob",
		Severity:    "high",
	},
}

// handleAPISemanticVersion handles GET /api/docs/api-semantic-version
func (s *Server) handleAPISemanticVersion(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := APISemanticVersionResult{
		ScannedAt: time.Now(),
	}

	// Collect resources from multiple API types
	apiMap := make(map[string]*APISemVerEntry)

	addResource := func(apiVersion, kind, namespace string) {
		if isSystemNamespace(namespace) {
			return
		}
		key := apiVersion + "/" + kind
		if _, ok := apiMap[key]; !ok {
			group := ""
			version := apiVersion
			if idx := strings.Index(apiVersion, "/"); idx > 0 {
				group = apiVersion[:idx]
				version = apiVersion[idx+1:]
			}

			maturity := "ga"
			switch {
			case strings.Contains(version, "alpha"):
				maturity = "alpha"
			case strings.Contains(version, "beta"):
				maturity = "beta"
			}

			entry := &APISemVerEntry{
				Resource:   kind,
				APIVersion: apiVersion,
				Group:      group,
				Version:    version,
				Maturity:   maturity,
				Namespaces: []string{},
			}

			// Check if deprecated
			if dep, ok := knownDeprecatedAPIs[key]; ok {
				entry.Deprecated = true
				entry.RemovedIn = dep.RemovedIn
				entry.Replacement = dep.Replacement
				entry.Severity = dep.Severity
			}

			apiMap[key] = entry
		}
		apiMap[key].Count++
		apiMap[key].Namespaces = appendUniqueSecretStr(apiMap[key].Namespaces, namespace)
	}

	// Scan Deployments
	deployments, _ := rc.clientset.AppsV1().Deployments("").List(ctx, metav1.ListOptions{})
	for _, d := range deployments.Items {
		addResource("apps/v1", "Deployment", d.Namespace)
	}

	// Scan StatefulSets
	sts, _ := rc.clientset.AppsV1().StatefulSets("").List(ctx, metav1.ListOptions{})
	for _, s := range sts.Items {
		addResource("apps/v1", "StatefulSet", s.Namespace)
	}

	// Scan Services
	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	for _, svc := range services.Items {
		addResource("v1", "Service", svc.Namespace)
	}

	// Scan ConfigMaps
	cms, _ := rc.clientset.CoreV1().ConfigMaps("").List(ctx, metav1.ListOptions{})
	for _, cm := range cms.Items {
		addResource("v1", "ConfigMap", cm.Namespace)
	}

	// Scan Ingresses
	ingresses, _ := rc.clientset.NetworkingV1().Ingresses("").List(ctx, metav1.ListOptions{})
	for _, ing := range ingresses.Items {
		addResource("networking.k8s.io/v1", "Ingress", ing.Namespace)
	}

	// Scan HPAs
	hpas, _ := rc.clientset.AutoscalingV2().HorizontalPodAutoscalers("").List(ctx, metav1.ListOptions{})
	for _, hpa := range hpas.Items {
		addResource("autoscaling/v2", "HorizontalPodAutoscaler", hpa.Namespace)
	}

	// Scan PDBs
	pdbs, _ := rc.clientset.PolicyV1().PodDisruptionBudgets("").List(ctx, metav1.ListOptions{})
	for _, pdb := range pdbs.Items {
		addResource("policy/v1", "PodDisruptionBudget", pdb.Namespace)
	}

	// Scan CronJobs
	cronjobs, _ := rc.clientset.BatchV1().CronJobs("").List(ctx, metav1.ListOptions{})
	for _, cj := range cronjobs.Items {
		addResource("batch/v1", "CronJob", cj.Namespace)
	}

	// Build result
	result.Summary.MajorVersionSplit = make(map[string]int)
	var entries []APISemVerEntry
	for _, e := range apiMap {
		result.Summary.TotalResources++
		result.Summary.MajorVersionSplit[e.Maturity]++

		switch e.Maturity {
		case "ga":
			result.Summary.GAResources++
		case "beta":
			result.Summary.BetaResources++
		case "alpha":
			result.Summary.AlphaResources++
		}

		if e.Deprecated {
			result.Summary.DeprecatedCount++
			result.Summary.RemovalInNext++
			result.DeprecatedAPIs = append(result.DeprecatedAPIs, *e)
			if e.Severity == "critical" {
				result.BreakingChanges = append(result.BreakingChanges, *e)
			}
		}

		entries = append(entries, *e)
	}

	// Sort by deprecated first, then by count
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Deprecated != entries[j].Deprecated {
			return entries[i].Deprecated
		}
		return entries[i].Count > entries[j].Count
	})
	result.ByResource = entries

	// Maturity score: ratio of GA resources
	if result.Summary.TotalResources > 0 {
		result.MaturityScore = result.Summary.GAResources * 100 / result.Summary.TotalResources
	}

	switch {
	case result.MaturityScore >= 90:
		result.Grade = "A"
	case result.MaturityScore >= 75:
		result.Grade = "B"
	case result.MaturityScore >= 50:
		result.Grade = "C"
	case result.MaturityScore >= 25:
		result.Grade = "D"
	default:
		result.Grade = "F"
	}

	result.Recommendations = buildAPISemVerRecs(&result)
	writeJSON(w, result)
}

func buildAPISemVerRecs(r *APISemanticVersionResult) []string {
	recs := []string{
		fmt.Sprintf("API 语义版本: %d 资源, %d GA / %d Beta / %d Alpha", r.Summary.TotalResources, r.Summary.GAResources, r.Summary.BetaResources, r.Summary.AlphaResources),
	}
	if r.Summary.DeprecatedCount > 0 {
		recs = append(recs, fmt.Sprintf("警告: %d 个废弃 API 正在使用中", r.Summary.DeprecatedCount))
	}
	if len(r.BreakingChanges) > 0 {
		recs = append(recs, fmt.Sprintf("紧急: %d 个 API 在升级后将导致破坏性变更", len(r.BreakingChanges)))
	}
	if r.Summary.RemovalInNext > 0 {
		recs = append(recs, fmt.Sprintf("%d 个 API 在下一个 K8s 版本中将被移除", r.Summary.RemovalInNext))
	}
	if r.MaturityScore < 80 {
		recs = append(recs, "建议: 迁移所有废弃 API 到稳定版本, 使用 'kubectl convert' 自动转换")
	}
	return recs
}
