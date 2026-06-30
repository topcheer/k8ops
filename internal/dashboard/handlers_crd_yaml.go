package dashboard

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	sigsyaml "sigs.k8s.io/yaml"
)

// handleCRDs lists all CustomResourceDefinitions.
func (s *Server) handleCRDs(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	withCounts := r.URL.Query().Get("with_counts") == "true"

	crdList := &apiextensionsv1.CustomResourceDefinitionList{}
	if err := rc.ctrlClient.List(r.Context(), crdList); err != nil {
		writeK8sError(w, err)
		return
	}

	type crdItem struct {
		Name    string `json:"name"`
		Group   string `json:"group"`
		Version string `json:"version"`
		Kind    string `json:"kind"`
		Plural  string `json:"plural"`
		Scope   string `json:"scope"`
		Count   int    `json:"count,omitempty"`
	}
	items := make([]crdItem, 0, len(crdList.Items))

	dyn := s.k8sClientTool.DynamicClient()

	for _, crd := range crdList.Items {
		ver := ""
		for _, v := range crd.Spec.Versions {
			if v.Served {
				ver = v.Name
				break
			}
		}

		item := crdItem{
			Name: crd.Name, Group: crd.Spec.Group, Version: ver,
			Kind: crd.Spec.Names.Kind, Plural: crd.Spec.Names.Plural,
			Scope: string(crd.Spec.Scope),
		}

		if withCounts && dyn != nil && ver != "" {
			gvr := schema.GroupVersionResource{
				Group: crd.Spec.Group, Version: ver, Resource: crd.Spec.Names.Plural,
			}
			list, err := dyn.Resource(gvr).List(r.Context(), metav1.ListOptions{Limit: 1})
			if err == nil && list != nil {
				item.Count = len(list.Items)
			}
		}

		items = append(items, item)
	}
	writeJSON(w, map[string]any{"count": len(items), "items": items})
}

// handleCRDResources lists instances of a CRD via dynamic client.
func (s *Server) handleCRDResources(w http.ResponseWriter, r *http.Request) {
	group := r.URL.Query().Get("group")
	version := r.URL.Query().Get("version")
	resource := r.URL.Query().Get("resource")
	ns := r.URL.Query().Get("namespace")

	if resource == "" || version == "" {
		writeError(w, 400, "resource and version are required")
		return
	}

	gvr := schema.GroupVersionResource{Group: group, Version: version, Resource: resource}

	dyn := s.k8sClientTool.DynamicClient()
	if dyn == nil {
		writeError(w, 500, "dynamic client not available")
		return
	}

	var list *unstructured.UnstructuredList
	var err error
	if ns != "" {
		list, err = dyn.Resource(gvr).Namespace(ns).List(r.Context(), metav1.ListOptions{Limit: 500})
	} else {
		list, err = dyn.Resource(gvr).List(r.Context(), metav1.ListOptions{Limit: 500})
	}
	if err != nil {
		writeK8sError(w, err)
		return
	}

	type rawItem struct {
		Name      string            `json:"name"`
		Namespace string            `json:"namespace"`
		Age       string            `json:"age"`
		Detail    map[string]string `json:"detail"`
	}
	items := make([]rawItem, 0, len(list.Items))
	for _, u := range list.Items {
		detail := map[string]string{}
		if spec, ok := u.Object["spec"].(map[string]any); ok {
			cnt := 0
			for k, v := range spec {
				detail[k] = fmt.Sprintf("%v", v)
				cnt++
				if cnt >= 5 {
					break
				}
			}
		}
		items = append(items, rawItem{
			Name: u.GetName(), Namespace: u.GetNamespace(),
			Age:    ageTime(u.GetCreationTimestamp().Time),
			Detail: detail,
		})
	}
	writeJSON(w, map[string]any{"gvr": fmt.Sprintf("%s/%s/%s", group, version, resource), "count": len(items), "items": items})
}

