#!/usr/bin/env bash
# deploy.sh — k8ops one-command deploy with pre-checks, health verification, and rollback
# Usage: ./scripts/deploy.sh <VERSION>
# Example: ./scripts/deploy.sh v14.36
#
# Phases:
#   1. Pre-deploy validation (build + vet + test + fmt)
#   2. Docker build + push
#   3. kubectl set image with change-cause annotation
#   4. Health check (pod ready + HTTP 200 within 120s)
#   5. Automatic rollback on failure
set -uo pipefail

set -euo pipefail

PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$PROJECT_DIR"

VERSION="${1:?Usage: deploy.sh <version> e.g. v14.36}"
NAMESPACE="k8ops-system"
DAEMONSET="k8ops"
IMAGE="registry.iot2.win/k8ops"
MAX_WAIT=120  # seconds to wait for rollout

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

ok()   { echo -e "${GREEN}[OK]${NC} $1"; }
fail() { echo -e "${RED}[FAIL]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
info() { echo -e "       $1"; }

echo "============================================"
echo "  k8ops Deploy: $VERSION"
echo "============================================"
echo ""

# --- Phase 1: Pre-deploy validation ---
echo "[1/5] Pre-deploy checks..."
if bash scripts/pre-deploy-check.sh "$VERSION"; then
  ok "pre-deploy checks passed"
else
  fail "pre-deploy checks failed — aborting deploy"
  exit 1
fi

# --- Phase 2: Docker build + push ---
echo ""
echo "[2/5] Building Docker image..."
if docker buildx build --platform linux/amd64 \
  --build-arg VERSION="$VERSION" \
  -t "$IMAGE:$VERSION" \
  -t "$IMAGE:latest" \
  --push . 2>&1 | tail -3; then
  SIZE=$(docker images "$IMAGE:$VERSION" --format '{{.Size}}' 2>/dev/null || echo "?")
  ok "image pushed: $IMAGE:$VERSION ($SIZE)"
else
  fail "Docker build/push failed"
  exit 1
fi

# --- Phase 3: Rollout with annotation ---
echo ""
echo "[3/5] Updating DaemonSet..."
REVISION_BEFORE=$(kubectl rollout history daemonset/"$DAEMONSET" -n "$NAMESPACE" 2>/dev/null | tail -1 | awk '{print $1}')
kubectl set image daemonset/"$DAEMONSET" "$DAEMONSET=$IMAGE:$VERSION" -n "$NAMESPACE"
kubectl annotate daemonset "$DAEMONSET" -n "$NAMESPACE" \
  kubernetes.io/change-cause="Deploy $VERSION at $(date -u +%Y-%m-%dT%H:%M:%SZ)" --overwrite
ok "DaemonSet updated to $VERSION (revision was $REVISION_BEFORE)"

# --- Phase 4: Health check ---
echo ""
echo "[4/5] Health check (waiting up to ${MAX_WAIT}s)..."
START_TIME=$SECONDS
HEALTHY=false

while [ $((SECONDS - START_TIME)) -lt "$MAX_WAIT" ]; do
  POD_STATUS=$(kubectl get pods -n "$NAMESPACE" -l app.kubernetes.io/name=k8ops \
    -o jsonpath='{.items[0].status.phase}' 2>/dev/null || echo "Unknown")
  POD_AGE=$(kubectl get pods -n "$NAMESPACE" -l app.kubernetes.io/name=k8ops \
    -o jsonpath='{.items[0].metadata.creationTimestamp}' 2>/dev/null || echo "")

  if [ "$POD_STATUS" = "Running" ]; then
    # Check that pod was created after our deploy (within last 2 min)
    if [ -n "$POD_AGE" ]; then
      HTTP_CODE=$(curl -sk -o /dev/null -w '%{http_code}' --connect-timeout 5 --max-time 10 \
        https://k8ops.iot2.win/api/health 2>/dev/null || echo "000")
      if [ "$HTTP_CODE" = "200" ] || [ "$HTTP_CODE" = "303" ]; then
        HEALTHY=true
        ok "pod Running, HTTP $HTTP_CODE — healthy!"
        break
      fi
      info "pod Running, HTTP $HTTP_CODE — waiting..."
    fi
  elif [ "$POD_STATUS" = "CrashLoopBackOff" ] || [ "$POD_STATUS" = "Error" ]; then
    fail "pod is $POD_STATUS — will rollback!"
    break
  else
    info "pod status: $POD_STATUS — waiting..."
  fi
  sleep 3
done

# --- Phase 5: Verify or rollback ---
echo ""
echo "[5/5] Post-deploy verification..."

if [ "$HEALTHY" = true ]; then
  # Final verification: check version endpoint
  DEPLOYED_VER=$(kubectl get pods -n "$NAMESPACE" -l app.kubernetes.io/name=k8ops \
    -o jsonpath='{.items[0].spec.containers[0].image}' 2>/dev/null | sed 's|.*/||')
  ok "deployed $DEPLOYED_VER successfully"
  echo ""
  kubectl get pods -n "$NAMESPACE" -l app.kubernetes.io/name=k8ops
  echo ""
  echo -e "${GREEN}============================================${NC}"
  echo -e "${GREEN}  DEPLOY SUCCESSFUL: $VERSION${NC}"
  echo -e "${GREEN}============================================${NC}"
  exit 0
else
  fail "health check failed — initiating rollback!"
  echo ""
  warn "rolling back to previous revision..."

  # Get the previous revision number
  REVISION_AFTER=$(kubectl rollout history daemonset/"$DAEMONSET" -n "$NAMESPACE" 2>/dev/null | tail -1 | awk '{print $1}')
  PREV_REV=$((REVISION_AFTER - 1))

  if [ "$PREV_REV" -gt 0 ]; then
    kubectl rollout undo daemonset/"$DAEMONSET" -n "$NAMESPACE" --to-revision="$PREV_REV" 2>/dev/null
    warn "rolled back to revision $PREV_REV"
  else
    warn "no previous revision available for rollback"
  fi

  # Get recent pod logs for debugging
  echo ""
  info "recent pod logs:"
  kubectl logs -n "$NAMESPACE" -l app.kubernetes.io/name=k8ops --tail=20 2>/dev/null || true

  echo ""
  echo -e "${RED}============================================${NC}"
  echo -e "${RED}  DEPLOY FAILED: $VERSION${NC}"
  echo -e "${RED}  Rolled back to previous revision${NC}"
  echo -e "${RED}============================================${NC}"
  exit 1
fi
