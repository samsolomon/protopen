# KEEP IN SYNC with skill/install.sh (go:embed cannot follow symlinks)
#!/usr/bin/env bash
set -euo pipefail

SKILL_DIR="${HOME}/.claude/skills/velori"
SCRIPTS_DIR="${SKILL_DIR}/scripts"
REPO_BASE="https://raw.githubusercontent.com/samsolomon/velori/main/skill"

die() {
  echo "error: $1" >&2
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "requires $1"
}

atomic_download() {
  local url="$1"
  local destination="$2"
  local tmp

  tmp="$(mktemp "${destination}.tmp.XXXXXX")"
  if ! curl -fsSL "$url" -o "$tmp"; then
    rm -f "$tmp"
    die "failed to download $url"
  fi
  mv "$tmp" "$destination"
}

echo "Installing velori skill..."

need_cmd curl
need_cmd zip

mkdir -p "$SCRIPTS_DIR"

atomic_download "${REPO_BASE}/SKILL.md" "$SKILL_DIR/SKILL.md"
atomic_download "${REPO_BASE}/scripts/publish.sh" "$SCRIPTS_DIR/publish.sh"

chmod +x "$SCRIPTS_DIR/publish.sh"

echo ""
echo "done - velori skill installed to ${SKILL_DIR}"
echo "restart Claude Code to start using it"
