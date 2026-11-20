#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
work="$(mktemp -d)"
pid=""
cleanup() {
  [ -n "$pid" ] && kill "$pid" 2>/dev/null && wait "$pid" 2>/dev/null || true
  rm -rf "$work"
}
trap cleanup EXIT

fail() { echo "smoke: $*" >&2; exit 1; }

cd "$root"
go build -o "$work/connector-engine" ./cmd/connector-engine
go build -o "$work/vsh" ./cmd/vsh

cat >"$work/connector-engine.yaml" <<YAML
mode: dev
server:
  listen: 127.0.0.1:0
  grpcListen: 127.0.0.1:0
  operatorListen: 127.0.0.1:0
  publicURL: http://engine.smoke
auth:
  localIssuer: {enabled: true}
  operator: {tokenRef: "env:CN_OPERATOR_TOKEN"}
bootstrap:
  roles:
    - {tenant: t1, person: ana, role: tenant_admin, scope: t/t1}
YAML
cat >"$work/adapters.yaml" <<YAML
apiVersion: acplug/v1
kind: Adapters
engine: connector-engine
mode: dev
ports:
  store: {use: memory}
  secrets: {use: memory}
  auth: {use: builtin:local}
YAML

export CN_OPERATOR_TOKEN=smoke-operator
"$work/connector-engine" --config "$work/connector-engine.yaml" --adapters "$work/adapters.yaml" >"$work/out.log" 2>"$work/err.log" &
pid=$!

for _ in $(seq 1 50); do
  grep -q 'http=' "$work/out.log" 2>/dev/null && break
  kill -0 "$pid" 2>/dev/null || fail "the engine exited: $(cat "$work/err.log")"
  sleep 0.1
done
http="http://$(sed -n 's/.*http=\([^ ]*\).*/\1/p' "$work/out.log")"
operator=""
for _ in $(seq 1 50); do
  operator="$(sed -n 's/.*operator=\([^ ]*\).*/\1/p' "$work/out.log")"
  [ -n "$operator" ] && break
  sleep 0.1
done
[ -n "$operator" ] || fail "no operator address in the boot line"
operator="http://$operator"

status() { curl -s -o /dev/null -w '%{http_code}' "$@"; }

[ "$(status "$http/livez")" = 204 ] || fail "livez"
[ "$(status "$http/readyz")" = 204 ] || fail "readyz"
[ "$(status "$http/v1/event-types")" = 200 ] || fail "event types are public"
[ "$(status "$http/v1/roles")" = 401 ] || fail "no token is refused"

token() { curl -s -X POST -H "Authorization: Bearer $CN_OPERATOR_TOKEN" "$http/local/token" -d "$1" | sed -n 's/.*"token":"\([^"]*\)".*/\1/p'; }
admin="$(token '{"sub":"ana","tenant":"t1"}')"
[ -n "$admin" ] || fail "no token from the local issuer"
agent="$(token '{"sub":"x","tenant":"t1","workspace":"w1","agent":"x","session":"s1","aud":["http://engine.smoke"]}')"
[ "$(status -H "Authorization: Bearer $agent" "$http/v1/roles")" = 403 ] || fail "an agent principal is refused (P1)"

vsh() { VSH_ADMIN_URL="$1" VSH_ADMIN_TOKEN="$2" "$work/vsh" admin "${@:3}"; }
vsh "$operator" "$CN_OPERATOR_TOKEN" tenants create t1 | grep -q '"id": "t1"' || fail "tenants create"
vsh "$operator" "$CN_OPERATOR_TOKEN" adapters list | grep -q '"engine": "connector-engine"' || fail "adapters list"
vsh "$http" "$admin" roles put --tenant t1 --person bia --role approver --scope t/t1 | grep -q '"person": "bia"' || fail "roles put"
vsh "$http" "$admin" roles list --tenant t1 --person bia | grep -q '"role": "approver"' || fail "roles list"
vsh "$http" "$admin" events list | grep -q 'connector.call.finished' || fail "events list"
if vsh "$http" "$agent" roles list --tenant t1 2>/dev/null; then fail "vsh with an agent token"; fi

kill -TERM "$pid"
wait "$pid" || fail "the engine did not stop cleanly"
pid=""
echo "smoke ok"
