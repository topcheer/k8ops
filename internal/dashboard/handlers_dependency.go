package dashboard

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// DepNode represents a single node in the dependency graph.
type DepNode struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
	Relation  string `json:"relation"` // depends-on, referenced-by, selects, scales, uses-sa, mounted-by, network-policy
	Depth     int    `json:"depth"`
}

// DependencyGraph is the full dependency analysis for a resource.
type DependencyGraph struct {
	ScannedAt    time.Time   `json:"scannedAt"`
	Root         ResourceRef `json:"root"`
	Dependencies []DepNode   `json:"dependencies"`
	Dependents   []DepNode   `json:"dependents"`
	Summary      DepSummary  `json:"summary"`
}

// ResourceRef identifies the root resource being analyzed.
type ResourceRef struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// DepSummary aggregates blast radius metrics.
type DepSummary struct {
	TotalDependencies int    `json:"totalDependencies"`
	TotalDependents   int    `json:"totalDependents"`
	ConfigMaps        int    `json:"configMaps"`
	Secrets           int    `json:"secrets"`
	PVCs              int    `json:"pvcs"`
	Services          int    `json:"services"`
	Ingresses         int    `json:"ingresses"`
	NetworkPolicies   int    `json:"networkPolicies"`
	HPAs              int    `json:"hpas"`
	ServiceAccounts   int    `json:"serviceAccounts"`
	DependentPods     int    `json:"dependentPods"`
	BlastRadius       int    `json:"blastRadius"` // total resources affected if root changes
	RiskLevel         string `json:"riskLevel"`
}

