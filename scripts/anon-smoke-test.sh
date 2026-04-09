#!/usr/bin/env bash

set -euo pipefail

API_BASE_URL="${API_BASE_URL:-http://localhost:8080}"
EMAIL="${VELORI_EMAIL:-sam@velori.dev}"
PASSWORD="${VELORI_PASSWORD:-velori-demo}"

tmp_dir="$(mktemp -d)"
cookie_file="${tmp_dir}/cookies.txt"
site_dir="${tmp_dir}/site"
mkdir -p "${site_dir}"

cleanup() {
  rm -rf "${tmp_dir}"
}
trap cleanup EXIT

cat > "${site_dir}/index.html" <<'EOF'
<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <title>Anonymous Smoke Test</title>
    <link rel="stylesheet" href="styles.css" />
  </head>
  <body>
    <h1>Anonymous Smoke Test</h1>
    <p>Published without an account.</p>
  </body>
</html>
EOF

cat > "${site_dir}/styles.css" <<'EOF'
body { font-family: sans-serif; background: #f5f7fb; color: #172030; }
h1 { color: #3b82f6; }
EOF

# --- 1. Publish ---
echo "Publishing anonymous site"
publish_response="$(curl -sS \
  -F "name=anon-smoke-test" \
  -F "files=@${site_dir}/index.html;filename=index.html;type=text/html" \
  -F "files=@${site_dir}/styles.css;filename=styles.css;type=text/css" \
  "${API_BASE_URL}/api/v1/publish")"

site_url="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['siteUrl'])" "${publish_response}")"
slug="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['slug'])" "${publish_response}")"
claim_token="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['claimToken'])" "${publish_response}")"

echo "  slug: ${slug}"
echo "  siteUrl: ${site_url}"

# --- 2. Serve ---
echo "Fetching anonymous site"
html="$(curl -sS "${site_url}")"

if [[ "${html}" != *"Anonymous Smoke Test"* ]]; then
  echo "FAIL: published content not found at ${site_url}" >&2
  exit 1
fi

if [[ "${html}" != *"data-velori-badge"* ]]; then
  echo "FAIL: Velori badge not injected" >&2
  exit 1
fi
echo "  content and badge verified"

# --- 3. Noindex header ---
headers="$(curl -sSI "${site_url}")"
if [[ "${headers}" != *"X-Robots-Tag"* ]]; then
  echo "FAIL: X-Robots-Tag header missing" >&2
  exit 1
fi
echo "  noindex header verified"

# --- 4. Claim ---
echo "Signing in as ${EMAIL}"
curl -sS -c "${cookie_file}" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"${EMAIL}\",\"password\":\"${PASSWORD}\"}" \
  "${API_BASE_URL}/api/sign-in" >/dev/null

echo "Claiming anonymous deploy"
claim_response="$(curl -sS -b "${cookie_file}" \
  -H "Content-Type: application/json" \
  -d "{\"slug\":\"${slug}\",\"claimToken\":\"${claim_token}\",\"name\":\"Claimed Smoke Test\"}" \
  "${API_BASE_URL}/api/v1/claim")"

live_url="$(python3 -c "import json,sys; print(json.loads(sys.argv[1])['liveUrl'])" "${claim_response}")"
echo "  claimed as: ${live_url}"

# --- 5. Claimed project serves ---
echo "Fetching claimed project"
claimed_html="$(curl -sS "${live_url}")"

if [[ "${claimed_html}" != *"Anonymous Smoke Test"* ]]; then
  echo "FAIL: claimed project content not found at ${live_url}" >&2
  exit 1
fi
echo "  claimed content verified"

# --- 6. Old slug is gone ---
echo "Verifying old anonymous slug returns 404"
old_status="$(curl -sS -o /dev/null -w "%{http_code}" "${site_url}")"
if [[ "${old_status}" != "404" ]]; then
  echo "FAIL: old anonymous slug returned ${old_status}, expected 404" >&2
  exit 1
fi
echo "  old slug returns 404"

# --- 7. Double claim fails ---
echo "Verifying double claim returns 410"
double_status="$(curl -sS -o /dev/null -w "%{http_code}" -b "${cookie_file}" \
  -H "Content-Type: application/json" \
  -d "{\"slug\":\"${slug}\",\"claimToken\":\"${claim_token}\",\"name\":\"Double Claim\"}" \
  "${API_BASE_URL}/api/v1/claim")"
if [[ "${double_status}" != "410" ]]; then
  echo "FAIL: double claim returned ${double_status}, expected 410" >&2
  exit 1
fi
echo "  double claim returns 410"

# --- 8. Rate limiting ---
# The initial publish above consumed 1 of 5 allowed requests per hour.
# Send 4 more to reach the limit, then verify the next one is blocked.
echo "Testing rate limit (5/hour, 1 already used)"
for i in {1..5}; do
  status="$(curl -sS -o /dev/null -w "%{http_code}" \
    -F "name=rate-test-${i}" \
    -F "files=@${site_dir}/index.html;filename=index.html;type=text/html" \
    "${API_BASE_URL}/api/v1/publish")"
  if [[ "${i}" -le 4 ]]; then
    if [[ "${status}" == "429" ]]; then
      echo "FAIL: request ${i} was rate-limited (expected success)" >&2
      exit 1
    fi
  else
    if [[ "${status}" != "429" ]]; then
      echo "FAIL: request ${i} returned ${status}, expected 429" >&2
      exit 1
    fi
  fi
done
echo "  rate limit verified (4 OK, 5th blocked)"

echo ""
echo "All anonymous publishing smoke tests passed"
