#!/usr/bin/env bash
# tcd installer — curl -fsSL https://raw.githubusercontent.com/iluxav/tinycd/main/install.sh | bash
#
# Environment variables:
#   TCD_VERSION             — tag to install (default: latest)
#   TCD_REPO                — GitHub repo (default: iluxav/tinycd)
#   PREFIX                  — install prefix (default: /usr/local)
#   TCD_USER                — if set to 1, install to ~/.local/bin instead
#   TCD_SKIP_DOCKER_GROUP   — if 1, skip the docker-group bootstrap on Linux

set -euo pipefail

REPO="${TCD_REPO:-iluxav/tinycd}"
VERSION="${TCD_VERSION:-latest}"
PREFIX="${PREFIX:-/usr/local}"

need() { command -v "$1" >/dev/null 2>&1 || { echo "tcd-install: missing required tool: $1" >&2; exit 1; }; }
need curl
need tar
need uname

case "${TCD_USER:-0}" in
  1|true|yes) BINDIR="${HOME}/.local/bin" ;;
  *)          BINDIR="${PREFIX}/bin" ;;
esac

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) echo "tcd-install: unsupported arch: $arch" >&2; exit 1 ;;
esac
case "$os" in
  linux|darwin) ;;
  mingw*|msys*|cygwin*) echo "tcd-install: use the Windows zip from https://github.com/${REPO}/releases" >&2; exit 1 ;;
  *) echo "tcd-install: unsupported OS: $os" >&2; exit 1 ;;
esac

resolve_version() {
  if [ "$VERSION" = "latest" ]; then
    curl -fsSL "https://api.github.com/repos/${REPO}/releases/latest" \
      | grep -o '"tag_name": *"[^"]*"' \
      | head -n1 \
      | sed -E 's/.*"tag_name": *"([^"]+)".*/\1/'
  else
    echo "$VERSION"
  fi
}

version="$(resolve_version)"
if [ -z "$version" ]; then
  echo "tcd-install: could not resolve version" >&2
  exit 1
fi

pkg="tcd_${version}_${os}_${arch}"
url="https://github.com/${REPO}/releases/download/${version}/${pkg}.tar.gz"
sum_url="https://github.com/${REPO}/releases/download/${version}/SHA256SUMS"

echo "tcd-install: downloading ${pkg} (${version})"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

curl -fsSL -o "$tmp/pkg.tar.gz" "$url"

# Checksum
if curl -fsSL -o "$tmp/SHA256SUMS" "$sum_url" 2>/dev/null; then
  want="$(grep "${pkg}.tar.gz" "$tmp/SHA256SUMS" | awk '{print $1}')"
  if [ -n "$want" ]; then
    have="$(shasum -a 256 "$tmp/pkg.tar.gz" 2>/dev/null | awk '{print $1}')"
    if [ -z "$have" ]; then
      have="$(sha256sum "$tmp/pkg.tar.gz" | awk '{print $1}')"
    fi
    if [ "$want" != "$have" ]; then
      echo "tcd-install: checksum mismatch (want=$want, have=$have)" >&2
      exit 1
    fi
    echo "tcd-install: checksum ok"
  fi
fi

tar -C "$tmp" -xzf "$tmp/pkg.tar.gz"
src="$tmp/$pkg/tcd"
if [ ! -f "$src" ]; then
  echo "tcd-install: binary not found in archive" >&2
  exit 1
fi

mkdir -p "$BINDIR"
dst="${BINDIR}/tcd"
if [ -w "$BINDIR" ]; then
  install -m 0755 "$src" "$dst"
else
  echo "tcd-install: ${BINDIR} is not writable; using sudo"
  sudo install -m 0755 "$src" "$dst"
fi

echo "tcd-install: installed $("$dst" --version 2>/dev/null || echo tcd) to $dst"
case ":$PATH:" in
  *":$BINDIR:"*) ;;
  *) echo "tcd-install: note: $BINDIR is not in PATH" ;;
esac

# ----------------------------------------------------------------------------
# Linux: ensure the invoking user can talk to the docker daemon. tcd talks to
# /var/run/docker.sock for everything; without docker-group membership every
# `tcd init` and `tcd deploy` fails with "permission denied". We add the user
# to the group here so they only have to log out once, after install.
# ----------------------------------------------------------------------------
ensure_docker_group() {
  [ "$os" = "linux" ] || return 0
  case "${TCD_SKIP_DOCKER_GROUP:-0}" in 1|true|yes) return 0 ;; esac
  command -v docker >/dev/null 2>&1 || {
    echo "tcd-install: docker not found — install docker before running 'tcd init'"
    return 0
  }
  # Resolve the real invoking user even when this script runs under sudo.
  local target="${SUDO_USER:-${USER:-$(id -un)}}"
  if id -nG "$target" 2>/dev/null | tr ' ' '\n' | grep -qx docker; then
    echo "tcd-install: user '$target' is already in the docker group"
    return 0
  fi
  if ! getent group docker >/dev/null 2>&1; then
    echo "tcd-install: 'docker' group does not exist — skipping (is docker really installed?)"
    return 0
  fi
  command -v usermod >/dev/null 2>&1 || {
    echo "tcd-install: usermod not available — add '$target' to the docker group manually"
    return 0
  }
  echo "tcd-install: adding '$target' to the docker group (sudo prompts may follow)"
  if sudo usermod -aG docker "$target"; then
    echo "tcd-install: '$target' added to docker group"
    echo "tcd-install: log out and back in (or reconnect SSH) before running 'tcd init'"
  else
    echo "tcd-install: warning: failed to add '$target' to docker group; do this manually:"
    echo "  sudo usermod -aG docker $target"
  fi
}

ensure_docker_group
