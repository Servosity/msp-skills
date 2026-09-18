# install.ps1 - install superops-cli and superops-mcp on Windows.
#
# Pulls prebuilt binaries from this skill's latest GitHub Release (superops-v*).
# Both the CLI and the MCP server are installed in one shot.
#
# What this script checks before it touches anything in INSTALL_DIR:
#   1. The release it resolved is SEALED: the GitHub API reports
#      "immutable": true for the tag (a sealed release can never have an asset
#      swapped after publication). Anything else - false, missing, malformed -
#      is refused as ambiguous. MSP_SKILLS_ALLOW_MUTABLE=1 waives only this.
#      Boundary: ConvertFrom-Json must parse the whole reply and the top-level
#      property must be a boolean; anything else makes the installer refuse.
#   2. Every binary matches its published .sha256 sidecar (one record, the
#      asset's own name, compared with Get-FileHash). Never waived.
#   3. No other installer is working in INSTALL_DIR: an exclusively opened lock
#      file, INSTALL_DIR\.msp-install.lock, is held for the whole run. A lock
#      older than one hour is reported as stale, never stolen.
#
# Replacement is one tracked transaction: both binaries are downloaded and
# verified into a private staging directory under INSTALL_DIR first; then, for
# the CLI and then the MCP server, the old file is renamed aside to
# <dest>.prev.<txid> and the new file is renamed into place. On ANY failure
# every completed rename is undone in reverse, so both destinations end up
# exactly as they were; if an undo itself fails the backups are kept, named,
# and the exit status is non-zero. Backups are deleted only after both
# replacements succeed. Windows lets a running program be renamed but not
# deleted, so a backup that cannot be deleted is reported by name, not treated
# as an error.
#
# Crash window: a process kill or power loss between the first and the last
# rename can leave one binary replaced and the other not, plus a
# <dest>.prev.<txid> backup. Re-running the installer repairs it: it
# re-verifies and replaces both binaries again.
#
# Env vars:
#   MSP_SKILLS_ALLOW_MUTABLE=1  Install even when the release is not sealed
#                               (or when MSP_SKILLS_RELEASE_BASE is set).
#   MSP_SKILLS_RELEASE_BASE     Override release base URL for testing. An
#                               arbitrary base cannot be proven sealed, so it
#                               also requires MSP_SKILLS_ALLOW_MUTABLE=1.
#   MSP_SKILLS_API_BASE         Test hook: replaces BOTH the api.github.com and
#                               the github.com origins with one fixture server.
#                               GITHUB_TOKEN is never sent to an override.
#   GITHUB_TOKEN / GH_TOKEN     Optional; sent to https://api.github.com only.
#   DRY_RUN=1                   Print resolved URLs and exit without locking,
#                               fetching or writing anything.
#   INSTALL_DIR                 Destination dir (default: $env:LOCALAPPDATA\Programs\msp-skills).

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

$Skill   = "superops"
$CliBin  = "superops-cli.exe"
$McpBin  = "superops-mcp.exe"

$Owner = if ($env:MSP_SKILLS_OWNER) { $env:MSP_SKILLS_OWNER } else { "servosity" }
$Repo  = if ($env:MSP_SKILLS_REPO)  { $env:MSP_SKILLS_REPO }  else { "msp-skills" }
if ($env:MSP_SKILLS_API_BASE) {
  $ApiBase = $env:MSP_SKILLS_API_BASE.TrimEnd('/')
  $DownloadOrigin = $ApiBase
} else {
  $ApiBase = "https://api.github.com"
  $DownloadOrigin = "https://github.com"
}

# Fetch a GitHub API URL and return the parsed JSON. GITHUB_TOKEN/GH_TOKEN
# (optional) lifts the 60/hr unauthenticated rate limit; it is attached ONLY to
# the real API origin, never to an override. Any transport or HTTP failure is
# reported as an API failure and stops the install.
function Get-GitHubJson {
  param([string]$Url)
  $headers = @{}
  if ($Url.StartsWith("https://api.github.com/")) {
    $tok = if ($env:GITHUB_TOKEN) { $env:GITHUB_TOKEN } elseif ($env:GH_TOKEN) { $env:GH_TOKEN } else { "" }
    if ($tok) { $headers["Authorization"] = "Bearer $tok" }
  }
  try {
    if ($headers.Count -gt 0) {
      # A request carrying the token follows NO redirects, so the header can
      # never be forwarded to another origin.
      $resp = Invoke-WebRequest -Uri $Url -Headers $headers -UseBasicParsing -MaximumRedirection 0
    } else {
      $resp = Invoke-WebRequest -Uri $Url -UseBasicParsing
    }
  } catch {
    throw "GitHub API request failed (HTTP error, rate limit, or unreachable): $Url : $($_.Exception.Message). Set GITHUB_TOKEN to lift the unauthenticated rate limit, then retry."
  }
  if ([string]::IsNullOrWhiteSpace($resp.Content)) {
    throw "GitHub API request failed (empty reply): $Url"
  }
  try {
    return ($resp.Content | ConvertFrom-Json)
  } catch {
    throw "GitHub API request failed (malformed JSON): $Url"
  }
}

