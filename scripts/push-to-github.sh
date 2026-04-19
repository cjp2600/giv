#!/bin/bash
#
# Push giv to GitHub as cjp2600 (PAT in .env), same idea as cortex/scripts/push-to-cortexx.sh
# Usage: ./scripts/push-to-github.sh [commit-message]
#
# Requires repo-root .env with GITHUB_TOKEN=ghp_...
# Optional: GIV_VERSION=1.0.0 (default) for release tag v1.0.0
#
set -e

REPO_SLUG="cjp2600/giv"
REPO_HTTPS="https://github.com/${REPO_SLUG}.git"
BRANCH="main"
VERSION="${GIV_VERSION:-1.0.0}"
TAG="v${VERSION}"

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m'

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
cd "$REPO_ROOT"

# Load .env from repo root
if [ -f ".env" ]; then
  set -a
  # shellcheck source=/dev/null
  source .env
  set +a
fi

echo -e "${BLUE}╔════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║   Push giv to GitHub                    ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════╝${NC}"
echo ""

if [ -z "$GITHUB_TOKEN" ]; then
  echo -e "${RED}✗ GITHUB_TOKEN is not set (add it to ${REPO_ROOT}/.env)${NC}"
  exit 1
fi

REPO_URL="https://${GITHUB_TOKEN}@github.com/${REPO_SLUG}.git"

echo -e "${GREEN}📦 Release version: ${VERSION} (tag ${TAG})${NC}"
echo -e "${BLUE}🌿 Branch: ${BRANCH}${NC}"
echo ""

echo -e "${BLUE}📝 Staging all changes...${NC}"
git add -A

HAS_CHANGES=true
if git diff-index --quiet --cached HEAD -- 2>/dev/null; then
  echo -e "${YELLOW}⚠  Nothing new to commit${NC}"
  HAS_CHANGES=false
else
  if [ -n "$1" ]; then
    COMMIT_MSG="$1"
  else
    COMMIT_MSG="Update (${TAG})"
  fi
  echo -e "${BLUE}💬 Commit: ${COMMIT_MSG}${NC}"
  git commit -m "$COMMIT_MSG"
  echo -e "${GREEN}✓ Committed${NC}"
fi

echo ""
echo -e "${BLUE}🏷️  Tag ${TAG}...${NC}"
if git rev-parse "$TAG" >/dev/null 2>&1; then
  echo -e "${YELLOW}⚠  Tag ${TAG} exists locally — deleting${NC}"
  git tag -d "$TAG"
  git push "$REPO_URL" ":refs/tags/$TAG" 2>/dev/null || true
fi
git tag -a "$TAG" -m "Release ${TAG}"
echo -e "${GREEN}✓ Tag ${TAG}${NC}"

echo ""
echo -e "${BLUE}⬆️  Push branch ${BRANCH}...${NC}"
git push "$REPO_URL" "$BRANCH" --force

echo ""
echo -e "${BLUE}⬆️  Push tag ${TAG}...${NC}"
git push "$REPO_URL" "$TAG" --force

echo ""
echo -e "${GREEN}╔════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║   Done                                  ║${NC}"
echo -e "${GREEN}╚════════════════════════════════════════╝${NC}"
echo ""
echo -e "${GREEN}Repo:${NC} ${REPO_HTTPS}"
echo -e "${GREEN}Branch:${NC} ${BRANCH}"
echo -e "${GREEN}Tag:${NC} ${TAG}"
echo ""
