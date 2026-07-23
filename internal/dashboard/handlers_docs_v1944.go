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
// v19.44 — Documentation Dimension (Round 10)
// 1. Dependency Graph Mapper — service-to-service dependencies
// 2. Storage Class Inventory — storage class documentation & usage
// 3. DNS Resolution Map — service DNS & name collision documentation
// ============================================================

// ---------------------------------------------------------------
// 1. Dependency Graph Mapper
// ---------------------------------------------------------------

type DependencyGraphResult1944 struct {
	ScannedAt       time.Time                  `json:"scannedAt"`
	HealthScore     int                        `json:"healthScore"`
	Grade           string                     `json:"grade"`
	Summary         DependencyGraphSummary1944 `json:"summary"`
	Dependencies    []DependencyEntry1944      `json:"dependencies"`
	UnresolvedDeps  []UnresolvedDepEntry1944   `json:"unresolvedDeps"`
	Recommendations []string                   `json:"recommendations"`
}

type DependencyGraphSummary1944 struct {
	TotalServices  int `json:"totalServices"`
	ReferencedSvcs int `json:"referencedServices"`
	UnresolvedRefs int `json:"unresolvedRefs"`
	EnvVarRefs     int `json:"envVarReferences"`
	ConfigMapRefs  int `json:"configMapReferences"`
	SecretRefs     int `json:"secretReferences"`
	ExternalRefs   int `json:"externalReferences"`
}

type DependencyEntry1944 struct {
	FromPod   string `json:"fromPod"`
	FromNS    string `json:"fromNamespace"`
	ToService string `json:"toService"`
	ToNS      string `json:"toNamespace"`
	RefType   string `json:"referenceType"`
	Resolved  bool   `json:"resolved"`
}

type UnresolvedDepEntry1944 struct {
	FromPod   string `json:"fromPod"`
	FromNS    string `json:"fromNamespace"`
	Reference string `json:"reference"`
	RefType   string `json:"referenceType"`
}

func (s *Server) handleDependencyGraphV2(w http.ResponseWriter, r *http.Request) {
	result := DependencyGraphResult1944{ScannedAt: time.Now()}
	score := 100

	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})
	podList, _ := s.clientset.CoreV1().Pods("").List(r.Context(), metav1.ListOptions{})

	// Build service lookup: "ns/name" -> exists
	svcExists := make(map[string]bool)
	for _, svc := range svcList.Items {
		if isSystemNamespace(svc.Namespace) {
			continue
		}
		key := fmt.Sprintf("%s/%s", svc.Namespace, svc.Name)
		svcExists[key] = true
		result.Summary.TotalServices++
	}

	// Scan pod env vars, volumes, and image references for service deps
	depSeen := make(map[string]bool)
	for _, pod := range podList.Items {
		if isSystemNamespace(pod.Namespace) || pod.Status.Phase != corev1.PodRunning {
			continue
		}

		for _, c := range pod.Spec.Containers {
			// Check env vars for service references
			for _, ev := range c.Env {
				val := ev.Value
				if val == "" && ev.ValueFrom != nil {
					continue
				}

				// Look for service-like patterns: "svc-name", "svc-name.namespace", URLs
				refs := extractServiceRefsV2(val, pod.Namespace)
				for _, ref := range refs {
					result.Summary.EnvVarRefs++
					key := fmt.Sprintf("%s/%s->%s", pod.Namespace, pod.Name, ref)
					if depSeen[key] {
						continue
					}
					depSeen[key] = true

					parts := strings.SplitN(ref, ".", 2)
					refNS := pod.Namespace
					svcName := ref
					if len(parts) == 2 {
						svcName = parts[0]
						refNS = parts[1]
					}
					lookupKey := fmt.Sprintf("%s/%s", refNS, svcName)
					resolved := svcExists[lookupKey]

					entry := DependencyEntry1944{
						FromPod: pod.Name, FromNS: pod.Namespace,
						ToService: svcName, ToNS: refNS,
						RefType: "env-var", Resolved: resolved,
					}
					if len(result.Dependencies) < 100 {
						result.Dependencies = append(result.Dependencies, entry)
					}
					if resolved {
						result.Summary.ReferencedSvcs++
					} else {
						result.Summary.UnresolvedRefs++
						if len(result.UnresolvedDeps) < 50 {
							result.UnresolvedDeps = append(result.UnresolvedDeps, UnresolvedDepEntry1944{
								FromPod: pod.Name, FromNS: pod.Namespace,
								Reference: ref, RefType: "env-var",
							})
						}
						score -= 1
					}
				}
			}

			// Check volume mounts for configmap/secret refs
			for _, vol := range pod.Spec.Volumes {
				if vol.ConfigMap != nil {
					result.Summary.ConfigMapRefs++
					depKey := fmt.Sprintf("%s/%s->configmap:%s", pod.Namespace, pod.Name, vol.ConfigMap.Name)
					if depSeen[depKey] {
						continue
					}
					depSeen[depKey] = true
				}
				if vol.Secret != nil {
					result.Summary.SecretRefs++
				}
			}
		}

		// Check for external references (image registries)
		for _, c := range pod.Spec.Containers {
			if strings.Contains(c.Image, ".") && !strings.Contains(c.Image, "docker.io") {
				result.Summary.ExternalRefs++
			}
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.UnresolvedRefs > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d unresolved service references — verify service names and namespaces", result.Summary.UnresolvedRefs))
	}
	if result.Summary.TotalServices > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d services with %d dependency references mapped", result.Summary.TotalServices, result.Summary.ReferencedSvcs))
	}
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

