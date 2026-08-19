#!/usr/bin/env bash
# install.sh — download gsc-mcp from GitHub Releases into ~/.local/bin
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/geniushub-seo/gsc-mcp/main/install.sh | bash
#   curl -fsSL ... | bash -s -- --dry-run
#   DRY_RUN=1 ./install.sh
#
# Does NOT: run gcloud login, merge MCP config, or edit shell rc files.
# After install, an assisting agent must run the printed follow-up commands.

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
    if command -v shasum >/dev/null 2>&1; then
      actual="$(shasum -a 256 "${asset}" | awk '{print $1}')"
    elif command -v sha256sum >/dev/null 2>&1; then
      actual="$(sha256sum "${asset}" | awk '{print $1}')"
    else
      die "need shasum or sha256sum to verify ${asset}"
    fi
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
    log "PATH: ${INSTALL_DIR} is not on your PATH. An assisting agent should use the absolute binary path immediately."
    log "Manual installers can add it with:"
    log "  echo 'export PATH=\"\$HOME/.local/bin:\$PATH\"' >> ~/.zshrc   # or ~/.bashrc"
    log "  then open a new terminal"
  fi
fi

cat <<EOF >&2

Next steps (this installer does not run them):

  Run these six commands in order. They do not branch. With an AI agent, the
  agent runs every one of them; the user's only task is choosing a Google
  account and ticking BOTH consent checkboxes in the browser page gcloud opens.
  Do not send the user to Terminal.

  1) gcloud auth login
     A separate login from step 3 - both are required. Without this one,
     step 2 fails with an expired-credentials error.

  2) gcloud projects list
     Copy an id from the PROJECT_ID column; steps 4 and 5 both need it. An
     empty list means the user has no GCP project: create one at
     https://console.cloud.google.com/projectcreate, then repeat this step.

  3) gcloud auth application-default login --scopes=https://www.googleapis.com/auth/webmasters.readonly,https://www.googleapis.com/auth/cloud-platform
     The consent page has TWO checkboxes and both must be ticked, or this
     fails with "cloud-platform scope is required but not consented".

  4) gcloud auth application-default set-quota-project PROJECT_ID
     PROJECT_ID is the id from step 2 - never send the literal placeholder.
     ADC has no project of its own; without this, every query fails with a
     403 that reads like a Search Console permission problem but is not one.
     If this exits 1 saying the account lacks "serviceusage.services.use",
     retrying will not help: sign in again with an account that has rights on
     that project, or have its administrator grant
     roles/serviceusage.serviceUsageConsumer, then repeat step 3.

  5) gcloud services enable searchconsole.googleapis.com --project=PROJECT_ID
     The same id as step 4. A fresh project does not have this API enabled,
     and skipping this step is the most common reason setup stalls after a
     login that appeared to succeed.

  6) gsc-mcp doctor
     It must print "list_sites OK" before the setup counts as working. Then:
     gsc-mcp setup
     If the shell reports "command not found", the install directory is not on
     PATH yet - use the absolute path printed above.

doctor is read-only: it runs every check plus one real list_sites call, writes
no files, and prints the fix for whatever it finds. Run it again after any
command above that fails.

Need gcloud? https://cloud.google.com/sdk/docs/install
  macOS: brew install --cask google-cloud-sdk
  A freshly installed gcloud may not be on the PATH of the current shell; open
  a new terminal, or use the absolute path the installer printed.

EOF
