#!/usr/bin/env bash
# install.sh — download gsc-mcp from GitHub Releases into ~/.local/bin
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/geniushub-seo/gsc-mcp/main/install.sh | bash
#   curl -fsSL ... | bash -s -- --dry-run
#   DRY_RUN=1 ./install.sh
#
# Also unpacks the bundled skills into ~/.claude/skills when that directory's
# parent already exists. --no-skills opts out, --with-skills forces it.
#
# Does NOT: run gcloud login, merge MCP config, or edit shell rc files.
# After install, an assisting agent must run the printed follow-up commands.

set -euo pipefail

REPO="${GSC_MCP_REPO:-geniushub-seo/gsc-mcp}"
INSTALL_DIR="${GSC_MCP_INSTALL_DIR:-${HOME}/.local/bin}"
SKILLS_DIR="${GSC_MCP_SKILLS_DIR:-${HOME}/.claude/skills}"
# auto: install only when SKILLS_DIR's parent exists, so we never create a
# Claude Code tree for someone who does not use Claude Code.
SKILLS="${GSC_MCP_SKILLS:-auto}"
DRY_RUN="${DRY_RUN:-0}"

log() { printf '%s\n' "$*" >&2; }
die() { log "error: $*"; exit 1; }

for arg in "$@"; do
  case "$arg" in
    --dry-run) DRY_RUN=1 ;;
    --no-skills) SKILLS=never ;;
    --with-skills) SKILLS=always ;;
    -h|--help)
      cat <<'EOF' >&2
install.sh — install gsc-mcp from GitHub Releases

Options:
  --dry-run      Print actions without downloading or writing files
  --no-skills    Do not install the bundled skills
  --with-skills  Install the bundled skills even if ~/.claude does not exist
  -h, --help     Show this help

Environment:
  GSC_MCP_REPO         GitHub owner/name (default: geniushub-seo/gsc-mcp)
  GSC_MCP_INSTALL_DIR  Install directory (default: ~/.local/bin)
  GSC_MCP_SKILLS_DIR   Skills directory (default: ~/.claude/skills)
  GSC_MCP_SKILLS       auto (default) | always | never
  GSC_MCP_BASE_URL     Release asset base URL (default: GitHub latest release)
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
skills_asset="gsc-mcp-skills.tar.gz"
base_url="${GSC_MCP_BASE_URL:-https://github.com/${REPO}/releases/latest/download}"
bin_url="${base_url}/${asset}"
sum_url="${base_url}/checksums.txt"
skills_url="${base_url}/${skills_asset}"

case "$SKILLS" in
  always) do_skills=1; skills_why="forced by --with-skills" ;;
  never)  do_skills=0; skills_why="disabled by --no-skills" ;;
  auto)
    if [ -d "$(dirname "$SKILLS_DIR")" ]; then
      do_skills=1; skills_why="$(dirname "$SKILLS_DIR") exists"
    else
      do_skills=0; skills_why="$(dirname "$SKILLS_DIR") does not exist"
    fi
    ;;
  *) die "invalid GSC_MCP_SKILLS='${SKILLS}' (want auto, always or never)" ;;
esac

# Printed in the closing Next steps block. Both branches below overwrite it;
# this value only survives if nothing at all runs.
skills_note="skills: skipped (${skills_why})"

log "gsc-mcp installer"
log "  repo:    ${REPO}"
log "  asset:   ${asset}"
log "  install: ${INSTALL_DIR}/gsc-mcp${ext}"
if [ "$do_skills" = "1" ]; then
  log "  skills:  ${SKILLS_DIR} (${skills_why})"
else
  log "  skills:  skipped (${skills_why})"
fi
if [ "$DRY_RUN" = "1" ]; then
  log "  mode:    dry-run (no download, no write)"
fi

tmp="$(mktemp -d "${TMPDIR:-/tmp}/gsc-mcp-install.XXXXXX")"
cleanup() { rm -rf "$tmp"; }
trap cleanup EXIT

