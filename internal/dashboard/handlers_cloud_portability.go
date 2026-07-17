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

// CloudPortabilityResult assesses cloud vendor lock-in and workload portability.
// It detects cloud-specific resources, annotations, storage classes, node selectors,
// and volume types to produce a portability score and migration readiness assessment.
type CloudPortabilityResult struct {
	ScannedAt       time.Time           `json:"scannedAt"`
	Summary         PortabilitySummary  `json:"summary"`
	CloudVendor     string              `json:"cloudVendor"`
	LockinFindings  []LockinFinding     `json:"lockinFindings"`
	ByNamespace     []PortabilityNSStat `json:"byNamespace"`
	MigrationPlan   []MigrationStep     `json:"migrationPlan"`
	HealthScore     int                 `json:"healthScore"`
	Grade           string              `json:"grade"`
	Recommendations []string            `json:"recommendations"`
}

// PortabilitySummary aggregates portability statistics.
type PortabilitySummary struct {
	DetectedCloud      string  `json:"detectedCloud"`
	TotalWorkloads     int     `json:"totalWorkloads"`
	PortableWorkloads  int     `json:"portableWorkloads"`
	LockedWorkloads    int     `json:"lockedWorkloads"`
	CloudSpecificSC    int     `json:"cloudSpecificSC"`
	CloudAnnotations   int     `json:"cloudAnnotations"`
	CloudNodeSelectors int     `json:"cloudNodeSelectors"`
	CloudVolumes       int     `json:"cloudVolumes"`
	PortabilityPct     float64 `json:"portabilityPct"`
	LockinRiskLevel    string  `json:"lockinRiskLevel"`
}

// LockinFinding describes one cloud-specific dependency.
type LockinFinding struct {
	Resource   string `json:"resource"`
	Namespace  string `json:"namespace"`
	Kind       string `json:"kind"`
	LockinType string `json:"lockinType"` // storage-class, annotation, node-selector, volume-type, provider-id
	Detail     string `json:"detail"`
	Cloud      string `json:"cloud"`
	Severity   string `json:"severity"` // low, medium, high
}

// PortabilityNSStat per-namespace portability stats.
type PortabilityNSStat struct {
	Namespace   string  `json:"namespace"`
	Workloads   int     `json:"workloads"`
	LockinCount int     `json:"lockinCount"`
	PortablePct float64 `json:"portablePct"`
	RiskLevel   string  `json:"riskLevel"`
}

// MigrationStep describes one action needed for cloud migration.
type MigrationStep struct {
	Priority int    `json:"priority"`
	Action   string `json:"action"`
	Impact   string `json:"impact"`
	Effort   string `json:"effort"` // low, medium, high
}

// Known cloud-specific patterns
var cloudStoragePatterns = map[string]string{
	"kubernetes.io/aws-ebs":    "aws",
	"kubernetes.io/gce-pd":     "gcp",
	"kubernetes.io/azure-disk": "azure",
	"kubernetes.io/azure-file": "azure",
	"ebs.csi.aws.com":          "aws",
	"gp2":                      "aws",
	"gp3":                      "aws",
	"io1":                      "aws",
	"standard":                 "gcp",
	"pd-standard":              "gcp",
	"pd-ssd":                   "gcp",
	"default":                  "generic",
	"managed-csi":              "azure",
	"managed-premium":          "azure",
	"managed-standard":         "azure",
	"azurefile":                "azure",
	"rancher.io/local-path":    "generic",
	"openebs-hostpath":         "generic",
	"rook-ceph-block":          "generic",
	"longhorn":                 "generic",
}

var cloudAnnotationPrefixes = map[string]string{
	"service.beta.kubernetes.io/aws":          "aws",
	"service.kubernetes.io/aws":               "aws",
	"pod.beta.kubernetes.io/aws":              "aws",
	"service.beta.kubernetes.io/azure":        "azure",
	"service.beta.kubernetes.io/gce":          "gcp",
	"networking.gke.io":                       "gcp",
	"cloud.google.com":                        "gcp",
	"controller.kubernetes.io/cloud-provider": "generic",
	"kubernetes.azure.com":                    "azure",
	"volume.beta.kubernetes.io/storage-class": "generic",
	"volume.beta.kubernetes.io/mount-options": "generic",
	"eks.amazonaws.com":                       "aws",
	"gke.io":                                  "gcp",
}

var cloudNodeSelectorKeys = map[string]string{
	"node.kubernetes.io/instance-type": "generic",
	"topology.kubernetes.io/zone":      "generic",
	"topology.kubernetes.io/region":    "generic",
	"kubernetes.io/os":                 "generic",
	"kubernetes.io/arch":               "generic",
	"node-role.kubernetes.io":          "generic",
}

