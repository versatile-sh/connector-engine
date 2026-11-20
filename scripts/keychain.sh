#!/usr/bin/env bash
set -euo pipefail

usage="usage: scripts/keychain.sh unlock|migrate|revert|status|recovery|import, with COMPOSE, CNCTL and PROJECT set by make"
op="${1:?$usage}"
read -r -a compose <<<"${COMPOSE:?$usage}"
cnctl="${CNCTL:-}"
project="${PROJECT:-connector-engine}"

deliver() {
  local cmd=(/usr/local/bin/connector-console unlock)
  if [ "$1" = engine ]; then cmd=(/usr/local/bin/connector-engine --config /etc/connector-engine/connector-engine.yaml unlock); fi
  "${compose[@]}" exec -T "$1" "${cmd[@]}" --op "$2"
}

keychain() {
  if [ -z "$cnctl" ] || ! [ -x "$cnctl" ]; then return 3; fi
  "$cnctl" keychain "$1" --name "$project"
}

report() {
  local protection
  if [ "$2" = key ]; then protection="$(keychain get | deliver "$1" "$3")"; else protection="$(deliver "$1" "$2")"; fi
  echo "$1: $protection"
}

warn() { printf 'warning: %s\n' "$*" >&2; }

unlock() {
  local code=0 protection
  keychain ensure >/dev/null || code=$?
  case "$code" in
    0)
      for service in engine console; do
        protection="$(report "$service" key unlock)"
        echo "$protection"
        if [ "${protection#*: }" = plaintext ]; then warn "the $service root key is still a plaintext file; run: make keychain-migrate"; fi
      done
      ;;
    3)
      warn "no OS keychain here (or no Go to build bin/cnctl): new root keys stay plaintext files in their volumes, and the console shows a warning"
      for service in engine console; do report "$service" plaintext; done
      ;;
    *) exit "$code" ;;
  esac
}

wrapped() {
  for service in engine console; do
    if [ "$(deliver "$service" status)" != wrapped ]; then echo "$service is not wrapped" >&2; return 1; fi
  done
}

case "$op" in
  unlock) unlock ;;
  status) for service in engine console; do report "$service" status; done ;;
  migrate)
    keychain ensure >/dev/null || { echo "keychain-migrate needs an OS keychain and bin/cnctl" >&2; exit 1; }
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
