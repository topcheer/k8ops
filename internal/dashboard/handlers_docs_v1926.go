package dashboard

import (
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ============================================================
// v19.26 — Documentation Dimension (Round 7)
// 1. Naming Convention Audit — resource naming compliance
// 2. Environment Variable Catalog — env var inventory & conflicts
// 3. Annotation Inventory — metadata annotation governance catalog
// ============================================================

// ---------------------------------------------------------------
// 1. Naming Convention Audit — resource naming compliance
// ---------------------------------------------------------------

type NamingAuditResult1926 struct {
	ScannedAt       time.Time              `json:"scannedAt"`
	HealthScore     int                    `json:"healthScore"`
	Grade           string                 `json:"grade"`
	Summary         NamingAuditSummary1926 `json:"summary"`
	Violations      []NamingViolation1926  `json:"violations"`
	ByResourceType  []NamingTypeStat1926   `json:"byResourceType"`
	Recommendations []string               `json:"recommendations"`
}

type NamingAuditSummary1926 struct {
	TotalResources  int     `json:"totalResources"`
	CompliantCount  int     `json:"compliantCount"`
	ViolationCount  int     `json:"violationCount"`
	ComplianceRate  float64 `json:"complianceRate"`
	UppercaseCount  int     `json:"uppercaseCount"`
	UnderscoreCount int     `json:"underscoreCount"`
	TooLongCount    int     `json:"tooLongCount"`
	WithSpecialChar int     `json:"withSpecialChar"`
}

type NamingViolation1926 struct {
	Name       string `json:"name"`
	Namespace  string `json:"namespace"`
	Kind       string `json:"kind"`
	Violation  string `json:"violation"`
	Severity   string `json:"severity"`
	Suggestion string `json:"suggestion"`
}

type NamingTypeStat1926 struct {
	Kind       string `json:"kind"`
	Total      int    `json:"total"`
	Violations int    `json:"violations"`
}

var dns1123Pattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)

func (s *Server) handleNamingAudit(w http.ResponseWriter, r *http.Request) {
	result := NamingAuditResult1926{
		ScannedAt: time.Now(),
	}
	score := 100

	typeStats := make(map[string]*NamingTypeStat1926)
	addResource := func(name, ns, kind string) {
		result.Summary.TotalResources++
		ts, exists := typeStats[kind]
		if !exists {
			ts = &NamingTypeStat1926{Kind: kind}
			typeStats[kind] = ts
		}
		ts.Total++

		violations := make([]string, 0)

		// Check DNS-1123 compliance
		if !dns1123Pattern.MatchString(name) {
			violations = append(violations, "Invalid DNS-1123 label")
			result.Summary.WithSpecialChar++
		}

		// Check uppercase
		if name != strings.ToLower(name) {
			violations = append(violations, "Contains uppercase letters")
			result.Summary.UppercaseCount++
		}

		// Check underscores
		if strings.Contains(name, "_") {
			violations = append(violations, "Contains underscores (use hyphens)")
			result.Summary.UnderscoreCount++
		}

		// Check length
		if len(name) > 63 {
			violations = append(violations, "Name exceeds 63 characters")
			result.Summary.TooLongCount++
		}

		// Check for reserved prefixes
		if strings.HasPrefix(name, "kube-") || strings.HasPrefix(name, "system-") {
			if ns != "kube-system" && ns != "k8ops-system" {
				violations = append(violations, "Uses reserved system prefix (kube-/system-)")
			}
		}

		if len(violations) > 0 {
			suggestion := strings.ToLower(strings.ReplaceAll(name, "_", "-"))
			if len(suggestion) > 63 {
				suggestion = suggestion[:60] + "..."
			}
			severity := "low"
			if len(violations) > 1 {
				severity = "medium"
			}
			result.Violations = append(result.Violations, NamingViolation1926{
				Name: name, Namespace: ns, Kind: kind,
				Violation:  strings.Join(violations, "; "),
				Severity:   severity,
				Suggestion: suggestion,
			})
			ts.Violations++
			result.Summary.ViolationCount++
		} else {
			result.Summary.CompliantCount++
		}
	}

	// Deployments
	depList, err := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, d := range depList.Items {
			if isSystemNamespace(d.Namespace) {
				continue
			}
			addResource(d.Name, d.Namespace, "Deployment")
		}
	}

	// Services
	svcList, err := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, sv := range svcList.Items {
			if isSystemNamespace(sv.Namespace) {
				continue
			}
			addResource(sv.Name, sv.Namespace, "Service")
		}
	}

	// ConfigMaps
	cmList, err := s.clientset.CoreV1().ConfigMaps("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, cm := range cmList.Items {
			if isSystemNamespace(cm.Namespace) {
				continue
			}
			addResource(cm.Name, cm.Namespace, "ConfigMap")
		}
	}

	// Secrets
	secList, err := s.clientset.CoreV1().Secrets("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, sc := range secList.Items {
			if isSystemNamespace(sc.Namespace) {
				continue
			}
			addResource(sc.Name, sc.Namespace, "Secret")
		}
	}

	for _, ts := range typeStats {
		result.ByResourceType = append(result.ByResourceType, *ts)
	}

	// Score
	if result.Summary.TotalResources > 0 {
		result.Summary.ComplianceRate = float64(result.Summary.CompliantCount) * 100 / float64(result.Summary.TotalResources)
	}
	if result.Summary.ViolationCount > 10 {
		score -= 15
	}
	if result.Summary.UppercaseCount > 5 {
		score -= 5
	}
	if result.Summary.UnderscoreCount > 5 {
		score -= 5
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.ViolationCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d naming violations — standardize on lowercase-hyphenated names", result.Summary.ViolationCount))
	}
	if result.Summary.UnderscoreCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d resources use underscores — Kubernetes prefers hyphens", result.Summary.UnderscoreCount))
	}
	sort.Strings(result.Recommendations)

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 2. Environment Variable Catalog — env var inventory & conflicts
// ---------------------------------------------------------------

