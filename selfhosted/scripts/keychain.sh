#!/usr/bin/env bash
set -euo pipefail

usage="usage: selfhosted/scripts/keychain.sh unlock|migrate|revert|status|recovery|import, with COMPOSE, VSH and PROJECT set by the mise tasks"
op="${1:?$usage}"
read -r -a compose <<<"${COMPOSE:?$usage}"
vsh="${VSH:-}"
project="${PROJECT:-connector-engine}"
attempts="${UNLOCK_ATTEMPTS:-6}"
backoff="${UNLOCK_BACKOFF:-1}"
confirm="${UNLOCK_CONFIRM:-60}"
ready_url="http://127.0.0.1:${CONSOLE_PORT:-7465}/api/ready"
protection=""

deliver() {
  local cmd=(/usr/local/bin/connector-console unlock)
  if [ "$1" = engine ]; then cmd=(/usr/local/bin/connector-engine-selfhosted --config /etc/connector-engine/connector-engine.yaml unlock); fi
  "${compose[@]}" exec -T "$1" "${cmd[@]}" --op "$2" "${@:3}"
}

running() { "${compose[@]}" ps --status running -q "$1" 2>/dev/null; }

keychain() {
  if [ -z "$vsh" ] || ! [ -x "$vsh" ]; then return 3; fi
  "$vsh" keychain "$1" --name "$project"
}

report() {
  if [ "$2" = key ]; then protection="$(keychain get | deliver "$1" "$3")" || return 1; else protection="$(deliver "$1" "$2")" || return 1; fi
  echo "$1: $protection"
}

warn() { printf 'warning: %s\n' "$*" >&2; }

open_both() {
  local service id
  for service in engine console; do
    id="$(running "$service")"
    if [ -z "$id" ]; then echo "$service is not running" >&2; return 1; fi
    printf -v "id_$service" '%s' "$id"
    if [ "$1" = key ]; then report "$service" key unlock || return 1; else report "$service" plaintext || return 1; fi
    if [ "$protection" = plaintext ] && [ "$1" = key ]; then warn "the $service root key is still a plaintext file; run: mise run keychain-migrate"; fi
  done
}

confirmed() {
  local service id state
  for _ in $(seq 1 "$confirm"); do
    for service in engine console; do
      id="id_$service"
      [ "$(running "$service")" = "${!id}" ] || { echo "$service was restarted or recreated after the unlock" >&2; return 1; }
      state="$(deliver "$service" status --wait 5s)" || return 1
      [ "$state" != locked ] || { echo "$service is locked again" >&2; return 1; }
    done
    case "$(curl -fsS --max-time 2 "$ready_url" 2>/dev/null)" in *'"ready":true'*) return 0 ;; esac
    sleep 1
  done
  echo "the console does not report the engine ready at $ready_url" >&2
  return 1
}

settle() {
  local attempt delay="$backoff"
  for attempt in $(seq 1 "$attempts"); do
    if open_both "$1" && confirmed; then return 0; fi
    if [ "$attempt" -lt "$attempts" ]; then
      warn "the stack is not unlocked yet (attempt $attempt of $attempts); retrying in ${delay}s"
      sleep "$delay"
      delay=$((delay * 2))
    fi
  done
  echo "unlock failed: the engine and the console did not both confirm unlocked after $attempts attempts; see mise run logs" >&2
  exit 1
}

unlock() {
  local code=0
  keychain ensure >/dev/null || code=$?
  case "$code" in
    0) settle key ;;
    3)
      warn "no OS keychain here (or no Go to build bin/vsh): new root keys stay plaintext files in their volumes, and the console shows a warning"
      settle plaintext
      ;;
    *) exit "$code" ;;
  esac
}

wrapped() {
  for service in engine console; do
    report "$service" status
    if [ "$protection" != wrapped ]; then echo "$service is not wrapped" >&2; return 1; fi
  done
}

case "$op" in
  unlock) unlock ;;
  status) for service in engine console; do report "$service" status; done ;;
  migrate)
    keychain ensure >/dev/null || { echo "keychain-migrate needs an OS keychain and bin/vsh" >&2; exit 1; }
    unlock
    for service in engine console; do report "$service" key migrate; done
    echo "restarting engine and console to prove they unlock from the keychain"
    "${compose[@]}" restart engine console
    unlock
    wrapped
    echo "done: both root keys rest wrapped; the unlock key is in the keychain as service connector-engine, account $project"
    ;;
  revert)
    for service in console engine; do report "$service" revert; done
    ;;
  recovery)
    warn "this prints the unlock key: keep it in a password manager, never in a file next to the volumes"
    keychain get
    ;;
  import) keychain import ;;
  *) echo "$usage" >&2; exit 2 ;;
esac