// handleDependencyGraph traces the full dependency graph for a workload.
// GET /api/dependencies?kind=Deployment&name=xxx&namespace=xxx
func (s *Server) handleDependencyGraph(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	kind := strings.TrimSpace(r.URL.Query().Get("kind"))
	name := strings.TrimSpace(r.URL.Query().Get("name"))
	ns := strings.TrimSpace(r.URL.Query().Get("namespace"))

	if kind == "" || name == "" {
		writeError(w, http.StatusBadRequest, "kind and name parameters are required")
		return
	}
	if ns == "" {
		ns = "default"
	}

	graph := DependencyGraph{
		ScannedAt: time.Now(),
		Root:      ResourceRef{Kind: kind, Name: name, Namespace: ns},
	}

	// --- Fetch all resources in namespace for analysis ---
	pods, _ := rc.clientset.CoreV1().Pods(ns).List(ctx, metav1.ListOptions{})
	services, _ := rc.clientset.CoreV1().Services(ns).List(ctx, metav1.ListOptions{})
	configmaps, _ := rc.clientset.CoreV1().ConfigMaps(ns).List(ctx, metav1.ListOptions{})
	secrets, _ := rc.clientset.CoreV1().Secrets(ns).List(ctx, metav1.ListOptions{})
	pvcs, _ := rc.clientset.CoreV1().PersistentVolumeClaims(ns).List(ctx, metav1.ListOptions{})
	netpols, _ := rc.clientset.NetworkingV1().NetworkPolicies(ns).List(ctx, metav1.ListOptions{})
	ingresses, _ := rc.clientset.NetworkingV1().Ingresses(ns).List(ctx, metav1.ListOptions{})

	// HPA (autoscaling v2)
	hpas, _ := rc.clientset.AutoscalingV2().HorizontalPodAutoscalers(ns).List(ctx, metav1.ListOptions{})

	// --- Get the target workload's pod template ---
	var podSpec *corev1.PodSpec
	var podLabels map[string]string
	var saName string

	switch strings.ToLower(kind) {
	case "deployment":
		dep, err := rc.clientset.AppsV1().Deployments(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			writeK8sError(w, err)
			return
		}
		podSpec = &dep.Spec.Template.Spec
		podLabels = dep.Spec.Template.Labels
		saName = podSpec.ServiceAccountName
	case "statefulset":
		sts, err := rc.clientset.AppsV1().StatefulSets(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			writeK8sError(w, err)
			return
		}
		podSpec = &sts.Spec.Template.Spec
		podLabels = sts.Spec.Template.Labels
		saName = podSpec.ServiceAccountName
	case "daemonset":
		ds, err := rc.clientset.AppsV1().DaemonSets(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			writeK8sError(w, err)
			return
		}
		podSpec = &ds.Spec.Template.Spec
		podLabels = ds.Spec.Template.Labels
		saName = podSpec.ServiceAccountName
	case "pod":
		pod, err := rc.clientset.CoreV1().Pods(ns).Get(ctx, name, metav1.GetOptions{})
		if err != nil {
			writeK8sError(w, err)
			return
		}
		podSpec = &pod.Spec
		podLabels = pod.Labels
		saName = podSpec.ServiceAccountName
	default:
		writeError(w, http.StatusBadRequest, "unsupported kind: "+kind+" (supported: Deployment, StatefulSet, DaemonSet, Pod)")
		return
	}

	// --- Trace dependencies (what this workload depends on) ---
	var cmRefs, secretRefs []string
	if podSpec != nil {
		// ConfigMaps from env vars and volumes
		cmRefs = extractConfigMapRefs(podSpec)
		for _, cmName := range cmRefs {
			exists := findConfigMap(configmaps, cmName)
			graph.Dependencies = append(graph.Dependencies, DepNode{
				Kind: "ConfigMap", Name: cmName, Namespace: ns,
				Relation: "depends-on", Depth: 1,
			})
			if exists {
				graph.Summary.ConfigMaps++
			}
		}

		// Secrets from env vars and volumes
		secretRefs = extractSecretRefs(podSpec)
		for _, sName := range secretRefs {
			exists := findSecret(secrets, sName)
			graph.Dependencies = append(graph.Dependencies, DepNode{
				Kind: "Secret", Name: sName, Namespace: ns,
				Relation: "depends-on", Depth: 1,
			})
			if exists {
				graph.Summary.Secrets++
			}
		}

		// PVCs from volumes
		pvcRefs := extractPVCRefs(podSpec)
		for _, pvcName := range pvcRefs {
			exists := findPVC(pvcs, pvcName)
			graph.Dependencies = append(graph.Dependencies, DepNode{
				Kind: "PersistentVolumeClaim", Name: pvcName, Namespace: ns,
				Relation: "depends-on", Depth: 1,
			})
			if exists {
				graph.Summary.PVCs++
			}
		}

		// ServiceAccount
		if saName != "" {
			graph.Dependencies = append(graph.Dependencies, DepNode{
				Kind: "ServiceAccount", Name: saName, Namespace: ns,
				Relation: "uses-sa", Depth: 1,
			})
			graph.Summary.ServiceAccounts++
		}
	}

	// Services that select this workload's pods
	for _, svc := range services.Items {
		if podLabels != nil && labelSelectorMatches(metav1.LabelSelector{MatchLabels: svc.Spec.Selector}, podLabels) {
			graph.Dependents = append(graph.Dependents, DepNode{
				Kind: "Service", Name: svc.Name, Namespace: ns,
				Relation: "selects", Depth: 1,
			})
			graph.Summary.Services++
		}
	}

	// Ingresses that reference services selecting this workload
	for _, ing := range ingresses.Items {
		if ingressTargetsServices(&ing, graph.Dependents) {
			graph.Dependents = append(graph.Dependents, DepNode{
				Kind: "Ingress", Name: ing.Name, Namespace: ns,
				Relation: "routes-to", Depth: 2,
			})
			graph.Summary.Ingresses++
		}
	}

	// NetworkPolicies that apply to these pods
	for _, np := range netpols.Items {
		if podLabels != nil && labelSelectorMatches(np.Spec.PodSelector, podLabels) {
			graph.Dependents = append(graph.Dependents, DepNode{
				Kind: "NetworkPolicy", Name: np.Name, Namespace: ns,
				Relation: "network-policy", Depth: 1,
			})
			graph.Summary.NetworkPolicies++
		}
	}

	// HPA targeting this workload
	for _, hpa := range hpas.Items {
		targetName := ""
		if hpa.Spec.ScaleTargetRef.Kind == kind {
			targetName = hpa.Spec.ScaleTargetRef.Name
		}
		if targetName == name {
			graph.Dependents = append(graph.Dependents, DepNode{
				Kind: "HorizontalPodAutoscaler", Name: hpa.Name, Namespace: ns,
				Relation: "scales", Depth: 1,
			})
			graph.Summary.HPAs++
		}
	}

	// Dependent pods (reverse: pods using the same ConfigMaps/Secrets)
	if pods != nil {
		dependentPodSet := make(map[string]bool)
		for _, pod := range pods.Items {
			if pod.Name == name && pod.Kind == kind {
				continue
			}
			podSpec := &pod.Spec
			podCMs := extractConfigMapRefs(podSpec)
			podSecrets := extractSecretRefs(podSpec)

			for _, cm := range cmRefs {
				for _, pcm := range podCMs {
					if cm == pcm {
						key := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
						if !dependentPodSet[key] {
							dependentPodSet[key] = true
							graph.Dependents = append(graph.Dependents, DepNode{
								Kind: podKind(&pod), Name: pod.Name, Namespace: pod.Namespace,
								Relation: "shares-config", Depth: 2,
							})
						}
					}
				}
			}
			for _, sec := range secretRefs {
				for _, psec := range podSecrets {
					if sec == psec {
						key := fmt.Sprintf("%s/%s", pod.Namespace, pod.Name)
						if !dependentPodSet[key] {
							dependentPodSet[key] = true
							graph.Dependents = append(graph.Dependents, DepNode{
								Kind: podKind(&pod), Name: pod.Name, Namespace: pod.Namespace,
								Relation: "shares-secret", Depth: 2,
							})
						}
					}
				}
			}
		}
		graph.Summary.DependentPods = len(dependentPodSet)
	}

	// Summary
	graph.Summary.TotalDependencies = len(graph.Dependencies)
	graph.Summary.TotalDependents = len(graph.Dependents)
	graph.Summary.BlastRadius = graph.Summary.TotalDependencies + graph.Summary.TotalDependents

	// Risk level
	switch {
	case graph.Summary.BlastRadius > 20:
		graph.Summary.RiskLevel = "critical"
	case graph.Summary.BlastRadius > 10:
		graph.Summary.RiskLevel = "high"
	case graph.Summary.BlastRadius > 5:
		graph.Summary.RiskLevel = "medium"
	default:
		graph.Summary.RiskLevel = "low"
	}

	// Sort by kind then name
	sort.Slice(graph.Dependencies, func(i, j int) bool {
		if graph.Dependencies[i].Kind != graph.Dependencies[j].Kind {
			return graph.Dependencies[i].Kind < graph.Dependencies[j].Kind
		}
		return graph.Dependencies[i].Name < graph.Dependencies[j].Name
	})
	sort.Slice(graph.Dependents, func(i, j int) bool {
		if graph.Dependents[i].Kind != graph.Dependents[j].Kind {
			return graph.Dependents[i].Kind < graph.Dependents[j].Kind
		}
		return graph.Dependents[i].Name < graph.Dependents[j].Name
	})

	writeJSON(w, graph)
}

