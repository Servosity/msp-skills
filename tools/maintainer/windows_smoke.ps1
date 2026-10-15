<#
.SYNOPSIS
  Run a connector's Windows binaries the way a Windows MSP does, and fail on
  anything that does not work.

.DESCRIPTION
  Most MSPs run these connectors on Windows, and until this script nothing ever
  EXECUTED a Windows binary: CI cross-compiled windows/amd64 and windows/arm64
  on Linux and stopped there. A connector can compile cleanly for Windows and
  still fail on the first run - a path built from $HOME (which Windows does not
  set), a companion-CLI lookup that misses the ".exe" suffix, a store that
  cannot open, a binary that panics at startup. This script runs the real
  binaries on a real Windows machine and checks each of those.

  Checks, in order (each one runs even when an earlier one failed, so a single
  run names every defect):

    cli-version   <cli>.exe --version exits 0 and its last word equals
                  -ExpectedVersion (when given). Proves the binary starts and
                  that the release stamped the version into it.
    cli-help      <cli>.exe --help exits 0 and prints a usage block.
    cli-doctor    <cli>.exe doctor --json does not crash and prints a JSON
                  object. Any path it reports (config_path, cache.db_path) must
                  be absolute and inside the scratch profile this script gives
                  the process - a relative or "/.config/..." path means the code
                  built it from $HOME, which a Windows user does not have.
    mcp-initialize
                  the MCP server answers initialize over stdio, and its
                  serverInfo.version equals -ExpectedVersion (when given).
    mcp-tools-list
                  tools/list returns at least one tool.
    mcp-sibling-cli
                  one tools/call to workflow_status, a tool the server answers
                  by SHELLING OUT to the companion CLI (`<cli> workflow
                  status`, which every connector has and which works offline
                  against an empty local store). It must succeed with output:
                  any error - "companion CLI binary not found", an exec
                  failure, access denied - fails the check. The
                  server is launched from a different working directory with its
                  own directory removed from PATH, so only the sibling-of-
                  executable lookup (the one that needs "<x>-cli.exe") can find
                  the CLI. This is the path every Claude Desktop tool call takes.

  Environment the binaries see: a throwaway USERPROFILE, APPDATA and
  LOCALAPPDATA under a scratch directory, and NO HOME and no XDG_* variables -
  the environment Claude Desktop gives an MCP server on a stock Windows
  machine. Setting HOME here would hide exactly the $HOME bugs this exists to
  catch. No credentials are passed; none are needed.

  -Mcpb mode runs the same checks against a .mcpb bundle's Windows layout: the
  bundle is unpacked, manifest.json is read, and the server is launched from the
  path platform_overrides.win32.command declares (${__dirname} resolved to the
  unpack directory), exactly as Claude Desktop launches it. The companion CLI
  must sit beside it.

  Written for Windows PowerShell 5.1, the version every supported Windows ships
  with, and also runs under PowerShell 7.

.PARAMETER BinDir
  Directory holding <CliName>.exe and <McpName>.exe (the installer layout).
  Not used with -Mcpb.

.PARAMETER Mcpb
  Path to a .mcpb bundle. The win32 layout inside it is tested instead of
  -BinDir.