type EnvVarCatalogResult1926 struct {
	ScannedAt       time.Time                `json:"scannedAt"`
	HealthScore     int                      `json:"healthScore"`
	Grade           string                   `json:"grade"`
	Summary         EnvVarCatalogSummary1926 `json:"summary"`
	Variables       []EnvVarEntry1926        `json:"variables"`
	Conflicts       []EnvVarConflict1926     `json:"conflicts"`
	SensitiveVars   []EnvVarSensitive1926    `json:"sensitiveVars"`
	Recommendations []string                 `json:"recommendations"`
}

type EnvVarCatalogSummary1926 struct {
	TotalEnvVars   int `json:"totalEnvVars"`
	UniqueEnvVars  int `json:"uniqueEnvVars"`
	FromSecretRef  int `json:"fromSecretRef"`
	FromConfigMap  int `json:"fromConfigMap"`
	DirectValues   int `json:"directValues"`
	SensitiveCount int `json:"sensitiveCount"`
	ConflictCount  int `json:"conflictCount"`
	WorkloadCount  int `json:"workloadCount"`
}

type EnvVarEntry1926 struct {
	Name        string   `json:"name"`
	UsageCount  int      `json:"usageCount"`
	Source      string   `json:"source"`
	Workloads   []string `json:"workloads"`
	IsSensitive bool     `json:"isSensitive"`
}

type EnvVarConflict1926 struct {
	EnvVar    string   `json:"envVar"`
	Values    []string `json:"values"`
	Workloads []string `json:"workloads"`
}

type EnvVarSensitive1926 struct {
	EnvVar   string `json:"envVar"`
	Workload string `json:"workload"`
	Reason   string `json:"reason"`
}

var sensitiveEnvPatterns = []string{
	"PASSWORD", "PASSWD", "SECRET", "TOKEN", "API_KEY", "APIKEY",
	"PRIVATE_KEY", "CREDENTIAL", "CRED", "ACCESS_KEY", "AUTH",
}