// extractConfigMapRefs extracts all ConfigMap names referenced in a pod spec.
func extractConfigMapRefs(spec *corev1.PodSpec) []string {
	var refs []string
	seen := make(map[string]bool)

	addIfNew := func(name string) {
		if name != "" && !seen[name] {
			seen[name] = true
			refs = append(refs, name)
		}
	}

	// Volumes
	for _, vol := range spec.Volumes {
		if vol.ConfigMap != nil {
			addIfNew(vol.ConfigMap.Name)
		}
		if vol.Projected != nil {
			for _, src := range vol.Projected.Sources {
				if src.ConfigMap != nil {
					addIfNew(src.ConfigMap.Name)
				}
			}
		}
	}

	// Env vars
	for _, c := range spec.Containers {
		for _, env := range c.Env {
			if env.ValueFrom != nil && env.ValueFrom.ConfigMapKeyRef != nil {
				addIfNew(env.ValueFrom.ConfigMapKeyRef.Name)
			}
		}
		for _, envFrom := range c.EnvFrom {
			if envFrom.ConfigMapRef != nil {
				addIfNew(envFrom.ConfigMapRef.Name)
			}
		}
	}

	// Init containers
	for _, c := range spec.InitContainers {
		for _, env := range c.Env {
			if env.ValueFrom != nil && env.ValueFrom.ConfigMapKeyRef != nil {
				addIfNew(env.ValueFrom.ConfigMapKeyRef.Name)
			}
		}
		for _, envFrom := range c.EnvFrom {
			if envFrom.ConfigMapRef != nil {
				addIfNew(envFrom.ConfigMapRef.Name)
			}
		}
	}

	return refs
}

