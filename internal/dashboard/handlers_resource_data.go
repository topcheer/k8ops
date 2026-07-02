package dashboard

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"sort"
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// handleResourceData returns the data content of a ConfigMap or Secret.
// GET /api/resource/data?kind=configmap&namespace=default&name=my-config
func (s *Server) handleResourceData(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	kind := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("kind")))
	ns := strings.TrimSpace(r.URL.Query().Get("namespace"))
	name := strings.TrimSpace(r.URL.Query().Get("name"))

	if ns == "" || name == "" {
		writeError(w, http.StatusBadRequest, "namespace and name are required")
		return
	}

	rc := s.clientsFromReq(r)
	if rc == nil || rc.clientset == nil {
		writeError(w, http.StatusServiceUnavailable, "kubernetes client not available")
		return
	}

	type dataItem struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	type resourceData struct {
		Kind      string     `json:"kind"`
		Namespace string     `json:"namespace"`
		Name      string     `json:"name"`
		Items     []dataItem `json:"items"`
		Count     int        `json:"count"`
	}

	result := resourceData{Kind: kind, Namespace: ns, Name: name}

	switch kind {
	case "configmap", "configmaps", "cm":
		cm, err := rc.clientset.CoreV1().ConfigMaps(ns).Get(r.Context(), name, metav1.GetOptions{})
		if err != nil {
			writeError(w, http.StatusNotFound, fmt.Sprintf("configmap not found: %v", err))
			return
		}
		keys := make([]string, 0, len(cm.Data))
		for k := range cm.Data {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			result.Items = append(result.Items, dataItem{Key: k, Value: cm.Data[k]})
		}

	case "secret", "secrets":
		sec, err := rc.clientset.CoreV1().Secrets(ns).Get(r.Context(), name, metav1.GetOptions{})
		if err != nil {
			writeError(w, http.StatusNotFound, fmt.Sprintf("secret not found: %v", err))
			return
		}
		keys := make([]string, 0, len(sec.Data))
		for k := range sec.Data {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			decoded := ""
			if decodedBytes, err := base64.StdEncoding.DecodeString(string(sec.Data[k])); err == nil {
				decoded = string(decodedBytes)
			} else {
				decoded = fmt.Sprintf("<decode error: %v>", err)
			}
			result.Items = append(result.Items, dataItem{Key: k, Value: decoded})
		}

	default:
		writeError(w, http.StatusBadRequest, "only configmap and secret are supported for data viewer")
		return
	}

	result.Count = len(result.Items)
	writeJSON(w, result)
}
