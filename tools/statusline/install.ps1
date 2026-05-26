# install.ps1 - install the canonical claude-code-statusline on Windows.
#
# Pulls statusline.py from github.com/Servosity/claude-code-statusline (MIT
# licensed; the Windows-fixing PRs that make this work are merged there) and
# writes it to %USERPROFILE%\.claude\statusline.py.
#
# Env vars:
#   STATUSLINE_RAW_URL  Override the source URL for testing.
#   DRY_RUN=1           Print the resolved URL and exit without downloading.

$ErrorActionPreference = "Stop"

$DefaultUrl = "https://raw.githubusercontent.com/Servosity/claude-code-statusline/main/statusline.py"
$SrcUrl = if ($env:STATUSLINE_RAW_URL) { $env:STATUSLINE_RAW_URL } else { $DefaultUrl }
$DestDir = Join-Path $env:USERPROFILE ".claude"
$Dest = Join-Path $DestDir "statusline.py"

Write-Host "Statusline source: $SrcUrl"
Write-Host "Destination:       $Dest"

if ($env:DRY_RUN -eq "1") {
  Write-Host "DRY_RUN=1 set; not downloading."
  exit 0
}

New-Item -ItemType Directory -Force -Path $DestDir | Out-Null
Invoke-WebRequest -Uri $SrcUrl -OutFile $Dest -UseBasicParsing

# Resolve a Python interpreter for the settings snippet below.
$PythonCmd = "python"
if (Get-Command python3 -ErrorAction SilentlyContinue) { $PythonCmd = "python3" }
elseif (-not (Get-Command python -ErrorAction SilentlyContinue)) {
  Write-Host ""
  Write-Host "NOTE: Neither python3 nor python was found on this machine."
  Write-Host "      Install Python 3 (https://www.python.org/downloads/) before"
  Write-Host "      Claude Code will be able to run the statusline."
}

# Convert Windows path to a form Claude Code can write into JSON.
$JsonPath = ($Dest -replace '\\', '\\\\')

Write-Host ""
Write-Host "Installed."
Write-Host ""
Write-Host "Wire it into Claude Code: open %USERPROFILE%\.claude\settings.json and add"
Write-Host "(merge with existing keys if the file exists):"
Write-Host ""
Write-Host "{"
Write-Host "  ""statusLine"": {"
Write-Host "    ""type"": ""command"","
Write-Host "    ""command"": ""$PythonCmd $JsonPath"""
Write-Host "  }"
Write-Host "}"
Write-Host ""
Write-Host "Then restart Claude Code. For options and full docs, see:"
Write-Host "  https://github.com/Servosity/claude-code-statusline"
