# harness.ps1 - runs one installer in a fresh PowerShell process with an
# optional Move-Item failure injection, then exits with the installer's status.
#
# FIXTURE_FAIL_MOVE_AT=<n>[,<n>...] makes the n-th Move-Item call throw. The
# function below shadows the cmdlet for the installer (functions win command
# resolution and child scopes see them), so the installer's transaction is
# exercised exactly as shipped.
param([Parameter(Mandatory = $true)][string]$Installer)

if ($env:FIXTURE_FAIL_MOVE_AT) {
  $global:FixtureMoveCalls = 0
  $global:FixtureFailAt = @(($env:FIXTURE_FAIL_MOVE_AT -split ',') | ForEach-Object { [int]$_ })
  function Move-Item {
    param([string]$LiteralPath, [string]$Destination)
    $global:FixtureMoveCalls++
    if ($global:FixtureFailAt -contains $global:FixtureMoveCalls) {
      throw "Move-Item: injected failure at call $($global:FixtureMoveCalls)"
    }
    Microsoft.PowerShell.Management\Move-Item -LiteralPath $LiteralPath -Destination $Destination -ErrorAction Stop
  }
}

try {
  & $Installer
  $code = $LASTEXITCODE
  if ($null -eq $code) { $code = 0 }
  exit $code
} catch {
  [Console]::Error.WriteLine("harness: uncaught: $($_.Exception.Message)")
  exit 1
}
