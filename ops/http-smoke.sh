#!/usr/bin/env bash
set -euo pipefail

api="${API_URL:-http://localhost:8080}"
scratch="$(mktemp -d)"
trap 'rm -rf "$scratch"' EXIT

expect() {
  local want="$1" actual
  shift
  actual="$(curl -sS --max-time 10 -o "$scratch/body" -w '%{http_code}' "$@")"
  if [[ "$actual" != "$want" ]]; then
    printf 'Expected HTTP %s, got %s\n' "$want" "$actual" >&2
    cat "$scratch/body" >&2
    exit 1
  fi
}

json=(-H 'Content-Type: application/json')
expect 200 "$api/ready"
for role in owner other; do
  credentials="{\"username\":\"smoke-$role-$(date +%s)-$$\",\"password\":\"smoke-password\"}"
  expect 200 "${json[@]}" -d "$credentials" "$api/register"
  expect 200 "${json[@]}" -c "$scratch/$role" -d "$credentials" "$api/login"
done

request='{"audio":"https://example.com/smoke.wav"}'
owner=(-b "$scratch/owner")
key=(-H 'Idempotency-Key: smoke')
expect 401 "${json[@]}" -d "$request" "$api/audio"
expect 400 "${owner[@]}" "${json[@]}" -d '{"audio":"invalid"}' "$api/audio"
expect 202 "${owner[@]}" "${json[@]}" "${key[@]}" -d "$request" "$api/audio"
id="$(python3 -c 'import json,sys; print(json.load(sys.stdin)["task_id"])' < "$scratch/body")"
expect 202 "${owner[@]}" "${json[@]}" "${key[@]}" -d "$request" "$api/audio"
python3 -c 'import json,sys; assert json.load(sys.stdin)["task_id"] == sys.argv[1]' "$id" < "$scratch/body"
expect 409 "${owner[@]}" "${json[@]}" "${key[@]}" -d '{"audio":"https://example.com/other.wav"}' "$api/audio"
expect 200 "${owner[@]}" "$api/tasks/$id"
expect 404 -b "$scratch/other" "$api/tasks/$id"
expect 404 -b "$scratch/other" -X DELETE "$api/tasks/$id"
expect 200 "${owner[@]}" -X DELETE "$api/tasks/$id"
expect 404 "${owner[@]}" "$api/tasks/$id"
printf 'HTTP smoke checks passed\n'
