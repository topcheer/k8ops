#!/usr/bin/env bash
# rollback.sh — Quick rollback for k8ops DaemonSet to a previous revision
# Usage:
#   ./scripts/rollback.sh              # rollback to previous revision
#   ./scripts/rollback.sh <revision>   # rollback to specific revision number
#   ./scripts/rollback.sh <version>    # rollback to specific image version (e.g. v14.30)
set -euo pipefail

NAMESPACE="k8ops-system"
DAEMONSET="k8ops"
IMAGE="registry.iot2.win/k8ops"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

ok()   { echo -e "${GREEN}[OK]${NC} $1"; }
fail() { echo -e "${RED}[FAIL]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
info() { echo -e "       $1"; }

TARGET="${1:-}"

echo "============================================"
echo "  k8ops Rollback"
echo "============================================"

# Show current state
CURRENT_IMAGE=$(kubectl get daemonset "$DAEMONSET" -n "$NAMESPACE" \
  -o jsonpath='{.spec.template.spec.containers[0].image}' 2>/dev/null || echo "unknown")
echo "  Current image: $CURRENT_IMAGE"
echo ""

# Show recent revisions
echo "  Recent rollout history:"
kubectl rollout history daemonset/"$DAEMONSET" -n "$NAMESPACE" 2>/dev/null | tail -10
echo ""

if [ -z "$TARGET" ]; then
  # No argument — rollback to previous revision
  echo "Rolling back to previous revision..."
  kubectl rollout undo daemonset/"$DAEMONSET" -n "$NAMESPACE"
  ok "rollback initiated"
elif [[ "$TARGET" =~ ^[0-9]+$ ]]; then
  # Numeric — treat as revision number
  echo "Rolling back to revision $TARGET..."
  kubectl rollout undo daemonset/"$DAEMONSET" -n "$NAMESPACE" --to-revision="$TARGET"
  ok "rollback to revision $TARGET initiated"
elif [[ "$TARGET" =~ ^v[0-9]+ ]]; then
  # Version string (v14.xx) — set image directly
  echo "Setting image to $IMAGE:$TARGET..."
  kubectl set image daemonset/"$DAEMONSET" \
    "$DAEMONSET=$IMAGE:$TARGET" -n "$NAMESPACE"
  kubectl annotate daemonset "$DAEMONSET" -n "$NAMESPACE" \
    kubernetes.io/change-cause="Rollback to $TARGET at $(date -u +%Y-%m-%dT%H:%M:%SZ)" --overwrite
  ok "image set to $IMAGE:$TARGET"
else
  fail "invalid target: $TARGET (use revision number, version vXX, or empty for previous)"
  exit 1
fi

# Wait for rollout to complete
echo ""
echo "Waiting for rollout to complete (up to 120s)..."
START_TIME=$SECONDS
while [ $((SECONDS - START_TIME)) -lt 120 ]; do
  POD_STATUS=$(kubectl get pods -n "$NAMESPACE" -l app.kubernetes.io/name=k8ops \
    -o jsonpath='{.items[0].status.phase}' 2>/dev/null || echo "Unknown")
  if [ "$POD_STATUS" = "Running" ]; then
    HTTP_CODE=$(curl -sk -o /dev/null -w '%{http_code}' --connect-timeout 5 \
      https://k8ops.iot2.win/api/health 2>/dev/null || echo "000")
    if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "303" ]; then
      ok "pod Running, HTTP $HTTP_CODE"
      echo ""
      kubectl get pods -n "$NAMESPACE" -l app.kubernetes.io/name=k8ops
      echo ""
      NEW_IMAGE=$(kubectl get daemonset "$DAEMONSET" -n "$NAMESPACE" \
        -o jsonpath='{.spec.template.spec.containers[0].image}' 2>/dev/null)
      echo -e "${GREEN}============================================${NC}"
      echo -e "${GREEN}  ROLLBACK COMPLETE${NC}"
      echo -e "${GREEN}  Image: $NEW_IMAGE${NC}"
      echo -e "${GREEN}============================================${NC}"
      exit 0
    fi
    info "pod Running, HTTP $HTTP_CODE — waiting..."
  else
    info "pod status: $POD_STATUS"
  fi
  sleep 3
done

fail "rollback timed out — check pod status manually"
kubectl get pods -n "$NAMESPACE" -l app.kubernetes.io/name=k8ops
exit 1