.EXAMPLE
  powershell -NoProfile -ExecutionPolicy Bypass -File tools\maintainer\windows_smoke.ps1 `
    -BinDir "$env:LOCALAPPDATA\Programs\msp-skills" -CliName hudu-cli -McpName hudu-mcp -ExpectedVersion 0.2.4
#>
param(
  [string]$BinDir = "",
  [string]$Mcpb = "",
  [Parameter(Mandatory = $true)][string]$CliName,
  [Parameter(Mandatory = $true)][string]$McpName,
  [string]$ExpectedVersion = "",
  [string]$Transcript = "",
  [int]$TimeoutSec = 60
)

$ErrorActionPreference = "Stop"
$ProgressPreference = "SilentlyContinue"
Set-StrictMode -Version 2.0

$script:Failures = New-Object System.Collections.ArrayList
$script:Passes = 0

function Write-Log {
  param([string]$Message)
  Write-Host $Message
  if ($Transcript) { Add-Content -LiteralPath $Transcript -Value $Message -Encoding UTF8 }
}

function Add-Result {
  param([string]$Check, [bool]$Ok, [string]$Detail)
  if ($Ok) {
    $script:Passes++
    Write-Log ("PASS  {0}: {1}" -f $Check, $Detail)
  } else {
    [void]$script:Failures.Add("${Check}: $Detail")
    Write-Log ("FAIL  {0}: {1}" -f $Check, $Detail)
    # GitHub Actions annotation; harmless anywhere else.
    Write-Host ("::error title=windows smoke {0}::{1}" -f $Check, ($Detail -replace "`r?`n", " "))
  }
}

# JSON parsing that survives large replies. Windows PowerShell 5.1's
# ConvertFrom-Json refuses anything over 2 MB (JavaScriptSerializer's default
# MaxJsonLength), and a connector with hundreds of tools has a tools/list reply
# bigger than that. Returns dictionaries and arrays in both editions.
function ConvertFrom-JsonText {
  param([string]$Text)
  if ($PSVersionTable.PSVersion.Major -ge 6) {
    return ($Text | ConvertFrom-Json -AsHashtable)
  }
  Add-Type -AssemblyName System.Web.Extensions
  $ser = New-Object System.Web.Script.Serialization.JavaScriptSerializer
  $ser.MaxJsonLength = [int]::MaxValue
  $ser.RecursionLimit = 1000
  return $ser.DeserializeObject($Text)
}

function Test-IsMap { param($Value) return ($Value -is [System.Collections.IDictionary]) }

# Works for a Hashtable (PowerShell 7), a generic Dictionary (5.1's
# JavaScriptSerializer) and anything else: a missing key or a non-map is $null.
# The leading comma matters: PowerShell unrolls an array on its way out of a
# function, so a one-element array (an MCP reply's usual one-item `content`,
# a one-tool tools/list) would otherwise come back as its single element and
# stop looking like a list.
function Get-Key {
  param($Map, [string]$Key)
  if ((Test-IsMap $Map) -and ($Map.Keys -ccontains $Key)) { return , ($Map[$Key]) }
  return $null
}

function Test-IsList {
  param($Value)
  return (($Value -is [System.Collections.IList]) -and -not (Test-IsMap $Value))
}

# --- the environment every child process gets --------------------------------
$scratch = Join-Path ([System.IO.Path]::GetTempPath()) ("msp-winsmoke-" + [System.IO.Path]::GetRandomFileName().Replace(".", ""))
$profileRoot = Join-Path $scratch "profile"
$workDir = Join-Path $scratch "cwd"
foreach ($d in @($profileRoot, (Join-Path $profileRoot "AppData\Roaming"), (Join-Path $profileRoot "AppData\Local"), $workDir)) {
  New-Item -ItemType Directory -Force -Path $d | Out-Null
}

function New-ChildStartInfo {
  param([string]$FileName, [string[]]$Arguments, [string]$StripFromPath)
  $psi = New-Object System.Diagnostics.ProcessStartInfo
  $psi.FileName = $FileName
  # Every argument here is a fixed literal from this script (no quoting needed).
  $psi.Arguments = ($Arguments -join " ")
  $psi.UseShellExecute = $false
  $psi.RedirectStandardInput = $true
  $psi.RedirectStandardOutput = $true
  $psi.RedirectStandardError = $true
  $psi.CreateNoWindow = $true
  $psi.StandardOutputEncoding = New-Object System.Text.UTF8Encoding($false)
  $psi.StandardErrorEncoding = New-Object System.Text.UTF8Encoding($false)
  $psi.WorkingDirectory = $workDir
  $envs = $psi.EnvironmentVariables
  foreach ($name in @($envs.Keys)) {
    # No HOME (a stock Windows user has none), no XDG overrides, and no
    # <SLUG>_CLI_PATH escape hatch: the sibling lookup must stand on its own.
    if ($name -ieq "HOME" -or $name -like "XDG_*" -or $name -like "*_CLI_PATH") { $envs.Remove($name) }
  }
  $envs["USERPROFILE"] = $profileRoot
  $envs["APPDATA"] = Join-Path $profileRoot "AppData\Roaming"
  $envs["LOCALAPPDATA"] = Join-Path $profileRoot "AppData\Local"
  $envs["NO_COLOR"] = "1"
  if ($StripFromPath) {
    $want = [System.IO.Path]::GetFullPath($StripFromPath).TrimEnd("\", "/")
    $kept = @()
    foreach ($p in ($envs["PATH"] -split ";")) {
      if (-not $p) { continue }
      $full = $p
      try { $full = [System.IO.Path]::GetFullPath($p).TrimEnd("\", "/") } catch { }
      if ($full -ine $want) { $kept += $p }
    }
    $envs["PATH"] = ($kept -join ";")
  }
  return $psi
}

# Run one command to completion with stdin closed. Returns exit code + output.
function Invoke-Child {
  param([string]$FileName, [string[]]$Arguments)
  $psi = New-ChildStartInfo -FileName $FileName -Arguments $Arguments -StripFromPath ""
  $p = New-Object System.Diagnostics.Process
  $p.StartInfo = $psi
  try {
    [void]$p.Start()
  } catch {
    return @{ Started = $false; ExitCode = -1; Stdout = ""; Stderr = $_.Exception.Message; TimedOut = $false }
  }
  $p.StandardInput.Close()
  $out = $p.StandardOutput.ReadToEndAsync()
  $err = $p.StandardError.ReadToEndAsync()
  $timedOut = -not $p.WaitForExit($TimeoutSec * 1000)
  if ($timedOut) { try { $p.Kill() } catch { } ; [void]$p.WaitForExit(5000) }
  [void]$out.Wait(5000); [void]$err.Wait(5000)
  $code = -1
  if (-not $timedOut) { $code = $p.ExitCode }
  return @{ Started = $true; ExitCode = $code; Stdout = $out.Result; Stderr = $err.Result; TimedOut = $timedOut }
}

function Test-Crashed {
  param($Result)
  return ($Result.Stderr -match "(?m)^panic:" -or $Result.Stderr -match "(?m)^goroutine \d+ \[")
}

# One MCP stdio session. Sends each message in turn and, for a request (it has
# an "id"), waits for the reply with that id before sending the next, so a
# slow tool call is never cut off by stdin closing. Returns the replies by id.
function Invoke-McpSession {
  param([string]$Server, [object[]]$Messages, [string]$ServerDir)
  $psi = New-ChildStartInfo -FileName $Server -Arguments @() -StripFromPath $ServerDir
  $p = New-Object System.Diagnostics.Process
  $p.StartInfo = $psi
  $replies = @{}
  $log = New-Object System.Collections.ArrayList
  try {
    [void]$p.Start()
  } catch {
    return @{ Started = $false; Replies = $replies; Stderr = $_.Exception.Message; Log = $log }
  }
  $err = $p.StandardError.ReadToEndAsync()
  $stdin = $p.StandardInput.BaseStream
  $utf8 = New-Object System.Text.UTF8Encoding($false)
  $pending = $null
  $deadline = [DateTime]::UtcNow.AddSeconds($TimeoutSec)
  $broken = $false
  foreach ($m in $Messages) {
    $json = $m.Json
    $bytes = $utf8.GetBytes($json + "`n")
    try { $stdin.Write($bytes, 0, $bytes.Length); $stdin.Flush() } catch { $broken = $true; break }
    if ($null -eq $m.Id) { continue }
    while (-not $replies.ContainsKey($m.Id)) {
      if ($null -eq $pending) { $pending = $p.StandardOutput.ReadLineAsync() }
      $left = [int](($deadline - [DateTime]::UtcNow).TotalMilliseconds)
      if ($left -le 0) { break }
      if (-not $pending.Wait($left)) { break }
      $line = $pending.Result
      $pending = $null
      if ($null -eq $line) { $broken = $true; break }   # stdout closed: the server exited
      $line = $line.Trim()
      if (-not $line.StartsWith("{")) { continue }
      [void]$log.Add($line)
      try { $msg = ConvertFrom-JsonText $line } catch { continue }
      $id = Get-Key $msg "id"
      if ($null -ne $id) { $replies[[int]$id] = $msg }
    }
    if ($broken -or -not $replies.ContainsKey($m.Id)) { break }
  }
  try { $p.StandardInput.Close() } catch { }
  if (-not $p.WaitForExit(10000)) { try { $p.Kill() } catch { } ; [void]$p.WaitForExit(5000) }
  [void]$err.Wait(5000)
  return @{ Started = $true; Replies = $replies; Stderr = $err.Result; Log = $log }
}

