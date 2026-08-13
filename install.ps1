# install.ps1 — download gsc-mcp from GitHub Releases into a local bin directory.
#
# Usage:
#   irm https://raw.githubusercontent.com/geniushub-seo/gsc-mcp/main/install.ps1 | iex
#
#   # with options (piping to iex cannot pass parameters):
#   & ([scriptblock]::Create((irm https://raw.githubusercontent.com/geniushub-seo/gsc-mcp/main/install.ps1))) -DryRun
#
#   # or set env vars before piping:
#   $env:DRY_RUN = '1'; irm https://.../install.ps1 | iex
#
# Does NOT: run gcloud login, merge MCP config, or edit PATH.
# After install, run the two printed follow-up commands yourself.
#
# PowerShell counterpart of install.sh. Keep behaviour aligned with that file.
#
# Paths are built with [System.IO.Path]::Combine rather than Join-Path: Join-Path
# resolves the drive qualifier through the PowerShell provider, which throws on
# non-Windows hosts and makes -DryRun impossible to exercise outside Windows.

param(
    [switch]$DryRun,
    [switch]$Help,
    [string]$Repo,
    [string]$InstallDir,
    [ValidateSet('', 'amd64', 'arm64')]
    [string]$Arch = ''
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'

# All diagnostics go to stderr so stdout stays clean, matching install.sh.
function Log { param([string]$Message = '') [Console]::Error.WriteLine($Message) }
function Die { param([string]$Message) Log "error: $Message"; exit 1 }

if ($Help -or $env:GSC_MCP_HELP -eq '1') {
    Log @'
install.ps1 — install gsc-mcp from GitHub Releases

Options:
  -DryRun       Print actions without downloading or writing files
  -Repo         GitHub owner/name (default: geniushub-seo/gsc-mcp)
  -InstallDir   Install directory (default: %LOCALAPPDATA%\Programs\gsc-mcp)
  -Arch         Force amd64 or arm64 (default: detect)
  -Help         Show this help

Environment (useful when piping to iex, which cannot pass parameters):
  GSC_MCP_REPO         Same as -Repo
  GSC_MCP_INSTALL_DIR  Same as -InstallDir
  GSC_MCP_ARCH         Same as -Arch
  DRY_RUN=1            Same as -DryRun
'@
    exit 0
}

# Environment fallbacks so `irm | iex` (which cannot bind parameters) still works.
if (-not $Repo) {
    $Repo = if ($env:GSC_MCP_REPO) { $env:GSC_MCP_REPO } else { 'geniushub-seo/gsc-mcp' }
}
if (-not $InstallDir) {
    $InstallDir = if ($env:GSC_MCP_INSTALL_DIR) {
        $env:GSC_MCP_INSTALL_DIR
    } else {
        [System.IO.Path]::Combine($env:LOCALAPPDATA, 'Programs', 'gsc-mcp')
    }
}
if (-not $Arch -and $env:GSC_MCP_ARCH) { $Arch = $env:GSC_MCP_ARCH }
if (-not $DryRun -and $env:DRY_RUN -eq '1') { $DryRun = $true }

if ($PSVersionTable.PSVersion.Major -lt 5) {
    Die "PowerShell 5.1 or newer is required (found $($PSVersionTable.PSVersion))."
}

# PROCESSOR_ARCHITEW6432 is set when a 32-bit process runs on 64-bit Windows;
# it reports the real machine architecture, so it must win over PROCESSOR_ARCHITECTURE.
if (-not $Arch) {
    $procArch = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
    switch ($procArch) {
        'AMD64' { $Arch = 'amd64' }
        'ARM64' {
            Die @"
Windows on ARM has no native build yet.
ARM64 Windows can run the x64 binary through emulation — retry with:
  & ([scriptblock]::Create((irm https://raw.githubusercontent.com/$Repo/main/install.ps1))) -Arch amd64
Or build from source: go build -o gsc-mcp.exe ./cmd/gsc-mcp
"@
        }
        'x86' {
            Die "32-bit Windows is not supported. Build from source: go build -o gsc-mcp.exe ./cmd/gsc-mcp"
        }
        default {
            Die "unsupported architecture '$procArch'. Build from source: go build -o gsc-mcp.exe ./cmd/gsc-mcp"
        }
    }
}

$asset   = "gsc-mcp-windows-$Arch.exe"
$baseUrl = "https://github.com/$Repo/releases/latest/download"
$binUrl  = "$baseUrl/$asset"
$sumUrl  = "$baseUrl/checksums.txt"
$dest    = [System.IO.Path]::Combine($InstallDir, 'gsc-mcp.exe')

Log "gsc-mcp installer"
Log "  repo:    $Repo"
Log "  asset:   $asset"
Log "  install: $dest"
if ($DryRun) { Log "  mode:    dry-run (no download, no write)" }

if ($DryRun) {
    Log "would download: $binUrl"
    Log "would download: $sumUrl"
    Log "would verify sha256 of $asset"
    Log "would install to $dest"
    Log "would run: Unblock-File $dest"
} else {
    # Windows PowerShell 5.1 defaults to older TLS on some builds; GitHub requires 1.2+.
    try {
        [Net.ServicePointManager]::SecurityProtocol =
            [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
    } catch {
        Log "note: could not raise TLS version; download may fail on older systems"
    }

    # Invoke-WebRequest's progress bar makes 5.1 downloads an order of magnitude slower.
    $prevProgress = $ProgressPreference
    $ProgressPreference = 'SilentlyContinue'

    $tmp = [System.IO.Path]::Combine([System.IO.Path]::GetTempPath(), "gsc-mcp-install." + [System.IO.Path]::GetRandomFileName())
    New-Item -ItemType Directory -Path $tmp -Force | Out-Null

    try {
        $tmpAsset = [System.IO.Path]::Combine($tmp, $asset)
        $tmpSums  = [System.IO.Path]::Combine($tmp, 'checksums.txt')

        Log "downloading $asset..."
        try {
            Invoke-WebRequest -Uri $binUrl -OutFile $tmpAsset -UseBasicParsing
        } catch {
            Die "download failed: $binUrl (is the repo public and has a release?)"
        }

        Log "downloading checksums.txt..."
        try {
            Invoke-WebRequest -Uri $sumUrl -OutFile $tmpSums -UseBasicParsing
        } catch {
            Die "download failed: $sumUrl"
        }

        Log "verifying checksum..."
        # checksums.txt lines: "<sha256>  <filename>"
        $pattern = '\s' + [regex]::Escape($asset) + '\s*$'
        $entry = Get-Content -LiteralPath $tmpSums | Where-Object { $_ -match $pattern } | Select-Object -First 1
        if (-not $entry) { Die "no checksum entry for $asset" }

        $expected = ($entry -split '\s+')[0]
        $actual   = (Get-FileHash -LiteralPath $tmpAsset -Algorithm SHA256).Hash
        if ($expected -ne $actual) {
            Remove-Item -LiteralPath $tmpAsset -Force -ErrorAction SilentlyContinue
            Die "checksum mismatch for $asset (expected $expected, got $actual)"
        }
        Log "checksum OK"

        New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
        try {
            Move-Item -LiteralPath $tmpAsset -Destination $dest -Force
        } catch {
            # Windows locks running executables, unlike POSIX rename-over-open-file.
            Die @"
could not write $dest
The file may be in use. Close any AI client that has gsc-mcp running
(Claude Desktop, Cursor, VS Code), then run this installer again.
Original error: $($_.Exception.Message)
"@
        }

        # Files downloaded from the internet carry a Zone.Identifier stream that
        # makes SmartScreen block execution; this is the Windows analogue of
        # macOS quarantine removal in install.sh.
        Log "removing download block marker (if present)..."
        try { Unblock-File -LiteralPath $dest -ErrorAction SilentlyContinue } catch { }

        Log "installed: $dest"

        $userPath = [Environment]::GetEnvironmentVariable('PATH', 'User')
        $onPath = ($env:PATH -split ';' | Where-Object { $_.TrimEnd('\') -ieq $InstallDir.TrimEnd('\') }).Count -gt 0
        if ($onPath) {
            Log "PATH: $InstallDir is available"
        } else {
            Log "PATH: $InstallDir is not on your PATH. Add it with:"
            Log "  [Environment]::SetEnvironmentVariable('PATH', `"$userPath;$InstallDir`", 'User')"
            Log "  then open a new terminal"
        }
    } finally {
        $ProgressPreference = $prevProgress
        Remove-Item -LiteralPath $tmp -Recurse -Force -ErrorAction SilentlyContinue
    }
}

Log @"

Next steps (run these yourself — this script will not):

  1) Sign in with Application Default Credentials (browser):
     gcloud auth application-default login ``
       --scopes=https://www.googleapis.com/auth/webmasters.readonly,https://www.googleapis.com/auth/cloud-platform

  2) Point ADC at a quota project (required — ADC has no project of its own,
     and without this every query fails with a 403 that looks like a
     permission problem):
     gcloud auth application-default set-quota-project YOUR_PROJECT_ID

     Use the GCP project that has the Search Console API enabled:
     https://console.cloud.google.com/apis/library/searchconsole.googleapis.com

  3) Wire MCP clients / verify access:
     $dest setup

If anything fails, run "$dest doctor": it checks everything, makes one real
list_sites call, writes no files, and prints the fix for what it finds.

Need gcloud? https://cloud.google.com/sdk/docs/install
  Windows: winget install Google.CloudSDK
  If gcloud is installed but "not recognized", it is on disk but not on PATH —
  it usually lives in %LOCALAPPDATA%\Google\Cloud SDK\google-cloud-sdk\bin.

"@
