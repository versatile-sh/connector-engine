#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
module="$(go list -m)"
status=0

core_imports="$(go list -f '{{.ImportPath}} {{join .Imports " "}} {{join .TestImports " "}} {{join .XTestImports " "}}' ./... \
  | grep -v "^$module/selfhosted[/ ]" | grep -E " $module/selfhosted(/| |$)" || true)"
if [ -n "$core_imports" ]; then
  echo "layering: core packages import selfhosted/ (the direction is core <- selfhosted):" >&2
  echo "$core_imports" >&2
  status=1
fi

headless="$(go list -deps ./cmd/connector-engine ./cmd/vsh | grep -E "^$module/selfhosted(/|$)|^modernc.org/|^github.com/agents-control/plugkit" || true)"
if [ -n "$headless" ]; then
  echo "layering: the headless binaries link self-hosted, SQLite or upstream plugkit packages:" >&2
  echo "$headless" >&2
  status=1
fi

[ "$status" -ne 0 ] || echo "layering ok"
exit "$status"