// handleYAML returns the full YAML definition of any resource.
func (s *Server) handleYAML(w http.ResponseWriter, r *http.Request) {
	ns := r.URL.Query().Get("namespace")
	name := r.URL.Query().Get("name")
	group := r.URL.Query().Get("group")
	version := r.URL.Query().Get("version")
	resource := r.URL.Query().Get("resource")
	kind := strings.ToLower(r.URL.Query().Get("kind"))

	if name == "" {
		writeError(w, 400, "name is required")
		return
	}

	var obj *unstructured.Unstructured

	if group != "" || resource != "" {
		if resource == "" {
			writeError(w, 400, "resource is required for dynamic resources")
			return
		}
		gvr := schema.GroupVersionResource{Group: group, Version: version, Resource: resource}
		dyn := s.k8sClientTool.DynamicClient()
		if dyn == nil {
			writeError(w, 500, "dynamic client not available")
			return
		}
		var err error
		if ns != "" {
			obj, err = dyn.Resource(gvr).Namespace(ns).Get(r.Context(), name, metav1.GetOptions{})
		} else {
			obj, err = dyn.Resource(gvr).Get(r.Context(), name, metav1.GetOptions{})
		}
		if err != nil {
			writeK8sError(w, err)
			return
		}
	} else {
		gvr, err := builtinGVR(kind)
		if err != nil {
			writeError(w, 400, err.Error())
			return
		}
		dyn := s.k8sClientTool.DynamicClient()
		if dyn == nil {
			writeError(w, 500, "dynamic client not available")
			return
		}
		if ns != "" {
			obj, err = dyn.Resource(gvr).Namespace(ns).Get(r.Context(), name, metav1.GetOptions{})
		} else {
			obj, err = dyn.Resource(gvr).Get(r.Context(), name, metav1.GetOptions{})
		}
		if err != nil {
			writeK8sError(w, err)
			return
		}
	}

	yamlBytes, err := yamlMarshal(obj.Object)
	if err != nil {
		writeError(w, 500, fmt.Sprintf("failed to marshal YAML: %v", err))
		return
	}

	writeJSON(w, map[string]any{
		"name": name,
		"yaml": string(yamlBytes),
	})
}

// builtinGVR maps common resource kind names to their GVR.
func builtinGVR(kind string) (schema.GroupVersionResource, error) {
	switch kind {
	case "pods", "pod", "po":
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "pods"}, nil
	case "services", "service", "svc":
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "services"}, nil
	case "namespaces", "namespace", "ns":
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "namespaces"}, nil
	case "configmaps", "configmap", "cm":
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}, nil
	case "secrets", "secret":
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "secrets"}, nil
	case "nodes", "node", "no":
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "nodes"}, nil
	case "deployments", "deployment", "deploy":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "deployments"}, nil
	case "statefulsets", "statefulset", "sts":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "statefulsets"}, nil
	case "daemonsets", "daemonset", "ds":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "daemonsets"}, nil
	case "ingresses", "ingress", "ing":
		return schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "ingresses"}, nil
	case "replicasets", "replicaset", "rs":
		return schema.GroupVersionResource{Group: "apps", Version: "v1", Resource: "replicasets"}, nil
	case "jobs":
		return schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "jobs"}, nil
	case "cronjobs":
		return schema.GroupVersionResource{Group: "batch", Version: "v1", Resource: "cronjobs"}, nil
	case "persistentvolumes", "persistentvolume", "pv":
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "persistentvolumes"}, nil
	case "persistentvolumeclaims", "persistentvolumeclaim", "pvc":
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "persistentvolumeclaims"}, nil
	case "serviceaccounts", "serviceaccount", "sa":
		return schema.GroupVersionResource{Group: "", Version: "v1", Resource: "serviceaccounts"}, nil
	case "roles":
		return schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "roles"}, nil
	case "rolebindings":
		return schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "rolebindings"}, nil
	case "clusterroles":
		return schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterroles"}, nil
	case "clusterrolebindings":
		return schema.GroupVersionResource{Group: "rbac.authorization.k8s.io", Version: "v1", Resource: "clusterrolebindings"}, nil
	case "networkpolicies", "networkpolicy", "netpol":
		return schema.GroupVersionResource{Group: "networking.k8s.io", Version: "v1", Resource: "networkpolicies"}, nil
	case "storageclasses", "storageclass", "sc":
		return schema.GroupVersionResource{Group: "storage.k8s.io", Version: "v1", Resource: "storageclasses"}, nil
	case "hpas", "horizontalpodautoscaler":
		return schema.GroupVersionResource{Group: "autoscaling", Version: "v2", Resource: "horizontalpodautoscalers"}, nil
	default:
		return schema.GroupVersionResource{}, fmt.Errorf("unsupported kind: %s", kind)
	}
}

// yamlMarshal converts a map to YAML using sigs.k8s.io/yaml.
func yamlMarshal(obj map[string]any) ([]byte, error) {
	jsonBytes, err := json.Marshal(obj)
	if err != nil {
		return nil, err
	}
	return sigsyaml.JSONToYAML(jsonBytes)
}
