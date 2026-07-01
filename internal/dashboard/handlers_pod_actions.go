package dashboard

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/remotecommand"
)

// handlePodActions dispatches pod sub-resource requests.
// /api/pods/{ns}/{name}/logs, /api/pods/{ns}/{name}/exec, /api/pods/{ns}/{name}/containers
func (s *Server) handlePodActions(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/pods/")
	parts := strings.Split(path, "/")
	if len(parts) < 3 {
		writeError(w, 400, "expected /api/pods/{ns}/{name}/{action}")
		return
	}
	action := parts[2]
	switch action {
	case "logs":
		s.handlePodLogs(w, r)
	case "exec":
		if r.Method == http.MethodPost {
			s.handlePodExec(w, r)
		} else {
			writeError(w, 405, "POST required for exec")
		}
	case "containers":
		s.handleContainers(w, r)
	default:
		writeError(w, 400, "unknown action: "+action)
	}
}

// handlePodLogs streams pod logs via SSE (follow mode).
// GET /api/pods/{ns}/{name}/logs?container=&follow=true&tailLines=500
func (s *Server) handlePodLogs(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/pods/"), "/")
	if len(parts) < 2 {
		writeError(w, 400, "invalid path, expected /api/pods/{ns}/{name}/logs")
		return
	}
	ns, name := parts[0], parts[1]

	container := r.URL.Query().Get("container")
	follow := r.URL.Query().Get("follow") == "true"
	tailLines := int64(500)
	if tl := r.URL.Query().Get("tailLines"); tl != "" {
		_, _ = fmt.Sscanf(tl, "%d", &tailLines)
	}

	opts := &corev1.PodLogOptions{Container: container, Follow: follow}
	if tailLines > 0 {
		opts.TailLines = &tailLines
	}

	req := rc.clientset.CoreV1().Pods(ns).GetLogs(name, opts)
	stream, err := req.Stream(r.Context())
	if err != nil {
		writeError(w, 500, fmt.Sprintf("failed to open log stream: %v", err))
		return
	}
	defer stream.Close()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeError(w, 500, "streaming not supported")
		return
	}

	scanner := bufio.NewScanner(stream)
	scanner.Buffer(make([]byte, 0, 256*1024), 256*1024)
	for scanner.Scan() {
		line := scanner.Text()
		data, _ := json.Marshal(map[string]string{"line": line})
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}
	fmt.Fprintf(w, "data: {\"done\":true}\n\n")
	flusher.Flush()
}

// handlePodExec runs a one-shot command in a pod.
// POST /api/pods/{ns}/{name}/exec  body: {"command":"...","container":""}
func (s *Server) handlePodExec(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/pods/"), "/")
	if len(parts) < 3 || parts[2] != "exec" {
		writeError(w, 400, "invalid path, expected /api/pods/{ns}/{name}/exec")
		return
	}
	ns, name := parts[0], parts[1]

	var req struct {
		Command   string `json:"command"`
		Container string `json:"container"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, 400, "invalid body")
		return
	}
	if req.Command == "" {
		req.Command = "env | head -20"
	}

	// Execute command via Kubernetes exec API
	req2 := rc.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(ns).
		Name(name).
		SubResource("exec").
		Param("container", req.Container).
		Param("command", "/bin/sh").
		Param("command", "-c").
		Param("command", req.Command).
		Param("stdout", "true").
		Param("stderr", "true")

	executor, err := remotecommand.NewSPDYExecutor(rc.restConfig, "POST", req2.URL())
	if err != nil {
		writeError(w, 500, fmt.Sprintf("failed to create executor: %v", err))
		return
	}

	var stdout, stderr strings.Builder
	err = executor.StreamWithContext(r.Context(), remotecommand.StreamOptions{
		Stdout: &stdout,
		Stderr: &stderr,
	})

	output := stdout.String()
	errMsg := ""
	if err != nil {
		errMsg = err.Error()
	}
	if stderr.Len() > 0 {
		if output != "" {
			output += "\n"
		}
		output += "[stderr]\n" + stderr.String()
	}

	writeJSON(w, map[string]any{
		"success": err == nil,
		"output":  output,
		"error":   errMsg,
	})
}

// handleContainers returns containers for a pod (for log selector).
// GET /api/pods/{ns}/{name}/containers
func (s *Server) handleContainers(w http.ResponseWriter, r *http.Request) {
	rc := s.clientsFromReq(r)
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/api/pods/"), "/")
	if len(parts) < 2 {
		writeError(w, 400, "invalid path")
		return
	}
	ns, name := parts[0], parts[1]

	pod, err := rc.clientset.CoreV1().Pods(ns).Get(r.Context(), name, metav1.GetOptions{})
	if err != nil {
		writeK8sError(w, err)
		return
	}

	type cInfo struct {
		Name  string `json:"name"`
		Image string `json:"image"`
		Ready bool   `json:"ready"`
	}
	containers := make([]cInfo, 0)
	readyMap := map[string]bool{}
	for _, cs := range pod.Status.ContainerStatuses {
		readyMap[cs.Name] = cs.Ready
	}
	for _, c := range pod.Spec.Containers {
		containers = append(containers, cInfo{
			Name: c.Name, Image: c.Image, Ready: readyMap[c.Name],
		})
	}
	writeJSON(w, map[string]any{"containers": containers})
}

