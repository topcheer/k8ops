package dashboard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// handleEventsStream is a Server-Sent Events endpoint that streams
// Kubernetes events in real-time using a Watch. The client connects
// with EventSource and receives JSON-formatted events as they occur.
//
// Query params:
//   - namespace: filter by namespace (default: all namespaces)
//   - warning: if "true", only stream Warning events
//
// The stream automatically closes after 30 minutes to prevent leaks.
func (s *Server) handleEventsStream(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	if rc.clientset == nil {
		writeError(w, 503, "k8s client not available")
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "streaming not supported")
		return
	}

	namespace := r.URL.Query().Get("namespace")
	warningOnly := r.URL.Query().Get("warning") == "true"

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	// Timeout context to prevent indefinite connections
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Minute)
	defer cancel()

	// Start with a resource version to avoid replaying all history
	listOpts := metav1.ListOptions{
		Limit:         1,
		FieldSelector: "",
	}
	if warningOnly {
		listOpts.FieldSelector = "type=Warning"
	}

	// Get initial resource version
	list, err := rc.clientset.CoreV1().Events(namespace).List(ctx, listOpts)
	if err != nil {
		fmt.Fprintf(w, "data: {\"error\":\"%s\"}\n\n", escapeJSON(err.Error()))
		flusher.Flush()
		return
	}
	resourceVersion := list.ResourceVersion

	// Start watching from current state forward
	watchOpts := metav1.ListOptions{
		ResourceVersion: resourceVersion,
		FieldSelector:   listOpts.FieldSelector,
		Watch:           true,
	}

	watcher, err := rc.clientset.CoreV1().Events(namespace).Watch(ctx, watchOpts)
	if err != nil {
		fmt.Fprintf(w, "data: {\"error\":\"%s\"}\n\n", escapeJSON(err.Error()))
		flusher.Flush()
		return
	}
	defer watcher.Stop()

	// Send heartbeat every 15 seconds to keep connection alive
	heartbeat := time.NewTicker(15 * time.Second)
	defer heartbeat.Stop()

	type eventInfo struct {
		Type      string `json:"type"`
		Reason    string `json:"reason"`
		Message   string `json:"message"`
		Object    string `json:"object"`
		Namespace string `json:"namespace"`
		Count     int32  `json:"count"`
		LastTime  string `json:"lastTime"`
		WatchType string `json:"watchType"` // ADDED, MODIFIED, DELETED
	}

	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()
		case event, ok := <-watcher.ResultChan():
			if !ok {
				// Watch channel closed, try to reconnect
				fmt.Fprintf(w, "event: reconnect\ndata: {\"message\":\"watch closed, reconnecting\"}\n\n")
				flusher.Flush()
				return
			}

			k8sEvent, ok := event.Object.(*corev1.Event)
			if !ok {
				continue
			}

			info := eventInfo{
				Type:      k8sEvent.Type,
				Reason:    k8sEvent.Reason,
				Message:   truncate(k8sEvent.Message, 300),
				Object:    fmt.Sprintf("%s/%s", k8sEvent.InvolvedObject.Kind, k8sEvent.InvolvedObject.Name),
				Namespace: k8sEvent.InvolvedObject.Namespace,
				Count:     k8sEvent.Count,
				LastTime:  k8sEvent.LastTimestamp.Format(time.RFC3339),
				WatchType: string(event.Type),
			}

			data, _ := json.Marshal(info)
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// escapeJSON escapes a string for safe inclusion in a JSON string value.
func escapeJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b[1 : len(b)-1]) // strip surrounding quotes
}
