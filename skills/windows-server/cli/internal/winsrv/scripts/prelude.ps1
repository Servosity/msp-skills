# windows-server-pp-cli prelude. Constant text: every script the CLI runs is
# assembled from embedded files only. Arguments arrive as base64 JSON in
# $env:WSPP_ARGS, are parsed as data with ConvertFrom-Json, re-validated here,
# and only ever bound to cmdlet parameters; they are never concatenated into
# code or evaluated.
# Must stay compatible with Windows PowerShell 5.1 (no ??, ?., &&, ternary, -AsHashtable).
# FIRST, before any cmdlet runs (a cmdlet call can autoload a module): search
# modules only under the running PowerShell's own module folder, so a module
# planted in a user or third-party folder cannot shadow a core cmdlet.
$env:PSModulePath = [System.IO.Path]::Combine($PSHOME, 'Modules')
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'
$WarningPreference = 'SilentlyContinue'
$VerbosePreference = 'SilentlyContinue'
$InformationPreference = 'SilentlyContinue'
# Windows PowerShell 5.1 can serialize arrays as {"value":[...],"Count":n}; dropping the
# ETS type data for System.Array in this process restores plain JSON arrays.
try { Remove-TypeData -TypeName System.Array -ErrorAction SilentlyContinue } catch { }

$A = New-Object PSObject
if ($env:WSPP_ARGS) { $A = ConvertFrom-Json -InputObject ([Text.Encoding]::UTF8.GetString([Convert]::FromBase64String($env:WSPP_ARGS))) }

function Arg([string]$name, $default) {
  $p = $A.PSObject.Properties[$name]
  if ($null -ne $p -and $null -ne $p.Value) { return $p.Value }
  return $default
}

function Out-Envelope($obj) {
  $json = ConvertTo-Json -InputObject $obj -Depth 10 -Compress
  $bytes = (New-Object System.Text.UTF8Encoding($false)).GetBytes($json)
  $out = [Console]::OpenStandardOutput()
  $out.Write($bytes, 0, $bytes.Length)
  $out.Flush()
}

function Emit-Ok($data) {
  Out-Envelope ([ordered]@{ ok = $true; data = $data })
  exit 0
}

function Emit-Err([string]$kind, [string]$msg) {
  Out-Envelope ([ordered]@{ ok = $false; error = [ordered]@{ kind = $kind; message = $msg } })
  exit 0
}

function Fail($err) {
  $ex = $err.Exception
  $kind = 'error'
  $tn = [string]$ex.GetType().FullName
  $fq = [string]$err.FullyQualifiedErrorId
  $cat = [string]$err.CategoryInfo.Category
  if ($ex -is [System.UnauthorizedAccessException] -or $tn -like '*UnauthorizedAccess*' -or $cat -eq 'PermissionDenied') { $kind = 'access_denied' }
  elseif ($ex -is [System.Management.Automation.CommandNotFoundException] -or $fq -like 'CommandNotFoundException*') { $kind = 'unavailable' }
  elseif ($fq -like 'NoMatchingEventsFound*' -or $cat -eq 'ObjectNotFound') { $kind = 'not_found' }
  elseif ($cat -eq 'ResourceUnavailable') { $kind = 'unavailable' }
  Emit-Err $kind ([string]$ex.Message)
}

function Need-Enum([string]$name, [string]$value, [string[]]$allowed) {
  if ($allowed -notcontains $value) { Emit-Err 'invalid_argument' ($name + ' is not one of the allowed values') }
  return $value
}

function Need-Int([string]$name, $value, [int]$min, [int]$max) {
  $n = 0
  if (-not [int]::TryParse([string]$value, [ref]$n) -or $n -lt $min -or $n -gt $max) { Emit-Err 'invalid_argument' ($name + ' is out of range') }
  return $n
}

function Need-Name([string]$name, $value) {
  $s = [string]$value
  if ($s -notmatch '^[A-Za-z0-9_.$-]{1,80}$') { Emit-Err 'invalid_argument' ($name + ' is not a valid service name') }
  return $s
}

function Iso($d) {
  if ($null -eq $d) { return $null }
  try { $dt = [datetime]$d } catch { return $null }
  if ($dt -le [datetime]'1601-01-02' -or $dt.Year -lt 2000) { return $null }
  return $dt.ToUniversalTime().ToString('o')
}

function Str($v) {
  if ($null -eq $v) { return $null }
  return [string]$v
}
