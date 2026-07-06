package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// OrphResult is the orphaned resource analysis.
type OrphResult struct {
	ScannedAt        time.Time     `json:"scannedAt"`
	Summary          OrphSummary   `json:"summary"`
	OrphanedServices []OrphEntry   `json:"orphanedServices"` // Services with no backing pods
	OrphanedConfigs  []OrphEntry   `json:"orphanedConfigs"`  // ConfigMaps not referenced
	OrphanedSecrets  []OrphEntry   `json:"orphanedSecrets"`  // Secrets not referenced
	OrphanedPVCs     []OrphEntry   `json:"orphanedPVCs"`     // PVCs not bound to any pod
	OrphanedIngress  []OrphEntry   `json:"orphanedIngress"`  // Ingresses pointing to missing services
	ByNamespace      []OrphNSEntry `json:"byNamespace"`
	Issues           []OrphIssue   `json:"issues"`
	Recommendations  []string      `json:"recommendations"`
}

// OrphSummary aggregates orphaned resource statistics.
type OrphSummary struct {
	TotalServices   int `json:"totalServices"`
	TotalConfigMaps int `json:"totalConfigMaps"`
	TotalSecrets    int `json:"totalSecrets"`
	TotalPVCs       int `json:"totalPVCs"`
	TotalIngresses  int `json:"totalIngresses"`
	OrphServices    int `json:"orphanedServices"`
	OrphConfigs     int `json:"orphanedConfigs"`
	OrphSecrets     int `json:"orphanedSecrets"`
	OrphPVCs        int `json:"orphanedPVCs"`
	OrphIngress     int `json:"orphanedIngress"`
	TotalOrphaned   int `json:"totalOrphaned"`
	HygieneScore    int `json:"hygieneScore"` // 0-100 (higher = cleaner)
}

// OrphEntry describes one orphaned resource.
type OrphEntry struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Kind      string `json:"kind"`   // Service / ConfigMap / Secret / PVC / Ingress
	Reason    string `json:"reason"` // why it's orphaned
	Age       string `json:"age"`
	RiskLevel string `json:"riskLevel"`
}

// OrphNSEntry per-namespace orphan stats.
type OrphNSEntry struct {
	Namespace   string         `json:"namespace"`
	OrphanCount int            `json:"orphanCount"`
	Details     map[string]int `json:"details"` // kind → count
}

// OrphIssue is a detected problem.
type OrphIssue struct {
	Severity string `json:"severity"`
	Type     string `json:"type"`
	Resource string `json:"resource"`
	Message  string `json:"message"`
}

