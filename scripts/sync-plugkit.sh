#!/usr/bin/env bash
set -euo pipefail

usage="usage: scripts/sync-plugkit.sh <plugkit-checkout> [commit]   (default commit: the one in internal/plugkit/UPSTREAM.md; upstream is a private repository, internal/plugkit is the public source of record)"
src="${1:?$usage}"
root="$(cd "$(dirname "$0")/.." && pwd)"
dest="$root/internal/plugkit"
upstream="github.com/agents-control/plugkit"
module="$(cd "$root" && go list -m)"
packages=(adapters api/acplug/v1 conform discovery host outbox usage)

commit="${2:-}"
if [ -z "$commit" ] && [ -f "$dest/UPSTREAM.md" ]; then
  commit="$(sed -n 's/^Commit: `\([0-9a-f]*\)`$/\1/p' "$dest/UPSTREAM.md")"
fi
[ -n "$commit" ] || { echo "$usage" >&2; exit 2; }

work="$(mktemp -d)"
cleanup() { git -C "$src" worktree remove --force "$work/src" >/dev/null 2>&1 || true; rm -rf -- "$work"; }
trap cleanup EXIT
git -C "$src" worktree add --detach -q "$work/src" "$commit"
commit="$(git -C "$work/src" rev-parse HEAD)"

in_set() {
  local p
  for p in "${packages[@]}"; do [ "$1" = "$p" ] && return 0; done
  return 1
}

rm -rf -- "$dest"
mkdir -p "$dest"
cp "$work/src/LICENSE" "$dest/LICENSE"
dropped=()
for p in "${packages[@]}"; do
  mkdir -p "$dest/$p"
  find "$work/src/$p" -maxdepth 1 -type f \( -name '*.go' -o -name '*.proto' \) -exec cp {} "$dest/$p/" \;
  [ -d "$work/src/$p/testdata" ] && cp -r "$work/src/$p/testdata" "$dest/$p/"
  outside=""
  for imp in $(cat "$dest/$p"/*_test.go 2>/dev/null | grep -oE "\"$upstream/[^\"]+\"" | tr -d '"' | sort -u); do
    in_set "${imp#"$upstream/"}" || outside="$outside $imp"
  done
  if [ -n "$outside" ]; then
    dropped+=("\`$p\`: imports$outside")
    rm -f -- "$dest/$p"/*_test.go
    rm -rf -- "$dest/$p/testdata"
  fi
done

find "$dest" -name '*.go' -print0 | xargs -0 sed -i -E \
  "s#^([[:space:]]*(import[[:space:]]+)?([A-Za-z_][A-Za-z0-9_]*[[:space:]]+|[._][[:space:]]+)?)\"$upstream/#\1\"$module/internal/plugkit/#"

left="$(grep -rnE "^[[:space:]]*(import[[:space:]]+)?([A-Za-z_][A-Za-z0-9_]*[[:space:]]+|[._][[:space:]]+)?\"$upstream/" "$dest" --include='*.go' || true)"
[ -z "$left" ] || { echo "sync-plugkit: imports outside the copied set:" >&2; echo "$left" >&2; exit 1; }

{
  echo "# plugkit, in-module copy"
  echo
  echo "Upstream: \`$upstream\` (MIT, see LICENSE in this directory), a private repository. This copy is the public source of record."
  echo
  echo "Commit: \`$commit\`"
  echo
  echo "Synced: $(date -u +%Y-%m-%d)"
  echo
  echo "Packages: $(printf '`%s` ' "${packages[@]}")"
  echo
  echo "Rule: files are upstream files, byte for byte, except import specs, where \`$upstream/<p>\` reads \`$module/internal/plugkit/<p>\`. Change this directory only with \`scripts/sync-plugkit.sh\`."
  echo
  echo "A binary must not link this copy and upstream \`api/acplug/v1\` together: both register the same .proto files and the protobuf registry panics at init."
  echo
  if [ "${#dropped[@]}" -gt 0 ]; then
    echo "Packages whose tests are not copied (a package's tests share helpers, so they come all or none), because a test imports an upstream package outside this set:"
    echo
    printf -- '- %s\n' "${dropped[@]}"
  else
    echo "Tests not copied: none."
  fi
} >"$dest/UPSTREAM.md"

cd "$root"
unformatted="$(gofmt -l internal/plugkit)"
[ -z "$unformatted" ] || { echo "sync-plugkit: gofmt differs: $unformatted" >&2; exit 1; }
go vet ./internal/plugkit/...
echo "synced internal/plugkit at $commit"
