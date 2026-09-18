# run.ps1 - behavioural fixture suite for skills/<slug>/install.ps1.
#
# Runs the shipped installer (default: skills/hudu/install.ps1; every other
# slug is the same script modulo slug, proven by render_fleet.py --check) in a
# fresh PowerShell process per case (through harness.ps1, which can inject a
# Move-Item failure at the n-th call) against fake_github.py, a local stand-in
# for api.github.com + github.com. Each case asserts the exit status AND, after
# every rejection, that the pre-existing binaries in INSTALL_DIR are
# byte-identical and no lock, staging directory or backup is left behind.
#
# Windows PowerShell 5.1 in CI (windows-latest, shell: powershell); pwsh on a
# Mac/Linux is a smoke test only (the running-executable case is Windows-only).
#
# Usage: powershell -File tools/maintainer/installer_fixtures/run.ps1 [-Slug hudu]
param([string]$Slug = "hudu")

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"

$Here = Split-Path -Parent $MyInvocation.MyCommand.Path
$Root = (Resolve-Path (Join-Path $Here "..\..\..")).Path
$Installer = Join-Path $Root "skills\$Slug\install.ps1"
if (-not (Test-Path -LiteralPath $Installer)) { throw "no installer at $Installer" }
$Harness = Join-Path $Here "harness.ps1"
$Fake = Join-Path $Here "fake_github.py"
$PS = (Get-Process -Id $PID).Path
$IsWin = ($env:OS -eq "Windows_NT")
# Resolve a REAL Python: on Windows prefer the py launcher, then python /
# python3, and reject the Microsoft Store app-execution-alias stub under
# \WindowsApps\ (it exits silently with no stderr).
$Py = $null; $PyArgs = @()
if ($IsWin) {
  $launcher = Get-Command py -ErrorAction SilentlyContinue
  if ($launcher -and ($launcher.Source -notlike "*\WindowsApps\*")) { $Py = $launcher.Source; $PyArgs = @("-3") }
}
if (-not $Py) {
  foreach ($name in @("python", "python3")) {
    $c = Get-Command $name -ErrorAction SilentlyContinue
    if ($c -and ($c.Source -notlike "*\WindowsApps\*")) { $Py = $c.Source; break }
  }
}
if (-not $Py) { throw "a real python is required for fake_github.py (the Microsoft Store stub does not count)" }

$arch = "amd64"
if ($env:PROCESSOR_ARCHITECTURE -eq "ARM64") { $arch = "arm64" }
$CliAsset = "$Slug-cli-windows-$arch.exe"
$McpAsset = "$Slug-mcp-windows-$arch.exe"
$CliBin = "$Slug-cli.exe"
$McpBin = "$Slug-mcp.exe"
$Tag = "$Slug-v9.9.9"

$Work = Join-Path ([System.IO.Path]::GetTempPath()) ("msp-fix-" + [System.IO.Path]::GetRandomFileName().Replace('.', ''))
New-Item -ItemType Directory -Path $Work | Out-Null
$script:Server = $null
function Stop-Server {
  if ($script:Server) {
    try { Stop-Process -Id $script:Server.Id -Force -ErrorAction SilentlyContinue } catch { }
    try { $script:Server.WaitForExit(5000) | Out-Null } catch { }
    $script:Server = $null
  }
}

function Get-Sha256Lower([string]$Path) { return (Get-FileHash -Algorithm SHA256 -LiteralPath $Path).Hash.ToLower() }
function Write-Text([string]$Path, [string]$Text) { [System.IO.File]::WriteAllText($Path, $Text) }

# New-Assets DIR: fresh "new" binaries plus correct sidecars.
function New-Assets([string]$Dir) {
  New-Item -ItemType Directory -Force -Path $Dir | Out-Null
  Write-Text (Join-Path $Dir $CliAsset) "new-cli $(Get-Random)`n"
  Write-Text (Join-Path $Dir $McpAsset) "new-mcp $(Get-Random)`n"
  foreach ($a in @($CliAsset, $McpAsset)) {
    Write-Text (Join-Path $Dir "$a.sha256") ("{0}  {1}`n" -f (Get-Sha256Lower (Join-Path $Dir $a)), $a)
  }
}

