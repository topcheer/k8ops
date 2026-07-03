package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ResourceWasteCategory describes the type of wasted resource.
type ResourceWasteCategory string

const (
	WasteDeadService    ResourceWasteCategory = "dead-service"       // Service with no backing endpoints
	WasteUnusedPVC      ResourceWasteCategory = "unused-pvc"         // PVC not mounted by any pod
	WasteOrphanedCM     ResourceWasteCategory = "orphaned-configmap" // ConfigMap not referenced by any pod
	WasteOrphanedSecret ResourceWasteCategory = "orphaned-secret"    // Secret not referenced by any pod
	WasteEmptyNamespace ResourceWasteCategory = "empty-namespace"    // Namespace with no workloads
	WasteUnattachedPV   ResourceWasteCategory = "unattached-pv"      // PV not bound to any PVC
)

// WasteSeverity rates the potential impact of the waste.
type WasteSeverity string

const (
	WasteSeverityCritical WasteSeverity = "critical" // LoadBalancer service with no endpoints, unattached PV
	WasteSeverityHigh     WasteSeverity = "high"     // Unused PVC, orphaned secret with sensitive data
	WasteSeverityMedium   WasteSeverity = "medium"   // Orphaned configmap, empty namespace
	WasteSeverityLow      WasteSeverity = "low"      // Small configmap, system namespace
)

// ResourceWasteItem describes a single detected waste.
type ResourceWasteItem struct {
	Category   ResourceWasteCategory `json:"category"`
	Severity   WasteSeverity         `json:"severity"`
	Kind       string                `json:"kind"` // Service, PersistentVolumeClaim, ConfigMap, Secret, Namespace, PersistentVolume
	Name       string                `json:"name"`
	Namespace  string                `json:"namespace,omitempty"`
	AgeHours   float64               `json:"ageHours"`
	Detail     string                `json:"detail"`
	Suggestion string                `json:"suggestion"`
}

// WasteResult is the full scan output.
type WasteResult struct {
	ScannedAt time.Time           `json:"scannedAt"`
	Summary   WasteSummary        `json:"summary"`
	Items     []ResourceWasteItem `json:"items"`
}

// WasteSummary aggregates waste statistics.
type WasteSummary struct {
	Total       int            `json:"total"`
	ByCategory  map[string]int `json:"byCategory"`
	BySeverity  map[string]int `json:"bySeverity"`
	EstCostRisk string         `json:"estCostRisk"` // low, moderate, high
}