# Skills are a bonus, not the product: a release without the asset must still
# leave a working binary behind, so a missing asset warns and returns 0. A
# checksum mismatch is different — that is tampering, and it stops the script.
install_skills() {
  log "downloading ${skills_asset}..."
  if ! curl -fsSL "$skills_url" -o "${tmp}/${skills_asset}" 2>/dev/null; then
    log "note: this release has no ${skills_asset}; skills not installed."
    log "      The binary works without them. Copy .agents/skills/* from the"
    log "      repo into ${SKILLS_DIR} by hand if you want them."
    skills_note="skills: not installed (release has no ${skills_asset})"
    return 0
  fi

  local expected actual
  expected="$(grep -E "[[:space:]]${skills_asset}\$" "${tmp}/checksums.txt" | awk '{print $1}')" || true
  if [ -z "${expected:-}" ]; then
    log "note: no checksum entry for ${skills_asset}; skills not installed."
    skills_note="skills: not installed (no checksum entry)"
    return 0
  fi
  if command -v shasum >/dev/null 2>&1; then
    actual="$(shasum -a 256 "${tmp}/${skills_asset}" | awk '{print $1}')"
  else
    actual="$(sha256sum "${tmp}/${skills_asset}" | awk '{print $1}')"
  fi
  [ "$expected" = "$actual" ] || die "checksum mismatch for ${skills_asset} (expected ${expected}, got ${actual})"
  log "checksum OK"

  local stage="${tmp}/skills"
  mkdir -p "$stage"
  tar -xzf "${tmp}/${skills_asset}" -C "$stage" || die "could not unpack ${skills_asset}"

  mkdir -p "$SKILLS_DIR"
  local added=0 kept=0 name path
  for path in "$stage"/*/; do
    [ -d "$path" ] || continue
    name="$(basename "$path")"
    if [ -e "${SKILLS_DIR}/${name}" ]; then
      # Never clobber: a local edit to a skill is worth more than our copy.
      log "  keep ${name} (already in ${SKILLS_DIR}, left untouched)"
      kept=$((kept + 1))
      continue
    fi
    cp -R "$path" "${SKILLS_DIR}/${name}"
    log "  add  ${name}"
    added=$((added + 1))
  done
  log "skills: ${added} added, ${kept} already present -> ${SKILLS_DIR}"
  skills_note="skills: ${added} added, ${kept} already present in ${SKILLS_DIR}"
}

if [ "$DRY_RUN" = "1" ]; then
  log "would download: ${bin_url}"
  log "would download: ${sum_url}"
  log "would verify sha256 of ${asset}"
  log "would install to ${INSTALL_DIR}/gsc-mcp${ext}"
  if [ "$os" = "darwin" ]; then
    log "would run: xattr -d com.apple.quarantine ${INSTALL_DIR}/gsc-mcp${ext}"
  fi
  if [ "$do_skills" = "1" ]; then
    log "would download: ${skills_url}"
    log "would verify sha256 of ${skills_asset}"
    log "would unpack skills into ${SKILLS_DIR} (existing skill dirs left untouched)"
    skills_note="skills: dry-run, nothing written (would unpack into ${SKILLS_DIR})"
  else
    log "would skip skills (${skills_why})"
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

  if [ "$do_skills" = "1" ]; then
    install_skills
  else
    log "skills: skipped (${skills_why})"
    log "  Copy .agents/skills/* from the repo into a directory your agent reads,"
    log "  or re-run with --with-skills to unpack them into ${SKILLS_DIR}."
  fi
fi

cat <<EOF >&2

${skills_note}

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
  macOS with homebrew:    brew install --cask google-cloud-sdk
  macOS without homebrew: do NOT install homebrew for this - its installer
  requires a local Administrator account. gcloud needs Python 3.10+, macOS
  ships 3.9.6, and install.sh is itself a Python program, so running it does
  not fix this. Set CLOUDSDK_PYTHON to a Python 3.10+ interpreter first
  (uv python install 3.12 needs no Administrator rights). Full steps are in
  INSTALL.md, section "No-homebrew macOS".
  A freshly installed gcloud may not be on the PATH of the current shell; open
  a new terminal, or use the absolute path the installer printed.

EOF