$tag = $null
$ReleaseBase = if ($env:MSP_SKILLS_RELEASE_BASE) {
  $env:MSP_SKILLS_RELEASE_BASE.TrimEnd('/')
} else {
  # Each skill is versioned/tagged independently (superops-vX.Y.Z),
  # so resolve THIS skill's latest release rather than the repo-wide /releases/latest/
  # (GitHub allows only one "latest" per repo). The releases API returns newest-first.
  # Paginate: with many skills releasing independently, this skill's newest
  # tag can sit beyond the first 100 repo releases. Bound: 5 pages x 100 = the
  # newest 500 releases. Every skill releases in fleet waves, so its newest tag
  # is always near the top; a skill absent from the newest 500 is reported
  # below with that bound named. An API failure throws above and stops the
  # install - only a successful listing with no match means "no release".
  for ($page = 1; $page -le 5 -and -not $tag; $page++) {
    $rels = Get-GitHubJson -Url "$ApiBase/repos/$Owner/$Repo/releases?per_page=100&page=$page"
    if (-not $rels) { break }
    $tag = ($rels | Where-Object { $_.tag_name -like "$Skill-v*" } | Select-Object -First 1).tag_name
  }
  if (-not $tag) { throw "No $Skill-v* release found among the newest 500 releases (5 pages x 100) of $Owner/$Repo. Has the first release been published?" }
  "$DownloadOrigin/$Owner/$Repo/releases/download/$tag"
}
$InstallDir = if ($env:INSTALL_DIR) { $env:INSTALL_DIR } else { "$env:LOCALAPPDATA\Programs\msp-skills" }

# Detect arch.
$arch = "amd64"
if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { $arch = "arm64" }

$cliUrl = "$ReleaseBase/$($CliBin.Replace('.exe',''))-windows-$arch.exe"
$mcpUrl = "$ReleaseBase/$($McpBin.Replace('.exe',''))-windows-$arch.exe"
$cliAsset = $cliUrl.Substring($cliUrl.LastIndexOf('/') + 1)
$mcpAsset = $mcpUrl.Substring($mcpUrl.LastIndexOf('/') + 1)

Write-Host "Skill:        $Skill"
Write-Host "Detected:     windows/$arch"
Write-Host "CLI URL:      $cliUrl"
Write-Host "MCP URL:      $mcpUrl"
Write-Host "Install dir:  $InstallDir"

if ($env:DRY_RUN -eq "1") {
  Write-Host "DRY_RUN=1 set; not downloading."
  exit 0
}

# --- 1. Sealed-release check -------------------------------------------------
if ($env:MSP_SKILLS_ALLOW_MUTABLE -eq "1") {
  Write-Host "WARNING: MSP_SKILLS_ALLOW_MUTABLE=1 set; skipping the sealed-release check."
} elseif ($env:MSP_SKILLS_RELEASE_BASE) {
  throw "MSP_SKILLS_RELEASE_BASE is set: an arbitrary release base cannot be proven to be a sealed (immutable) GitHub release. Set MSP_SKILLS_ALLOW_MUTABLE=1 to install from it anyway."
} else {
  $rel = Get-GitHubJson -Url "$ApiBase/repos/$Owner/$Repo/releases/tags/$tag"
  # ONE release object; its top-level "immutable" must be the JSON boolean true.
  # A missing property, a string "true", null, or anything else is ambiguous.
  $prop = $null
  if ($rel -is [System.Management.Automation.PSCustomObject]) { $prop = $rel.PSObject.Properties["immutable"] }
  if ($prop -and ($prop.Value -is [bool]) -and ($prop.Value -eq $true)) {
    Write-Host "Release:      $tag (sealed)"
  } elseif ($prop -and ($prop.Value -is [bool]) -and ($prop.Value -eq $false)) {
    throw "Release $tag is NOT sealed (`"immutable`": false): its assets could be replaced after publication. Refusing to install. Set MSP_SKILLS_ALLOW_MUTABLE=1 to install anyway."
  } else {
    throw "Release ${tag}: the GitHub API did not report a usable `"immutable`" field; refusing as ambiguous. Set MSP_SKILLS_ALLOW_MUTABLE=1 to install anyway."
  }
}

