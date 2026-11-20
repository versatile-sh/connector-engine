#!/bin/sh
# Install connector-engine from a GitHub release, verified against checksums.txt.
#
#   curl -fsSL https://raw.githubusercontent.com/versatile-sh/connector-engine/main/scripts/install.sh | sh
#
# Settings, all optional:
#   CONNECTOR_ENGINE_VERSION      a release tag such as v0.1.0; default: the latest release
#   CONNECTOR_ENGINE_PROFILE      person (default: vsh) or headless (connector-engine and vsh)
#   CONNECTOR_ENGINE_INSTALL_DIR  where the binaries go; default: $HOME/.local/bin
#   CONNECTOR_ENGINE_BASE_URL     where the release files are; default: the GitHub release
set -eu

repo="versatile-sh/connector-engine"
version="${CONNECTOR_ENGINE_VERSION:-latest}"
profile="${CONNECTOR_ENGINE_PROFILE:-person}"
install_dir="${CONNECTOR_ENGINE_INSTALL_DIR:-$HOME/.local/bin}"

say() { printf '%s\n' "$*"; }
fail() { printf 'install.sh: %s\n' "$*" >&2; exit 1; }

case "$(uname -s)" in
  Linux) os=linux ;;
  Darwin) os=darwin ;;
  *) fail "unsupported system $(uname -s); on Windows use install.ps1" ;;
esac

case "$(uname -m)" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) fail "unsupported architecture $(uname -m)" ;;
esac

case "$profile" in
  person) archive="connector-engine_${os}_${arch}.tar.gz"; binaries="vsh" ;;
  headless) archive="connector-engine-headless_${os}_${arch}.tar.gz"; binaries="connector-engine vsh" ;;
  *) fail "CONNECTOR_ENGINE_PROFILE must be person or headless, not $profile" ;;
esac

if [ -n "${CONNECTOR_ENGINE_BASE_URL:-}" ]; then
  base_url="$CONNECTOR_ENGINE_BASE_URL"
elif [ "$version" = latest ]; then
  base_url="https://github.com/$repo/releases/latest/download"
else
  base_url="https://github.com/$repo/releases/download/$version"
fi

if command -v curl >/dev/null 2>&1; then
  fetch() { curl -fsSL --proto '=https,file' -o "$2" "$1"; }
elif command -v wget >/dev/null 2>&1; then
  fetch() { wget -q -O "$2" "$1"; }
else
  fail "needs curl or wget"
fi

if command -v sha256sum >/dev/null 2>&1; then
  sha256() { sha256sum "$1" | cut -d' ' -f1; }
elif command -v shasum >/dev/null 2>&1; then
  sha256() { shasum -a 256 "$1" | cut -d' ' -f1; }
else
  fail "needs sha256sum or shasum"
fi

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT INT TERM

say "Downloading $archive ($version) from $base_url"
fetch "$base_url/$archive" "$tmp/$archive" || fail "could not download $base_url/$archive"
fetch "$base_url/checksums.txt" "$tmp/checksums.txt" || fail "could not download $base_url/checksums.txt"

want="$(awk -v f="$archive" '$2 == f { print $1 }' "$tmp/checksums.txt")"
[ -n "$want" ] || fail "checksums.txt has no entry for $archive"
got="$(sha256 "$tmp/$archive")"
[ "$want" = "$got" ] || fail "SHA-256 mismatch for $archive: expected $want, got $got. Nothing was installed."
say "Verified SHA-256 $got"

mkdir -p "$tmp/x" "$install_dir"
tar -xzf "$tmp/$archive" -C "$tmp/x"
for b in $binaries; do
  [ -f "$tmp/x/$b" ] || fail "$archive has no $b"
  cp "$tmp/x/$b" "$install_dir/$b.new"
  chmod 0755 "$install_dir/$b.new"
  mv -f "$install_dir/$b.new" "$install_dir/$b"
  say "Installed $install_dir/$b"
done

case ":$PATH:" in
  *":$install_dir:"*) ;;
  *) say "" ; say "$install_dir is not on your PATH. Add it, for example:"; say "  export PATH=\"$install_dir:\$PATH\"" ;;
esac

say ""
if [ "$profile" = person ]; then
  say "Next step: start it; it opens the console in your browser."
  say "  vsh up --open"
else
  say "Next step: write connector-engine.yaml and adapters.yaml, then run"
  say "  connector-engine --config connector-engine.yaml --adapters adapters.yaml adapters check"
  say "Guide: https://github.com/$repo/blob/main/docs/enterprise/headless.md"
fi