func extractServiceRefsV2(value, defaultNS string) []string {
	var refs []string
	// Look for patterns like "service-name", "service.namespace.svc", "http://service"
	words := strings.FieldsFunc(value, func(c rune) bool {
		return c == ' ' || c == '=' || c == '"' || c == '\'' || c == ',' || c == ';' || c == ':' || c == '/' || c == '\n' || c == '\t'
	})
	for _, word := range words {
		// Skip empty, IPs, and common non-service values
		if word == "" || word == "true" || word == "false" || word == "http" || word == "https" {
			continue
		}
		if isIPAddress(word) {
			continue
		}
		// Potential service name: lowercase, contains hyphen or is known pattern
		if isLowerAlpha(word) && len(word) > 2 {
			refs = append(refs, word)
		}
		// service.namespace pattern
		if strings.Count(word, ".") >= 1 {
			refs = append(refs, word)
		}
	}
	return refs
}

func isIPAddress(s string) bool {
	parts := strings.Split(s, ".")
	if len(parts) != 4 {
		return false
	}
	for _, p := range parts {
		if len(p) == 0 || len(p) > 3 {
			return false
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return false
			}
		}
	}
	return true
}

// reclaimPolicyStr converts *PersistentVolumeReclaimPolicy to string
func reclaimPolicyStr(rp *corev1.PersistentVolumeReclaimPolicy) string {
	if rp == nil {
		return ""
	}
	return string(*rp)
}
func isLowerAlpha(s string) bool {
	for _, c := range s {
		if !((c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-') {
			return false
		}
	}
	return len(s) > 0
}

// ---------------------------------------------------------------
// 2. Storage Class Inventory
// ---------------------------------------------------------------

type StorageClassInvResult1944 struct {
	ScannedAt       time.Time                  `json:"scannedAt"`
	HealthScore     int                        `json:"healthScore"`
	Grade           string                     `json:"grade"`
	Summary         StorageClassInvSummary1944 `json:"summary"`
	StorageClasses  []StorageClassEntry1944    `json:"storageClasses"`
	Recommendations []string                   `json:"recommendations"`
}

type StorageClassInvSummary1944 struct {
	TotalStorageClasses int `json:"totalStorageClasses"`
	DefaultSCCount      int `json:"defaultSCCount"`
	TotalPVCs           int `json:"totalPVCs"`
	BoundPVCs           int `json:"boundPVCs"`
	PendingPVCs         int `json:"pendingPVCs"`
	ReclaimPolicyDelete int `json:"reclaimPolicyDelete"`
	ReclaimPolicyRetain int `json:"reclaimPolicyRetain"`
	VolumeBindingWait   int `json:"volumeBindingWaitForFirstConsumer"`
}

type StorageClassEntry1944 struct {
	Name              string `json:"name"`
	Provisioner       string `json:"provisioner"`
	IsDefault         bool   `json:"isDefault"`
	ReclaimPolicy     string `json:"reclaimPolicy"`
	VolumeBindingMode string `json:"volumeBindingMode"`
	PVCCount          int    `json:"pvcCount"`
	BoundPVCs         int    `json:"boundPVCs"`
}

func (s *Server) handleStorageClassInv(w http.ResponseWriter, r *http.Request) {
	result := StorageClassInvResult1944{ScannedAt: time.Now()}
	score := 100

	scList, _ := s.clientset.StorageV1().StorageClasses().List(r.Context(), metav1.ListOptions{})
	pvcList, _ := s.clientset.CoreV1().PersistentVolumeClaims("").List(r.Context(), metav1.ListOptions{})

	// Count PVCs per storage class
	pvcBySC := make(map[string]*struct{ total, bound int })
	for _, pvc := range pvcList.Items {
		if isSystemNamespace(pvc.Namespace) {
			continue
		}
		scName := ""
		if pvc.Spec.StorageClassName != nil {
			scName = *pvc.Spec.StorageClassName
		}
		if scName == "" {
			scName = "(default)"
		}
		if pvcBySC[scName] == nil {
			pvcBySC[scName] = &struct{ total, bound int }{}
		}
		pvcBySC[scName].total++
		result.Summary.TotalPVCs++
		if pvc.Status.Phase == corev1.ClaimBound {
			pvcBySC[scName].bound++
			result.Summary.BoundPVCs++
		} else {
			result.Summary.PendingPVCs++
		}
	}

	for _, sc := range scList.Items {
		result.Summary.TotalStorageClasses++
		isDefault := sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true"
		reclaimPolicy := reclaimPolicyStr(sc.ReclaimPolicy)
		bindingMode := "Immediate"
		if sc.VolumeBindingMode != nil {
			bindingMode = string(*sc.VolumeBindingMode)
		}

		if isDefault {
			result.Summary.DefaultSCCount++
		}
		if reclaimPolicy == "Delete" {
			result.Summary.ReclaimPolicyDelete++
		}
		if reclaimPolicy == "Retain" {
			result.Summary.ReclaimPolicyRetain++
		}
		if bindingMode == "WaitForFirstConsumer" {
			result.Summary.VolumeBindingWait++
		}

		pvcStats := pvcBySC[sc.Name]
		if pvcStats == nil {
			pvcStats = &struct{ total, bound int }{}
		}

		entry := StorageClassEntry1944{
			Name: sc.Name, Provisioner: sc.Provisioner,
			IsDefault: isDefault, ReclaimPolicy: reclaimPolicy,
			VolumeBindingMode: bindingMode,
			PVCCount:          pvcStats.total, BoundPVCs: pvcStats.bound,
		}
		result.StorageClasses = append(result.StorageClasses, entry)
	}

	if result.Summary.PendingPVCs > 5 {
		score -= 5
	}
	if result.Summary.DefaultSCCount == 0 && result.Summary.TotalStorageClasses > 0 {
		score -= 5
	}
	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.PendingPVCs > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d pending PVCs — check provisioner health", result.Summary.PendingPVCs))
	}
	if result.Summary.ReclaimPolicyDelete > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d StorageClasses with Delete reclaim — data loss on PVC deletion", result.Summary.ReclaimPolicyDelete))
	}
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d StorageClasses, %d PVCs (%d bound)", result.Summary.TotalStorageClasses, result.Summary.TotalPVCs, result.Summary.BoundPVCs))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}