// extractSecretRefs extracts all Secret names referenced in a pod spec.
func extractSecretRefs(spec *corev1.PodSpec) []string {
	var refs []string
	seen := make(map[string]bool)

	addIfNew := func(name string) {
		if name != "" && !seen[name] {
			seen[name] = true
			refs = append(refs, name)
		}
	}

	// Volumes
	for _, vol := range spec.Volumes {
		if vol.Secret != nil {
			addIfNew(vol.Secret.SecretName)
		}
		if vol.Projected != nil {
			for _, src := range vol.Projected.Sources {
				if src.Secret != nil {
					addIfNew(src.Secret.Name)
				}
			}
		}
		if vol.CSI != nil && vol.CSI.NodePublishSecretRef != nil {
			addIfNew(vol.CSI.NodePublishSecretRef.Name)
		}
	}

	// Env vars
	for _, c := range spec.Containers {
		for _, env := range c.Env {
			if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
				addIfNew(env.ValueFrom.SecretKeyRef.Name)
			}
		}
		for _, envFrom := range c.EnvFrom {
			if envFrom.SecretRef != nil {
				addIfNew(envFrom.SecretRef.Name)
			}
		}
	}

	// Init containers
	for _, c := range spec.InitContainers {
		for _, env := range c.Env {
			if env.ValueFrom != nil && env.ValueFrom.SecretKeyRef != nil {
				addIfNew(env.ValueFrom.SecretKeyRef.Name)
			}
		}
		for _, envFrom := range c.EnvFrom {
			if envFrom.SecretRef != nil {
				addIfNew(envFrom.SecretRef.Name)
			}
		}
	}

	// Image pull secrets
	for _, ips := range spec.ImagePullSecrets {
		addIfNew(ips.Name)
	}

	return refs
}

// extractPVCRefs extracts all PVC names referenced in a pod spec.
func extractPVCRefs(spec *corev1.PodSpec) []string {
	var refs []string
	seen := make(map[string]bool)

	for _, vol := range spec.Volumes {
		if vol.PersistentVolumeClaim != nil {
			name := vol.PersistentVolumeClaim.ClaimName
			if name != "" && !seen[name] {
				seen[name] = true
				refs = append(refs, name)
			}
		}
	}

	return refs
}

// labelSelectorMatches checks if a label selector matches the given labels.
func labelSelectorMatches(selector metav1.LabelSelector, podLabels map[string]string) bool {
	if len(selector.MatchLabels) == 0 && len(selector.MatchExpressions) == 0 {
		return false // empty selector = selects nothing (not all)
	}
	sel, err := metav1.LabelSelectorAsSelector(&selector)
	if err != nil {
		return false
	}
	return sel.Matches(labels.Set(podLabels))
}

// ingressTargetsServices checks if an ingress routes to any of the dependent services.
func ingressTargetsServices(ing *networkingv1.Ingress, dependents []DepNode) bool {
	svcNames := make(map[string]bool)
	for _, d := range dependents {
		if d.Kind == "Service" {
			svcNames[d.Name] = true
		}
	}

	if ing.Spec.DefaultBackend != nil && ing.Spec.DefaultBackend.Service != nil {
		if svcNames[ing.Spec.DefaultBackend.Service.Name] {
			return true
		}
	}

	for _, rule := range ing.Spec.Rules {
		if rule.HTTP != nil {
			for _, path := range rule.HTTP.Paths {
				if path.Backend.Service != nil && svcNames[path.Backend.Service.Name] {
					return true
				}
			}
		}
	}

	return false
}

// findConfigMap returns true if a ConfigMap exists in the list.
func findConfigMap(cms *corev1.ConfigMapList, name string) bool {
	if cms == nil {
		return false
	}
	for _, cm := range cms.Items {
		if cm.Name == name {
			return true
		}
	}
	return false
}

// findSecret returns true if a Secret exists in the list.
func findSecret(secrets *corev1.SecretList, name string) bool {
	if secrets == nil {
		return false
	}
	for _, s := range secrets.Items {
		if s.Name == name {
			return true
		}
	}
	return false
}

// findPVC returns true if a PVC exists in the list.
func findPVC(pvcs *corev1.PersistentVolumeClaimList, name string) bool {
	if pvcs == nil {
		return false
	}
	for _, p := range pvcs.Items {
		if p.Name == name {
			return true
		}
	}
	return false
}

// Ensure appsv1 is used.
var _ appsv1.DeploymentSpec = appsv1.DeploymentSpec{}