// handleCloudPortability handles GET /api/product/cloud-portability
func (s *Server) handleCloudPortability(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	result := CloudPortabilityResult{ScannedAt: time.Now()}

	nodes, _ := rc.clientset.CoreV1().Nodes().List(ctx, metav1.ListOptions{})
	pods, _ := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	services, _ := rc.clientset.CoreV1().Services("").List(ctx, metav1.ListOptions{})
	pvcs, _ := rc.clientset.CoreV1().PersistentVolumeClaims("").List(ctx, metav1.ListOptions{})
	scs, _ := rc.clientset.StorageV1().StorageClasses().List(ctx, metav1.ListOptions{})
	ingresses, _ := rc.clientset.NetworkingV1().Ingresses("").List(ctx, metav1.ListOptions{})

	// Detect cloud vendor from node providerIDs
	cloudVendor := detectCloudVendor(nodes.Items)
	result.CloudVendor = cloudVendor
	result.Summary.DetectedCloud = cloudVendor

	// Classify storage classes
	cloudSCSet := map[string]bool{}
	for _, sc := range scs.Items {
		provisioner := sc.Provisioner
		if cloud, ok := cloudStoragePatterns[provisioner]; ok && cloud != "generic" {
			result.Summary.CloudSpecificSC++
			cloudSCSet[sc.Name] = true
			result.LockinFindings = append(result.LockinFindings, LockinFinding{
				Resource:   "StorageClass/" + sc.Name,
				Kind:       "StorageClass",
				LockinType: "storage-class",
				Detail:     fmt.Sprintf("Provisioner '%s' is cloud-specific to %s", provisioner, cloud),
				Cloud:      cloud,
				Severity:   "medium",
			})
		}
	}

	// Build PVC to SC mapping
	pvcSCMap := map[string]string{}
	for _, pvc := range pvcs.Items {
		scName := ""
		if pvc.Spec.StorageClassName != nil {
			scName = *pvc.Spec.StorageClassName
		}
		pvcSCMap[pvc.Namespace+"/"+pvc.Name] = scName
	}

	// Check pods for cloud-specific dependencies
	nsStats := map[string]*PortabilityNSStat{}
	lockedWorkloads := map[string]bool{}

	for _, pod := range pods.Items {
		if strings.HasPrefix(pod.Namespace, "kube-") || strings.HasPrefix(pod.Namespace, "k8s-") {
			continue
		}
		result.Summary.TotalWorkloads++

		wkName := pod.Namespace + "/" + pod.Name
		hasLockin := false

		// Check PVC references for cloud-specific storage
		for _, vol := range pod.Spec.Volumes {
			if vol.PersistentVolumeClaim != nil {
				key := pod.Namespace + "/" + vol.PersistentVolumeClaim.ClaimName
				if sc, ok := pvcSCMap[key]; ok && cloudSCSet[sc] {
					result.Summary.CloudVolumes++
					hasLockin = true
					result.LockinFindings = append(result.LockinFindings, LockinFinding{
						Resource:   wkName,
						Namespace:  pod.Namespace,
						Kind:       "Pod",
						LockinType: "volume-type",
						Detail:     fmt.Sprintf("Uses PVC '%s' backed by cloud-specific StorageClass '%s'", vol.PersistentVolumeClaim.ClaimName, sc),
						Cloud:      cloudVendor,
						Severity:   "high",
					})
				}
			}
		}

		// Check node selectors for cloud-specific labels
		for key, val := range pod.Spec.NodeSelector {
			if cloud, ok := cloudNodeSelectorKeys[key]; ok {
				// Only flag instance-type as cloud-specific
				if key == "node.kubernetes.io/instance-type" {
					result.Summary.CloudNodeSelectors++
					hasLockin = true
					result.LockinFindings = append(result.LockinFindings, LockinFinding{
						Resource:   wkName,
						Namespace:  pod.Namespace,
						Kind:       "Pod",
						LockinType: "node-selector",
						Detail:     fmt.Sprintf("Node selector '%s=%s' is cloud-specific", key, val),
						Cloud:      cloudVendor,
						Severity:   "medium",
					})
				}
				_ = cloud
			}
		}

		// Check annotations for cloud-specific patterns
		for key := range pod.Annotations {
			for prefix, cloud := range cloudAnnotationPrefixes {
				if strings.HasPrefix(key, prefix) && cloud != "generic" {
					result.Summary.CloudAnnotations++
					hasLockin = true
					result.LockinFindings = append(result.LockinFindings, LockinFinding{
						Resource:   wkName,
						Namespace:  pod.Namespace,
						Kind:       "Pod",
						LockinType: "annotation",
						Detail:     fmt.Sprintf("Annotation '%s' is cloud-specific to %s", key, cloud),
						Cloud:      cloud,
						Severity:   "low",
					})
				}
			}
		}

		if hasLockin {
			lockedWorkloads[wkName] = true
		}

		// Track namespace stats
		ns := pod.Namespace
		if _, ok := nsStats[ns]; !ok {
			nsStats[ns] = &PortabilityNSStat{Namespace: ns}
		}
		nsStats[ns].Workloads++
		if hasLockin {
			nsStats[ns].LockinCount++
		}
	}

	// Check services for cloud annotations
	for _, svc := range services.Items {
		if strings.HasPrefix(svc.Namespace, "kube-") {
			continue
		}
		for key := range svc.Annotations {
			for prefix, cloud := range cloudAnnotationPrefixes {
				if strings.HasPrefix(key, prefix) && cloud != "generic" {
					result.Summary.CloudAnnotations++
					result.LockinFindings = append(result.LockinFindings, LockinFinding{
						Resource:   "Service/" + svc.Namespace + "/" + svc.Name,
						Namespace:  svc.Namespace,
						Kind:       "Service",
						LockinType: "annotation",
						Detail:     fmt.Sprintf("Service annotation '%s' ties to %s cloud", key, cloud),
						Cloud:      cloud,
						Severity:   "medium",
					})
				}
			}
		}
	}

	// Check ingresses for cloud-specific annotations
	for _, ing := range ingresses.Items {
		if ing.Annotations == nil {
			continue
		}
		for key := range ing.Annotations {
			for prefix, cloud := range cloudAnnotationPrefixes {
				if strings.HasPrefix(key, prefix) && cloud != "generic" {
					result.Summary.CloudAnnotations++
					result.LockinFindings = append(result.LockinFindings, LockinFinding{
						Resource:   "Ingress/" + ing.Namespace + "/" + ing.Name,
						Namespace:  ing.Namespace,
						Kind:       "Ingress",
						LockinType: "annotation",
						Detail:     fmt.Sprintf("Ingress annotation '%s' ties to %s cloud", key, cloud),
						Cloud:      cloud,
						Severity:   "medium",
					})
				}
			}
		}
	}

	// Calculate summary
	result.Summary.LockedWorkloads = len(lockedWorkloads)
	result.Summary.PortableWorkloads = result.Summary.TotalWorkloads - result.Summary.LockedWorkloads
	if result.Summary.TotalWorkloads > 0 {
		result.Summary.PortabilityPct = float64(result.Summary.PortableWorkloads) / float64(result.Summary.TotalWorkloads) * 100
	}

	// Determine lockin risk level
	lockinPct := 100 - result.Summary.PortabilityPct
	switch {
	case lockinPct > 50:
		result.Summary.LockinRiskLevel = "critical"
	case lockinPct > 25:
		result.Summary.LockinRiskLevel = "high"
	case lockinPct > 10:
		result.Summary.LockinRiskLevel = "medium"
	default:
		result.Summary.LockinRiskLevel = "low"
	}

	// Build namespace stats
	for _, ns := range nsStats {
		if ns.Workloads > 0 {
			ns.PortablePct = float64(ns.Workloads-ns.LockinCount) / float64(ns.Workloads) * 100
		}
		switch {
		case ns.PortablePct < 50:
			ns.RiskLevel = "high"
		case ns.PortablePct < 80:
			ns.RiskLevel = "medium"
		default:
			ns.RiskLevel = "low"
		}
		result.ByNamespace = append(result.ByNamespace, *ns)
	}
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].LockinCount > result.ByNamespace[j].LockinCount
	})

	// Generate migration plan
	result.MigrationPlan = generateMigrationPlan(result.Summary, result.LockinFindings)

	// Compute health score
	result.HealthScore = computePortabilityScore(result.Summary, len(result.LockinFindings))
	result.Grade = scoreToGrade(result.HealthScore)

	// Generate recommendations
	result.Recommendations = generatePortabilityRecs(result.Summary, result.LockinFindings, cloudVendor)

	writeJSON(w, result)
}

