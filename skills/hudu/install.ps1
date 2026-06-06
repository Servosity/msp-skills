# install.ps1 - install hudu-cli and hudu-mcp on Windows.
#
# Pulls prebuilt binaries from this skill's latest GitHub Release (hudu-v*).
# Both the CLI and the MCP server are installed in one shot.
#
# Env vars:
#   MSP_SKILLS_RELEASE_BASE  Override release base URL for testing.
#   DRY_RUN=1                Print resolved URLs and exit without downloading.
#   INSTALL_DIR              Destination dir (default: $env:LOCALAPPDATA\Programs\msp-skills).

$ErrorActionPreference = "Stop"

$Skill   = "hudu"
$CliBin  = "hudu-cli.exe"
$McpBin  = "hudu-mcp.exe"

$Owner = if ($env:MSP_SKILLS_OWNER) { $env:MSP_SKILLS_OWNER } else { "servosity" }
$Repo  = if ($env:MSP_SKILLS_REPO)  { $env:MSP_SKILLS_REPO }  else { "msp-skills" }
$ReleaseBase = if ($env:MSP_SKILLS_RELEASE_BASE) {
  $env:MSP_SKILLS_RELEASE_BASE
} else {
  # Each skill is versioned/tagged independently (hudu-vX.Y.Z),
  # so resolve THIS skill's latest release rather than the repo-wide /releases/latest/
  # (GitHub allows only one "latest" per repo). The releases API returns newest-first.
  # Paginate: with many skills releasing independently, this skill's newest
  # tag can sit beyond the first 100 repo releases.
  $tag = $null
  for ($page = 1; $page -le 5 -and -not $tag; $page++) {
    $rels = Invoke-RestMethod -Uri "https://api.github.com/repos/$Owner/$Repo/releases?per_page=100&page=$page" -UseBasicParsing
    if (-not $rels) { break }
    $tag = ($rels | Where-Object { $_.tag_name -like "$Skill-v*" } | Select-Object -First 1).tag_name
  }
  if (-not $tag) { throw "No $Skill-v* release found in $Owner/$Repo. Has the first release been published?" }
  "https://github.com/$Owner/$Repo/releases/download/$tag"
}
$InstallDir = if ($env:INSTALL_DIR) { $env:INSTALL_DIR } else { "$env:LOCALAPPDATA\Programs\msp-skills" }

# Detect arch.
$arch = "amd64"
if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { $arch = "arm64" }

$cliUrl = "$ReleaseBase/$($CliBin.Replace('.exe',''))-windows-$arch.exe"
$mcpUrl = "$ReleaseBase/$($McpBin.Replace('.exe',''))-windows-$arch.exe"

Write-Host "Skill:        $Skill"
Write-Host "Detected:     windows/$arch"
Write-Host "CLI URL:      $cliUrl"
Write-Host "MCP URL:      $mcpUrl"
Write-Host "Install dir:  $InstallDir"

if ($env:DRY_RUN -eq "1") {
  Write-Host "DRY_RUN=1 set; not downloading."
  exit 0
}

New-Item -ItemType Directory -Force -Path $InstallDir | Out-Null

function Get-File {
  param([string]$Url, [string]$Dest)
  Write-Host "  fetching $Url"
  Invoke-WebRequest -Uri $Url -OutFile $Dest -UseBasicParsing
}

Get-File -Url $cliUrl -Dest (Join-Path $InstallDir $CliBin)
Get-File -Url $mcpUrl -Dest (Join-Path $InstallDir $McpBin)

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
Write-Host "  hudu-cli --version"
Write-Host ""
Write-Host "Next:"
Write-Host "  First command + auth: https://github.com/$Owner/$Repo/tree/main/skills/hudu#readme"
Write-Host "  Claude Desktop / ChatGPT wire-up: https://github.com/$Owner/$Repo/blob/main/skills/hudu/mcp-install.md"
