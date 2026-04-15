#!/usr/bin/env bash
set -euo pipefail

REPO="samsolomon/velori-cli"
INSTALL_DIR="${HOME}/.velori/bin"
SKILL_DIR="${HOME}/.claude/skills/velori"
SCRIPTS_DIR="${SKILL_DIR}/scripts"
SKILL_BASE="https://app.velori.dev/skill"

die() {
  echo "error: $1" >&2
  exit 1
}

need_cmd() {
  command -v "$1" >/dev/null 2>&1 || die "requires $1"
}

detect_platform() {
  local os arch
  os="$(uname -s | tr '[:upper:]' '[:lower:]')"
  arch="$(uname -m)"

  case "$os" in
    darwin) ;;
    linux) ;;
    *) die "unsupported OS: ${os}" ;;
  esac

  case "$arch" in
    x86_64)  arch="amd64" ;;
    aarch64) arch="arm64" ;;
    arm64)   arch="arm64" ;;
    *)       die "unsupported architecture: ${arch}" ;;
  esac

  echo "${os}_${arch}"
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

echo "Installing velori..."

need_cmd curl
need_cmd tar

# --- Install CLI binary ---
platform="$(detect_platform)"
archive="velori_${platform}.tar.gz"
url="https://github.com/${REPO}/releases/latest/download/${archive}"

mkdir -p "$INSTALL_DIR"
tmp_dir="$(mktemp -d)"
trap 'rm -rf "$tmp_dir"' EXIT

echo "Downloading velori CLI (${platform})..."
curl -fsSL "$url" -o "${tmp_dir}/${archive}"
tar -xzf "${tmp_dir}/${archive}" -C "$tmp_dir"
mv "${tmp_dir}/velori" "${INSTALL_DIR}/velori"
chmod +x "${INSTALL_DIR}/velori"

# --- Install skill files ---
echo "Installing agent skill..."
mkdir -p "$SCRIPTS_DIR"
atomic_download "${SKILL_BASE}/SKILL.md" "$SKILL_DIR/SKILL.md"
atomic_download "${SKILL_BASE}/scripts/publish.sh" "$SCRIPTS_DIR/publish.sh"
chmod +x "$SCRIPTS_DIR/publish.sh"

# --- PATH setup ---
version="$("${INSTALL_DIR}/velori" version 2>/dev/null || echo "")"

echo ""
echo "done — velori${version:+ ${version}}"
echo ""
echo "  Binary:  ${INSTALL_DIR}/velori"
echo "  Skill:   ${SKILL_DIR}"

# Auto-configure PATH
if command -v velori >/dev/null 2>&1; then
  echo ""
  echo "velori is on your PATH. You're ready to go."
else
  case "$SHELL" in
    */zsh)  SHELL_RC="$HOME/.zshrc" ;;
    */bash)
      if [ "$(uname -s)" = "Darwin" ]; then
        SHELL_RC="$HOME/.bash_profile"
      else
        SHELL_RC="$HOME/.bashrc"
      fi
      ;;
    *)      SHELL_RC="" ;;
  esac

  EXPORT_LINE='export PATH="$HOME/.velori/bin:$PATH"'

  if [ -n "$SHELL_RC" ]; then
    if ! grep -qF '.velori/bin' "$SHELL_RC" 2>/dev/null; then
      echo "" >> "$SHELL_RC"
      echo "$EXPORT_LINE" >> "$SHELL_RC"
    fi
    echo ""
    echo "Added velori to PATH in $SHELL_RC"
    echo "Open a new terminal to use it."
  else
    echo ""
    echo "Add velori to your PATH:"
    echo ""
    echo "  $EXPORT_LINE"
  fi
fi