// detectCloudVendor determines the cloud provider from node providerIDs.
func detectCloudVendor(nodes []corev1.Node) string {
	for _, node := range nodes {
		pid := strings.ToLower(node.Spec.ProviderID)
		switch {
		case strings.HasPrefix(pid, "aws://") || strings.Contains(pid, "aws/"):
			return "aws"
		case strings.HasPrefix(pid, "gce://") || strings.Contains(pid, "gce/"):
			return "gcp"
		case strings.HasPrefix(pid, "azure://") || strings.Contains(pid, "azure/"):
			return "azure"
		}
	}
	return "unknown"
}

// computePortabilityScore computes a 0-100 portability score.
func computePortabilityScore(s PortabilitySummary, findingCount int) int {
	score := 100
	// Penalize locked workloads
	if s.TotalWorkloads > 0 {
		lockedRatio := float64(s.LockedWorkloads) / float64(s.TotalWorkloads)
		score -= int(lockedRatio * 40)
	}
	// Penalize cloud-specific storage classes
	score -= minInt(s.CloudSpecificSC*5, 20)
	// Penalize cloud-specific annotations
	score -= minInt(s.CloudAnnotations*2, 15)
	// Penalize cloud-specific volumes
	score -= minInt(s.CloudVolumes*3, 15)
	// Penalize cloud-specific node selectors
	score -= minInt(s.CloudNodeSelectors*2, 10)

	if score < 0 {
		score = 0
	}
	return score
}