// handleWasteDetection scans for wasted/orphaned resources across the cluster.
// GET /api/resources/waste?namespace=xxx
func (s *Server) handleWasteDetection(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	ns := r.URL.Query().Get("namespace")
	ctx := r.Context()

	result := WasteResult{
		ScannedAt: time.Now(),
		Summary: WasteSummary{
			ByCategory: make(map[string]int),
			BySeverity: make(map[string]int),
		},
	}

	// Gather all resources
	podList, err := rc.clientset.CoreV1().Pods("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}
	svcList, err := rc.clientset.CoreV1().Services(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}
	endpointList, err := rc.clientset.CoreV1().Endpoints("").List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}
	pvcList, err := rc.clientset.CoreV1().PersistentVolumeClaims(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}
	pvList, err := rc.clientset.CoreV1().PersistentVolumes().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}
	cmList, err := rc.clientset.CoreV1().ConfigMaps(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}
	secretList, err := rc.clientset.CoreV1().Secrets(ns).List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}
	nsList, err := rc.clientset.CoreV1().Namespaces().List(ctx, metav1.ListOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	// Build reference sets from pods
	mountedPVCs := buildMountedPVCSet(podList.Items)
	usedCMs := buildUsedConfigMapSet(podList.Items)
	usedSecrets := buildUsedSecretSet(podList.Items)

	// Build endpoint map: service -> has endpoints
	endpointMap := buildEndpointMap(endpointList.Items)

	// Build namespace workload set
	nsWorkloads := buildNamespaceWorkloadSet(podList.Items)

	// --- Detect dead services ---
	for i := range svcList.Items {
		svc := &svcList.Items[i]
		if isKnownSystemNamespace(svc.Namespace) {
			continue // skip kube-system etc for noise reduction
		}
		if svc.Spec.Type == corev1.ServiceTypeExternalName {
			continue // external name services don't need endpoints
		}
		key := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)
		if !endpointMap[key] {
			severity := WasteSeverityMedium
			if svc.Spec.Type == corev1.ServiceTypeLoadBalancer {
				severity = WasteSeverityCritical // LoadBalancers cost money
			}
			result.Items = append(result.Items, ResourceWasteItem{
				Category:   WasteDeadService,
				Severity:   severity,
				Kind:       "Service",
				Name:       svc.Name,
				Namespace:  svc.Namespace,
				AgeHours:   hoursSince(svc.CreationTimestamp),
				Detail:     fmt.Sprintf("Service type %s has no backing endpoints", svc.Spec.Type),
				Suggestion: "Check if the target pods are running. If unused, delete the service to release resources (especially LoadBalancer).",
			})
		}
	}

	// --- Detect unused PVCs ---
	for i := range pvcList.Items {
		pvc := &pvcList.Items[i]
		if pvc.Status.Phase != corev1.ClaimBound && pvc.Status.Phase != corev1.ClaimPending {
			continue // skip released/lost PVCs handled by PV section
		}
		key := fmt.Sprintf("%s/%s", pvc.Namespace, pvc.Name)
		if !mountedPVCs[key] {
			result.Items = append(result.Items, ResourceWasteItem{
				Category:   WasteUnusedPVC,
				Severity:   WasteSeverityHigh,
				Kind:       "PersistentVolumeClaim",
				Name:       pvc.Name,
				Namespace:  pvc.Namespace,
				AgeHours:   hoursSince(pvc.CreationTimestamp),
				Detail:     fmt.Sprintf("PVC %s is %s but not mounted by any pod", pvc.Name, pvc.Status.Phase),
				Suggestion: "Verify no pods reference this PVC. If truly unused, delete it to reclaim storage capacity.",
			})
		}
	}

	// --- Detect unattached PVs ---
	for i := range pvList.Items {
		pv := &pvList.Items[i]
		if pv.Status.Phase == corev1.VolumeAvailable {
			result.Items = append(result.Items, ResourceWasteItem{
				Category:   WasteUnattachedPV,
				Severity:   WasteSeverityCritical,
				Kind:       "PersistentVolume",
				Name:       pv.Name,
				AgeHours:   hoursSince(pv.CreationTimestamp),
				Detail:     fmt.Sprintf("PV %s is Available (not bound to any PVC)", pv.Name),
				Suggestion: "If no PVC will claim this volume, consider deleting it to reclaim storage.",
			})
		}
	}

	// --- Detect orphaned ConfigMaps ---
	for i := range cmList.Items {
		cm := &cmList.Items[i]
		if isSystemConfigMap(cm) {
			continue
		}
		key := fmt.Sprintf("%s/%s", cm.Namespace, cm.Name)
		if !usedCMs[key] {
			severity := WasteSeverityLow
			if cm.Annotations["kubectl.kubernetes.io/last-applied-configuration"] != "" {
				severity = WasteSeverityMedium // user-created CM
			}
			result.Items = append(result.Items, ResourceWasteItem{
				Category:   WasteOrphanedCM,
				Severity:   severity,
				Kind:       "ConfigMap",
				Name:       cm.Name,
				Namespace:  cm.Namespace,
				AgeHours:   hoursSince(cm.CreationTimestamp),
				Detail:     fmt.Sprintf("ConfigMap not referenced by any pod in namespace %s", cm.Namespace),
				Suggestion: "If no longer needed, delete this ConfigMap to reduce clutter.",
			})
		}
	}

	// --- Detect orphaned Secrets ---
	for i := range secretList.Items {
		secret := &secretList.Items[i]
		if isSystemSecret(secret) {
			continue
		}
		key := fmt.Sprintf("%s/%s", secret.Namespace, secret.Name)
		if !usedSecrets[key] {
			severity := WasteSeverityHigh // secrets are more sensitive
			result.Items = append(result.Items, ResourceWasteItem{
				Category:   WasteOrphanedSecret,
				Severity:   severity,
				Kind:       "Secret",
				Name:       secret.Name,
				Namespace:  secret.Namespace,
				AgeHours:   hoursSince(secret.CreationTimestamp),
				Detail:     fmt.Sprintf("Secret type %s not referenced by any pod in namespace %s", secret.Type, secret.Namespace),
				Suggestion: "Unused secrets pose a security risk. Review and delete if no longer needed.",
			})
		}
	}

	// --- Detect empty namespaces ---
	for i := range nsList.Items {
		n := &nsList.Items[i]
		if n.Status.Phase != corev1.NamespaceActive {
			continue
		}
		if isKnownSystemNamespace(n.Name) {
			continue
		}
		if !nsWorkloads[n.Name] {
			result.Items = append(result.Items, ResourceWasteItem{
				Category:   WasteEmptyNamespace,
				Severity:   WasteSeverityMedium,
				Kind:       "Namespace",
				Name:       n.Name,
				AgeHours:   hoursSince(n.CreationTimestamp),
				Detail:     fmt.Sprintf("Namespace %s has no running pods", n.Name),
				Suggestion: "If this namespace is abandoned, consider cleaning up any remaining resources and deleting it.",
			})
		}
	}

	// --- Sort: severity first, then age ---
	sort.Slice(result.Items, func(i, j int) bool {
		si := wasteSeverityRank(result.Items[i].Severity)
		sj := wasteSeverityRank(result.Items[j].Severity)
		if si != sj {
			return si < sj
		}
		return result.Items[i].AgeHours > result.Items[j].AgeHours
	})

	// --- Build summary ---
	result.Summary.Total = len(result.Items)
	for _, item := range result.Items {
		result.Summary.ByCategory[string(item.Category)]++
		result.Summary.BySeverity[string(item.Severity)]++
	}
	critical := result.Summary.BySeverity[string(WasteSeverityCritical)]
	high := result.Summary.BySeverity[string(WasteSeverityHigh)]
	if critical >= 3 || high >= 5 {
		result.Summary.EstCostRisk = "high"
	} else if critical >= 1 || high >= 2 {
		result.Summary.EstCostRisk = "moderate"
	} else {
		result.Summary.EstCostRisk = "low"
	}

	writeJSON(w, result)
}

