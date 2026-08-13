#!/usr/bin/env bash
# install.sh — download gsc-mcp from GitHub Releases into ~/.local/bin
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/geniushub-seo/gsc-mcp/main/install.sh | bash
#   curl -fsSL ... | bash -s -- --dry-run
#   DRY_RUN=1 ./install.sh
#
# Does NOT: run gcloud login, merge MCP config, or edit shell rc files.
# After install, run the two printed follow-up commands yourself.

set -euo pipefail

REPO="${GSC_MCP_REPO:-geniushub-seo/gsc-mcp}"
INSTALL_DIR="${GSC_MCP_INSTALL_DIR:-${HOME}/.local/bin}"
DRY_RUN="${DRY_RUN:-0}"

log() { printf '%s\n' "$*" >&2; }
die() { log "error: $*"; exit 1; }

for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=1 ;;
    -h|--help)
      cat <<'EOF' >&2
install.sh — install gsc-mcp from GitHub Releases

Options:
  --dry-run   Print actions without downloading or writing files
  -h, --help  Show this help

Environment:
  GSC_MCP_REPO         GitHub owner/name (default: geniushub-seo/gsc-mcp)
  GSC_MCP_INSTALL_DIR  Install directory (default: ~/.local/bin)
  DRY_RUN=1            Same as --dry-run
EOF
      exit 0
      ;;
    *) die "unknown argument: $arg (try --help)" ;;
  esac
done

os="$(uname -s | tr '[:upper:]' '[:lower:]')"
arch="$(uname -m)"
case "$os" in
  darwin|linux) ;;
  msys*|mingw*|cygwin*) os=windows ;;
  *) die "unsupported OS '$os'. Build from source: go build -o gsc-mcp ./cmd/gsc-mcp" ;;
esac
case "$arch" in
  x86_64|amd64) arch=amd64 ;;
  aarch64|arm64) arch=arm64 ;;
  *) die "unsupported arch '$arch'. Build from source: go build -o gsc-mcp ./cmd/gsc-mcp" ;;
esac

ext=""
if [ "$os" = "windows" ]; then ext=".exe"; fi
asset="gsc-mcp-${os}-${arch}${ext}"
base_url="https://github.com/${REPO}/releases/latest/download"
bin_url="${base_url}/${asset}"
sum_url="${base_url}/checksums.txt"

log "gsc-mcp installer"
log "  repo:    ${REPO}"
log "  asset:   ${asset}"
log "  install: ${INSTALL_DIR}/gsc-mcp${ext}"
if [ "$DRY_RUN" = "1" ]; then
  log "  mode:    dry-run (no download, no write)"
fi

tmp="$(mktemp -d "${TMPDIR:-/tmp}/gsc-mcp-install.XXXXXX")"
cleanup() { rm -rf "$tmp"; }
trap cleanup EXIT

if [ "$DRY_RUN" = "1" ]; then
  log "would download: ${bin_url}"
  log "would download: ${sum_url}"
  log "would verify sha256 of ${asset}"
  log "would install to ${INSTALL_DIR}/gsc-mcp${ext}"
  if [ "$os" = "darwin" ]; then
    log "would run: xattr -d com.apple.quarantine ${INSTALL_DIR}/gsc-mcp${ext}"
  fi
else
  log "downloading ${asset}..."
  curl -fsSL "$bin_url" -o "${tmp}/${asset}" || die "download failed: ${bin_url} (is the repo public and has a release?)"
  log "downloading checksums.txt..."
  curl -fsSL "$sum_url" -o "${tmp}/checksums.txt" || die "download failed: ${sum_url}"

  log "verifying checksum..."
  (
    cd "$tmp"
    # checksums.txt lines: "<sha256>  <filename>"
    expected="$(grep -E "[[:space:]]${asset}\$" checksums.txt | awk '{print $1}')"
    [ -n "$expected" ] || die "no checksum entry for ${asset}"
    actual="$(shasum -a 256 "${asset}" | awk '{print $1}')"
    if [ "$expected" != "$actual" ]; then
      rm -f "${asset}"
      die "checksum mismatch for ${asset} (expected ${expected}, got ${actual})"
    fi
  )
  log "checksum OK"

  mkdir -p "$INSTALL_DIR"
  dest="${INSTALL_DIR}/gsc-mcp${ext}"
  # Atomic-ish replace for idempotent upgrades
  mv -f "${tmp}/${asset}" "$dest"
  chmod +x "$dest"

  if [ "$os" = "darwin" ]; then
    log "removing macOS quarantine attribute (if present)..."
    xattr -d com.apple.quarantine "$dest" 2>/dev/null || true
  fi

  log "installed: $dest"
  if command -v "$dest" >/dev/null 2>&1 || echo ":$PATH:" | grep -q ":${INSTALL_DIR}:"; then
    log "PATH: ${INSTALL_DIR} is available"
  else
    log "PATH: ${INSTALL_DIR} is not on your PATH. Add it with:"
    log "  echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.zshrc   # or ~/.bashrc"
    log "  then open a new terminal"
  fi
fi

cat <<EOF >&2

Next steps (run these yourself — this script will not):

  1) Sign in with Application Default Credentials (browser):
     gcloud auth application-default login \\
       --scopes=https://www.googleapis.com/auth/webmasters.readonly,https://www.googleapis.com/auth/cloud-platform

  2) Wire MCP clients / verify access:
     gsc-mcp setup

Need gcloud? https://cloud.google.com/sdk/docs/install
  macOS: brew install --cask google-cloud-sdk

EOF