// ---------------------------------------------------------------
// 3. DNS Resolution Map
// ---------------------------------------------------------------

type DNSMapResult1944 struct {
	ScannedAt       time.Time          `json:"scannedAt"`
	HealthScore     int                `json:"healthScore"`
	Grade           string             `json:"grade"`
	Summary         DNSMapSummary1944  `json:"summary"`
	Entries         []DNSEntry1944     `json:"entries"`
	Collisions      []DNSCollision1944 `json:"collisions"`
	Recommendations []string           `json:"recommendations"`
}

type DNSMapSummary1944 struct {
	TotalServices     int `json:"totalServices"`
	WithClusterIP     int `json:"withClusterIP"`
	WithExternalName  int `json:"withExternalName"`
	HeadlessServices  int `json:"headlessServices"`
	WithMultiplePorts int `json:"withMultiplePorts"`
	DNSCollisions     int `json:"dnsCollisions"`
}

type DNSEntry1944 struct {
	ServiceName string `json:"serviceName"`
	Namespace   string `json:"namespace"`
	ServiceType string `json:"serviceType"`
	ClusterIP   string `json:"clusterIP"`
	DNSName     string `json:"dnsName"`
	PortCount   int    `json:"portCount"`
	Headless    bool   `json:"headless"`
}

type DNSCollision1944 struct {
	ServiceName string   `json:"serviceName"`
	Namespaces  []string `json:"namespaces"`
	Severity    string   `json:"severity"`
}

