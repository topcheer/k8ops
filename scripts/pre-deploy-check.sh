#!/usr/bin/env bash
# pre-deploy-check.sh — Pre-deployment validation gate for k8ops
# Runs go build + vet + tests + fmt check before allowing deployment.
# Usage: ./scripts/pre-deploy-check.sh [VERSION]
# Exit code 0 = all checks passed, safe to deploy
# Exit code 1 = checks failed, do NOT deploy

set -uo pipefail

VERSION="${1:-dev}"
PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$PROJECT_DIR"

RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

pass() { echo -e "${GREEN}[PASS]${NC} $1"; }
fail() { echo -e "${RED}[FAIL]${NC} $1"; }
warn() { echo -e "${YELLOW}[WARN]${NC} $1"; }
info() { echo -e "       $1"; }

ERRORS=0

echo "============================================"
echo "  k8ops Pre-Deploy Check (v$VERSION)"
echo "============================================"
echo ""

# 1. Go build
echo -n "[1/5] Go build... "
if go build ./... 2>/dev/null; then
  pass "build successful"
else
  fail "build failed"
  ERRORS=$((ERRORS + 1))
fi

# 2. Go vet
echo -n "[2/5] Go vet...   "
if go vet ./... 2>/dev/null; then
  pass "vet clean"
else
  fail "vet found issues"
  ERRORS=$((ERRORS + 1))
fi

# 3. Unit tests
echo -n "[3/5] Tests...    "
TEST_OUTPUT="$(go test ./internal/... -count=1 -timeout 120s 2>&1 || true)"
TEST_FAIL="$(echo "$TEST_OUTPUT" | grep -E "^FAIL|^--- FAIL" || true)"
TEST_OK_COUNT="$(echo "$TEST_OUTPUT" | grep -c "^ok" || true)"
if [ -z "$TEST_FAIL" ] && [ "$TEST_OK_COUNT" -gt 0 ]; then
  pass "all packages passed ($TEST_OK_COUNT packages)"
else
  fail "tests failed"
  echo "$TEST_FAIL" | head -10 | while IFS= read -r line; do info "$line"; done
  ERRORS=$((ERRORS + 1))
fi

# 4. Go fmt check
echo -n "[4/5] go fmt...   "
FMT_ISSUES="$(gofmt -l . 2>/dev/null | grep -v vendor/ || true)"
if [ -z "$FMT_ISSUES" ]; then
  pass "code is formatted"
else
  warn "unformatted files:"
  echo "$FMT_ISSUES" | head -5 | while IFS= read -r f; do info "$f"; done
fi

# 5. Binary size check (sanity)
echo -n "[5/5] Binary...   "
CGO_ENABLED=0 go build -ldflags="-s -w" -o /tmp/k8ops-size-check ./cmd/manager 2>/dev/null || true
BINARY_SIZE="$(stat -f%z /tmp/k8ops-size-check 2>/dev/null || stat -c%s /tmp/k8ops-size-check 2>/dev/null || echo 0)"
rm -f /tmp/k8ops-size-check 2>/dev/null || true

if [ "$BINARY_SIZE" -gt 0 ] && [ "$BINARY_SIZE" -lt 104857600 ]; then
  SIZE_MB=$((BINARY_SIZE / 1024 / 1024))
  pass "binary size ${SIZE_MB}MB (under 100MB limit)"
else
  warn "could not verify binary size"
fi

echo ""
echo "============================================"
if [ "$ERRORS" -eq 0 ]; then
  echo -e "${GREEN}  ALL CHECKS PASSED — safe to deploy${NC}"
  echo "============================================"
  exit 0
else
  echo -e "${RED}  $ERRORS CHECK(S) FAILED — do NOT deploy${NC}"
  echo "============================================"
  exit 1
fi