function New-Message {
  param($Id, [string]$Json)
  return @{ Id = $Id; Json = $Json }
}

$initialize = New-Message 1 '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"msp-skills-windows-smoke","version":"1"}}}'
$initialized = New-Message $null '{"jsonrpc":"2.0","method":"notifications/initialized"}'

# --- resolve what to run ------------------------------------------------------
if ($Transcript) {
  $tdir = Split-Path -Parent $Transcript
  if ($tdir) { New-Item -ItemType Directory -Force -Path $tdir | Out-Null }
}
Write-Log ("windows smoke: {0} / {1}  expected version: {2}" -f $CliName, $McpName, $(if ($ExpectedVersion) { $ExpectedVersion } else { "(any)" }))
Write-Log ("host: {0} {1}, PowerShell {2}" -f [System.Environment]::OSVersion.VersionString, $env:PROCESSOR_ARCHITECTURE, $PSVersionTable.PSVersion)

$serverCommand = $null
$cliPath = $null
if ($Mcpb) {
  if (-not (Test-Path -LiteralPath $Mcpb)) { throw "-Mcpb: no such file: $Mcpb" }
  # A space in the path, like Claude Desktop's "Claude Extensions" folder.
  $unpack = Join-Path $scratch "Claude Extensions\bundle"
  # Expand-Archive in 5.1 insists on a .zip extension.
  $zip = Join-Path $scratch "bundle.zip"
  Copy-Item -LiteralPath $Mcpb -Destination $zip
  Expand-Archive -LiteralPath $zip -DestinationPath $unpack -Force
  $manifest = ConvertFrom-JsonText ([System.IO.File]::ReadAllText((Join-Path $unpack "manifest.json")))
  $cfg = Get-Key (Get-Key $manifest "server") "mcp_config"
  $declared = Get-Key (Get-Key (Get-Key $cfg "platform_overrides") "win32") "command"
  $label = "server.mcp_config.platform_overrides.win32.command"
  if (-not $declared) { $declared = Get-Key $cfg "command"; $label = "server.mcp_config.command" }
  if (-not $declared) {
    Add-Result "mcpb-win32-command" $false "manifest.json declares no server command"
  } else {
    # Claude Desktop substitutes ${__dirname} and passes the rest through,
    # forward slashes included; Windows accepts them, so this does too.
    $serverCommand = $declared.Replace('${__dirname}', $unpack)
    $ok = Test-Path -LiteralPath $serverCommand
    Add-Result "mcpb-win32-command" $ok ("{0} = {1} -> {2}" -f $label, $declared, $(if ($ok) { "present" } else { "NOT IN THE BUNDLE" }))
    $serverDir = Split-Path -Parent ([System.IO.Path]::GetFullPath($serverCommand))
    $cliPath = Join-Path $serverDir "$CliName.exe"
    $okCli = Test-Path -LiteralPath $cliPath
    Add-Result "mcpb-win32-companion" $okCli ("{0}.exe beside the server: {1}" -f $CliName, $(if ($okCli) { "present" } else { "MISSING" }))
    if (-not $ok) { $serverCommand = $null }
    if (-not $okCli) { $cliPath = $null }
  }
} else {
  if (-not $BinDir) { throw "pass -BinDir (installer layout) or -Mcpb (bundle layout)" }
  $cliPath = Join-Path $BinDir "$CliName.exe"
  $serverCommand = Join-Path $BinDir "$McpName.exe"
  foreach ($f in @($cliPath, $serverCommand)) {
    if (-not (Test-Path -LiteralPath $f)) { throw "missing binary: $f" }
  }
}

