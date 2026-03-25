#!/usr/bin/env bash

set -euo pipefail

API_BASE_URL="${API_BASE_URL:-http://localhost:8080}"
CONTENT_BASE_URL="${CONTENT_BASE_URL:-http://127.0.0.1:8081}"
EMAIL="${VELORI_EMAIL:-sam@velori.dev}"
PASSWORD="${VELORI_PASSWORD:-velori-demo}"

tmp_dir="$(mktemp -d)"
cookie_file="${tmp_dir}/cookies.txt"
site_dir="${tmp_dir}/zip-prototype"
zip_file="${tmp_dir}/zip-prototype.zip"
mkdir -p "${site_dir}"

project_name="zip-smoke-test"

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
    <title>Velori Zip Smoke Test</title>
    <link rel="stylesheet" href="styles.css" />
  </head>
  <body>
    <main>
      <h1>Velori Zip Smoke Test</h1>
      <p>If you can read this, the zip upload is working.</p>
    </main>
  </body>
</html>
EOF

cat > "${site_dir}/styles.css" <<'EOF'
body { font-family: sans-serif; background: #f5f7fb; color: #172030; }
main { max-width: 720px; margin: 80px auto; padding: 24px; background: white; border-radius: 16px; }
EOF

echo "Creating zip archive"
(cd "${tmp_dir}" && zip -r "${zip_file}" "zip-prototype/" >/dev/null 2>&1)

echo "Signing in to ${API_BASE_URL}"
curl -sS -c "${cookie_file}" \
  -H "Content-Type: application/json" \
  -d "{\"email\":\"${EMAIL}\",\"password\":\"${PASSWORD}\"}" \
  "${API_BASE_URL}/api/sign-in" >/dev/null

echo "Uploading zip archive"
upload_response="$(curl -sS -b "${cookie_file}" \
  -F "name=${project_name}" \
  -F 'mode=zip' \
  -F "files=@${zip_file};filename=zip-prototype.zip;type=application/zip" \
  "${API_BASE_URL}/api/uploads")"

live_url="$(python3 - <<'PY' "${upload_response}"
import json, sys
payload = json.loads(sys.argv[1])
print(payload["project"]["liveUrl"])
PY
)"

echo "Fetching hosted page: ${live_url}"
html="$(curl -sS "${live_url}")"

if [[ "${html}" != *"Velori Zip Smoke Test"* ]]; then
  echo "Zip smoke test failed: hosted content did not match expected output" >&2
  echo "Response: ${html}" >&2
  exit 1
fi

echo "Verifying CSS asset is served"
css="$(curl -sS "${live_url}/styles.css")"

if [[ "${css}" != *"font-family"* ]]; then
  echo "Zip smoke test failed: CSS asset not served correctly" >&2
  exit 1
fi

echo "Zip smoke test passed"
echo "Content origin: ${CONTENT_BASE_URL}"
echo "Live URL: ${live_url}"
