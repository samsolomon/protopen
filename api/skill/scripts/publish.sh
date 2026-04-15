#!/usr/bin/env bash
set -euo pipefail

BASE_URL="https://app.velori.dev"
API_KEY="${VELORI_TOKEN:-}"
NAME=""
TARGET=""

usage() {
  cat <<'USAGE'
Usage: publish.sh <file-or-dir> [options]

Options:
  --name <name>       Project name (defaults to directory name)
  --api-key <key>     API token for authenticated deploys (or set $VELORI_TOKEN)
  --base-url <url>    API base (default: https://app.velori.dev)
USAGE
  exit 1
}

die() { echo "error: $1" >&2; exit 1; }

for cmd in curl zip; do
  command -v "$cmd" >/dev/null 2>&1 || die "requires $cmd"
done

while [[ $# -gt 0 ]]; do
  case "$1" in
    --name)      NAME="$2"; shift 2 ;;
    --api-key)   API_KEY="$2"; shift 2 ;;
    --base-url)  BASE_URL="$2"; shift 2 ;;
    --help|-h)   usage ;;
    -*)          die "unknown option: $1" ;;
    *)           [[ -z "$TARGET" ]] && TARGET="$1" || die "unexpected argument: $1"; shift ;;
  esac
done

[[ -n "$TARGET" ]] || usage
[[ -e "$TARGET" ]] || die "path does not exist: $TARGET"
BASE_URL="${BASE_URL%/}"

# Default name to directory/file basename
if [[ -z "$NAME" ]]; then
  NAME="$(basename "$TARGET" .zip)"
fi

# Build the upload
if [[ -d "$TARGET" ]]; then
  # Directory: zip it
  tmp_dir="$(mktemp -d /tmp/velori-publish-XXXXXX)"
  tmp_zip="${tmp_dir}/site.zip"
  trap 'rm -rf "$tmp_dir"' EXIT
  (cd "$TARGET" && zip -qr "$tmp_zip" .)
  UPLOAD_FILE="$tmp_zip"
  MODE="zip"
elif [[ "$TARGET" == *.zip ]]; then
  UPLOAD_FILE="$TARGET"
  MODE="zip"
else
  UPLOAD_FILE="$TARGET"
  MODE="files"
fi

# Choose endpoint based on auth
if [[ -n "$API_KEY" ]]; then
  ENDPOINT="${BASE_URL}/api/uploads"
  AUTH_HEADER="Authorization: Bearer ${API_KEY}"
else
  echo "⚠ No VELORI_TOKEN set — deploying anonymously (site expires in 24 hours)" >&2
  echo "  Run 'velori login' or set VELORI_TOKEN for permanent deploys." >&2
  ENDPOINT="${BASE_URL}/api/v1/publish"
  AUTH_HEADER=""
fi

# Upload
if [[ "$MODE" == "zip" ]]; then
  CURL_ARGS=(-F "name=${NAME}" -F "mode=zip" -F "files=@${UPLOAD_FILE};type=application/zip")
else
  CURL_ARGS=(-F "name=${NAME}" -F "files=@${UPLOAD_FILE};filename=index.html;type=text/html")
fi

if [[ -n "$AUTH_HEADER" ]]; then
  CURL_ARGS+=(-H "$AUTH_HEADER")
fi

response="$(curl -fsSL "${CURL_ARGS[@]}" "$ENDPOINT")"

# Extract and display the URL
if [[ -n "$API_KEY" ]]; then
  url="$(echo "$response" | grep -o '"liveUrl":"[^"]*"' | head -1 | cut -d'"' -f4)"
else
  url="$(echo "$response" | grep -o '"siteUrl":"[^"]*"' | head -1 | cut -d'"' -f4)"

  # Save claim token for later
  claim_token="$(echo "$response" | grep -o '"claimToken":"[^"]*"' | head -1 | cut -d'"' -f4)"
  slug="$(echo "$response" | grep -o '"slug":"[^"]*"' | head -1 | cut -d'"' -f4)"
  expires="$(echo "$response" | grep -o '"expiresAt":"[^"]*"' | head -1 | cut -d'"' -f4)"

  STATE_DIR=".velori"
  mkdir -p "$STATE_DIR"
  cat > "${STATE_DIR}/state.json" <<STATEJSON
{"slug":"${slug}","claimToken":"${claim_token}","expiresAt":"${expires}","siteUrl":"${url}"}
STATEJSON
fi

echo "$url"