# --- 2. Lock, staging, download, verify --------------------------------------
New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null

$lockPath = Join-Path $InstallDir ".msp-install.lock"
$lock = $null
# A lock file older than one hour is stale: reported, never silently reused.
if (Test-Path -LiteralPath $lockPath) {
  $age = $null
  try { $age = (Get-Item -LiteralPath $lockPath -Force).LastWriteTimeUtc } catch { }
  if ($age -and ($age -lt [DateTime]::UtcNow.AddHours(-1))) {
    throw "Stale lock: $lockPath is older than one hour. If no installer is running, delete it and retry."
  }
}
try {
  $lock = [System.IO.File]::Open($lockPath, [System.IO.FileMode]::OpenOrCreate, [System.IO.FileAccess]::ReadWrite, [System.IO.FileShare]::None)
} catch {
  throw "Another installer is running in $InstallDir (lock: $lockPath). Retry when it finishes."
}

# Staging lives INSIDE INSTALL_DIR so every rename below stays on one volume
# and is therefore atomic.
$txid = [System.IO.Path]::GetRandomFileName().Replace('.', '')
$stage = Join-Path $InstallDir ".msp-install.$txid"
$undo = New-Object System.Collections.ArrayList   # entries: @(from, to), in order
$committed = $false
$failure = $null
$restoreIncomplete = $false

function Get-File {
  param([string]$Url, [string]$Dest)
  Write-Host "  fetching $Url"
  try {
    Invoke-WebRequest -Uri $Url -OutFile $Dest -UseBasicParsing
  } catch {
    throw "Download failed: $Url : $($_.Exception.Message)"
  }
}

# Test-Sha256 <asset-name>: the sidecar <asset-name>.sha256 must hold exactly
# one record, "<64 hex>  <asset-name>" (shasum/sha256sum format, optional "*"
# binary marker). Nothing else in the file is interpreted. The hex digest is
# compared case-insensitively with Get-FileHash.
function Test-Sha256 {
  param([string]$Name)
  $sidecar = Join-Path $stage "$Name.sha256"
  $lines = @([System.IO.File]::ReadAllText($sidecar) -split "`r?`n" | Where-Object { $_.Trim() -ne "" })
  if ($lines.Count -ne 1) { throw "Checksum sidecar $Name.sha256 has $($lines.Count) lines; expected exactly one." }
  $m = [regex]::Match($lines[0], '^([0-9A-Fa-f]{64})\s+\*?(.+?)\s*$')
  if (-not $m.Success) { throw "Checksum sidecar $Name.sha256 is not a SHA-256 record." }
  $expected = $m.Groups[1].Value
  $recorded = $m.Groups[2].Value
  if ($recorded -cne $Name) { throw "Checksum sidecar $Name.sha256 names '$recorded', not '$Name'; refusing." }
  $actual = (Get-FileHash -Algorithm SHA256 -LiteralPath (Join-Path $stage $Name)).Hash
  if ($actual -ine $expected) { throw "SHA-256 MISMATCH for ${Name}; refusing to install." }
  Write-Host "  verified $Name (sha256 $($expected.ToLower()))"
}

# Invoke-TxMove <from> <to>: journal the rename, THEN run it, so a stop
# landing between the two can never leave a completed rename unrecorded. <to>
# never exists beforehand (backups carry the transaction id; a destination has
# just been moved aside), so the rollback reverses exactly the entries whose
# <to> exists.
function Invoke-TxMove {
  param([string]$From, [string]$To)
  [void]$undo.Add(@($From, $To))
  Move-Item -LiteralPath $From -Destination $To
}