func (s *Server) handleDNSMap(w http.ResponseWriter, r *http.Request) {
	result := DNSMapResult1944{ScannedAt: time.Now()}
	score := 100

	svcList, _ := s.clientset.CoreV1().Services("").List(r.Context(), metav1.ListOptions{})

	// Track service names per namespace for collision detection
	nameNSMap := make(map[string][]string)

	for _, svc := range svcList.Items {
		if isSystemNamespace(svc.Namespace) {
			continue
		}
		result.Summary.TotalServices++

		isHeadless := svc.Spec.ClusterIP == "None"
		isExternalName := svc.Spec.Type == corev1.ServiceTypeExternalName
		dnsName := fmt.Sprintf("%s.%s.svc.cluster.local", svc.Name, svc.Namespace)

		entry := DNSEntry1944{
			ServiceName: svc.Name, Namespace: svc.Namespace,
			ServiceType: string(svc.Spec.Type),
			ClusterIP:   svc.Spec.ClusterIP,
			DNSName:     dnsName,
			PortCount:   len(svc.Spec.Ports),
			Headless:    isHeadless,
		}
		result.Entries = append(result.Entries, entry)

		if svc.Spec.ClusterIP != "None" && svc.Spec.ClusterIP != "" {
			result.Summary.WithClusterIP++
		}
		if isExternalName {
			result.Summary.WithExternalName++
		}
		if isHeadless {
			result.Summary.HeadlessServices++
		}
		if len(svc.Spec.Ports) > 1 {
			result.Summary.WithMultiplePorts++
		}

		nameNSMap[svc.Name] = append(nameNSMap[svc.Name], svc.Namespace)
	}

	// Detect collisions: same service name in multiple namespaces
	for name, namespaces := range nameNSMap {
		if len(namespaces) > 1 {
			result.Summary.DNSCollisions++
			severity := "low"
			if len(namespaces) > 3 {
				severity = "medium"
			}
			result.Collisions = append(result.Collisions, DNSCollision1944{
				ServiceName: name, Namespaces: namespaces, Severity: severity,
			})
		}
	}

	if score < 0 {
		score = 0
	}
	result.HealthScore = score
	result.Grade = scoreToGrade(score)

	if result.Summary.DNSCollisions > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d service name collisions across namespaces — document for clarity", result.Summary.DNSCollisions))
	}
	if result.Summary.HeadlessServices > 0 {
		result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d headless services — used for StatefulSet DNS", result.Summary.HeadlessServices))
	}
	result.Recommendations = append(result.Recommendations, fmt.Sprintf("%d services mapped, %d with cluster IP, %d headless", result.Summary.TotalServices, result.Summary.WithClusterIP, result.Summary.HeadlessServices))
	sort.Strings(result.Recommendations)
	writeJSON(w, result)
}
