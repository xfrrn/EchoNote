#!/usr/bin/env bash
set -euo pipefail

base_url="${1:?usage: smoke.sh https://notes.example.com}"
case "$base_url" in
  https://*) ;;
  *) echo "base URL must use HTTPS" >&2; exit 2 ;;
esac
base_url="${base_url%/}"

tmp_dir="$(mktemp -d)"
trap 'rm -rf -- "$tmp_dir"' EXIT
headers="$tmp_dir/headers"
cookies="$tmp_dir/cookies"

curl --fail --silent --show-error --head "$base_url/" > "$headers"
for expected in \
  'strict-transport-security:' \
  'content-security-policy:' \
  'x-content-type-options: nosniff' \
  'x-frame-options: deny' \
  'referrer-policy: no-referrer'; do
  grep -Fqi "$expected" "$headers" || { echo "missing security header: $expected" >&2; exit 1; }
done

test "$(curl --silent --show-error --output /dev/null --write-out '%{http_code}' "$base_url/api/v1/me")" = 401
curl --fail --silent --show-error "$base_url/healthz" | grep -Fq '"status":"ok"'

read -r -p 'Smoke username: ' username
read -r -s -p 'Smoke password: ' password
printf '\n' >&2
payload="$(printf '%s' "$password" | jq -Rsc --arg username "$username" '{username:$username,password:.}')"
unset password

printf '%s' "$payload" | curl --fail --silent --show-error \
  --cookie-jar "$cookies" \
  --header "Origin: $base_url" \
  --header 'Content-Type: application/json' \
  --data-binary @- \
  "$base_url/api/v1/auth/login" > /dev/null
unset payload

curl --fail --silent --show-error --cookie "$cookies" "$base_url/api/v1/me" | jq -e '.user.id and .user.username' > /dev/null
curl --fail --silent --show-error --cookie "$cookies" --header "Origin: $base_url" --request POST "$base_url/api/v1/auth/logout" > /dev/null
echo 'EchoNote smoke passed'
