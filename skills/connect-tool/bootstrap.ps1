# bootstrap.ps1 - install the connect-tool Skill on Windows, no Git required.
#
# Downloads the repo tarball over HTTPS, extracts ONLY skills/connect-tool, and
# installs it to %USERPROFILE%\.claude\skills\connect-tool. Then reports every
# dependency and stops at the one step a script cannot do for you: approving the
# Chrome extension.
#
#   irm https://raw.githubusercontent.com/servosity/msp-skills/main/skills/connect-tool/bootstrap.ps1 | iex
#
# Env vars:
#   MSP_SKILLS_OWNER / MSP_SKILLS_REPO / MSP_SKILLS_REF   override the source
#   INSTALL_DIR                                           override the destination
#   DRY_RUN=1                                             print the plan and exit

$ErrorActionPreference = "Stop"

$Owner = if ($env:MSP_SKILLS_OWNER) { $env:MSP_SKILLS_OWNER } else { "servosity" }
$Repo  = if ($env:MSP_SKILLS_REPO)  { $env:MSP_SKILLS_REPO }  else { "msp-skills" }
$Ref   = if ($env:MSP_SKILLS_REF)   { $env:MSP_SKILLS_REF }   else { "main" }
$InstallDir = if ($env:INSTALL_DIR) { $env:INSTALL_DIR } else { "$env:USERPROFILE\.claude\skills\connect-tool" }

$ExtensionUrl = "https://chromewebstore.google.com/detail/opencli/ildkmabpimmkaediidaifkhjpohdnifk"
$Tarball = "https://codeload.github.com/$Owner/$Repo/tar.gz/refs/heads/$Ref"

Write-Host "connect-tool bootstrap"
Write-Host "  source:      $Owner/$Repo@$Ref"
Write-Host "  destination: $InstallDir"

if ($env:DRY_RUN -eq "1") {
  Write-Host "DRY_RUN=1 set; not downloading."
  exit 0
}

# --- 1. Fetch the Skill ------------------------------------------------------
# A tarball, not a clone: this must work on a machine with no Git installed.
$tmp = Join-Path ([System.IO.Path]::GetTempPath()) ("connect-tool-" + [guid]::NewGuid().ToString("N"))
New-Item -ItemType Directory -Force -Path $tmp | Out-Null
try {
  $archive = Join-Path $tmp "repo.tar.gz"
  Write-Host "  fetching $Tarball"
  Invoke-WebRequest -Uri $Tarball -OutFile $archive -UseBasicParsing

  # tar.exe ships with Windows 10 1803+ and can extract a single subtree.
  $extract = Join-Path $tmp "x"
  New-Item -ItemType Directory -Force -Path $extract | Out-Null
  & tar.exe -xzf $archive -C $extract
  if ($LASTEXITCODE -ne 0) { throw "tar extraction failed (exit $LASTEXITCODE)" }

  $src = Get-ChildItem -Path $extract -Directory |
         ForEach-Object { Join-Path $_.FullName "skills\connect-tool" } |
         Where-Object { Test-Path $_ } |
         Select-Object -First 1
  if (-not $src) { throw "skills/connect-tool not found in the downloaded archive" }
  if (-not (Test-Path (Join-Path $src "SKILL.md"))) { throw "downloaded copy has no SKILL.md" }

  # Replace atomically enough: stage beside the destination, then swap.
  $parent = Split-Path -Parent $InstallDir
  New-Item -ItemType Directory -Force -Path $parent | Out-Null
  $staging = "$InstallDir.new"
  if (Test-Path $staging) { Remove-Item -Recurse -Force $staging }
  Copy-Item -Recurse -Force $src $staging
  if (Test-Path $InstallDir) {
    $backup = "$InstallDir.old"
    if (Test-Path $backup) { Remove-Item -Recurse -Force $backup }
    Move-Item $InstallDir $backup
  }
  Move-Item $staging $InstallDir
  if (Test-Path "$InstallDir.old") { Remove-Item -Recurse -Force "$InstallDir.old" }
  Write-Host "  installed the Skill to $InstallDir"
} finally {
  Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}

# --- 2. Dependencies ---------------------------------------------------------
function Have($name) { $null -ne (Get-Command $name -ErrorAction SilentlyContinue) }

$missing = @()
if (-not (Have "node")) {
  $missing += "Node 20+       winget install OpenJS.NodeJS.LTS"
} else {
  $major = ((& node --version) -replace '^v','' -split '\.')[0] -as [int]
  if ($major -lt 20) { $missing += "Node 20+       you have v$major; winget install OpenJS.NodeJS.LTS" }
}
if (-not (Have "uv")) {
  $missing += 'uv             powershell -ExecutionPolicy ByPass -c "irm https://astral.sh/uv/install.ps1 | iex"'
}
if (-not (Have "opencli")) { $missing += "opencli        npm install -g @jackwener/opencli" }

if ($missing.Count -gt 0) {
  Write-Host ""
  Write-Host "Install these, then re-run this script:"
  $missing | ForEach-Object { Write-Host "  - $_" }
  Write-Host ""
}

# --- 3. The step no script can do for you ------------------------------------
Write-Host ""
Write-Host "NEXT, in this order:"
Write-Host "  1. Add the OpenCLI Chrome extension (one click, opening now):"
Write-Host "     $ExtensionUrl"
Write-Host "  2. Restart Claude Code so it picks up the new Skill."
Write-Host "  3. Ask it: run the connect-tool dependency check"
Write-Host ""
Start-Process $ExtensionUrl -ErrorAction SilentlyContinue

if ($missing.Count -gt 0) { exit 1 }
exit 0