func (s *Server) handleEnvVarCatalog(w http.ResponseWriter, r *http.Request) {
	result := EnvVarCatalogResult1926{
		ScannedAt: time.Now(),
	}
	score := 100

	podList, err := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})
	if err != nil {
		writeJSON(w, result)
		return
	}

	type varInfo struct {
		count     int
		workloads map[string]bool
		values    map[string]bool // unique direct values
		sources   map[string]bool // secret, configmap, direct
		isSens    bool
	}
	varMap := make(map[string]*varInfo)

	for _, pod := range podList.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}
		appName := pod.Labels["app"]
		if appName == "" {
			appName = pod.Labels["app.kubernetes.io/name"]
		}
		if appName == "" {
			appName = pod.Name
		}
		result.Summary.WorkloadCount++

		for _, c := range pod.Spec.Containers {
			for _, ev := range c.Env {
				info, exists := varMap[ev.Name]
				if !exists {
					info = &varInfo{
						workloads: make(map[string]bool),
						values:    make(map[string]bool),
						sources:   make(map[string]bool),
					}
					// Check if sensitive
					nameUpper := strings.ToUpper(ev.Name)
					for _, pat := range sensitiveEnvPatterns {
						if strings.Contains(nameUpper, pat) {
							info.isSens = true
							break
						}
					}
					varMap[ev.Name] = info
				}
				info.count++
				info.workloads[appName] = true

				source := "direct"
				if ev.ValueFrom != nil {
					if ev.ValueFrom.SecretKeyRef != nil {
						source = "secret"
						result.Summary.FromSecretRef++
					} else if ev.ValueFrom.ConfigMapKeyRef != nil {
						source = "configmap"
						result.Summary.FromConfigMap++
					}
				} else {
					info.values[ev.Value] = true
					result.Summary.DirectValues++
				}
				info.sources[source] = true

				// Flag sensitive direct values
				if info.isSens && ev.Value != "" && ev.ValueFrom == nil {
					result.SensitiveVars = append(result.SensitiveVars, EnvVarSensitive1926{
						EnvVar: ev.Name, Workload: appName,
						Reason: "Sensitive env var with hardcoded value — move to Secret",
					})
					score -= 3
				}
			}
		}
	}

	// Build entries
	for name, info := range varMap {
		wlList := make([]string, 0, len(info.workloads))
		for wl := range info.workloads {
			wlList = append(wlList, wl)
		}
		sourceList := make([]string, 0, len(info.sources))
		for src := range info.sources {
			sourceList = append(sourceList, src)
		}

		result.Variables = append(result.Variables, EnvVarEntry1926{
			Name:        name,
			UsageCount:  info.count,
			Source:      strings.Join(sourceList, "+"),
			Workloads:   wlList,
			IsSensitive: info.isSens,
		})
		result.Summary.TotalEnvVars += info.count
		result.Summary.UniqueEnvVars++

		if info.isSens {
			result.Summary.SensitiveCount++
		}

		// Detect conflicts (same env var name with different direct values)
		if info.values != nil && len(info.values) > 1 {
			values := make([]string, 0)
			for v := range info.values {
				if len(v) > 20 {
					v = v[:20] + "..."
				}
				values = append(values, v)
			}
			result.Conflicts = append(result.Conflicts, EnvVarConflict1926{
				EnvVar: name, Values: values, Workloads: wlList,
			})
			result.Summary.ConflictCount++
			score -= 2
		}
	}

	sort.Slice(result.Variables, func(i, j int) bool {
		return result.Variables[i].UsageCount > result.Variables[j].UsageCount
	})

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.SensitiveCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d sensitive env vars detected — migrate to Secret references", result.Summary.SensitiveCount))
	}
	if result.Summary.ConflictCount > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d env var conflicts (same name, different values) — standardize configuration", result.Summary.ConflictCount))
	}
	if result.Summary.DirectValues > 20 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d hardcoded env values — externalize to ConfigMaps for consistency", result.Summary.DirectValues))
	}
	sort.Strings(result.Recommendations)

	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. Annotation Inventory — metadata annotation governance catalog
// ---------------------------------------------------------------

type AnnotationInventoryResult1926 struct {
	ScannedAt       time.Time                      `json:"scannedAt"`
	HealthScore     int                            `json:"healthScore"`
	Grade           string                         `json:"grade"`
	Summary         AnnotationInventorySummary1926 `json:"summary"`
	Annotations     []AnnotationEntry1926          `json:"annotations"`
	ByNamespace     []AnnotationNSStat1926         `json:"byNamespace"`
	OrphanedKeys    []string                       `json:"orphanedKeys"`
	Recommendations []string                       `json:"recommendations"`
}

type AnnotationInventorySummary1926 struct {
	TotalAnnotations  int `json:"totalAnnotations"`
	UniqueKeys        int `json:"uniqueKeys"`
	StandardKeys      int `json:"standardKeys"`
	CustomKeys        int `json:"customKeys"`
	DeprecatedKeys    int `json:"deprecatedKeys"`
	NamespacesCovered int `json:"namespacesCovered"`
}

type AnnotationEntry1926 struct {
	Key          string   `json:"key"`
	UsageCount   int      `json:"usageCount"`
	Category     string   `json:"category"`
	IsStandard   bool     `json:"isStandard"`
	IsDeprecated bool     `json:"isDeprecated"`
	Namespaces   []string `json:"namespaces"`
}

type AnnotationNSStat1926 struct {
	Namespace       string `json:"namespace"`
	AnnotationCount int    `json:"annotationCount"`
	UniqueKeys      int    `json:"uniqueKeys"`
}

