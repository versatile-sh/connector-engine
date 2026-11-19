#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"
compose=(docker compose -f deploy/compose.yaml -f deploy/compose.check.yaml --profile person)
port="${CONSOLE_PORT:-7465}"
export CONSOLE_PORT="$port"
base="http://127.0.0.1:$port"
work="$(mktemp -d)"
jar="$work/cookies"
keep="${CHECK_KEEP:-0}"

cleanup() {
  if [ "$keep" != 1 ]; then
    "${compose[@]}" down -v --remove-orphans >/dev/null 2>&1 || true
    docker image rm connector-engine:local connector-engine-testmcp:local >/dev/null 2>&1 || true
  fi
  rm -rf -- "$work"
}
trap cleanup EXIT

fail() { echo "check: $*" >&2; "${compose[@]}" logs --tail 40 engine console >&2 || true; exit 1; }
step() { echo "== $*"; }

step "build and start the person profile with the check override"
"${compose[@]}" up -d --build

for _ in $(seq 1 90); do
  curl -fsS -o /dev/null "$base/api/session" 2>/dev/null && break
  sleep 1
done
curl -fsS -o /dev/null "$base/api/session" || fail "the console did not answer"

step "published ports bind 127.0.0.1 only"
ports="$(docker ps --filter label=com.docker.compose.project=connector-engine --format '{{.Names}} {{.Ports}}')"
echo "$ports"
if echo "$ports" | grep -E '0\.0\.0\.0:[0-9]+->|\[::\]:[0-9]+->' >/dev/null; then fail "a port is published beyond loopback"; fi
echo "$ports" | grep -q "127.0.0.1:$port->$port" || fail "the console port is not on 127.0.0.1:$port"

api() {
  local method="$1" path="$2" body="${3:-}"
  local args=(-sS -b "$jar" -c "$jar" -X "$method" -H "Origin: $base" -w '\n%{http_code}')
  [ -n "${csrf:-}" ] && args+=(-H "X-CSRF-Token: $csrf")
  [ -n "$body" ] && args+=(-H 'Content-Type: application/json' --data "$body")
  curl "${args[@]}" "$base$path"
}
body() { sed '$d' <<<"$1"; }
code() { tail -n1 <<<"$1"; }

step "wait for the engine and the local tenant"
for _ in $(seq 1 60); do
  "${compose[@]}" logs console 2>/dev/null | grep -q '"tenant ready"' && break
  sleep 1
done

step "first visit creates the owner"
csrf=""
r="$(api GET /api/session)"; echo "$(body "$r")"
[ "$(body "$r" | jq -r .owner_exists)" = false ] || fail "an owner already exists"
r="$(api POST /api/setup '{"username":"owner","password":"correct horse battery"}')"
[ "$(code "$r")" = 201 ] || fail "setup: $r"
csrf="$(body "$r" | jq -r .csrf)"

step "sign out and sign in again"
api POST /api/logout >/dev/null
csrf=""
r="$(api POST /api/login '{"username":"owner","password":"wrong password!!"}')"
[ "$(code "$r")" = 401 ] || fail "a wrong password: $r"
r="$(api POST /api/login '{"username":"owner","password":"correct horse battery"}')"
[ "$(code "$r")" = 200 ] || fail "login: $r"
csrf="$(body "$r" | jq -r .csrf)"

step "guards"
[ "$(curl -s -o /dev/null -w '%{http_code}' -H "Host: evil.example:$port" "$base/api/session")" = 421 ] || fail "a foreign Host was served"
[ "$(curl -s -o /dev/null -w '%{http_code}' -b "$jar" -X POST -H 'Content-Type: application/json' -H "X-CSRF-Token: $csrf" --data '{"name":"x"}' "$base/api/agents")" = 403 ] || fail "a POST without Origin was accepted"
[ "$(curl -s -o /dev/null -w '%{http_code}' -b "$jar" -X POST -H "Origin: $base" -H 'Content-Type: application/json' -H 'X-CSRF-Token: wrong' --data '{"name":"x"}' "$base/api/agents")" = 403 ] || fail "a wrong CSRF token was accepted"
[ "$(curl -s -o /dev/null -w '%{http_code}' -X POST -H "Origin: $base" -H 'Content-Type: application/json' --data '{"sub":"owner","tenant":"local"}' "$base/local/token")" != 200 ] || fail "the local issuer is reachable"
r="$(api POST /api/preview '{"mcp":"http://testmcp:7999/mcp"}')"
[ "$(code "$r")" = 400 ] || fail "an http MCP without the local tick: $r"
echo "foreign Host 421, missing Origin 403, wrong CSRF 403, /local/token not served, http without tick 400"

