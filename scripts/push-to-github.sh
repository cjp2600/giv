#!/bin/bash
#
# Push giv to GitHub as cjp2600 (PAT in .env), same idea as cortex/scripts/push-to-cortexx.sh
# Usage: ./scripts/push-to-github.sh [commit-message]
#
# Requires repo-root .env with GITHUB_TOKEN=ghp_...
# Does not create or push git tags — use explicit "git tag" + "./scripts/sync-homebrew-formula.sh"
# when cutting a release so Homebrew tarball checksums stay stable.
#
set -e

REPO_SLUG="cjp2600/giv"
REPO_HTTPS="https://github.com/${REPO_SLUG}.git"
BRANCH="main"

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

echo -e "${BLUE}🌿 Branch: ${BRANCH}${NC}"
echo ""

echo -e "${BLUE}📝 Staging all changes...${NC}"
git add -A

if git diff-index --quiet --cached HEAD -- 2>/dev/null; then
  echo -e "${YELLOW}⚠  Nothing new to commit${NC}"
else
  if [ -n "$1" ]; then
    COMMIT_MSG="$1"
  else
    COMMIT_MSG="Update (main)"
  fi
  echo -e "${BLUE}💬 Commit: ${COMMIT_MSG}${NC}"
  git commit -m "$COMMIT_MSG"
  echo -e "${GREEN}✓ Committed${NC}"
fi

echo ""
echo -e "${BLUE}⬆️  Push branch ${BRANCH}...${NC}"
git push "$REPO_URL" "$BRANCH" --force

echo ""
echo -e "${GREEN}╔════════════════════════════════════════╗${NC}"
echo -e "${GREEN}║   Done                                  ║${NC}"
echo -e "${GREEN}╚════════════════════════════════════════╝${NC}"
echo ""
echo -e "${GREEN}Repo:${NC} ${REPO_HTTPS}"
echo -e "${GREEN}Branch:${NC} ${BRANCH}"
echo ""