# --- CLI checks ---------------------------------------------------------------
if ($cliPath) {
  $r = Invoke-Child $cliPath @("--version")
  $text = ($r.Stdout + "").Trim()
  $last = ""
  if ($text) { $last = ($text -split "\s+")[-1] }
  if (-not $r.Started) {
    Add-Result "cli-version" $false ("could not start {0}: {1}" -f $cliPath, $r.Stderr)
  } elseif ($r.TimedOut) {
    Add-Result "cli-version" $false "timed out after ${TimeoutSec}s"
  } elseif ($r.ExitCode -ne 0) {
    Add-Result "cli-version" $false ("exit {0}; stderr: {1}" -f $r.ExitCode, $r.Stderr.Trim())
  } elseif ($ExpectedVersion -and $last -cne $ExpectedVersion) {
    Add-Result "cli-version" $false ("printed '{0}', expected version '{1}'" -f $text, $ExpectedVersion)
  } elseif (-not $text) {
    Add-Result "cli-version" $false "printed nothing"
  } else {
    Add-Result "cli-version" $true $text
  }

  $r = Invoke-Child $cliPath @("--help")
  $ok = $r.Started -and -not $r.TimedOut -and $r.ExitCode -eq 0 -and ($r.Stdout -match "Usage:")
  Add-Result "cli-help" $ok $(if ($ok) { "exit 0 with a usage block" } else { "exit {0}, timed out {1}; stderr: {2}" -f $r.ExitCode, $r.TimedOut, ($r.Stderr + "").Trim() })

  $r = Invoke-Child $cliPath @("doctor", "--json")
  $doctor = $null
  $why = $null
  if (-not $r.Started) { $why = "could not start: $($r.Stderr)" }
  elseif ($r.TimedOut) { $why = "timed out after ${TimeoutSec}s" }
  elseif (Test-Crashed $r) { $why = "crashed: " + (($r.Stderr -split "`n" | Select-Object -First 3) -join " | ") }
  else {
    # doctor reports a missing credential as a finding; the exit code is not
    # the verdict here, a crash or unparseable output is.
    $docText = ($r.Stdout + "").Trim()
    if (-not $docText.StartsWith("{")) { $why = "stdout is not a JSON object (exit $($r.ExitCode)): " + ((($docText -split "`n") | Select-Object -First 2) -join " | ") }
    else { try { $doctor = ConvertFrom-JsonText $docText } catch { $why = "stdout is not JSON (exit $($r.ExitCode)): " + ((($docText -split "`n") | Select-Object -First 2) -join " | ") } }
    if (-not $why -and -not (Test-IsMap $doctor)) { $why = "stdout is JSON but not an object" }
  }
  if ($why) {
    Add-Result "cli-doctor" $false $why
  } else {
    $paths = @()
    $cp = Get-Key $doctor "config_path"
    if ($cp -is [string] -and $cp) { $paths += ,@("config_path", $cp) }
    $db = Get-Key (Get-Key $doctor "cache") "db_path"
    if ($db -is [string] -and $db) { $paths += ,@("cache.db_path", $db) }
    $bad = @()
    foreach ($pair in $paths) {
      $name = $pair[0]; $value = $pair[1]
      $rooted = $false
      try { $rooted = [System.IO.Path]::IsPathRooted($value) -and ($value -match '^(?:[A-Za-z]:[\\/]|\\\\)') } catch { }
      $inside = $false
      if ($rooted) {
        try { $inside = [System.IO.Path]::GetFullPath($value).StartsWith($profileRoot, [System.StringComparison]::OrdinalIgnoreCase) } catch { }
      }
      if (-not ($rooted -and $inside)) { $bad += "$name='$value'" }
    }
    if ($bad.Count -gt 0) {
      Add-Result "cli-doctor" $false ("path(s) not under the Windows user profile (built from `$HOME?): {0}; profile: {1}" -f ($bad -join ", "), $profileRoot)
    } else {
      $shown = @()
      foreach ($pair in $paths) { $shown += ("{0}={1}" -f $pair[0], $pair[1]) }
      Add-Result "cli-doctor" $true ("JSON report, exit {0}; {1}" -f $r.ExitCode, $(if ($shown.Count) { $shown -join "; " } else { "reports no paths" }))
    }
  }
}

