package dashboard

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// handleResources lists workloads by kind.
// GET /api/resources?kind=deployments&namespace=
func (s *Server) handleResources(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	kind := strings.ToLower(r.URL.Query().Get("kind"))
	if kind == "" {
		kind = "deployments"
	}
	ns := r.URL.Query().Get("namespace")

	type resItem struct {
		Name      string            `json:"name"`
		Namespace string            `json:"namespace"`
		Ready     string            `json:"ready,omitempty"`
		Type      string            `json:"type,omitempty"`
		Age       string            `json:"age"`
		Detail    map[string]string `json:"detail,omitempty"`
	}
	var items []resItem

	switch kind {
	case "deployments", "deployment", "deploy":
		list, err := rc.clientset.AppsV1().Deployments(ns).List(r.Context(), metav1.ListOptions{Limit: 500})
		if err != nil {
			writeK8sError(w, err)
			return
		}
		for _, d := range list.Items {
			img := ""
			if len(d.Spec.Template.Spec.Containers) > 0 {
				img = d.Spec.Template.Spec.Containers[0].Image
			}
			items = append(items, resItem{
				Name: d.Name, Namespace: d.Namespace,
				Ready: fmt.Sprintf("%d/%d", d.Status.ReadyReplicas, ptrInt32(d.Spec.Replicas)),
				Age:   ageTime(d.CreationTimestamp.Time),
				Detail: map[string]string{"image": img},
			})
		}

	case "services", "service", "svc":
		list, err := rc.clientset.CoreV1().Services(ns).List(r.Context(), metav1.ListOptions{Limit: 500})
		if err != nil {
			writeK8sError(w, err)
			return
		}
		for _, sv := range list.Items {
			ports := []string{}
			for _, p := range sv.Spec.Ports {
				ports = append(ports, fmt.Sprintf("%d/%s", p.Port, string(p.Protocol)))
			}
			items = append(items, resItem{
				Name: sv.Name, Namespace: sv.Namespace, Type: string(sv.Spec.Type),
				Age: ageTime(sv.CreationTimestamp.Time),
				Detail: map[string]string{
					"clusterIP": sv.Spec.ClusterIP,
					"ports":     strings.Join(ports, ", "),
				},
			})
		}

	case "ingresses", "ingress":
		list, err := rc.clientset.NetworkingV1().Ingresses(ns).List(r.Context(), metav1.ListOptions{Limit: 500})
		if err != nil {
			writeK8sError(w, err)
			return
		}
		for _, ing := range list.Items {
			hosts := []string{}
			for _, rule := range ing.Spec.Rules {
				if rule.Host != "" {
					hosts = append(hosts, rule.Host)
				}
			}
			cls := ""
			if ing.Spec.IngressClassName != nil {
				cls = *ing.Spec.IngressClassName
			}
			items = append(items, resItem{
				Name: ing.Name, Namespace: ing.Namespace,
				Age: ageTime(ing.CreationTimestamp.Time),
				Detail: map[string]string{"hosts": strings.Join(hosts, ", "), "class": cls},
			})
		}

	case "configmaps", "configmap", "cm":
		list, err := rc.clientset.CoreV1().ConfigMaps(ns).List(r.Context(), metav1.ListOptions{Limit: 500})
		if err != nil {
			writeK8sError(w, err)
			return
		}
		for _, cm := range list.Items {
			items = append(items, resItem{
				Name: cm.Name, Namespace: cm.Namespace, Age: ageTime(cm.CreationTimestamp.Time),
				Detail: map[string]string{"keys": fmt.Sprintf("%d keys", len(cm.Data))},
			})
		}

	case "statefulsets", "statefulset", "sts":
		list, err := rc.clientset.AppsV1().StatefulSets(ns).List(r.Context(), metav1.ListOptions{Limit: 500})
		if err != nil {
			writeK8sError(w, err)
			return
		}
		for _, ss := range list.Items {
			items = append(items, resItem{
				Name: ss.Name, Namespace: ss.Namespace,
				Ready: fmt.Sprintf("%d/%d", ss.Status.ReadyReplicas, ptrInt32(ss.Spec.Replicas)),
				Age:   ageTime(ss.CreationTimestamp.Time),
			})
		}

	case "daemonsets", "daemonset", "ds":
		list, err := rc.clientset.AppsV1().DaemonSets(ns).List(r.Context(), metav1.ListOptions{Limit: 500})
		if err != nil {
			writeK8sError(w, err)
			return
		}
		for _, ds := range list.Items {
			items = append(items, resItem{
				Name: ds.Name, Namespace: ds.Namespace,
				Ready: fmt.Sprintf("%d/%d", ds.Status.NumberReady, ds.Status.DesiredNumberScheduled),
				Age:   ageTime(ds.CreationTimestamp.Time),
			})
		}

	case "namespaces", "namespace", "ns":
		list, err := rc.clientset.CoreV1().Namespaces().List(r.Context(), metav1.ListOptions{Limit: 500})
		if err != nil {
			writeK8sError(w, err)
			return
		}
		for _, n := range list.Items {
			items = append(items, resItem{
				Name: n.Name, Namespace: "", Age: ageTime(n.CreationTimestamp.Time),
				Detail: map[string]string{"status": string(n.Status.Phase)},
			})
		}

	case "pods", "pod":
		list, err := rc.clientset.CoreV1().Pods(ns).List(r.Context(), metav1.ListOptions{Limit: 500})
		if err != nil {
			writeK8sError(w, err)
			return
		}
		for _, p := range list.Items {
			restarts := int32(0)
			for _, cs := range p.Status.ContainerStatuses {
				restarts += cs.RestartCount
			}
			items = append(items, resItem{
				Name: p.Name, Namespace: p.Namespace,
				Ready: fmt.Sprintf("%d/%d", countReadyContainers(p.Status.ContainerStatuses), len(p.Spec.Containers)),
				Age:   ageTime(p.CreationTimestamp.Time),
				Detail: map[string]string{
					"node":     p.Spec.NodeName,
					"status":   string(p.Status.Phase),
					"restarts": fmt.Sprintf("%d", restarts),
				},
			})
		}

	case "secrets", "secret":
		list, err := rc.clientset.CoreV1().Secrets(ns).List(r.Context(), metav1.ListOptions{Limit: 500})
		if err != nil {
			writeK8sError(w, err)
			return
		}
		for _, s := range list.Items {
			items = append(items, resItem{
				Name: s.Name, Namespace: s.Namespace,
				Type: string(s.Type), Age: ageTime(s.CreationTimestamp.Time),
				Detail: map[string]string{"keys": fmt.Sprintf("%d keys", len(s.Data))},
			})
		}

	case "pvc", "persistentvolumeclaims", "persistentvolumeclaim":
		list, err := rc.clientset.CoreV1().PersistentVolumeClaims(ns).List(r.Context(), metav1.ListOptions{Limit: 500})
		if err != nil {
			writeK8sError(w, err)
			return
		}
		for _, p := range list.Items {
			size := ""
			if len(p.Spec.Resources.Requests) > 0 {
				size = p.Spec.Resources.Requests.Storage().String()
			}
			sc := ""
			if p.Spec.StorageClassName != nil {
				sc = *p.Spec.StorageClassName
			}
			items = append(items, resItem{
				Name: p.Name, Namespace: p.Namespace,
				Ready: string(p.Status.Phase), Age: ageTime(p.CreationTimestamp.Time),
				Detail: map[string]string{"capacity": size, "storageClass": sc, "volume": p.Spec.VolumeName},
			})
		}

	case "pv", "persistentvolumes", "persistentvolume":
		list, err := rc.clientset.CoreV1().PersistentVolumes().List(r.Context(), metav1.ListOptions{Limit: 500})
		if err != nil {
			writeK8sError(w, err)
			return
		}
		for _, p := range list.Items {
			items = append(items, resItem{
				Name:  p.Name,
				Ready: string(p.Status.Phase), Age: ageTime(p.CreationTimestamp.Time),
				Detail: map[string]string{
					"capacity":      p.Spec.Capacity.Storage().String(),
					"accessModes":   fmt.Sprintf("%v", p.Spec.AccessModes),
					"reclaimPolicy": string(p.Spec.PersistentVolumeReclaimPolicy),
				},
			})
		}

	case "storageclasses", "storageclass", "sc":
		list, err := rc.clientset.StorageV1().StorageClasses().List(r.Context(), metav1.ListOptions{Limit: 500})
		if err != nil {
			writeK8sError(w, err)
			return
		}
		for _, sc := range list.Items {
			items = append(items, resItem{
				Name: sc.Name, Age: ageTime(sc.CreationTimestamp.Time),
				Detail: map[string]string{
					"provisioner": sc.Provisioner,
					"default":     fmt.Sprintf("%v", sc.Annotations["storageclass.kubernetes.io/is-default-class"] == "true"),
				},
			})
		}

	case "serviceaccounts", "serviceaccount", "sa":
		list, err := rc.clientset.CoreV1().ServiceAccounts(ns).List(r.Context(), metav1.ListOptions{Limit: 500})
		if err != nil {
			writeK8sError(w, err)
			return
		}
		for _, sa := range list.Items {
			items = append(items, resItem{
				Name: sa.Name, Namespace: sa.Namespace, Age: ageTime(sa.CreationTimestamp.Time),
				Detail: map[string]string{"secrets": fmt.Sprintf("%d", len(sa.Secrets))},
			})
		}

	case "jobs", "job":
		list, err := rc.clientset.BatchV1().Jobs(ns).List(r.Context(), metav1.ListOptions{Limit: 500})
		if err != nil {
			writeK8sError(w, err)
			return
		}
		for _, j := range list.Items {
			items = append(items, resItem{
				Name: j.Name, Namespace: j.Namespace,
				Ready: fmt.Sprintf("%d/%d", j.Status.Succeeded, ptrInt32(j.Spec.Completions)),
				Age:   ageTime(j.CreationTimestamp.Time),
				Detail: map[string]string{
					"completions": fmt.Sprintf("%d", j.Status.Succeeded),
				},
			})
		}

	case "cronjobs", "cronjob":
		list, err := rc.clientset.BatchV1().CronJobs(ns).List(r.Context(), metav1.ListOptions{Limit: 500})
		if err != nil {
			writeK8sError(w, err)
			return
		}
		for _, c := range list.Items {
			suspend := "false"
			if c.Spec.Suspend != nil && *c.Spec.Suspend {
				suspend = "true"
			}
			lastRun := "never"
			if c.Status.LastScheduleTime != nil {
				lastRun = c.Status.LastScheduleTime.Format(time.RFC3339)
			}
			items = append(items, resItem{
				Name: c.Name, Namespace: c.Namespace, Age: ageTime(c.CreationTimestamp.Time),
				Detail: map[string]string{
					"schedule": c.Spec.Schedule,
					"suspend":  suspend,
					"lastRun":  lastRun,
				},
			})
		}

	case "roles", "role":
		list, err := rc.clientset.RbacV1().Roles(ns).List(r.Context(), metav1.ListOptions{Limit: 500})
		if err != nil {
			writeK8sError(w, err)
			return
		}
		for _, role := range list.Items {
			rules := []string{}
			for _, rule := range role.Rules {
				rules = append(rules, fmt.Sprintf("[%s] %s/%s",
					strings.Join(rule.Verbs, ","),
					strings.Join(rule.APIGroups, ","),
					strings.Join(rule.Resources, ",")))
			}
			items = append(items, resItem{
				Name: role.Name, Namespace: role.Namespace, Age: ageTime(role.CreationTimestamp.Time),
				Detail: map[string]string{"rules": fmt.Sprintf("%d rules", len(role.Rules)),
					"summary": truncate(strings.Join(rules, "; "), 120)},
			})
		}

	case "rolebindings", "rolebinding", "rb":
		list, err := rc.clientset.RbacV1().RoleBindings(ns).List(r.Context(), metav1.ListOptions{Limit: 500})
		if err != nil {
			writeK8sError(w, err)
			return
		}
		for _, rb := range list.Items {
			subj := []string{}
			for _, s := range rb.Subjects {
				subj = append(subj, s.Kind+":"+s.Name)
			}
			items = append(items, resItem{
				Name: rb.Name, Namespace: rb.Namespace, Age: ageTime(rb.CreationTimestamp.Time),
				Detail: map[string]string{
					"roleRef":  rb.RoleRef.Kind + "/" + rb.RoleRef.Name,
					"subjects": truncate(strings.Join(subj, ", "), 120),
				},
			})
		}

	case "clusterrolebindings", "clusterrolebinding", "crb":
		list, err := rc.clientset.RbacV1().ClusterRoleBindings().List(r.Context(), metav1.ListOptions{Limit: 500})
		if err != nil {
			writeK8sError(w, err)
			return
		}
		for _, crb := range list.Items {
			subj := []string{}
			for _, s := range crb.Subjects {
				subj = append(subj, s.Kind+":"+s.Name)
			}
			items = append(items, resItem{
				Name: crb.Name, Namespace: "", Age: ageTime(crb.CreationTimestamp.Time),
				Detail: map[string]string{
					"roleRef":  crb.RoleRef.Kind + "/" + crb.RoleRef.Name,
					"subjects": truncate(strings.Join(subj, ", "), 120),
				},
			})
		}

	case "networkpolicies", "networkpolicy", "netpol":
		list, err := rc.clientset.NetworkingV1().NetworkPolicies(ns).List(r.Context(), metav1.ListOptions{Limit: 500})
		if err != nil {
			writeK8sError(w, err)
			return
		}
		for _, np := range list.Items {
			items = append(items, resItem{
				Name: np.Name, Namespace: np.Namespace, Age: ageTime(np.CreationTimestamp.Time),
				Detail: map[string]string{
					"ingressRules": fmt.Sprintf("%d", len(np.Spec.Ingress)),
					"egressRules":  fmt.Sprintf("%d", len(np.Spec.Egress)),
				},
			})
		}

	case "hpa", "horizontalpodautoscalers":
		list, err := rc.clientset.AutoscalingV2().HorizontalPodAutoscalers(ns).List(r.Context(), metav1.ListOptions{Limit: 500})
		if err != nil {
			writeK8sError(w, err)
			return
		}
		for _, h := range list.Items {
			minReps := int32(1)
			if h.Spec.MinReplicas != nil {
				minReps = *h.Spec.MinReplicas
			}
			items = append(items, resItem{
				Name: h.Name, Namespace: h.Namespace,
				Ready: fmt.Sprintf("%d/%d", h.Status.CurrentReplicas, minReps),
				Age:   ageTime(h.CreationTimestamp.Time),
				Detail: map[string]string{
					"target": h.Spec.ScaleTargetRef.Kind + "/" + h.Spec.ScaleTargetRef.Name,
					"minMax": fmt.Sprintf("%d-%d", minReps, h.Spec.MaxReplicas),
				},
			})
		}

	default:
		writeError(w, 400, "unsupported kind: "+kind+
			". Supported: deployments, services, ingresses, configmaps, secrets, statefulsets, daemonsets, pods, pvc, pv, storageclasses, serviceaccounts, jobs, cronjobs, roles, rolebindings, clusterrolebindings, networkpolicies, hpa, namespaces")
		return
	}

	writeJSON(w, map[string]any{"kind": kind, "count": len(items), "items": items})
}

// --- shared helpers ---

// ageTime returns a human-readable age string from a creation timestamp.
func ageTime(t time.Time) string {
	d := time.Since(t)
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	if d < time.Hour {
		return fmt.Sprintf("%dm", int(d.Minutes()))
	}
	if d < 24*time.Hour {
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}

// ptrInt32 safely dereferences an *int32, returning 0 for nil.
func ptrInt32(p *int32) int32 {
	if p == nil {
		return 0
	}
	return *p
}

