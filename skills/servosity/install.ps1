# install.ps1 - install servosity-cli and servosity-mcp on Windows.
#
# Pulls prebuilt binaries from the latest GitHub Release of msp-skills.
# Both the CLI and the MCP server are installed in one shot.
#
# Env vars:
#   MSP_SKILLS_RELEASE_BASE  Override release base URL for testing.
#   DRY_RUN=1                Print resolved URLs and exit without downloading.
#   INSTALL_DIR              Destination dir (default: $env:LOCALAPPDATA\Programs\msp-skills).

$ErrorActionPreference = "Stop"

$Skill   = "servosity"
$CliBin  = "servosity-cli.exe"
$McpBin  = "servosity-mcp.exe"

$Owner = if ($env:MSP_SKILLS_OWNER) { $env:MSP_SKILLS_OWNER } else { "servosity" }
$Repo  = if ($env:MSP_SKILLS_REPO)  { $env:MSP_SKILLS_REPO }  else { "msp-skills" }
$ReleaseBase = if ($env:MSP_SKILLS_RELEASE_BASE) {
  $env:MSP_SKILLS_RELEASE_BASE
} else {
  "https://github.com/$Owner/$Repo/releases/latest/download"
}
$InstallDir = if ($env:INSTALL_DIR) { $env:INSTALL_DIR } else { "$env:LOCALAPPDATA\Programs\msp-skills" }

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
Write-Host "  servosity-cli --version"
Write-Host ""
Write-Host "Next:"
Write-Host "  Read skills\servosity\README.md for first command + auth."
Write-Host "  For Claude Desktop or ChatGPT Desktop, read skills\servosity\mcp-install.md."
