#!/usr/bin/env bash

set -euo pipefail

log() { printf '%s\n' "$*" >&2; }
fail() { log "error: $*"; exit 1; }

command -v hermes >/dev/null 2>&1 || fail "Hermes is not installed or is not on PATH"

script_dir="$(CDPATH= cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(CDPATH= cd -- "${script_dir}/.." && pwd)"

if [ -n "${GSC_MCP_BIN:-}" ]; then
  gsc_mcp_bin="${GSC_MCP_BIN}"
elif command -v gsc-mcp >/dev/null 2>&1; then
  gsc_mcp_bin="$(command -v gsc-mcp)"
elif [ -x "${HOME}/.local/bin/gsc-mcp" ]; then
  gsc_mcp_bin="${HOME}/.local/bin/gsc-mcp"
else
  install_dir="${GSC_MCP_INSTALL_DIR:-${HOME}/.local/bin}"
  log "gsc-mcp was not found; installing it with the repository install.sh"
  bash "${repo_root}/install.sh"
  gsc_mcp_bin="${install_dir}/gsc-mcp"
fi

[ -x "${gsc_mcp_bin}" ] || fail "not executable: ${gsc_mcp_bin}"

log "Verifying real GSC access with: ${gsc_mcp_bin} doctor"
doctor_output="$("${gsc_mcp_bin}" doctor 2>&1 || true)"
printf '%s\n' "${doctor_output}"
if ! printf '%s\n' "${doctor_output}" | grep -Eq 'list_sites OK.*[1-9][0-9]* propert'; then
  log ""
  log "AGENT ACTION REQUIRED: doctor did not confirm a non-empty list_sites result."
  log "The agent must run the commands below through its own terminal tool."
  log "The user only chooses an account and approves access in Google's browser page; do not send the user to Terminal."
  log "Replace YOUR_PROJECT_ID with a verified project ID, then rerun this script:"
  log "  gcloud auth application-default login --scopes=https://www.googleapis.com/auth/webmasters.readonly,https://www.googleapis.com/auth/cloud-platform"
  log "  gcloud auth application-default set-quota-project YOUR_PROJECT_ID"
  log "  bash ${script_dir}/setup.sh"
  exit 2
fi

if existing_command="$(hermes config get mcp_servers.gsc.command 2>/dev/null)"; then
  if [ "${existing_command}" != "${gsc_mcp_bin}" ]; then
    log "error: Hermes already has gsc pointing to: ${existing_command}"
    log "Preserving that setting. To replace it explicitly, run:"
    log "  hermes mcp remove gsc"
    log "  bash ${script_dir}/setup.sh"
    exit 1
  fi
  log "Hermes already has gsc pointing to the selected binary; preserving it"
else
  log "Adding gsc to the active Hermes profile"
  printf 'Y\n' | hermes mcp add gsc --command "${gsc_mcp_bin}" --connect-timeout 30
fi

if ! hermes config get mcp_servers.gsc --json >/dev/null 2>&1; then
  fail "Hermes did not save mcp_servers.gsc"
fi

test_output="$(hermes mcp test gsc 2>&1 || true)"
printf '%s\n' "${test_output}"
if ! printf '%s\n' "${test_output}" | grep -q 'Tools discovered: 6'; then
  fail "Hermes did not discover all six gsc tools; inspect the test output above"
fi

log "Hermes onboarding complete. Start a new Hermes session, then call gsc list_sites."