try {
  # Stamp the lock so a later installer can tell a live lock from a stale one.
  $stamp = [System.Text.Encoding]::ASCII.GetBytes("$PID")
  $lock.SetLength(0); $lock.Write($stamp, 0, $stamp.Length); $lock.Flush()

  New-Item -ItemType Directory -Path $stage | Out-Null

  Get-File -Url $cliUrl           -Dest (Join-Path $stage $cliAsset)
  Get-File -Url "$cliUrl.sha256"  -Dest (Join-Path $stage "$cliAsset.sha256")
  Get-File -Url $mcpUrl           -Dest (Join-Path $stage $mcpAsset)
  Get-File -Url "$mcpUrl.sha256"  -Dest (Join-Path $stage "$mcpAsset.sha256")

  Test-Sha256 $cliAsset
  Test-Sha256 $mcpAsset

  # --- 3. Tracked rename transaction -----------------------------------------
  foreach ($pair in @(@($cliAsset, $CliBin), @($mcpAsset, $McpBin))) {
    $asset = $pair[0]; $bin = $pair[1]
    $dest = Join-Path $InstallDir $bin
    $prev = "$dest.prev.$txid"
    if (Test-Path -LiteralPath $dest) { Invoke-TxMove -From $dest -To $prev }
    Invoke-TxMove -From (Join-Path $stage $asset) -To $dest
  }
  $committed = $true

  foreach ($bin in @($CliBin, $McpBin)) {
    $prev = Join-Path $InstallDir "$bin.prev.$txid"
    if (Test-Path -LiteralPath $prev) {
      try {
        Remove-Item -LiteralPath $prev -Force
      } catch {
        Write-Host "NOTE: the previous $bin is still running, so its backup could not be deleted: $prev"
        Write-Host "      Delete it once that program has exited."
      }
    }
  }
} catch {
  $failure = $_.Exception.Message
  [Console]::Error.WriteLine("$failure")
} finally {
  # Rollback lives in finally, not catch: finally also runs when the pipeline
  # is stopped (Ctrl+C), which never reaches catch.
  if (-not $committed -and $undo.Count -gt 0) {
    [Console]::Error.WriteLine("Install failed; restoring the previous binaries...")
    for ($i = $undo.Count - 1; $i -ge 0; $i--) {
      $from = $undo[$i][0]; $to = $undo[$i][1]
      if (Test-Path -LiteralPath $to) {
        try {
          Move-Item -LiteralPath $to -Destination $from
        } catch {
          $restoreIncomplete = $true
          [Console]::Error.WriteLine("  could not restore $from from $to")
        }
      }
    }
    if ($restoreIncomplete) {
      [Console]::Error.WriteLine("Restore incomplete. Backups kept (transaction $txid):")
      foreach ($bin in @($CliBin, $McpBin)) {
        $b = Join-Path $InstallDir "$bin.prev.$txid"
        if (Test-Path -LiteralPath $b) { [Console]::Error.WriteLine("  $b") }
      }
    } else {
      [Console]::Error.WriteLine("Previous binaries restored; nothing changed.")
    }
    if (-not $failure) { $failure = "installer was stopped" }
  }
  if (Test-Path -LiteralPath $stage) { Remove-Item -LiteralPath $stage -Recurse -Force -ErrorAction SilentlyContinue }
  if ($lock) { $lock.Close() }
  Remove-Item -LiteralPath $lockPath -Force -ErrorAction SilentlyContinue
}
# throw, not exit: the README one-liner runs this through the caller's own
# session, where exit would end that session.
if ($restoreIncomplete) { throw "Install failed and the restore was incomplete; backups are listed above." }
if ($failure) { throw "Install failed; nothing was changed (see above)." }

# Add to user PATH if not present.
$userPath = [Environment]::GetEnvironmentVariable("Path", "User")
if ($userPath -notlike "*$InstallDir*") {
  $newPath = if ([string]::IsNullOrEmpty($userPath)) { $InstallDir } else { "$userPath;$InstallDir" }
  [Environment]::SetEnvironmentVariable("Path", $newPath, "User")
  Write-Host ""
  Write-Host "Added $InstallDir to your user PATH. Open a new terminal to pick it up."
}

Write-Host ""
Write-Host "Installed:"
Write-Host "  $InstallDir\$CliBin"
Write-Host "  $InstallDir\$McpBin"
Write-Host ""
Write-Host "Verify (in a new terminal):"
Write-Host "  superops-cli --version"
Write-Host ""
Write-Host "Next:"
Write-Host "  First command + auth: https://github.com/$Owner/$Repo/tree/main/skills/superops#readme"
Write-Host "  Claude Desktop / ChatGPT wire-up: https://github.com/$Owner/$Repo/blob/main/skills/superops/mcp-install.md"
