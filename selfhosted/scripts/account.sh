#!/usr/bin/env bash
set -euo pipefail

usage="usage: selfhosted/scripts/account.sh reset-password|export [--yes]|import FILE [--yes], with COMPOSE and UNLOCK set by make; MODE=setup exports a shareable setup"
op="${1:?$usage}"
shift
read -r -a compose <<<"${COMPOSE:?$usage}"
read -r -a unlock <<<"${UNLOCK:-}"
backups="${BACKUP_DIR:-$HOME/connector-engine-backups}"
keep=7
yes="${YES:-}"
args=()
for a in "$@"; do
  if [ "$a" = --yes ]; then yes=1; else args+=("$a"); fi
done

host() { "${compose[@]}" exec -T console /usr/local/bin/connector-console host "$@"; }

ask() {
  local answer
  IFS= read -r -s -p "$1: " answer </dev/tty
  echo >&2
  printf '%s' "$answer"
}

ask_twice() {
  local first second
  first="$(ask "$1")"
  second="$(ask "$1 again")"
  if [ "$first" != "$second" ]; then echo "the two entries differ" >&2; return 1; fi
  printf '%s' "$first"
}

confirm() {
  if [ -n "$yes" ]; then return 0; fi
  local answer
  read -r -p "$1 [y/N] " answer </dev/tty
  case "$answer" in y | Y | yes) return 0 ;; esac
  echo "nothing changed" >&2
  exit 1
}

started() {
  local s
  for s in engine console; do docker inspect -f '{{.State.StartedAt}}' "$("${compose[@]}" ps -q "$s")"; done
}

warn_credentials() {
  cat >&2 <<'EOF'
A full backup holds every credential of this install:
  - the provider sign-ins and API keys of your connectors,
  - the access keys of your agents and the root keys that open everything else,
  - your account and its activity.
Anyone who has the file and its passphrase gets full access. Keep it somewhere safe and never share it.
The passphrase cannot be recovered: without it the file opens for nobody, you included.
EOF
}

export_backup() {
  local stamp out
  stamp="$(date -u +%Y%m%dT%H%M%SZ)"
  umask 077
  mkdir -p "$backups"
  if [ "${MODE:-full}" = setup ]; then
    out="$backups/connector-engine-setup-$stamp.setup.yaml"
    if ! host POST "/export?mode=setup" </dev/null >"$out.part"; then
      rm -f "$out.part"
      exit 1
    fi
    mv "$out.part" "$out"
    echo "wrote $out (no credentials: share it with anyone)"
    return
  fi
  warn_credentials
  confirm "Export a full backup with every credential?"
  local passphrase
  passphrase="$(ask_twice "Backup passphrase (at least 12 characters)")"
  out="$backups/connector-engine-backup-$stamp.cebak"
  if ! printf '%s' "$passphrase" | host POST /export >"$out.part"; then
    rm -f "$out.part"
    exit 1
  fi
  mv "$out.part" "$out"
  ls -1t "$backups"/connector-engine-backup-*.cebak | tail -n +$((keep + 1)) | while read -r old; do rm -f -- "$old"; done
  echo "wrote $out (the $keep newest backups are kept in $backups)"
}

import_file() {
  local file="${args[0]:-${FILE:-}}" passphrase="" before now
  if [ -z "$file" ] || ! [ -f "$file" ]; then echo "$usage" >&2; exit 2; fi
  if [ "$(head -c 5 "$file")" = CEBAK ]; then passphrase="$(ask "Passphrase of $(basename "$file")")"; fi
  { printf '%s\n' "$passphrase"; cat "$file"; } | host POST /inspect
  echo
  if [ -n "$passphrase" ]; then
    confirm "Replace everything on this install with this backup?"
    before="$(started)"
  else
    confirm "Add what this setup holds? Nothing already here changes."
  fi
  { printf '%s\n' "$passphrase"; cat "$file"; } | host POST /import
  echo
  if [ -z "$passphrase" ]; then
    echo "sign in to each added connector from the console"
    return
  fi
  echo "restored; waiting for the engine and the console to restart"
  for _ in $(seq 60); do
    if now="$(started 2>/dev/null)" && [ -z "$(comm -12 <(sort <<<"$before") <(sort <<<"$now"))" ]; then break; fi
    sleep 1
  done
  if [ "${#unlock[@]}" -gt 0 ]; then "${unlock[@]}"; fi
}

case "$op" in
  reset-password)
    password="$(ask_twice "New console password (at least 10 characters)")"
    printf '%s' "$password" | host POST /password
    echo "password changed; every console session is signed out"
    echo "access keys and apps signed in through the browser still work: revoke them on the Access page if you suspect a compromise"
    ;;
  export) export_backup ;;
  import) import_file ;;
  *) echo "$usage" >&2; exit 2 ;;
esac
