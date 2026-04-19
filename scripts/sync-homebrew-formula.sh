#!/usr/bin/env bash
#
# Sync packaging/homebrew/Formula/giv.rb (and optional tap clone) with a GitHub release tag:
# downloads the official source tarball for that tag, recomputes SHA256, updates url + sha256.
#
# Typical flow (after your release commit is on main):
#   git tag v1.2.0
#   git push origin main && git push origin v1.2.0   # tag must exist on GitHub before sha matches
#   ./scripts/sync-homebrew-formula.sh v1.2.0
#   git add packaging/homebrew/Formula/giv.rb && git commit -m "fix(homebrew): bump formula to v1.2.0"
#   # plus commit/push $HOMEBREW_TAP_DIR if you use the sibling tap repo
#
# Env:
#   GITHUB_REPO   default: cjp2600/giv  (owner/name of the app repo that hosts tags)
#   HOMEBREW_TAP_DIR  if set, also patches Formula/giv.rb there.
#                     If unset, uses ../homebrew-giv relative to this repo when that directory exists.
#
set -euo pipefail

SCRIPT_DIR=$(cd "$(dirname "$0")" && pwd)
REPO_ROOT=$(cd "$SCRIPT_DIR/.." && pwd)
GITHUB_REPO="${GITHUB_REPO:-cjp2600/giv}"

usage() {
  cat <<'EOF'
Update Homebrew formula url + sha256 from the GitHub source tarball for a release tag.

Usage:
  sync-homebrew-formula.sh [--dry-run] <tag>

Examples:
  ./scripts/sync-homebrew-formula.sh v1.2.0
  ./scripts/sync-homebrew-formula.sh 1.2.0
  HOMEBREW_TAP_DIR=$HOME/dev/homebrew-giv ./scripts/sync-homebrew-formula.sh v1.2.0

Env:
  GITHUB_REPO       default cjp2600/giv
  HOMEBREW_TAP_DIR  optional second Formula/giv.rb to patch (else ../homebrew-giv if present)
EOF
}

DRY_RUN=0
TAG_ARG=
while [[ $# -gt 0 ]]; do
  case "$1" in
    --dry-run) DRY_RUN=1 ;;
    -h|--help) usage; exit 0 ;;
    *)
      if [[ -n "$TAG_ARG" ]]; then
        echo "error: unexpected argument: $1" >&2
        exit 1
      fi
      TAG_ARG="$1"
      ;;
  esac
  shift || true
done

if [[ -z "${TAG_ARG:-}" ]]; then
  usage
  exit 1
fi

# Normalize to vX.Y.Z
if [[ "$TAG_ARG" == v* ]]; then
  TAG="$TAG_ARG"
else
  TAG="v${TAG_ARG}"
fi

TAP_DIR="${HOMEBREW_TAP_DIR:-}"
if [[ -z "$TAP_DIR" ]]; then
  if [[ -d "$REPO_ROOT/../homebrew-giv" ]]; then
    TAP_DIR=$(cd "$REPO_ROOT/../homebrew-giv" && pwd)
  fi
fi

URL="https://github.com/${GITHUB_REPO}/archive/refs/tags/${TAG}.tar.gz"

echo "Repo:     $GITHUB_REPO"
echo "Tag:      $TAG"
echo "Tarball:  $URL"
echo ""

TMP=$(mktemp)
trap 'rm -f "$TMP"' EXIT

if ! curl -fsSL "$URL" -o "$TMP"; then
  echo "error: could not download tarball (tag missing on GitHub or network)." >&2
  echo "      Push the tag first: git push origin $TAG" >&2
  exit 1
fi

SHA=$(shasum -a 256 "$TMP" | awk '{print $1}')
echo "SHA256:   $SHA"
echo ""

FORMULA_FILES=("$REPO_ROOT/packaging/homebrew/Formula/giv.rb")
if [[ -n "$TAP_DIR" && -f "$TAP_DIR/Formula/giv.rb" ]]; then
  FORMULA_FILES+=("$TAP_DIR/Formula/giv.rb")
fi

update_file() {
  local path=$1
  ruby - "$path" "$URL" "$SHA" <<'RUBY'
path, url, sha = ARGV
s = File.read(path)
unless s.sub!(/^  url "[^"]*"$/, %(  url "#{url}"))
  warn "warning: url line not updated in #{path}"
end
unless s.sub!(/^  sha256 "[^"]*"$/, %(  sha256 "#{sha}"))
  warn "warning: sha256 line not updated in #{path}"
end
File.write(path, s)
RUBY
}

for f in "${FORMULA_FILES[@]}"; do
  echo "→ $f"
  if [[ "$DRY_RUN" -eq 1 ]]; then
    echo "   (dry-run, skip write)"
    continue
  fi
  update_file "$f"
done

echo ""
echo "Done. Review diffs, then commit (and push tap repo if applicable)."
if [[ -n "$TAP_DIR" ]]; then
  echo "Tap dir: $TAP_DIR"
else
  echo "Tip: clone your tap next to this repo as ../homebrew-giv or set HOMEBREW_TAP_DIR."
fi