// generateMigrationPlan produces prioritized migration steps.
func generateMigrationPlan(s PortabilitySummary, findings []LockinFinding) []MigrationStep {
	var steps []MigrationStep
	prio := 1

	if s.CloudSpecificSC > 0 {
		steps = append(steps, MigrationStep{
			Priority: prio,
			Action:   fmt.Sprintf("Replace %d cloud-specific StorageClass provisioner(s) with CSI driver equivalents or generic alternatives", s.CloudSpecificSC),
			Impact:   "high",
			Effort:   "medium",
		})
		prio++
	}

	if s.CloudVolumes > 0 {
		steps = append(steps, MigrationStep{
			Priority: prio,
			Action:   fmt.Sprintf("Migrate %d PVC(s) from cloud-specific storage to portable CSI drivers or shared file systems", s.CloudVolumes),
			Impact:   "high",
			Effort:   "high",
		})
		prio++
	}

	if s.CloudAnnotations > 0 {
		steps = append(steps, MigrationStep{
			Priority: prio,
			Action:   fmt.Sprintf("Replace %d cloud-specific service/ingress annotation(s) with IngressClass or Gateway API standards", s.CloudAnnotations),
			Impact:   "medium",
			Effort:   "low",
		})
		prio++
	}

	if s.CloudNodeSelectors > 0 {
		steps = append(steps, MigrationStep{
			Priority: prio,
			Action:   fmt.Sprintf("Replace %d cloud-specific node selector(s) with standard labels (kubernetes.io/os, arch, or custom node pools)", s.CloudNodeSelectors),
			Impact:   "medium",
			Effort:   "low",
		})
		prio++
	}

	if s.LockedWorkloads > 0 {
		steps = append(steps, MigrationStep{
			Priority: prio,
			Action:   fmt.Sprintf("Refactor %d workload(s) to remove cloud-specific dependencies — use ConfigMaps for provider-specific configs", s.LockedWorkloads),
			Impact:   "high",
			Effort:   "high",
		})
	}

	if len(steps) == 0 {
		steps = append(steps, MigrationStep{
			Priority: 1,
			Action:   "Workloads are portable — maintain current practices and avoid cloud-specific resource types",
			Impact:   "none",
			Effort:   "none",
		})
	}

	return steps
}

// generatePortabilityRecs produces human-readable recommendations.
func generatePortabilityRecs(s PortabilitySummary, findings []LockinFinding, vendor string) []string {
	var recs []string

	if s.TotalWorkloads == 0 {
		recs = append(recs, "No user workloads detected — portability assessment not applicable")
		return recs
	}

	recs = append(recs, fmt.Sprintf("Portability score: %d/100 (grade %s) — %.1f%% of workloads are cloud-portable",
		computePortabilityScore(s, len(findings)), scoreToGrade(computePortabilityScore(s, len(findings))), s.PortabilityPct))

	if s.LockinRiskLevel == "critical" || s.LockinRiskLevel == "high" {
		recs = append(recs, fmt.Sprintf("Vendor lock-in risk is %s — %d of %d workloads have cloud-specific dependencies", s.LockinRiskLevel, s.LockedWorkloads, s.TotalWorkloads))
	}

	if s.CloudSpecificSC > 0 {
		recs = append(recs, fmt.Sprintf("%d cloud-specific StorageClass(es) detected — standardize on CSI drivers for multi-cloud portability", s.CloudSpecificSC))
	}

	if s.CloudVolumes > 0 {
		recs = append(recs, fmt.Sprintf("%d PVC(s) use cloud-specific storage — consider Rook-Ceph, Longhorn, or OpenEBS for portable storage", s.CloudVolumes))
	}

	if s.CloudAnnotations > 0 {
		recs = append(recs, fmt.Sprintf("%d cloud-specific annotation(s) found — migrate to standard IngressClass, Gateway API, or ExternalDNS", s.CloudAnnotations))
	}

	if vendor != "unknown" {
		recs = append(recs, fmt.Sprintf("Detected cloud: %s — use multi-cloud tools like Crossplane or Cluster API for portability", strings.ToUpper(vendor)))
	}

	if len(recs) <= 1 {
		recs = append(recs, "No significant cloud-specific dependencies detected — workloads are portable")
	}

	return recs
}

// minInt helper
func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
