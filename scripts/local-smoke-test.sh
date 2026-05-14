#!/usr/bin/env bash

set -euo pipefail

API_BASE_URL="${API_BASE_URL:-http://localhost:8080}"
CONTENT_BASE_URL="${CONTENT_BASE_URL:-http://127.0.0.1:8081}"
EMAIL="${PROTOPEN_EMAIL:-sam@protopen.dev}"
PASSWORD="${PROTOPEN_PASSWORD:-protopen-demo}"

tmp_dir="$(mktemp -d)"
cookie_file="${tmp_dir}/cookies.txt"
site_dir="${tmp_dir}/prototype"
mkdir -p "${site_dir}"

site_name="smoke-test"

cleanup() {
  rm -rf "${tmp_dir}"
}
trap cleanup EXIT

cat > "${site_dir}/index.html" <<'EOF'
<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>Protopen Smoke Test</title>
    <link rel="stylesheet" href="styles.css" />
  </head>
  <body>
    <main>
      <h1>Protopen Smoke Test</h1>
      <p>If you can read this, the hosted prototype is live.</p>
    </main>
  </body>
</html>
EOF

cat > "${site_dir}/styles.css" <<'EOF'
body { font-family: sans-serif; background: #f5f7fb; color: #172030; }
main { max-width: 720px; margin: 80px auto; padding: 24px; background: white; border-radius: 16px; }
EOF

echo "Signing in to ${API_BASE_URL}"
curl -sS -c "${cookie_file}" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"${EMAIL}\",\"password\":\"${PASSWORD}\"}" \
  "${API_BASE_URL}/api/sign-in" >/dev/null

echo "Uploading smoke-test prototype"
upload_response="$(curl -sS -b "${cookie_file}" \
  -F "name=${site_name}" \
  -F 'mode=files' \
  -F 'paths=["prototype/index.html","prototype/styles.css"]' \
  -F "files=@${site_dir}/index.html;filename=index.html;type=text/html" \
  -F "files=@${site_dir}/styles.css;filename=styles.css;type=text/css" \
  "${API_BASE_URL}/api/uploads")"

live_url="$(python3 - <<'PY' "${upload_response}"
import json, sys
payload = json.loads(sys.argv[1])
print(payload["site"]["liveUrl"])
PY
)"

echo "Fetching hosted page: ${live_url}"
html="$(curl -sS "${live_url}")"

if [[ "${html}" != *"Protopen Smoke Test"* ]]; then
  echo "Smoke test failed: hosted content did not match expected output" >&2
  exit 1
fi

cat > "${site_dir}/index.html" <<'EOF'
<!doctype html>
<html lang="en">
  <head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>Protopen Smoke Test Updated</title>
    <link rel="stylesheet" href="styles.css" />
  </head>
  <body>
    <main>
      <h1>Protopen Smoke Test Updated</h1>
      <p>The stable URL now shows the redeployed content.</p>
    </main>
  </body>
</html>
EOF

echo "Redeploying the same site name to verify stable URL updates"
second_upload_response="$(curl -sS -b "${cookie_file}" \
  -F "name=${project_name}" \
  -F 'mode=files' \
  -F 'paths=["prototype/index.html","prototype/styles.css"]' \
  -F "files=@${site_dir}/index.html;filename=index.html;type=text/html" \
  -F "files=@${site_dir}/styles.css;filename=styles.css;type=text/css" \
  "${API_BASE_URL}/api/uploads")"

second_live_url="$(python3 - <<'PY' "${second_upload_response}"
import json, sys
payload = json.loads(sys.argv[1])
print(payload["project"]["liveUrl"])
PY
)"

if [[ "${second_live_url}" != "${live_url}" ]]; then
  echo "Smoke test failed: redeploy changed the live URL" >&2
  echo "first:  ${live_url}" >&2
  echo "second: ${second_live_url}" >&2
  exit 1
fi

updated_html="$(curl -sS "${live_url}")"

if [[ "${updated_html}" != *"Protopen Smoke Test Updated"* ]]; then
  echo "Smoke test failed: stable URL did not serve updated content after redeploy" >&2
  exit 1
fi

echo "Smoke test passed"
echo "Content origin: ${CONTENT_BASE_URL}"
echo "Live URL: ${live_url}"