func (s *Server) handleAnnotationInventory(w http.ResponseWriter, r *http.Request) {
	result := AnnotationInventoryResult1926{
		ScannedAt: time.Now(),
	}
	score := 100

	// Standard annotation prefixes
	standardPrefixes := []string{
		"kubernetes.io/", "k8s.io/", "app.kubernetes.io/",
		"pod-security.kubernetes.io/", "deployment.kubernetes.io/",
		"controller.kubernetes.io/", "service.kubernetes.io/",
		"rbac.authorization.kubernetes.io/",
	}
	// Deprecated keys
	deprecatedKeys := map[string]bool{
		"kubernetes.io/change-cause":             true,
		"pod.beta.kubernetes.io/":                true,
		"pod.beta.kubernetes.io/init-containers": true,
		"scheduler.alpha.kubernetes.io/":         true,
	}

	type annInfo struct {
		count int
		nsSet map[string]bool
	}
	annMap := make(map[string]*annInfo)
	nsStats := make(map[string]*AnnotationNSStat1926)
	nsKeysSet := make(map[string]map[string]bool)

	depList, err := s.clientset.AppsV1().Deployments("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, dep := range depList.Items {
			if isSystemNamespace(dep.Namespace) {
				continue
			}
			ns, exists := nsStats[dep.Namespace]
			if !exists {
				ns = &AnnotationNSStat1926{Namespace: dep.Namespace}
				nsStats[dep.Namespace] = ns
				nsKeysSet[dep.Namespace] = make(map[string]bool)
			}
			for k := range dep.Annotations {
				info, e := annMap[k]
				if !e {
					info = &annInfo{nsSet: make(map[string]bool)}
					annMap[k] = info
				}
				info.count++
				info.nsSet[dep.Namespace] = true
				ns.AnnotationCount++
				nsKeysSet[dep.Namespace][k] = true
			}
		}
	}

	svcList, err := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	if err == nil {
		for _, sv := range svcList.Items {
			if isSystemNamespace(sv.Namespace) {
				continue
			}
			ns, exists := nsStats[sv.Namespace]
			if !exists {
				ns = &AnnotationNSStat1926{Namespace: sv.Namespace}
				nsStats[sv.Namespace] = ns
				nsKeysSet[sv.Namespace] = make(map[string]bool)
			}
			for k := range sv.Annotations {
				info, e := annMap[k]
				if !e {
					info = &annInfo{nsSet: make(map[string]bool)}
					annMap[k] = info
				}
				info.count++
				info.nsSet[sv.Namespace] = true
				ns.AnnotationCount++
				nsKeysSet[sv.Namespace][k] = true
			}
		}
	}

	for key, info := range annMap {
		nsList := make([]string, 0, len(info.nsSet))
		for ns := range info.nsSet {
			nsList = append(nsList, ns)
		}

		category := "custom"
		isStandard := false
		for _, prefix := range standardPrefixes {
			if strings.HasPrefix(key, prefix) {
				category = "standard"
				isStandard = true
				break
			}
		}

		isDeprecated := false
		if deprecatedKeys[key] {
			isDeprecated = true
			category = "deprecated"
		}
		// Check deprecated prefixes
		for depKey := range deprecatedKeys {
			if strings.HasPrefix(key, depKey) {
				isDeprecated = true
				category = "deprecated"
				break
			}
		}

		entry := AnnotationEntry1926{
			Key:          key,
			UsageCount:   info.count,
			Category:     category,
			IsStandard:   isStandard,
			IsDeprecated: isDeprecated,
			Namespaces:   nsList,
		}
		result.Annotations = append(result.Annotations, entry)
		result.Summary.TotalAnnotations += info.count
		result.Summary.UniqueKeys++

		if isStandard {
			result.Summary.StandardKeys++
		} else if isDeprecated {
			result.Summary.DeprecatedKeys++
			result.OrphanedKeys = append(result.OrphanedKeys, key)
		} else {
			result.Summary.CustomKeys++
		}
	}

	for ns, st := range nsStats {
		st.UniqueKeys = len(nsKeysSet[ns])
		result.ByNamespace = append(result.ByNamespace, *st)
	}
	result.Summary.NamespacesCovered = len(nsStats)

	sort.Slice(result.Annotations, func(i, j int) bool {
		return result.Annotations[i].UsageCount > result.Annotations[j].UsageCount
	})

	// Score
	if result.Summary.DeprecatedKeys > 0 {
		score -= result.Summary.DeprecatedKeys * 3
	}
	if result.Summary.CustomKeys > 20 {
		score -= 5
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.DeprecatedKeys > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d deprecated annotation keys — replace with current equivalents", result.Summary.DeprecatedKeys))
	}
	if result.Summary.CustomKeys > 20 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d custom annotation keys — document for team governance", result.Summary.CustomKeys))
	}
	sort.Strings(result.Recommendations)

	writeJSON(w, result)
}
