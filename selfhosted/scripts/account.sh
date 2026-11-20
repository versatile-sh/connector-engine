#!/usr/bin/env bash
set -euo pipefail

usage="usage: selfhosted/scripts/account.sh reset-password, with COMPOSE set by make"
op="${1:?$usage}"
read -r -a compose <<<"${COMPOSE:?$usage}"

host() { "${compose[@]}" exec -T console /usr/local/bin/connector-console host "$@"; }

ask_twice() {
  local first second
  read -r -s -p "$1: " first </dev/tty; echo >&2
  read -r -s -p "$1 again: " second </dev/tty; echo >&2
  if [ "$first" != "$second" ]; then echo "the two entries differ" >&2; return 1; fi
  printf '%s' "$first"
}

case "$op" in
  reset-password)
    password="$(ask_twice "New console password (at least 10 characters)")"
    printf '%s' "$password" | host POST /password
    echo "password changed; every console session is signed out"
    ;;
  *) echo "$usage" >&2; exit 2 ;;
esac