// handleOrphanedResources detects orphaned resources across the cluster.
// GET /api/product/orphaned-resources
func (s *Server) handleOrphanedResources(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	ns := r.URL.Query().Get("namespace")

	pods, err := rc.clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	services, err := rc.clientset.CoreV1().Services(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	configMaps, err := rc.clientset.CoreV1().ConfigMaps(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	secrets, err := rc.clientset.CoreV1().Secrets(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	pvcs, err := rc.clientset.CoreV1().PersistentVolumeClaims(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	ingresses, err := rc.clientset.NetworkingV1().Ingresses(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	result := OrphResult{ScannedAt: time.Now()}
	now := time.Now()
	nsMap := make(map[string]*OrphNSEntry)

	// Build reference sets from pods
	usedConfigMaps := make(map[string]bool) // ns/name
	usedSecrets := make(map[string]bool)
	usedPVCs := make(map[string]bool)

	for _, pod := range pods.Items {
		podNS := pod.Namespace

		// Check volumes for ConfigMap, Secret, PVC references
		for _, vol := range pod.Spec.Volumes {
			if vol.ConfigMap != nil {
				usedConfigMaps[fmt.Sprintf("%s/%s", podNS, vol.ConfigMap.Name)] = true
			}
			if vol.Secret != nil {
				usedSecrets[fmt.Sprintf("%s/%s", podNS, vol.Secret.SecretName)] = true
			}
			if vol.PersistentVolumeClaim != nil {
				usedPVCs[fmt.Sprintf("%s/%s", podNS, vol.PersistentVolumeClaim.ClaimName)] = true
			}
			if vol.Projected != nil {
				for _, src := range vol.Projected.Sources {
					if src.ConfigMap != nil {
						usedConfigMaps[fmt.Sprintf("%s/%s", podNS, src.ConfigMap.Name)] = true
					}
					if src.Secret != nil {
						usedSecrets[fmt.Sprintf("%s/%s", podNS, src.Secret.Name)] = true
					}
				}
			}
		}

		// Check env var references
		for _, c := range pod.Spec.Containers {
			for _, ev := range c.EnvFrom {
				if ev.ConfigMapRef != nil {
					usedConfigMaps[fmt.Sprintf("%s/%s", podNS, ev.ConfigMapRef.Name)] = true
				}
				if ev.SecretRef != nil {
					usedSecrets[fmt.Sprintf("%s/%s", podNS, ev.SecretRef.Name)] = true
				}
			}
			for _, ev := range c.Env {
				if ev.ValueFrom != nil {
					if ev.ValueFrom.ConfigMapKeyRef != nil {
						usedConfigMaps[fmt.Sprintf("%s/%s", podNS, ev.ValueFrom.ConfigMapKeyRef.Name)] = true
					}
					if ev.ValueFrom.SecretKeyRef != nil {
						usedSecrets[fmt.Sprintf("%s/%s", podNS, ev.ValueFrom.SecretKeyRef.Name)] = true
					}
				}
			}
		}

		// Check imagePullSecrets
		for _, ips := range pod.Spec.ImagePullSecrets {
			usedSecrets[fmt.Sprintf("%s/%s", podNS, ips.Name)] = true
		}
	}

	// Build service name set for ingress checking
	serviceNames := make(map[string]bool)
	for _, svc := range services.Items {
		serviceNames[fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)] = true
	}

	// 1. Check Services for orphaned (no backing pods)
	for _, svc := range services.Items {
		result.Summary.TotalServices++

		// Skip system services
		if svc.Namespace == "kube-system" && (strings.HasPrefix(svc.Name, "kube-") || svc.Name == "kubernetes") {
			continue
		}

		// ExternalName services don't need backing pods
		if svc.Spec.Type == corev1.ServiceTypeExternalName {
			continue
		}

		// Services without selectors are headless/external — skip
		if len(svc.Spec.Selector) == 0 {
			continue
		}

		// Check if any pod matches the selector
		sel := labels.Set(svc.Spec.Selector).AsSelector()
		hasPods := false
		for _, pod := range pods.Items {
			if pod.Namespace != svc.Namespace {
				continue
			}
			if sel.Matches(labels.Set(pod.Labels)) {
				hasPods = true
				break
			}
		}

		if !hasPods {
			result.Summary.OrphServices++
			age := now.Sub(svc.CreationTimestamp.Time).Round(time.Hour).String()
			entry := OrphEntry{
				Name:      svc.Name,
				Namespace: svc.Namespace,
				Kind:      "Service",
				Reason:    "No pods match selector — traffic goes nowhere",
				Age:       age,
				RiskLevel: "medium",
			}
			result.OrphanedServices = append(result.OrphanedServices, entry)
			orphAddNS(nsMap, svc.Namespace, "Service")
		}
	}

	// 2. Check ConfigMaps for orphaned
	for _, cm := range configMaps.Items {
		result.Summary.TotalConfigMaps++

		// Skip auto-created configmaps
		if strings.HasSuffix(cm.Name, "-kube-root-ca.crt") {
			continue
		}

		key := fmt.Sprintf("%s/%s", cm.Namespace, cm.Name)
		if !usedConfigMaps[key] {
			result.Summary.OrphConfigs++
			age := now.Sub(cm.CreationTimestamp.Time).Round(time.Hour).String()
			entry := OrphEntry{
				Name:      cm.Name,
				Namespace: cm.Namespace,
				Kind:      "ConfigMap",
				Reason:    "Not referenced by any pod",
				Age:       age,
				RiskLevel: "low",
			}
			result.OrphanedConfigs = append(result.OrphanedConfigs, entry)
			orphAddNS(nsMap, cm.Namespace, "ConfigMap")
		}
	}

	// 3. Check Secrets for orphaned
	for _, secret := range secrets.Items {
		result.Summary.TotalSecrets++

		// Skip auto-created service account tokens
		if secret.Type == corev1.SecretTypeServiceAccountToken {
			continue
		}

		key := fmt.Sprintf("%s/%s", secret.Namespace, secret.Name)
		if !usedSecrets[key] {
			result.Summary.OrphSecrets++
			age := now.Sub(secret.CreationTimestamp.Time).Round(time.Hour).String()
			entry := OrphEntry{
				Name:      secret.Name,
				Namespace: secret.Namespace,
				Kind:      "Secret",
				Reason:    "Not referenced by any pod",
				Age:       age,
				RiskLevel: "high",
			}
			result.OrphanedSecrets = append(result.OrphanedSecrets, entry)
			result.Issues = append(result.Issues, OrphIssue{
				Severity: "warning", Type: "orphaned-secret",
				Resource: fmt.Sprintf("%s/%s", secret.Namespace, secret.Name),
				Message:  fmt.Sprintf("Secret %s/%s is not used by any pod — potential stale credential, delete if unneeded", secret.Namespace, secret.Name),
			})
			orphAddNS(nsMap, secret.Namespace, "Secret")
		}
	}

	// 4. Check PVCs for orphaned
	for _, pvc := range pvcs.Items {
		result.Summary.TotalPVCs++

		key := fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name)
		if !usedPVCs[key] {
			result.Summary.OrphPVCs++
			age := now.Sub(pvc.CreationTimestamp.Time).Round(time.Hour).String()
			entry := OrphEntry{
				Name:      pvc.Name,
				Namespace: pvc.Namespace,
				Kind:      "PVC",
				Reason:    "Not mounted by any pod",
				Age:       age,
				RiskLevel: "medium",
			}
			result.OrphanedPVCs = append(result.OrphanedPVCs, entry)
			orphAddNS(nsMap, pvc.Namespace, "PVC")
		}
	}

	// 5. Check Ingresses for orphaned (pointing to missing services)
	for _, ing := range ingresses.Items {
		result.Summary.TotalIngresses++

		// Check default backend
		if ing.Spec.DefaultBackend != nil && ing.Spec.DefaultBackend.Service != nil {
			svcKey := fmt.Sprintf("%s/%s", ing.Namespace, ing.Spec.DefaultBackend.Service.Name)
			if !serviceNames[svcKey] {
				result.Summary.OrphIngress++
				entry := OrphEntry{
					Name:      ing.Name,
					Namespace: ing.Namespace,
					Kind:      "Ingress",
					Reason:    fmt.Sprintf("Default backend service %s does not exist", ing.Spec.DefaultBackend.Service.Name),
					Age:       now.Sub(ing.CreationTimestamp.Time).Round(time.Hour).String(),
					RiskLevel: "high",
				}
				result.OrphanedIngress = append(result.OrphanedIngress, entry)
				orphAddNS(nsMap, ing.Namespace, "Ingress")
				continue
			}
		}

		// Check rule backends
		for _, rule := range ing.Spec.Rules {
			if rule.HTTP == nil {
				continue
			}
			for _, path := range rule.HTTP.Paths {
				if path.Backend.Service != nil {
					svcKey := fmt.Sprintf("%s/%s", ing.Namespace, path.Backend.Service.Name)
					if !serviceNames[svcKey] {
						result.Summary.OrphIngress++
						entry := OrphEntry{
							Name:      ing.Name,
							Namespace: ing.Namespace,
							Kind:      "Ingress",
							Reason:    fmt.Sprintf("Backend service %s does not exist for host %s", path.Backend.Service.Name, rule.Host),
							Age:       now.Sub(ing.CreationTimestamp.Time).Round(time.Hour).String(),
							RiskLevel: "high",
						}
						result.OrphanedIngress = append(result.OrphanedIngress, entry)
						result.Issues = append(result.Issues, OrphIssue{
							Severity: "critical", Type: "orphaned-ingress",
							Resource: fmt.Sprintf("%s/%s", ing.Namespace, ing.Name),
							Message:  fmt.Sprintf("Ingress %s/%s routes to non-existent service — 502/404 for users", ing.Namespace, ing.Name),
						})
						orphAddNS(nsMap, ing.Namespace, "Ingress")
						break
					}
				}
			}
		}
	}

	// Finalize namespace stats
	for _, nsStat := range nsMap {
		result.ByNamespace = append(result.ByNamespace, *nsStat)
	}

	result.Summary.TotalOrphaned = result.Summary.OrphServices + result.Summary.OrphConfigs + result.Summary.OrphSecrets + result.Summary.OrphPVCs + result.Summary.OrphIngress
	result.Summary.HygieneScore = orphScore(result.Summary)
	result.Recommendations = orphGenRecs(result.Summary)

	// Sort
	sort.Slice(result.OrphanedSecrets, func(i, j int) bool {
		return result.OrphanedSecrets[i].RiskLevel > result.OrphanedSecrets[j].RiskLevel
	})
	sort.Slice(result.OrphanedIngress, func(i, j int) bool {
		return result.OrphanedIngress[i].RiskLevel > result.OrphanedIngress[j].RiskLevel
	})
	sort.Slice(result.ByNamespace, func(i, j int) bool {
		return result.ByNamespace[i].OrphanCount > result.ByNamespace[j].OrphanCount
	})
	sort.Slice(result.Issues, func(i, j int) bool {
		return orphIssueRank(result.Issues[i].Severity) < orphIssueRank(result.Issues[j].Severity)
	})

	writeJSON(w, result)
}

func orphScore(s OrphSummary) int {
	total := s.TotalServices + s.TotalConfigMaps + s.TotalSecrets + s.TotalPVCs + s.TotalIngresses
	if total == 0 {
		return 100
	}
	orphanPct := float64(s.TotalOrphaned) / float64(total) * 100
	score := 100 - int(orphanPct*2)
	if score < 0 {
		score = 0
	}
	return score
}

func orphGenRecs(s OrphSummary) []string {
	var recs []string

	if s.OrphSecrets > 0 {
		recs = append(recs, fmt.Sprintf("%d orphaned Secret(s) — delete stale credentials to reduce attack surface and secret rotation overhead", s.OrphSecrets))
	}
	if s.OrphIngress > 0 {
		recs = append(recs, fmt.Sprintf("%d orphaned Ingress(es) — routes to non-existent services, causing 404/502 for users", s.OrphIngress))
	}
	if s.OrphServices > 0 {
		recs = append(recs, fmt.Sprintf("%d orphaned Service(s) — no backing pods, traffic goes nowhere", s.OrphServices))
	}
	if s.OrphPVCs > 0 {
		recs = append(recs, fmt.Sprintf("%d orphaned PVC(s) — not mounted by any pod, wasting storage capacity", s.OrphPVCs))
	}
	if s.OrphConfigs > 0 {
		recs = append(recs, fmt.Sprintf("%d orphaned ConfigMap(s) — not referenced by any pod, safe to clean up", s.OrphConfigs))
	}
	if s.TotalOrphaned > 10 {
		recs = append(recs, fmt.Sprintf("Total %d orphaned resources across cluster — implement resource cleanup in CI/CD pipeline", s.TotalOrphaned))
	}
	if s.HygieneScore < 70 {
		recs = append(recs, fmt.Sprintf("Resource hygiene score is %d/100 — high orphan rate detected", s.HygieneScore))
	}
	if s.TotalOrphaned == 0 {
		recs = append(recs, "No orphaned resources detected — excellent resource hygiene")
	}

	return recs
}

func orphAddNS(m map[string]*OrphNSEntry, ns, kind string) {
	if e, ok := m[ns]; ok {
		e.OrphanCount++
		e.Details[kind]++
		return
	}
	e := &OrphNSEntry{Namespace: ns, Details: make(map[string]int)}
	e.OrphanCount++
	e.Details[kind]++
	m[ns] = e
}

func orphIssueRank(s string) int {
	switch s {
	case "critical":
		return 0
	case "warning":
		return 1
	case "info":
		return 2
	default:
		return 3
	}
}

// Ensure imports used
var _ = netv1.IngressSpec{}