# Start-Server ASSETS [fake_github.py flags...]; sets $script:ApiBase and $script:Log.
function Start-Server([string]$Assets, [string[]]$Extra = @()) {
  Stop-Server
  $portFile = Join-Path $Work ("port." + (Get-Random))
  $script:Log = Join-Path $Work ("requests." + (Get-Random) + ".log")
  Write-Text $script:Log ""
  $argList = $PyArgs + @($Fake, "--port-file", $portFile, "--assets", $Assets, "--slug", $Slug, "--tag", $Tag, "--log", $script:Log) + $Extra
  $errPath = Join-Path $Work "server.err"
  $script:Server = Start-Process -FilePath $Py -ArgumentList $argList -PassThru -NoNewWindow -RedirectStandardError $errPath
  # Up to 30 s for a cold Windows python start; fail fast if the process dies.
  for ($i = 0; $i -lt 600 -and -not (Test-Path -LiteralPath $portFile); $i++) {
    if ($script:Server.HasExited) { break }
    Start-Sleep -Milliseconds 50
  }
  if (-not (Test-Path -LiteralPath $portFile)) {
    $why = if ($script:Server.HasExited) { "exited with code $($script:Server.ExitCode)" } else { "still running after 30 s" }
    throw "fake_github.py did not start ($Py $($PyArgs -join ' '); $why): $(Get-Content -LiteralPath $errPath -Raw -ErrorAction SilentlyContinue)"
  }
  $script:ApiBase = "http://127.0.0.1:" + (Get-Content -LiteralPath $portFile -Raw).Trim()
}

$script:Pass = 0; $script:Fail = 0; $script:Cur = ""; $script:CurOk = $true
function Begin-Case([string]$Name) { $script:Cur = $Name; $script:CurOk = $true }
function Check([string]$Desc, [bool]$Cond) { if (-not $Cond) { $script:CurOk = $false; Write-Host "    FAIL: $($script:Cur): $Desc" } }
function End-Case {
  if ($script:CurOk) { $script:Pass++; Write-Host "  PASS  $($script:Cur)" }
  else {
    $script:Fail++; Write-Host "  FAIL  $($script:Cur)"
    if (Test-Path -LiteralPath $script:Out) { Get-Content -LiteralPath $script:Out | Select-Object -First 40 | ForEach-Object { Write-Host "        | $_" } }
  }
}

# New-InstallDir NAME: an INSTALL_DIR with pre-existing "old" binaries; snapshots kept.
function New-InstallDir([string]$Name) {
  $script:Dir = Join-Path $Work "cases\$Name"
  New-Item -ItemType Directory -Force -Path $script:Dir | Out-Null
  Write-Text (Join-Path $script:Dir $CliBin) "old-cli $Name`n"
  Write-Text (Join-Path $script:Dir $McpBin) "old-mcp $Name`n"
  Copy-Item -LiteralPath (Join-Path $script:Dir $CliBin) -Destination (Join-Path $Work "cases\$Name.old-cli")
  Copy-Item -LiteralPath (Join-Path $script:Dir $McpBin) -Destination (Join-Path $Work "cases\$Name.old-mcp")
  $script:Name = $Name
}

