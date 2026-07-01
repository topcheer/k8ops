#!/bin/bash
# =============================================================================
# k8ops — One-click install/uninstall
# =============================================================================
# Usage:
#   ./install.sh          # Install (Deployment mode)
#   ./install.sh daemonset # Install (DaemonSet mode)
#   ./install.sh uninstall # Remove everything
#   ./install.sh status    # Show status
#   ./install.sh port-forward [port]  # Forward dashboard port
set -euo pipefail

# --- Config ---
NAMESPACE="k8ops-system"
OVERLAY_DIR="config/deploy/overlays/local"
DAEMONSET_DIR="config/daemonset"
DASHBOARD_PORT="${2:-9090}"
REGISTRY="registry.iot2.win/k8ops"
IMAGE_TAG="latest"

# --- Helpers ---
log()  { echo -e "\033[36m[k8ops]\033[0m $*"; }
ok()   { echo -e "\033[32m✓\033[0m $*"; }
err()  { echo -e "\033[31m✗\033[0m $*" >&2; }
wait_ready() {
  local resource=$1 name=$2 ns=$3 timeout=${4:-120}
  log "Waiting for $resource/$name to be ready (timeout ${timeout}s)..."
  kubectl wait --for=condition=Ready "$resource/$name" -n "$ns" --timeout="${timeout}s" 2>/dev/null || {
    err "Timeout waiting for $resource/$name"
    kubectl describe "$resource/$name" -n "$ns" 2>/dev/null | tail -20
    return 1
  }
  ok "$resource/$name is ready"
}

# --- Commands ---
cmd_install() {
  local mode="${1:-deployment}"
  log "Installing k8ops ($mode mode)..."

  # Apply kustomize (namespace + secrets + configmap + workload + service + ingress + PVC all at once)
  if [[ "$mode" == "daemonset" ]]; then
    kubectl apply -k "$DAEMONSET_DIR" 2>/dev/null || {
      err "DaemonSet kustomize not found, falling back to local overlay"
      kubectl apply -k "$OVERLAY_DIR"
    }
  else
    kubectl apply -k "$OVERLAY_DIR"
  fi

  # Wait for pods
  log "Waiting for pods to start..."
  sleep 3
  kubectl wait --for=condition=Ready pod -l app.kubernetes.io/name=k8ops -n "$NAMESPACE" --timeout=120s 2>/dev/null || {
    log "Pods not ready yet, checking status..."
    kubectl get pods -n "$NAMESPACE"
  }

  # Show result
  ok "k8ops installed successfully!"
  echo ""
  cmd_status
  echo ""
  log "Access dashboard:"
  log "  Browser:  https://k8ops.iot2.win"
  log "  Port-fwd: ./install.sh port-forward"
}

cmd_uninstall() {
  log "Uninstalling k8ops..."
  kubectl delete -k "$OVERLAY_DIR" --ignore-not-found=true 2>/dev/null
  kubectl delete -k "$DAEMONSET_DIR" --ignore-not-found=true 2>/dev/null
  kubectl delete namespace "$NAMESPACE" --ignore-not-found=true 2>/dev/null
  ok "k8ops completely removed."
}

cmd_status() {
  log "Status:"
  kubectl get all -n "$NAMESPACE" 2>/dev/null || err "Namespace $NAMESPACE not found"
  echo ""
  kubectl get ingress -n "$NAMESPACE" 2>/dev/null
}

cmd_portforward() {
  local port="${1:-$DASHBOARD_PORT}"
  log "Port-forwarding dashboard to http://localhost:$port"
  log "Press Ctrl+C to stop."
  kubectl port-forward svc/k8ops-dashboard "$port:9090" -n "$NAMESPACE"
}

cmd_logs() {
  kubectl logs -f -l app.kubernetes.io/name=k8ops -n "$NAMESPACE" --tail=50
}

# --- Main ---
case "${1:-install}" in
  install|deploy)
    cmd_install "${2:-deployment}"
    ;;
  daemonset|ds)
    cmd_install daemonset
    ;;
  uninstall|remove|delete)
    cmd_uninstall
    ;;
  status)
    cmd_status
    ;;
  port-forward|forward|pf)
    cmd_portforward "${2:-9090}"
    ;;
  logs)
    cmd_logs
    ;;
  *)
    echo "Usage: $0 {install|daemonset|uninstall|status|port-forward|logs}"
    echo ""
    echo "Commands:"
    echo "  install [deployment]  Install k8ops (Deployment mode, default)"
    echo "  daemonset             Install k8ops (DaemonSet mode)"
    echo "  uninstall             Remove k8ops completely"
    echo "  status                Show deployment status"
    echo "  port-forward [port]   Forward dashboard to localhost (default: 9090)"
    echo "  logs                  Tail pod logs"
    exit 1
    ;;
esac