# --- MCP checks ---------------------------------------------------------------
if ($serverCommand) {
  $serverDir = Split-Path -Parent ([System.IO.Path]::GetFullPath($serverCommand))
  $s = Invoke-McpSession -Server $serverCommand -ServerDir $serverDir -Messages @($initialize, $initialized, (New-Message 2 '{"jsonrpc":"2.0","id":2,"method":"tools/list"}'))
  if (-not $s.Started) {
    Add-Result "mcp-initialize" $false ("could not start {0}: {1}" -f $serverCommand, $s.Stderr)
  } else {
    $init = $null
    if ($s.Replies.ContainsKey(1)) { $init = $s.Replies[1] }
    $info = Get-Key (Get-Key $init "result") "serverInfo"
    $ver = Get-Key $info "version"
    if ($null -eq $init) {
      Add-Result "mcp-initialize" $false ("no reply to initialize within ${TimeoutSec}s; stderr: " + (((($s.Stderr + "").Trim()) -split "`n" | Select-Object -First 5) -join " | "))
    } elseif ($null -eq $info) {
      Add-Result "mcp-initialize" $false ("initialize reply has no serverInfo: " + ($s.Log | Select-Object -First 1))
    } elseif ($ExpectedVersion -and $ver -cne $ExpectedVersion) {
      Add-Result "mcp-initialize" $false ("serverInfo.version='{0}', expected '{1}'" -f $ver, $ExpectedVersion)
    } else {
      Add-Result "mcp-initialize" $true ("serverInfo {0} {1}" -f (Get-Key $info "name"), $ver)
    }

    $tools = @()
    if ($s.Replies.ContainsKey(2)) { $tools = Get-Key (Get-Key $s.Replies[2] "result") "tools" }
    $count = 0
    if (Test-IsList $tools) { $count = $tools.Count }
    Add-Result "mcp-tools-list" ($count -gt 0) ("{0} tool(s)" -f $count)

    # The shell-out tool. workflow_status mirrors `<cli> workflow status`,
    # which every connector has and which answers offline from the local store,
    # so it has exactly one healthy outcome: success with output. No fallback
    # to some other tool: a tool that may legitimately error offline cannot
    # tell "the CLI ran and said no" from "the CLI never started" (fail closed).
    $pick = $null
    if ($count -gt 0) {
      foreach ($t in $tools) { if ((Get-Key $t "name") -eq "workflow_status") { $pick = "workflow_status"; break } }
    }
    if (-not $pick) {
      Add-Result "mcp-sibling-cli" $false "tools/list has no workflow_status tool, so the companion-CLI shell-out cannot be exercised"
    } else {
      $call = New-Message 3 ('{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"' + $pick + '","arguments":{}}}')
      $c = Invoke-McpSession -Server $serverCommand -ServerDir $serverDir -Messages @($initialize, $initialized, $call)
      $reply = $null
      if ($c.Replies.ContainsKey(3)) { $reply = $c.Replies[3] }
      $result = Get-Key $reply "result"
      $texts = @()
      $content = Get-Key $result "content"
      if (Test-IsList $content) { foreach ($item in $content) { $tx = Get-Key $item "text"; if ($tx) { $texts += $tx } } }
      $body = ($texts -join "`n").Trim()
      $isError = (Get-Key $result "isError") -eq $true
      $notFound = $body -match "companion CLI binary not found|executable file not found|not a valid Win32 application|exec format error|The system cannot find the (file|path) specified"
      $brief = ($body -split "`n" | Select-Object -First 2) -join " | "
      if ($null -eq $reply) {
        Add-Result "mcp-sibling-cli" $false ("no reply to tools/call $pick within ${TimeoutSec}s; stderr: " + (((($c.Stderr + "").Trim()) -split "`n" | Select-Object -First 5) -join " | "))
      } elseif ($null -eq $result) {
        Add-Result "mcp-sibling-cli" $false ("tools/call $pick returned a protocol error: " + ($c.Log | Select-Object -Last 1))
      } elseif ($notFound) {
        Add-Result "mcp-sibling-cli" $false ("$pick could not run the companion CLI: $brief")
      } elseif ($isError) {
        Add-Result "mcp-sibling-cli" $false ("$pick answered an error: $brief")
      } elseif (-not $body) {
        Add-Result "mcp-sibling-cli" $false ("$pick returned no text; the companion CLI printed nothing")
      } else {
        Add-Result "mcp-sibling-cli" $true ("{0} shelled out to {1}.exe via the sibling lookup -> {2}" -f $pick, $CliName, $brief)
      }
    }
  }
}

Write-Log ""
if ($script:Failures.Count -gt 0) {
  Write-Log ("windows smoke FAILED: {0} check(s) failed, {1} passed" -f $script:Failures.Count, $script:Passes)
  foreach ($f in $script:Failures) { Write-Log "  - $f" }
  Remove-Item -LiteralPath $scratch -Recurse -Force -ErrorAction SilentlyContinue
  exit 1
}
Write-Log ("windows smoke PASSED: {0} check(s)" -f $script:Passes)
Remove-Item -LiteralPath $scratch -Recurse -Force -ErrorAction SilentlyContinue
exit 0