// buildMountedPVCSet returns a set of "namespace/pvcname" that are mounted by pods.
func buildMountedPVCSet(pods []corev1.Pod) map[string]bool {
	set := make(map[string]bool)
	for _, pod := range pods {
		for _, vol := range pod.Spec.Volumes {
			if vol.PersistentVolumeClaim != nil {
				key := fmt.Sprintf("%s/%s", pod.Namespace, vol.PersistentVolumeClaim.ClaimName)
				set[key] = true
			}
		}
	}
	return set
}

// buildUsedConfigMapSet returns a set of "namespace/cmname" referenced by pods.
func buildUsedConfigMapSet(pods []corev1.Pod) map[string]bool {
	set := make(map[string]bool)
	for _, pod := range pods {
		for _, vol := range pod.Spec.Volumes {
			if vol.ConfigMap != nil {
				key := fmt.Sprintf("%s/%s", pod.Namespace, vol.ConfigMap.Name)
				set[key] = true
			}
			if vol.Projected != nil {
				for _, src := range vol.Projected.Sources {
					if src.ConfigMap != nil {
						key := fmt.Sprintf("%s/%s", pod.Namespace, src.ConfigMap.Name)
						set[key] = true
					}
				}
			}
		}
		// Check env var references
		for _, c := range pod.Spec.Containers {
			for _, e := range c.Env {
				if e.ValueFrom != nil && e.ValueFrom.ConfigMapKeyRef != nil {
					key := fmt.Sprintf("%s/%s", pod.Namespace, e.ValueFrom.ConfigMapKeyRef.Name)
					set[key] = true
				}
			}
			for _, e := range c.EnvFrom {
				if e.ConfigMapRef != nil {
					key := fmt.Sprintf("%s/%s", pod.Namespace, e.ConfigMapRef.Name)
					set[key] = true
				}
			}
		}
	}
	return set
}