$EnvKeys = @("MSP_SKILLS_API_BASE", "MSP_SKILLS_RELEASE_BASE", "MSP_SKILLS_ALLOW_MUTABLE", "DRY_RUN", "GITHUB_TOKEN", "GH_TOKEN", "FIXTURE_FAIL_MOVE_AT", "INSTALL_DIR")
# Invoke-Installer @{ VAR = value }: fresh process via harness.ps1; sets $script:Rc and $script:Out.
function Invoke-Installer([hashtable]$Vars = @{}) {
  foreach ($k in $EnvKeys) { [Environment]::SetEnvironmentVariable($k, $null, "Process") }
  [Environment]::SetEnvironmentVariable("INSTALL_DIR", $script:Dir, "Process")
  foreach ($k in $Vars.Keys) { [Environment]::SetEnvironmentVariable($k, [string]$Vars[$k], "Process") }
  $script:Out = Join-Path $Work ("out." + (Get-Random))
  $errFile = "$($script:Out).err"
  $p = Start-Process -FilePath $PS -ArgumentList @("-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-File", $Harness, "-Installer", $Installer) `
    -PassThru -NoNewWindow -Wait -RedirectStandardOutput $script:Out -RedirectStandardError $errFile
  $script:Rc = $p.ExitCode
  if (Test-Path -LiteralPath $errFile) { Add-Content -LiteralPath $script:Out -Value (Get-Content -LiteralPath $errFile -Raw) }
  foreach ($k in $EnvKeys) { [Environment]::SetEnvironmentVariable($k, $null, "Process") }
}

function Same([string]$A, [string]$B) {
  if (-not (Test-Path -LiteralPath $A) -or -not (Test-Path -LiteralPath $B)) { return $false }
  return ([System.IO.File]::ReadAllBytes($A) -join ",") -eq ([System.IO.File]::ReadAllBytes($B) -join ",")
}
function Unchanged { return (Same (Join-Path $script:Dir $CliBin) (Join-Path $Work "cases\$($script:Name).old-cli")) -and (Same (Join-Path $script:Dir $McpBin) (Join-Path $Work "cases\$($script:Name).old-mcp")) }
function Installed { return (Same (Join-Path $script:Dir $CliBin) (Join-Path $script:Assets $CliAsset)) -and (Same (Join-Path $script:Dir $McpBin) (Join-Path $script:Assets $McpAsset)) }
function Clean {
  $leftovers = @(Get-ChildItem -LiteralPath $script:Dir -Force | Where-Object { $_.Name -like ".msp-install*" -or $_.Name -like "*.prev.*" })
  return ($leftovers.Count -eq 0)
}
function OutHas([string]$Text) { return ((Get-Content -LiteralPath $script:Out -Raw) -like "*$Text*") }
function LogHas([string]$Text) { return ((Get-Content -LiteralPath $script:Log -Raw) -like "*$Text*") }
function LogLacks([string]$Text) { return -not (LogHas $Text) }

Write-Host "installer fixtures: $Installer (windows/$arch, host $($PSVersionTable.PSVersion))"
try {

# ---------------------------------------------------------------- happy path
Begin-Case "happy path installs both binaries, hashes verified, backups gone"
$script:Assets = Join-Path $Work "assets.happy"; New-Assets $script:Assets; Start-Server $script:Assets
New-InstallDir "happy"; Invoke-Installer @{ MSP_SKILLS_API_BASE = $script:ApiBase }
Check "exit 0" ($script:Rc -eq 0)
Check "both binaries replaced" (Installed)
Check "sealed reported" (OutHas "(sealed)")
Check "both verified" (OutHas "verified $McpAsset")
Check "clean" (Clean)
Check "tag endpoint consulted" (LogHas "/releases/tags/$Tag")
Check "tag found on page 2 (pagination exercised)" (LogHas "page=2")
End-Case

# ---------------------------------------------------------------- sidecar cases
Begin-Case "tampered sidecar (hash altered) is refused"
$script:Assets = Join-Path $Work "assets.tampered"; New-Assets $script:Assets
$sc = Join-Path $script:Assets "$McpAsset.sha256"; $h = (Get-Content -LiteralPath $sc -Raw).Split(' ')[0]
$x = if ($h[0] -eq '0') { '1' } else { '0' }
Write-Text $sc ("{0}{1}  {2}`n" -f $x, $h.Substring(1), $McpAsset)
Start-Server $script:Assets; New-InstallDir "tampered"; Invoke-Installer @{ MSP_SKILLS_API_BASE = $script:ApiBase }
Check "non-zero" ($script:Rc -ne 0); Check "mismatch named" (OutHas "SHA-256 MISMATCH for $McpAsset")
Check "unchanged" (Unchanged); Check "clean" (Clean)
End-Case

Begin-Case "tampered binary (sidecar honest) is refused"
$script:Assets = Join-Path $Work "assets.tbin"; New-Assets $script:Assets
Add-Content -LiteralPath (Join-Path $script:Assets $CliAsset) -Value "evil"
Start-Server $script:Assets; New-InstallDir "tbin"; Invoke-Installer @{ MSP_SKILLS_API_BASE = $script:ApiBase }
Check "non-zero" ($script:Rc -ne 0); Check "mismatch named" (OutHas "SHA-256 MISMATCH for $CliAsset")
Check "unchanged" (Unchanged); Check "clean" (Clean)
End-Case

Begin-Case "sidecar hash in UPPER case still verifies (case-insensitive compare)"
$script:Assets = Join-Path $Work "assets.upper"; New-Assets $script:Assets
$sc = Join-Path $script:Assets "$CliAsset.sha256"; Write-Text $sc ((Get-Content -LiteralPath $sc -Raw).ToUpper().Replace($CliAsset.ToUpper(), $CliAsset))
Start-Server $script:Assets; New-InstallDir "upper"; Invoke-Installer @{ MSP_SKILLS_API_BASE = $script:ApiBase }
Check "exit 0" ($script:Rc -eq 0); Check "installed" (Installed); Check "clean" (Clean)
End-Case

Begin-Case "missing sidecar (404) is refused"
$script:Assets = Join-Path $Work "assets.missing"; New-Assets $script:Assets
Remove-Item -LiteralPath (Join-Path $script:Assets "$McpAsset.sha256")
Start-Server $script:Assets; New-InstallDir "missing"; Invoke-Installer @{ MSP_SKILLS_API_BASE = $script:ApiBase }
Check "non-zero" ($script:Rc -ne 0); Check "download failure named" (OutHas "Download failed:")
Check "names the sidecar" (OutHas "$McpAsset.sha256")
Check "unchanged" (Unchanged); Check "clean" (Clean)
End-Case

Begin-Case "sidecar naming another asset is refused"
$script:Assets = Join-Path $Work "assets.wrongname"; New-Assets $script:Assets
Write-Text (Join-Path $script:Assets "$CliAsset.sha256") ("{0}  {1}`n" -f (Get-Sha256Lower (Join-Path $script:Assets $CliAsset)), $McpAsset)
Start-Server $script:Assets; New-InstallDir "wrongname"; Invoke-Installer @{ MSP_SKILLS_API_BASE = $script:ApiBase }
Check "non-zero" ($script:Rc -ne 0); Check "name mismatch named" (OutHas "names '$McpAsset', not '$CliAsset'")
Check "unchanged" (Unchanged); Check "clean" (Clean)
End-Case

Begin-Case "sidecar with two records is refused (exactly one interpreted)"
$script:Assets = Join-Path $Work "assets.two"; New-Assets $script:Assets
Add-Content -LiteralPath (Join-Path $script:Assets "$CliAsset.sha256") -Value ("{0}  {1}" -f (Get-Sha256Lower (Join-Path $script:Assets $McpAsset)), $McpAsset)
Start-Server $script:Assets; New-InstallDir "two"; Invoke-Installer @{ MSP_SKILLS_API_BASE = $script:ApiBase }
Check "non-zero" ($script:Rc -ne 0); Check "line count named" (OutHas "has 2 lines; expected exactly one")
Check "unchanged" (Unchanged); Check "clean" (Clean)
End-Case

Begin-Case "sidecar whose record is not a SHA-256 is refused"
$script:Assets = Join-Path $Work "assets.nothex"; New-Assets $script:Assets
Write-Text (Join-Path $script:Assets "$CliAsset.sha256") "deadbeef  $CliAsset`n"
Start-Server $script:Assets; New-InstallDir "nothex"; Invoke-Installer @{ MSP_SKILLS_API_BASE = $script:ApiBase }
Check "non-zero" ($script:Rc -ne 0); Check "not a record named" (OutHas "is not a SHA-256 record")
Check "unchanged" (Unchanged); Check "clean" (Clean)
End-Case

# ---------------------------------------------------------------- immutability
Begin-Case "top-level immutable:true with a nested immutable:false is sealed (field, not substring)"
$script:Assets = Join-Path $Work "assets.imm-nested"; New-Assets $script:Assets; Start-Server $script:Assets @("--immutable", "nested")
New-InstallDir "imm-nested"; Invoke-Installer @{ MSP_SKILLS_API_BASE = $script:ApiBase }
Check "exit 0" ($script:Rc -eq 0); Check "sealed reported" (OutHas "(sealed)"); Check "installed" (Installed); Check "clean" (Clean)
End-Case

foreach ($mode in @("missing", "string", "null", "nested-only", "broken", "nul")) {
  Begin-Case "immutable field $mode is refused"
  $script:Assets = Join-Path $Work "assets.imm-$mode"; New-Assets $script:Assets; Start-Server $script:Assets @("--immutable", $mode)
  New-InstallDir "imm-$mode"; Invoke-Installer @{ MSP_SKILLS_API_BASE = $script:ApiBase }
  Check "non-zero" ($script:Rc -ne 0)
  if ($mode -eq "broken" -or $mode -eq "nul") { Check "malformed JSON named" (OutHas "malformed JSON") } else { Check "ambiguous named" (OutHas "refusing as ambiguous") }
  Check "unchanged" (Unchanged); Check "clean" (Clean); Check "nothing downloaded" (LogLacks "/releases/download/")
  End-Case
}

Begin-Case "immutable:false without the hatch is refused"
$script:Assets = Join-Path $Work "assets.mut"; New-Assets $script:Assets; Start-Server $script:Assets @("--immutable", "false")
New-InstallDir "mut"; Invoke-Installer @{ MSP_SKILLS_API_BASE = $script:ApiBase }
Check "non-zero" ($script:Rc -ne 0); Check "not sealed named" (OutHas "is NOT sealed")
Check "unchanged" (Unchanged); Check "clean" (Clean); Check "nothing downloaded" (LogLacks "/releases/download/")
End-Case

Begin-Case "immutable:false with MSP_SKILLS_ALLOW_MUTABLE=1 installs"
$script:Assets = Join-Path $Work "assets.hatch"; New-Assets $script:Assets; Start-Server $script:Assets @("--immutable", "false")
New-InstallDir "hatch"; Invoke-Installer @{ MSP_SKILLS_API_BASE = $script:ApiBase; MSP_SKILLS_ALLOW_MUTABLE = "1" }
Check "exit 0" ($script:Rc -eq 0); Check "warning printed" (OutHas "skipping the sealed-release check")
Check "installed" (Installed); Check "clean" (Clean)
End-Case

Begin-Case "the hatch does not waive hash verification"
$script:Assets = Join-Path $Work "assets.hatchhash"; New-Assets $script:Assets
Add-Content -LiteralPath (Join-Path $script:Assets $McpAsset) -Value "evil"
Start-Server $script:Assets @("--immutable", "false")
New-InstallDir "hatchhash"; Invoke-Installer @{ MSP_SKILLS_API_BASE = $script:ApiBase; MSP_SKILLS_ALLOW_MUTABLE = "1" }
Check "non-zero" ($script:Rc -ne 0); Check "mismatch named" (OutHas "SHA-256 MISMATCH")
Check "unchanged" (Unchanged); Check "clean" (Clean)
End-Case

Begin-Case "MSP_SKILLS_RELEASE_BASE override cannot claim immutability"
$script:Assets = Join-Path $Work "assets.rb"; New-Assets $script:Assets; Start-Server $script:Assets
New-InstallDir "rb"; Invoke-Installer @{ MSP_SKILLS_RELEASE_BASE = "$($script:ApiBase)/servosity/msp-skills/releases/download/$Tag" }
Check "non-zero" ($script:Rc -ne 0); Check "override named" (OutHas "MSP_SKILLS_RELEASE_BASE is set")
Check "unchanged" (Unchanged); Check "clean" (Clean); Check "no API call" (LogLacks "/repos/")
End-Case

Begin-Case "MSP_SKILLS_RELEASE_BASE override with the hatch installs (hashes still verified)"
$script:Assets = Join-Path $Work "assets.rbh"; New-Assets $script:Assets; Start-Server $script:Assets
New-InstallDir "rbh"; Invoke-Installer @{ MSP_SKILLS_RELEASE_BASE = "$($script:ApiBase)/servosity/msp-skills/releases/download/$Tag"; MSP_SKILLS_ALLOW_MUTABLE = "1" }
Check "exit 0" ($script:Rc -eq 0); Check "installed" (Installed); Check "clean" (Clean); Check "no API call" (LogLacks "/repos/")
End-Case

# ---------------------------------------------------------------- API failures
Begin-Case "API 500 on the releases list is an API error, not 'no release'"
$script:Assets = Join-Path $Work "assets.api500"; New-Assets $script:Assets; Start-Server $script:Assets @("--list-status", "500")
New-InstallDir "api500"; Invoke-Installer @{ MSP_SKILLS_API_BASE = $script:ApiBase }
Check "non-zero" ($script:Rc -ne 0); Check "API failure named" (OutHas "GitHub API request failed")
Check "not misreported" (-not (OutHas "release found"))
Check "unchanged" (Unchanged); Check "clean" (Clean)
End-Case

Begin-Case "API 500 on /releases/tags/<tag> stops the install"
$script:Assets = Join-Path $Work "assets.tag500"; New-Assets $script:Assets; Start-Server $script:Assets @("--tag-status", "500")
New-InstallDir "tag500"; Invoke-Installer @{ MSP_SKILLS_API_BASE = $script:ApiBase }
Check "non-zero" ($script:Rc -ne 0); Check "API failure named" (OutHas "GitHub API request failed")
Check "unchanged" (Unchanged); Check "clean" (Clean); Check "nothing downloaded" (LogLacks "/releases/download/")
End-Case

Begin-Case "a listing with no matching tag names the 500-release bound"
$script:Assets = Join-Path $Work "assets.norel"; New-Assets $script:Assets; Start-Server $script:Assets @("--slug", "nosuch", "--tag", "nosuch-v1.0.0")
New-InstallDir "norel"; Invoke-Installer @{ MSP_SKILLS_API_BASE = $script:ApiBase }
Check "non-zero" ($script:Rc -ne 0); Check "bound named" (OutHas "No $Slug-v* release found among the newest 500 releases")
Check "unchanged" (Unchanged); Check "clean" (Clean)
End-Case

Begin-Case "unreachable API is an API error"
New-InstallDir "apidown"; Stop-Server; Invoke-Installer @{ MSP_SKILLS_API_BASE = "http://127.0.0.1:9" }
Check "non-zero" ($script:Rc -ne 0); Check "API failure named" (OutHas "GitHub API request failed")
Check "unchanged" (Unchanged); Check "clean" (Clean)
End-Case

# ---------------------------------------------------------------- download
Begin-Case "truncated download is refused"
$script:Assets = Join-Path $Work "assets.trunc"; New-Assets $script:Assets; Start-Server $script:Assets @("--truncate", $CliAsset)
New-InstallDir "trunc"; Invoke-Installer @{ MSP_SKILLS_API_BASE = $script:ApiBase }
Check "non-zero" ($script:Rc -ne 0); Check "refused as download or hash failure" ((OutHas "Download failed") -or (OutHas "SHA-256 MISMATCH"))
Check "unchanged" (Unchanged); Check "clean" (Clean)
End-Case

# ---------------------------------------------------------------- rename boundaries
# Move-Item calls, in order: 1 old CLI -> prev, 2 new CLI -> dest, 3 old MCP -> prev, 4 new MCP -> dest.
foreach ($n in 1..4) {
  Begin-Case "failure at rename boundary $n restores both destinations"
  $script:Assets = Join-Path $Work "assets.ren$n"; New-Assets $script:Assets; Start-Server $script:Assets
  New-InstallDir "ren$n"; Invoke-Installer @{ MSP_SKILLS_API_BASE = $script:ApiBase; FIXTURE_FAIL_MOVE_AT = "$n" }
  Check "non-zero" ($script:Rc -ne 0); Check "injected failure surfaced" (OutHas "injected failure at call $n")
  if ($n -gt 1) { Check "restore reported" (OutHas "Previous binaries restored; nothing changed") }
  Check "unchanged" (Unchanged); Check "clean" (Clean)
  End-Case
}

Begin-Case "failed undo keeps and names the backup, exits non-zero"
$script:Assets = Join-Path $Work "assets.undo"; New-Assets $script:Assets; Start-Server $script:Assets
New-InstallDir "undo"; Invoke-Installer @{ MSP_SKILLS_API_BASE = $script:ApiBase; FIXTURE_FAIL_MOVE_AT = "4,5" }
Check "non-zero" ($script:Rc -ne 0); Check "restore incomplete reported" (OutHas "Restore incomplete. Backups kept")
$bk = @(Get-ChildItem -LiteralPath $script:Dir -Force | Where-Object { $_.Name -like "*.prev.*" })
Check "backup exists and is named" (($bk.Count -ge 1) -and (OutHas $bk[0].FullName))
Check "lock released" (-not (Test-Path -LiteralPath (Join-Path $script:Dir ".msp-install.lock")))
End-Case

# ---------------------------------------------------------------- locking
Begin-Case "a second installer is refused while the lock is held"
$script:Assets = Join-Path $Work "assets.lock"; New-Assets $script:Assets; Start-Server $script:Assets
New-InstallDir "lock"
$lockPath = Join-Path $script:Dir ".msp-install.lock"
$held = [System.IO.File]::Open($lockPath, [System.IO.FileMode]::OpenOrCreate, [System.IO.FileAccess]::ReadWrite, [System.IO.FileShare]::None)
try { Invoke-Installer @{ MSP_SKILLS_API_BASE = $script:ApiBase } } finally { $held.Close() }
Check "non-zero" ($script:Rc -ne 0); Check "lock named" (OutHas "Another installer is running")
Check "unchanged" (Unchanged); Check "lock not stolen" (Test-Path -LiteralPath $lockPath)
Check "nothing downloaded" (LogLacks "/releases/download/")
End-Case

Begin-Case "a stale lock (older than 1h) is reported, never stolen"
$script:Assets = Join-Path $Work "assets.stale"; New-Assets $script:Assets; Start-Server $script:Assets
New-InstallDir "stale"
$lockPath = Join-Path $script:Dir ".msp-install.lock"; Write-Text $lockPath "12345"
(Get-Item -LiteralPath $lockPath -Force).LastWriteTimeUtc = [DateTime]::UtcNow.AddHours(-2)
Invoke-Installer @{ MSP_SKILLS_API_BASE = $script:ApiBase }
Check "non-zero" ($script:Rc -ne 0); Check "stale named" (OutHas "Stale lock")
Check "unchanged" (Unchanged); Check "lock not stolen" (Test-Path -LiteralPath $lockPath)
End-Case

# ---------------------------------------------------------------- DRY_RUN
Begin-Case "DRY_RUN=1 exits 0, prints URLs, creates nothing, consults no tag/download"
$script:Assets = Join-Path $Work "assets.dry"; New-Assets $script:Assets; Start-Server $script:Assets
$script:Dir = Join-Path $Work "cases\dry\never-created"; Invoke-Installer @{ MSP_SKILLS_API_BASE = $script:ApiBase; DRY_RUN = "1" }
Check "exit 0" ($script:Rc -eq 0)
Check "CLI URL printed" (OutHas "CLI URL:      $($script:ApiBase)/servosity/msp-skills/releases/download/$Tag/$CliAsset")
Check "MCP URL printed" (OutHas "MCP URL:      $($script:ApiBase)/servosity/msp-skills/releases/download/$Tag/$McpAsset")
Check "install dir not created" (-not (Test-Path -LiteralPath $script:Dir))
Check "no tag fetch" (LogLacks "/releases/tags/"); Check "no download" (LogLacks "/releases/download/")
End-Case

Begin-Case "DRY_RUN=1 with MSP_SKILLS_RELEASE_BASE makes no network call at all"
Stop-Server; $script:Dir = Join-Path $Work "cases\drypin\never-created"
Invoke-Installer @{ MSP_SKILLS_API_BASE = "http://127.0.0.1:9"; MSP_SKILLS_RELEASE_BASE = "https://github.com/servosity/msp-skills/releases/download/$Tag"; DRY_RUN = "1" }
Check "exit 0" ($script:Rc -eq 0); Check "URL printed" (OutHas "github.com/servosity/msp-skills/releases/download/$Tag/$CliAsset")
Check "install dir not created" (-not (Test-Path -LiteralPath $script:Dir))
End-Case

# ---------------------------------------------------------------- token scoping
Begin-Case "GITHUB_TOKEN is not sent to an override API base"
$script:Assets = Join-Path $Work "assets.tok"; New-Assets $script:Assets; Start-Server $script:Assets
New-InstallDir "tok"; Invoke-Installer @{ MSP_SKILLS_API_BASE = $script:ApiBase; GITHUB_TOKEN = "ghp_test_should_not_leak" }
Check "exit 0" ($script:Rc -eq 0); Check "installed" (Installed)
Check "no Authorization header received (server)" (LogLacks "auth=1")
End-Case

# ---------------------------------------------------------------- Windows only
if ($IsWin) {
  Begin-Case "running executable at the destination: rename succeeds, delete-denied backup is reported"
  $script:Assets = Join-Path $Work "assets.running"; New-Assets $script:Assets; Start-Server $script:Assets
  New-InstallDir "running"
  $exe = Join-Path $Work "sleeper.exe"
  Add-Type -OutputType ConsoleApplication -OutputAssembly $exe -TypeDefinition @"
public static class Sleeper { public static int Main() { System.Threading.Thread.Sleep(120000); return 0; } }
"@
  Copy-Item -LiteralPath $exe -Destination (Join-Path $script:Dir $CliBin) -Force
  $proc = Start-Process -FilePath (Join-Path $script:Dir $CliBin) -PassThru -WindowStyle Hidden
  try {
    Invoke-Installer @{ MSP_SKILLS_API_BASE = $script:ApiBase }
    Check "exit 0" ($script:Rc -eq 0)
    Check "new CLI in place" (Same (Join-Path $script:Dir $CliBin) (Join-Path $script:Assets $CliAsset))
    Check "new MCP in place" (Same (Join-Path $script:Dir $McpBin) (Join-Path $script:Assets $McpAsset))
    $bk = @(Get-ChildItem -LiteralPath $script:Dir -Force | Where-Object { $_.Name -like "$CliBin.prev.*" })
    Check "backup left behind" ($bk.Count -eq 1)
    Check "backup reported by name" (($bk.Count -eq 1) -and (OutHas $bk[0].FullName))
    Check "no lock/staging left" (-not (Test-Path -LiteralPath (Join-Path $script:Dir ".msp-install.lock")))
  } finally {
    try { Stop-Process -Id $proc.Id -Force -ErrorAction SilentlyContinue } catch { }
  }
  End-Case
} else {
  Write-Host "  SKIP  running executable at the destination (Windows only)"
}

} finally {
  Stop-Server
  try { Remove-Item -LiteralPath $Work -Recurse -Force -ErrorAction SilentlyContinue } catch { }
}

Write-Host ""
Write-Host "installer fixtures: $($script:Pass) passed, $($script:Fail) failed"
if ($script:Fail -ne 0) { exit 1 }
exit 0