step "import the test MCP server (local tick) and choose per function"
r="$(api POST /api/preview '{"mcp":"http://testmcp:7999/mcp","local":true}')"
[ "$(code "$r")" = 200 ] || fail "preview: $r"
body "$r" | jq -c '{title, name, protocol, tools: [.tools[] | {name, proposed}]}'
r="$(api POST /api/import '{"mcp":"http://testmcp:7999/mcp","local":true,"choices":{"create":"write","mystery":"off"}}')"
[ "$(code "$r")" = 201 ] || fail "import: $r"
conn="$(body "$r" | jq -r .connector)"

step "set a grant: purge stays ask, search goes ask then back to read"
r="$(api PUT "/api/connectors/$conn/functions/search" '{"access":"ask"}')"; [ "$(code "$r")" = 204 ] || fail "set ask: $r"
r="$(api PUT "/api/connectors/$conn/functions/search" '{"access":"read"}')"; [ "$(code "$r")" = 204 ] || fail "set read: $r"
r="$(api GET /api/connectors)"
body "$r" | jq -c '.connectors[] | {alias, title, functions: [.functions[] | {name, class, access}]}'
alias="$(body "$r" | jq -r '.connectors[0].alias')"
want='{"create":"write","mystery":"off","purge":"ask","search":"read"}'
got="$(body "$r" | jq -c '[.connectors[0].functions[] | {(.name): .access}] | add')"
[ "$(jq -S -c . <<<"$got")" = "$(jq -S -c . <<<"$want")" ] || fail "access read back $got, want $want"

step "mint an agent token"
r="$(api POST /api/agents '{"name":"check-agent","ttl_hours":24}')"
[ "$(code "$r")" = 201 ] || fail "mint: $r"
token="$(body "$r" | jq -r .token)"
body "$r" | jq -c '{agent, expires_at, mcp_url}'

step "every response so far left the operator token out"
optoken="$("${compose[@]}" exec -T postgres cat /run/connector-engine/operator-token)"
[ ${#optoken} -eq 64 ] || fail "the operator token is not a 32-byte random value"
for p in /api/session /api/connectors /api/agents /api/approvals; do
  if api GET "$p" | grep -qF "$optoken"; then fail "$p carries the operator token"; fi
done

cnctl() { docker run --rm --network host -e CN_ENDPOINT="$base/mcp" -e CN_AGENT_TOKEN="$token" --entrypoint /usr/local/bin/cnctl connector-engine:local tools "$@"; }

step "cnctl tools list through the console's /mcp"
out="$(cnctl list)"; echo "$out"
grep -q "${alias}__search" <<<"$out" || fail "search is not listed"
if grep -q "${alias}__mystery" <<<"$out"; then fail "an off function is listed"; fi

step "cnctl tools call: a read runs at once"
out="$(cnctl call "${alias}__search" --args '{}')"; echo "$out"
grep -q '^ok' <<<"$out" || fail "search did not run"

step "cnctl tools call: an ask function waits for the owner"
out="$(cnctl call "${alias}__purge" --args '{}')"; echo "$out"
grep -q pending_approval <<<"$out" || fail "purge did not wait"
r="$(api GET /api/approvals)"
id="$(body "$r" | jq -r '.approvals[0].id')"
[ -n "$id" ] && [ "$id" != null ] || fail "no approval listed: $r"
r="$(api POST "/api/approvals/$id" '{"approve":true}')"; [ "$(code "$r")" = 204 ] || fail "approve: $r"
for _ in $(seq 1 30); do
  out="$(cnctl call approval_status --args "{\"approval_id\":\"$id\"}")"
  grep -q '^ok' <<<"$out" && break
  sleep 1
done
echo "$out"
grep -q '^ok' <<<"$out" || fail "the approved call did not answer"

echo "console check ok"