// buildUsedSecretSet returns a set of "namespace/secretname" referenced by pods.
func buildUsedSecretSet(pods []corev1.Pod) map[string]bool {
	set := make(map[string]bool)
	for _, pod := range pods {
		// Image pull secrets
		for _, ips := range pod.Spec.ImagePullSecrets {
			key := fmt.Sprintf("%s/%s", pod.Namespace, ips.Name)
			set[key] = true
		}
		for _, vol := range pod.Spec.Volumes {
			if vol.Secret != nil {
				key := fmt.Sprintf("%s/%s", pod.Namespace, vol.Secret.SecretName)
				set[key] = true
			}
			if vol.Projected != nil {
				for _, src := range vol.Projected.Sources {
					if src.Secret != nil {
						key := fmt.Sprintf("%s/%s", pod.Namespace, src.Secret.Name)
						set[key] = true
					}
				}
			}
		}
		// Env var references
		for _, c := range pod.Spec.Containers {
			for _, e := range c.Env {
				if e.ValueFrom != nil && e.ValueFrom.SecretKeyRef != nil {
					key := fmt.Sprintf("%s/%s", pod.Namespace, e.ValueFrom.SecretKeyRef.Name)
					set[key] = true
				}
			}
			for _, e := range c.EnvFrom {
				if e.SecretRef != nil {
					key := fmt.Sprintf("%s/%s", pod.Namespace, e.SecretRef.Name)
					set[key] = true
				}
			}
		}
	}
	return set
}

// buildEndpointMap returns a map of "namespace/servicename" -> has at least one endpoint.
func buildEndpointMap(endpoints []corev1.Endpoints) map[string]bool {
	m := make(map[string]bool)
	for _, ep := range endpoints {
		key := fmt.Sprintf("%s/%s", ep.Namespace, ep.Name)
		for _, subset := range ep.Subsets {
			if len(subset.Addresses) > 0 || len(subset.NotReadyAddresses) > 0 {
				m[key] = true
				break
			}
		}
	}
	return m
}

// buildNamespaceWorkloadSet returns namespaces that have at least one pod.
func buildNamespaceWorkloadSet(pods []corev1.Pod) map[string]bool {
	set := make(map[string]bool)
	for _, pod := range pods {
		set[pod.Namespace] = true
	}
	return set
}

// isSystemNamespace returns true for well-known system namespaces.
func isKnownSystemNamespace(ns string) bool {
	switch ns {
	case "kube-system", "kube-public", "kube-node-lease", "default":
		return true
	}
	return false
}

// isSystemConfigMap returns true for well-known auto-generated ConfigMaps.
func isSystemConfigMap(cm *corev1.ConfigMap) bool {
	// Skip auto-generated CMs
	if cm.Name == "kube-root-ca.crt" {
		return true
	}
	if _, ok := cm.Annotations["control-plane.alpha.kubernetes.io/leader"]; ok {
		return true
	}
	// Skip kubeadm-config, kube-proxy, etc.
	systemCMs := map[string]bool{
		"kubeadm-config":                     true,
		"kube-proxy":                         true,
		"coredns":                            true,
		"extension-apiserver-authentication": true,
	}
	return systemCMs[cm.Name]
}

// isSystemSecret returns true for well-known auto-generated Secrets.
func isSystemSecret(secret *corev1.Secret) bool {
	// Skip service-account tokens
	if secret.Type == corev1.SecretTypeServiceAccountToken {
		return true
	}
	// Skip Helm release secrets
	if secret.Type == "helm.sh/release.v1" {
		return true
	}
	return false
}

// wasteSeverityRank returns a numeric severity rank for sorting.
func wasteSeverityRank(s WasteSeverity) int {
	switch s {
	case WasteSeverityCritical:
		return 0
	case WasteSeverityHigh:
		return 1
	case WasteSeverityMedium:
		return 2
	case WasteSeverityLow:
		return 3
	}
	return 9
}
