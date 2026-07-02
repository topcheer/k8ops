#!/usr/bin/env bash
# generate-changelog.sh — Auto-generate changelog entries from git commits
# Usage: ./scripts/generate-changelog.sh [from-tag] [to-ref]
# Default: generates from last tag to HEAD
set -euo pipefail

FROM="${1:-$(git describe --tags --abbrev=0 2>/dev/null || echo "")}"
TO="${2:-HEAD}"
PROJECT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
cd "$PROJECT_DIR"

if [ -z "$FROM" ]; then
  RANGE=""
  COMMITS="$(git log --pretty=format:"%H|%s|%an|%ad" --date=short -50)"
else
  RANGE="${FROM}..${TO}"
  COMMITS="$(git log --pretty=format:"%H|%s|%an|%ad" --date=short "$RANGE")"
fi

# Categorize commits
ADDED=""
CHANGED=""
FIXED=""
SECURITY=""
DOCS=""
OTHER=""

while IFS='|' read -r hash subject author date; do
  # Skip merge commits
  [[ "$subject" =~ ^Merge ]] && continue

  # Categorize by conventional commit prefix
  if [[ "$subject" =~ ^(feat|add): ]]; then
    ADDED="${ADDED}- ${subject#feat: }\n"
  elif [[ "$subject" =~ ^fix: ]]; then
    FIXED="${FIXED}- ${subject#fix: }\n"
  elif [[ "$subject" =~ ^docs?: ]]; then
    DOCS="${DOCS}- ${subject#docs: }\n"
  elif [[ "$subject" =~ ^security: ]]; then
    SECURITY="${SECURITY}- ${subject#security: }\n"
  elif [[ "$subject" =~ ^(refactor|perf|chore): ]]; then
    CHANGED="${CHANGED}- ${subject#refactor: }\n"
  else
    OTHER="${OTHER}- ${subject}\n"
  fi
done <<< "$COMMITS"

# Output changelog block
echo ""
echo "## [Unreleased] — $(date +%Y-%m-%d)"
echo ""

if [ -n "$ADDED" ]; then
  echo "### Added"
  echo -e "$ADDED"
fi
if [ -n "$CHANGED" ]; then
  echo "### Changed"
  echo -e "$CHANGED"
fi
if [ -n "$FIXED" ]; then
  echo "### Fixed"
  echo -e "$FIXED"
fi
if [ -n "$SECURITY" ]; then
  echo "### Security"
  echo -e "$SECURITY"
fi
if [ -n "$DOCS" ]; then
  echo "### Documentation"
  echo -e "$DOCS"
fi
if [ -n "$OTHER" ]; then
  echo "### Other"
  echo -e "$OTHER"
fi
