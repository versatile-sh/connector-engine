#!/usr/bin/env bash
set -euo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"
status=0
fail() { echo "check-docs: $*" >&2; status=1; }

docs=(README.md ARCHITECTURE.md CONTRIBUTING.md SECURITY.md selfhosted/README.md)
while IFS= read -r f; do docs+=("$f"); done < <(find docs -name '*.md' -not -path '*/node_modules/*' | sort)

targets="$(grep -hoE '^[a-zA-Z][a-zA-Z0-9_-]*:' Makefile selfhosted/selfhosted.mk | tr -d ':' | sort -u)"
for f in "${docs[@]}"; do
  while IFS= read -r t; do
    grep -qx "$t" <<<"$targets" || fail "$f names 'make $t', which is not a target"
  done < <(grep -oE '\bmake [a-z][a-z0-9-]*[a-z0-9]([^a-z0-9*-]|$)' "$f" | awk '{print $2}' | tr -cd 'a-z0-9-\n' | grep -vE '^(targets|sure)$' | sort -u)
  while IFS= read -r s; do
    [ -e "$s" ] || fail "$f names $s, which does not exist"
  done < <(grep -oE '\bscripts/[a-zA-Z0-9_.-]+\.(sh|ps1)' "$f" | sort -u)
done

max=65536
svgs=(docs/assets/*.svg)
for svg in "${svgs[@]}"; do
  size="$(wc -c <"$svg")"
  [ "$size" -le "$max" ] || fail "$svg is $size bytes, over $max"
  if grep -qE '<text|font-family|@font-face|href="(https?:)?//' "$svg"; then
    fail "$svg uses a font or an external reference; outline the text with docs/assets/src"
  fi
done

[ "$status" = 0 ] && echo "check-docs ok: ${#docs[@]} pages, ${#svgs[@]} banners"
exit "$status"
