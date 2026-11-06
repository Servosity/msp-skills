$svc = Get-Service -Name W32Time -ErrorAction SilentlyContinue
$state = $null
if ($null -ne $svc) { $state = [string]$svc.Status }
$lines = @()
if ($state -eq 'Running') { $lines = @(w32tm.exe /query /status /verbose | ForEach-Object { [string]$_ }) }
# w32tm prints the sync time in the server's own time zone and date format;
# only the server can read it back reliably, so it is converted here.
$sync = $null
foreach ($l in $lines) {
  if ($l -like 'Last Successful Sync Time:*') {
    try { $sync = [datetime]::Parse($l.Substring($l.IndexOf(':') + 1).Trim(), [Globalization.CultureInfo]::CurrentCulture).ToUniversalTime().ToString('o') } catch { $sync = $null }
  }
}
Emit-Ok ([ordered]@{ service_state = $state; lines = $lines; last_sync_utc = $sync })
